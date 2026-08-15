package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"servicemanager/internal/queue"
	"servicemanager/internal/tasks"
)

type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	IsHTML  bool   `json:"is_html"`
}

// QueueEmailTask creates a new Asynq task for sending an email and pushes it to the email queue.
func QueueEmailTask(client *asynq.Client, to, subject, body string, isHTML bool) error {
	payload, err := json.Marshal(EmailPayload{
		To:      to,
		Subject: subject,
		Body:    body,
		IsHTML:  isHTML,
	})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tasks.TypeEmailSend, payload, asynq.Queue(tasks.QueueEmail))
	info, err := client.Enqueue(task)
	if err != nil {
		slog.Error("Failed to enqueue email task", slog.Any("error", err))
		return err
	}
	slog.Info("Enqueued email task", slog.String("id", info.ID), slog.String("queue", info.Queue))
	return nil
}

// EmailWorker encapsulates the mail service and its Asynq handler.
type EmailWorker struct {
	mailService *MailService
}

func NewEmailWorker(mailService *MailService) *EmailWorker {
	return &EmailWorker{
		mailService: mailService,
	}
}

// HandleEmailTask is the Asynq handler for email tasks.
func (w *EmailWorker) HandleEmailTask(ctx context.Context, t *asynq.Task) error {
	var payload EmailPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal email payload: %w", err)
	}

	slog.Info("Processing email task from queue",
		slog.String("to", payload.To),
		slog.String("subject", payload.Subject),
	)

	if err := w.mailService.SendEmail(payload.To, payload.Subject, payload.Body, payload.IsHTML); err != nil {
		slog.Error("Failed to send email via MailService", slog.Any("error", err))
		return err
	}
	return nil
}

// StartWorker initializes and starts the Asynq email worker in the background.
// It returns a shutdown function.
func StartWorker(redisURI string, mailSvc *MailService) func() {
	if redisURI == "" {
		slog.Warn("REDIS_URI not configured; email queue worker will not start")
		return func() {}
	}

	asynqSrv := queue.NewWorkerServer(redisURI, tasks.QueueEmail, 5)
	mux := queue.NewWorkerMux()
	
	worker := NewEmailWorker(mailSvc)
	mux.HandleFunc(tasks.TypeEmailSend, worker.HandleEmailTask)

	slog.Info("Starting Asynq email queue worker...", slog.String("redis", redisURI))
	go func() {
		if err := asynqSrv.Start(mux); err != nil {
			slog.Error("Asynq email worker failed", slog.Any("error", err))
		}
	}()

	return func() {
		slog.Info("Shutting down Asynq email worker...")
		asynqSrv.Shutdown()
	}
}
