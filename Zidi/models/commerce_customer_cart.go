package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	CommerceIdentityChannelWhatsApp = "whatsapp"
	CommerceIdentityChannelPhone    = "phone"
	CommerceIdentityChannelEmail    = "email"
	CommerceIdentityChannelWeb      = "web"

	CommerceCartStatusActive    = "active"
	CommerceCartStatusConverted = "converted"
	CommerceCartStatusAbandoned = "abandoned"
	CommerceCartStatusExpired   = "expired"
)

type CommerceCustomer struct {
	ID             uuid.UUID                  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID                  `gorm:"type:uuid;not null;index"`
	DisplayName    string                     `gorm:"not null"`
	Email          *string                    `gorm:"type:text"`
	Status         string                     `gorm:"not null"`
	CreatedAt      time.Time                  `gorm:"not null"`
	UpdatedAt      time.Time                  `gorm:"not null"`
	DeletedAt      gorm.DeletedAt             `gorm:"index"`
	Identities     []CommerceCustomerIdentity `gorm:"foreignKey:CustomerID"`
}

func (CommerceCustomer) TableName() string {
	return "commerce_customers"
}

type CommerceCustomerIdentity struct {
	ID                   uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID       uuid.UUID      `gorm:"type:uuid;not null;index"`
	CustomerID           uuid.UUID      `gorm:"type:uuid;not null;index"`
	Channel              string         `gorm:"not null"`
	NormalizedIdentifier string         `gorm:"not null"`
	DisplayIdentifier    string         `gorm:"not null"`
	VerifiedAt           *time.Time     `gorm:"type:timestamptz"`
	CreatedAt            time.Time      `gorm:"not null"`
	UpdatedAt            time.Time      `gorm:"not null"`
	DeletedAt            gorm.DeletedAt `gorm:"index"`
}

func (CommerceCustomerIdentity) TableName() string {
	return "commerce_customer_identities"
}

type CommerceCart struct {
	ID             uuid.UUID          `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID          `gorm:"type:uuid;not null;index"`
	CustomerID     uuid.UUID          `gorm:"type:uuid;not null;index"`
	StoreID        uuid.UUID          `gorm:"type:uuid;not null;index"`
	Currency       string             `gorm:"type:char(3);not null"`
	Status         string             `gorm:"not null"`
	Version        int64              `gorm:"not null"`
	ExpiresAt      time.Time          `gorm:"not null;index"`
	CreatedAt      time.Time          `gorm:"not null"`
	UpdatedAt      time.Time          `gorm:"not null"`
	Items          []CommerceCartItem `gorm:"foreignKey:CartID"`
}

func (CommerceCart) TableName() string {
	return "commerce_carts"
}

type CommerceCartItem struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index"`
	CartID         uuid.UUID `gorm:"type:uuid;not null;index"`
	VariantID      uuid.UUID `gorm:"type:uuid;not null;index"`
	Quantity       int       `gorm:"not null"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (CommerceCartItem) TableName() string {
	return "commerce_cart_items"
}
