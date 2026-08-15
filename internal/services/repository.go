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
	                 port, framework, env_vars, infisical_workspace_id, infisical_env, 
	                 directory_path, ssl_status, build_logs, runtime_logs, root_directory, domain, created_at 
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
		var framework *string
		var envVarsBytes []byte
		var workspaceID *string
		var infEnv *string
		var dirPath *string
		var domainScan *string

		err := rows.Scan(
			&s.ID, &s.Name, &desc, &repoName, &s.Status, &buildCmd, &runCmd,
			&port, &framework, &envVarsBytes, &workspaceID, &infEnv,
			&dirPath, &s.SSLStatus, &s.BuildLogs, &s.RuntimeLogs, &s.RootDirectory, &domainScan, &s.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		
		if framework != nil {
			s.Framework = *framework
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
		if domainScan != nil {
			s.Domain = *domainScan
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
	                 port, framework, env_vars, infisical_workspace_id, infisical_env, 
	                 directory_path, ssl_status, build_logs, runtime_logs, root_directory, domain, created_at 
	          FROM services WHERE id = $1`

	var s models.Service
	var desc *string
	var repoName *string
	var buildCmd *string
	var runCmd *string
	var port *int
	var framework *string
	var envVarsBytes []byte
	var workspaceID *string
	var infEnv *string
	var dirPath *string
	var domainScan *string

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&s.ID, &s.Name, &desc, &repoName, &s.Status, &buildCmd, &runCmd,
		&port, &framework, &envVarsBytes, &workspaceID, &infEnv,
		&dirPath, &s.SSLStatus, &s.BuildLogs, &s.RuntimeLogs, &s.RootDirectory, &domainScan, &s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrServiceNotFound
		}
		return nil, err
	}
	
	if framework != nil {
		s.Framework = *framework
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
	if domainScan != nil {
		s.Domain = *domainScan
	}

	return &s, nil
}

func (r *ServiceRepository) CreateService(ctx context.Context, name string, description string, repoName string, rootDirectory string, buildCommand string, runCommand string, framework string, port int, envVars map[string]string, domain string) (*models.Service, error) {
	envVarsBytes, err := json.Marshal(envVars)
	if err != nil {
		envVarsBytes = []byte("{}")
	}

	query := `INSERT INTO services (name, description, github_repo_name, status, ssl_status, build_logs, runtime_logs, root_directory, build_command, run_command, framework, port, env_vars, domain) 
	          VALUES ($1, $2, $3, 'draft', 'inactive', '', '', $4, $5, $6, $7, $8, $9, $10) 
	          RETURNING id, name, description, github_repo_name, status, build_command, run_command, framework,
	                    port, env_vars, infisical_workspace_id, infisical_env, 
	                    directory_path, ssl_status, build_logs, runtime_logs, root_directory, domain, created_at`

	var s models.Service
	var desc *string
	var repoNameStr *string
	var buildCmd *string
	var runCmd *string
	var fw *string
	var portScan *int
	var envVarsScan []byte
	var workspaceID *string
	var infEnv *string
	var dirPath *string
	var domainScan *string

	err = r.pool.QueryRow(ctx, query, name, description, repoName, rootDirectory, buildCommand, runCommand, framework, port, envVarsBytes, domain).Scan(
		&s.ID, &s.Name, &desc, &repoNameStr, &s.Status, &buildCmd, &runCmd, &fw,
		&portScan, &envVarsScan, &workspaceID, &infEnv,
		&dirPath, &s.SSLStatus, &s.BuildLogs, &s.RuntimeLogs, &s.RootDirectory, &domainScan, &s.CreatedAt,
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
	if fw != nil {
		s.Framework = *fw
	}
	if portScan != nil {
		s.Port = *portScan
	}
	if len(envVarsScan) > 0 {
		_ = json.Unmarshal(envVarsScan, &s.EnvVars)
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

func (r *ServiceRepository) UpdateService(ctx context.Context, id int, payload UpdateServiceRequestPayload) (*models.Service, error) {
	envVarsBytes, err := json.Marshal(payload.EnvVars)
	if err != nil {
		return nil, err
	}

	query := `UPDATE services 
	          SET name = $2, description = $3, build_command = $4, run_command = $5, port = $6, domain = $7, env_vars = $8
	          WHERE id = $1`

	cmdTag, err := r.pool.Exec(
		ctx, query, id, payload.Name, payload.Description, payload.BuildCommand, payload.RunCommand, payload.Port, payload.Domain, envVarsBytes,
	)
	if err != nil {
		return nil, err
	}
	if cmdTag.RowsAffected() == 0 {
		return nil, ErrServiceNotFound
	}

	return r.GetServiceByID(ctx, id)
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
