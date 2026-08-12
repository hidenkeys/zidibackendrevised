package whatsappbot

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestWebhookRejectsMissingSignature(t *testing.T) {
	app := fiber.New()
	app.Post("/whatsapp/webhook", WebhookHandler(nil))

	request := httptest.NewRequest("POST", "/whatsapp/webhook", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("execute webhook request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusForbidden {
		t.Fatalf("expected status %d, got %d", fiber.StatusForbidden, response.StatusCode)
	}
}

func TestWebhookVerificationRejectsWhenServerTokenIsMissing(t *testing.T) {
	previous, existed := os.LookupEnv("WHATSAPP_WEBHOOK_VERIFY_TOKEN")
	_ = os.Unsetenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN")
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN", previous)
		} else {
			_ = os.Unsetenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN")
		}
	})
	app := fiber.New()
	app.Get("/whatsapp/webhook", WebhookVerification)
	request := httptest.NewRequest("GET", "/whatsapp/webhook?hub.mode=subscribe&hub.challenge=challenge", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("execute verification request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusForbidden {
		t.Fatalf("expected status %d, got %d", fiber.StatusForbidden, response.StatusCode)
	}
}
