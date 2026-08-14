package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type InfisicalClient struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
}

func NewInfisicalClient(baseURL, clientID, clientSecret string) *InfisicalClient {
	return &InfisicalClient{
		BaseURL:      baseURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
}

type loginResponse struct {
	AccessToken string `json:"accessToken"`
}

func (c *InfisicalClient) login(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/api/v1/auth/universal-auth/login", c.BaseURL)
	body, err := json.Marshal(map[string]string{
		"clientId":     c.ClientID,
		"clientSecret": c.ClientSecret,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("infisical login failed with status: %d", resp.StatusCode)
	}

	var res loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	return res.AccessToken, nil
}

type projectModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type createProjectResponse struct {
	Project projectModel `json:"project"`
}

type listProjectsResponse struct {
	Projects []projectModel `json:"projects"`
}

// UpsertProject checks if project exists, creates if not. Returns project ID.
func (c *InfisicalClient) UpsertProject(ctx context.Context, name, slug string) (string, error) {
	token, err := c.login(ctx)
	if err != nil {
		return "", err
	}

	// Try to create project first
	createURL := fmt.Sprintf("%s/api/v1/projects", c.BaseURL)
	createBody, err := json.Marshal(map[string]any{
		"projectName":             name,
		"slug":                    slug,
		"projectDescription":      fmt.Sprintf("Automatically managed project for service: %s", name),
		"template":                "default",
		"type":                    "secret-manager",
		"shouldCreateDefaultEnvs": true,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", createURL, bytes.NewBuffer(createBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// If successful, extract and return ID
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		var res createProjectResponse
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.Project.ID != "" {
			return res.Project.ID, nil
		}
	}

	// If create failed (e.g. project already exists), fetch list of projects to find it
	listURL := fmt.Sprintf("%s/api/v1/projects", c.BaseURL)
	listReq, err := http.NewRequestWithContext(ctx, "GET", listURL, nil)
	if err != nil {
		return "", err
	}
	listReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	listResp, err := httpClient.Do(listReq)
	if err != nil {
		return "", err
	}
	defer listResp.Body.Close()

	if listResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to create or list projects: status %d", listResp.StatusCode)
	}

	var listRes listProjectsResponse
	if err := json.NewDecoder(listResp.Body).Decode(&listRes); err != nil {
		return "", err
	}

	for _, p := range listRes.Projects {
		if p.Slug == slug || p.Name == name {
			return p.ID, nil
		}
	}

	return "", fmt.Errorf("project '%s' could not be created or found in list", slug)
}

type secretInput struct {
	SecretKey   string `json:"secretKey"`
	SecretValue string `json:"secretValue"`
}

type batchSecretsRequest struct {
	ProjectID   string        `json:"projectId"`
	Environment string        `json:"environment"`
	Secrets     []secretInput `json:"secrets"`
}

// SyncSecrets uploads the environment variables map to the specified Infisical project
func (c *InfisicalClient) SyncSecrets(ctx context.Context, projectID, environment string, envVars map[string]string) error {
	if len(envVars) == 0 {
		return nil
	}

	token, err := c.login(ctx)
	if err != nil {
		return err
	}

	var secretsList []secretInput
	for k, v := range envVars {
		secretsList = append(secretsList, secretInput{
			SecretKey:   k,
			SecretValue: v,
		})
	}

	batchBody, err := json.Marshal(batchSecretsRequest{
		ProjectID:   projectID,
		Environment: environment,
		Secrets:     secretsList,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v4/secrets/batch", c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(batchBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to sync secrets to infisical: status %d", resp.StatusCode)
	}

	return nil
}
