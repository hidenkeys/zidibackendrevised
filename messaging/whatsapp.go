package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type WhatsAppButton struct {
	ID    string
	Title string
}

type WhatsAppOutboundMessage struct {
	PhoneNumberID string
	To            string
	Body          string
	Buttons       []WhatsAppButton
	ImageURL      string
}

type WhatsAppSender interface {
	Send(context.Context, WhatsAppOutboundMessage) (string, error)
}

type MetaWhatsAppClient struct {
	accessToken  string
	graphVersion string
	baseURL      string
	httpClient   *http.Client
}

func NewMetaWhatsAppClient(accessToken, graphVersion, baseURL string, httpClient *http.Client) *MetaWhatsAppClient {
	if strings.TrimSpace(graphVersion) == "" {
		graphVersion = "v23.0"
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://graph.facebook.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 4 * time.Second}
	}
	return &MetaWhatsAppClient{
		accessToken: strings.TrimSpace(accessToken), graphVersion: strings.Trim(strings.TrimSpace(graphVersion), "/"),
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), httpClient: httpClient,
	}
}

func (c *MetaWhatsAppClient) Send(ctx context.Context, message WhatsAppOutboundMessage) (string, error) {
	if c.accessToken == "" {
		return "", errors.New("WHATSAPP_ACCESS_TOKEN is not configured")
	}
	if strings.TrimSpace(message.PhoneNumberID) == "" || strings.TrimSpace(message.To) == "" || strings.TrimSpace(message.Body) == "" {
		return "", errors.New("WhatsApp phone number, recipient, and body are required")
	}
	if len(message.Buttons) > 3 {
		return "", errors.New("WhatsApp supports at most three reply buttons")
	}

	payload := metaMessagePayload{MessagingProduct: "whatsapp", To: strings.TrimSpace(message.To), Type: "text"}
	payload.Text = &metaText{Body: strings.TrimSpace(message.Body)}
	if strings.TrimSpace(message.ImageURL) != "" {
		parsed, err := url.ParseRequestURI(strings.TrimSpace(message.ImageURL))
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return "", errors.New("WhatsApp image URL must be a public HTTPS URL")
		}
		payload.Type = "image"
		payload.Text = nil
		payload.Image = &metaImage{Link: parsed.String(), Caption: strings.TrimSpace(message.Body)}
	} else if len(message.Buttons) > 0 {
		payload.Type = "interactive"
		payload.Text = nil
		payload.Interactive = &metaInteractive{Type: "button", Body: metaInteractiveBody{Text: strings.TrimSpace(message.Body)}}
		for _, button := range message.Buttons {
			id := strings.TrimSpace(button.ID)
			title := strings.TrimSpace(button.Title)
			if id == "" || len(id) > 256 || title == "" || len(title) > 20 {
				return "", errors.New("WhatsApp reply button identifiers or titles are invalid")
			}
			payload.Interactive.Action.Buttons = append(payload.Interactive.Action.Buttons, metaButton{Type: "reply", Reply: metaButtonReply{ID: id, Title: title}})
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("%s/%s/%s/messages", c.baseURL, c.graphVersion, url.PathEscape(strings.TrimSpace(message.PhoneNumberID)))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+c.accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("send WhatsApp message: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("read WhatsApp response: %w", err)
	}
	var result metaMessageResponse
	_ = json.Unmarshal(body, &result)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		providerMessage := strings.TrimSpace(result.Error.Message)
		if providerMessage == "" {
			providerMessage = http.StatusText(response.StatusCode)
		}
		return "", fmt.Errorf("WhatsApp API returned %d: %s", response.StatusCode, providerMessage)
	}
	if len(result.Messages) == 0 || strings.TrimSpace(result.Messages[0].ID) == "" {
		return "", errors.New("WhatsApp API returned no message identifier")
	}
	return result.Messages[0].ID, nil
}

type metaMessagePayload struct {
	MessagingProduct string           `json:"messaging_product"`
	To               string           `json:"to"`
	Type             string           `json:"type"`
	Text             *metaText        `json:"text,omitempty"`
	Interactive      *metaInteractive `json:"interactive,omitempty"`
	Image            *metaImage       `json:"image,omitempty"`
}

type metaImage struct {
	Link    string `json:"link"`
	Caption string `json:"caption,omitempty"`
}

type metaText struct {
	Body string `json:"body"`
}

type metaInteractive struct {
	Type   string              `json:"type"`
	Body   metaInteractiveBody `json:"body"`
	Action struct {
		Buttons []metaButton `json:"buttons"`
	} `json:"action"`
}

type metaInteractiveBody struct {
	Text string `json:"text"`
}

type metaButton struct {
	Type  string          `json:"type"`
	Reply metaButtonReply `json:"reply"`
}

type metaButtonReply struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type metaMessageResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}
