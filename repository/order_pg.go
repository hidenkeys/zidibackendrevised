package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type OrderRepoPG struct {
	db *gorm.DB
}

func NewOrderRepoPG(db *gorm.DB) *OrderRepoPG {
	return &OrderRepoPG{db: db}
}

// Create creates an order and its items in a transaction
func (r *OrderRepoPG) Create(order *models.Order) (*models.Order, error) {
	return order, r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		// OrderItems are created automatically via association if properly set up,
		// but checking if we need explicit handling or if GORM handles it.
		// Assuming GORM handles it since Items are passed in the struct.
		return nil
	})
}

func (r *OrderRepoPG) GetByID(id uuid.UUID) (*models.Order, error) {
	var order models.Order
	if err := r.db.Preload("Items").Where("id = ?", id).First(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *OrderRepoPG) GetByTrackingNumber(trackingNumber string) (*models.Order, error) {
	var order models.Order
	if err := r.db.Preload("Items").Where("tracking_number = ?", trackingNumber).First(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *OrderRepoPG) GetByClientID(clientID uuid.UUID, limit, offset int) ([]models.Order, int64, error) {
	var orders []models.Order
	var total int64

	if err := r.db.Model(&models.Order{}).Where("client_id = ?", clientID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.Preload("Items").Where("client_id = ?", clientID).Limit(limit).Offset(offset).Order("created_at desc").Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

func (r *OrderRepoPG) UpdateStatus(orderID uuid.UUID, status string) error {
	return r.db.Model(&models.Order{}).Where("id = ?", orderID).Update("status", status).Error
}

func (r *OrderRepoPG) AddTrackingUpdate(tracking *models.OrderTracking) error {
	return r.db.Create(tracking).Error
}

func (r *OrderRepoPG) GetTrackingHistory(orderID uuid.UUID) ([]models.OrderTracking, error) {
	var history []models.OrderTracking
	if err := r.db.Where("order_id = ?", orderID).Order("timestamp desc").Find(&history).Error; err != nil {
		return nil, err
	}
	return history, nil
}
