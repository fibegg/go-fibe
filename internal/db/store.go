package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/fibegg/go-fibe/internal/config"
	"github.com/fibegg/go-fibe/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func Connect(ctx context.Context, cfg config.Config) (*Store, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	poolConfig.MaxConns = cfg.DBMaxConnections
	poolConfig.MinConns = cfg.DBMinConnections
	poolConfig.HealthCheckPeriod = 30 * time.Second
	poolConfig.ConnConfig.ConnectTimeout = cfg.DBAcquireTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Seed(ctx context.Context, password string) error {
	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO users (email, name, role, password_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE SET
			name = EXCLUDED.name,
			role = EXCLUDED.role,
			password_hash = EXCLUDED.password_hash
	`, "admin@example.com", "Demo Admin", "admin", hash)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO monitors (name, url, expected_status, interval_seconds, enabled, last_status)
		VALUES
			('Primary Website', 'https://example.com', 200, 120, true, 'pending'),
			('Go Website', 'https://go.dev', 200, 300, true, 'pending'),
			('Documentation Site', 'https://pkg.go.dev', 200, 300, true, 'pending')
		ON CONFLICT (name, url) DO UPDATE SET
			expected_status = EXCLUDED.expected_status,
			interval_seconds = EXCLUDED.interval_seconds,
			enabled = EXCLUDED.enabled,
			updated_at = now()
	`)
	return err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (models.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		SELECT id::text, email, name, role, password_hash, created_at
		FROM users
		WHERE email = $1
	`, strings.ToLower(strings.TrimSpace(email))))
}

func (s *Store) UserByID(ctx context.Context, id string) (models.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		SELECT id::text, email, name, role, password_hash, created_at
		FROM users
		WHERE id = $1::uuid
	`, id))
}

func (s *Store) Dashboard(ctx context.Context) (models.Dashboard, error) {
	var row struct {
		monitorCount      int
		upCount           int
		downCount         int
		openIncidentCount int
		avgLatency        *float64
	}
	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*)::int AS monitor_count,
			COUNT(*) FILTER (WHERE last_status = 'up')::int AS up_count,
			COUNT(*) FILTER (WHERE last_status = 'down')::int AS down_count,
			(SELECT COUNT(*)::int FROM incidents WHERE status = 'open') AS open_incident_count,
			AVG(last_latency_ms)::float8 AS avg_latency_ms
		FROM monitors
	`).Scan(&row.monitorCount, &row.upCount, &row.downCount, &row.openIncidentCount, &row.avgLatency)
	if err != nil {
		return models.Dashboard{}, err
	}
	return models.Dashboard{
		MonitorCount:      row.monitorCount,
		UpCount:           row.upCount,
		DownCount:         row.downCount,
		OpenIncidentCount: row.openIncidentCount,
		AvgLatencyMs:      row.avgLatency,
	}, nil
}

func (s *Store) ListMonitors(ctx context.Context) ([]models.Monitor, error) {
	rows, err := s.pool.Query(ctx, monitorSelectSQL()+` ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMonitors(rows)
}

