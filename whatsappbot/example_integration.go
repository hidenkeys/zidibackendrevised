package whatsappbot

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// SetupWhatsAppRoutes adds WhatsApp routes to your Fiber app
func SetupWhatsAppRoutes(app *fiber.App, db *gorm.DB) {
	whatsapp := app.Group("/whatsapp")
	
	// Webhook verification (GET)
	whatsapp.Get("/webhook", WebhookVerification)
	
	// Webhook handler (POST)
	whatsapp.Post("/webhook", WebhookHandler(db))
	
	// Test endpoint (optional, remove in production)
	whatsapp.Post("/test", TestMessage)
}

// Example usage in your main.go:
/*
import (
	"github.com/hidenkeys/zidibackend/whatsappbot"
)

func main() {
	app := fiber.New()
	db := setupDatabase() // your database setup
	
	// Setup WhatsApp routes
	whatsappbot.SetupWhatsAppRoutes(app, db)
	
	app.Listen(":8080")
}
*/