package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

type CustomerRepository interface {
	Create(customer *models.Customer) (*models.Customer, error)
	UpdateByID(id uuid.UUID, customer *models.Customer) (*models.Customer, error)
	DeleteByID(id uuid.UUID) error
	GetByID(id uuid.UUID) (*models.Customer, error)
	GetAll(limit, offset int) ([]models.Customer, int64, error)
	GetAllByOrganization(orgID uuid.UUID, limit, offset int) ([]models.Customer, int64, error)
	GetAllByCampaign(campaignID uuid.UUID, limit, offset int) ([]models.Customer, int64, error)
}
