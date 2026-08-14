package services

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"servicemanager/internal/utils"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ActiveProcess struct {
	Cmd          *exec.Cmd
	Cancel       context.CancelFunc
	DeploymentID int
}

type ServiceLogMessage struct {
	ServiceID    int    `json:"service_id"`
	DeploymentID int    `json:"deployment_id"`
	Type         string `json:"type"` // "build" or "runtime"
	Log          string `json:"log"`
}

type ServiceLogBroadcaster struct {
	mu        sync.Mutex
	listeners map[int][]chan ServiceLogMessage // keyed by service_id
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
	list := b.listeners[msg.ServiceID]
	for _, ch := range list {
		select {
		case ch <- msg:
		default:
			// skip slow readers
		}
	}
}

type ServiceBuilder struct {
	pool          *pgxpool.Pool
	deployRepo    *DeploymentRepository
	githubAppID   string
	githubPrivKey string
	workRootDir   string

	mu         sync.Mutex
	activeRuns map[int]*ActiveProcess // keyed by service_id
}

func NewServiceBuilder(pool *pgxpool.Pool, deployRepo *DeploymentRepository, appID, privKey, workRootDir string) *ServiceBuilder {
	return &ServiceBuilder{
		pool:          pool,
		deployRepo:    deployRepo,
		githubAppID:   appID,
		githubPrivKey: privKey,
		workRootDir:   workRootDir,
		activeRuns:    make(map[int]*ActiveProcess),
	}
}

