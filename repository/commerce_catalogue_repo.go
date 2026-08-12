package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCommerceInventoryUnavailable = errors.New("commerce inventory unavailable")
	ErrCommerceReservationState     = errors.New("invalid inventory reservation state")
)

type CommerceStoreCatalogueEntry struct {
	StoreID             uuid.UUID
	CategoryID          uuid.UUID
	CategoryName        string
	ProductID           uuid.UUID
	ProductName         string
	ProductDescription  string
	ProductCurrency     string
	VariantID           uuid.UUID
	VariantName         string
	SKU                 string
	Attributes          []byte
	BasePriceMinor      int64
	PriceOverrideMinor  *int64
	EffectivePriceMinor int64
	PrimaryImageURL     *string
	Enabled             bool
	QuantityOnHand      int
	QuantityReserved    int
	AvailableQuantity   int
	ReorderThreshold    int
}

type InventoryAdjustment struct {
	OrganizationID uuid.UUID
	StoreID        uuid.UUID
	VariantID      uuid.UUID
	QuantityDelta  int
	Reference      string
	Reason         string
	ActorUserID    uuid.UUID
}

type InventoryReservationRequest struct {
	OrganizationID uuid.UUID
	StoreID        uuid.UUID
	VariantID      uuid.UUID
	ReservationKey string
	Quantity       int
	ExpiresAt      time.Time
}

type CommerceCatalogueRepository interface {
	CreateCategory(ctx context.Context, category *models.CommerceCategory) error
	UpdateCategory(ctx context.Context, category *models.CommerceCategory) error
	ListCategories(ctx context.Context, organizationID uuid.UUID) ([]models.CommerceCategory, error)
	CreateProduct(ctx context.Context, product *models.CommerceProduct) error
	UpdateProduct(ctx context.Context, product *models.CommerceProduct) error
	UpdateProductVariant(ctx context.Context, variant *models.CommerceProductVariant) error
	ListProducts(ctx context.Context, organizationID uuid.UUID, categoryID *uuid.UUID, limit, offset int) ([]models.CommerceProduct, int64, error)
	GetProduct(ctx context.Context, organizationID, productID uuid.UUID) (*models.CommerceProduct, error)
	GetProductVariant(ctx context.Context, organizationID, variantID uuid.UUID) (*models.CommerceProductVariant, error)
	ConfigureStoreCatalogueItem(ctx context.Context, item *models.CommerceStoreCatalogueItem, reorderThreshold int) (*models.CommerceStoreCatalogueItem, error)
	ListStoreCatalogue(ctx context.Context, organizationID, storeID uuid.UUID) ([]CommerceStoreCatalogueEntry, error)
	GetStoreCatalogueEntry(ctx context.Context, organizationID, storeID, variantID uuid.UUID) (*CommerceStoreCatalogueEntry, error)
	GetInventoryLevel(ctx context.Context, organizationID, storeID, variantID uuid.UUID) (*models.CommerceInventoryLevel, error)
	AdjustInventory(ctx context.Context, input InventoryAdjustment) (*models.CommerceInventoryLevel, error)
	ReserveInventory(ctx context.Context, input InventoryReservationRequest) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error)
	CommitInventoryReservation(ctx context.Context, organizationID, reservationID uuid.UUID) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error)
	ReleaseInventoryReservation(ctx context.Context, organizationID, reservationID uuid.UUID, expired bool) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error)
	ExpireInventoryReservations(ctx context.Context, organizationID, storeID, variantID uuid.UUID, before time.Time, limit int) (int, error)
}

type CommerceCatalogueRepoPG struct {
	db *gorm.DB
}

func NewCommerceCatalogueRepoPG(db *gorm.DB) *CommerceCatalogueRepoPG {
	return &CommerceCatalogueRepoPG{db: db}
}

func (r *CommerceCatalogueRepoPG) CreateCategory(ctx context.Context, category *models.CommerceCategory) error {
	if err := r.db.WithContext(ctx).Create(category).Error; err != nil {
		return mapCommerceWriteError("create category", err)
	}
	return nil
}

