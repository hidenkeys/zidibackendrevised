package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	CommerceStatusActive   = "active"
	CommerceStatusInactive = "inactive"

	FulfilmentModeCustomerPickup = "customer_pickup"
	FulfilmentModeCustomerRider  = "customer_rider"
	FulfilmentModeMerchantRider  = "merchant_rider"
)

type CommerceMerchantProfile struct {
	ID              uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID  uuid.UUID      `gorm:"type:uuid;not null;index"`
	Slug            string         `gorm:"not null"`
	DisplayName     string         `gorm:"not null"`
	DefaultCurrency string         `gorm:"type:char(3);not null"`
	Timezone        string         `gorm:"not null"`
	Status          string         `gorm:"not null"`
	CreatedAt       time.Time      `gorm:"not null"`
	UpdatedAt       time.Time      `gorm:"not null"`
	DeletedAt       gorm.DeletedAt `gorm:"index"`
}

func (CommerceMerchantProfile) TableName() string {
	return "commerce_merchant_profiles"
}

type CommerceStore struct {
	ID                 uuid.UUID                     `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID     uuid.UUID                     `gorm:"type:uuid;not null;index"`
	Code               string                        `gorm:"not null"`
	Name               string                        `gorm:"not null"`
	Address            string                        `gorm:"not null"`
	City               string                        `gorm:"not null"`
	State              string                        `gorm:"not null"`
	CountryCode        string                        `gorm:"type:char(2);not null"`
	Latitude           *float64                      `gorm:"type:numeric(10,7)"`
	Longitude          *float64                      `gorm:"type:numeric(10,7)"`
	Timezone           string                        `gorm:"not null"`
	PreparationMinutes int                           `gorm:"not null"`
	Status             string                        `gorm:"not null"`
	CreatedAt          time.Time                     `gorm:"not null"`
	UpdatedAt          time.Time                     `gorm:"not null"`
	DeletedAt          gorm.DeletedAt                `gorm:"index"`
	Hours              []CommerceStoreHour           `gorm:"foreignKey:StoreID"`
	FulfilmentModes    []CommerceStoreFulfilmentMode `gorm:"foreignKey:StoreID"`
}

func (CommerceStore) TableName() string {
	return "commerce_stores"
}

type CommerceStoreHour struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null;index"`
	StoreID        uuid.UUID      `gorm:"type:uuid;not null;index"`
	DayOfWeek      int            `gorm:"not null"`
	OpenMinute     *int           `gorm:"type:smallint"`
	CloseMinute    *int           `gorm:"type:smallint"`
	IsClosed       bool           `gorm:"not null"`
	CreatedAt      time.Time      `gorm:"not null"`
	UpdatedAt      time.Time      `gorm:"not null"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (CommerceStoreHour) TableName() string {
	return "commerce_store_hours"
}

type CommerceStoreFulfilmentMode struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null;index"`
	StoreID        uuid.UUID      `gorm:"type:uuid;not null;index"`
	Mode           string         `gorm:"not null"`
	Enabled        bool           `gorm:"not null"`
	CreatedAt      time.Time      `gorm:"not null"`
	UpdatedAt      time.Time      `gorm:"not null"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (CommerceStoreFulfilmentMode) TableName() string {
	return "commerce_store_fulfilment_modes"
}

type CommerceStaffStoreAssignment struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null;index"`
	StoreID        uuid.UUID      `gorm:"type:uuid;not null;index"`
	UserID         uuid.UUID      `gorm:"type:uuid;not null;index"`
	Role           string         `gorm:"not null"`
	Status         string         `gorm:"not null"`
	CreatedAt      time.Time      `gorm:"not null"`
	UpdatedAt      time.Time      `gorm:"not null"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (CommerceStaffStoreAssignment) TableName() string {
	return "commerce_staff_store_assignments"
}
