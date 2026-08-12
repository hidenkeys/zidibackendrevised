package commerceonboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type ApplyReport struct {
	Summary
	OrganizationID         uuid.UUID
	MerchantProfileID      uuid.UUID
	InventoryRowsCreated   int
	InventoryRowsPreserved int
}

func Apply(ctx context.Context, db *gorm.DB, config Config) (ApplyReport, error) {
	if db == nil {
		return ApplyReport{}, fmt.Errorf("database is required")
	}
	if err := config.Validate(); err != nil {
		return ApplyReport{}, err
	}
	organizationID, err := uuid.Parse(config.OrganizationID)
	if err != nil {
		return ApplyReport{}, fmt.Errorf("parse organization_id: %w", err)
	}
	report := ApplyReport{Summary: config.Summary(), OrganizationID: organizationID}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var organizationCount int64
		if err := tx.Model(&models.Organization{}).Where("id = ?", organizationID).Count(&organizationCount).Error; err != nil {
			return fmt.Errorf("check organization: %w", err)
		}
		if organizationCount == 0 {
			return fmt.Errorf("organization %s does not exist", organizationID)
		}

		profile, err := upsertMerchant(tx, organizationID, config.Merchant)
		if err != nil {
			return err
		}
		report.MerchantProfileID = profile.ID

		stores := make(map[string]models.CommerceStore, len(config.Stores))
		for _, item := range config.Stores {
			store, err := upsertStore(tx, organizationID, item)
			if err != nil {
				return err
			}
			stores[strings.ToLower(item.Code)] = store
		}

		variants := make([]configuredVariant, 0, report.Variants)
		for _, categoryConfig := range config.Categories {
			category, err := upsertCategory(tx, organizationID, categoryConfig)
			if err != nil {
				return err
			}
			for _, productConfig := range categoryConfig.Products {
				product, err := upsertProduct(tx, organizationID, category.ID, productConfig)
				if err != nil {
					return err
				}
				for _, variantConfig := range productConfig.Variants {
					variant, err := upsertVariant(tx, organizationID, product.ID, variantConfig)
					if err != nil {
						return err
					}
					variants = append(variants, configuredVariant{Product: productConfig, Variant: variant})
				}
				if err := upsertImages(tx, organizationID, product.ID, productConfig.Images); err != nil {
					return err
				}
			}
		}

		for _, store := range stores {
			for _, item := range variants {
				created, err := upsertStoreInventory(tx, organizationID, store.ID, item, config.Inventory)
				if err != nil {
					return err
				}
				if created {
					report.InventoryRowsCreated++
				} else {
					report.InventoryRowsPreserved++
				}
			}
		}
		return upsertWhatsApp(tx, organizationID, config.WhatsApp)
	})
	if err != nil {
		return ApplyReport{}, fmt.Errorf("apply commerce onboarding: %w", err)
	}
	return report, nil
}

type configuredVariant struct {
	Product ProductConfig
	Variant models.CommerceProductVariant
}

func upsertMerchant(tx *gorm.DB, organizationID uuid.UUID, config MerchantConfig) (models.CommerceMerchantProfile, error) {
	var item models.CommerceMerchantProfile
	err := tx.Unscoped().Where("organization_id = ?", organizationID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item = models.CommerceMerchantProfile{ID: uuid.New(), OrganizationID: organizationID}
	} else if err != nil {
		return item, fmt.Errorf("find merchant profile: %w", err)
	}
	item.Slug, item.DisplayName = config.Slug, config.DisplayName
	item.DefaultCurrency, item.Timezone, item.Status = strings.ToUpper(config.DefaultCurrency), config.Timezone, config.Status
	item.DeletedAt = gorm.DeletedAt{}
	if item.CreatedAt.IsZero() {
		if err := tx.Create(&item).Error; err != nil {
			return item, fmt.Errorf("create merchant profile: %w", err)
		}
	} else if err := tx.Unscoped().Save(&item).Error; err != nil {
		return item, fmt.Errorf("update merchant profile: %w", err)
	}
	return item, nil
}

