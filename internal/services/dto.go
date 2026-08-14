package services

type CreateServiceRequestPayload struct {
	Name           string `json:"name" validate:"required"`
	Description    string `json:"description"`
	GithubRepoName string `json:"github_repo_name"`
}
