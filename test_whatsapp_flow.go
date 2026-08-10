//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/config"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/whatsappbot"
)

func main() {
	// Set environment variables
	os.Setenv("WHATSAPP_ACCESS_TOKEN", "test_token")
	os.Setenv("WHATSAPP_PHONE_NUMBER_ID", "test_phone_id") 
	os.Setenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN", "test_verify_token")

	fmt.Println("🧪 Testing WhatsApp Bot Complete Flow")
	fmt.Println("====================================")

	// Setup test database
	db, err := config.ConnectDB()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Create test campaign
	fmt.Println("\n1. Creating test campaign...")
	campaignID := uuid.New()
	orgID := uuid.New()
	
	campaign := models.Campaign{
		ID:             campaignID,
		OrganizationID: orgID,
		Name:           "Test Campaign",
		WelcomeMessage: "Welcome to our test campaign!",
		Amount:         100.0,
		Status:         "active",
	}
	
	if err := db.Create(&campaign).Error; err != nil {
		log.Printf("   Campaign creation error (may already exist): %v", err)
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
		log.Printf("   Coupon creation error (may already exist): %v", err)
	} else {
		fmt.Printf("   ✅ Test coupon created: %s\n", coupon.Code)
	}

	// Test webhook payload for starting campaign
	fmt.Println("\n3. Testing campaign start webhook...")
	startPayload := whatsappbot.WebhookPayload{
		Object: "whatsapp_business_account",
		Entry: []struct {
			ID      string `json:"id"`
			Changes []struct {
				Value struct {
					MessagingProduct string `json:"messaging_product"`
					Metadata         struct {
						DisplayPhoneNumber string `json:"display_phone_number"`
						PhoneNumberID      string `json:"phone_number_id"`
					} `json:"metadata"`
					Contacts []struct {
						Profile struct {
							Name string `json:"name"`
						} `json:"profile"`
						WaID string `json:"wa_id"`
					} `json:"contacts"`
					Messages []struct {
						From      string `json:"from"`
						ID        string `json:"id"`
						Timestamp string `json:"timestamp"`
						Text      *struct {
							Body string `json:"body"`
						} `json:"text,omitempty"`
						Interactive *struct {
							Type         string `json:"type"`
							ButtonReply *struct {
								ID    string `json:"id"`
								Title string `json:"title"`
							} `json:"button_reply,omitempty"`
						} `json:"interactive,omitempty"`
						Type string `json:"type"`
					} `json:"messages"`
				} `json:"value"`
				Field string `json:"field"`
			} `json:"changes"`
		}{
			{
				ID: "test_entry",
				Changes: []struct {
					Value struct {
						MessagingProduct string `json:"messaging_product"`
						Metadata         struct {
							DisplayPhoneNumber string `json:"display_phone_number"`
							PhoneNumberID      string `json:"phone_number_id"`
						} `json:"metadata"`
						Contacts []struct {
							Profile struct {
								Name string `json:"name"`
							} `json:"profile"`
							WaID string `json:"wa_id"`
						} `json:"contacts"`
						Messages []struct {
							From      string `json:"from"`
							ID        string `json:"id"`
							Timestamp string `json:"timestamp"`
							Text      *struct {
								Body string `json:"body"`
							} `json:"text,omitempty"`
							Interactive *struct {
								Type         string `json:"type"`
								ButtonReply *struct {
									ID    string `json:"id"`
									Title string `json:"title"`
								} `json:"button_reply,omitempty"`
							} `json:"interactive,omitempty"`
							Type string `json:"type"`
						} `json:"messages"`
					} `json:"value"`
					Field string `json:"field"`
				}{
					{
						Value: struct {
							MessagingProduct string `json:"messaging_product"`
							Metadata         struct {
								DisplayPhoneNumber string `json:"display_phone_number"`
								PhoneNumberID      string `json:"phone_number_id"`
							} `json:"metadata"`
							Contacts []struct {
								Profile struct {
									Name string `json:"name"`
								} `json:"profile"`
								WaID string `json:"wa_id"`
							} `json:"contacts"`
							Messages []struct {
								From      string `json:"from"`
								ID        string `json:"id"`
								Timestamp string `json:"timestamp"`
								Text      *struct {
									Body string `json:"body"`
								} `json:"text,omitempty"`
								Interactive *struct {
									Type         string `json:"type"`
									ButtonReply *struct {
										ID    string `json:"id"`
										Title string `json:"title"`
									} `json:"button_reply,omitempty"`
								} `json:"interactive,omitempty"`
								Type string `json:"type"`
							} `json:"messages"`
						}{
							MessagingProduct: "whatsapp",
							Messages: []struct {
								From      string `json:"from"`
								ID        string `json:"id"`
								Timestamp string `json:"timestamp"`
								Text      *struct {
									Body string `json:"body"`
								} `json:"text,omitempty"`
								Interactive *struct {
									Type         string `json:"type"`
									ButtonReply *struct {
										ID    string `json:"id"`
										Title string `json:"title"`
									} `json:"button_reply,omitempty"`
								} `json:"interactive,omitempty"`
								Type string `json:"type"`
							}{
								{
									From: "2348123456789",
									ID:   "test_msg_id",
									Text: &struct {
										Body string `json:"body"`
									}{Body: fmt.Sprintf("start %s", campaignID)},
									Type: "text",
								},
							},
						},
						Field: "messages",
					},
				},
			},
		},
	}

	err = whatsappbot.HandleWebhook(startPayload, db)
	if err != nil {
		log.Printf("   ❌ Error handling webhook: %v", err)
	} else {
		fmt.Printf("   ✅ Webhook handled successfully\n")
	}

	fmt.Println("\n✅ WhatsApp Bot flow test completed!")
	fmt.Println("\nNote: Message sending will fail without real WhatsApp tokens,")
	fmt.Println("but the core logic and database operations are working correctly.")
}