func (r *CommerceCatalogueRepoPG) UpdateCategory(ctx context.Context, category *models.CommerceCategory) error {
	result := r.db.WithContext(ctx).Model(&models.CommerceCategory{}).
		Where("id = ? AND organization_id = ?", category.ID, category.OrganizationID).
		Updates(map[string]interface{}{
			"name": category.Name, "slug": category.Slug, "description": category.Description,
			"sort_order": category.SortOrder, "status": category.Status,
		})
	if result.Error != nil {
		return mapCommerceWriteError("update category", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrCommerceNotFound
	}
	return nil
}

func (r *CommerceCatalogueRepoPG) ListCategories(ctx context.Context, organizationID uuid.UUID) ([]models.CommerceCategory, error) {
	var categories []models.CommerceCategory
	err := r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Order("sort_order ASC, name ASC").
		Find(&categories).Error
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	return categories, nil
}

func (r *CommerceCatalogueRepoPG) CreateProduct(ctx context.Context, product *models.CommerceProduct) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var categoryCount int64
		if err := tx.Model(&models.CommerceCategory{}).
			Where("id = ? AND organization_id = ?", product.CategoryID, product.OrganizationID).
			Count(&categoryCount).Error; err != nil {
			return err
		}
		if categoryCount == 0 {
			return ErrCommerceNotFound
		}

		if err := tx.Omit("Variants", "Images").Create(product).Error; err != nil {
			return err
		}
		if len(product.Variants) > 0 {
			if err := tx.Create(&product.Variants).Error; err != nil {
				return err
			}
		}
		if len(product.Images) > 0 {
			if err := tx.Create(&product.Images).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return mapCommerceWriteError("create product", err)
	}
	return nil
}

func (r *CommerceCatalogueRepoPG) UpdateProduct(ctx context.Context, product *models.CommerceProduct) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var categoryCount int64
		if err := tx.Model(&models.CommerceCategory{}).
			Where("id = ? AND organization_id = ?", product.CategoryID, product.OrganizationID).
			Count(&categoryCount).Error; err != nil {
			return err
		}
		if categoryCount == 0 {
			return ErrCommerceNotFound
		}
		result := tx.Model(&models.CommerceProduct{}).
			Where("id = ? AND organization_id = ?", product.ID, product.OrganizationID).
			Updates(map[string]interface{}{
				"category_id": product.CategoryID, "name": product.Name, "slug": product.Slug,
				"description": product.Description, "currency": product.Currency, "status": product.Status,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrCommerceNotFound
		}
		return nil
	})
	if err != nil {
		return mapCommerceWriteError("update product", err)
	}
	return nil
}

func (r *CommerceCatalogueRepoPG) UpdateProductVariant(ctx context.Context, variant *models.CommerceProductVariant) error {
	result := r.db.WithContext(ctx).Model(&models.CommerceProductVariant{}).
		Where("id = ? AND organization_id = ? AND product_id = ?", variant.ID, variant.OrganizationID, variant.ProductID).
		Updates(map[string]interface{}{
			"name": variant.Name, "price_minor": variant.PriceMinor, "attributes": variant.Attributes, "status": variant.Status,
		})
	if result.Error != nil {
		return mapCommerceWriteError("update product variant", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrCommerceNotFound
	}
	return nil
}

func (r *CommerceCatalogueRepoPG) ListProducts(ctx context.Context, organizationID uuid.UUID, categoryID *uuid.UUID, limit, offset int) ([]models.CommerceProduct, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.CommerceProduct{}).Where("organization_id = ?", organizationID)
	if categoryID != nil {
		query = query.Where("category_id = ?", *categoryID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}
	var products []models.CommerceProduct
	err := query.
		Preload("Variants", func(db *gorm.DB) *gorm.DB { return db.Order("is_default DESC, name ASC") }).
		Preload("Images", func(db *gorm.DB) *gorm.DB { return db.Order("sort_order ASC") }).
		Order("name ASC").
		Limit(limit).
		Offset(offset).
		Find(&products).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	return products, total, nil
}

func (r *CommerceCatalogueRepoPG) GetProduct(ctx context.Context, organizationID, productID uuid.UUID) (*models.CommerceProduct, error) {
	var product models.CommerceProduct
	err := r.db.WithContext(ctx).
		Preload("Variants", func(db *gorm.DB) *gorm.DB { return db.Order("is_default DESC, name ASC") }).
		Preload("Images", func(db *gorm.DB) *gorm.DB { return db.Order("sort_order ASC") }).
		Where("organization_id = ? AND id = ?", organizationID, productID).
		First(&product).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	return &product, nil
}

func (r *CommerceCatalogueRepoPG) GetProductVariant(ctx context.Context, organizationID, variantID uuid.UUID) (*models.CommerceProductVariant, error) {
	var variant models.CommerceProductVariant
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND id = ?", organizationID, variantID).
		First(&variant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get product variant: %w", err)
	}
	return &variant, nil
}

func (r *CommerceCatalogueRepoPG) ConfigureStoreCatalogueItem(ctx context.Context, item *models.CommerceStoreCatalogueItem, reorderThreshold int) (*models.CommerceStoreCatalogueItem, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`
			INSERT INTO commerce_store_catalogue_items
				(id, organization_id, store_id, variant_id, enabled, price_override_minor, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
			ON CONFLICT (organization_id, store_id, variant_id) WHERE deleted_at IS NULL
			DO UPDATE SET enabled = EXCLUDED.enabled,
			              price_override_minor = EXCLUDED.price_override_minor,
			              updated_at = NOW()
		`, item.ID, item.OrganizationID, item.StoreID, item.VariantID, item.Enabled, item.PriceOverrideMinor)
		if result.Error != nil {
			return result.Error
		}

		level := models.CommerceInventoryLevel{
			ID:               uuid.New(),
			OrganizationID:   item.OrganizationID,
			StoreID:          item.StoreID,
			VariantID:        item.VariantID,
			ReorderThreshold: reorderThreshold,
			Version:          1,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "organization_id"}, {Name: "store_id"}, {Name: "variant_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"reorder_threshold": reorderThreshold, "updated_at": time.Now().UTC()}),
		}).Create(&level).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, mapCommerceWriteError("configure store catalogue item", err)
	}

	var saved models.CommerceStoreCatalogueItem
	err = r.db.WithContext(ctx).
		Where("organization_id = ? AND store_id = ? AND variant_id = ?", item.OrganizationID, item.StoreID, item.VariantID).
		First(&saved).Error
	if err != nil {
		return nil, fmt.Errorf("read configured store catalogue item: %w", err)
	}
	return &saved, nil
}

