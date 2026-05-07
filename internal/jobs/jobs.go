package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/fibegg/go-fibe/internal/app"
	"github.com/fibegg/go-fibe/internal/cache"
	"github.com/fibegg/go-fibe/internal/config"
	"github.com/fibegg/go-fibe/internal/db"
	"github.com/fibegg/go-fibe/internal/event"
	"github.com/fibegg/go-fibe/internal/models"
	"github.com/fibegg/go-fibe/internal/security"
	"github.com/hibiken/asynq"
)

const (
	TypeCheckMonitor   = "check_monitor"
	TypeRebuildRollups = "rebuild_rollups"
	TypeClearCache     = "clear_cache"
	TypePruneEvents    = "prune_events"
	TypeReseedDemo     = "reseed_demo"
)

type Job struct {
	Type      string
	MonitorID string
}

type payload struct {
	JobRunID  string `json:"jobRunId"`
	MonitorID string `json:"monitorId,omitempty"`
}

func RunWorker(ctx context.Context, cfg config.Config) error {
	rt, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer rt.Close()

	redisOpt, err := cfg.RedisAsynqOpt()
	if err != nil {
		return err
	}
	server := asynq.NewServer(redisOpt, asynq.Config{Concurrency: cfg.WorkerConcurrency})
	mux := asynq.NewServeMux()
	handler := &handler{rt: rt}
	mux.HandleFunc(TypeCheckMonitor, handler.handleCheckMonitor)
	mux.HandleFunc(TypeRebuildRollups, handler.handleRebuildRollups)
	mux.HandleFunc(TypeClearCache, handler.handleClearCache)
	mux.HandleFunc(TypePruneEvents, handler.handlePruneEvents)
	mux.HandleFunc(TypeReseedDemo, handler.handleReseedDemo)

	go scheduleDueMonitorChecks(ctx, rt)
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(mux)
	}()

	select {
	case <-ctx.Done():
		server.Shutdown()
		return nil
	case err := <-errCh:
		return err
	}
}

func Enqueue(ctx context.Context, rt *app.App, job Job) (string, error) {
	jobRunID, err := rt.Store.InsertJobRun(ctx, job.Type, publicDetail(job.Type))
	if err != nil {
		return "", err
	}
	taskPayload, err := json.Marshal(payload{JobRunID: jobRunID, MonitorID: job.MonitorID})
	if err != nil {
		return "", err
	}
	task := asynq.NewTask(job.Type, taskPayload)
	if _, err := rt.AsynqClient.EnqueueContext(ctx, task); err != nil {
		detail := "failed to queue background job"
		_ = rt.Store.MarkJobFinished(ctx, jobRunID, "failed", &detail)
		return "", err
	}
	PublishEvent(ctx, rt, fmt.Sprintf("job:%s:queued", jobRunID))
	return jobRunID, nil
}

func PublishEvent(ctx context.Context, rt *app.App, value string) {
	if err := rt.Redis.Publish(ctx, event.Channel, value).Err(); err != nil {
		slog.Debug("failed to publish event through redis", "error", err)
		rt.Events.Publish(value)
	}
}

type handler struct {
	rt *app.App
}

func (h *handler) handleCheckMonitor(ctx context.Context, task *asynq.Task) error {
	data, err := decodePayload(task)
	if err != nil {
		return err
	}
	return h.perform(ctx, data.JobRunID, func() error {
		if data.MonitorID == "" {
			return errors.New("monitor id missing")
		}
		return CheckMonitor(ctx, h.rt, data.MonitorID)
	})
}

func (h *handler) handleRebuildRollups(ctx context.Context, task *asynq.Task) error {
	data, err := decodePayload(task)
	if err != nil {
		return err
	}
	return h.perform(ctx, data.JobRunID, func() error {
		return cache.Delete(ctx, h.rt.Redis, "cache:dashboard", "cache:public_status")
	})
}

func (h *handler) handleClearCache(ctx context.Context, task *asynq.Task) error {
	data, err := decodePayload(task)
	if err != nil {
		return err
	}
	return h.perform(ctx, data.JobRunID, func() error {
		return cache.Delete(ctx, h.rt.Redis, "cache:dashboard", "cache:public_status")
	})
}

func (h *handler) handlePruneEvents(ctx context.Context, task *asynq.Task) error {
	data, err := decodePayload(task)
	if err != nil {
		return err
	}
	return h.perform(ctx, data.JobRunID, func() error {
		return h.rt.Store.PruneEvents(ctx)
	})
}