func upsertStore(tx *gorm.DB, organizationID uuid.UUID, config StoreConfig) (models.CommerceStore, error) {
	var item models.CommerceStore
	err := tx.Unscoped().Where("organization_id = ? AND LOWER(code) = LOWER(?)", organizationID, config.Code).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item = models.CommerceStore{ID: uuid.New(), OrganizationID: organizationID}
	} else if err != nil {
		return item, fmt.Errorf("find store %s: %w", config.Code, err)
	}
	item.Code, item.Name, item.Address = config.Code, config.Name, config.Address
	item.City, item.State, item.CountryCode = config.City, config.State, strings.ToUpper(config.CountryCode)
	item.Latitude, item.Longitude, item.Timezone = config.Latitude, config.Longitude, config.Timezone
	item.PreparationMinutes, item.Status, item.DeletedAt = config.PreparationMinutes, config.Status, gorm.DeletedAt{}
	if item.CreatedAt.IsZero() {
		if err := tx.Create(&item).Error; err != nil {
			return item, fmt.Errorf("create store %s: %w", config.Code, err)
		}
	} else if err := tx.Unscoped().Save(&item).Error; err != nil {
		return item, fmt.Errorf("update store %s: %w", config.Code, err)
	}

	for _, hour := range config.Hours {
		var existing models.CommerceStoreHour
		err := tx.Unscoped().Where("organization_id = ? AND store_id = ? AND day_of_week = ?", organizationID, item.ID, hour.DayOfWeek).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			existing = models.CommerceStoreHour{ID: uuid.New(), OrganizationID: organizationID, StoreID: item.ID}
		} else if err != nil {
			return item, fmt.Errorf("find store hours %s: %w", config.Code, err)
		}
		existing.DayOfWeek, existing.OpenMinute, existing.CloseMinute, existing.IsClosed = hour.DayOfWeek, hour.OpenMinute, hour.CloseMinute, hour.IsClosed
		existing.DeletedAt = gorm.DeletedAt{}
		if existing.CreatedAt.IsZero() {
			err = tx.Create(&existing).Error
		} else {
			err = tx.Unscoped().Save(&existing).Error
		}
		if err != nil {
			return item, fmt.Errorf("upsert store hours %s: %w", config.Code, err)
		}
	}
	for _, mode := range config.FulfilmentModes {
		var existing models.CommerceStoreFulfilmentMode
		err := tx.Unscoped().Where("organization_id = ? AND store_id = ? AND mode = ?", organizationID, item.ID, mode.Mode).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			existing = models.CommerceStoreFulfilmentMode{ID: uuid.New(), OrganizationID: organizationID, StoreID: item.ID}
		} else if err != nil {
			return item, fmt.Errorf("find store fulfilment %s: %w", config.Code, err)
		}
		existing.Mode, existing.Enabled, existing.DeletedAt = mode.Mode, mode.Enabled, gorm.DeletedAt{}
		if existing.CreatedAt.IsZero() {
			err = tx.Create(&existing).Error
		} else {
			err = tx.Unscoped().Save(&existing).Error
		}
		if err != nil {
			return item, fmt.Errorf("upsert store fulfilment %s: %w", config.Code, err)
		}
	}
	return item, nil
}

func upsertCategory(tx *gorm.DB, organizationID uuid.UUID, config CategoryConfig) (models.CommerceCategory, error) {
	var item models.CommerceCategory
	err := tx.Unscoped().Where("organization_id = ? AND LOWER(slug) = LOWER(?)", organizationID, config.Slug).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item = models.CommerceCategory{ID: uuid.New(), OrganizationID: organizationID}
	} else if err != nil {
		return item, fmt.Errorf("find category %s: %w", config.Slug, err)
	}
	item.Name, item.Slug, item.Description = config.Name, config.Slug, config.Description
	item.SortOrder, item.Status, item.DeletedAt = config.SortOrder, config.Status, gorm.DeletedAt{}
	if item.CreatedAt.IsZero() {
		err = tx.Create(&item).Error
	} else {
		err = tx.Unscoped().Save(&item).Error
	}
	if err != nil {
		return item, fmt.Errorf("upsert category %s: %w", config.Slug, err)
	}
	return item, nil
}

func upsertProduct(tx *gorm.DB, organizationID, categoryID uuid.UUID, config ProductConfig) (models.CommerceProduct, error) {
	var item models.CommerceProduct
	err := tx.Unscoped().Where("organization_id = ? AND LOWER(slug) = LOWER(?)", organizationID, config.Slug).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item = models.CommerceProduct{ID: uuid.New(), OrganizationID: organizationID}
	} else if err != nil {
		return item, fmt.Errorf("find product %s: %w", config.Slug, err)
	}
	item.CategoryID, item.Name, item.Slug = categoryID, config.Name, config.Slug
	item.Description, item.Currency, item.Status = config.Description, strings.ToUpper(config.Currency), config.Status
	item.DeletedAt = gorm.DeletedAt{}
	if item.CreatedAt.IsZero() {
		err = tx.Create(&item).Error
	} else {
		err = tx.Unscoped().Save(&item).Error
	}
	if err != nil {
		return item, fmt.Errorf("upsert product %s: %w", config.Slug, err)
	}
	return item, nil
}

func upsertVariant(tx *gorm.DB, organizationID, productID uuid.UUID, config VariantConfig) (models.CommerceProductVariant, error) {
	attributes, err := json.Marshal(config.Attributes)
	if err != nil {
		return models.CommerceProductVariant{}, fmt.Errorf("encode variant attributes %s: %w", config.SKU, err)
	}
	var item models.CommerceProductVariant
	err = tx.Unscoped().Where("organization_id = ? AND LOWER(sku) = LOWER(?)", organizationID, config.SKU).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item = models.CommerceProductVariant{ID: uuid.New(), OrganizationID: organizationID}
	} else if err != nil {
		return item, fmt.Errorf("find variant %s: %w", config.SKU, err)
	}
	item.ProductID, item.Name, item.SKU = productID, config.Name, config.SKU
	item.PriceMinor, item.Attributes, item.IsDefault, item.Status = config.PriceMinor, attributes, config.IsDefault, config.Status
	item.DeletedAt = gorm.DeletedAt{}
	if item.CreatedAt.IsZero() {
		err = tx.Create(&item).Error
	} else {
		err = tx.Unscoped().Save(&item).Error
	}
	if err != nil {
		return item, fmt.Errorf("upsert variant %s: %w", config.SKU, err)
	}
	return item, nil
}

