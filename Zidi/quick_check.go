//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/hidenkeys/zidibackend/whatsappbot"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found")
	}

	// Check credentials
	token := os.Getenv("WHATSAPP_ACCESS_TOKEN")
	phoneID := os.Getenv("WHATSAPP_PHONE_NUMBER_ID")
	
	fmt.Println("🔍 WhatsApp Configuration Check:")
	fmt.Printf("✅ Access Token: %s...\n", token[:20])
	fmt.Printf("✅ Phone Number ID: %s\n", phoneID)
	
	// Test message (will show error details)
	testPhone := "2348085105382" // Your phone number
	err := whatsappbot.SendWhatsAppMessage(testPhone, "🤖 Test message from Zidi Bot!")
	
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		fmt.Println("\n💡 This error will help us fix the configuration")
	} else {
		fmt.Println("✅ Message sent successfully!")
	}
}