package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Balance struct {
	gorm.Model
	ID              uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	CampaignId      uuid.UUID `gorm:"type:uuid;not null"`
	StartingBalance float64   `gorm:"type:decimal(10,2);not null"`
	Amount          float64   `gorm:"not null"`
}