func (r *CommerceCatalogueRepoPG) ListStoreCatalogue(ctx context.Context, organizationID, storeID uuid.UUID) ([]CommerceStoreCatalogueEntry, error) {
	var entries []CommerceStoreCatalogueEntry
	err := r.db.WithContext(ctx).Raw(`
		SELECT items.store_id,
		       categories.id AS category_id,
		       categories.name AS category_name,
		       products.id AS product_id,
		       products.name AS product_name,
		       products.description AS product_description,
		       products.currency AS product_currency,
		       variants.id AS variant_id,
		       variants.name AS variant_name,
		       variants.sku,
		       variants.attributes,
		       variants.price_minor AS base_price_minor,
		       items.price_override_minor,
		       COALESCE(items.price_override_minor, variants.price_minor) AS effective_price_minor,
		       (
		           SELECT images.url
		           FROM commerce_product_images images
		           WHERE images.organization_id = products.organization_id
		             AND images.product_id = products.id
		             AND images.deleted_at IS NULL
		           ORDER BY images.sort_order ASC, images.created_at ASC
		           LIMIT 1
		       ) AS primary_image_url,
		       items.enabled,
		       COALESCE(inventory.quantity_on_hand, 0) AS quantity_on_hand,
		       COALESCE(inventory.quantity_reserved, 0) AS quantity_reserved,
		       COALESCE(inventory.quantity_on_hand - inventory.quantity_reserved, 0) AS available_quantity,
		       COALESCE(inventory.reorder_threshold, 0) AS reorder_threshold
		FROM commerce_store_catalogue_items items
		JOIN commerce_product_variants variants
		  ON variants.id = items.variant_id
		 AND variants.organization_id = items.organization_id
		 AND variants.deleted_at IS NULL
		JOIN commerce_products products
		  ON products.id = variants.product_id
		 AND products.organization_id = variants.organization_id
		 AND products.deleted_at IS NULL
		JOIN commerce_categories categories
		  ON categories.id = products.category_id
		 AND categories.organization_id = products.organization_id
		 AND categories.deleted_at IS NULL
		LEFT JOIN commerce_inventory_levels inventory
		  ON inventory.organization_id = items.organization_id
		 AND inventory.store_id = items.store_id
		 AND inventory.variant_id = items.variant_id
		WHERE items.organization_id = ?
		  AND items.store_id = ?
		  AND items.deleted_at IS NULL
		  AND categories.status = 'active'
		  AND products.status = 'active'
		  AND variants.status = 'active'
		ORDER BY categories.sort_order ASC, products.name ASC, variants.is_default DESC, variants.name ASC
	`, organizationID, storeID).Scan(&entries).Error
	if err != nil {
		return nil, fmt.Errorf("list store catalogue: %w", err)
	}
	return entries, nil
}

