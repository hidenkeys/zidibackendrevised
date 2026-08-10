package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// WhatsApp API structures
type WhatsAppMessage struct {
	MessagingProduct string `json:"messaging_product"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Text             *struct {
		Body string `json:"body"`
	} `json:"text,omitempty"`
	Interactive *struct {
		Type   string `json:"type"`
		Header *struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"header,omitempty"`
		Body struct {
			Text string `json:"text"`
		} `json:"body"`
		Action struct {
			Buttons []struct {
				Type  string `json:"type"`
				Reply struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				} `json:"reply"`
			} `json:"buttons"`
		} `json:"action"`
	} `json:"interactive,omitempty"`
}

type WebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
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
						Type        string `json:"type"`
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
	} `json:"entry"`
}

func SendWhatsAppMessage(to, message string) error {
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", os.Getenv("WHATSAPP_PHONE_NUMBER_ID"))

	msg := WhatsAppMessage{
		MessagingProduct: "whatsapp",
		To:               to,
		Type:             "text",
		Text: &struct {
			Body string `json:"body"`
		}{Body: message},
	}

	return sendWhatsAppRequest(url, msg)
}

func SendWhatsAppButtons(to, bodyText string, buttons []string) error {
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", os.Getenv("WHATSAPP_PHONE_NUMBER_ID"))

	msg := WhatsAppMessage{
		MessagingProduct: "whatsapp",
		To:               to,
		Type:             "interactive",
		Interactive: &struct {
			Type   string `json:"type"`
			Header *struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"header,omitempty"`
			Body struct {
				Text string `json:"text"`
			} `json:"body"`
			Action struct {
				Buttons []struct {
					Type  string `json:"type"`
					Reply struct {
						ID    string `json:"id"`
						Title string `json:"title"`
					} `json:"reply"`
				} `json:"buttons"`
			} `json:"action"`
		}{
			Type: "button",
			Body: struct {
				Text string `json:"text"`
			}{Text: bodyText},
			Action: struct {
				Buttons []struct {
					Type  string `json:"type"`
					Reply struct {
						ID    string `json:"id"`
						Title string `json:"title"`
					} `json:"reply"`
				} `json:"buttons"`
			}{},
		},
	}

	for i, btn := range buttons {
		if i >= 3 { // WhatsApp allows max 3 buttons
			break
		}

		// WhatsApp button title max length is 20 characters
		title := btn
		if len(title) > 20 {
			title = title[:17] + "..."
			log.Printf("⚠️ Button title truncated from '%s' to '%s'", btn, title)
		}

		msg.Interactive.Action.Buttons = append(msg.Interactive.Action.Buttons, struct {
			Type  string `json:"type"`
			Reply struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"reply"`
		}{
			Type: "reply",
			Reply: struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			}{
				ID:    strings.ToLower(btn),
				Title: title,
			},
		})
	}

	return sendWhatsAppRequest(url, msg)
}

func sendWhatsAppRequest(url string, payload interface{}) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("❌ Error marshaling WhatsApp payload: %v", err)
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("❌ Error creating WhatsApp request: %v", err)
		return err
	}

	req.Header.Set("Authorization", "Bearer "+os.Getenv("WHATSAPP_ACCESS_TOKEN"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Error sending WhatsApp request: %v", err)
		return err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		log.Printf("❌ WhatsApp API error: status=%d failed_to_read_body=%v", resp.StatusCode, readErr)
		return fmt.Errorf("whatsapp api error: status=%d failed_to_read_body=%v", resp.StatusCode, readErr)
	}

	trimmedBody := strings.TrimSpace(string(body))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("❌ WhatsApp API error: status=%d body=%s", resp.StatusCode, trimmedBody)
		return fmt.Errorf("whatsapp api error: status=%d body=%s", resp.StatusCode, trimmedBody)
	}

	log.Printf("✅ WhatsApp API send ok: status=%d body=%s", resp.StatusCode, trimmedBody)
	return nil
}
