package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type ClientRepoPG struct {
	db *gorm.DB
}

func NewClientRepoPG(db *gorm.DB) *ClientRepoPG {
	return &ClientRepoPG{db: db}
}

func (r *ClientRepoPG) Create(client *models.Client) (*models.Client, error) {
	if err := r.db.Create(client).Error; err != nil {
		return nil, err
	}
	return client, nil
}

func (r *ClientRepoPG) GetByID(id uuid.UUID) (*models.Client, error) {
	var client models.Client
	if err := r.db.Where("id = ?", id).First(&client).Error; err != nil {
		return nil, err
	}
	return &client, nil
}

func (r *ClientRepoPG) GetByPhone(institutionID uuid.UUID, phone string) (*models.Client, error) {
	var client models.Client
	if err := r.db.Where("institution_id = ? AND phone = ?", institutionID, phone).First(&client).Error; err != nil {
		return nil, err
	}
	return &client, nil
}

func (r *ClientRepoPG) GetByEmail(institutionID uuid.UUID, email string) (*models.Client, error) {
	var client models.Client
	if err := r.db.Where("institution_id = ? AND email = ?", institutionID, email).First(&client).Error; err != nil {
		return nil, err
	}
	return &client, nil
}

func (r *ClientRepoPG) Update(client *models.Client) (*models.Client, error) {
	if err := r.db.Save(client).Error; err != nil {
		return nil, err
	}
	return client, nil
}

func (r *ClientRepoPG) Delete(id uuid.UUID) error {
	if err := r.db.Where("id = ?", id).Delete(&models.Client{}).Error; err != nil {
		return err
	}
	return nil
}
