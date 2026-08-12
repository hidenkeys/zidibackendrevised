package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrCommerceCartInactive = errors.New("commerce cart is not active")

const commerceCartMaxQuantity = 100

type CommerceCustomerRepository interface {
	ResolveCustomerIdentity(ctx context.Context, customer *models.CommerceCustomer, identity *models.CommerceCustomerIdentity) (*models.CommerceCustomer, bool, error)
	GetCustomer(ctx context.Context, organizationID, customerID uuid.UUID) (*models.CommerceCustomer, error)
	UpdateCustomerProfile(ctx context.Context, organizationID, customerID uuid.UUID, displayName string, email *string) (*models.CommerceCustomer, error)
}

type CommerceCartRepository interface {
	GetOrCreateActiveCart(ctx context.Context, cart *models.CommerceCart) (*models.CommerceCart, bool, error)
	GetActiveCart(ctx context.Context, organizationID, cartID uuid.UUID) (*models.CommerceCart, error)
	SetCartItem(ctx context.Context, organizationID, cartID, variantID uuid.UUID, quantity int) (*models.CommerceCart, error)
	DeleteCartItem(ctx context.Context, organizationID, cartID, variantID uuid.UUID) (*models.CommerceCart, error)
	ClearCart(ctx context.Context, organizationID, cartID uuid.UUID) (*models.CommerceCart, error)
}

type CommerceCustomerCartRepoPG struct {
	db *gorm.DB
}

func NewCommerceCustomerCartRepoPG(db *gorm.DB) *CommerceCustomerCartRepoPG {
	return &CommerceCustomerCartRepoPG{db: db}
}

func (r *CommerceCustomerCartRepoPG) ResolveCustomerIdentity(ctx context.Context, customer *models.CommerceCustomer, identity *models.CommerceCustomerIdentity) (*models.CommerceCustomer, bool, error) {
	var resolved *models.CommerceCustomer
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := findCommerceCustomerByIdentity(tx, identity.OrganizationID, identity.Channel, identity.NormalizedIdentifier)
		if err == nil {
			if identity.VerifiedAt != nil {
				if err := tx.Model(&models.CommerceCustomerIdentity{}).
					Where("organization_id = ? AND channel = ? AND normalized_identifier = ? AND verified_at IS NULL", identity.OrganizationID, identity.Channel, identity.NormalizedIdentifier).
					Updates(map[string]interface{}{"verified_at": *identity.VerifiedAt, "updated_at": time.Now().UTC()}).Error; err != nil {
					return err
				}
				existing, err = findCommerceCustomerByIdentity(tx, identity.OrganizationID, identity.Channel, identity.NormalizedIdentifier)
				if err != nil {
					return err
				}
			}
			resolved = existing
			return nil
		}
		if !errors.Is(err, ErrCommerceNotFound) {
			return err
		}

		if err := tx.Omit("Identities").Create(customer).Error; err != nil {
			return err
		}
		identity.CustomerID = customer.ID
		if err := tx.Create(identity).Error; err != nil {
			return err
		}
		resolved = customer
		resolved.Identities = []models.CommerceCustomerIdentity{*identity}
		created = true
		return nil
	})
	if err == nil {
		return resolved, created, nil
	}

	mapped := mapCommerceWriteError("resolve commerce customer identity", err)
	if errors.Is(mapped, ErrCommerceConflict) {
		existing, lookupErr := findCommerceCustomerByIdentity(r.db.WithContext(ctx), identity.OrganizationID, identity.Channel, identity.NormalizedIdentifier)
		if lookupErr == nil {
			return existing, false, nil
		}
	}
	return nil, false, mapped
}

func (r *CommerceCustomerCartRepoPG) GetCustomer(ctx context.Context, organizationID, customerID uuid.UUID) (*models.CommerceCustomer, error) {
	var customer models.CommerceCustomer
	err := r.db.WithContext(ctx).
		Preload("Identities", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
		Where("organization_id = ? AND id = ?", organizationID, customerID).
		First(&customer).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get commerce customer: %w", err)
	}
	return &customer, nil
}

