package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CommercePayment struct {
	gorm.Model
	ID               uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrderID          uuid.UUID `gorm:"type:uuid;not null;index"`
	Order            Order     `gorm:"foreignKey:OrderID"`
	Amount           float64   `gorm:"not null"`
	PaymentReference string    `gorm:"uniqueIndex;not null"`
	Status           string    `gorm:"type:varchar(20);not null"` // "success", "failed"
	PayerEmail       string
	Gateway          string // "paystack", "flutterwave"
}
