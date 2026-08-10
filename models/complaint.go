package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Complaint struct {
	gorm.Model
	ID            uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	InstitutionID uuid.UUID  `gorm:"type:uuid;not null;index"`
	ClientID      uuid.UUID  `gorm:"type:uuid;not null;index"`
	OrderID       *uuid.UUID `gorm:"type:uuid;index"` // Optional, nullable
	Category      string
	Status        string `gorm:"default:'open'"` // open, in_progress, resolved
	Description   string
	Resolution    string
}
