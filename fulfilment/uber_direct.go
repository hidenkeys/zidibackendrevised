package fulfilment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type UberDirectProvider struct {
	clientID     string
	clientSecret string
	customerID   string
	tokenURL     string
	apiBaseURL   string
	httpClient   *http.Client
}

func NewUberDirectProvider(clientID, clientSecret, customerID, tokenURL, apiBaseURL string, httpClient *http.Client) *UberDirectProvider {
	if strings.TrimSpace(tokenURL) == "" {
		tokenURL = "https://login.uber.com/oauth/v2/token"
	}
	if strings.TrimSpace(apiBaseURL) == "" {
		apiBaseURL = "https://api.uber.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &UberDirectProvider{
		clientID: strings.TrimSpace(clientID), clientSecret: strings.TrimSpace(clientSecret), customerID: strings.TrimSpace(customerID),
		tokenURL: strings.TrimSpace(tokenURL), apiBaseURL: strings.TrimRight(strings.TrimSpace(apiBaseURL), "/"), httpClient: httpClient,
	}
}

func (*UberDirectProvider) Name() string { return "uber_direct" }

func (p *UberDirectProvider) Quote(ctx context.Context, request DeliveryQuoteRequest) (*DeliveryQuote, error) {
	if p == nil || p.clientID == "" || p.clientSecret == "" || p.customerID == "" {
		return nil, errors.New("Uber Direct credentials are not configured")
	}
	if strings.TrimSpace(request.Pickup.Address) == "" || strings.TrimSpace(request.Destination.Address) == "" {
		return nil, errors.New("pickup and destination addresses are required")
	}
	token, err := p.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	pickup, _ := json.Marshal(map[string]interface{}{"street_address": []string{request.Pickup.Address}, "country": "NG"})
	dropoff, _ := json.Marshal(map[string]interface{}{"street_address": []string{request.Destination.Address}, "country": "NG"})
	payload, err := json.Marshal(map[string]interface{}{
		"pickup_address": string(pickup), "dropoff_address": string(dropoff), "external_id": request.Reference,
	})
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/v1/customers/%s/delivery_quotes", p.apiBaseURL, url.PathEscape(p.customerID))
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := p.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("request Uber Direct quote: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 256*1024))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Uber Direct quote returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		ID         string `json:"id"`
		Fee        int64  `json:"fee"`
		Currency   string `json:"currency"`
		Duration   int    `json:"duration"`
		Expires    string `json:"expires"`
		DropoffETA string `json:"dropoff_eta"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode Uber Direct quote: %w", err)
	}
	if result.ID == "" || result.Fee < 0 || len(result.Currency) != 3 {
		return nil, errors.New("Uber Direct returned an incomplete quote")
	}
	var expiresAt *time.Time
	if value, parseErr := time.Parse(time.RFC3339, result.Expires); parseErr == nil {
		expiresAt = &value
	}
	var duration *int
	if result.Duration > 0 {
		duration = &result.Duration
	} else if eta, parseErr := time.Parse(time.RFC3339, result.DropoffETA); parseErr == nil {
		seconds := int(time.Until(eta).Seconds())
		if seconds > 0 {
			duration = &seconds
		}
	}
	return &DeliveryQuote{
		ProviderQuoteID: result.ID, EstimatedFeeMinor: result.Fee, Currency: strings.ToUpper(result.Currency),
		DurationSeconds: duration, ExpiresAt: expiresAt, RawResponse: body,
	}, nil
}

func (p *UberDirectProvider) accessToken(ctx context.Context) (string, error) {
	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("grant_type", "client_credentials")
	form.Set("scope", "eats.deliveries")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := p.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("authenticate Uber Direct: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("Uber Direct authentication returned %s: %s", strconv.Itoa(response.StatusCode), strings.TrimSpace(string(body)))
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil || strings.TrimSpace(result.AccessToken) == "" {
		return "", errors.New("Uber Direct authentication returned no access token")
	}
	return strings.TrimSpace(result.AccessToken), nil
}
