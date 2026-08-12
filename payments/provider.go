package payments

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

var (
	ErrProviderNotConfigured = errors.New("payment provider is not configured")
	ErrProviderRejected      = errors.New("payment provider rejected the request")
	ErrInvalidWebhook        = errors.New("invalid payment webhook")
)

type InitializeRequest struct {
	Reference   string
	Email       string
	AmountMinor int64
	Currency    string
	CallbackURL string
	Metadata    map[string]string
}

type Initialization struct {
	Reference        string
	AuthorizationURL string
	AccessCode       string
	ProviderResponse []byte
}

type WebhookEvent struct {
	Key                   string
	Type                  string
	Reference             string
	ProviderTransactionID string
	Payload               []byte
}

type Verification struct {
	Reference             string
	ProviderTransactionID string
	Status                string
	AmountMinor           int64
	Currency              string
	PaidAt                *time.Time
	ProviderResponse      []byte
}

type Provider interface {
	Name() string
	SignatureHeader() string
	Initialize(ctx context.Context, request InitializeRequest) (*Initialization, error)
	Verify(ctx context.Context, reference string) (*Verification, error)
	VerifyWebhook(body []byte, signature string) bool
	ParseWebhook(body []byte) (*WebhookEvent, error)
}

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	registry := &Registry{providers: make(map[string]Provider, len(providers))}
	for _, provider := range providers {
		registry.Register(provider)
	}
	return registry
}

func (r *Registry) Register(provider Provider) {
	if provider == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[strings.ToLower(strings.TrimSpace(provider.Name()))] = provider
}

func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[strings.ToLower(strings.TrimSpace(name))]
	return provider, ok
}
