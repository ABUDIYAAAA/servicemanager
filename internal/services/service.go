package services

import (
	"context"
	"servicemanager/internal/models"
)

type ServiceService struct {
	repository *ServiceRepository
}

func NewServiceService(repo *ServiceRepository) *ServiceService {
	return &ServiceService{
		repository: repo,
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
) error {
	return s.repository.UpdateDeployConfig(ctx, id, status, buildCmd, runCmd, port, envVars, workspaceID, infEnv)
}

func (s *ServiceService) DeleteService(ctx context.Context, id int) error {
	return s.repository.DeleteService(ctx, id)
}
