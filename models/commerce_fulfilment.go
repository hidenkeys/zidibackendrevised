package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	CommerceFulfilmentStatusReadyForPickup               = "ready_for_pickup"
	CommerceFulfilmentStatusAwaitingQuote                = "awaiting_quote"
	CommerceFulfilmentStatusAwaitingCustomerConfirmation = "awaiting_customer_confirmation"
	CommerceFulfilmentStatusRiderRequested               = "rider_requested"
	CommerceFulfilmentStatusRiderAssigned                = "rider_assigned"
	CommerceFulfilmentStatusOutForDelivery               = "out_for_delivery"
	CommerceFulfilmentStatusDelivered                    = "delivered"
	CommerceFulfilmentStatusCompleted                    = "completed"
	CommerceFulfilmentStatusCancelled                    = "cancelled"

	CommerceDeliveryQuoteSourceManual   = "manual"
	CommerceDeliveryQuoteSourceProvider = "provider"
	CommerceDeliveryQuoteStatusQuoted   = "quoted"
	CommerceDeliveryQuoteStatusAccepted = "accepted"
	CommerceDeliveryQuoteStatusRejected = "rejected"
	CommerceDeliveryQuoteStatusExpired  = "expired"

	CommerceDeliveryFeePaymentDirectToRider = "direct_to_rider"
	CommerceDeliveryFeePaymentZidiCollected = "zidi_collected"
	CommerceDeliveryFeeStatusNotCollected   = "not_collected"
	CommerceDeliveryFeeStatusDue            = "due"
	CommerceDeliveryFeeStatusPaidExternal   = "paid_external"
	CommerceDeliveryFeeStatusPaid           = "paid"

	CommerceRiderSourceCustomer  = "customer"
	CommerceRiderSourceMerchant  = "merchant"
	CommerceRiderStatusAssigned  = "assigned"
	CommerceRiderStatusArrived   = "arrived"
	CommerceRiderStatusPickedUp  = "picked_up"
	CommerceRiderStatusDelivered = "delivered"
	CommerceRiderStatusCancelled = "cancelled"

	CommerceFulfilmentActorUser     = "user"
	CommerceFulfilmentActorSystem   = "system"
	CommerceFulfilmentActorCustomer = "customer"
	CommerceFulfilmentActorProvider = "provider"

	CommerceFulfilmentEventStarted         = "fulfilment_started"
	CommerceFulfilmentEventQuoteCreated    = "delivery_quote_created"
	CommerceFulfilmentEventQuoteAccepted   = "delivery_quote_accepted"
	CommerceFulfilmentEventQuoteRejected   = "delivery_quote_rejected"
	CommerceFulfilmentEventQuoteExpired    = "delivery_quote_expired"
	CommerceFulfilmentEventRiderAssigned   = "rider_assigned"
	CommerceFulfilmentEventCustomerArrived = "customer_arrived"
	CommerceFulfilmentEventRiderArrived    = "rider_arrived"
	CommerceFulfilmentEventHandoverFailed  = "handover_verification_failed"
	CommerceFulfilmentEventHandedOver      = "order_handed_over"
	CommerceFulfilmentEventDelivered       = "delivery_confirmed"
	CommerceFulfilmentEventCompleted       = "fulfilment_completed"

	CommerceOutboxTopicFulfilmentReady        = "commerce.fulfilment.ready"
	CommerceOutboxTopicDeliveryQuoteAvailable = "commerce.fulfilment.delivery_quote_available"
	CommerceOutboxTopicRiderAssigned          = "commerce.fulfilment.rider_assigned"
	CommerceOutboxTopicOutForDelivery         = "commerce.fulfilment.out_for_delivery"
	CommerceOutboxTopicFulfilmentDelivered    = "commerce.fulfilment.delivered"
)