func (s *Store) DueMonitors(ctx context.Context, limit int) ([]models.Monitor, error) {
	rows, err := s.pool.Query(ctx, monitorSelectSQL()+`
		WHERE enabled = true
			AND (last_checked_at IS NULL OR last_checked_at < now() - (interval_seconds || ' seconds')::interval)
		ORDER BY COALESCE(last_checked_at, 'epoch'::timestamptz)
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMonitors(rows)
}

func (s *Store) MonitorByID(ctx context.Context, id string) (models.Monitor, error) {
	return scanMonitor(s.pool.QueryRow(ctx, monitorSelectSQL()+` WHERE id = $1::uuid`, id))
}

func (s *Store) MonitorExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM monitors WHERE id = $1::uuid)", id).Scan(&exists)
	return exists, err
}

func (s *Store) CheckEvents(ctx context.Context, monitorID string) ([]models.CheckEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, monitor_id::text, status, latency_ms, status_code, message, checked_at
		FROM check_events
		WHERE monitor_id = $1::uuid
		ORDER BY checked_at DESC
		LIMIT 50
	`, monitorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.CheckEvent
	for rows.Next() {
		var event models.CheckEvent
		if err := rows.Scan(&event.ID, &event.MonitorID, &event.Status, &event.LatencyMs, &event.StatusCode, &event.Message, &event.CheckedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) CreateMonitor(ctx context.Context, input models.MonitorInput) (models.Monitor, error) {
	return scanMonitor(s.pool.QueryRow(ctx, `
		INSERT INTO monitors (name, url, method, expected_status, interval_seconds, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, name, url, method, expected_status, interval_seconds, enabled, last_status,
			last_latency_ms, last_checked_at, created_at, updated_at
	`, input.Name, input.URL, input.Method, input.ExpectedStatus, input.IntervalSeconds, input.Enabled))
}

func (s *Store) DeleteMonitor(ctx context.Context, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx, "DELETE FROM monitors WHERE id = $1::uuid", id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ListIncidents(ctx context.Context) ([]models.Incident, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, monitor_id::text, title, status, severity, opened_at, resolved_at
		FROM incidents
		ORDER BY opened_at DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var incidents []models.Incident
	for rows.Next() {
		var incident models.Incident
		if err := rows.Scan(&incident.ID, &incident.MonitorID, &incident.Title, &incident.Status, &incident.Severity, &incident.OpenedAt, &incident.ResolvedAt); err != nil {
			return nil, err
		}
		incidents = append(incidents, incident)
	}
	return incidents, rows.Err()
}

func (s *Store) OpenIncident(ctx context.Context, input models.IncidentInput) (models.Incident, error) {
	return scanIncident(s.pool.QueryRow(ctx, `
		INSERT INTO incidents (monitor_id, title, severity)
		VALUES ($1::uuid, $2, $3)
		RETURNING id::text, monitor_id::text, title, status, severity, opened_at, resolved_at
	`, nullableUUID(input.MonitorID), input.Title, input.Severity))
}

func (s *Store) ResolveIncident(ctx context.Context, id string) (models.Incident, error) {
	return scanIncident(s.pool.QueryRow(ctx, `
		UPDATE incidents
		SET status = 'resolved', resolved_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, monitor_id::text, title, status, severity, opened_at, resolved_at
	`, id))
}

func (s *Store) InsertCheckEvent(ctx context.Context, monitor models.Monitor, status string, latencyMs *int, statusCode *int, message *string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO check_events (monitor_id, status, latency_ms, status_code, message)
		VALUES ($1::uuid, $2, $3, $4, $5)
	`, monitor.ID, status, latencyMs, statusCode, message); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE monitors
		SET last_status = $2, last_latency_ms = $3, last_checked_at = now(), updated_at = now()
		WHERE id = $1::uuid
	`, monitor.ID, status, latencyMs); err != nil {
		return err
	}
	if err := syncIncidentForCheck(ctx, tx, monitor, status, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) InsertJobRun(ctx context.Context, jobType, detail string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO job_runs (job_type, status, detail)
		VALUES ($1, 'queued', $2)
		RETURNING id::text
	`, jobType, detail).Scan(&id)
	return id, err
}

func (s *Store) MarkJobRunning(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, "UPDATE job_runs SET status = 'running', started_at = now() WHERE id = $1::uuid", id)
	return err
}

func (s *Store) MarkJobFinished(ctx context.Context, id, status string, detail *string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE job_runs
		SET status = $2, detail = COALESCE($3, detail), finished_at = now()
		WHERE id = $1::uuid
	`, id, status, detail)
	return err
}

func (s *Store) RecentJobRuns(ctx context.Context) ([]models.JobRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, job_type, status, detail, created_at, started_at, finished_at
		FROM job_runs
		ORDER BY created_at DESC
		LIMIT 20
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.JobRun
	for rows.Next() {
		var job models.JobRun
		if err := rows.Scan(&job.ID, &job.JobType, &job.Status, &job.Detail, &job.CreatedAt, &job.StartedAt, &job.FinishedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) InsertMaintenanceRun(ctx context.Context, name string) (models.MaintenanceRun, error) {
	return scanMaintenanceRun(s.pool.QueryRow(ctx, `
		INSERT INTO maintenance_runs (task_name, status)
		VALUES ($1, 'running')
		RETURNING id::text, task_name, status, output, started_at, finished_at
	`, name))
}

func (s *Store) CompleteMaintenanceRun(ctx context.Context, id, status string, output *string) (models.MaintenanceRun, error) {
	return scanMaintenanceRun(s.pool.QueryRow(ctx, `
		UPDATE maintenance_runs
		SET status = $2, output = $3, finished_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, task_name, status, output, started_at, finished_at
	`, id, status, output))
}

func (s *Store) RecentMaintenanceRuns(ctx context.Context) ([]models.MaintenanceRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, task_name, status, output, started_at, finished_at
		FROM maintenance_runs
		ORDER BY started_at DESC
		LIMIT 20
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []models.MaintenanceRun
	for rows.Next() {
		run, err := scanMaintenanceRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) PruneEvents(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM check_events WHERE checked_at < now() - interval '14 days'")
	return err
}

func IsNoRows(err error) bool {
	return err == pgx.ErrNoRows
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func monitorSelectSQL() string {
	return `
		SELECT id::text, name, url, method, expected_status, interval_seconds, enabled, last_status,
			last_latency_ms, last_checked_at, created_at, updated_at
		FROM monitors
	`
}

func scanUser(row pgx.Row) (models.User, error) {
	var user models.User
	err := row.Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.PasswordHash, &user.CreatedAt)
	return user, err
}

func scanMonitor(row pgx.Row) (models.Monitor, error) {
	var monitor models.Monitor
	err := row.Scan(&monitor.ID, &monitor.Name, &monitor.URL, &monitor.Method, &monitor.ExpectedStatus, &monitor.IntervalSeconds, &monitor.Enabled, &monitor.LastStatus, &monitor.LastLatencyMs, &monitor.LastCheckedAt, &monitor.CreatedAt, &monitor.UpdatedAt)
	return monitor, err
}

func scanMonitors(rows pgx.Rows) ([]models.Monitor, error) {
	var monitors []models.Monitor
	for rows.Next() {
		monitor, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		monitors = append(monitors, monitor)
	}
	return monitors, rows.Err()
}

func scanIncident(row pgx.Row) (models.Incident, error) {
	var incident models.Incident
	err := row.Scan(&incident.ID, &incident.MonitorID, &incident.Title, &incident.Status, &incident.Severity, &incident.OpenedAt, &incident.ResolvedAt)
	return incident, err
}

func scanMaintenanceRun(row pgx.Row) (models.MaintenanceRun, error) {
	var run models.MaintenanceRun
	err := row.Scan(&run.ID, &run.TaskName, &run.Status, &run.Output, &run.StartedAt, &run.FinishedAt)
	return run, err
}

func syncIncidentForCheck(ctx context.Context, tx pgx.Tx, monitor models.Monitor, status string, message *string) error {
	switch status {
	case "down", "degraded":
		severity := "minor"
		if status == "down" {
			severity = "major"
		}
		reason := "monitor check failed"
		if message != nil && *message != "" {
			reason = *message
		}
		title := fmt.Sprintf("%s is %s: %s", monitor.Name, status, reason)
		_, err := tx.Exec(ctx, `
			INSERT INTO incidents (monitor_id, title, severity)
			VALUES ($1::uuid, $2, $3)
			ON CONFLICT (monitor_id) WHERE status = 'open' AND monitor_id IS NOT NULL DO NOTHING
		`, monitor.ID, title, severity)
		return err
	case "up":
		_, err := tx.Exec(ctx, `
			UPDATE incidents
			SET status = 'resolved', resolved_at = now()
			WHERE monitor_id = $1::uuid AND status = 'open'
		`, monitor.ID)
		return err
	default:
		return nil
	}
}

func nullableUUID(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}
