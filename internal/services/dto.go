package services

type CreateServiceRequestPayload struct {
	Name           string `json:"name" validate:"required"`
	Description    string `json:"description"`
	GithubRepoName string `json:"github_repo_name"`
	RootDirectory  string            `json:"root_directory"`
	BuildCommand   string            `json:"build_command"`
	RunCommand     string            `json:"run_command"`
	Port           int               `json:"port"`
	Framework      string            `json:"framework"`
	EnvVars        map[string]string `json:"env_vars"`
	Domain         string            `json:"domain"`
}
