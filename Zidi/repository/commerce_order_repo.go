package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCommerceCheckoutEmptyCart = errors.New("commerce checkout cart is empty")
	ErrCommerceOrderTransition   = errors.New("invalid commerce order transition")
)

type CommerceCheckoutInput struct {
	OrderID          uuid.UUID
	OrganizationID   uuid.UUID
	CartID           uuid.UUID
	OrderNumber      string
	CheckoutKey      string
	FulfilmentMode   string
	PaymentExpiresAt time.Time
	ActorType        string
	ActorUserID      *uuid.UUID
}

type CommerceOrderTransitionInput struct {
	OrganizationID uuid.UUID
	OrderID        uuid.UUID
	FromStatus     string
	ToStatus       string
	EventType      string
	Reason         string
	IdempotencyKey string
	ActorType      string
	ActorUserID    *uuid.UUID
	Allowed        bool
}

type CommerceOrderListFilter struct {
	StoreID    *uuid.UUID
	CustomerID *uuid.UUID
	Status     *string
	Statuses   []string
	Limit      int
	Offset     int
}

type CommerceOrderRepository interface {
	CheckoutCart(ctx context.Context, input CommerceCheckoutInput) (*models.CommerceOrder, bool, error)
	GetOrder(ctx context.Context, organizationID, orderID uuid.UUID) (*models.CommerceOrder, error)
	GetOrderByNumber(ctx context.Context, organizationID uuid.UUID, orderNumber string) (*models.CommerceOrder, error)
	GetOrderByCheckoutKey(ctx context.Context, organizationID uuid.UUID, checkoutKey string) (*models.CommerceOrder, error)
	SetOrderDestination(ctx context.Context, organizationID, customerID, orderID uuid.UUID, address string, latitude, longitude *float64) (*models.CommerceOrder, error)
	ListOrders(ctx context.Context, organizationID uuid.UUID, assignedUserID *uuid.UUID, filter CommerceOrderListFilter) ([]models.CommerceOrder, int64, error)
	TransitionOrder(ctx context.Context, input CommerceOrderTransitionInput) (*models.CommerceOrder, error)
}

type CommerceOrderRepoPG struct {
	db *gorm.DB
}

type commerceCheckoutCatalogueRow struct {
	ProductID           uuid.UUID
	ProductName         string
	ProductCurrency     string
	VariantID           uuid.UUID
	VariantName         string
	SKU                 string
	Attributes          []byte
	EffectivePriceMinor int64
	PrimaryImageURL     *string
	AvailableQuantity   int
}

func NewCommerceOrderRepoPG(db *gorm.DB) *CommerceOrderRepoPG {
	return &CommerceOrderRepoPG{db: db}
}

