package models

import "time"

type User struct {
	ID                           int       `json:"id,omitempty"`
	Email                        string    `json:"email,omitempty"`
	PasswordHash                 string    `json:"-"`
	UserRole                     UserRole  `json:"role,omitempty"`
	PasswordResetToken           string    `json:"-"`
	PasswordResetTokenExpiration string    `json:"-"`
	CreatedAt                    time.Time `json:"created_at,omitempty"`
}

type UserRole string

var (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)
