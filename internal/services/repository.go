package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"servicemanager/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrServiceNotFound = errors.New("service not found")

type ServiceRepository struct {
	pool *pgxpool.Pool
}

func NewServiceRepository(pool *pgxpool.Pool) *ServiceRepository {
	return &ServiceRepository{
		pool: pool,
	}
}

func (r *ServiceRepository) GetAllServices(ctx context.Context) ([]models.Service, error) {
	var services []models.Service

	query := `SELECT id, name, description, github_repo_name, status, build_command, run_command, 
	                 port, env_vars, infisical_workspace_id, infisical_env, 
	                 directory_path, ssl_status, build_logs, runtime_logs, root_directory, created_at 
	          FROM services ORDER BY id ASC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var s models.Service
		var desc *string
		var repoName *string
		var buildCmd *string
		var runCmd *string
		var port *int
		var envVarsBytes []byte
		var workspaceID *string
		var infEnv *string
		var dirPath *string

		err := rows.Scan(
			&s.ID, &s.Name, &desc, &repoName, &s.Status, &buildCmd, &runCmd,
			&port, &envVarsBytes, &workspaceID, &infEnv,
			&dirPath, &s.SSLStatus, &s.BuildLogs, &s.RuntimeLogs, &s.RootDirectory, &s.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if desc != nil {
			s.Description = *desc
		}
		if repoName != nil {
			s.GithubRepoName = *repoName
		}
		if buildCmd != nil {
			s.BuildCommand = *buildCmd
		}
		if runCmd != nil {
			s.RunCommand = *runCmd
		}
		if port != nil {
			s.Port = *port
		}
		if len(envVarsBytes) > 0 {
			_ = json.Unmarshal(envVarsBytes, &s.EnvVars)
		}
		if s.EnvVars == nil {
			s.EnvVars = make(map[string]string)
		}
		if workspaceID != nil {
			s.InfisicalWorkspaceID = *workspaceID
		}
		if infEnv != nil {
			s.InfisicalEnv = *infEnv
		}
		if dirPath != nil {
			s.DirectoryPath = *dirPath
		}

		services = append(services, s)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return services, nil
}

func (r *ServiceRepository) GetServiceByID(ctx context.Context, id int) (*models.Service, error) {
	query := `SELECT id, name, description, github_repo_name, status, build_command, run_command, 
	                 port, env_vars, infisical_workspace_id, infisical_env, 
	                 directory_path, ssl_status, build_logs, runtime_logs, root_directory, created_at 
	          FROM services WHERE id = $1`

	var s models.Service
	var desc *string
	var repoName *string
	var buildCmd *string
	var runCmd *string
	var port *int
	var envVarsBytes []byte
	var workspaceID *string
	var infEnv *string
	var dirPath *string

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&s.ID, &s.Name, &desc, &repoName, &s.Status, &buildCmd, &runCmd,
		&port, &envVarsBytes, &workspaceID, &infEnv,
		&dirPath, &s.SSLStatus, &s.BuildLogs, &s.RuntimeLogs, &s.RootDirectory, &s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrServiceNotFound
		}
		return nil, err
	}

	if desc != nil {
		s.Description = *desc
	}
	if repoName != nil {
		s.GithubRepoName = *repoName
	}
	if buildCmd != nil {
		s.BuildCommand = *buildCmd
	}
	if runCmd != nil {
		s.RunCommand = *runCmd
	}
	if port != nil {
		s.Port = *port
	}
	if len(envVarsBytes) > 0 {
		_ = json.Unmarshal(envVarsBytes, &s.EnvVars)
	}
	if s.EnvVars == nil {
		s.EnvVars = make(map[string]string)
	}
	if workspaceID != nil {
		s.InfisicalWorkspaceID = *workspaceID
	}
	if infEnv != nil {
		s.InfisicalEnv = *infEnv
	}
	if dirPath != nil {
		s.DirectoryPath = *dirPath
	}

	return &s, nil
}

func (r *ServiceRepository) CreateService(ctx context.Context, name string, description string, repoName string) (*models.Service, error) {
	query := `INSERT INTO services (name, description, github_repo_name, status, ssl_status, build_logs, runtime_logs, root_directory) 
	          VALUES ($1, $2, $3, 'draft', 'inactive', '', '', '.') 
	          RETURNING id, name, description, github_repo_name, status, build_command, run_command, 
	                    port, env_vars, infisical_workspace_id, infisical_env, 
	                    directory_path, ssl_status, build_logs, runtime_logs, root_directory, created_at`

	var s models.Service
	var desc *string
	var repoNameStr *string
	var buildCmd *string
	var runCmd *string
	var port *int
	var envVarsBytes []byte
	var workspaceID *string
	var infEnv *string
	var dirPath *string

	err := r.pool.QueryRow(ctx, query, name, description, repoName).Scan(
		&s.ID, &s.Name, &desc, &repoNameStr, &s.Status, &buildCmd, &runCmd,
		&port, &envVarsBytes, &workspaceID, &infEnv,
		&dirPath, &s.SSLStatus, &s.BuildLogs, &s.RuntimeLogs, &s.RootDirectory, &s.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	dirPlaceholder := fmt.Sprintf("/opt/servicesmanager/run/%d", s.ID)
	_, _ = r.pool.Exec(ctx, "UPDATE services SET directory_path = $2 WHERE id = $1", s.ID, dirPlaceholder)
	s.DirectoryPath = dirPlaceholder

	if desc != nil {
		s.Description = *desc
	}
	if repoNameStr != nil {
		s.GithubRepoName = *repoNameStr
	}
	if buildCmd != nil {
		s.BuildCommand = *buildCmd
	}
	if runCmd != nil {
		s.RunCommand = *runCmd
	}
	if port != nil {
		s.Port = *port
	}
	if len(envVarsBytes) > 0 {
		_ = json.Unmarshal(envVarsBytes, &s.EnvVars)
	}
	if s.EnvVars == nil {
		s.EnvVars = make(map[string]string)
	}
	if workspaceID != nil {
		s.InfisicalWorkspaceID = *workspaceID
	}
	if infEnv != nil {
		s.InfisicalEnv = *infEnv
	}

	return &s, nil
}

func (r *ServiceRepository) UpdateDeployConfig(
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
	envVarsBytes, err := json.Marshal(envVars)
	if err != nil {
		return err
	}

	mockBuildLogs := fmt.Sprintf("Starting build for service ID %d...\nFetching code...\nRunning build command: %s...\nBuild successful!\nReady for runtime execution.", id, buildCmd)
	mockRuntimeLogs := fmt.Sprintf("Starting runtime execution for service ID %d...\nRunning run command: %s on port %d...\nServer listening on port %d...", id, runCmd, port, port)

	query := `UPDATE services 
	          SET status = $2, build_command = $3, run_command = $4, port = $5, env_vars = $6, 
	              infisical_workspace_id = $7, infisical_env = $8,
	              build_logs = $9, runtime_logs = $10, root_directory = $11
	          WHERE id = $1`

	cmdTag, err := r.pool.Exec(
		ctx, query, id, status, buildCmd, runCmd, port, envVarsBytes,
		workspaceID, infEnv, mockBuildLogs, mockRuntimeLogs, rootDirectory,
	)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrServiceNotFound
	}
	return nil
}

func (r *ServiceRepository) DeleteService(ctx context.Context, id int) error {
	query := "DELETE FROM services WHERE id = $1"
	cmdTag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrServiceNotFound
	}
	return nil
}

func (r *ServiceRepository) UpdateStatus(ctx context.Context, id int, status string) error {
	query := "UPDATE services SET status = $2 WHERE id = $1"
	cmdTag, err := r.pool.Exec(ctx, query, id, status)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrServiceNotFound
	}
	return nil
}
