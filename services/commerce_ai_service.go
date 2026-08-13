package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hidenkeys/zidibackend/repository"
)

type CommerceAIProvider interface {
	InterpretOrder(context.Context, CommerceAIOrderInterpretationInput) (*CommerceAIOrderInterpretation, error)
}

type CommerceAIOrderInterpretationInput struct {
	Message   string
	Catalogue []repository.CommerceStoreCatalogueEntry
}

type CommerceAIOrderInterpretation struct {
	Intent        string                      `json:"intent"`
	Items         []CommerceAIInterpretedItem `json:"items"`
	Missing       []string                    `json:"missing_fields"`
	Confidence    float64                     `json:"confidence"`
	Clarification string                      `json:"clarification"`
}

type CommerceAIInterpretedItem struct {
	ProductQuery string `json:"product_query"`
	VariantQuery string `json:"variant_query"`
	Quantity     int    `json:"quantity"`
}

type GroqCommerceAIProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewConfiguredCommerceAIProvider(enabled, provider, groqAPIKey, baseURL, model string) CommerceAIProvider {
	if strings.ToLower(strings.TrimSpace(enabled)) != "true" {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(provider)) != "groq" {
		return nil
	}
	if strings.TrimSpace(groqAPIKey) == "" {
		return nil
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.groq.com/openai/v1"
	}
	if strings.TrimSpace(model) == "" {
		model = "openai/gpt-oss-120b"
	}
	return &GroqCommerceAIProvider{
		apiKey: strings.TrimSpace(groqAPIKey), baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		model: strings.TrimSpace(model), httpClient: &http.Client{Timeout: 12 * time.Second},
	}
}

func (p *GroqCommerceAIProvider) InterpretOrder(ctx context.Context, input CommerceAIOrderInterpretationInput) (*CommerceAIOrderInterpretation, error) {
	if p == nil || p.apiKey == "" {
		return nil, errors.New("commerce AI provider is not configured")
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return nil, ErrCommerceValidation
	}
	catalogue := make([]map[string]interface{}, 0, minCommerceChannelInt(len(input.Catalogue), 60))
	for index, entry := range input.Catalogue {
		if index >= 60 {
			break
		}
		name := strings.TrimSpace(entry.ProductName)
		if strings.TrimSpace(entry.VariantName) != "" && !strings.EqualFold(entry.VariantName, "default") {
			name += " " + strings.TrimSpace(entry.VariantName)
		}
		catalogue = append(catalogue, map[string]interface{}{
			"product": entry.ProductName, "variant": entry.VariantName, "category": entry.CategoryName,
			"name": name, "price_minor": entry.EffectivePriceMinor, "available_quantity": entry.AvailableQuantity,
		})
	}
	body := map[string]interface{}{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "system", "content": commerceAIOrderSystemPrompt()},
			{"role": "user", "content": mustCommerceAIJSON(map[string]interface{}{"message": message, "catalogue": catalogue})},
		},
		"temperature": 0,
		"response_format": map[string]interface{}{
			"type": "json_schema",
			"json_schema": map[string]interface{}{
				"name": "commerce_order_interpretation",
				"schema": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"intent", "items", "missing_fields", "confidence", "clarification"},
					"properties": map[string]interface{}{
						"intent": map[string]interface{}{"type": "string", "enum": []string{"place_order", "track_order", "complaint", "faq", "unknown"}},
						"items": map[string]interface{}{"type": "array", "maxItems": 5, "items": map[string]interface{}{
							"type": "object", "additionalProperties": false,
							"required": []string{"product_query", "variant_query", "quantity"},
							"properties": map[string]interface{}{
								"product_query": map[string]interface{}{"type": "string"},
								"variant_query": map[string]interface{}{"type": "string"},
								"quantity":      map[string]interface{}{"type": "integer", "minimum": 0, "maximum": commerceCartMaxQuantity},
							},
						}},
						"missing_fields": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
						"confidence":     map[string]interface{}{"type": "number", "minimum": 0, "maximum": 1},
						"clarification":  map[string]interface{}{"type": "string"},
					},
				},
			},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := p.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("commerce AI request failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("commerce AI request returned %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return nil, fmt.Errorf("decode commerce AI response: %w", err)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return nil, errors.New("commerce AI returned no interpretation")
	}
	var interpretation CommerceAIOrderInterpretation
	if err := json.Unmarshal([]byte(decoded.Choices[0].Message.Content), &interpretation); err != nil {
		return nil, fmt.Errorf("decode commerce AI interpretation: %w", err)
	}
	return &interpretation, nil
}

func commerceAIOrderSystemPrompt() string {
	return strings.Join([]string{
		"You interpret short WhatsApp commerce messages for Zidi.",
		"Return only structured JSON matching the schema.",
		"Use the supplied catalogue as the only source of sellable items.",
		"Do not invent products, prices, payment status, or delivery promises.",
		"If the user appears to be ordering, intent is place_order.",
		"Extract quantity when present. If no quantity is stated, use 0.",
		"Use product_query and variant_query as natural text from the customer, not IDs.",
		"If item or quantity is unclear, include the missing field and set confidence lower.",
	}, "\n")
}

func mustCommerceAIJSON(value interface{}) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
