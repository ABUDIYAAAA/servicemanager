package users

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
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
	repository       *UserRepository
	jwtSecret        string
	jwtExpiry        string
	githubAppID      string
	githubPrivateKey string
	baseURL          string
}

func NewUserService(userRepository *UserRepository, jwtSecret string, jwtExpiry string, githubAppID string, githubPrivateKey string, baseURL string) *UserService {
	return &UserService{
		repository:       userRepository,
		jwtSecret:        jwtSecret,
		jwtExpiry:        jwtExpiry,
		githubAppID:      githubAppID,
		githubPrivateKey: githubPrivateKey,
		baseURL:          baseURL,
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

	inst, _ := s.repository.GetGithubInstallationByUserID(ctx, user.ID)
	githubInstalled := inst != nil

	return &LoginResponsePayload{
		Token: token,
		User: UserResponsePayload{
			ID:              user.ID,
			Email:           user.Email,
			Role:            string(user.UserRole),
			GithubInstalled: githubInstalled,
			CreatedAt:       user.CreatedAt,
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

	invite, err := s.repository.CreateInvite(ctx, email, token)
	if err == nil {
		inviteURL := fmt.Sprintf("%s/accept-invite?token=%s&email=%s", s.baseURL, token, email)
		htmlBody := fmt.Sprintf(`
			<p>You have been invited to join Service Manager.</p>
			<p><a href="%s" style="display:inline-block;padding:10px 20px;background-color:#0d6efd;color:#ffffff;text-decoration:none;border-radius:5px;">Accept Invitation</a></p>
			<hr style="margin-top:20px;margin-bottom:20px;border:0;border-top:1px solid #eee;" />
			<p style="font-size:12px;color:#666;">If the button above does not work, copy and paste the following URL into your browser:</p>
			<p style="font-size:12px;color:#666;"><a href="%s">%s</a></p>
		`, inviteURL, inviteURL, inviteURL)

		utils.QueueHTMLEmail(
			email,
			"Invitation to Join Service Manager",
			htmlBody,
		)
	}
	return invite, err
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

	utils.QueueEmail(
		email,
		"Password Reset Request",
		fmt.Sprintf("We received a request to reset your password. Use token %s to reset your password. This token expires in 1 hour.", token),
	)

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

func (s *UserService) InstallGithub(ctx context.Context, userID int, installationID int64) error {
	details, err := utils.GetInstallationDetails(s.githubAppID, s.githubPrivateKey, installationID)
	if err != nil {
		return fmt.Errorf("failed to fetch installation from github: %w", err)
	}

	return s.repository.SaveGithubInstallation(ctx, userID, installationID, details.Account.ID, details.Account.Login)
}

func (s *UserService) IsGithubInstalled(ctx context.Context, userID int) (bool, error) {
	inst, err := s.repository.GetGithubInstallationByUserID(ctx, userID)
	if err != nil {
		return false, err
	}
	return inst != nil, nil
}

func (s *UserService) GetAllGithubInstallations(ctx context.Context, userID int) ([]models.GithubInstallation, error) {
	return s.repository.GetAllGithubInstallationsByUserID(ctx, userID)
}

func (s *UserService) GetGithubRepositories(ctx context.Context, userID int) ([]models.GithubRepo, error) {
	installations, err := s.repository.GetAllGithubInstallationsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(installations) == 0 {
		return nil, fmt.Errorf("github app not installed for this user")
	}

	var allRepos []models.GithubRepo
	for _, inst := range installations {
		repos, err := utils.GetInstallationRepositories(s.githubAppID, s.githubPrivateKey, inst.InstallationID)
		if err != nil {
			// Log error and continue to fetch from other installations
			continue
		}
		allRepos = append(allRepos, repos...)
	}

	// Sort by pushed_at descending (most recent pushed first)
	sort.Slice(allRepos, func(i, j int) bool {
		return allRepos[i].PushedAt.After(allRepos[j].PushedAt)
	})

	return allRepos, nil
}
