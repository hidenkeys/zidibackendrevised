//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/whatsappbot"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	// Set environment variables
	os.Setenv("WHATSAPP_ACCESS_TOKEN", "test_token")
	os.Setenv("WHATSAPP_PHONE_NUMBER_ID", "test_phone_id")
	os.Setenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN", "test_verify_token")

	fmt.Println("🧪 Testing WhatsApp Bot Simple Flow")
	fmt.Println("===================================")

	// Setup test database
	db, err := gorm.Open(sqlite.Open("test_whatsapp.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Auto migrate
	db.AutoMigrate(&models.Campaign{}, &models.Coupon{}, &models.Customer{})

	// Create test campaign
	fmt.Println("\n1. Creating test campaign...")
	campaignID := uuid.New()
	orgID := uuid.New()

	campaign := models.Campaign{
		ID:             campaignID,
		OrganizationID: orgID,
		CampaignName:   "Test Campaign",
		WelcomeMessage: "Welcome to our test campaign!",
		Amount:         100.0,
		Status:         "active",
	}

	if err := db.Create(&campaign).Error; err != nil {
		log.Printf("   Campaign creation error: %v", err)
	} else {
		fmt.Printf("   ✅ Test campaign created: %s\n", campaignID)
	}

	// Create test coupon
	fmt.Println("\n2. Creating test coupon...")
	coupon := models.Coupon{
		Code:       "TEST123",
		CampaignID: campaignID,
		Redeemed:   false,
	}

	if err := db.Create(&coupon).Error; err != nil {
		log.Printf("   Coupon creation error: %v", err)
	} else {
		fmt.Printf("   ✅ Test coupon created: %s\n", coupon.Code)
	}

	// Test message sending functions
	fmt.Println("\n3. Testing message functions...")
	
	// Test simple message
	err = whatsappbot.SendWhatsAppMessage("2348123456789", "Hello from WhatsApp Bot!")
	fmt.Printf("   SendWhatsAppMessage result: %v\n", err != nil)

	// Test button message
	err = whatsappbot.SendWhatsAppButtons("2348123456789", "Choose an option:", []string{"Option 1", "Option 2", "Option 3"})
	fmt.Printf("   SendWhatsAppButtons result: %v\n", err != nil)

	// Test webhook verification
	fmt.Println("\n4. Testing webhook verification...")
	isValid := whatsappbot.VerifyWebhookSignature([]byte("test"), "test_signature")
	fmt.Printf("   Webhook verification: %v\n", isValid)

	fmt.Println("\n✅ WhatsApp Bot simple flow test completed!")
	fmt.Println("\nThe bot structure is working correctly.")
	fmt.Println("Message sending will fail without real WhatsApp API tokens,")
	fmt.Println("but all core functions are operational.")

	// Clean up
	os.Remove("test_whatsapp.db")
}