// ExecuteDeployment is the main entry point called by the Asynq worker.
// It performs the full deployment lifecycle: stop old processes, clone, build, run.
func (b *ServiceBuilder) ExecuteDeployment(ctx context.Context, serviceID, deploymentID, userID int) error {
	// 1. Stop any active runtime process for this service
	b.stopActiveProcess(serviceID)

	// 2. Mark all previous deployments as stopped
	_ = b.deployRepo.StopAllActiveDeployments(ctx, serviceID)

	// Re-activate current deployment (it was just stopped by the blanket stop above)
	_ = b.deployRepo.UpdateDeploymentStatus(ctx, deploymentID, "building")

	// 3. Fetch the service details
	var service struct {
		Name           string
		GithubRepoName string
		BuildCommand   string
		RunCommand     string
		EnvVars        map[string]string
		RootDirectory  string
	}

	var githubRepoName *string
	var buildCommand *string
	var runCommand *string

	err := b.pool.QueryRow(ctx,
		`SELECT name, github_repo_name, build_command, run_command, env_vars, root_directory FROM services WHERE id = $1`, serviceID,
	).Scan(&service.Name, &githubRepoName, &buildCommand, &runCommand, &service.EnvVars, &service.RootDirectory)
	if err != nil {
		b.appendBuildLog(serviceID, deploymentID, fmt.Sprintf("Error: failed to fetch service: %v", err))
		_ = b.deployRepo.UpdateDeploymentStatus(ctx, deploymentID, "failed")
		b.updateServiceStatus(serviceID, "failed")
		return err
	}

	if githubRepoName != nil {
		service.GithubRepoName = *githubRepoName
	}
	if buildCommand != nil {
		service.BuildCommand = *buildCommand
	}
	if runCommand != nil {
		service.RunCommand = *runCommand
	}

	if service.EnvVars == nil {
		service.EnvVars = make(map[string]string)
	}

	// 4. Prepare workspace directory
	targetDir := filepath.Join(b.workRootDir, fmt.Sprintf("service-%d", serviceID))

	// Clean up old deployment files
	_ = os.RemoveAll(targetDir)
	_ = os.MkdirAll(targetDir, 0755)

	_ = b.deployRepo.SetDeploymentDirectory(ctx, deploymentID, targetDir)

	// Also update the service record
	_, _ = b.pool.Exec(ctx, "UPDATE services SET directory_path = $2 WHERE id = $1", serviceID, targetDir)

	b.appendBuildLog(serviceID, deploymentID, "Initializing workspace directory...")

	// 5. Get GitHub installation token
	var installationID int64
	err = b.pool.QueryRow(ctx,
		"SELECT installation_id FROM github_installations WHERE user_id = $1 LIMIT 1", userID,
	).Scan(&installationID)

	if err != nil {
		// Fallback: try any installation that has access
		err = b.pool.QueryRow(ctx,
			"SELECT installation_id FROM github_installations LIMIT 1",
		).Scan(&installationID)
	}

	if err != nil {
		b.appendBuildLog(serviceID, deploymentID, "Error: No GitHub App installation found. Cannot clone repository.")
		_ = b.deployRepo.UpdateDeploymentStatus(ctx, deploymentID, "failed")
		b.updateServiceStatus(serviceID, "failed")
		return fmt.Errorf("no github installation found")
	}

	token, err := utils.GetInstallationAccessToken(b.githubAppID, b.githubPrivKey, installationID)
	if err != nil {
		b.appendBuildLog(serviceID, deploymentID, fmt.Sprintf("Error generating access token: %v", err))
		_ = b.deployRepo.UpdateDeploymentStatus(ctx, deploymentID, "failed")
		b.updateServiceStatus(serviceID, "failed")
		return err
	}

	// 6. Clone repository
	b.appendBuildLog(serviceID, deploymentID, fmt.Sprintf("Cloning repository: %s...", service.GithubRepoName))
	err = utils.CloneRepository(token, service.GithubRepoName, targetDir)
	if err != nil {
		b.appendBuildLog(serviceID, deploymentID, fmt.Sprintf("Clone failed: %v", err))
		_ = b.deployRepo.UpdateDeploymentStatus(ctx, deploymentID, "failed")
		b.updateServiceStatus(serviceID, "failed")
		return err
	}
	b.appendBuildLog(serviceID, deploymentID, "Repository cloned successfully.")

	// 7. Execute Build Command
	if service.BuildCommand != "" {
		b.appendBuildLog(serviceID, deploymentID, fmt.Sprintf("Running build command: %s", service.BuildCommand))

		buildDir := targetDir
		if service.RootDirectory != "" && service.RootDirectory != "." {
			buildDir = filepath.Join(targetDir, service.RootDirectory)
		}
		cmd := exec.CommandContext(ctx, "sh", "-c", service.BuildCommand)
		cmd.Dir = buildDir

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			b.appendBuildLog(serviceID, deploymentID, fmt.Sprintf("Error acquiring stdout: %v", err))
			_ = b.deployRepo.UpdateDeploymentStatus(ctx, deploymentID, "failed")
			b.updateServiceStatus(serviceID, "failed")
			return err
		}
		cmd.Stderr = cmd.Stdout

		if err := cmd.Start(); err != nil {
			b.appendBuildLog(serviceID, deploymentID, fmt.Sprintf("Failed to start build: %v", err))
			_ = b.deployRepo.UpdateDeploymentStatus(ctx, deploymentID, "failed")
			b.updateServiceStatus(serviceID, "failed")
			return err
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			b.appendBuildLog(serviceID, deploymentID, scanner.Text())
		}

		if err := cmd.Wait(); err != nil {
			b.appendBuildLog(serviceID, deploymentID, fmt.Sprintf("Build command failed: %v", err))
			_ = b.deployRepo.UpdateDeploymentStatus(ctx, deploymentID, "failed")
			b.updateServiceStatus(serviceID, "failed")
			return err
		}
		b.appendBuildLog(serviceID, deploymentID, "Build completed successfully.")
	} else {
		b.appendBuildLog(serviceID, deploymentID, "No build command specified. Skipping build.")
	}

	// 8. Start Runtime Process
	if service.RunCommand != "" {
		b.appendBuildLog(serviceID, deploymentID, "Starting runtime process...")
		_ = b.deployRepo.UpdateDeploymentStatus(ctx, deploymentID, "running")
		runDir := targetDir
		if service.RootDirectory != "" && service.RootDirectory != "." {
			runDir = filepath.Join(targetDir, service.RootDirectory)
		}
		b.startRuntimeProcess(serviceID, deploymentID, service.RunCommand, runDir, service.EnvVars)
	} else {
		b.appendBuildLog(serviceID, deploymentID, "No run command specified. Deployment completed.")
		_ = b.deployRepo.UpdateDeploymentStatus(ctx, deploymentID, "stopped")
		b.updateServiceStatus(serviceID, "inactive")
	}

	return nil
}

