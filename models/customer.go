package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Customer struct {
	gorm.Model
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	FirstName      string
	LastName       string
	Phone          string
	Email          string
	Feedback       string
	Network        string
	Amount         float64
	Status         string
	Channel        string
	OrganizationID uuid.UUID `gorm:"constraint:OnDelete:CASCADE;"`
	CampaignID     uuid.UUID `gorm:"constraint:OnDelete:CASCADE;"`
}
