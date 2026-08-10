package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

type PaymentRepository interface {
	Create(campaign *models.Payment) (*models.Payment, error)
	GetAll(limit, offset int) ([]models.Payment, int64, error)
	GetAllByOrganization(orgID uuid.UUID, limit, offset int) ([]models.Payment, int64, error)
	GetByID(id uuid.UUID) (*models.Payment, error)
	UpdateByID(id uuid.UUID, campaign *models.Payment) (*models.Payment, error)
	GetByTransactionRef(ref string) (*models.Payment, error)
}
