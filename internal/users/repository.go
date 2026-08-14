package users

import (
	"context"
	"errors"
	"time"

	"servicemanager/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUserNotFound   = errors.New("user not found")
	ErrInviteNotFound = errors.New("invite not found")
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{
		pool: pool,
	}
}

func (r *UserRepository) GetAllUsers(ctx context.Context) ([]models.User, error) {
	var users []models.User

	query := "SELECT id, email, user_role, created_at FROM users ORDER BY id ASC"

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var user models.User
		err := rows.Scan(&user.ID, &user.Email, &user.UserRole, &user.CreatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return users, nil
}

func (r *UserRepository) RemoveUser(ctx context.Context, id int) error {
	query := "DELETE FROM users WHERE id = $1"
	cmdTag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *UserRepository) ChangeUserRole(ctx context.Context, id int, role models.UserRole) error {
	query := "UPDATE users SET user_role = $2 WHERE id = $1"
	cmdTag, err := r.pool.Exec(ctx, query, id, role)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `SELECT id, email, user_role, password_hash, password_reset_token, password_reset_token_expiration, created_at 
	          FROM users WHERE email = $1`

	var user models.User
	var resetToken *string
	var resetTokenExp *time.Time

	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.UserRole,
		&user.PasswordHash,
		&resetToken,
		&resetTokenExp,
		&user.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	if resetToken != nil {
		user.PasswordResetToken = *resetToken
	}
	if resetTokenExp != nil {
		user.PasswordResetTokenExpiration = resetTokenExp.Format(time.RFC3339)
	}

	return &user, nil
}

func (r *UserRepository) GetUserByID(ctx context.Context, id int) (*models.User, error) {
	query := `SELECT id, email, user_role, password_hash, password_reset_token, password_reset_token_expiration, created_at 
	          FROM users WHERE id = $1`

	var user models.User
	var resetToken *string
	var resetTokenExp *time.Time

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.UserRole,
		&user.PasswordHash,
		&resetToken,
		&resetTokenExp,
		&user.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	if resetToken != nil {
		user.PasswordResetToken = *resetToken
	}
	if resetTokenExp != nil {
		user.PasswordResetTokenExpiration = resetTokenExp.Format(time.RFC3339)
	}

	return &user, nil
}

func (r *UserRepository) CreateUser(ctx context.Context, email string, passwordHash string, role models.UserRole) (*models.User, error) {
	query := `INSERT INTO users (email, password_hash, user_role) 
	          VALUES ($1, $2, $3) 
	          RETURNING id, email, user_role, created_at`

	var user models.User
	err := r.pool.QueryRow(ctx, query, email, passwordHash, role).Scan(
		&user.ID,
		&user.Email,
		&user.UserRole,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *UserRepository) UpdateUserPassword(ctx context.Context, userID int, passwordHash string) error {
	query := "UPDATE users SET password_hash = $2 WHERE id = $1"
	cmdTag, err := r.pool.Exec(ctx, query, userID, passwordHash)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *UserRepository) SetPasswordResetToken(ctx context.Context, email string, token string, expiresAt time.Time) error {
	query := "UPDATE users SET password_reset_token = $2, password_reset_token_expiration = $3 WHERE email = $1"
	cmdTag, err := r.pool.Exec(ctx, query, email, token, expiresAt)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *UserRepository) GetUserByResetToken(ctx context.Context, token string) (*models.User, error) {
	query := `SELECT id, email, user_role, password_hash, password_reset_token, password_reset_token_expiration, created_at 
	          FROM users WHERE password_reset_token = $1`

	var user models.User
	var resetToken *string
	var resetTokenExp *time.Time

	err := r.pool.QueryRow(ctx, query, token).Scan(
		&user.ID,
		&user.Email,
		&user.UserRole,
		&user.PasswordHash,
		&resetToken,
		&resetTokenExp,
		&user.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	if resetToken != nil {
		user.PasswordResetToken = *resetToken
	}
	if resetTokenExp != nil {
		user.PasswordResetTokenExpiration = resetTokenExp.Format(time.RFC3339)
	}

	return &user, nil
}

func (r *UserRepository) ClearPasswordResetToken(ctx context.Context, userID int) error {
	query := "UPDATE users SET password_reset_token = NULL, password_reset_token_expiration = NULL WHERE id = $1"
	_, err := r.pool.Exec(ctx, query, userID)
	return err
}

func (r *UserRepository) CreateInvite(ctx context.Context, email string, token string) (*models.Invite, error) {
	query := `INSERT INTO invites (email, token) 
	          VALUES ($1, $2) 
	          ON CONFLICT (email) 
	          DO UPDATE SET token = EXCLUDED.token, created_at = CURRENT_TIMESTAMP 
	          RETURNING id, email, token, created_at`

	var invite models.Invite
	err := r.pool.QueryRow(ctx, query, email, token).Scan(
		&invite.ID,
		&invite.Email,
		&invite.Token,
		&invite.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &invite, nil
}

func (r *UserRepository) GetInviteByToken(ctx context.Context, token string) (*models.Invite, error) {
	query := "SELECT id, email, token, created_at FROM invites WHERE token = $1"

	var invite models.Invite
	err := r.pool.QueryRow(ctx, query, token).Scan(
		&invite.ID,
		&invite.Email,
		&invite.Token,
		&invite.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInviteNotFound
		}
		return nil, err
	}

	return &invite, nil
}

func (r *UserRepository) DeleteInvite(ctx context.Context, token string) error {
	query := "DELETE FROM invites WHERE token = $1"
	cmdTag, err := r.pool.Exec(ctx, query, token)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrInviteNotFound
	}
	return nil
}

func (r *UserRepository) GetAllInvites(ctx context.Context) ([]models.Invite, error) {
	var invites []models.Invite

	query := "SELECT id, email, token, created_at FROM invites ORDER BY id ASC"

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var invite models.Invite
		err := rows.Scan(&invite.ID, &invite.Email, &invite.Token, &invite.CreatedAt)
		if err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return invites, nil
}

func (r *UserRepository) SaveGithubInstallation(ctx context.Context, userID int, installationID int64, accountID int64, accountLogin string) error {
	query := `INSERT INTO github_installations (user_id, installation_id, account_id, account_login) 
	          VALUES ($1, $2, $3, $4) 
	          ON CONFLICT (installation_id) 
	          DO UPDATE SET user_id = EXCLUDED.user_id, account_id = EXCLUDED.account_id, account_login = EXCLUDED.account_login`
	_, err := r.pool.Exec(ctx, query, userID, installationID, accountID, accountLogin)
	return err
}

func (r *UserRepository) GetGithubInstallationByUserID(ctx context.Context, userID int) (*models.GithubInstallation, error) {
	query := `SELECT id, user_id, installation_id, account_id, account_login, created_at 
	          FROM github_installations WHERE user_id = $1 LIMIT 1`

	var inst models.GithubInstallation
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&inst.ID,
		&inst.UserID,
		&inst.InstallationID,
		&inst.AccountID,
		&inst.AccountLogin,
		&inst.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &inst, nil
}

func (r *UserRepository) DeleteGithubInstallation(ctx context.Context, userID int) error {
	query := "DELETE FROM github_installations WHERE user_id = $1"
	_, err := r.pool.Exec(ctx, query, userID)
	return err
}
