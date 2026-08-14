package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"servicemanager/internal/queue"
	"servicemanager/internal/services"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WebhookHandler struct {
	pool           *pgxpool.Pool
	serviceService *services.ServiceService
	asynqClient    *asynq.Client
	webhookSecret  string
}

func NewWebhookHandler(pool *pgxpool.Pool, ss *services.ServiceService, client *asynq.Client, secret string) *WebhookHandler {
	return &WebhookHandler{
		pool:           pool,
		serviceService: ss,
		asynqClient:    client,
		webhookSecret:  secret,
	}
}

func (h *WebhookHandler) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Verify signature if webhook secret is configured
	if h.webhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifySignature(body, sig, h.webhookSecret) {
			slog.Warn("GitHub webhook signature verification failed")
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	eventType := r.Header.Get("X-GitHub-Event")
	slog.Info("Received GitHub webhook", slog.String("event", eventType))

	switch eventType {
	case "installation":
		h.handleInstallation(body)
	case "push":
		h.handlePush(body)
	case "ping":
		slog.Info("GitHub webhook ping received")
	default:
		slog.Info("Ignoring unhandled webhook event", slog.String("event", eventType))
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// --- Installation Event ---

type installationEvent struct {
	Action       string `json:"action"` // created, deleted, new_permissions_accepted, suspend, unsuspend
	Installation struct {
		ID      int64 `json:"id"`
		Account struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"account"`
	} `json:"installation"`
	Sender struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"sender"`
}

func (h *WebhookHandler) handleInstallation(body []byte) {
	var event installationEvent
	if err := json.Unmarshal(body, &event); err != nil {
		slog.Error("Failed to parse installation event", slog.Any("error", err))
		return
	}

	slog.Info("GitHub installation event",
		slog.String("action", event.Action),
		slog.Int64("installation_id", event.Installation.ID),
		slog.String("account", event.Installation.Account.Login),
	)

	ctx := context.Background()

	switch event.Action {
	case "created":
		// Find the first admin user to associate the installation with
		var adminUserID int
		err := h.pool.QueryRow(ctx,
			"SELECT id FROM users WHERE user_role = 'admin' ORDER BY id ASC LIMIT 1",
		).Scan(&adminUserID)
		if err != nil {
			slog.Error("No admin user found to associate GitHub installation", slog.Any("error", err))
			return
		}

		// Delete any previous installations for this account to prevent stale records
		_, _ = h.pool.Exec(ctx, "DELETE FROM github_installations WHERE account_login = $1", event.Installation.Account.Login)

		// Upsert the installation
		_, err = h.pool.Exec(ctx,
			`INSERT INTO github_installations (user_id, installation_id, account_id, account_login)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (installation_id) DO UPDATE SET account_id = $3, account_login = $4`,
			adminUserID, event.Installation.ID, event.Installation.Account.ID, event.Installation.Account.Login,
		)
		if err != nil {
			slog.Error("Failed to upsert GitHub installation from webhook", slog.Any("error", err))
		} else {
			slog.Info("GitHub installation saved via webhook",
				slog.Int64("installation_id", event.Installation.ID),
				slog.String("account", event.Installation.Account.Login),
			)
		}

	case "deleted":
		_, err := h.pool.Exec(ctx,
			"DELETE FROM github_installations WHERE installation_id = $1",
			event.Installation.ID,
		)
		if err != nil {
			slog.Error("Failed to delete GitHub installation", slog.Any("error", err))
		} else {
			slog.Info("GitHub installation removed via webhook", slog.Int64("installation_id", event.Installation.ID))
		}
	}
}

// --- Push Event ---

type pushEvent struct {
	Ref        string `json:"ref"`
	After      string `json:"after"` // commit SHA
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Sender struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"sender"`
}

func (h *WebhookHandler) handlePush(body []byte) {
	var event pushEvent
	if err := json.Unmarshal(body, &event); err != nil {
		slog.Error("Failed to parse push event", slog.Any("error", err))
		return
	}

	repoFullName := event.Repository.FullName
	commitSHA := event.After

	slog.Info("GitHub push event",
		slog.String("repo", repoFullName),
		slog.String("ref", event.Ref),
		slog.String("commit", commitSHA),
	)

	ctx := context.Background()

	// Find all services linked to this repository
	allServices, err := h.serviceService.GetAllServices(ctx)
	if err != nil {
		slog.Error("Failed to fetch services for push webhook", slog.Any("error", err))
		return
	}

	// Find the user who installed this GitHub App (for deployment auth)
	var deployUserID int
	err = h.pool.QueryRow(ctx,
		"SELECT user_id FROM github_installations WHERE installation_id = $1 LIMIT 1",
		event.Installation.ID,
	).Scan(&deployUserID)
	if err != nil {
		// Fallback to first admin
		err = h.pool.QueryRow(ctx,
			"SELECT id FROM users WHERE user_role = 'admin' ORDER BY id ASC LIMIT 1",
		).Scan(&deployUserID)
		if err != nil {
			slog.Error("No user found to deploy for push webhook", slog.Any("error", err))
			return
		}
	}

	for _, svc := range allServices {
		// Match by repo full name (case-insensitive)
		if !strings.EqualFold(svc.GithubRepoName, repoFullName) {
			continue
		}

		// Only deploy services that have been configured (not drafts)
		if svc.RunCommand == "" {
			slog.Info("Skipping draft service for push event", slog.String("service", svc.Name))
			continue
		}

		slog.Info("Triggering deployment for push event",
			slog.String("service", svc.Name),
			slog.String("commit", commitSHA),
		)

		// Create a new deployment record
		deployment, err := h.serviceService.CreateDeployment(ctx, svc.ID, "webhook_push", commitSHA)
		if err != nil {
			slog.Error("Failed to create deployment for push webhook",
				slog.String("service", svc.Name),
				slog.Any("error", err),
			)
			continue
		}

		// Enqueue deployment task
		task, err := queue.NewDeployTask(svc.ID, deployment.ID, deployUserID)
		if err != nil {
			slog.Error("Failed to create deploy task", slog.Any("error", err))
			continue
		}

		_, err = h.asynqClient.Enqueue(task)
		if err != nil {
			slog.Error("Failed to enqueue deploy task", slog.Any("error", err))
			continue
		}

		slog.Info("Deployment queued via webhook",
			slog.String("service", svc.Name),
			slog.Int("deployment_id", deployment.ID),
		)
	}
}

// --- Signature Verification ---

func verifySignature(payload []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}

	// Signature format: sha256=<hex>
	parts := strings.SplitN(signature, "=", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(parts[1]), []byte(expectedMAC))
}

// GetWebhookURL returns a formatted webhook URL for display purposes.
func GetWebhookURL(baseURL string) string {
	return fmt.Sprintf("%s/api/v1/webhooks/github", baseURL)
}
