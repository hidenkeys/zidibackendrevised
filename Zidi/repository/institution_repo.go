package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

type InstitutionRepository interface {
	Create(institution *models.Institution) (*models.Institution, error)
	GetByID(id uuid.UUID) (*models.Institution, error)
	GetByName(name string) (*models.Institution, error)
	UpdateByID(id uuid.UUID, institution *models.Institution) (*models.Institution, error)
	DeleteByID(id uuid.UUID) error
}
