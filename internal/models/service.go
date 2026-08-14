package models

import "time"

type Service struct {
	ID                   int               `json:"id"`
	Name                 string            `json:"name"`
	Description          string            `json:"description"`
	GithubRepoName       string            `json:"github_repo_name"`
	Status               string            `json:"status"`
	BuildCommand         string            `json:"build_command,omitempty"`
	RunCommand           string            `json:"run_command,omitempty"`
	Port                 int               `json:"port,omitempty"`
	EnvVars              map[string]string `json:"env_vars,omitempty"`
	InfisicalWorkspaceID string            `json:"infisical_workspace_id,omitempty"`
	InfisicalEnv         string            `json:"infisical_env,omitempty"`
	DirectoryPath        string            `json:"directory_path,omitempty"`
	SSLStatus            string            `json:"ssl_status,omitempty"`
	BuildLogs            string            `json:"build_logs,omitempty"`
	RuntimeLogs          string            `json:"runtime_logs,omitempty"`
	RootDirectory        string            `json:"root_directory"`
	CreatedAt            time.Time         `json:"created_at"`
}
