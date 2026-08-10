package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type ComplaintRepoPG struct {
	db *gorm.DB
}

func NewComplaintRepoPG(db *gorm.DB) *ComplaintRepoPG {
	return &ComplaintRepoPG{db: db}
}

func (r *ComplaintRepoPG) Create(complaint *models.Complaint) (*models.Complaint, error) {
	if err := r.db.Create(complaint).Error; err != nil {
		return nil, err
	}
	return complaint, nil
}

func (r *ComplaintRepoPG) GetByID(id uuid.UUID) (*models.Complaint, error) {
	var complaint models.Complaint
	if err := r.db.Where("id = ?", id).First(&complaint).Error; err != nil {
		return nil, err
	}
	return &complaint, nil
}

func (r *ComplaintRepoPG) GetByClientID(clientID uuid.UUID, limit, offset int) ([]models.Complaint, int64, error) {
	var complaints []models.Complaint
	var total int64

	if err := r.db.Model(&models.Complaint{}).Where("client_id = ?", clientID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.Where("client_id = ?", clientID).Limit(limit).Offset(offset).Order("created_at desc").Find(&complaints).Error; err != nil {
		return nil, 0, err
	}

	return complaints, total, nil
}

func (r *ComplaintRepoPG) UpdateStatus(id uuid.UUID, status, resolution string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if resolution != "" {
		updates["resolution"] = resolution
	}
	return r.db.Model(&models.Complaint{}).Where("id = ?", id).Updates(updates).Error
}
