package users

import (
	"time"
)

type LoginRequestPayload struct {
	Email    string `json:"email" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type LoginResponsePayload struct {
	Token string              `json:"token,omitempty"`
	User  UserResponsePayload `json:"user"`
}

type InviteRequestPayload struct {
	Email string `json:"email" validate:"required"`
}

type AcceptInviteRequestPayload struct {
	Token    string `json:"token" validate:"required"`
	Email    string `json:"email" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type ForgotPasswordRequestPayload struct {
	Email string `json:"email" validate:"required"`
}

type ResetPasswordRequestPayload struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type ChangeRoleRequestPayload struct {
	Role string `json:"role" validate:"required"`
}

type UserResponsePayload struct {
	ID              int       `json:"id"`
	Email           string    `json:"email"`
	Role            string    `json:"role"`
	GithubInstalled bool      `json:"github_installed"`
	CreatedAt       time.Time `json:"created_at"`
}

type InviteResponsePayload struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type GithubInstallRequestPayload struct {
	InstallationID int64 `json:"installation_id" validate:"required"`
}
