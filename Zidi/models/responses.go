package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Response struct {
	gorm.Model
	ID         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	CustomerID uuid.UUID `gorm:"not null;index"` // Links to Customer
	QuestionID uuid.UUID `gorm:"not null;index"` // Links to Question
	Answer     string    `gorm:"type:text;not null"`
}
