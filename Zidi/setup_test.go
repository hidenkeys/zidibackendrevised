//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidibackend/config"
	"github.com/hidenkeys/zidibackend/whatsappbot"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	// Check credentials
	fmt.Println("🔍 Checking WhatsApp Configuration...")
	
	token := os.Getenv("WHATSAPP_ACCESS_TOKEN")
	phoneID := os.Getenv("WHATSAPP_PHONE_NUMBER_ID")
	verifyToken := os.Getenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN")

	if token == "" || phoneID == "" || verifyToken == "" {
		log.Fatal("❌ Missing WhatsApp credentials in .env file")
	}

	fmt.Printf("✅ Access Token: %s...\n", token[:20])
	fmt.Printf("✅ Phone Number ID: %s\n", phoneID)
	fmt.Printf("✅ Verify Token: %s\n", verifyToken)

	// Initialize database
	db, err := config.InitDB()
	if err != nil {
		log.Fatal("❌ Database connection failed:", err)
	}
	fmt.Println("✅ Database connected")

	// Create Fiber app
	app := fiber.New()

	// Setup WhatsApp routes
	whatsappbot.SetupWhatsAppRoutes(app, db)

	// Test endpoint
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "WhatsApp Bot Ready",
			"webhook": "http://localhost:8080/whatsapp/webhook",
		})
	})

	fmt.Println("\n🚀 WhatsApp Bot Server starting on :8080")
	fmt.Println("🔗 Webhook URL: http://localhost:8080/whatsapp/webhook")
	fmt.Println("🔑 Verify Token: " + verifyToken)
	
	log.Fatal(app.Listen(":8080"))
}