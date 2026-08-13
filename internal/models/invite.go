package models

import "time"

type Invite struct {
	ID        int       `json:"id,omitempty"`
	Email     string    `json:"email,omitempty"`
	Token     string    `json:"token,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}