func (h *handler) handleReseedDemo(ctx context.Context, task *asynq.Task) error {
	data, err := decodePayload(task)
	if err != nil {
		return err
	}
	return h.perform(ctx, data.JobRunID, func() error {
		return h.rt.Store.Seed(ctx, h.rt.Config.DemoPassword)
	})
}

func (h *handler) perform(ctx context.Context, jobRunID string, fn func() error) error {
	if err := h.rt.Store.MarkJobRunning(ctx, jobRunID); err != nil {
		return err
	}
	PublishEvent(ctx, h.rt, fmt.Sprintf("job:%s:running", jobRunID))
	err := fn()
	if err != nil {
		slog.Warn("background job failed", "error", err, "jobRunId", jobRunID)
		detail := "job failed; check server logs"
		_ = h.rt.Store.MarkJobFinished(ctx, jobRunID, "failed", &detail)
		PublishEvent(ctx, h.rt, fmt.Sprintf("job:%s:failed", jobRunID))
		return err
	}
	if err := h.rt.Store.MarkJobFinished(ctx, jobRunID, "succeeded", nil); err != nil {
		return err
	}
	cache.Delete(ctx, h.rt.Redis, "cache:dashboard")
	PublishEvent(ctx, h.rt, fmt.Sprintf("job:%s:succeeded", jobRunID))
	return nil
}

func CheckMonitor(ctx context.Context, rt *app.App, monitorID string) error {
	monitor, err := rt.Store.MonitorByID(ctx, monitorID)
	if err != nil {
		return err
	}
	if err := security.EnsureMonitorURLAllowed(ctx, rt.Config, monitor.URL); err != nil {
		message := "request blocked by URL safety policy"
		return recordCheckResult(ctx, rt, monitor, "down", nil, nil, &message)
	}

	req, err := http.NewRequestWithContext(ctx, monitor.Method, monitor.URL, nil)
	if err != nil {
		message := "invalid monitor request"
		return recordCheckResult(ctx, rt, monitor, "down", nil, nil, &message)
	}
	req.Header.Set("User-Agent", "uptime-console/0.1")
	client := &http.Client{Timeout: rt.Config.MonitorHTTPTimeout}
	started := time.Now()
	resp, err := client.Do(req)
	latency := int(time.Since(started).Milliseconds())
	if err != nil {
		message := "request failed"
		return recordCheckResult(ctx, rt, monitor, "down", &latency, nil, &message)
	}
	defer resp.Body.Close()

	statusCode := resp.StatusCode
	if statusCode == monitor.ExpectedStatus {
		message := fmt.Sprintf("expected HTTP %d", statusCode)
		return recordCheckResult(ctx, rt, monitor, "up", &latency, &statusCode, &message)
	}
	message := fmt.Sprintf("expected %d, got %d", monitor.ExpectedStatus, statusCode)
	return recordCheckResult(ctx, rt, monitor, "down", &latency, &statusCode, &message)
}

func recordCheckResult(ctx context.Context, rt *app.App, monitor models.Monitor, status string, latencyMs *int, statusCode *int, message *string) error {
	if err := rt.Store.InsertCheckEvent(ctx, monitor, status, latencyMs, statusCode, message); err != nil {
		return err
	}
	_ = cache.Delete(ctx, rt.Redis, "cache:dashboard", "cache:public_status")
	PublishEvent(ctx, rt, fmt.Sprintf("monitor:%s:%s", monitor.ID, status))
	return nil
}

func scheduleDueMonitorChecks(ctx context.Context, rt *app.App) {
	ticker := time.NewTicker(rt.Config.MonitorCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := enqueueDueMonitorChecks(ctx, rt); err != nil {
				slog.Warn("failed to enqueue due monitor checks", "error", err)
			}
		}
	}
}

func enqueueDueMonitorChecks(ctx context.Context, rt *app.App) error {
	monitors, err := rt.Store.DueMonitors(ctx, 10)
	if err != nil {
		return err
	}
	for _, monitor := range monitors {
		if _, err := Enqueue(ctx, rt, Job{Type: TypeCheckMonitor, MonitorID: monitor.ID}); err != nil {
			return err
		}
	}
	return nil
}

func decodePayload(task *asynq.Task) (payload, error) {
	var data payload
	err := json.Unmarshal(task.Payload(), &data)
	return data, err
}

func publicDetail(jobType string) string {
	switch jobType {
	case TypeCheckMonitor:
		return "check monitor"
	case TypeRebuildRollups:
		return "rebuild dashboard rollups"
	case TypeClearCache:
		return "clear application cache"
	case TypePruneEvents:
		return "prune old check events"
	case TypeReseedDemo:
		return "reseed demo data"
	default:
		return "background job"
	}
}

func IsNotFound(err error) bool {
	return db.IsNoRows(err)
}
