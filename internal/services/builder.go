package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"servicemanager/internal/docker"
	"servicemanager/internal/utils"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ServiceLogMessage is the payload sent over the SSE log stream.
type ServiceLogMessage struct {
	ServiceID    int    `json:"service_id"`
	DeploymentID int    `json:"deployment_id"`
	Type         string `json:"type"` // "build" | "runtime" | "status"
	Log          string `json:"log"`
}

// ServiceLogBroadcaster multiplexes log events to connected SSE clients.
type ServiceLogBroadcaster struct {
	mu        sync.Mutex
	listeners map[int][]chan ServiceLogMessage
}

var LogBroadcaster = &ServiceLogBroadcaster{
	listeners: make(map[int][]chan ServiceLogMessage),
}

func (b *ServiceLogBroadcaster) Register(serviceID int, ch chan ServiceLogMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.listeners[serviceID] = append(b.listeners[serviceID], ch)
}

func (b *ServiceLogBroadcaster) Unregister(serviceID int, ch chan ServiceLogMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.listeners[serviceID]
	for i, c := range list {
		if c == ch {
			b.listeners[serviceID] = append(list[:i], list[i+1:]...)
			close(ch)
			break
		}
	}
}

func (b *ServiceLogBroadcaster) Broadcast(msg ServiceLogMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.listeners[msg.ServiceID] {
		select {
		case ch <- msg:
		default:
		}
	}
}

// ServiceBuilder orchestrates Docker-based service deployments.
type ServiceBuilder struct {
	pool          *pgxpool.Pool
	deployRepo    *DeploymentRepository
	githubAppID   string
	githubPrivKey string
}

func NewServiceBuilder(pool *pgxpool.Pool, deployRepo *DeploymentRepository, githubAppID, githubPrivKey, _ string) *ServiceBuilder {
	return &ServiceBuilder{
		pool:          pool,
		deployRepo:    deployRepo,
		githubAppID:   githubAppID,
		githubPrivKey: githubPrivKey,
	}
}