func (r *CommerceOrderRepoPG) CheckoutCart(ctx context.Context, input CommerceCheckoutInput) (*models.CommerceOrder, bool, error) {
	created := false
	orderID := input.OrderID
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := getCommerceOrderByCheckoutKey(tx, input.OrganizationID, input.CheckoutKey)
		if err == nil {
			if existing.CartID != input.CartID || existing.FulfilmentMode != input.FulfilmentMode {
				return ErrCommerceConflict
			}
			orderID = existing.ID
			return nil
		}
		if !errors.Is(err, ErrCommerceNotFound) {
			return err
		}

		now := time.Now().UTC()
		var cart models.CommerceCart
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND id = ?", input.OrganizationID, input.CartID).
			First(&cart).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommerceNotFound
		}
		if err != nil {
			return err
		}
		if cart.Status != models.CommerceCartStatusActive || !cart.ExpiresAt.After(now) {
			existing, lookupErr := getCommerceOrderByCheckoutKey(tx, input.OrganizationID, input.CheckoutKey)
			if lookupErr == nil && existing.CartID == input.CartID && existing.FulfilmentMode == input.FulfilmentMode {
				orderID = existing.ID
				return nil
			}
			return ErrCommerceCartInactive
		}

		var customerCount int64
		if err := tx.Model(&models.CommerceCustomer{}).
			Where("organization_id = ? AND id = ? AND status = ?", input.OrganizationID, cart.CustomerID, models.CommerceStatusActive).
			Count(&customerCount).Error; err != nil {
			return err
		}
		if customerCount != 1 {
			return ErrCommerceNotFound
		}

		var fulfilmentCount int64
		if err := tx.Table("commerce_stores stores").
			Joins("JOIN commerce_store_fulfilment_modes modes ON modes.organization_id = stores.organization_id AND modes.store_id = stores.id AND modes.deleted_at IS NULL").
			Where("stores.organization_id = ? AND stores.id = ? AND stores.status = ? AND stores.deleted_at IS NULL", input.OrganizationID, cart.StoreID, models.CommerceStatusActive).
			Where("modes.mode = ? AND modes.enabled = TRUE", input.FulfilmentMode).
			Count(&fulfilmentCount).Error; err != nil {
			return err
		}
		if fulfilmentCount != 1 {
			return ErrCommerceNotFound
		}

		var cartItems []models.CommerceCartItem
		if err := tx.Where("organization_id = ? AND cart_id = ?", input.OrganizationID, cart.ID).
			Order("variant_id ASC").
			Find(&cartItems).Error; err != nil {
			return err
		}
		if len(cartItems) == 0 {
			return ErrCommerceCheckoutEmptyCart
		}
		sort.Slice(cartItems, func(i, j int) bool { return cartItems[i].VariantID.String() < cartItems[j].VariantID.String() })

		order := &models.CommerceOrder{
			ID:               input.OrderID,
			OrganizationID:   input.OrganizationID,
			CartID:           cart.ID,
			CustomerID:       cart.CustomerID,
			StoreID:          cart.StoreID,
			OrderNumber:      input.OrderNumber,
			CheckoutKey:      input.CheckoutKey,
			FulfilmentMode:   input.FulfilmentMode,
			Status:           models.CommerceOrderStatusPendingPayment,
			Currency:         cart.Currency,
			Version:          1,
			PaymentExpiresAt: input.PaymentExpiresAt.UTC(),
			Items:            make([]models.CommerceOrderItem, 0, len(cartItems)),
		}

		for _, cartItem := range cartItems {
			if err := expireCheckoutReservations(tx, input.OrganizationID, cart.StoreID, cartItem.VariantID, now); err != nil {
				return err
			}
			entry, err := lockCheckoutCatalogueEntry(tx, input.OrganizationID, cart.StoreID, cartItem.VariantID)
			if err != nil {
				return err
			}
			if entry.AvailableQuantity < cartItem.Quantity {
				return ErrCommerceInventoryUnavailable
			}
			if entry.ProductCurrency != cart.Currency {
				return ErrCommerceConflict
			}
			orderItem, err := buildCommerceOrderItem(input.OrganizationID, input.OrderID, cartItem, entry)
			if err != nil {
				return err
			}
			if order.SubtotalMinor > math.MaxInt64-orderItem.LineTotalMinor {
				return ErrCommerceConflict
			}
			order.SubtotalMinor += orderItem.LineTotalMinor
			order.Items = append(order.Items, orderItem)
		}
		order.TotalMinor = order.SubtotalMinor
		if err := tx.Omit("Items", "Events").Create(order).Error; err != nil {
			return err
		}

		for index := range order.Items {
			item := &order.Items[index]
			reservation := models.CommerceInventoryReservation{
				ID:             uuid.New(),
				OrganizationID: input.OrganizationID,
				StoreID:        cart.StoreID,
				VariantID:      item.VariantID,
				ReservationKey: commerceOrderReservationKey(order.ID, item.VariantID),
				Quantity:       item.Quantity,
				Status:         models.InventoryReservationActive,
				ExpiresAt:      input.PaymentExpiresAt.UTC(),
			}
			result := tx.Model(&models.CommerceInventoryLevel{}).
				Where("organization_id = ? AND store_id = ? AND variant_id = ? AND quantity_on_hand - quantity_reserved >= ?", input.OrganizationID, cart.StoreID, item.VariantID, item.Quantity).
				Updates(map[string]interface{}{
					"quantity_reserved": gorm.Expr("quantity_reserved + ?", item.Quantity),
					"version":           gorm.Expr("version + 1"),
					"updated_at":        now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrCommerceInventoryUnavailable
			}
			if err := tx.Create(&reservation).Error; err != nil {
				return err
			}
			movement := models.CommerceInventoryMovement{
				ID:                    uuid.New(),
				OrganizationID:        input.OrganizationID,
				StoreID:               cart.StoreID,
				VariantID:             item.VariantID,
				ReservationID:         &reservation.ID,
				MovementType:          models.InventoryMovementReservation,
				QuantityReservedDelta: item.Quantity,
				Reference:             commerceOrderReservationMovementReference(reservation.ID, "reserve"),
				Reason:                "inventory reserved for order checkout",
				CreatedByUserID:       input.ActorUserID,
			}
			if err := tx.Create(&movement).Error; err != nil {
				return err
			}
			item.InventoryReservationID = reservation.ID
		}
		if err := tx.Create(&order.Items).Error; err != nil {
			return err
		}

		event := models.CommerceOrderEvent{
			ID:             uuid.New(),
			OrganizationID: input.OrganizationID,
			OrderID:        order.ID,
			EventType:      models.CommerceOrderEventCreated,
			ToStatus:       models.CommerceOrderStatusPendingPayment,
			ActorType:      input.ActorType,
			ActorUserID:    input.ActorUserID,
			Reason:         "order created from authoritative cart checkout",
			Metadata:       json.RawMessage(`{}`),
			IdempotencyKey: "checkout:" + input.CheckoutKey,
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		order.Events = []models.CommerceOrderEvent{event}

		result := tx.Model(&models.CommerceCart{}).
			Where("organization_id = ? AND id = ? AND status = ?", input.OrganizationID, cart.ID, models.CommerceCartStatusActive).
			Updates(map[string]interface{}{
				"status":     models.CommerceCartStatusConverted,
				"version":    gorm.Expr("version + 1"),
				"updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCommerceCartInactive
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, false, mapCommerceOrderWriteError("checkout commerce cart", err)
	}
	order, err := r.GetOrder(ctx, input.OrganizationID, orderID)
	return order, created, err
}

func (r *CommerceOrderRepoPG) GetOrder(ctx context.Context, organizationID, orderID uuid.UUID) (*models.CommerceOrder, error) {
	var order models.CommerceOrder
	err := commerceOrderQuery(r.db.WithContext(ctx)).
		Where("commerce_orders.organization_id = ? AND commerce_orders.id = ?", organizationID, orderID).
		First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get commerce order: %w", err)
	}
	return &order, nil
}

func (r *CommerceOrderRepoPG) GetOrderByNumber(ctx context.Context, organizationID uuid.UUID, orderNumber string) (*models.CommerceOrder, error) {
	var order models.CommerceOrder
	err := commerceOrderQuery(r.db.WithContext(ctx)).
		Where("commerce_orders.organization_id = ? AND UPPER(commerce_orders.order_number) = UPPER(?)", organizationID, strings.TrimSpace(orderNumber)).
		First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get commerce order by number: %w", err)
	}
	return &order, nil
}

