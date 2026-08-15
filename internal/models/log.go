package models

import "time"

// LogEvent represents a single line of log output from a container
type LogEvent struct {
	ServiceID    int       `bson:"service_id" json:"service_id"`
	DeploymentID int       `bson:"deployment_id" json:"deployment_id"`
	ContainerID  string    `bson:"container_id" json:"container_id"`
	Timestamp    time.Time `bson:"timestamp" json:"timestamp"`
	Stream       string    `bson:"stream" json:"stream"`   // "stdout" or "stderr"
	Message      string    `bson:"message" json:"message"` // The actual log text
}
