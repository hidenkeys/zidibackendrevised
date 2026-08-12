package repository

import (
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

func TestBuildCommerceOrderItemSnapshotsAuthoritativeMinorUnitPrice(t *testing.T) {
	organizationID, orderID, variantID := uuid.New(), uuid.New(), uuid.New()
	entry := &commerceCheckoutCatalogueRow{
		ProductID: uuid.New(), ProductName: "Fruit Tea", ProductCurrency: "NGN",
		VariantID: variantID, VariantName: "Regular", SKU: "TEA-1",
		Attributes: []byte(`{"size":"regular"}`), EffectivePriceMinor: 420000,
	}
	item, err := buildCommerceOrderItem(organizationID, orderID, models.CommerceCartItem{VariantID: variantID, Quantity: 2}, entry)
	if err != nil {
		t.Fatal(err)
	}
	if item.OrganizationID != organizationID || item.OrderID != orderID || item.UnitPriceMinor != 420000 || item.LineTotalMinor != 840000 {
		t.Fatalf("order item did not preserve the authoritative snapshot: %+v", item)
	}
	if string(item.Attributes) != `{"size":"regular"}` || item.ProductName != "Fruit Tea" || item.SKU != "TEA-1" {
		t.Fatalf("order item product snapshot is incomplete: %+v", item)
	}
}

func TestBuildCommerceOrderItemRejectsMoneyOverflow(t *testing.T) {
	_, err := buildCommerceOrderItem(uuid.New(), uuid.New(), models.CommerceCartItem{Quantity: 2}, &commerceCheckoutCatalogueRow{EffectivePriceMinor: math.MaxInt64})
	if !errors.Is(err, ErrCommerceConflict) {
		t.Fatalf("expected overflow conflict, got %v", err)
	}
}