// ExecuteDeployment is the Asynq worker handler for deployment.create tasks.
// It is fully stateless: all state lives in Docker and the database.
func (b *ServiceBuilder) ExecuteDeployment(ctx context.Context, serviceID, deploymentID, userID int) error {
	slog.Info("Starting deployment", slog.Int("service_id", serviceID), slog.Int("deployment_id", deploymentID))

	// 1. Cancel any other queued deployments for this service (keep only this one running)
	if err := b.deployRepo.CancelQueuedDeploymentsExcept(ctx, serviceID, deploymentID); err != nil {
		slog.Warn("Failed to cancel stale queued deployments", slog.Any("error", err))
	}

	// 2. Mark as building
	_ = b.deployRepo.UpdateDeploymentStatus(ctx, deploymentID, "building")
	b.updateServiceStatus(serviceID, "building")
	b.broadcast(serviceID, deploymentID, "status", "Building...")

	// 3. Fetch service record
	var service struct {
		Name          string
		GithubRepo    string
		RootDirectory string
		BuildCommand  string
		RunCommand    string
		Framework     string
		Port          int
		WorkspaceID   string
		InfisicalEnv  string
		Domain        *string
	}
	err := b.pool.QueryRow(ctx,
		`SELECT name, github_repo_name, root_directory, build_command, run_command, framework, port, infisical_workspace_id, infisical_env, domain
		 FROM services WHERE id = $1`, serviceID,
	).Scan(
		&service.Name, &service.GithubRepo, &service.RootDirectory,
		&service.BuildCommand, &service.RunCommand, &service.Framework,
		&service.Port, &service.WorkspaceID, &service.InfisicalEnv, &service.Domain,
	)
	if err != nil {
		return b.fail(ctx, serviceID, deploymentID, "Failed to fetch service record", err)
	}

	// 4. Get GitHub installation token
	var installationID int64
	_ = b.pool.QueryRow(ctx,
		`SELECT installation_id FROM github_installations WHERE user_id = $1 LIMIT 1`, userID,
	).Scan(&installationID)
	if installationID == 0 {
		// fallback to any installation
		_ = b.pool.QueryRow(ctx,
			`SELECT installation_id FROM github_installations LIMIT 1`,
		).Scan(&installationID)
	}
	if installationID == 0 {
		return b.fail(ctx, serviceID, deploymentID, "No GitHub installation found", fmt.Errorf("no github installation"))
	}

	token, err := utils.GetInstallationAccessToken(b.githubAppID, b.githubPrivKey, installationID)
	if err != nil {
		return b.fail(ctx, serviceID, deploymentID, "Failed to get GitHub token", err)
	}

	// 5. Fetch env vars from Infisical (fresh at deploy time)
	envVars := map[string]string{}
	if service.WorkspaceID != "" {
		infisicalURL, _ := b.getConfigValue(ctx, "INFISICAL_URL")
		infisicalClientID, _ := b.getConfigValue(ctx, "INFISICAL_CLIENT_ID")
		infisicalClientSecret, _ := b.getConfigValue(ctx, "INFISICAL_CLIENT_SECRET")
		if infisicalURL != "" && infisicalClientID != "" && infisicalClientSecret != "" {
			infClient := utils.NewInfisicalClient(infisicalURL, infisicalClientID, infisicalClientSecret)
			fetched, ferr := infClient.GetSecrets(ctx, service.WorkspaceID, service.InfisicalEnv)
			if ferr != nil {
				b.log(serviceID, deploymentID, "build", fmt.Sprintf("Warning: Could not fetch secrets from Infisical: %v", ferr))
			} else {
				envVars = fetched
			}
		}
	} else {
		// Fall back to env_vars stored in DB
		var envJSON []byte
		_ = b.pool.QueryRow(ctx, `SELECT env_vars FROM services WHERE id = $1`, serviceID).Scan(&envJSON)
		if len(envJSON) > 0 {
			_ = json.Unmarshal(envJSON, &envVars)
		}
	}

	// 6. Clone repository to temp directory
	cloneDir, err := os.MkdirTemp("", fmt.Sprintf("svc-%d-deploy-%d-*", serviceID, deploymentID))
	if err != nil {
		return b.fail(ctx, serviceID, deploymentID, "Failed to create temp directory", err)
	}
	defer os.RemoveAll(cloneDir)

	b.log(serviceID, deploymentID, "build", fmt.Sprintf("Cloning %s...", service.GithubRepo))
	if err := utils.CloneRepository(token, service.GithubRepo, cloneDir); err != nil {
		return b.fail(ctx, serviceID, deploymentID, "Clone failed", err)
	}
	b.log(serviceID, deploymentID, "build", "Repository cloned successfully.")

	// 7. Generate and write Dockerfile
	rootDir := service.RootDirectory
	if rootDir == "" {
		rootDir = "."
	}
	dockerfileContent := docker.GenerateDockerfile(service.Framework, service.BuildCommand, service.RunCommand, rootDir, service.Port)
	dockerfilePath := filepath.Join(cloneDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644); err != nil {
		return b.fail(ctx, serviceID, deploymentID, "Failed to write Dockerfile", err)
	}
	b.log(serviceID, deploymentID, "build", fmt.Sprintf("Generated Dockerfile for %s framework.", service.Framework))

	// 8. Find a free host port above 4000
	hostPort, err := docker.FreePort(4000)
	if err != nil {
		return b.fail(ctx, serviceID, deploymentID, "No free port available", err)
	}
	b.log(serviceID, deploymentID, "build", fmt.Sprintf("Allocated host port %d.", hostPort))

	// 9. Build Docker image
	imageTag := fmt.Sprintf("svc-%d:deploy-%d", serviceID, deploymentID)
	b.log(serviceID, deploymentID, "build", fmt.Sprintf("Building Docker image %s...", imageTag))

	buildErr := docker.BuildImage(ctx, imageTag, cloneDir, func(line string) {
		b.log(serviceID, deploymentID, "build", line)
	})
	if buildErr != nil {
		return b.fail(ctx, serviceID, deploymentID, "Docker build failed", buildErr)
	}
	b.log(serviceID, deploymentID, "build", "Docker image built successfully.")

	// 10. Stop & remove the old container for this service (if any)
	oldContainerName := fmt.Sprintf("svc-%d", serviceID)
	b.log(serviceID, deploymentID, "build", "Stopping previous container (if running)...")
	_ = docker.StopAndRemoveContainer(oldContainerName)

	// 11. Run new container
	containerPort := service.Port
	if containerPort == 0 {
		containerPort = 3000
	}
	b.log(serviceID, deploymentID, "build", fmt.Sprintf("Starting container on port %d → %d...", hostPort, containerPort))

	labels := map[string]string{
		"seed.managed":      "true",
		"seed.service_id":    fmt.Sprintf("%d", serviceID),
		"seed.deployment_id": fmt.Sprintf("%d", deploymentID),
	}
	if err := docker.RunContainer(imageTag, oldContainerName, hostPort, containerPort, envVars, labels); err != nil {
		return b.fail(ctx, serviceID, deploymentID, "Failed to start container", err)
	}

	// 12. Update DB: container info, deployment status = running
	if err := b.deployRepo.UpdateContainerInfo(ctx, deploymentID, oldContainerName, hostPort); err != nil {
		slog.Error("Failed to update container info in DB", slog.Any("error", err))
	}
	b.updateServiceStatus(serviceID, "active")
	b.broadcast(serviceID, deploymentID, "status", fmt.Sprintf("running:%d", hostPort))

	if service.Domain != nil && *service.Domain != "" {
		domain := *service.Domain
		b.log(serviceID, deploymentID, "build", fmt.Sprintf("Configuring custom domain %s...", domain))
		
		if err := docker.WriteNginxConfig(domain, hostPort); err != nil {
			b.log(serviceID, deploymentID, "build", fmt.Sprintf("Warning: Failed to write Nginx config: %v", err))
		} else {
			if err := docker.ReloadNginx(); err != nil {
				b.log(serviceID, deploymentID, "build", fmt.Sprintf("Warning: Failed to reload Nginx: %v", err))
			} else {
				b.log(serviceID, deploymentID, "build", "Nginx configured successfully. Setting up TLS with Certbot...")
				if err := docker.SetupTLS(domain); err != nil {
					b.log(serviceID, deploymentID, "build", fmt.Sprintf("Warning: Certbot TLS setup failed: %v", err))
				} else {
					b.log(serviceID, deploymentID, "build", "TLS configured successfully!")
				}
			}
		}
	}

	b.log(serviceID, deploymentID, "build", fmt.Sprintf("✓ Deployment complete. Service running on port %d.", hostPort))
	slog.Info("Deployment complete",
		slog.Int("service_id", serviceID),
		slog.Int("deployment_id", deploymentID),
		slog.Int("host_port", hostPort),
	)
	return nil
}

