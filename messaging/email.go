package messaging

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/smtp"
	"strings"

	brevo "github.com/sendinblue/APIv3-go-library/v2/lib"
)

type EmailMessage struct {
	To       string
	Subject  string
	HTMLBody string
}

type EmailSender interface {
	Send(context.Context, EmailMessage) error
}

type SMTPEmailSender struct {
	from     string
	password string
	host     string
	address  string
}

type BrevoEmailSender struct {
	apiKey   string
	from     string
	fromName string
}

func NewConfiguredEmailSender(brevoAPIKey, brevoFrom, brevoFromName, smtpFrom, smtpPassword, smtpHost, smtpAddress string) EmailSender {
	if strings.TrimSpace(smtpFrom) != "" && strings.TrimSpace(smtpPassword) != "" {
		return NewSMTPEmailSender(smtpFrom, smtpPassword, smtpHost, smtpAddress)
	}
	if strings.TrimSpace(brevoAPIKey) != "" {
		return NewBrevoEmailSender(brevoAPIKey, brevoFrom, brevoFromName)
	}
	return nil
}

func NewBrevoEmailSender(apiKey, from, fromName string) *BrevoEmailSender {
	if strings.TrimSpace(from) == "" {
		from = "letimapro23@gmail.com"
	}
	if strings.TrimSpace(fromName) == "" {
		fromName = "Zidi"
	}
	return &BrevoEmailSender{apiKey: strings.TrimSpace(apiKey), from: strings.TrimSpace(from), fromName: strings.TrimSpace(fromName)}
}

func (s *BrevoEmailSender) Send(ctx context.Context, message EmailMessage) error {
	if s == nil || s.apiKey == "" {
		return errors.New("commerce Brevo sender is not configured")
	}
	if _, err := mail.ParseAddress(message.To); err != nil {
		return errors.New("commerce email recipient is invalid")
	}
	subject := strings.NewReplacer("\r", "", "\n", "").Replace(strings.TrimSpace(message.Subject))
	if subject == "" || strings.TrimSpace(message.HTMLBody) == "" {
		return errors.New("commerce email subject and body are required")
	}
	configuration := brevo.NewConfiguration()
	configuration.AddDefaultHeader("api-key", s.apiKey)
	client := brevo.NewAPIClient(configuration)
	_, _, err := client.TransactionalEmailsApi.SendTransacEmail(ctx, brevo.SendSmtpEmail{
		Sender: &brevo.SendSmtpEmailSender{Name: s.fromName, Email: s.from},
		To:     []brevo.SendSmtpEmailTo{{Email: message.To}}, Subject: subject, HtmlContent: message.HTMLBody,
	})
	if err != nil {
		return fmt.Errorf("send commerce email through Brevo: %w", err)
	}
	return nil
}

func NewSMTPEmailSender(from, password, host, address string) *SMTPEmailSender {
	if strings.TrimSpace(host) == "" {
		host = "smtp.zoho.com"
	}
	if strings.TrimSpace(address) == "" {
		address = host + ":587"
	}
	return &SMTPEmailSender{from: strings.TrimSpace(from), password: strings.TrimSpace(password), host: strings.TrimSpace(host), address: strings.TrimSpace(address)}
}

func (s *SMTPEmailSender) Send(ctx context.Context, message EmailMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.from == "" || s.password == "" {
		return errors.New("commerce SMTP sender is not configured")
	}
	if _, err := mail.ParseAddress(message.To); err != nil {
		return errors.New("commerce email recipient is invalid")
	}
	subject := strings.NewReplacer("\r", "", "\n", "").Replace(strings.TrimSpace(message.Subject))
	if subject == "" || strings.TrimSpace(message.HTMLBody) == "" {
		return errors.New("commerce email subject and body are required")
	}
	payload := []byte("From: " + s.from + "\r\n" +
		"To: " + message.To + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n\r\n" + message.HTMLBody)
	if err := smtp.SendMail(s.address, smtp.PlainAuth("", s.from, s.password, s.host), s.from, []string{message.To}, payload); err != nil {
		return fmt.Errorf("send commerce email: %w", err)
	}
	return nil
}
