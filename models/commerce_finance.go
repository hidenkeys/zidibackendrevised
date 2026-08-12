package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	CommerceInvoiceStatusIssued = "issued"
	CommerceInvoiceStatusPaid   = "paid"
	CommerceInvoiceStatusVoid   = "void"

	CommercePaymentStatusInitializing   = "initializing"
	CommercePaymentStatusPending        = "pending"
	CommercePaymentStatusSucceeded      = "succeeded"
	CommercePaymentStatusFailed         = "failed"
	CommercePaymentStatusExpired        = "expired"
	CommercePaymentStatusReviewRequired = "review_required"

	CommerceWebhookStatusReceived  = "received"
	CommerceWebhookStatusProcessed = "processed"
	CommerceWebhookStatusIgnored   = "ignored"
	CommerceWebhookStatusFailed    = "failed"

	CommerceOutboxStatusPending    = "pending"
	CommerceOutboxStatusProcessing = "processing"
	CommerceOutboxStatusDelivered  = "delivered"
	CommerceOutboxStatusFailed     = "failed"

	CommerceOutboxTopicPaymentCustomer = "commerce.payment_confirmed.customer"
	CommerceOutboxTopicPaymentStore    = "commerce.payment_confirmed.store"
)

type CommerceInvoice struct {
	ID               uuid.UUID             `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID   uuid.UUID             `gorm:"type:uuid;not null;index"`
	OrderID          uuid.UUID             `gorm:"type:uuid;not null;index"`
	StoreID          uuid.UUID             `gorm:"type:uuid;not null;index"`
	CustomerID       uuid.UUID             `gorm:"type:uuid;not null;index"`
	InvoiceNumber    string                `gorm:"not null"`
	Status           string                `gorm:"not null"`
	MerchantName     string                `gorm:"not null"`
	StoreName        string                `gorm:"not null"`
	StoreAddress     string                `gorm:"not null"`
	CustomerName     string                `gorm:"not null"`
	CustomerEmail    *string               `gorm:"type:text"`
	OrderNumber      string                `gorm:"not null"`
	FulfilmentMode   string                `gorm:"not null"`
	Currency         string                `gorm:"type:char(3);not null"`
	SubtotalMinor    int64                 `gorm:"not null"`
	DiscountMinor    int64                 `gorm:"not null"`
	DeliveryFeeMinor int64                 `gorm:"not null"`
	TotalMinor       int64                 `gorm:"not null"`
	IssuedAt         time.Time             `gorm:"not null"`
	PaidAt           *time.Time            `gorm:"type:timestamptz"`
	VoidedAt         *time.Time            `gorm:"type:timestamptz"`
	CreatedAt        time.Time             `gorm:"not null"`
	UpdatedAt        time.Time             `gorm:"not null"`
	Items            []CommerceInvoiceItem `gorm:"foreignKey:InvoiceID"`
}

func (CommerceInvoice) TableName() string { return "commerce_invoices" }

type CommerceInvoiceItem struct {
	ID             uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID       `gorm:"type:uuid;not null;index"`
	InvoiceID      uuid.UUID       `gorm:"type:uuid;not null;index"`
	OrderItemID    uuid.UUID       `gorm:"type:uuid;not null"`
	ProductID      uuid.UUID       `gorm:"type:uuid;not null"`
	VariantID      uuid.UUID       `gorm:"type:uuid;not null"`
	ProductName    string          `gorm:"not null"`
	VariantName    string          `gorm:"not null"`
	SKU            string          `gorm:"not null"`
	Attributes     json.RawMessage `gorm:"type:jsonb;not null"`
	Quantity       int             `gorm:"not null"`
	UnitPriceMinor int64           `gorm:"not null"`
	LineTotalMinor int64           `gorm:"not null"`
	CreatedAt      time.Time       `gorm:"not null"`
}

func (CommerceInvoiceItem) TableName() string { return "commerce_invoice_items" }

type CommercePaymentTransaction struct {
	ID                    uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID        uuid.UUID       `gorm:"type:uuid;not null;index"`
	OrderID               uuid.UUID       `gorm:"type:uuid;not null;index"`
	InvoiceID             uuid.UUID       `gorm:"type:uuid;not null;index"`
	Provider              string          `gorm:"not null"`
	ProviderReference     string          `gorm:"not null"`
	ProviderTransactionID *string         `gorm:"type:text"`
	IdempotencyKey        string          `gorm:"not null"`
	PayerEmail            string          `gorm:"not null"`
	Status                string          `gorm:"not null"`
	Currency              string          `gorm:"type:char(3);not null"`
	AmountMinor           int64           `gorm:"not null"`
	AuthorizationURL      *string         `gorm:"type:text"`
	AccessCode            *string         `gorm:"type:text"`
	FailureReason         string          `gorm:"not null"`
	ProviderResponse      json.RawMessage `gorm:"type:jsonb;not null"`
	ExpiresAt             time.Time       `gorm:"not null"`
	InitializedAt         *time.Time      `gorm:"type:timestamptz"`
	ConfirmedAt           *time.Time      `gorm:"type:timestamptz"`
	FailedAt              *time.Time      `gorm:"type:timestamptz"`
	CreatedAt             time.Time       `gorm:"not null"`
	UpdatedAt             time.Time       `gorm:"not null"`
}

func (CommercePaymentTransaction) TableName() string { return "commerce_payment_transactions" }

type CommercePaymentWebhookEvent struct {
	ID                uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID    *uuid.UUID      `gorm:"type:uuid;index"`
	OrderID           *uuid.UUID      `gorm:"type:uuid;index"`
	PaymentID         *uuid.UUID      `gorm:"type:uuid;index"`
	Provider          string          `gorm:"not null"`
	EventKey          string          `gorm:"not null"`
	EventType         string          `gorm:"not null"`
	ProviderReference string          `gorm:"not null"`
	Status            string          `gorm:"not null"`
	FailureReason     string          `gorm:"not null"`
	Payload           json.RawMessage `gorm:"type:jsonb;not null"`
	ReceivedAt        time.Time       `gorm:"not null"`
	ProcessedAt       *time.Time      `gorm:"type:timestamptz"`
	UpdatedAt         time.Time       `gorm:"not null"`
}

func (CommercePaymentWebhookEvent) TableName() string {
	return "commerce_payment_webhook_events"
}

type CommerceOutboxEvent struct {
	ID               uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID   uuid.UUID       `gorm:"type:uuid;not null;index"`
	AggregateType    string          `gorm:"not null"`
	AggregateID      uuid.UUID       `gorm:"type:uuid;not null;index"`
	Topic            string          `gorm:"not null"`
	DeduplicationKey string          `gorm:"not null"`
	Payload          json.RawMessage `gorm:"type:jsonb;not null"`
	Status           string          `gorm:"not null"`
	Attempts         int             `gorm:"not null"`
	AvailableAt      time.Time       `gorm:"not null"`
	ProcessedAt      *time.Time      `gorm:"type:timestamptz"`
	LockedAt         *time.Time      `gorm:"type:timestamptz"`
	LastError        *string
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (CommerceOutboxEvent) TableName() string { return "commerce_outbox_events" }