// StopService stops the running container for a service.
func (b *ServiceBuilder) StopService(serviceID int) {
	containerName := fmt.Sprintf("svc-%d", serviceID)
	_ = docker.StopAndRemoveContainer(containerName)
	b.updateServiceStatus(serviceID, "inactive")
}

// --- Helpers ---

func (b *ServiceBuilder) fail(ctx context.Context, serviceID, deploymentID int, msg string, err error) error {
	b.log(serviceID, deploymentID, "build", fmt.Sprintf("✗ Error: %s: %v", msg, err))
	_ = b.deployRepo.UpdateDeploymentStatus(ctx, deploymentID, "failed")
	b.updateServiceStatus(serviceID, "failed")
	b.broadcast(serviceID, deploymentID, "status", "failed")
	return fmt.Errorf("%s: %w", msg, err)
}

func (b *ServiceBuilder) log(serviceID, deploymentID int, logType, line string) {
	slog.Info("[DEPLOY]",
		slog.Int("service_id", serviceID),
		slog.Int("deployment_id", deploymentID),
		slog.String("type", logType),
		slog.String("log", line),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if logType == "build" {
		_ = b.deployRepo.AppendBuildLog(ctx, deploymentID, line)
	} else {
		_ = b.deployRepo.AppendRuntimeLog(ctx, deploymentID, line)
	}

	b.broadcast(serviceID, deploymentID, logType, line)
}

func (b *ServiceBuilder) broadcast(serviceID, deploymentID int, msgType, line string) {
	LogBroadcaster.Broadcast(ServiceLogMessage{
		ServiceID:    serviceID,
		DeploymentID: deploymentID,
		Type:         msgType,
		Log:          line,
	})
}

func (b *ServiceBuilder) updateServiceStatus(serviceID int, status string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = b.pool.Exec(ctx, `UPDATE services SET status = $2 WHERE id = $1`, serviceID, status)
}

// getConfigValue reads an env-like config value from the process environment.
// This avoids threading the full config struct into the builder.
func (b *ServiceBuilder) getConfigValue(ctx context.Context, key string) (string, error) {
	val := os.Getenv(key)
	return val, nil
}
