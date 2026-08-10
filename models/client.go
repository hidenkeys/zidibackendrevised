package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Client struct {
	gorm.Model
	ID               uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	InstitutionID    uuid.UUID `gorm:"type:uuid;not null;index"`
	Name             string
	FirstName        string
	LastName         string
	Phone            string `gorm:"index"`
	Email            string
	AgeRange         string
	OnboardingStatus string `gorm:"default:'incomplete'"` // incomplete, complete
}