func (b *ServiceBuilder) startRuntimeProcess(serviceID, deploymentID int, runCmd, dir string, envVars map[string]string) {
	b.stopActiveProcess(serviceID)

	runCtx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(runCtx, "sh", "-c", runCmd)
	cmd.Dir = dir

	// Inject environment variables
	cmd.Env = os.Environ()
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		b.appendRuntimeLog(serviceID, deploymentID, fmt.Sprintf("Error creating stdout pipe: %v", err))
		_ = b.deployRepo.UpdateDeploymentStatus(context.Background(), deploymentID, "failed")
		b.updateServiceStatus(serviceID, "failed")
		cancel()
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		b.appendRuntimeLog(serviceID, deploymentID, fmt.Sprintf("Failed to start runtime: %v", err))
		_ = b.deployRepo.UpdateDeploymentStatus(context.Background(), deploymentID, "failed")
		b.updateServiceStatus(serviceID, "failed")
		cancel()
		return
	}

	b.mu.Lock()
	b.activeRuns[serviceID] = &ActiveProcess{
		Cmd:          cmd,
		Cancel:       cancel,
		DeploymentID: deploymentID,
	}
	b.mu.Unlock()

	b.updateServiceStatus(serviceID, "active")
	b.appendRuntimeLog(serviceID, deploymentID, fmt.Sprintf("Runtime started (PID: %d).", cmd.Process.Pid))

	go func() {
		defer cancel()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			b.appendRuntimeLog(serviceID, deploymentID, scanner.Text())
		}

		exitMsg := "Runtime process exited cleanly."
		finalStatus := "stopped"
		if err := cmd.Wait(); err != nil {
			exitMsg = fmt.Sprintf("Runtime process exited: %v", err)
			finalStatus = "failed"
		}

		b.appendRuntimeLog(serviceID, deploymentID, exitMsg)
		_ = b.deployRepo.UpdateDeploymentStatus(context.Background(), deploymentID, finalStatus)
		b.updateServiceStatus(serviceID, "inactive")

		b.mu.Lock()
		if active, ok := b.activeRuns[serviceID]; ok && active.Cmd == cmd {
			delete(b.activeRuns, serviceID)
		}
		b.mu.Unlock()
	}()
}

func (b *ServiceBuilder) stopActiveProcess(serviceID int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if active, exists := b.activeRuns[serviceID]; exists {
		active.Cancel()
		if active.Cmd.Process != nil {
			_ = active.Cmd.Process.Kill()
		}
		delete(b.activeRuns, serviceID)
	}
}

// StopService stops the runtime process for a service (admin action).
func (b *ServiceBuilder) StopService(serviceID int) {
	b.stopActiveProcess(serviceID)
	b.updateServiceStatus(serviceID, "inactive")
}

// --- Log helpers ---

func (b *ServiceBuilder) appendBuildLog(serviceID, deploymentID int, logLine string) {
	slog.Info("[BUILD]", slog.Int("service_id", serviceID), slog.Int("deployment_id", deploymentID), slog.String("log", logLine))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = b.deployRepo.AppendBuildLog(ctx, deploymentID, logLine)

	LogBroadcaster.Broadcast(ServiceLogMessage{
		ServiceID:    serviceID,
		DeploymentID: deploymentID,
		Type:         "build",
		Log:          logLine,
	})
}

func (b *ServiceBuilder) appendRuntimeLog(serviceID, deploymentID int, logLine string) {
	slog.Info("[RUNTIME]", slog.Int("service_id", serviceID), slog.Int("deployment_id", deploymentID), slog.String("log", logLine))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = b.deployRepo.AppendRuntimeLog(ctx, deploymentID, logLine)

	LogBroadcaster.Broadcast(ServiceLogMessage{
		ServiceID:    serviceID,
		DeploymentID: deploymentID,
		Type:         "runtime",
		Log:          logLine,
	})
}

func (b *ServiceBuilder) updateServiceStatus(serviceID int, status string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = b.pool.Exec(ctx, "UPDATE services SET status = $2 WHERE id = $1", serviceID, status)
}