func (r *CommerceOrderRepoPG) SetOrderDestination(ctx context.Context, organizationID, customerID, orderID uuid.UUID, address string, latitude, longitude *float64) (*models.CommerceOrder, error) {
	result := r.db.WithContext(ctx).Model(&models.CommerceOrder{}).
		Where("organization_id = ? AND customer_id = ? AND id = ? AND status = ?", organizationID, customerID, orderID, models.CommerceOrderStatusPendingPayment).
		Updates(map[string]interface{}{"destination_address": strings.TrimSpace(address), "destination_latitude": latitude, "destination_longitude": longitude, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrCommerceNotFound
	}
	return r.GetOrder(ctx, organizationID, orderID)
}

func (r *CommerceOrderRepoPG) GetOrderByCheckoutKey(ctx context.Context, organizationID uuid.UUID, checkoutKey string) (*models.CommerceOrder, error) {
	var order models.CommerceOrder
	err := commerceOrderQuery(r.db.WithContext(ctx)).
		Where("commerce_orders.organization_id = ? AND commerce_orders.checkout_key = ?", organizationID, checkoutKey).
		First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get commerce order by checkout key: %w", err)
	}
	return &order, nil
}

func (r *CommerceOrderRepoPG) ListOrders(ctx context.Context, organizationID uuid.UUID, assignedUserID *uuid.UUID, filter CommerceOrderListFilter) ([]models.CommerceOrder, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.CommerceOrder{}).
		Where("commerce_orders.organization_id = ?", organizationID)
	if assignedUserID != nil {
		query = query.Joins(`
			JOIN commerce_staff_store_assignments assignments
			  ON assignments.organization_id = commerce_orders.organization_id
			 AND assignments.store_id = commerce_orders.store_id
			 AND assignments.user_id = ?
			 AND assignments.status = ?
			 AND assignments.deleted_at IS NULL
		`, *assignedUserID, models.CommerceStatusActive)
	}
	if filter.StoreID != nil {
		query = query.Where("commerce_orders.store_id = ?", *filter.StoreID)
	}
	if filter.CustomerID != nil {
		query = query.Where("commerce_orders.customer_id = ?", *filter.CustomerID)
	}
	if filter.Status != nil {
		query = query.Where("commerce_orders.status = ?", *filter.Status)
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("commerce_orders.status IN ?", filter.Statuses)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count commerce orders: %w", err)
	}

	var orders []models.CommerceOrder
	err := query.
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
		Preload("Events", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
		Order("commerce_orders.created_at DESC").
		Limit(filter.Limit).
		Offset(filter.Offset).
		Find(&orders).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list commerce orders: %w", err)
	}
	return orders, total, nil
}

