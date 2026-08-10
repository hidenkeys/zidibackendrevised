package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	FirstName      string
	LastName       string
	Email          string `gorm:"unique;not null"`
	Address        string
	Password       string
	Role           string
	OrganizationID uuid.UUID `gorm:"constraint:OnDelete:CASCADE;"`
}
