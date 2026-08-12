package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPaystackInitializeUsesServerReferenceAndMinorUnits(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/transaction/initialize" || request.Method != http.MethodPost {
			t.Fatalf("unexpected Paystack request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-secret" {
			t.Fatal("missing Paystack authorization header")
		}
		var payload struct {
			Amount    string `json:"amount"`
			Reference string `json:"reference"`
			Metadata  string `json:"metadata"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Amount != "420000" || payload.Reference != "ZCP-payment-001" {
			t.Fatalf("provider received altered financial identity: %+v", payload)
		}
		var metadata map[string]string
		if err := json.Unmarshal([]byte(payload.Metadata), &metadata); err != nil || metadata["order_id"] != "order-001" {
			t.Fatalf("metadata was not stringified JSON: value=%q err=%v", payload.Metadata, err)
		}
		return jsonHTTPResponse(http.StatusOK, `{"status":true,"message":"Authorization URL created","data":{"authorization_url":"https://checkout.paystack.com/access","access_code":"access","reference":"ZCP-payment-001"}}`), nil
	})}

	provider := NewPaystackProvider("test-secret", "https://api.paystack.test", client)
	result, err := provider.Initialize(context.Background(), InitializeRequest{
		Reference: "ZCP-payment-001", Email: "customer@example.com", AmountMinor: 420000,
		Currency: "NGN", Metadata: map[string]string{"order_id": "order-001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Reference != "ZCP-payment-001" || result.AuthorizationURL != "https://checkout.paystack.com/access" {
		t.Fatalf("unexpected initialization result: %+v", result)
	}
}

func TestPaystackVerifyPreservesLargeTransactionIDAndAuthoritativeValues(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/transaction/verify/ZCP-payment-002" {
			t.Fatalf("unexpected verification path: %s", request.URL.Path)
		}
		return jsonHTTPResponse(http.StatusOK, `{"status":true,"message":"Verification successful","data":{"id":18446744073709551615,"status":"success","reference":"ZCP-payment-002","amount":840000,"currency":"NGN","gateway_response":"Successful","paid_at":"2026-08-11T09:30:00Z"}}`), nil
	})}

	provider := NewPaystackProvider("test-secret", "https://api.paystack.test", client)
	result, err := provider.Verify(context.Background(), "ZCP-payment-002")
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderTransactionID != "18446744073709551615" || result.AmountMinor != 840000 || result.Currency != "NGN" || result.Status != "success" {
		t.Fatalf("verification lost authoritative provider values: %+v", result)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func jsonHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestPaystackWebhookSignatureAndParsing(t *testing.T) {
	body := []byte(`{"event":"charge.success","data":{"id":123456789,"reference":"ZCP-payment-003","status":"success","amount":1,"currency":"NGN","customer":{"email":"private@example.com"}}}`)
	mac := hmac.New(sha512.New, []byte("test-secret"))
	_, _ = mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))
	provider := NewPaystackProvider("test-secret", "", nil)

	if !provider.VerifyWebhook(body, signature) {
		t.Fatal("valid webhook signature was rejected")
	}
	if provider.VerifyWebhook([]byte(strings.ReplaceAll(string(body), `"amount":1`, `"amount":2`)), signature) {
		t.Fatal("tampered webhook payload was accepted")
	}
	event, err := provider.ParseWebhook(body)
	if err != nil {
		t.Fatal(err)
	}
	if event.Key != "charge.success:123456789" || event.Reference != "ZCP-payment-003" {
		t.Fatalf("unexpected parsed webhook: %+v", event)
	}
	if strings.Contains(string(event.Payload), "private@example.com") {
		t.Fatal("webhook audit payload retained unnecessary customer data")
	}
}
