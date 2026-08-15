package tasks

// Task Types represent the different asynchronous operations handled by workers.
const (
	TypeDeploymentCreate = "deployment.create"
	TypeEmailSend        = "email.send"
)

// Queue Names specify which queue a task should be routed to.
const (
	QueueDeployments = "deployments"
	QueueEmail       = "email"
	QueueDefault     = "default"
	QueueCritical    = "critical"
)
