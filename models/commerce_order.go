package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	CommerceOrderStatusDraft             = "draft"
	CommerceOrderStatusPendingPayment    = "pending_payment"
	CommerceOrderStatusPaid              = "paid"
	CommerceOrderStatusProcessing        = "processing"
	CommerceOrderStatusReady             = "ready"
	CommerceOrderStatusFulfilmentPending = "fulfilment_pending"
	CommerceOrderStatusReadyForPickup    = "ready_for_pickup"
	CommerceOrderStatusOutForDelivery    = "out_for_delivery"
	CommerceOrderStatusDelivered         = "delivered"
	CommerceOrderStatusCompleted         = "completed"
	CommerceOrderStatusPaymentFailed     = "payment_failed"
	CommerceOrderStatusPaymentExpired    = "payment_expired"
	CommerceOrderStatusCancelled         = "cancelled"
	CommerceOrderStatusRefunded          = "refunded"

	CommerceOrderEventCreated           = "order_created"
	CommerceOrderEventPaymentInitiated  = "payment_initiated"
	CommerceOrderEventPaymentConfirmed  = "payment_confirmed"
	CommerceOrderEventPaymentFailed     = "payment_failed"
	CommerceOrderEventPaymentExpired    = "payment_expired"
	CommerceOrderEventProcessing        = "order_processing"
	CommerceOrderEventReady             = "order_ready"
	CommerceOrderEventFulfilmentPending = "fulfilment_pending"
	CommerceOrderEventReadyForPickup    = "ready_for_pickup"
	CommerceOrderEventOutForDelivery    = "out_for_delivery"
	CommerceOrderEventDelivered         = "order_delivered"
	CommerceOrderEventCompleted         = "order_completed"
	CommerceOrderEventCancelled         = "order_cancelled"
	CommerceOrderEventRefunded          = "order_refunded"
	CommerceOrderEventCustomerNotified  = "customer_notified"
	CommerceOrderEventRiderRequested    = "rider_requested"
	CommerceOrderEventRiderAssigned     = "rider_assigned"
	CommerceOrderEventPickedUp          = "order_picked_up"

	CommerceOrderActorSystem  = "system"
	CommerceOrderActorUser    = "user"
	CommerceOrderActorPayment = "payment"
	CommerceOrderActorChannel = "channel"
)

type CommerceOrder struct {
	ID                   uuid.UUID            `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID       uuid.UUID            `gorm:"type:uuid;not null;index"`
	CartID               uuid.UUID            `gorm:"type:uuid;not null;index"`
	CustomerID           uuid.UUID            `gorm:"type:uuid;not null;index"`
	StoreID              uuid.UUID            `gorm:"type:uuid;not null;index"`
	OrderNumber          string               `gorm:"not null"`
	CheckoutKey          string               `gorm:"not null"`
	CustomerName         string               `gorm:"not null"`
	CustomerPhone        string               `gorm:"not null"`
	CustomerEmail        *string              `gorm:"type:text"`
	FulfilmentMode       string               `gorm:"not null"`
	DestinationAddress   *string              `gorm:"type:text"`
	DestinationLatitude  *float64             `gorm:"type:numeric(10,7)"`
	DestinationLongitude *float64             `gorm:"type:numeric(10,7)"`
	Status               string               `gorm:"not null"`
	Currency             string               `gorm:"type:char(3);not null"`
	SubtotalMinor        int64                `gorm:"not null"`
	DiscountMinor        int64                `gorm:"not null"`
	DeliveryFeeMinor     int64                `gorm:"not null"`
	TotalMinor           int64                `gorm:"not null"`
	Version              int64                `gorm:"not null"`
	PaymentExpiresAt     time.Time            `gorm:"not null;index"`
	CreatedAt            time.Time            `gorm:"not null"`
	UpdatedAt            time.Time            `gorm:"not null"`
	Items                []CommerceOrderItem  `gorm:"foreignKey:OrderID"`
	Events               []CommerceOrderEvent `gorm:"foreignKey:OrderID"`
}

func (CommerceOrder) TableName() string {
	return "commerce_orders"
}

type CommerceOrderItem struct {
	ID                     uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID         uuid.UUID       `gorm:"type:uuid;not null;index"`
	OrderID                uuid.UUID       `gorm:"type:uuid;not null;index"`
	ProductID              uuid.UUID       `gorm:"type:uuid;not null;index"`
	VariantID              uuid.UUID       `gorm:"type:uuid;not null;index"`
	InventoryReservationID uuid.UUID       `gorm:"type:uuid;not null;index"`
	ProductName            string          `gorm:"not null"`
	VariantName            string          `gorm:"not null"`
	SKU                    string          `gorm:"not null"`
	Attributes             json.RawMessage `gorm:"type:jsonb;not null"`
	PrimaryImageURL        *string         `gorm:"type:text"`
	Quantity               int             `gorm:"not null"`
	UnitPriceMinor         int64           `gorm:"not null"`
	LineTotalMinor         int64           `gorm:"not null"`
	CreatedAt              time.Time       `gorm:"not null"`
}

func (CommerceOrderItem) TableName() string {
	return "commerce_order_items"
}

type CommerceOrderEvent struct {
	ID             uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID       `gorm:"type:uuid;not null;index"`
	OrderID        uuid.UUID       `gorm:"type:uuid;not null;index"`
	EventType      string          `gorm:"not null"`
	FromStatus     *string         `gorm:"type:text"`
	ToStatus       string          `gorm:"not null"`
	ActorType      string          `gorm:"not null"`
	ActorUserID    *uuid.UUID      `gorm:"type:uuid"`
	Reason         string          `gorm:"not null"`
	Metadata       json.RawMessage `gorm:"type:jsonb;not null"`
	IdempotencyKey string          `gorm:"not null"`
	CreatedAt      time.Time       `gorm:"not null"`
}

func (CommerceOrderEvent) TableName() string {
	return "commerce_order_events"
}