func (r *CommerceCatalogueRepoPG) GetStoreCatalogueEntry(ctx context.Context, organizationID, storeID, variantID uuid.UUID) (*CommerceStoreCatalogueEntry, error) {
	entries, err := r.ListStoreCatalogue(ctx, organizationID, storeID)
	if err != nil {
		return nil, err
	}
	for index := range entries {
		if entries[index].VariantID == variantID {
			return &entries[index], nil
		}
	}
	return nil, ErrCommerceNotFound
}

func (r *CommerceCatalogueRepoPG) GetInventoryLevel(ctx context.Context, organizationID, storeID, variantID uuid.UUID) (*models.CommerceInventoryLevel, error) {
	var level models.CommerceInventoryLevel
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND store_id = ? AND variant_id = ?", organizationID, storeID, variantID).
		First(&level).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get inventory level: %w", err)
	}
	return &level, nil
}

func (r *CommerceCatalogueRepoPG) AdjustInventory(ctx context.Context, input InventoryAdjustment) (*models.CommerceInventoryLevel, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.CommerceInventoryMovement
		err := tx.Where("organization_id = ? AND reference = ?", input.OrganizationID, input.Reference).First(&existing).Error
		if err == nil {
			if !sameInventoryAdjustment(&existing, input) {
				return ErrCommerceConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		result := tx.Model(&models.CommerceInventoryLevel{}).
			Where(`organization_id = ? AND store_id = ? AND variant_id = ?
				AND quantity_on_hand + ? >= quantity_reserved
				AND quantity_on_hand + ? >= 0`, input.OrganizationID, input.StoreID, input.VariantID, input.QuantityDelta, input.QuantityDelta).
			Updates(map[string]interface{}{
				"quantity_on_hand": gorm.Expr("quantity_on_hand + ?", input.QuantityDelta),
				"version":          gorm.Expr("version + 1"),
				"updated_at":       time.Now().UTC(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCommerceInventoryUnavailable
		}

		movement := models.CommerceInventoryMovement{
			ID:                  uuid.New(),
			OrganizationID:      input.OrganizationID,
			StoreID:             input.StoreID,
			VariantID:           input.VariantID,
			MovementType:        models.InventoryMovementAdjustment,
			QuantityOnHandDelta: input.QuantityDelta,
			Reference:           input.Reference,
			Reason:              input.Reason,
			CreatedByUserID:     &input.ActorUserID,
		}
		return tx.Create(&movement).Error
	})
	if err != nil {
		mapped := mapInventoryWriteError("adjust inventory", err)
		if errors.Is(mapped, ErrCommerceConflict) {
			existing, lookupErr := r.getInventoryMovementByReference(ctx, input.OrganizationID, input.Reference)
			if lookupErr == nil && sameInventoryAdjustment(existing, input) {
				return r.GetInventoryLevel(ctx, input.OrganizationID, input.StoreID, input.VariantID)
			}
		}
		return nil, mapped
	}
	return r.GetInventoryLevel(ctx, input.OrganizationID, input.StoreID, input.VariantID)
}

func (r *CommerceCatalogueRepoPG) ReserveInventory(ctx context.Context, input InventoryReservationRequest) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error) {
	reservation := &models.CommerceInventoryReservation{
		ID:             uuid.New(),
		OrganizationID: input.OrganizationID,
		StoreID:        input.StoreID,
		VariantID:      input.VariantID,
		ReservationKey: input.ReservationKey,
		Quantity:       input.Quantity,
		Status:         models.InventoryReservationActive,
		ExpiresAt:      input.ExpiresAt,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.CommerceInventoryReservation
		err := tx.Where("organization_id = ? AND reservation_key = ?", input.OrganizationID, input.ReservationKey).First(&existing).Error
		if err == nil {
			if !sameReservation(&existing, input) {
				return ErrCommerceConflict
			}
			*reservation = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var enabledCount int64
		if err := tx.Table("commerce_store_catalogue_items items").
			Joins("JOIN commerce_stores stores ON stores.id = items.store_id AND stores.organization_id = items.organization_id AND stores.deleted_at IS NULL").
			Joins("JOIN commerce_product_variants variants ON variants.id = items.variant_id AND variants.organization_id = items.organization_id AND variants.deleted_at IS NULL").
			Joins("JOIN commerce_products products ON products.id = variants.product_id AND products.organization_id = variants.organization_id AND products.deleted_at IS NULL").
			Joins("JOIN commerce_categories categories ON categories.id = products.category_id AND categories.organization_id = products.organization_id AND categories.deleted_at IS NULL").
			Where("items.organization_id = ? AND items.store_id = ? AND items.variant_id = ? AND items.enabled = TRUE", input.OrganizationID, input.StoreID, input.VariantID).
			Where("stores.status = ? AND variants.status = ? AND products.status = ? AND categories.status = ?", models.CommerceStatusActive, models.CommerceStatusActive, models.CommerceStatusActive, models.CommerceStatusActive).
			Count(&enabledCount).Error; err != nil {
			return err
		}
		if enabledCount == 0 {
			return ErrCommerceInventoryUnavailable
		}

		result := tx.Model(&models.CommerceInventoryLevel{}).
			Where(`organization_id = ? AND store_id = ? AND variant_id = ?
				AND quantity_on_hand - quantity_reserved >= ?`, input.OrganizationID, input.StoreID, input.VariantID, input.Quantity).
			Updates(map[string]interface{}{
				"quantity_reserved": gorm.Expr("quantity_reserved + ?", input.Quantity),
				"version":           gorm.Expr("version + 1"),
				"updated_at":        time.Now().UTC(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCommerceInventoryUnavailable
		}
		if err := tx.Create(reservation).Error; err != nil {
			return err
		}
		movement := models.CommerceInventoryMovement{
			ID:                    uuid.New(),
			OrganizationID:        input.OrganizationID,
			StoreID:               input.StoreID,
			VariantID:             input.VariantID,
			ReservationID:         &reservation.ID,
			MovementType:          models.InventoryMovementReservation,
			QuantityReservedDelta: input.Quantity,
			Reference:             reservationMovementReference(reservation.ID, "reserve"),
			Reason:                "inventory reserved",
		}
		return tx.Create(&movement).Error
	})
	if err != nil {
		mapped := mapInventoryWriteError("reserve inventory", err)
		if errors.Is(mapped, ErrCommerceConflict) {
			existing, lookupErr := r.getReservationByKey(ctx, input.OrganizationID, input.ReservationKey)
			if lookupErr == nil && sameReservation(existing, input) {
				level, levelErr := r.GetInventoryLevel(ctx, input.OrganizationID, input.StoreID, input.VariantID)
				return existing, level, levelErr
			}
		}
		return nil, nil, mapped
	}
	level, err := r.GetInventoryLevel(ctx, input.OrganizationID, input.StoreID, input.VariantID)
	return reservation, level, err
}

func (r *CommerceCatalogueRepoPG) CommitInventoryReservation(ctx context.Context, organizationID, reservationID uuid.UUID) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error) {
	return r.transitionReservation(ctx, organizationID, reservationID, models.InventoryReservationCommitted)
}

func (r *CommerceCatalogueRepoPG) ReleaseInventoryReservation(ctx context.Context, organizationID, reservationID uuid.UUID, expired bool) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error) {
	targetStatus := models.InventoryReservationReleased
	if expired {
		targetStatus = models.InventoryReservationExpired
	}
	return r.transitionReservation(ctx, organizationID, reservationID, targetStatus)
}

func (r *CommerceCatalogueRepoPG) ExpireInventoryReservations(ctx context.Context, organizationID, storeID, variantID uuid.UUID, before time.Time, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	var reservationIDs []uuid.UUID
	err := r.db.WithContext(ctx).
		Model(&models.CommerceInventoryReservation{}).
		Where("organization_id = ? AND store_id = ? AND variant_id = ? AND status = ? AND expires_at <= ?", organizationID, storeID, variantID, models.InventoryReservationActive, before.UTC()).
		Order("expires_at ASC").
		Limit(limit).
		Pluck("id", &reservationIDs).Error
	if err != nil {
		return 0, fmt.Errorf("list expired inventory reservations: %w", err)
	}

	expiredCount := 0
	for _, reservationID := range reservationIDs {
		_, _, err := r.ReleaseInventoryReservation(ctx, organizationID, reservationID, true)
		if errors.Is(err, ErrCommerceReservationState) {
			continue
		}
		if err != nil {
			return expiredCount, err
		}
		expiredCount++
	}
	return expiredCount, nil
}

func (r *CommerceCatalogueRepoPG) transitionReservation(ctx context.Context, organizationID, reservationID uuid.UUID, targetStatus string) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error) {
	var reservation models.CommerceInventoryReservation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND id = ?", organizationID, reservationID).
			First(&reservation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommerceNotFound
		}
		if err != nil {
			return err
		}
		if reservation.Status == targetStatus {
			return nil
		}
		if reservation.Status != models.InventoryReservationActive {
			return ErrCommerceReservationState
		}

		now := time.Now().UTC()
		updates := map[string]interface{}{"status": targetStatus, "updated_at": now}
		movement := models.CommerceInventoryMovement{
			ID:             uuid.New(),
			OrganizationID: reservation.OrganizationID,
			StoreID:        reservation.StoreID,
			VariantID:      reservation.VariantID,
			ReservationID:  &reservation.ID,
			Reference:      reservationMovementReference(reservation.ID, targetStatus),
		}

		inventoryUpdates := map[string]interface{}{
			"quantity_reserved": gorm.Expr("quantity_reserved - ?", reservation.Quantity),
			"version":           gorm.Expr("version + 1"),
			"updated_at":        now,
		}
		if targetStatus == models.InventoryReservationCommitted {
			updates["committed_at"] = now
			reservation.CommittedAt = &now
			inventoryUpdates["quantity_on_hand"] = gorm.Expr("quantity_on_hand - ?", reservation.Quantity)
			movement.MovementType = models.InventoryMovementReservationCommit
			movement.QuantityOnHandDelta = -reservation.Quantity
			movement.QuantityReservedDelta = -reservation.Quantity
			movement.Reason = "inventory reservation committed"
		} else {
			updates["released_at"] = now
			reservation.ReleasedAt = &now
			movement.MovementType = models.InventoryMovementReservationRelease
			movement.QuantityReservedDelta = -reservation.Quantity
			movement.Reason = "inventory reservation released"
		}

		result := tx.Model(&models.CommerceInventoryLevel{}).
			Where("organization_id = ? AND store_id = ? AND variant_id = ? AND quantity_reserved >= ?", reservation.OrganizationID, reservation.StoreID, reservation.VariantID, reservation.Quantity).
			Updates(inventoryUpdates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCommerceInventoryUnavailable
		}
		if err := tx.Model(&reservation).Updates(updates).Error; err != nil {
			return err
		}
		reservation.Status = targetStatus
		reservation.UpdatedAt = now
		if err := tx.Create(&movement).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, mapInventoryWriteError("transition inventory reservation", err)
	}
	level, err := r.GetInventoryLevel(ctx, reservation.OrganizationID, reservation.StoreID, reservation.VariantID)
	return &reservation, level, err
}

