package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type Campaign struct {
	gorm.Model
	ID                    uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	CampaignName          string    `gorm:"not null"`
	CouponID              string
	CharacterType         string
	CouponLength          int
	CouponNumber          int
	OrganizationID        uuid.UUID `gorm:"constraint:OnDelete:CASCADE;"`
	WelcomeMessage        string    `gorm:"type:text"`
	QuestionNumber        int
	Amount                float64
	Price                 float64
	Status                string
	CommunicationChannels string `gorm:"type:text"` // Stores comma-separated values: "whatsapp", "telegram", "whatsapp,telegram"
	CreatedAt             time.Time

	// Relations
	Questions []Question `gorm:"foreignKey:CampaignID"`
}