func (r *CommerceCustomerCartRepoPG) UpdateCustomerProfile(ctx context.Context, organizationID, customerID uuid.UUID, displayName string, email *string) (*models.CommerceCustomer, error) {
	updates := map[string]interface{}{"updated_at": time.Now().UTC()}
	if value := strings.TrimSpace(displayName); value != "" {
		updates["display_name"] = value
	}
	if email != nil {
		updates["email"] = *email
	}
	result := r.db.WithContext(ctx).Model(&models.CommerceCustomer{}).
		Where("organization_id = ? AND id = ? AND deleted_at IS NULL", organizationID, customerID).
		Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("update commerce customer profile: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return nil, ErrCommerceNotFound
	}
	return r.GetCustomer(ctx, organizationID, customerID)
}

func (r *CommerceCustomerCartRepoPG) GetOrCreateActiveCart(ctx context.Context, cart *models.CommerceCart) (*models.CommerceCart, bool, error) {
	var resolved *models.CommerceCart
	created := false
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.CommerceCart{}).
			Where("organization_id = ? AND customer_id = ? AND store_id = ? AND status = ? AND expires_at <= ?", cart.OrganizationID, cart.CustomerID, cart.StoreID, models.CommerceCartStatusActive, now).
			Updates(map[string]interface{}{"status": models.CommerceCartStatusExpired, "updated_at": now, "version": gorm.Expr("version + 1")}).Error; err != nil {
			return err
		}

		var existing models.CommerceCart
		err := tx.Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
			Where("organization_id = ? AND customer_id = ? AND store_id = ? AND status = ? AND expires_at > ?", cart.OrganizationID, cart.CustomerID, cart.StoreID, models.CommerceCartStatusActive, now).
			First(&existing).Error
		if err == nil {
			resolved = &existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Omit("Items").Create(cart).Error; err != nil {
			return err
		}
		resolved = cart
		resolved.Items = []models.CommerceCartItem{}
		created = true
		return nil
	})
	if err == nil {
		return resolved, created, nil
	}

	mapped := mapCommerceWriteError("create commerce cart", err)
	if errors.Is(mapped, ErrCommerceConflict) {
		existing, lookupErr := r.getActiveCartForCustomerStore(ctx, cart.OrganizationID, cart.CustomerID, cart.StoreID, now)
		if lookupErr == nil {
			return existing, false, nil
		}
	}
	return nil, false, mapped
}

func (r *CommerceCustomerCartRepoPG) GetActiveCart(ctx context.Context, organizationID, cartID uuid.UUID) (*models.CommerceCart, error) {
	return r.getActiveCart(ctx, organizationID, cartID, time.Now().UTC())
}

func (r *CommerceCustomerCartRepoPG) SetCartItem(ctx context.Context, organizationID, cartID, variantID uuid.UUID, quantity int) (*models.CommerceCart, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockActiveCommerceCart(tx, organizationID, cartID, now); err != nil {
			return err
		}
		item := models.CommerceCartItem{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			CartID:         cartID,
			VariantID:      variantID,
			Quantity:       quantity,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "organization_id"}, {Name: "cart_id"}, {Name: "variant_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"quantity": quantity, "updated_at": now}),
		}).Create(&item).Error; err != nil {
			return err
		}
		return touchCommerceCart(tx, organizationID, cartID, now)
	})
	if err != nil {
		return nil, mapCartWriteError("set commerce cart item", err)
	}
	return r.getActiveCart(ctx, organizationID, cartID, now)
}

