package mail

import (
	"log/slog"
	"strconv"

	"github.com/wneessen/go-mail"
)

type MailService struct {
	client *mail.Client
	from   string
}

func NewMailService(host, portStr, user, pass, from string) *MailService {
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
		slog.Error("Failed to create SMTP client", slog.Any("error", err))
		client = nil
	}

	return &MailService{
		client: client,
		from:   from,
	}
}

func (s *MailService) SendEmail(to, subject, body string, isHTML bool) error {
	if s.client == nil {
		slog.Info("[DUMMY SMTP LOG]",
			slog.String("to", to),
			slog.String("subject", subject),
			slog.String("body", body),
		)
		return nil
	}

	m := mail.NewMsg()
	if err := m.From(s.from); err != nil {
		return err
	}
	if err := m.To(to); err != nil {
		return err
	}
	m.Subject(subject)
	if isHTML {
		m.SetBodyString(mail.TypeTextHTML, body)
	} else {
		m.SetBodyString(mail.TypeTextPlain, body)
	}

	return s.client.DialAndSend(m)
}
