package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"servicemanager/internal/deployment"
	"servicemanager/internal/docker"
	"servicemanager/internal/models"
	"servicemanager/internal/utils"
)

type ServiceService struct {
	repository *ServiceRepository
	deployRepo *DeploymentRepository
	infisical  *utils.InfisicalClient
}

func NewServiceService(repo *ServiceRepository, deployRepo *DeploymentRepository, infisical *utils.InfisicalClient) *ServiceService {
	return &ServiceService{
		repository: repo,
		deployRepo: deployRepo,
		infisical:  infisical,
	}
}

func (s *ServiceService) GetAllServices(ctx context.Context) ([]models.Service, error) {
	return s.repository.GetAllServices(ctx)
}

func (s *ServiceService) GetServiceByID(ctx context.Context, id int) (*models.Service, error) {
	return s.repository.GetServiceByID(ctx, id)
}

func (s *ServiceService) UpdateService(ctx context.Context, id int, payload UpdateServiceRequestPayload) (*models.Service, error) {
	svc, err := s.repository.GetServiceByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service not found: %w", err)
	}

	if payload.EnvVars != nil && svc.InfisicalWorkspaceID != "" {
		err = s.infisical.SyncSecrets(ctx, svc.InfisicalWorkspaceID, svc.InfisicalEnv, payload.EnvVars)
		if err != nil {
			return nil, fmt.Errorf("failed to sync secrets to infisical: %w", err)
		}
	}

	updatedSvc, err := s.repository.UpdateService(ctx, id, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to update service in database: %w", err)
	}

	return updatedSvc, nil
}

func (s *ServiceService) CreateService(ctx context.Context, name, description, repoName, rootDirectory, buildCommand, runCommand, framework string, port int, envVars map[string]string, domain string) (*models.Service, error) {
	// 1. Create DB record first to get unique ID
	svc, err := s.repository.CreateService(ctx, name, description, repoName, rootDirectory, buildCommand, runCommand, framework, port, envVars, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to save service to database: %w", err)
	}

	// 2. Setup Infisical project
	rawSlug := strings.ToLower(fmt.Sprintf("%s-%d", name, svc.ID))
	reg := regexp.MustCompile(`[^a-z0-9-]+`)
	slug := reg.ReplaceAllString(rawSlug, "-")
	
	projectID, err := s.infisical.UpsertProject(ctx, name, slug)
	if err != nil {
		_ = s.repository.DeleteService(ctx, svc.ID)
		return nil, fmt.Errorf("failed to create infisical project: %w", err)
	}

	// 3. Sync Env Vars
	err = s.infisical.SyncSecrets(ctx, projectID, "dev", envVars)
	if err != nil {
		_ = s.infisical.DeleteProject(ctx, projectID)
		_ = s.repository.DeleteService(ctx, svc.ID)
		return nil, fmt.Errorf("failed to sync secrets to infisical: %w", err)
	}

	// 4. Update the DB with the generated Infisical workspace ID
	err = s.repository.UpdateDeployConfig(ctx, svc.ID, svc.Status, svc.BuildCommand, svc.RunCommand, svc.Port, svc.EnvVars, projectID, "dev", svc.RootDirectory)
	if err != nil {
		// Just log or return error, but it's partially created. Let's fully rollback to be safe.
		_ = s.infisical.DeleteProject(ctx, projectID)
		_ = s.repository.DeleteService(ctx, svc.ID)
		return nil, fmt.Errorf("failed to update service with infisical workspace: %w", err)
	}

	svc.InfisicalWorkspaceID = projectID
	svc.InfisicalEnv = "dev"
	
	return svc, nil
}

func (s *ServiceService) UpdateDeployConfig(
	ctx context.Context,
	id int,
	status string,
	buildCmd string,
	runCmd string,
	port int,
	envVars map[string]string,
	workspaceID string,
	infEnv string,
	rootDirectory string,
) error {
	return s.repository.UpdateDeployConfig(ctx, id, status, buildCmd, runCmd, port, envVars, workspaceID, infEnv, rootDirectory)
}

func (s *ServiceService) DeleteService(ctx context.Context, id int) error {
	svc, err := s.repository.GetServiceByID(ctx, id)
	if err != nil {
		return err
	}

	if svc.InfisicalWorkspaceID != "" {
		err = s.infisical.DeleteProject(ctx, svc.InfisicalWorkspaceID)
		if err != nil {
			return fmt.Errorf("failed to delete infisical project for service: %w", err)
		}
	}

	containerName := fmt.Sprintf("svc-%d", id)
	_ = docker.StopAndRemoveContainer(containerName)

	return s.repository.DeleteService(ctx, id)
}

func (s *ServiceService) CreateDeployment(ctx context.Context, serviceID int, trigger, commitSHA string) (*models.Deployment, error) {
	return s.deployRepo.CreateDeployment(ctx, serviceID, trigger, commitSHA)
}

func (s *ServiceService) GetActiveDeployment(ctx context.Context, serviceID int) (*models.Deployment, error) {
	return s.deployRepo.GetActiveDeployment(ctx, serviceID)
}

func (s *ServiceService) GetDeploymentByID(ctx context.Context, id int) (*models.Deployment, error) {
	return s.deployRepo.GetDeploymentByID(ctx, id)
}

func (s *ServiceService) GetDeploymentsByServiceID(ctx context.Context, serviceID int) ([]models.Deployment, error) {
	return s.deployRepo.GetDeploymentsByServiceID(ctx, serviceID)
}

func (s *ServiceService) UpdateDeploymentStatus(ctx context.Context, id int, status string) error {
	return s.deployRepo.UpdateDeploymentStatus(ctx, id, status)
}

func (s *ServiceService) GetLatestDeployment(ctx context.Context, serviceID int) (*models.Deployment, error) {
	return s.deployRepo.GetLatestDeployment(ctx, serviceID)
}

func (s *ServiceService) StopAllActiveDeployments(ctx context.Context, serviceID int) error {
	return s.deployRepo.StopAllActiveDeployments(ctx, serviceID)
}

func (s *ServiceService) UpdateStatus(ctx context.Context, id int, status string) error {
	return s.repository.UpdateStatus(ctx, id, status)
}

func (s *ServiceService) GetRepositoryDirectories(ctx context.Context, userID int, repoFullName string, githubAppID, githubPrivateKey string) (*models.RepositoryDetails, error) {
	rows, err := s.repository.pool.Query(ctx, "SELECT installation_id FROM github_installations WHERE user_id = $1", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch github installations: %w", err)
	}
	defer rows.Close()

	var installationIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			continue
		}
		installationIDs = append(installationIDs, id)
	}

	if len(installationIDs) == 0 {
		return nil, fmt.Errorf("github app not installed for this user")
	}

	for _, installationID := range installationIDs {
		token, err := utils.GetInstallationAccessToken(githubAppID, githubPrivateKey, installationID)
		if err != nil {
			continue
		}

		dirs, files, err := utils.GetRepositoryDirectories(token, repoFullName)
		if err == nil {
			frameworks := deployment.DetectFrameworks(dirs, files)
			return &models.RepositoryDetails{
				Directories: dirs,
				Frameworks:  frameworks,
			}, nil
		}
	}

	return nil, fmt.Errorf("repository not found or you do not have access to it")
}
