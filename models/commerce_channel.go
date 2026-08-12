package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	CommerceChannelWhatsApp = "whatsapp"

	CommerceConversationStateWelcome              = "welcome"
	CommerceConversationStateIntent               = "intent"
	CommerceConversationStateLocation             = "location"
	CommerceConversationStateStore                = "store"
	CommerceConversationStateCategory             = "category"
	CommerceConversationStateProduct              = "product"
	CommerceConversationStateQuantity             = "quantity"
	CommerceConversationStateCart                 = "cart"
	CommerceConversationStateCustomerName         = "customer_name"
	CommerceConversationStateFulfilment           = "fulfilment"
	CommerceConversationStateDeliveryAddress      = "delivery_address"
	CommerceConversationStatePaymentEmail         = "payment_email"
	CommerceConversationStatePaymentRenewal       = "payment_renewal"
	CommerceConversationStateOrderID              = "order_id"
	CommerceConversationStateComplaintOrder       = "complaint_order"
	CommerceConversationStateComplaintDescription = "complaint_description"

	CommerceConversationIntentOrder      = "order"
	CommerceConversationIntentTrackOrder = "track_order"
	CommerceConversationIntentComplaint  = "complaint"

	CommerceChannelDirectionInbound  = "inbound"
	CommerceChannelDirectionOutbound = "outbound"

	CommerceChannelMessageStatusReceived   = "received"
	CommerceChannelMessageStatusProcessing = "processing"
	CommerceChannelMessageStatusProcessed  = "processed"
	CommerceChannelMessageStatusPending    = "pending"
	CommerceChannelMessageStatusSent       = "sent"
	CommerceChannelMessageStatusFailed     = "failed"

	CommerceComplaintStatusOpen       = "open"
	CommerceComplaintStatusInProgress = "in_progress"
	CommerceComplaintStatusResolved   = "resolved"
	CommerceComplaintStatusClosed     = "closed"
)

type CommerceChannelConfiguration struct {
	ID                 uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID     uuid.UUID `gorm:"type:uuid;not null;index"`
	Channel            string    `gorm:"not null"`
	ProviderAccountID  string    `gorm:"not null"`
	DisplayPhoneNumber *string
	WelcomeMessage     string    `gorm:"not null"`
	Status             string    `gorm:"not null"`
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

func (CommerceChannelConfiguration) TableName() string { return "commerce_channel_configurations" }

type CommerceConversation struct {
	ID                     uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID         uuid.UUID `gorm:"type:uuid;not null;index"`
	ChannelConfigurationID uuid.UUID `gorm:"type:uuid;not null;index"`
	CustomerID             uuid.UUID `gorm:"type:uuid;not null;index"`
	Channel                string    `gorm:"not null"`
	ExternalUserID         string    `gorm:"not null"`
	State                  string    `gorm:"not null"`
	CurrentIntent          *string
	Context                json.RawMessage `gorm:"type:jsonb;not null"`
	Version                int64           `gorm:"not null"`
	ProcessingMessageID    *uuid.UUID      `gorm:"type:uuid"`
	LockedUntil            *time.Time      `gorm:"type:timestamptz"`
	LastMessageAt          time.Time       `gorm:"not null"`
	CreatedAt              time.Time       `gorm:"not null"`
	UpdatedAt              time.Time       `gorm:"not null"`
}

func (CommerceConversation) TableName() string { return "commerce_conversations" }

type CommerceChannelMessage struct {
	ID                     uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID         uuid.UUID `gorm:"type:uuid;not null;index"`
	ChannelConfigurationID uuid.UUID `gorm:"type:uuid;not null;index"`
	ConversationID         uuid.UUID `gorm:"type:uuid;not null;index"`
	Direction              string    `gorm:"not null"`
	ExternalMessageID      *string
	ProviderMessageID      *string
	SenderID               string          `gorm:"not null"`
	RecipientID            string          `gorm:"not null"`
	MessageType            string          `gorm:"not null"`
	Body                   string          `gorm:"not null"`
	Payload                json.RawMessage `gorm:"type:jsonb;not null"`
	Status                 string          `gorm:"not null"`
	Attempts               int             `gorm:"not null"`
	AvailableAt            time.Time       `gorm:"not null"`
	LockedAt               *time.Time      `gorm:"type:timestamptz"`
	ProcessedAt            *time.Time      `gorm:"type:timestamptz"`
	LastError              *string
	CreatedAt              time.Time `gorm:"not null"`
	UpdatedAt              time.Time `gorm:"not null"`
}

func (CommerceChannelMessage) TableName() string { return "commerce_channel_messages" }

type CommerceComplaint struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index"`
	CustomerID     uuid.UUID `gorm:"type:uuid;not null;index"`
	OrderID        *uuid.UUID
	StoreID        *uuid.UUID
	ConversationID *uuid.UUID
	Category       string `gorm:"not null"`
	Description    string `gorm:"not null"`
	Status         string `gorm:"not null"`
	Resolution     *string
	ResolvedAt     *time.Time `gorm:"type:timestamptz"`
	CreatedAt      time.Time  `gorm:"not null"`
	UpdatedAt      time.Time  `gorm:"not null"`
}

func (CommerceComplaint) TableName() string { return "commerce_complaints" }
