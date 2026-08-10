package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

type OrderRepository interface {
	Create(order *models.Order) (*models.Order, error)
	GetByID(id uuid.UUID) (*models.Order, error)
	GetByTrackingNumber(trackingNumber string) (*models.Order, error)
	GetByClientID(clientID uuid.UUID, limit, offset int) ([]models.Order, int64, error)
	UpdateStatus(orderID uuid.UUID, status string) error
	AddTrackingUpdate(tracking *models.OrderTracking) error
	GetTrackingHistory(orderID uuid.UUID) ([]models.OrderTracking, error)
}
