package models

import "time"

type User struct {
	ID           string
	Email        string
	Name         string
	Role         string
	PasswordHash string
	CreatedAt    time.Time
}

type CurrentUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

type Monitor struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	URL             string     `json:"url"`
	Method          string     `json:"method"`
	ExpectedStatus  int        `json:"expectedStatus"`
	IntervalSeconds int        `json:"intervalSeconds"`
	Enabled         bool       `json:"enabled"`
	LastStatus      string     `json:"lastStatus"`
	LastLatencyMs   *int       `json:"lastLatencyMs"`
	LastCheckedAt   *time.Time `json:"lastCheckedAt"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type CheckEvent struct {
	ID         string    `json:"id"`
	MonitorID  string    `json:"monitorId"`
	Status     string    `json:"status"`
	LatencyMs  *int      `json:"latencyMs"`
	StatusCode *int      `json:"statusCode"`
	Message    *string   `json:"message"`
	CheckedAt  time.Time `json:"checkedAt"`
}

type Incident struct {
	ID         string     `json:"id"`
	MonitorID  *string    `json:"monitorId"`
	Title      string     `json:"title"`
	Status     string     `json:"status"`
	Severity   string     `json:"severity"`
	OpenedAt   time.Time  `json:"openedAt"`
	ResolvedAt *time.Time `json:"resolvedAt"`
}

type JobRun struct {
	ID         string     `json:"id"`
	JobType    string     `json:"jobType"`
	Status     string     `json:"status"`
	Detail     *string    `json:"detail"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

type MaintenanceRun struct {
	ID         string     `json:"id"`
	TaskName   string     `json:"taskName"`
	Status     string     `json:"status"`
	Output     *string    `json:"output"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

type Dashboard struct {
	MonitorCount      int      `json:"monitorCount"`
	UpCount           int      `json:"upCount"`
	DownCount         int      `json:"downCount"`
	OpenIncidentCount int      `json:"openIncidentCount"`
	AvgLatencyMs      *float64 `json:"avgLatencyMs"`
}

type MaintenanceTask struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Dangerous   bool   `json:"dangerous"`
}

type MonitorInput struct {
	Name            string
	URL             string
	Method          string
	ExpectedStatus  int
	IntervalSeconds int
	Enabled         bool
}

type IncidentInput struct {
	MonitorID *string
	Title     string
	Severity  string
}
