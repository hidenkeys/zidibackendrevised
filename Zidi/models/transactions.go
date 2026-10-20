package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Transaction struct {
	gorm.Model
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null"`
	CampaignID     uuid.UUID `gorm:"type:uuid;not null"`
	CustomerID     uuid.UUID `gorm:"type:uuid;not null"`
	Amount         float64   `gorm:"not null"`
	PhoneNumber    string    `gorm:"not null"`
	Network        string    `gorm:"not null"`
	TxReference    string    `gorm:"not null"`
	Status         string    `gorm:"type:varchar(50);not null"` // e.g., "pending", "successful", "failed"
	Type           string    `gorm:"type:varchar(50);not null"` // e.g., "airtime", "data"
	Commisson      float64   `gorm:"not null"`
}
