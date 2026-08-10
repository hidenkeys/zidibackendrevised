package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

type ClientRepository interface {
	Create(client *models.Client) (*models.Client, error)
	GetByID(id uuid.UUID) (*models.Client, error)
	GetByPhone(institutionID uuid.UUID, phone string) (*models.Client, error)
	GetByEmail(institutionID uuid.UUID, email string) (*models.Client, error)
	Update(client *models.Client) (*models.Client, error)
	Delete(id uuid.UUID) error
}
