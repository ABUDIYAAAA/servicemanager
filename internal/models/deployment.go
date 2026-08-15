package models

import "time"

type Deployment struct {
	ID            int        `json:"id"`
	ServiceID     int        `json:"service_id"`
	Status        string     `json:"status"`                   // queued, building, running, failed, stopped
	Trigger       string     `json:"trigger"`                  // manual, webhook_push
	CommitSHA     string     `json:"commit_sha,omitempty"`
	BuildLogs     string     `json:"build_logs,omitempty"`
	RuntimeLogs   string     `json:"runtime_logs,omitempty"`
	DirectoryPath string     `json:"directory_path,omitempty"`
	ContainerName string     `json:"container_name,omitempty"`
	HostPort      int        `json:"host_port,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}
