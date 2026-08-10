package models

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Institution struct {
	gorm.Model
	ID                 uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Name               string    `gorm:"not null"`
	LogoURL            string
	ContactPersonName  string
	ContactPersonPhone string
	WelcomeMessage     string
	FAQContent         json.RawMessage `gorm:"type:jsonb"` // Store FAQ structure as JSON
}
