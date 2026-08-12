package commerceonboarding

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const CurrentConfigVersion = 1

var (
	slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	e164Pattern = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
)

type Config struct {
	Version        int               `json:"version"`
	OrganizationID string            `json:"organization_id"`
	Merchant       MerchantConfig    `json:"merchant"`
	Stores         []StoreConfig     `json:"stores"`
	Categories     []CategoryConfig  `json:"categories"`
	Inventory      InventoryDefaults `json:"inventory_defaults"`
	WhatsApp       WhatsAppConfig    `json:"whatsapp"`
}

type MerchantConfig struct {
	Slug            string `json:"slug"`
	DisplayName     string `json:"display_name"`
	DefaultCurrency string `json:"default_currency"`
	Timezone        string `json:"timezone"`
	Status          string `json:"status"`
}

type StoreConfig struct {
	Code               string                 `json:"code"`
	Name               string                 `json:"name"`
	Address            string                 `json:"address"`
	City               string                 `json:"city"`
	State              string                 `json:"state"`
	CountryCode        string                 `json:"country_code"`
	Latitude           *float64               `json:"latitude"`
	Longitude          *float64               `json:"longitude"`
	Timezone           string                 `json:"timezone"`
	PreparationMinutes int                    `json:"preparation_minutes"`
	Status             string                 `json:"status"`
	Hours              []StoreHourConfig      `json:"hours"`
	FulfilmentModes    []FulfilmentModeConfig `json:"fulfilment_modes"`
}

type StoreHourConfig struct {
	DayOfWeek   int  `json:"day_of_week"`
	OpenMinute  *int `json:"open_minute"`
	CloseMinute *int `json:"close_minute"`
	IsClosed    bool `json:"is_closed"`
}

type FulfilmentModeConfig struct {
	Mode    string `json:"mode"`
	Enabled bool   `json:"enabled"`
}

type CategoryConfig struct {
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description"`
	SortOrder   int             `json:"sort_order"`
	Status      string          `json:"status"`
	Products    []ProductConfig `json:"products"`
}

type ProductConfig struct {
	Name             string          `json:"name"`
	Slug             string          `json:"slug"`
	Description      string          `json:"description"`
	Currency         string          `json:"currency"`
	Status           string          `json:"status"`
	Variants         []VariantConfig `json:"variants"`
	Images           []ImageConfig   `json:"images"`
	Enabled          *bool           `json:"enabled"`
	InitialQuantity  *int            `json:"initial_quantity"`
	ReorderThreshold *int            `json:"reorder_threshold"`
}

type VariantConfig struct {
	Name       string            `json:"name"`
	SKU        string            `json:"sku"`
	PriceMinor int64             `json:"price_minor"`
	Attributes map[string]string `json:"attributes"`
	IsDefault  bool              `json:"is_default"`
	Status     string            `json:"status"`
}

type ImageConfig struct {
	URL       string `json:"url"`
	AltText   string `json:"alt_text"`
	SortOrder int    `json:"sort_order"`
}

type InventoryDefaults struct {
	Enabled          bool `json:"enabled"`
	InitialQuantity  int  `json:"initial_quantity"`
	ReorderThreshold int  `json:"reorder_threshold"`
}

type WhatsAppConfig struct {
	ProviderAccountID  string `json:"provider_account_id"`
	DisplayPhoneNumber string `json:"display_phone_number"`
	WelcomeMessage     string `json:"welcome_message"`
	Status             string `json:"status"`
}

type Summary struct {
	Stores     int
	Categories int
	Products   int
	Variants   int
	StoreItems int
}

