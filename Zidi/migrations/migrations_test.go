package migrations

import (
	"strings"
	"testing"
)

func TestEmbeddedMigrationsAreValidAndOrdered(t *testing.T) {
	items, err := load()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one embedded migration")
	}
	for index, item := range items {
		if item.Checksum == "" || item.SQL == "" || item.Name == "" {
			t.Fatalf("migration %d is incomplete", item.Version)
		}
		if index > 0 && items[index-1].Version >= item.Version {
			t.Fatalf("migrations are not strictly ordered: %d then %d", items[index-1].Version, item.Version)
		}
		upperSQL := strings.ToUpper(item.SQL)
		for _, destructiveStatement := range []string{"DROP TABLE", "TRUNCATE TABLE", "DROP COLUMN"} {
			if strings.Contains(upperSQL, destructiveStatement) {
				t.Fatalf("migration %d contains destructive statement %q", item.Version, destructiveStatement)
			}
		}
	}
}

func TestCommerceCustomerCartMigrationPreservesTenantAndCartInvariants(t *testing.T) {
	items, err := load()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var phaseThreeSQL string
	for _, item := range items {
		if item.Version == 3 {
			phaseThreeSQL = strings.ToLower(item.SQL)
			break
		}
	}
	if phaseThreeSQL == "" {
		t.Fatal("phase 3 commerce customer/cart migration is missing")
	}
	for _, required := range []string{
		"create table if not exists commerce_customers",
		"create table if not exists commerce_customer_identities",
		"create table if not exists commerce_carts",
		"create table if not exists commerce_cart_items",
		"foreign key (customer_id, organization_id)",
		"foreign key (store_id, organization_id)",
		"foreign key (variant_id, organization_id)",
		"uq_commerce_customer_identity",
		"uq_commerce_active_cart_customer_store",
		"where status = 'active'",
	} {
		if !strings.Contains(phaseThreeSQL, required) {
			t.Fatalf("phase 3 migration is missing invariant %q", required)
		}
	}
}

func TestCommerceOrderMigrationPreservesFinancialAndTenantInvariants(t *testing.T) {
	items, err := load()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var phaseFourSQL string
	for _, item := range items {
		if item.Version == 4 {
			phaseFourSQL = strings.ToLower(item.SQL)
			break
		}
	}
	if phaseFourSQL == "" {
		t.Fatal("phase 4 commerce order migration is missing")
	}
	for _, required := range []string{
		"create table if not exists commerce_orders",
		"create table if not exists commerce_order_items",
		"create table if not exists commerce_order_events",
		"foreign key (cart_id, organization_id)",
		"foreign key (customer_id, organization_id)",
		"foreign key (store_id, organization_id)",
		"foreign key (inventory_reservation_id, organization_id)",
		"subtotal_minor bigint",
		"unit_price_minor bigint",
		"line_total_minor bigint",
		"uq_commerce_orders_checkout_key",
		"uq_commerce_orders_cart",
		"uq_commerce_order_event_idempotency",
		"total_minor = subtotal_minor - discount_minor + delivery_fee_minor",
	} {
		if !strings.Contains(phaseFourSQL, required) {
			t.Fatalf("phase 4 migration is missing invariant %q", required)
		}
	}
	if strings.Contains(phaseFourSQL, "double precision") || strings.Contains(phaseFourSQL, " real ") {
		t.Fatal("phase 4 financial schema uses floating-point money")
	}
}

func TestCommercePaymentMigrationPreservesFinancialAndIdempotencyInvariants(t *testing.T) {
	items, err := load()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var phaseFiveSQL string
	for _, item := range items {
		if item.Version == 5 {
			phaseFiveSQL = strings.ToLower(item.SQL)
			break
		}
	}
	if phaseFiveSQL == "" {
		t.Fatal("phase 5 commerce payments/invoices migration is missing")
	}
	for _, required := range []string{
		"create table if not exists commerce_invoices",
		"create table if not exists commerce_invoice_items",
		"create table if not exists commerce_payment_transactions",
		"create table if not exists commerce_payment_webhook_events",
		"create table if not exists commerce_outbox_events",
		"foreign key (order_id, organization_id)",
		"foreign key (invoice_id, organization_id)",
		"foreign key (payment_id, organization_id)",
		"foreign key (order_item_id, organization_id)",
		"uq_commerce_payment_reference",
		"uq_commerce_payment_idempotency",
		"uq_commerce_webhook_event",
		"uq_commerce_outbox_deduplication",
		"amount_minor bigint",
		"total_minor bigint",
		"total_minor = subtotal_minor - discount_minor + delivery_fee_minor",
	} {
		if !strings.Contains(phaseFiveSQL, required) {
			t.Fatalf("phase 5 migration is missing invariant %q", required)
		}
	}
	if strings.Contains(phaseFiveSQL, "double precision") || strings.Contains(phaseFiveSQL, " real ") {
		t.Fatal("phase 5 financial schema uses floating-point money")
	}
}

func TestCommerceFulfilmentMigrationPreservesVerificationAndAuditInvariants(t *testing.T) {
	items, err := load()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var phaseSixSQL string
	for _, item := range items {
		if item.Version == 6 {
			phaseSixSQL = strings.ToLower(item.SQL)
			break
		}
	}
	if phaseSixSQL == "" {
		t.Fatal("phase 6 commerce fulfilment migration is missing")
	}
	for _, required := range []string{
		"create table if not exists commerce_fulfilments",
		"create table if not exists commerce_delivery_quotes",
		"create table if not exists commerce_rider_assignments",
		"create table if not exists commerce_fulfilment_events",
		"verification_code_hash bytea",
		"verification_code_ciphertext bytea",
		"foreign key (fulfilment_id, organization_id)",
		"foreign key (order_id, organization_id)",
		"uq_commerce_fulfilment_order",
		"uq_commerce_delivery_quote_idempotency",
		"uq_commerce_rider_assignment_idempotency",
		"uq_commerce_fulfilment_event_idempotency",
		"where status in ('assigned', 'arrived', 'picked_up')",
		"estimated_fee_minor bigint",
		"direct_to_rider",
	} {
		if !strings.Contains(phaseSixSQL, required) {
			t.Fatalf("phase 6 migration is missing invariant %q", required)
		}
	}
}

func TestCommerceChannelMigrationPreservesIdentityAndDeliveryInvariants(t *testing.T) {
	items, err := load()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var phaseSevenSQL string
	for _, item := range items {
		if item.Version == 7 {
			phaseSevenSQL = strings.ToLower(item.SQL)
			break
		}
	}
	if phaseSevenSQL == "" {
		t.Fatal("phase 7 commerce customer channel migration is missing")
	}
	for _, required := range []string{
		"create table if not exists commerce_channel_configurations",
		"create table if not exists commerce_conversations",
		"create table if not exists commerce_channel_messages",
		"create table if not exists commerce_complaints",
		"foreign key (channel_configuration_id, organization_id)",
		"foreign key (customer_id, organization_id)",
		"foreign key (conversation_id, organization_id)",
		"uq_commerce_channel_provider_account",
		"uq_commerce_conversation_identity",
		"uq_commerce_channel_inbound_message",
		"processing_message_id uuid",
		"locked_until timestamptz",
		"add column if not exists locked_at timestamptz",
		"add column if not exists destination_address text",
	} {
		if !strings.Contains(phaseSevenSQL, required) {
			t.Fatalf("phase 7 migration is missing invariant %q", required)
		}
	}
}
