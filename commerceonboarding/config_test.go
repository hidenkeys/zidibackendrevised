package commerceonboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExpandsEnvironmentAndSummarizes(t *testing.T) {
	t.Setenv("TEST_ORGANIZATION_ID", "11111111-1111-4111-8111-111111111111")
	t.Setenv("TEST_PROVIDER_ID", "provider-123")
	t.Setenv("TEST_PHONE", "+2348000000000")
	path := writeConfig(t, validConfigJSON())

	config, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.OrganizationID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("organization variable was not expanded: %q", config.OrganizationID)
	}
	summary := config.Summary()
	if summary.Stores != 1 || summary.Categories != 1 || summary.Products != 1 || summary.Variants != 1 || summary.StoreItems != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestLoadReportsMissingEnvironment(t *testing.T) {
	_ = os.Unsetenv("TEST_ORGANIZATION_ID")
	_ = os.Unsetenv("TEST_PROVIDER_ID")
	_ = os.Unsetenv("TEST_PHONE")
	_, err := Load(writeConfig(t, validConfigJSON()))
	if err == nil || !strings.Contains(err.Error(), "TEST_ORGANIZATION_ID") {
		t.Fatalf("expected missing environment error, got %v", err)
	}
}

func TestValidateRejectsInvalidOrganizationID(t *testing.T) {
	t.Setenv("TEST_ORGANIZATION_ID", "not-a-uuid")
	t.Setenv("TEST_PROVIDER_ID", "provider-123")
	t.Setenv("TEST_PHONE", "+2348000000000")
	_, err := Load(writeConfig(t, validConfigJSON()))
	if err == nil || !strings.Contains(err.Error(), "valid UUID") {
		t.Fatalf("expected organization UUID validation error, got %v", err)
	}
}

func TestValidateRejectsDuplicateSKU(t *testing.T) {
	config := Config{
		Version: CurrentConfigVersion, OrganizationID: "11111111-1111-4111-8111-111111111111",
		Merchant: MerchantConfig{Slug: "merchant", DisplayName: "Merchant", DefaultCurrency: "NGN", Timezone: "Africa/Lagos", Status: "active"},
		Stores:   []StoreConfig{{Code: "ONE", Name: "One", Address: "Address", CountryCode: "NG", Status: "active"}},
		Categories: []CategoryConfig{{Name: "Tea", Slug: "tea", Status: "active", Products: []ProductConfig{
			{Name: "One", Slug: "one", Currency: "NGN", Status: "active", Variants: []VariantConfig{{Name: "Regular", SKU: "SAME", IsDefault: true, Status: "active"}}},
			{Name: "Two", Slug: "two", Currency: "NGN", Status: "active", Variants: []VariantConfig{{Name: "Regular", SKU: "SAME", IsDefault: true, Status: "active"}}},
		}}},
		WhatsApp: WhatsAppConfig{ProviderAccountID: "provider", DisplayPhoneNumber: "+2348000000000", Status: "active"},
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate variant SKU") {
		t.Fatalf("expected duplicate SKU error, got %v", err)
	}
}

func TestValidateRejectsLocalWhatsAppNumber(t *testing.T) {
	config := Config{
		Version: CurrentConfigVersion, OrganizationID: "11111111-1111-4111-8111-111111111111",
		Merchant: MerchantConfig{Slug: "merchant", DisplayName: "Merchant", DefaultCurrency: "NGN", Timezone: "Africa/Lagos", Status: "active"},
		Stores:   []StoreConfig{{Code: "ONE", Name: "One", Address: "Address", CountryCode: "NG", Status: "active"}},
		Categories: []CategoryConfig{{Name: "Tea", Slug: "tea", Status: "active", Products: []ProductConfig{{
			Name: "Tea", Slug: "tea-product", Currency: "NGN", Status: "active", Variants: []VariantConfig{{Name: "Regular", SKU: "TEA", IsDefault: true, Status: "active"}},
		}}}},
		WhatsApp: WhatsAppConfig{ProviderAccountID: "provider", DisplayPhoneNumber: "08001234567", Status: "active"},
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "E.164") {
		t.Fatalf("expected E.164 validation error, got %v", err)
	}
}

func TestBingChunConfigHasSafeOpeningDefaults(t *testing.T) {
	t.Setenv("BING_CHUN_ORGANIZATION_ID", "11111111-1111-4111-8111-111111111111")
	t.Setenv("BING_CHUN_WHATSAPP_PROVIDER_ACCOUNT_ID", "provider-123")
	t.Setenv("BING_CHUN_WHATSAPP_DISPLAY_PHONE_NUMBER", "+2348000000000")

	config, err := Load(filepath.Join("..", "config", "merchants", "bing-chun-nigeria.json"))
	if err != nil {
		t.Fatalf("load Bing Chun config: %v", err)
	}
	if summary := config.Summary(); summary != (Summary{Stores: 7, Categories: 6, Products: 28, Variants: 28, StoreItems: 196}) {
		t.Fatalf("unexpected Bing Chun summary: %+v", summary)
	}
	activeStores := 0
	for _, store := range config.Stores {
		if store.Status == "active" {
			activeStores++
			if len(store.Hours) != 7 {
				t.Fatalf("active store %s does not have complete weekly hours", store.Code)
			}
		}
		modes := map[string]bool{}
		for _, mode := range store.FulfilmentModes {
			modes[mode.Mode] = mode.Enabled
		}
		if !modes["customer_pickup"] || !modes["customer_rider"] || modes["merchant_rider"] {
			t.Fatalf("store %s has unsafe fulfilment defaults: %+v", store.Code, modes)
		}
	}
	if activeStores != 2 {
		t.Fatalf("expected only stores with verified hours active, got %d", activeStores)
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "merchant.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func validConfigJSON() string {
	return `{
  "version": 1,
  "organization_id": "${TEST_ORGANIZATION_ID}",
  "merchant": {"slug":"merchant","display_name":"Merchant","default_currency":"NGN","timezone":"Africa/Lagos","status":"active"},
  "stores": [{"code":"ONE","name":"One","address":"Address","city":"Lagos","state":"Lagos","country_code":"NG","timezone":"Africa/Lagos","preparation_minutes":20,"status":"active","hours":[],"fulfilment_modes":[]}],
  "categories": [{"name":"Tea","slug":"tea","description":"Tea","sort_order":1,"status":"active","products":[{"name":"Tea","slug":"tea-product","description":"Tea","currency":"NGN","status":"active","variants":[{"name":"Regular","sku":"TEA-REG","price_minor":10000,"attributes":{},"is_default":true,"status":"active"}],"images":[]}]}],
  "inventory_defaults": {"enabled":true,"initial_quantity":10,"reorder_threshold":2},
  "whatsapp": {"provider_account_id":"${TEST_PROVIDER_ID}","display_phone_number":"${TEST_PHONE}","welcome_message":"Welcome","status":"active"}
}`
}
