package models

import "time"

type RepositoryDetails struct {
	Directories []string `json:"directories"`
	Frameworks  map[string]string `json:"frameworks"`
}


type GithubInstallation struct {
	ID             int       `json:"id"`
	UserID         int       `json:"user_id"`
	InstallationID int64     `json:"installation_id"`
	AccountID      int64     `json:"account_id"`
	AccountLogin   string    `json:"account_login"`
	CreatedAt      time.Time `json:"created_at"`
}

type GithubRepo struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Private       bool      `json:"private"`
	Description   string    `json:"description"`
	HTMLURL       string    `json:"html_url"`
	PushedAt      time.Time `json:"pushed_at"`
	CloneURL      string    `json:"clone_url"`
	DefaultBranch string    `json:"default_branch"`
}
