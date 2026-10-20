//go:build ignore

package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/hidenkeys/zidibackend/whatsappbot"
	"gorm.io/gorm"
)

func main() {
	// Set test environment variables
	os.Setenv("WHATSAPP_ACCESS_TOKEN", "test_token")
	os.Setenv("WHATSAPP_PHONE_NUMBER_ID", "test_phone_id")
	os.Setenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN", "test_verify_token")

	// Setup test database
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			log.Printf("Error: %v", err)
			return c.Status(500).SendString("Internal Server Error")
		},
	})

	// Middleware
	app.Use(logger.New())
	app.Use(cors.New())

	// Setup WhatsApp routes
	whatsappbot.SetupWhatsAppRoutes(app, db)

	// Test endpoint
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "WhatsApp Bot Test Server",
			"endpoints": map[string]string{
				"webhook_verify": "GET /whatsapp/webhook",
				"webhook_handle": "POST /whatsapp/webhook",
				"test_message":   "POST /whatsapp/test",
			},
		})
	})

	log.Println("🚀 WhatsApp Bot Test Server starting on :3000")
	log.Fatal(app.Listen(":3000"))
}