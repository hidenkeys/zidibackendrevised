//go:build ignore

package main

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidibackend/whatsappbot"
	"gorm.io/gorm"
)

// Add this to your existing main.go or create new routes
func setupWhatsAppRoutes(app *fiber.App, db *gorm.DB) {
	// WhatsApp webhook routes
	app.Get("/whatsapp/webhook", whatsappbot.WebhookVerification)
	app.Post("/whatsapp/webhook", whatsappbot.WebhookHandler(db))
	
	// Optional: Test endpoint
	app.Post("/whatsapp/test", whatsappbot.TestMessage)
}