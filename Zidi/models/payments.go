package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Payment struct {
	gorm.Model
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID `gorm:"not null;index"`
	CampaignID     uuid.UUID `gorm:"not null;index"`
	Amount         float64   `gorm:"not null"`
	Status         string    `gorm:"type:varchar(20);not null"` // e.g., "pending", "completed", "failed"
	TransactionRef string    `gorm:"uniqueIndex;not null"`
	TransactionID  string    `gorm:"not null"`
}