func (r *CommerceOrderRepoPG) TransitionOrder(ctx context.Context, input CommerceOrderTransitionInput) (*models.CommerceOrder, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existingEvent models.CommerceOrderEvent
		err := tx.Where("organization_id = ? AND order_id = ? AND idempotency_key = ?", input.OrganizationID, input.OrderID, input.IdempotencyKey).
			First(&existingEvent).Error
		if err == nil {
			if existingEvent.ToStatus != input.ToStatus {
				return ErrCommerceConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !input.Allowed || input.FromStatus == input.ToStatus {
			return ErrCommerceOrderTransition
		}

		var order models.CommerceOrder
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND id = ?", input.OrganizationID, input.OrderID).
			First(&order).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommerceNotFound
		}
		if err != nil {
			return err
		}
		if order.Status != input.FromStatus {
			var concurrentEvent models.CommerceOrderEvent
			eventErr := tx.Where("organization_id = ? AND order_id = ? AND idempotency_key = ?", input.OrganizationID, input.OrderID, input.IdempotencyKey).
				First(&concurrentEvent).Error
			if eventErr == nil {
				if concurrentEvent.ToStatus == input.ToStatus {
					return nil
				}
				return ErrCommerceConflict
			}
			if !errors.Is(eventErr, gorm.ErrRecordNotFound) {
				return eventErr
			}
			return ErrCommerceOrderTransition
		}
		if input.ToStatus == models.CommerceOrderStatusCancelled {
			if err := releaseCommerceOrderReservations(tx, &order); err != nil {
				return err
			}
		}

		now := time.Now().UTC()
		result := tx.Model(&models.CommerceOrder{}).
			Where("organization_id = ? AND id = ? AND status = ?", input.OrganizationID, input.OrderID, input.FromStatus).
			Updates(map[string]interface{}{
				"status":     input.ToStatus,
				"version":    gorm.Expr("version + 1"),
				"updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCommerceOrderTransition
		}

		event := models.CommerceOrderEvent{
			ID:             uuid.New(),
			OrganizationID: input.OrganizationID,
			OrderID:        input.OrderID,
			EventType:      input.EventType,
			FromStatus:     &input.FromStatus,
			ToStatus:       input.ToStatus,
			ActorType:      input.ActorType,
			ActorUserID:    input.ActorUserID,
			Reason:         input.Reason,
			Metadata:       json.RawMessage(`{}`),
			IdempotencyKey: input.IdempotencyKey,
		}
		return tx.Create(&event).Error
	})
	if err != nil {
		return nil, mapCommerceOrderWriteError("transition commerce order", err)
	}
	return r.GetOrder(ctx, input.OrganizationID, input.OrderID)
}