func (r *CommerceCustomerCartRepoPG) DeleteCartItem(ctx context.Context, organizationID, cartID, variantID uuid.UUID) (*models.CommerceCart, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockActiveCommerceCart(tx, organizationID, cartID, now); err != nil {
			return err
		}
		if err := tx.Where("organization_id = ? AND cart_id = ? AND variant_id = ?", organizationID, cartID, variantID).
			Delete(&models.CommerceCartItem{}).Error; err != nil {
			return err
		}
		return touchCommerceCart(tx, organizationID, cartID, now)
	})
	if err != nil {
		return nil, mapCartWriteError("delete commerce cart item", err)
	}
	return r.getActiveCart(ctx, organizationID, cartID, now)
}

func (r *CommerceCustomerCartRepoPG) ClearCart(ctx context.Context, organizationID, cartID uuid.UUID) (*models.CommerceCart, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockActiveCommerceCart(tx, organizationID, cartID, now); err != nil {
			return err
		}
		if err := tx.Where("organization_id = ? AND cart_id = ?", organizationID, cartID).
			Delete(&models.CommerceCartItem{}).Error; err != nil {
			return err
		}
		return touchCommerceCart(tx, organizationID, cartID, now)
	})
	if err != nil {
		return nil, mapCartWriteError("clear commerce cart", err)
	}
	return r.getActiveCart(ctx, organizationID, cartID, now)
}

func findCommerceCustomerByIdentity(db *gorm.DB, organizationID uuid.UUID, channel, normalizedIdentifier string) (*models.CommerceCustomer, error) {
	var identity models.CommerceCustomerIdentity
	err := db.Where("organization_id = ? AND channel = ? AND normalized_identifier = ?", organizationID, channel, normalizedIdentifier).
		First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find commerce customer identity: %w", err)
	}

	var customer models.CommerceCustomer
	err = db.Preload("Identities", func(query *gorm.DB) *gorm.DB { return query.Order("created_at ASC") }).
		Where("organization_id = ? AND id = ?", organizationID, identity.CustomerID).
		First(&customer).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get commerce customer for identity: %w", err)
	}
	return &customer, nil
}

func (r *CommerceCustomerCartRepoPG) getActiveCartForCustomerStore(ctx context.Context, organizationID, customerID, storeID uuid.UUID, now time.Time) (*models.CommerceCart, error) {
	var cart models.CommerceCart
	err := r.db.WithContext(ctx).
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
		Where("organization_id = ? AND customer_id = ? AND store_id = ? AND status = ? AND expires_at > ?", organizationID, customerID, storeID, models.CommerceCartStatusActive, now).
		First(&cart).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get active commerce cart: %w", err)
	}
	return &cart, nil
}

func (r *CommerceCustomerCartRepoPG) getActiveCart(ctx context.Context, organizationID, cartID uuid.UUID, now time.Time) (*models.CommerceCart, error) {
	var cart models.CommerceCart
	err := r.db.WithContext(ctx).
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
		Where("organization_id = ? AND id = ? AND status = ? AND expires_at > ?", organizationID, cartID, models.CommerceCartStatusActive, now).
		First(&cart).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get active commerce cart: %w", err)
	}
	return &cart, nil
}

func lockActiveCommerceCart(tx *gorm.DB, organizationID, cartID uuid.UUID, now time.Time) error {
	var cart models.CommerceCart
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("organization_id = ? AND id = ?", organizationID, cartID).
		First(&cart).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrCommerceNotFound
	}
	if err != nil {
		return err
	}
	if cart.Status != models.CommerceCartStatusActive || !cart.ExpiresAt.After(now) {
		return ErrCommerceCartInactive
	}
	return nil
}

func touchCommerceCart(tx *gorm.DB, organizationID, cartID uuid.UUID, now time.Time) error {
	result := tx.Model(&models.CommerceCart{}).
		Where("organization_id = ? AND id = ?", organizationID, cartID).
		Updates(map[string]interface{}{"updated_at": now, "version": gorm.Expr("version + 1")})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCommerceNotFound
	}
	return nil
}

func mapCartWriteError(action string, err error) error {
	if errors.Is(err, ErrCommerceNotFound) || errors.Is(err, ErrCommerceCartInactive) {
		return err
	}
	return mapCommerceWriteError(action, err)
}
