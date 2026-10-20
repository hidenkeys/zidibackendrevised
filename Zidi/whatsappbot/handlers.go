package whatsappbot

import (
	"encoding/json"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidibackend/services"
	"github.com/hidenkeys/zidibackend/utils"
	"gorm.io/gorm"
)

// WebhookVerification handles the webhook verification
func WebhookVerification(c *fiber.Ctx) error {
	mode := c.Query("hub.mode")
	token := c.Query("hub.verify_token")
	challenge := c.Query("hub.challenge")

	verifyToken := os.Getenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN")

	if verifyToken != "" && mode == "subscribe" && token == verifyToken {
		log.Println("✅ Webhook verified successfully")
		return c.SendString(challenge)
	}

	log.Println("❌ Webhook verification failed")
	return c.Status(403).SendString("Forbidden")
}

// WebhookHandler handles incoming WhatsApp messages
func WebhookHandler(db *gorm.DB) fiber.Handler {
	return WebhookHandlerWithCommerce(db, nil)
}

func WebhookHandlerWithCommerce(db *gorm.DB, channel *services.CommerceChannelService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		signature := c.Get("X-Hub-Signature-256")
		body := c.Body()

		if signature == "" || !VerifyWebhookSignature(body, signature) {
			log.Println("❌ Invalid webhook signature")
			return c.Status(403).SendString("Invalid signature")
		}

		// Parse webhook payload
		var payload utils.WebhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			log.Printf("❌ Error parsing webhook payload: %v", err)
			return c.Status(400).SendString("Invalid payload")
		}

		// Handle the webhook
		if err := HandleWebhookWithCommerceContext(c.UserContext(), payload, db, channel); err != nil {
			log.Printf("❌ Error handling webhook: %v", err)
			return c.Status(500).SendString("Internal server error")
		}

		return c.SendStatus(200)
	}
}

// TestMessage sends a test message (for development)
func TestMessage(c *fiber.Ctx) error {
	type TestRequest struct {
		To      string `json:"to"`
		Message string `json:"message"`
	}

	var req TestRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := utils.SendWhatsAppMessage(req.To, req.Message); err != nil {
		log.Printf("❌ Error sending test message: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to send message"})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Message sent"})
}