func lockCheckoutCatalogueEntry(tx *gorm.DB, organizationID, storeID, variantID uuid.UUID) (*commerceCheckoutCatalogueRow, error) {
	var entry commerceCheckoutCatalogueRow
	result := tx.Raw(`
		SELECT products.id AS product_id,
		       products.name AS product_name,
		       products.currency AS product_currency,
		       variants.id AS variant_id,
		       variants.name AS variant_name,
		       variants.sku,
		       variants.attributes,
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
		       inventory.quantity_on_hand - inventory.quantity_reserved AS available_quantity
		FROM commerce_store_catalogue_items items
		JOIN commerce_product_variants variants
		  ON variants.organization_id = items.organization_id
		 AND variants.id = items.variant_id
		 AND variants.status = 'active'
		 AND variants.deleted_at IS NULL
		JOIN commerce_products products
		  ON products.organization_id = variants.organization_id
		 AND products.id = variants.product_id
		 AND products.status = 'active'
		 AND products.deleted_at IS NULL
		JOIN commerce_categories categories
		  ON categories.organization_id = products.organization_id
		 AND categories.id = products.category_id
		 AND categories.status = 'active'
		 AND categories.deleted_at IS NULL
		JOIN commerce_inventory_levels inventory
		  ON inventory.organization_id = items.organization_id
		 AND inventory.store_id = items.store_id
		 AND inventory.variant_id = items.variant_id
		WHERE items.organization_id = ?
		  AND items.store_id = ?
		  AND items.variant_id = ?
		  AND items.enabled = TRUE
		  AND items.deleted_at IS NULL
		FOR UPDATE OF inventory, items, variants, products, categories
	`, organizationID, storeID, variantID).Scan(&entry)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrCommerceInventoryUnavailable
	}
	return &entry, nil
}

func buildCommerceOrderItem(organizationID, orderID uuid.UUID, cartItem models.CommerceCartItem, entry *commerceCheckoutCatalogueRow) (models.CommerceOrderItem, error) {
	if cartItem.Quantity < 1 || cartItem.Quantity > commerceCartMaxQuantity || entry.EffectivePriceMinor < 0 || entry.EffectivePriceMinor > math.MaxInt64/int64(cartItem.Quantity) {
		return models.CommerceOrderItem{}, ErrCommerceConflict
	}
	return models.CommerceOrderItem{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		OrderID:         orderID,
		ProductID:       entry.ProductID,
		VariantID:       entry.VariantID,
		ProductName:     entry.ProductName,
		VariantName:     entry.VariantName,
		SKU:             entry.SKU,
		Attributes:      json.RawMessage(entry.Attributes),
		PrimaryImageURL: entry.PrimaryImageURL,
		Quantity:        cartItem.Quantity,
		UnitPriceMinor:  entry.EffectivePriceMinor,
		LineTotalMinor:  entry.EffectivePriceMinor * int64(cartItem.Quantity),
	}, nil
}

func expireCheckoutReservations(tx *gorm.DB, organizationID, storeID, variantID uuid.UUID, now time.Time) error {
	var reservations []models.CommerceInventoryReservation
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("organization_id = ? AND store_id = ? AND variant_id = ? AND status = ? AND expires_at <= ?", organizationID, storeID, variantID, models.InventoryReservationActive, now).
		Order("expires_at ASC").
		Find(&reservations).Error
	if err != nil {
		return err
	}
	for index := range reservations {
		if err := releaseCommerceInventoryReservation(tx, &reservations[index], models.InventoryReservationExpired, now); err != nil {
			return err
		}
	}
	return nil
}

