package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Category struct {
	gorm.Model
	ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	InstitutionID uuid.UUID `gorm:"type:uuid;not null;index"`
	Name          string    `gorm:"not null"`
	Description   string
}