func Load(path string) (Config, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read commerce onboarding config: %w", err)
	}
	missing := map[string]struct{}{}
	expanded := os.Expand(string(contents), func(key string) string {
		value, ok := os.LookupEnv(key)
		if !ok || strings.TrimSpace(value) == "" {
			missing[key] = struct{}{}
		}
		return value
	})
	if len(missing) > 0 {
		keys := make([]string, 0, len(missing))
		for key := range missing {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(keys, ", "))
	}

	var config Config
	decoder := json.NewDecoder(strings.NewReader(expanded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode commerce onboarding config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (config Config) Validate() error {
	if config.Version != CurrentConfigVersion {
		return fmt.Errorf("unsupported commerce onboarding config version %d", config.Version)
	}
	if _, err := uuid.Parse(strings.TrimSpace(config.OrganizationID)); err != nil {
		return fmt.Errorf("organization_id must be a valid UUID")
	}
	if err := validateMerchant(config.Merchant); err != nil {
		return err
	}
	if len(config.Stores) == 0 {
		return fmt.Errorf("at least one store is required")
	}
	if len(config.Categories) == 0 {
		return fmt.Errorf("at least one category is required")
	}
	if config.Inventory.InitialQuantity < 0 || config.Inventory.ReorderThreshold < 0 {
		return fmt.Errorf("inventory defaults cannot be negative")
	}
	if strings.TrimSpace(config.WhatsApp.ProviderAccountID) == "" || strings.TrimSpace(config.WhatsApp.DisplayPhoneNumber) == "" {
		return fmt.Errorf("WhatsApp provider account and display phone number are required")
	}
	if !e164Pattern.MatchString(strings.TrimSpace(config.WhatsApp.DisplayPhoneNumber)) {
		return fmt.Errorf("WhatsApp display phone number must use E.164 format")
	}
	if err := validateStatus("WhatsApp", config.WhatsApp.Status); err != nil {
		return err
	}

	storeCodes := map[string]struct{}{}
	for _, store := range config.Stores {
		code := strings.ToLower(strings.TrimSpace(store.Code))
		if code == "" || strings.TrimSpace(store.Name) == "" || strings.TrimSpace(store.Address) == "" {
			return fmt.Errorf("every store requires code, name, and address")
		}
		if _, exists := storeCodes[code]; exists {
			return fmt.Errorf("duplicate store code %q", store.Code)
		}
		storeCodes[code] = struct{}{}
		if len(strings.TrimSpace(store.CountryCode)) != 2 {
			return fmt.Errorf("store %q requires a two-letter country code", store.Code)
		}
		if store.PreparationMinutes < 0 || store.PreparationMinutes > 1440 {
			return fmt.Errorf("store %q has invalid preparation_minutes", store.Code)
		}
		if err := validateStatus("store "+store.Code, store.Status); err != nil {
			return err
		}
		seenDays := map[int]struct{}{}
		for _, hour := range store.Hours {
			if hour.DayOfWeek < 0 || hour.DayOfWeek > 6 {
				return fmt.Errorf("store %q has invalid day_of_week", store.Code)
			}
			if _, exists := seenDays[hour.DayOfWeek]; exists {
				return fmt.Errorf("store %q repeats day_of_week %d", store.Code, hour.DayOfWeek)
			}
			seenDays[hour.DayOfWeek] = struct{}{}
			if !hour.IsClosed && (hour.OpenMinute == nil || hour.CloseMinute == nil || *hour.OpenMinute < 0 || *hour.CloseMinute > 1440 || *hour.OpenMinute >= *hour.CloseMinute) {
				return fmt.Errorf("store %q has invalid opening hours for day %d", store.Code, hour.DayOfWeek)
			}
		}
	}

	categorySlugs := map[string]struct{}{}
	productSlugs := map[string]struct{}{}
	skus := map[string]struct{}{}
	for _, category := range config.Categories {
		if strings.TrimSpace(category.Name) == "" || !slugPattern.MatchString(category.Slug) {
			return fmt.Errorf("category %q has an invalid name or slug", category.Name)
		}
		if _, exists := categorySlugs[category.Slug]; exists {
			return fmt.Errorf("duplicate category slug %q", category.Slug)
		}
		categorySlugs[category.Slug] = struct{}{}
		if err := validateStatus("category "+category.Slug, category.Status); err != nil {
			return err
		}
		for _, product := range category.Products {
			if strings.TrimSpace(product.Name) == "" || !slugPattern.MatchString(product.Slug) {
				return fmt.Errorf("product %q has an invalid name or slug", product.Name)
			}
			if _, exists := productSlugs[product.Slug]; exists {
				return fmt.Errorf("duplicate product slug %q", product.Slug)
			}
			productSlugs[product.Slug] = struct{}{}
			if len(product.Currency) != 3 || len(product.Variants) == 0 {
				return fmt.Errorf("product %q requires a three-letter currency and at least one variant", product.Name)
			}
			if err := validateStatus("product "+product.Slug, product.Status); err != nil {
				return err
			}
			defaults := 0
			for _, variant := range product.Variants {
				sku := strings.ToLower(strings.TrimSpace(variant.SKU))
				if strings.TrimSpace(variant.Name) == "" || sku == "" || variant.PriceMinor < 0 {
					return fmt.Errorf("product %q has an invalid variant", product.Name)
				}
				if _, exists := skus[sku]; exists {
					return fmt.Errorf("duplicate variant SKU %q", variant.SKU)
				}
				skus[sku] = struct{}{}
				if variant.IsDefault {
					defaults++
				}
				if err := validateStatus("variant "+variant.SKU, variant.Status); err != nil {
					return err
				}
			}
			if defaults != 1 {
				return fmt.Errorf("product %q must have exactly one default variant", product.Name)
			}
		}
	}
	return nil
}

func (config Config) Summary() Summary {
	summary := Summary{Stores: len(config.Stores), Categories: len(config.Categories)}
	for _, category := range config.Categories {
		summary.Products += len(category.Products)
		for _, product := range category.Products {
			summary.Variants += len(product.Variants)
		}
	}
	summary.StoreItems = summary.Stores * summary.Variants
	return summary
}

func validateMerchant(merchant MerchantConfig) error {
	if strings.TrimSpace(merchant.DisplayName) == "" || !slugPattern.MatchString(merchant.Slug) {
		return fmt.Errorf("merchant display_name or slug is invalid")
	}
	if len(merchant.DefaultCurrency) != 3 || strings.TrimSpace(merchant.Timezone) == "" {
		return fmt.Errorf("merchant currency or timezone is invalid")
	}
	return validateStatus("merchant", merchant.Status)
}

func validateStatus(resource, status string) error {
	if status != "active" && status != "inactive" {
		return fmt.Errorf("%s status must be active or inactive", resource)
	}
	return nil
}
