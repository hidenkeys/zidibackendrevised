package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

type ComplaintRepository interface {
	Create(complaint *models.Complaint) (*models.Complaint, error)
	GetByID(id uuid.UUID) (*models.Complaint, error)
	GetByClientID(clientID uuid.UUID, limit, offset int) ([]models.Complaint, int64, error)
	UpdateStatus(id uuid.UUID, status, resolution string) error
}
