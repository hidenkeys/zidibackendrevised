//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/hidenkeys/zidibackend/config"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found")
	}

	// Check required environment variables
	requiredVars := []string{
		"WHATSAPP_ACCESS_TOKEN",
		"WHATSAPP_PHONE_NUMBER_ID", 
		"WHATSAPP_WEBHOOK_VERIFY_TOKEN",
	}

	fmt.Println("🔍 Checking WhatsApp configuration...")
	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			log.Fatalf("❌ Missing required environment variable: %s", v)
		}
		fmt.Printf("✅ %s: configured\n", v)
	}

	// Initialize database
	db, err := config.InitDB()
	if err != nil {
		log.Fatal("❌ Failed to connect to database:", err)
	}
	fmt.Println("✅ Database connected")

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
			"status":  "running",
			"endpoints": map[string]string{
				"webhook_verify": "GET /whatsapp/webhook",
				"webhook_handle": "POST /whatsapp/webhook", 
				"test_message":   "POST /whatsapp/test",
			},
		})
	})

	// Test message endpoint
	app.Post("/test-send", func(c *fiber.Ctx) error {
		type TestRequest struct {
			Phone   string `json:"phone"`
			Message string `json:"message"`
		}

		var req TestRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
		}

		if req.Phone == "" || req.Message == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Phone and message are required"})
		}

		err := whatsappbot.SendWhatsAppMessage(req.Phone, req.Message)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"success": true, "message": "Message sent successfully"})
	})

	fmt.Println("\n🚀 WhatsApp Bot Test Server starting on :8080")
	fmt.Println("📱 Test your bot by sending a POST request to /test-send")
	fmt.Println("🔗 Webhook URL: http://localhost:8080/whatsapp/webhook")
	
	log.Fatal(app.Listen(":8080"))
}