type CommerceFulfilment struct {
	ID                         uuid.UUID                 `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID             uuid.UUID                 `gorm:"type:uuid;not null;index"`
	OrderID                    uuid.UUID                 `gorm:"type:uuid;not null;index"`
	StoreID                    uuid.UUID                 `gorm:"type:uuid;not null;index"`
	CustomerID                 uuid.UUID                 `gorm:"type:uuid;not null;index"`
	Mode                       string                    `gorm:"not null"`
	Status                     string                    `gorm:"not null"`
	PickupAddress              string                    `gorm:"not null"`
	PickupLatitude             *float64                  `gorm:"type:numeric(10,7)"`
	PickupLongitude            *float64                  `gorm:"type:numeric(10,7)"`
	DestinationAddress         *string                   `gorm:"type:text"`
	DestinationLatitude        *float64                  `gorm:"type:numeric(10,7)"`
	DestinationLongitude       *float64                  `gorm:"type:numeric(10,7)"`
	VerificationCodeHash       []byte                    `gorm:"type:bytea;not null" json:"-"`
	VerificationCodeCiphertext []byte                    `gorm:"type:bytea;not null" json:"-"`
	VerificationAttempts       int                       `gorm:"not null"`
	VerificationLockedUntil    *time.Time                `gorm:"type:timestamptz"`
	VerificationCodeExpiresAt  time.Time                 `gorm:"not null"`
	VerifiedAt                 *time.Time                `gorm:"type:timestamptz"`
	VerifiedByUserID           *uuid.UUID                `gorm:"type:uuid"`
	HandedOverAt               *time.Time                `gorm:"type:timestamptz"`
	HandedOverByUserID         *uuid.UUID                `gorm:"type:uuid"`
	DeliveredAt                *time.Time                `gorm:"type:timestamptz"`
	CompletedAt                *time.Time                `gorm:"type:timestamptz"`
	Version                    int64                     `gorm:"not null"`
	CreatedAt                  time.Time                 `gorm:"not null"`
	UpdatedAt                  time.Time                 `gorm:"not null"`
	Quotes                     []CommerceDeliveryQuote   `gorm:"foreignKey:FulfilmentID"`
	RiderAssignments           []CommerceRiderAssignment `gorm:"foreignKey:FulfilmentID"`
	Events                     []CommerceFulfilmentEvent `gorm:"foreignKey:FulfilmentID"`
}

func (CommerceFulfilment) TableName() string { return "commerce_fulfilments" }

type CommerceDeliveryQuote struct {
	ID                   uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID       uuid.UUID       `gorm:"type:uuid;not null;index"`
	FulfilmentID         uuid.UUID       `gorm:"type:uuid;not null;index"`
	OrderID              uuid.UUID       `gorm:"type:uuid;not null;index"`
	Source               string          `gorm:"not null"`
	Provider             *string         `gorm:"type:text"`
	ProviderQuoteID      *string         `gorm:"type:text"`
	Status               string          `gorm:"not null"`
	PickupAddress        string          `gorm:"not null"`
	PickupLatitude       *float64        `gorm:"type:numeric(10,7)"`
	PickupLongitude      *float64        `gorm:"type:numeric(10,7)"`
	DestinationAddress   string          `gorm:"not null"`
	DestinationLatitude  *float64        `gorm:"type:numeric(10,7)"`
	DestinationLongitude *float64        `gorm:"type:numeric(10,7)"`
	DistanceMeters       *int            `gorm:"type:integer"`
	DurationSeconds      *int            `gorm:"type:integer"`
	EstimatedFeeMinor    int64           `gorm:"not null"`
	Currency             string          `gorm:"type:char(3);not null"`
	FeePaymentMode       string          `gorm:"not null"`
	FeeStatus            string          `gorm:"not null"`
	RawResponse          json.RawMessage `gorm:"type:jsonb;not null"`
	IdempotencyKey       string          `gorm:"not null"`
	CreatedByUserID      uuid.UUID       `gorm:"type:uuid;not null"`
	ExpiresAt            *time.Time      `gorm:"type:timestamptz"`
	AcceptedAt           *time.Time      `gorm:"type:timestamptz"`
	RejectedAt           *time.Time      `gorm:"type:timestamptz"`
	CreatedAt            time.Time       `gorm:"not null"`
	UpdatedAt            time.Time       `gorm:"not null"`
}

func (CommerceDeliveryQuote) TableName() string { return "commerce_delivery_quotes" }

type CommerceRiderAssignment struct {
	ID                   uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID       uuid.UUID  `gorm:"type:uuid;not null;index"`
	FulfilmentID         uuid.UUID  `gorm:"type:uuid;not null;index"`
	OrderID              uuid.UUID  `gorm:"type:uuid;not null;index"`
	StoreID              uuid.UUID  `gorm:"type:uuid;not null;index"`
	Source               string     `gorm:"not null"`
	Provider             *string    `gorm:"type:text"`
	ProviderAssignmentID *string    `gorm:"type:text"`
	RiderName            string     `gorm:"not null"`
	RiderPhone           string     `gorm:"not null"`
	VehicleDescription   *string    `gorm:"type:text"`
	TrackingURL          *string    `gorm:"type:text"`
	Status               string     `gorm:"not null"`
	IdempotencyKey       string     `gorm:"not null"`
	AssignedByUserID     uuid.UUID  `gorm:"type:uuid;not null"`
	ArrivedAt            *time.Time `gorm:"type:timestamptz"`
	PickedUpAt           *time.Time `gorm:"type:timestamptz"`
	DeliveredAt          *time.Time `gorm:"type:timestamptz"`
	CancelledAt          *time.Time `gorm:"type:timestamptz"`
	CreatedAt            time.Time  `gorm:"not null"`
	UpdatedAt            time.Time  `gorm:"not null"`
}

func (CommerceRiderAssignment) TableName() string { return "commerce_rider_assignments" }

type CommerceFulfilmentEvent struct {
	ID             uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID       `gorm:"type:uuid;not null;index"`
	FulfilmentID   uuid.UUID       `gorm:"type:uuid;not null;index"`
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

func (CommerceFulfilmentEvent) TableName() string { return "commerce_fulfilment_events" }
