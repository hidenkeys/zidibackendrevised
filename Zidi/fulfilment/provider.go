package fulfilment

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Location struct {
	Address   string
	Latitude  *float64
	Longitude *float64
}

type DeliveryQuoteRequest struct {
	Reference   string
	Pickup      Location
	Destination Location
	Currency    string
}

type DeliveryQuote struct {
	ProviderQuoteID   string
	EstimatedFeeMinor int64
	Currency          string
	DistanceMeters    *int
	DurationSeconds   *int
	ExpiresAt         *time.Time
	RawResponse       []byte
}

type DeliveryQuoteProvider interface {
	Name() string
	Quote(ctx context.Context, request DeliveryQuoteRequest) (*DeliveryQuote, error)
}

type Registry struct {
	mu        sync.RWMutex
	providers map[string]DeliveryQuoteProvider
}

func NewRegistry(providers ...DeliveryQuoteProvider) *Registry {
	registry := &Registry{providers: make(map[string]DeliveryQuoteProvider, len(providers))}
	for _, provider := range providers {
		if provider != nil {
			registry.providers[strings.ToLower(strings.TrimSpace(provider.Name()))] = provider
		}
	}
	return registry
}

func (r *Registry) Get(name string) (DeliveryQuoteProvider, error) {
	if r == nil {
		return nil, fmt.Errorf("delivery quote provider %q is not configured", name)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, fmt.Errorf("delivery quote provider %q is not configured", name)
	}
	return provider, nil
}