func upsertImages(tx *gorm.DB, organizationID, productID uuid.UUID, configs []ImageConfig) error {
	for _, config := range configs {
		var item models.CommerceProductImage
		err := tx.Unscoped().Where("organization_id = ? AND product_id = ? AND url = ?", organizationID, productID, config.URL).First(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			item = models.CommerceProductImage{ID: uuid.New(), OrganizationID: organizationID, ProductID: productID, URL: config.URL}
		} else if err != nil {
			return fmt.Errorf("find product image: %w", err)
		}
		item.AltText, item.SortOrder, item.DeletedAt = config.AltText, config.SortOrder, gorm.DeletedAt{}
		if item.CreatedAt.IsZero() {
			err = tx.Create(&item).Error
		} else {
			err = tx.Unscoped().Save(&item).Error
		}
		if err != nil {
			return fmt.Errorf("upsert product image: %w", err)
		}
	}
	return nil
}

func upsertStoreInventory(tx *gorm.DB, organizationID, storeID uuid.UUID, item configuredVariant, defaults InventoryDefaults) (bool, error) {
	enabled := defaults.Enabled
	quantity := defaults.InitialQuantity
	reorder := defaults.ReorderThreshold
	if item.Product.Enabled != nil {
		enabled = *item.Product.Enabled
	}
	if item.Product.InitialQuantity != nil {
		quantity = *item.Product.InitialQuantity
	}
	if item.Product.ReorderThreshold != nil {
		reorder = *item.Product.ReorderThreshold
	}

	var catalogue models.CommerceStoreCatalogueItem
	err := tx.Unscoped().Where("organization_id = ? AND store_id = ? AND variant_id = ?", organizationID, storeID, item.Variant.ID).First(&catalogue).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		catalogue = models.CommerceStoreCatalogueItem{ID: uuid.New(), OrganizationID: organizationID, StoreID: storeID, VariantID: item.Variant.ID}
	} else if err != nil {
		return false, fmt.Errorf("find store catalogue item: %w", err)
	}
	catalogue.Enabled, catalogue.DeletedAt = enabled, gorm.DeletedAt{}
	if catalogue.CreatedAt.IsZero() {
		err = tx.Create(&catalogue).Error
	} else {
		err = tx.Unscoped().Save(&catalogue).Error
	}
	if err != nil {
		return false, fmt.Errorf("upsert store catalogue item: %w", err)
	}

	var inventory models.CommerceInventoryLevel
	err = tx.Where("organization_id = ? AND store_id = ? AND variant_id = ?", organizationID, storeID, item.Variant.ID).First(&inventory).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		inventory = models.CommerceInventoryLevel{
			ID: uuid.New(), OrganizationID: organizationID, StoreID: storeID, VariantID: item.Variant.ID,
			QuantityOnHand: quantity, ReorderThreshold: reorder, Version: 1,
		}
		if err := tx.Create(&inventory).Error; err != nil {
			return false, fmt.Errorf("create inventory level: %w", err)
		}
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("find inventory level: %w", err)
	}
	if err := tx.Model(&inventory).Update("reorder_threshold", reorder).Error; err != nil {
		return false, fmt.Errorf("update inventory reorder threshold: %w", err)
	}
	return false, nil
}

func upsertWhatsApp(tx *gorm.DB, organizationID uuid.UUID, config WhatsAppConfig) error {
	var item models.CommerceChannelConfiguration
	err := tx.Where("organization_id = ? AND channel = ?", organizationID, models.CommerceChannelWhatsApp).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item = models.CommerceChannelConfiguration{ID: uuid.New(), OrganizationID: organizationID, Channel: models.CommerceChannelWhatsApp}
	} else if err != nil {
		return fmt.Errorf("find WhatsApp configuration: %w", err)
	}
	displayPhone := strings.TrimSpace(config.DisplayPhoneNumber)
	item.ProviderAccountID = strings.TrimSpace(config.ProviderAccountID)
	item.DisplayPhoneNumber = &displayPhone
	item.WelcomeMessage, item.Status = config.WelcomeMessage, config.Status
	if item.CreatedAt.IsZero() {
		err = tx.Create(&item).Error
	} else {
		err = tx.Save(&item).Error
	}
	if err != nil {
		return fmt.Errorf("upsert WhatsApp configuration: %w", err)
	}
	return nil
}