func (r *CommerceCatalogueRepoPG) getReservationByKey(ctx context.Context, organizationID uuid.UUID, key string) (*models.CommerceInventoryReservation, error) {
	var reservation models.CommerceInventoryReservation
	err := r.db.WithContext(ctx).Where("organization_id = ? AND reservation_key = ?", organizationID, key).First(&reservation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get inventory reservation: %w", err)
	}
	return &reservation, nil
}

func (r *CommerceCatalogueRepoPG) getInventoryMovementByReference(ctx context.Context, organizationID uuid.UUID, reference string) (*models.CommerceInventoryMovement, error) {
	var movement models.CommerceInventoryMovement
	err := r.db.WithContext(ctx).Where("organization_id = ? AND reference = ?", organizationID, reference).First(&movement).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get inventory movement: %w", err)
	}
	return &movement, nil
}

func sameReservation(reservation *models.CommerceInventoryReservation, input InventoryReservationRequest) bool {
	return reservation.OrganizationID == input.OrganizationID &&
		reservation.StoreID == input.StoreID &&
		reservation.VariantID == input.VariantID &&
		reservation.Quantity == input.Quantity
}

func sameInventoryAdjustment(movement *models.CommerceInventoryMovement, input InventoryAdjustment) bool {
	return movement.OrganizationID == input.OrganizationID &&
		movement.StoreID == input.StoreID &&
		movement.VariantID == input.VariantID &&
		movement.QuantityOnHandDelta == input.QuantityDelta &&
		movement.MovementType == models.InventoryMovementAdjustment
}

func reservationMovementReference(reservationID uuid.UUID, transition string) string {
	return "inventory-reservation:" + reservationID.String() + ":" + transition
}

func mapInventoryWriteError(action string, err error) error {
	if errors.Is(err, ErrCommerceNotFound) || errors.Is(err, ErrCommerceConflict) || errors.Is(err, ErrCommerceInventoryUnavailable) || errors.Is(err, ErrCommerceReservationState) {
		return err
	}
	return mapCommerceWriteError(action, err)
}
