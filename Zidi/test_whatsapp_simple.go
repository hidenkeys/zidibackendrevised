//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/hidenkeys/zidibackend/whatsappbot"
)
 
func main() {
	// Set test environment variables
	os.Setenv("WHATSAPP_ACCESS_TOKEN", "test_token")
	os.Setenv("WHATSAPP_PHONE_NUMBER_ID", "test_phone_id")
	os.Setenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN", "test_verify_token")

	fmt.Println("🧪 Testing WhatsApp Bot Functions")
	fmt.Println("=================================")

	// Test 1: Webhook signature verification
	fmt.Println("\n1. Testing webhook signature verification...")
	testPayload := []byte(`{"test": "data"}`)
	isValid := whatsappbot.VerifyWebhookSignature(testPayload, "test_signature")
	fmt.Printf("   Signature verification result: %v\n", isValid)

	// Test 2: Test webhook payload parsing
	fmt.Println("\n2. Testing webhook payload parsing...")
	testWebhookJSON := `{
		"object": "whatsapp_business_account",
		"entry": [{
			"id": "test_id",
			"changes": [{
				"value": {
					"messaging_product": "whatsapp",
					"metadata": {
						"display_phone_number": "15550559999",
						"phone_number_id": "test_phone_id"
					},
					"contacts": [{
						"profile": {"name": "Test User"},
						"wa_id": "2348123456789"
					}],
					"messages": [{
						"from": "2348123456789",
						"id": "test_msg_id",
						"timestamp": "1234567890",
						"text": {"body": "Hello"},
						"type": "text"
					}]
				},
				"field": "messages"
			}]
		}]
	}`

	var payload whatsappbot.WebhookPayload
	err := json.Unmarshal([]byte(testWebhookJSON), &payload)
	if err != nil {
		log.Printf("   ❌ Error parsing webhook payload: %v", err)
	} else {
		fmt.Printf("   ✅ Webhook payload parsed successfully\n")
		fmt.Printf("   Object: %s\n", payload.Object)
		if len(payload.Entry) > 0 && len(payload.Entry[0].Changes) > 0 {
			fmt.Printf("   Messages count: %d\n", len(payload.Entry[0].Changes[0].Value.Messages))
		}
	}

	// Test 3: Message sending (will fail without real token but tests structure)
	fmt.Println("\n3. Testing message sending structure...")
	err = whatsappbot.SendWhatsAppMessage("2348123456789", "Test message")
	if err != nil {
		fmt.Printf("   Expected error (no real token): %v\n", err)
	} else {
		fmt.Printf("   ✅ Message sending function executed\n")
	}

	fmt.Println("\n✅ WhatsApp Bot basic functionality test completed!")
}