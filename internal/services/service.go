package services

import (
	"context"
	"servicemanager/internal/models"
	"servicemanager/internal/utils"
)

type ServiceService struct {
	repository *ServiceRepository
	deployRepo *DeploymentRepository
}

func NewServiceService(repo *ServiceRepository, deployRepo *DeploymentRepository) *ServiceService {
	return &ServiceService{
		repository: repo,
		deployRepo: deployRepo,
	}
}

func (s *ServiceService) GetAllServices(ctx context.Context) ([]models.Service, error) {
	return s.repository.GetAllServices(ctx)
}

func (s *ServiceService) GetServiceByID(ctx context.Context, id int) (*models.Service, error) {
	return s.repository.GetServiceByID(ctx, id)
}

func (s *ServiceService) CreateService(ctx context.Context, name string, description string, repoName string) (*models.Service, error) {
	return s.repository.CreateService(ctx, name, description, repoName)
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

func (s *ServiceService) StopAllActiveDeployments(ctx context.Context, serviceID int) error {
	return s.deployRepo.StopAllActiveDeployments(ctx, serviceID)
}

func (s *ServiceService) UpdateStatus(ctx context.Context, id int, status string) error {
	return s.repository.UpdateStatus(ctx, id, status)
}

func (s *ServiceService) GetRepositoryDirectories(ctx context.Context, userID int, repoFullName string, githubAppID, githubPrivateKey string) ([]string, error) {
	var installationID int64
	err := s.repository.pool.QueryRow(ctx, "SELECT installation_id FROM github_installations WHERE user_id = $1 LIMIT 1", userID).Scan(&installationID)
	if err != nil {
		err = s.repository.pool.QueryRow(ctx, "SELECT installation_id FROM github_installations LIMIT 1").Scan(&installationID)
		if err != nil {
			return nil, err
		}
	}

	token, err := utils.GetInstallationAccessToken(githubAppID, githubPrivateKey, installationID)
	if err != nil {
		return nil, err
	}

	return utils.GetRepositoryDirectories(token, repoFullName)
}
