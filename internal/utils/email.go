package utils

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/wneessen/go-mail"
)

type EmailTask struct {
	To      string
	Subject string
	Body    string
	IsHTML  bool
}

var emailChan = make(chan EmailTask, 100)

// QueueEmail pushes an email to the in-memory queue.
func QueueEmail(to, subject, body string) {
	select {
	case emailChan <- EmailTask{To: to, Subject: subject, Body: body, IsHTML: false}:
		slog.Info("Queued email", slog.String("to", to), slog.String("subject", subject))
	default:
		slog.Warn("Email queue is full, dropping email", slog.String("to", to), slog.String("subject", subject))
	}
}

// QueueHTMLEmail pushes an HTML email to the in-memory queue.
func QueueHTMLEmail(to, subject, htmlBody string) {
	select {
	case emailChan <- EmailTask{To: to, Subject: subject, Body: htmlBody, IsHTML: true}:
		slog.Info("Queued HTML email", slog.String("to", to), slog.String("subject", subject))
	default:
		slog.Warn("Email queue is full, dropping email", slog.String("to", to), slog.String("subject", subject))
	}
}

// StartEmailWorker starts a background worker loop to consume from the channel.
func StartEmailWorker(ctx context.Context, host, portStr, user, pass, from string) {
	port, err := strconv.Atoi(portStr)
	if err != nil {
		port = 25
	}

	var opts []mail.Option
	opts = append(opts, mail.WithPort(port))
	opts = append(opts, mail.WithTLSPolicy(mail.TLSOpportunistic))

	if user != "" && pass != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthPlain))
		opts = append(opts, mail.WithUsername(user))
		opts = append(opts, mail.WithPassword(pass))
	}

	client, err := mail.NewClient(host, opts...)
	if err != nil {
		slog.Error("Failed to create SMTP client (falling back to stdout logs)", slog.Any("error", err))
	}

	slog.Info("Starting background email queue worker...")

	go func() {
		for {
			select {
			case task := <-emailChan:
				if client == nil {
					slog.Info("[DUMMY SMTP LOG]",
						slog.String("to", task.To),
						slog.String("subject", task.Subject),
						slog.String("body", task.Body),
					)
					continue
				}

				m := mail.NewMsg()
				if err := m.From(from); err != nil {
					slog.Error("Failed to set SMTP sender", slog.String("from", from), slog.Any("error", err))
					continue
				}
				if err := m.To(task.To); err != nil {
					slog.Error("Failed to set SMTP recipient", slog.String("to", task.To), slog.Any("error", err))
					continue
				}
				m.Subject(task.Subject)
				if task.IsHTML {
					m.SetBodyString(mail.TypeTextHTML, task.Body)
				} else {
					m.SetBodyString(mail.TypeTextPlain, task.Body)
				}

				if err := client.DialAndSend(m); err != nil {
					slog.Error("Failed to send email",
						slog.String("to", task.To),
						slog.Any("error", err),
						slog.String("fallback_subject", task.Subject),
						slog.String("fallback_body", task.Body),
					)
				} else {
					slog.Info("Successfully sent email", slog.String("to", task.To), slog.String("subject", task.Subject))
				}

			case <-ctx.Done():
				slog.Info("Stopping email queue worker...")
				return
			}
		}
	}()
}
