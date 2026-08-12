package repository

import (
	"testing"
	"time"

	"github.com/hidenkeys/zidibackend/models"
)

func TestCommerceExpectedDeliveryAt(t *testing.T) {
	updatedAt := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	explicitETA := updatedAt.Add(25 * time.Minute)

	if got := commerceExpectedDeliveryAt(&models.CommerceFulfilment{UpdatedAt: updatedAt, ExpectedDeliveryAt: &explicitETA}); !got.Equal(explicitETA) {
		t.Fatalf("expected explicit ETA %s, got %s", explicitETA, got)
	}
	if got := commerceExpectedDeliveryAt(&models.CommerceFulfilment{UpdatedAt: updatedAt}); !got.Equal(updatedAt.Add(40 * time.Minute)) {
		t.Fatalf("expected legacy 40-minute ETA, got %s", got)
	}
}
