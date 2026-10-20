package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type Coupon struct {
	gorm.Model
	ID         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	CampaignID uuid.UUID `gorm:"not null;index"`
	Code       string    `gorm:"uniqueIndex;not null"`
	Redeemed   bool      `gorm:"default:false"`
	RedeemedAt *time.Time
}
