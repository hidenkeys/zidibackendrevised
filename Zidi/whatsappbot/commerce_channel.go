package whatsappbot

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hidenkeys/zidibackend/services"
	"github.com/hidenkeys/zidibackend/utils"
	"gorm.io/gorm"
)

func HandleWebhookWithCommerce(payload utils.WebhookPayload, db *gorm.DB, channel *services.CommerceChannelService) error {
	return HandleWebhookWithCommerceContext(context.Background(), payload, db, channel)
}

func HandleWebhookWithCommerceContext(ctx context.Context, payload utils.WebhookPayload, db *gorm.DB, channel *services.CommerceChannelService) error {
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			for _, message := range change.Value.Messages {
				if channel != nil {
					input := services.CommerceChannelInbound{
						ProviderAccountID: change.Value.Metadata.PhoneNumberID,
						ExternalMessageID: message.ID, SenderID: message.From,
						SenderName: commerceContactName(change.Value.Contacts, message.From), MessageType: message.Type,
						ProviderTimestamp: commerceMessageTimestamp(message.Timestamp),
					}
					if message.Text != nil {
						input.Text = message.Text.Body
					}
					if message.Interactive != nil && message.Interactive.ButtonReply != nil {
						input.Text = message.Interactive.ButtonReply.Title
						input.SelectionID = message.Interactive.ButtonReply.ID
					}
					if message.Interactive != nil && message.Interactive.ListReply != nil {
						input.Text = message.Interactive.ListReply.Title
						input.SelectionID = message.Interactive.ListReply.ID
					}
					if message.Location != nil {
						input.Latitude = &message.Location.Latitude
						input.Longitude = &message.Location.Longitude
					}
					raw, _ := json.Marshal(message)
					input.Payload = raw
					result, err := channel.HandleInbound(ctx, input)
					if err != nil {
						return err
					}
					if result.Handled {
						continue
					}
				}

				if err := handleMessage(message.From, message, db); err != nil {
					log.Printf("legacy WhatsApp message failed for %s: %v", message.From, err)
					if sendErr := utils.SendWhatsAppMessage(message.From, "Sorry, something went wrong. Please try again."); sendErr != nil {
						log.Printf("legacy WhatsApp error reply failed for %s: %v", message.From, sendErr)
					}
				}
			}
		}
	}
	return nil
}

func commerceMessageTimestamp(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return nil
	}
	timestamp := time.Unix(seconds, 0).UTC()
	return &timestamp
}

func commerceContactName(contacts []struct {
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
	WaID string `json:"wa_id"`
}, senderID string) string {
	for _, contact := range contacts {
		if contact.WaID == senderID {
			return contact.Profile.Name
		}
	}
	return ""
}
