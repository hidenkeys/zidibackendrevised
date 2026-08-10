package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

type TransactionRepository interface {
	Create(transaction *models.Transaction) (*models.Transaction, error)
	GetAll(limit, offset int) ([]models.Transaction, int64, error)
	GetByID(id uuid.UUID) (*models.Transaction, error)
	GetByReference(reference string) (*models.Transaction, error)
	GetByOrganization(orgID uuid.UUID, limit, offset int) ([]models.Transaction, int64, error)
	UpdateStatus(id uuid.UUID, status string) (*models.Transaction, error)
}
