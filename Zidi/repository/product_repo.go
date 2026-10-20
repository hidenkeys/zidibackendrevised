package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

type ProductRepository interface {
	Create(product *models.Product) (*models.Product, error)
	GetByID(id uuid.UUID) (*models.Product, error)
	GetByInstitutionID(institutionID uuid.UUID, categoryID string, limit, offset int) ([]models.Product, int64, error)
	Update(product *models.Product) (*models.Product, error)
	Delete(id uuid.UUID) error
	GetBySKU(institutionID uuid.UUID, sku string) (*models.Product, error)
}
