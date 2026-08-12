package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	InventoryReservationActive    = "active"
	InventoryReservationCommitted = "committed"
	InventoryReservationReleased  = "released"
	InventoryReservationExpired   = "expired"

	InventoryMovementAdjustment         = "adjustment"
	InventoryMovementReservation        = "reservation"
	InventoryMovementReservationCommit  = "reservation_commit"
	InventoryMovementReservationRelease = "reservation_release"
)

type CommerceCategory struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null;index"`
	Name           string         `gorm:"not null"`
	Slug           string         `gorm:"not null"`
	Description    string         `gorm:"not null"`
	SortOrder      int            `gorm:"not null"`
	Status         string         `gorm:"not null"`
	CreatedAt      time.Time      `gorm:"not null"`
	UpdatedAt      time.Time      `gorm:"not null"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (CommerceCategory) TableName() string {
	return "commerce_categories"
}

type CommerceProduct struct {
	ID             uuid.UUID                `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID                `gorm:"type:uuid;not null;index"`
	CategoryID     uuid.UUID                `gorm:"type:uuid;not null;index"`
	Name           string                   `gorm:"not null"`
	Slug           string                   `gorm:"not null"`
	Description    string                   `gorm:"not null"`
	Currency       string                   `gorm:"type:char(3);not null"`
	Status         string                   `gorm:"not null"`
	CreatedAt      time.Time                `gorm:"not null"`
	UpdatedAt      time.Time                `gorm:"not null"`
	DeletedAt      gorm.DeletedAt           `gorm:"index"`
	Variants       []CommerceProductVariant `gorm:"foreignKey:ProductID"`
	Images         []CommerceProductImage   `gorm:"foreignKey:ProductID"`
}

func (CommerceProduct) TableName() string {
	return "commerce_products"
}

type CommerceProductVariant struct {
	ID             uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID       `gorm:"type:uuid;not null;index"`
	ProductID      uuid.UUID       `gorm:"type:uuid;not null;index"`
	Name           string          `gorm:"not null"`
	SKU            string          `gorm:"not null"`
	PriceMinor     int64           `gorm:"not null"`
	Attributes     json.RawMessage `gorm:"type:jsonb;not null"`
	IsDefault      bool            `gorm:"not null"`
	Status         string          `gorm:"not null"`
	CreatedAt      time.Time       `gorm:"not null"`
	UpdatedAt      time.Time       `gorm:"not null"`
	DeletedAt      gorm.DeletedAt  `gorm:"index"`
}

func (CommerceProductVariant) TableName() string {
	return "commerce_product_variants"
}

type CommerceProductImage struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null;index"`
	ProductID      uuid.UUID      `gorm:"type:uuid;not null;index"`
	URL            string         `gorm:"not null"`
	AltText        string         `gorm:"not null"`
	SortOrder      int            `gorm:"not null"`
	CreatedAt      time.Time      `gorm:"not null"`
	UpdatedAt      time.Time      `gorm:"not null"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (CommerceProductImage) TableName() string {
	return "commerce_product_images"
}

type CommerceStoreCatalogueItem struct {
	ID                 uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID     uuid.UUID      `gorm:"type:uuid;not null;index"`
	StoreID            uuid.UUID      `gorm:"type:uuid;not null;index"`
	VariantID          uuid.UUID      `gorm:"type:uuid;not null;index"`
	Enabled            bool           `gorm:"not null"`
	PriceOverrideMinor *int64         `gorm:"type:bigint"`
	CreatedAt          time.Time      `gorm:"not null"`
	UpdatedAt          time.Time      `gorm:"not null"`
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

func (CommerceStoreCatalogueItem) TableName() string {
	return "commerce_store_catalogue_items"
}

type CommerceInventoryLevel struct {
	ID               uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID   uuid.UUID `gorm:"type:uuid;not null;index"`
	StoreID          uuid.UUID `gorm:"type:uuid;not null;index"`
	VariantID        uuid.UUID `gorm:"type:uuid;not null;index"`
	QuantityOnHand   int       `gorm:"not null"`
	QuantityReserved int       `gorm:"not null"`
	ReorderThreshold int       `gorm:"not null"`
	Version          int64     `gorm:"not null"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (CommerceInventoryLevel) TableName() string {
	return "commerce_inventory_levels"
}

func (level CommerceInventoryLevel) AvailableQuantity() int {
	return level.QuantityOnHand - level.QuantityReserved
}

type CommerceInventoryReservation struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index"`
	StoreID        uuid.UUID `gorm:"type:uuid;not null;index"`
	VariantID      uuid.UUID `gorm:"type:uuid;not null;index"`
	ReservationKey string    `gorm:"not null"`
	Quantity       int       `gorm:"not null"`
	Status         string    `gorm:"not null"`
	ExpiresAt      time.Time `gorm:"not null;index"`
	CommittedAt    *time.Time
	ReleasedAt     *time.Time
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (CommerceInventoryReservation) TableName() string {
	return "commerce_inventory_reservations"
}

type CommerceInventoryMovement struct {
	ID                    uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID        uuid.UUID `gorm:"type:uuid;not null;index"`
	StoreID               uuid.UUID `gorm:"type:uuid;not null;index"`
	VariantID             uuid.UUID `gorm:"type:uuid;not null;index"`
	ReservationID         *uuid.UUID
	MovementType          string `gorm:"not null"`
	QuantityOnHandDelta   int    `gorm:"not null"`
	QuantityReservedDelta int    `gorm:"not null"`
	Reference             string `gorm:"not null"`
	Reason                string `gorm:"not null"`
	CreatedByUserID       *uuid.UUID
	CreatedAt             time.Time `gorm:"not null"`
}

func (CommerceInventoryMovement) TableName() string {
	return "commerce_inventory_movements"
}
