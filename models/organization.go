package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Organization struct {
	gorm.Model
	ID                 uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Email              string    `gorm:"unique;not null"`
	ContactPersonName  string
	ContactPersonPhone string
	Address            string
	Industry           string
	CompanySize        int
	CompanyName        string
}
