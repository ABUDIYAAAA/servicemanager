package tasks

// Task Types represent the different asynchronous operations handled by workers.
const (
	// Deployment tasks
	TypeDeploymentCreate = "deployment.create"
	TypeDeploymentUpdate = "deployment.update"
	TypeDeploymentDelete = "deployment.delete"
	TypeServiceDeploy    = "service.deploy"

	// Add more task types below as the application grows
	// ...
)

// Queue Names specify which queue a task should be routed to.
const (
	QueueDeployments = "deployments"
	QueueDefault     = "default"
	QueueCritical    = "critical"
)
