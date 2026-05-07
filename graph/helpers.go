package graph

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/fibegg/go-fibe/graph/model"
	"github.com/fibegg/go-fibe/internal/config"
	"github.com/fibegg/go-fibe/internal/jobs"
	"github.com/fibegg/go-fibe/internal/models"
	"github.com/fibegg/go-fibe/internal/security"
)

func validateMonitorInput(cfg config.Config, input model.MonitorInput) (models.MonitorInput, error) {
	name := cleanRequired(input.Name, 120)
	if name == "" {
		return models.MonitorInput{}, fmt.Errorf("name is required")
	}
	normalizedURL, err := security.NormalizeMonitorURL(cfg, input.URL)
	if err != nil {
		return models.MonitorInput{}, err
	}
	method := strings.ToUpper(valueOr(input.Method, "GET"))
	if method != "GET" && method != "HEAD" {
		return models.MonitorInput{}, fmt.Errorf("method is invalid")
	}
	expectedStatus := valueOr(input.ExpectedStatus, 200)
	if expectedStatus < 100 || expectedStatus > 599 {
		return models.MonitorInput{}, fmt.Errorf("expected status is invalid")
	}
	intervalSeconds := valueOr(input.IntervalSeconds, 60)
	if intervalSeconds < 15 || intervalSeconds > 86400 {
		return models.MonitorInput{}, fmt.Errorf("interval seconds is invalid")
	}
	return models.MonitorInput{
		Name:            name,
		URL:             normalizedURL,
		Method:          method,
		ExpectedStatus:  expectedStatus,
		IntervalSeconds: intervalSeconds,
		Enabled:         valueOr(input.Enabled, true),
	}, nil
}

func cleanRequired(value string, limit int) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(trimmed) > limit {
		trimmed = trimmed[:limit]
	}
	return trimmed
}

func valueOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}

func maintenanceJobType(name string) string {
	switch name {
	case "rebuild_rollups":
		return jobs.TypeRebuildRollups
	case "clear_cache":
		return jobs.TypeClearCache
	case "prune_events":
		return jobs.TypePruneEvents
	case "reseed_demo":
		return jobs.TypeReseedDemo
	default:
		return jobs.TypeClearCache
	}
}

func internalError(err error) error {
	slog.Error("internal GraphQL error", "error", err)
	return fmt.Errorf("internal server error")
}

func monitorPointers(items []models.Monitor) []*models.Monitor {
	result := make([]*models.Monitor, 0, len(items))
	for i := range items {
		result = append(result, &items[i])
	}
	return result
}

func checkEventPointers(items []models.CheckEvent) []*models.CheckEvent {
	result := make([]*models.CheckEvent, 0, len(items))
	for i := range items {
		result = append(result, &items[i])
	}
	return result
}

func incidentPointers(items []models.Incident) []*models.Incident {
	result := make([]*models.Incident, 0, len(items))
	for i := range items {
		result = append(result, &items[i])
	}
	return result
}

func jobRunPointers(items []models.JobRun) []*models.JobRun {
	result := make([]*models.JobRun, 0, len(items))
	for i := range items {
		result = append(result, &items[i])
	}
	return result
}

func maintenanceTaskPointers(items []models.MaintenanceTask) []*models.MaintenanceTask {
	result := make([]*models.MaintenanceTask, 0, len(items))
	for i := range items {
		result = append(result, &items[i])
	}
	return result
}

func maintenanceRunPointers(items []models.MaintenanceRun) []*models.MaintenanceRun {
	result := make([]*models.MaintenanceRun, 0, len(items))
	for i := range items {
		result = append(result, &items[i])
	}
	return result
}
