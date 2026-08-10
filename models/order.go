package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Order struct {
	gorm.Model
	ID               uuid.UUID   `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	InstitutionID    uuid.UUID   `gorm:"type:uuid;not null;index"`
	ClientID         uuid.UUID   `gorm:"type:uuid;not null;index"`
	Items            []OrderItem `gorm:"foreignKey:OrderID"`
	TotalAmount      float64     `gorm:"not null"`
	Status           string      `gorm:"type:varchar(50);default:'pending_payment'"` // pending_payment, paid, processing, shipped, delivered, cancelled
	PaymentStatus    string      `gorm:"type:varchar(50);default:'pending'"`
	PaymentReference string
	TrackingNumber   string `gorm:"uniqueIndex"`
}

const (
	OrderStatusPendingPayment = "pending_payment"
	OrderStatusPaid           = "paid"
	OrderStatusProcessing     = "processing"
	OrderStatusShipped        = "shipped"
	OrderStatusDelivered      = "delivered"
	OrderStatusCancelled      = "cancelled"
)

type OrderItem struct {
	gorm.Model
	ID              uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrderID         uuid.UUID       `gorm:"type:uuid;not null;index"`
	ProductID       uuid.UUID       `gorm:"type:uuid;not null;index"`
	Product         Product         `gorm:"foreignKey:ProductID"`
	ProductSnapshot json.RawMessage `gorm:"type:jsonb"` // Snapshot of product details at time of purchase
	Quantity        int             `gorm:"not null"`
	UnitPrice       float64         `gorm:"not null"`
	SubTotal        float64         `gorm:"not null"`
}

type OrderTracking struct {
	gorm.Model
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrderID     uuid.UUID `gorm:"type:uuid;not null;index"`
	Status      string    `gorm:"not null"`
	Description string
	Location    string
	Timestamp   time.Time `gorm:"default:current_timestamp"`
}
