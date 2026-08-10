package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type CustomerPG struct {
	db *gorm.DB
}

func NewCustomerRepoPG(db *gorm.DB) *CustomerPG {
	return &CustomerPG{db: db}
}

func (r *CustomerPG) Create(customer *models.Customer) (*models.Customer, error) {
	if err := r.db.Create(customer).Error; err != nil {
		return nil, err
	}
	return customer, nil
}

func (r *CustomerPG) UpdateByID(id uuid.UUID, customer *models.Customer) (*models.Customer, error) {
	if err := r.db.Model(&models.Customer{}).Where("id = ?", id).Updates(customer).Error; err != nil {
		return nil, err
	}
	return customer, nil
}

func (r *CustomerPG) DeleteByID(id uuid.UUID) error {
	if err := r.db.Delete(&models.Customer{}, "id = ?", id).Error; err != nil {
		return err
	}
	return nil
}

func (r *CustomerPG) GetByID(id uuid.UUID) (*models.Customer, error) {
	var customer models.Customer
	if err := r.db.First(&customer, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &customer, nil
}

func (r *CustomerPG) GetAll(limit, offset int) ([]models.Customer, int64, error) {
	var customers []models.Customer
	var total int64

	// Count total customers
	if err := r.db.Model(&models.Customer{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply order, limit and offset for pagination
	if err := r.db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&customers).Error; err != nil {
		return nil, 0, err
	}

	return customers, total, nil
}

func (r *CustomerPG) GetAllByOrganization(orgID uuid.UUID, limit, offset int) ([]models.Customer, int64, error) {
	var customers []models.Customer
	var total int64

	// Count total customers for the organization
	if err := r.db.Model(&models.Customer{}).Where("organization_id = ?", orgID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply order, limit and offset for pagination
	if err := r.db.Where("organization_id = ?", orgID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&customers).Error; err != nil {
		return nil, 0, err
	}

	return customers, total, nil
}

func (r *CustomerPG) GetAllByCampaign(campaignID uuid.UUID, limit, offset int) ([]models.Customer, int64, error) {
	var customers []models.Customer
	var total int64

	// Count total customers for the campaign
	if err := r.db.Model(&models.Customer{}).Where("campaign_id = ?", campaignID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply order, limit and offset for pagination
	if err := r.db.Where("campaign_id = ?", campaignID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&customers).Error; err != nil {
		return nil, 0, err
	}

	return customers, total, nil
}
