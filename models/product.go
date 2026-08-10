package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Product struct {
	gorm.Model
	ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	InstitutionID uuid.UUID `gorm:"type:uuid;not null;index"`
	Name          string    `gorm:"not null"`
	Description   string
	Price         float64 `gorm:"not null"`
	SKU           string  `gorm:"uniqueIndex"`
	StockQuantity int     `gorm:"not null;default:0"`
	ImageURL      string
	CategoryID    uuid.UUID `gorm:"type:uuid;index"`
	Category      Category  `gorm:"foreignKey:CategoryID"`
}
