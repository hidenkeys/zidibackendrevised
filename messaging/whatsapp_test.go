package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestMetaWhatsAppClientSendsStableButtonIDs(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer token" || request.URL.Path != "/v23.0/phone-id/messages" {
			t.Fatalf("unexpected request: path=%s auth=%s", request.URL.Path, request.Header.Get("Authorization"))
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		encoded, _ := json.Marshal(payload)
		if !strings.Contains(string(encoded), "intent:order") {
			t.Fatalf("button ID was not preserved: %s", encoded)
		}
		interactive, ok := payload["interactive"].(map[string]interface{})
		if !ok {
			t.Fatalf("interactive payload is missing: %s", encoded)
		}
		body, ok := interactive["body"].(map[string]interface{})
		if !ok || body["text"] != "Choose" || body["body"] != nil {
			t.Fatalf("interactive body does not match Meta's schema: %s", encoded)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(`{"messages":[{"id":"wamid.123"}]}`))}, nil
	})}

	client := NewMetaWhatsAppClient("token", "v23.0", "https://graph.test", httpClient)
	messageID, err := client.Send(context.Background(), WhatsAppOutboundMessage{
		PhoneNumberID: "phone-id", To: "2348000000000", Body: "Choose", Buttons: []WhatsAppButton{{ID: "intent:order", Title: "Order"}},
	})
	if err != nil || messageID != "wamid.123" {
		t.Fatalf("send message: id=%s err=%v", messageID, err)
	}
}

func TestMetaWhatsAppClientReturnsProviderErrorWithoutTokenData(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(`{"error":{"message":"invalid recipient"}}`))}, nil
	})}
	client := NewMetaWhatsAppClient("secret-token", "v23.0", "https://graph.test", httpClient)
	_, err := client.Send(context.Background(), WhatsAppOutboundMessage{PhoneNumberID: "phone", To: "bad", Body: "Hello"})
	if err == nil || strings.Contains(err.Error(), "secret-token") || !strings.Contains(err.Error(), "invalid recipient") {
		t.Fatalf("unexpected provider error: %v", err)
	}
}

func TestMetaWhatsAppClientSendsImageMessage(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload map[string]interface{}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode image request: %v", err)
		}
		image, ok := payload["image"].(map[string]interface{})
		if payload["type"] != "image" || !ok || image["link"] != "https://cdn.example.com/lemon-tea.jpg" {
			t.Fatalf("unexpected image payload: %#v", payload)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(`{"messages":[{"id":"wamid.image"}]}`))}, nil
	})}
	client := NewMetaWhatsAppClient("token", "v23.0", "https://graph.test", httpClient)
	messageID, err := client.Send(context.Background(), WhatsAppOutboundMessage{
		PhoneNumberID: "phone-id", To: "2348000000000", Body: "Lemon Tea", ImageURL: "https://cdn.example.com/lemon-tea.jpg",
	})
	if err != nil || messageID != "wamid.image" {
		t.Fatalf("send image: id=%s err=%v", messageID, err)
	}
}
