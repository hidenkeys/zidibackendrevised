package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type InstitutionRepoPG struct {
	db *gorm.DB
}

func NewInstitutionRepoPG(db *gorm.DB) *InstitutionRepoPG {
	return &InstitutionRepoPG{db: db}
}

func (r *InstitutionRepoPG) Create(institution *models.Institution) (*models.Institution, error) {
	if err := r.db.Create(institution).Error; err != nil {
		return nil, err
	}
	return institution, nil
}

func (r *InstitutionRepoPG) GetByID(id uuid.UUID) (*models.Institution, error) {
	var institution models.Institution
	if err := r.db.Where("id = ?", id).First(&institution).Error; err != nil {
		return nil, err
	}
	return &institution, nil
}

func (r *InstitutionRepoPG) GetByName(name string) (*models.Institution, error) {
	var institution models.Institution
	if err := r.db.Where("name = ?", name).First(&institution).Error; err != nil {
		return nil, err
	}
	return &institution, nil
}

func (r *InstitutionRepoPG) UpdateByID(id uuid.UUID, institution *models.Institution) (*models.Institution, error) {
	if err := r.db.Model(&models.Institution{}).Where("id = ?", id).Updates(institution).Error; err != nil {
		return nil, err
	}
	return institution, nil
}

func (r *InstitutionRepoPG) DeleteByID(id uuid.UUID) error {
	if err := r.db.Where("id = ?", id).Delete(&models.Institution{}).Error; err != nil {
		return err
	}
	return nil
}
