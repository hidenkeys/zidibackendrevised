package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	PaystackProviderName = "paystack"
	paystackDefaultURL   = "https://api.paystack.co"
	paystackMaxBodyBytes = 1 << 20
)

type PaystackProvider struct {
	secretKey string
	baseURL   string
	client    *http.Client
}

func NewPaystackProvider(secretKey, baseURL string, client *http.Client) *PaystackProvider {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = paystackDefaultURL
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &PaystackProvider{secretKey: strings.TrimSpace(secretKey), baseURL: baseURL, client: client}
}

func (p *PaystackProvider) Name() string { return PaystackProviderName }

func (p *PaystackProvider) SignatureHeader() string { return "x-paystack-signature" }

func (p *PaystackProvider) Initialize(ctx context.Context, request InitializeRequest) (*Initialization, error) {
	if p.secretKey == "" {
		return nil, ErrProviderNotConfigured
	}
	metadata, err := json.Marshal(request.Metadata)
	if err != nil {
		return nil, fmt.Errorf("encode paystack metadata: %w", err)
	}
	payload := struct {
		Email       string `json:"email"`
		Amount      string `json:"amount"`
		Currency    string `json:"currency"`
		Reference   string `json:"reference"`
		CallbackURL string `json:"callback_url,omitempty"`
		Metadata    string `json:"metadata,omitempty"`
	}{
		Email: request.Email, Amount: strconv.FormatInt(request.AmountMinor, 10),
		Currency: request.Currency, Reference: request.Reference,
		CallbackURL: request.CallbackURL, Metadata: string(metadata),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode paystack initialization: %w", err)
	}
	responseBody, status, err := p.do(ctx, http.MethodPost, "/transaction/initialize", body)
	if err != nil {
		return nil, err
	}
	var response struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			AuthorizationURL string `json:"authorization_url"`
			AccessCode       string `json:"access_code"`
			Reference        string `json:"reference"`
		} `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, fmt.Errorf("decode paystack initialization: %w", err)
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices || !response.Status {
		return nil, fmt.Errorf("%w: %s", ErrProviderRejected, strings.TrimSpace(response.Message))
	}
	if response.Data.Reference != request.Reference || strings.TrimSpace(response.Data.AuthorizationURL) == "" {
		return nil, fmt.Errorf("%w: paystack returned inconsistent initialization data", ErrProviderRejected)
	}
	sanitized, _ := json.Marshal(map[string]string{
		"reference":         response.Data.Reference,
		"authorization_url": response.Data.AuthorizationURL,
		"access_code":       response.Data.AccessCode,
	})
	return &Initialization{
		Reference: response.Data.Reference, AuthorizationURL: response.Data.AuthorizationURL,
		AccessCode: response.Data.AccessCode, ProviderResponse: sanitized,
	}, nil
}

func (p *PaystackProvider) Verify(ctx context.Context, reference string) (*Verification, error) {
	if p.secretKey == "" {
		return nil, ErrProviderNotConfigured
	}
	responseBody, status, err := p.do(ctx, http.MethodGet, "/transaction/verify/"+url.PathEscape(reference), nil)
	if err != nil {
		return nil, err
	}
	var response struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			ID              json.Number `json:"id"`
			Status          string      `json:"status"`
			Reference       string      `json:"reference"`
			Amount          int64       `json:"amount"`
			Currency        string      `json:"currency"`
			GatewayResponse string      `json:"gateway_response"`
			PaidAt          *time.Time  `json:"paid_at"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("decode paystack verification: %w", err)
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices || !response.Status {
		return nil, fmt.Errorf("%w: %s", ErrProviderRejected, strings.TrimSpace(response.Message))
	}
	transactionID := response.Data.ID.String()
	if transactionID == "" {
		transactionID = "0"
	}
	sanitized, _ := json.Marshal(map[string]interface{}{
		"id": transactionID, "status": response.Data.Status, "reference": response.Data.Reference,
		"amount": response.Data.Amount, "currency": response.Data.Currency,
		"gateway_response": response.Data.GatewayResponse, "paid_at": response.Data.PaidAt,
	})
	return &Verification{
		Reference: response.Data.Reference, ProviderTransactionID: transactionID,
		Status: strings.ToLower(response.Data.Status), AmountMinor: response.Data.Amount,
		Currency: strings.ToUpper(response.Data.Currency), PaidAt: response.Data.PaidAt,
		ProviderResponse: sanitized,
	}, nil
}

func (p *PaystackProvider) VerifyWebhook(body []byte, signature string) bool {
	if p.secretKey == "" || strings.TrimSpace(signature) == "" {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return false
	}
	mac := hmac.New(sha512.New, []byte(p.secretKey))
	_, _ = mac.Write(body)
	return hmac.Equal(mac.Sum(nil), provided)
}

func (p *PaystackProvider) ParseWebhook(body []byte) (*WebhookEvent, error) {
	var payload struct {
		Event string `json:"event"`
		Data  struct {
			ID        json.Number `json:"id"`
			Reference string      `json:"reference"`
			Status    string      `json:"status"`
			Amount    int64       `json:"amount"`
			Currency  string      `json:"currency"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON", ErrInvalidWebhook)
	}
	eventType := strings.TrimSpace(payload.Event)
	reference := strings.TrimSpace(payload.Data.Reference)
	if eventType == "" || reference == "" || !json.Valid(body) {
		return nil, fmt.Errorf("%w: event type and reference are required", ErrInvalidWebhook)
	}
	transactionID := payload.Data.ID.String()
	key := eventType + ":" + transactionID
	if transactionID == "" {
		digest := sha256.Sum256(body)
		key = eventType + ":" + hex.EncodeToString(digest[:])
	}
	sanitized, _ := json.Marshal(map[string]interface{}{
		"event": eventType,
		"data": map[string]interface{}{
			"id": transactionID, "reference": reference, "status": payload.Data.Status,
			"amount": payload.Data.Amount, "currency": payload.Data.Currency,
		},
	})
	return &WebhookEvent{
		Key: key, Type: eventType, Reference: reference,
		ProviderTransactionID: transactionID, Payload: sanitized,
	}, nil
}

func (p *PaystackProvider) do(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("create paystack request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+p.secretKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("call paystack: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, paystackMaxBodyBytes))
	if err != nil {
		return nil, response.StatusCode, fmt.Errorf("read paystack response: %w", err)
	}
	return responseBody, response.StatusCode, nil
}
