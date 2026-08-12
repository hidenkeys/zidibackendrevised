package messaging

import (
	"testing"
)

func TestConfiguredEmailSenderPrefersSMTP(t *testing.T) {
	sender := NewConfiguredEmailSender("brevo-key", "sender@example.com", "Zidi", "smtp@example.com", "password", "smtp.example.com", "smtp.example.com:587")
	if _, ok := sender.(*SMTPEmailSender); !ok {
		t.Fatalf("expected SMTP sender, got %T", sender)
	}
}

func TestConfiguredEmailSenderFallsBackToBrevo(t *testing.T) {
	sender := NewConfiguredEmailSender("brevo-key", "sender@example.com", "Zidi", "", "", "", "")
	brevoSender, ok := sender.(*BrevoEmailSender)
	if !ok {
		t.Fatalf("expected Brevo sender, got %T", sender)
	}
	if brevoSender.from != "sender@example.com" || brevoSender.fromName != "Zidi" {
		t.Fatalf("unexpected Brevo sender configuration: %#v", brevoSender)
	}
}

func TestConfiguredEmailSenderReturnsNilWithoutCredentials(t *testing.T) {
	if sender := NewConfiguredEmailSender("", "", "", "", "", "", ""); sender != nil {
		t.Fatalf("expected no email sender, got %T", sender)
	}
}
