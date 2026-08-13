package users

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"servicemanager/internal/models"
	"servicemanager/internal/utils"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrTokenExpired       = errors.New("token has expired")
	ErrEmailMismatch      = errors.New("invite token email mismatch")
)

type UserService struct {
	repository *UserRepository
	jwtSecret  string
	jwtExpiry  string
}

func NewUserService(userRepository *UserRepository, jwtSecret string, jwtExpiry string) *UserService {
	return &UserService{
		repository: userRepository,
		jwtSecret:  jwtSecret,
		jwtExpiry:  jwtExpiry,
	}
}

func (s *UserService) GetAllUsers(ctx context.Context) ([]models.User, error) {
	return s.repository.GetAllUsers(ctx)
}

func (s *UserService) RemoveUser(ctx context.Context, id int) error {
	return s.repository.RemoveUser(ctx, id)
}

func (s *UserService) ChangeUserRole(ctx context.Context, id int, role models.UserRole) error {
	if role != models.RoleAdmin && role != models.RoleUser {
		return fmt.Errorf("invalid role: %s", role)
	}
	return s.repository.ChangeUserRole(ctx, id, role)
}

func (s *UserService) Login(ctx context.Context, email, password string) (*LoginResponsePayload, error) {
	user, err := s.repository.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	// Verify Argon2 password hash
	match, err := utils.VerifyPassword(password, user.PasswordHash)
	if err != nil || !match {
		return nil, ErrInvalidCredentials
	}

	// Generate JWT
	token, err := utils.GenerateToken(user.ID, user.Email, string(user.UserRole), s.jwtSecret, s.jwtExpiry)
	if err != nil {
		return nil, err
	}

	return &LoginResponsePayload{
		Token: token,
		User: UserResponsePayload{
			ID:        user.ID,
			Email:     user.Email,
			Role:      string(user.UserRole),
			CreatedAt: user.CreatedAt,
		},
	}, nil
}

func (s *UserService) CreateInvite(ctx context.Context, email string) (*models.Invite, error) {
	// Generate 32-byte secure random hex token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)

	return s.repository.CreateInvite(ctx, email, token)
}

func (s *UserService) GetAllInvites(ctx context.Context) ([]models.Invite, error) {
	return s.repository.GetAllInvites(ctx)
}

func (s *UserService) DeleteInvite(ctx context.Context, token string) error {
	return s.repository.DeleteInvite(ctx, token)
}

func (s *UserService) AcceptInvite(ctx context.Context, token, email, password string) (*models.User, error) {
	invite, err := s.repository.GetInviteByToken(ctx, token)
	if err != nil {
		return nil, err
	}

	if invite.Email != email {
		return nil, ErrEmailMismatch
	}

	// Hash password using Argon2
	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return nil, err
	}

	// Create user
	user, err := s.repository.CreateUser(ctx, email, passwordHash, models.RoleUser)
	if err != nil {
		return nil, err
	}

	// Clean up invite
	_ = s.repository.DeleteInvite(ctx, token)

	return user, nil
}

func (s *UserService) ForgotPassword(ctx context.Context, email string) (string, error) {
	// Verify user exists
	_, err := s.repository.GetUserByEmail(ctx, email)
	if err != nil {
		return "", err
	}

	// Generate 32-byte secure random hex token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(tokenBytes)

	// Set expiration to 1 hour from now
	expiresAt := time.Now().Add(1 * time.Hour)

	err = s.repository.SetPasswordResetToken(ctx, email, token, expiresAt)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s *UserService) ResetPassword(ctx context.Context, token, newPassword string) error {
	user, err := s.repository.GetUserByResetToken(ctx, token)
	if err != nil {
		return err
	}

	// Parse and check expiration
	expiresAt, err := time.Parse(time.RFC3339, user.PasswordResetTokenExpiration)
	if err != nil {
		return err
	}

	if time.Now().After(expiresAt) {
		return ErrTokenExpired
	}

	// Hash new password using Argon2
	passwordHash, err := utils.HashPassword(newPassword)
	if err != nil {
		return err
	}

	// Update password
	err = s.repository.UpdateUserPassword(ctx, user.ID, passwordHash)
	if err != nil {
		return err
	}

	// Clear reset token
	return s.repository.ClearPasswordResetToken(ctx, user.ID)
}

func (s *UserService) GetUserByID(ctx context.Context, id int) (*models.User, error) {
	return s.repository.GetUserByID(ctx, id)
}
