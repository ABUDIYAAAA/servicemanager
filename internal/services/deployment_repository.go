package services

import (
	"context"

	"servicemanager/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeploymentRepository struct {
	db *pgxpool.Pool
}

func NewDeploymentRepository(db *pgxpool.Pool) *DeploymentRepository {
	return &DeploymentRepository{db: db}
}
type rowScanner interface {
	Scan(dest ...any) error
}

func scanDeployment(s rowScanner, d *models.Deployment) error {
	var commitSHA *string
	var directoryPath *string
	err := s.Scan(
		&d.ID, &d.ServiceID, &d.Status, &d.Trigger, &commitSHA, &d.BuildLogs, &d.RuntimeLogs, &directoryPath, &d.CreatedAt, &d.FinishedAt,
	)
	if err != nil {
		return err
	}
	if commitSHA != nil {
		d.CommitSHA = *commitSHA
	}
	if directoryPath != nil {
		d.DirectoryPath = *directoryPath
	}
	return nil
}

func (r *DeploymentRepository) CreateDeployment(ctx context.Context, serviceID int, trigger string, commitSHA string) (*models.Deployment, error) {
	var d models.Deployment
	var commitSHAPtr *string
	if commitSHA != "" {
		commitSHAPtr = &commitSHA
	}
	row := r.db.QueryRow(ctx,
		`INSERT INTO deployments (service_id, status, trigger, commit_sha, build_logs, runtime_logs) 
		VALUES ($1, 'queued', $2, $3, '', '') RETURNING id, service_id, status, trigger, commit_sha, build_logs, runtime_logs, directory_path, created_at, finished_at`,
		serviceID, trigger, commitSHAPtr)
	if err := scanDeployment(row, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DeploymentRepository) GetDeploymentByID(ctx context.Context, id int) (*models.Deployment, error) {
	var d models.Deployment
	row := r.db.QueryRow(ctx,
		`SELECT id, service_id, status, trigger, commit_sha, build_logs, runtime_logs, directory_path, created_at, finished_at 
		FROM deployments WHERE id = $1`, id)
	if err := scanDeployment(row, &d); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

func (r *DeploymentRepository) GetActiveDeployment(ctx context.Context, serviceID int) (*models.Deployment, error) {
	var d models.Deployment
	row := r.db.QueryRow(ctx,
		`SELECT id, service_id, status, trigger, commit_sha, build_logs, runtime_logs, directory_path, created_at, finished_at 
		FROM deployments WHERE service_id = $1 AND status NOT IN ('stopped', 'failed') ORDER BY created_at DESC LIMIT 1`, serviceID)
	if err := scanDeployment(row, &d); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

func (r *DeploymentRepository) GetDeploymentsByServiceID(ctx context.Context, serviceID int) ([]models.Deployment, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, service_id, status, trigger, commit_sha, build_logs, runtime_logs, directory_path, created_at, finished_at 
		FROM deployments WHERE service_id = $1 ORDER BY created_at DESC`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []models.Deployment
	for rows.Next() {
		var d models.Deployment
		if err := scanDeployment(rows, &d); err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return deployments, nil
}

func (r *DeploymentRepository) UpdateDeploymentStatus(ctx context.Context, id int, status string) error {
	if status == "stopped" || status == "failed" {
		_, err := r.db.Exec(ctx, `UPDATE deployments SET status = $1, finished_at = NOW() WHERE id = $2`, status, id)
		return err
	}
	_, err := r.db.Exec(ctx, `UPDATE deployments SET status = $1 WHERE id = $2`, status, id)
	return err
}

func (r *DeploymentRepository) AppendBuildLog(ctx context.Context, id int, logLine string) error {
	_, err := r.db.Exec(ctx, `UPDATE deployments SET build_logs = build_logs || $2 || '\n' WHERE id = $1`, id, logLine)
	return err
}

func (r *DeploymentRepository) AppendRuntimeLog(ctx context.Context, id int, logLine string) error {
	_, err := r.db.Exec(ctx, `UPDATE deployments SET runtime_logs = runtime_logs || $2 || '\n' WHERE id = $1`, id, logLine)
	return err
}

func (r *DeploymentRepository) SetDeploymentDirectory(ctx context.Context, id int, path string) error {
	_, err := r.db.Exec(ctx, `UPDATE deployments SET directory_path = $1 WHERE id = $2`, path, id)
	return err
}

func (r *DeploymentRepository) StopAllActiveDeployments(ctx context.Context, serviceID int) error {
	_, err := r.db.Exec(ctx, `UPDATE deployments SET status = 'stopped', finished_at = NOW() WHERE service_id = $1 AND status NOT IN ('stopped', 'failed')`, serviceID)
	return err
}
