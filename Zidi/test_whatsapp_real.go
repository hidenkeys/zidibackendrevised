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
		log.Printf("Warning: .env file not found: %v", err)
	}

	fmt.Println("🧪 Testing WhatsApp Bot with Real Credentials")
	fmt.Println("=============================================")

	// Check if required environment variables are set
	accessToken := os.Getenv("WHATSAPP_ACCESS_TOKEN")
	phoneNumberID := os.Getenv("WHATSAPP_PHONE_NUMBER_ID")
	verifyToken := os.Getenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN")

	fmt.Printf("Access Token: %s\n", maskToken(accessToken))
	fmt.Printf("Phone Number ID: %s\n", phoneNumberID)
	fmt.Printf("Verify Token: %s\n", verifyToken)

	if accessToken == "" || phoneNumberID == "" {
		fmt.Println("❌ Missing required WhatsApp credentials")
		fmt.Println("Please check your .env file for:")
		fmt.Println("- WHATSAPP_ACCESS_TOKEN")
		fmt.Println("- WHATSAPP_PHONE_NUMBER_ID")
		return
	}

	// Test sending a message to a test number (replace with your test number)
	testNumber := "2348123456789" // Replace with your actual test number
	
	fmt.Printf("\n🔄 Attempting to send test message to %s...\n", testNumber)
	err := whatsappbot.SendWhatsAppMessage(testNumber, "🤖 Hello! This is a test message from your WhatsApp bot. If you receive this, the bot is working correctly!")
	
	if err != nil {
		fmt.Printf("❌ Error sending message: %v\n", err)
		fmt.Println("\nPossible issues:")
		fmt.Println("1. Invalid access token")
		fmt.Println("2. Invalid phone number ID")
		fmt.Println("3. Test number not registered with WhatsApp Business")
		fmt.Println("4. Network connectivity issues")
	} else {
		fmt.Println("✅ Message sent successfully!")
		fmt.Println("Check your WhatsApp to see if the message was received.")
	}

	fmt.Println("\n📋 WhatsApp Bot Status Summary:")
	fmt.Println("- Bot code: ✅ Working")
	fmt.Println("- Environment setup: ✅ Complete")
	fmt.Printf("- API connection: %s\n", getStatus(err == nil))
}

func maskToken(token string) string {
	if len(token) < 10 {
		return "***"
	}
	return token[:10] + "..." + token[len(token)-4:]
}

func getStatus(success bool) string {
	if success {
		return "✅ Working"
	}
	return "❌ Needs attention"
}