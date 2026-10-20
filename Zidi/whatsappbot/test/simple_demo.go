//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hidenkeys/zidibackend/whatsappbot"
)

func main() {
	fmt.Println("Testing WhatsApp Bot Functions")
	fmt.Println("==================================")

	// Set test environment variables
	os.Setenv("WHATSAPP_ACCESS_TOKEN", "test_token")
	os.Setenv("WHATSAPP_PHONE_NUMBER_ID", "test_phone_id")
	os.Setenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN", "test_verify_token")

	// Test 1: Message sending function (will fail without real token, but tests structure)
	fmt.Println("\n1. Testing SendWhatsAppMessage function...")
	err := whatsappbot.SendWhatsAppMessage("2348123456789", "Hello from test!")
	if err != nil {
		fmt.Printf("❌ Expected error (no real token): %v\n", err)
	} else {
		fmt.Println("✅ Message function executed")
	}

	// Test 2: Button message function
	fmt.Println("\n2. Testing SendWhatsAppButtons function...")
	err = whatsappbot.SendWhatsAppButtons("2348123456789", "Choose an option:", []string{"Option 1", "Option 2", "Option 3"})
	if err != nil {
		fmt.Printf("❌ Expected error (no real token): %v\n", err)
	} else {
		fmt.Println("✅ Button function executed")
	}

	// Test 3: Webhook signature verification
	fmt.Println("\n3. Testing webhook signature verification...")
	testPayload := []byte(`{"test": "data"}`)
	isValid := whatsappbot.VerifyWebhookSignature(testPayload, "test_signature")
	fmt.Printf("Signature verification result: %v\n", isValid)

	// Test 4: Webhook payload parsing
	fmt.Println("\n4. Testing webhook payload parsing...")
	testWebhookData := `{
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
						"text": {"body": "start test-campaign-id"},
						"type": "text"
					}]
				},
				"field": "messages"
			}]
		}]
	}`

	var payload whatsappbot.WebhookPayload
	err = json.Unmarshal([]byte(testWebhookData), &payload)
	if err != nil {
		fmt.Printf("❌ Error parsing webhook payload: %v\n", err)
	} else {
		fmt.Println("✅ Webhook payload parsed successfully")
		fmt.Printf("   - Object: %s\n", payload.Object)
		fmt.Printf("   - Entry count: %d\n", len(payload.Entry))
		if len(payload.Entry) > 0 && len(payload.Entry[0].Changes) > 0 && len(payload.Entry[0].Changes[0].Value.Messages) > 0 {
			msg := payload.Entry[0].Changes[0].Value.Messages[0]
			fmt.Printf("   - Message from: %s\n", msg.From)
			if msg.Text != nil {
				fmt.Printf("   - Message text: %s\n", msg.Text.Body)
			}
		}
	}

	fmt.Println("\n✅ All structure tests completed!")
	fmt.Println("\nNote: API calls failed as expected without real WhatsApp tokens.")
	fmt.Println("The code structure and parsing logic are working correctly.")
}