func releaseCommerceOrderReservations(tx *gorm.DB, order *models.CommerceOrder) error {
	var reservations []models.CommerceInventoryReservation
	err := tx.Table("commerce_inventory_reservations reservations").
		Select("reservations.*").
		Joins("JOIN commerce_order_items items ON items.organization_id = reservations.organization_id AND items.inventory_reservation_id = reservations.id").
		Where("items.organization_id = ? AND items.order_id = ?", order.OrganizationID, order.ID).
		Order("reservations.variant_id ASC").
		Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "reservations"}}).
		Find(&reservations).Error
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for index := range reservations {
		switch reservations[index].Status {
		case models.InventoryReservationActive:
			if err := releaseCommerceInventoryReservation(tx, &reservations[index], models.InventoryReservationReleased, now); err != nil {
				return err
			}
		case models.InventoryReservationReleased, models.InventoryReservationExpired:
			continue
		default:
			return ErrCommerceOrderTransition
		}
	}
	return nil
}

func releaseCommerceInventoryReservation(tx *gorm.DB, reservation *models.CommerceInventoryReservation, targetStatus string, now time.Time) error {
	result := tx.Model(&models.CommerceInventoryLevel{}).
		Where("organization_id = ? AND store_id = ? AND variant_id = ? AND quantity_reserved >= ?", reservation.OrganizationID, reservation.StoreID, reservation.VariantID, reservation.Quantity).
		Updates(map[string]interface{}{
			"quantity_reserved": gorm.Expr("quantity_reserved - ?", reservation.Quantity),
			"version":           gorm.Expr("version + 1"),
			"updated_at":        now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCommerceInventoryUnavailable
	}
	if err := tx.Model(reservation).Updates(map[string]interface{}{
		"status":      targetStatus,
		"released_at": now,
		"updated_at":  now,
	}).Error; err != nil {
		return err
	}
	movement := models.CommerceInventoryMovement{
		ID:                    uuid.New(),
		OrganizationID:        reservation.OrganizationID,
		StoreID:               reservation.StoreID,
		VariantID:             reservation.VariantID,
		ReservationID:         &reservation.ID,
		MovementType:          models.InventoryMovementReservationRelease,
		QuantityReservedDelta: -reservation.Quantity,
		Reference:             commerceOrderReservationMovementReference(reservation.ID, targetStatus),
		Reason:                "inventory reservation " + targetStatus,
	}
	return tx.Create(&movement).Error
}

func getCommerceOrderByCheckoutKey(tx *gorm.DB, organizationID uuid.UUID, checkoutKey string) (*models.CommerceOrder, error) {
	var order models.CommerceOrder
	err := tx.Where("organization_id = ? AND checkout_key = ?", organizationID, checkoutKey).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func commerceOrderQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&models.CommerceOrder{}).
		Preload("Items", func(query *gorm.DB) *gorm.DB { return query.Order("created_at ASC") }).
		Preload("Events", func(query *gorm.DB) *gorm.DB { return query.Order("created_at ASC") })
}

func commerceOrderReservationKey(orderID, variantID uuid.UUID) string {
	return "order:" + orderID.String() + ":variant:" + variantID.String()
}

func commerceOrderReservationMovementReference(reservationID uuid.UUID, transition string) string {
	return "inventory-reservation:" + reservationID.String() + ":" + transition
}

func mapCommerceOrderWriteError(action string, err error) error {
	if errors.Is(err, ErrCommerceNotFound) || errors.Is(err, ErrCommerceConflict) ||
		errors.Is(err, ErrCommerceCartInactive) || errors.Is(err, ErrCommerceCheckoutEmptyCart) ||
		errors.Is(err, ErrCommerceInventoryUnavailable) || errors.Is(err, ErrCommerceOrderTransition) {
		return err
	}
	return mapCommerceWriteError(action, err)
}
