package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"servicemanager/internal/models"

	"github.com/golang-jwt/jwt/v5"
)

type GithubAppInstallationResponse struct {
	ID      int64 `json:"id"`
	Account struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"account"`
}

type GithubAccessTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

type GithubReposResponse struct {
	TotalCount   int                 `json:"total_count"`
	Repositories []models.GithubRepo `json:"repositories"`
}

func GenerateGithubAppJWT(appID string, privateKeyPEM string) (string, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyPEM))
	if err != nil {
		return "", fmt.Errorf("failed to parse RSA private key: %w", err)
	}

	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)), // account for clock skew
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		Issuer:    appID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(key)
}

func GetInstallationDetails(appID, privateKeyPEM string, installationID int64) (*GithubAppInstallationResponse, error) {
	appJWT, err := GenerateGithubAppJWT(appID, privateKeyPEM)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d", installationID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch installation: status code %d", resp.StatusCode)
	}

	var details GithubAppInstallationResponse
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, err
	}

	return &details, nil
}

func GetInstallationAccessToken(appID, privateKeyPEM string, installationID int64) (string, error) {
	appJWT, err := GenerateGithubAppJWT(appID, privateKeyPEM)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("failed to fetch access token: status code %d", resp.StatusCode)
	}

	var tokenResp GithubAccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	return tokenResp.Token, nil
}

func GetInstallationRepositories(appID, privateKeyPEM string, installationID int64) ([]models.GithubRepo, error) {
	token, err := GetInstallationAccessToken(appID, privateKeyPEM, installationID)
	if err != nil {
		return nil, err
	}

	url := "https://api.github.com/installation/repositories?per_page=100"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch repositories: status code %d", resp.StatusCode)
	}

	var reposResp GithubReposResponse
	if err := json.NewDecoder(resp.Body).Decode(&reposResp); err != nil {
		return nil, err
	}

	return reposResp.Repositories, nil
}

func CloneRepository(token string, repoFullName string, targetDir string) error {
	cloneURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", token, repoFullName)
	cmd := exec.Command("git", "clone", cloneURL, targetDir)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", errBuf.String(), err)
	}
	return nil
}

func CreateCommitStatus(token string, repoFullName string, commitSHA string, state string, targetURL string, description string, ctxStr string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/statuses/%s", repoFullName, commitSHA)
	payload := map[string]string{
		"state":       state,
		"target_url":  targetURL,
		"description": description,
		"context":     ctxStr,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create status: status code %d", resp.StatusCode)
	}
	return nil
}

type GithubDeploymentPayload struct {
	Ref                  string   `json:"ref"`
	Environment          string   `json:"environment"`
	RequiredContexts     []string `json:"required_contexts"`
	AutoMerge            bool     `json:"auto_merge"`
}

func CreateDeployment(token string, repoFullName string, ref string, environment string) (int64, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/deployments", repoFullName)
	payload := GithubDeploymentPayload{
		Ref:              ref,
		Environment:      environment,
		RequiredContexts: []string{},
		AutoMerge:        false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("failed to create deployment: status code %d", resp.StatusCode)
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	return result.ID, nil
}

type GithubDeploymentStatusPayload struct {
	State          string `json:"state"`
	EnvironmentURL string `json:"environment_url,omitempty"`
	LogURL         string `json:"log_url,omitempty"`
	Description    string `json:"description,omitempty"`
}

func CreateDeploymentStatus(token string, repoFullName string, deploymentID int64, state string, environmentURL string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/deployments/%d/statuses", repoFullName, deploymentID)
	payload := GithubDeploymentStatusPayload{
		State:          state,
		EnvironmentURL: environmentURL,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create deployment status: status code %d", resp.StatusCode)
	}

	return nil
}

func GetAppURL(appID, privateKeyPEM string) (string, error) {
	appJWT, err := GenerateGithubAppJWT(appID, privateKeyPEM)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("GET", "https://api.github.com/app", nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch app details: status code %d", resp.StatusCode)
	}

	var appDetails struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&appDetails); err != nil {
		return "", err
	}

	return appDetails.HTMLURL + "/installations/new", nil
}

type GithubTreeResponse struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"tree"`
}

func GetRepositoryDirectories(token string, repoFullName string) ([]string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s", repoFullName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch repo info: status code %d", resp.StatusCode)
	}

	var repoInfo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repoInfo); err != nil {
		return nil, err
	}

	branch := repoInfo.DefaultBranch
	if branch == "" {
		branch = "main"
	}

	treeURL := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/%s?recursive=1", repoFullName, branch)
	treeReq, err := http.NewRequest("GET", treeURL, nil)
	if err != nil {
		return nil, err
	}
	treeReq.Header.Set("Authorization", "token "+token)
	treeReq.Header.Set("Accept", "application/vnd.github.v3+json")

	treeResp, err := http.DefaultClient.Do(treeReq)
	if err != nil {
		return nil, err
	}
	defer treeResp.Body.Close()

	if treeResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch git tree: status code %d", treeResp.StatusCode)
	}

	var treeResponse GithubTreeResponse
	if err := json.NewDecoder(treeResp.Body).Decode(&treeResponse); err != nil {
		return nil, err
	}

	var dirs []string
	dirs = append(dirs, ".")

	for _, item := range treeResponse.Tree {
		if item.Type == "tree" {
			dirs = append(dirs, item.Path)
		}
	}

	return dirs, nil
}

