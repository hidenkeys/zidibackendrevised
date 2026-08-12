package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	CommerceEmailStatusPending    = "pending"
	CommerceEmailStatusProcessing = "processing"
	CommerceEmailStatusSent       = "sent"
	CommerceEmailStatusFailed     = "failed"
)

type CommerceEmailMessage struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index"`
	CustomerID     uuid.UUID `gorm:"type:uuid;not null;index"`
	OrderID        uuid.UUID `gorm:"type:uuid;not null;index"`
	OutboxEventID  uuid.UUID `gorm:"type:uuid;not null;index"`
	Recipient      string    `gorm:"not null"`
	Subject        string    `gorm:"not null"`
	HTMLBody       string    `gorm:"column:html_body;not null"`
	Status         string    `gorm:"not null"`
	Attempts       int       `gorm:"not null"`
	AvailableAt    time.Time `gorm:"not null"`
	LockedAt       *time.Time
	SentAt         *time.Time
	LastError      *string
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (CommerceEmailMessage) TableName() string { return "commerce_email_messages" }
