package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type PaymentRepoPG struct {
	db *gorm.DB
}

func NewPaymentRepoPG(db *gorm.DB) *PaymentRepoPG {
	return &PaymentRepoPG{db: db}
}

func (r *PaymentRepoPG) Create(payment *models.Payment) (*models.Payment, error) {
	if err := r.db.Create(payment).Error; err != nil {
		return nil, err
	}
	return payment, nil
}

func (r *PaymentRepoPG) GetAll(limit, offset int) ([]models.Payment, int64, error) {
	var payments []models.Payment
	var total int64

	r.db.Model(&models.Payment{}).Count(&total)

	if err := r.db.Limit(limit).Offset(offset).Find(&payments).Error; err != nil {
		return nil, 0, err
	}
	return payments, total, nil
}

func (r *PaymentRepoPG) GetAllByOrganization(orgID uuid.UUID, limit, offset int) ([]models.Payment, int64, error) {
	var payments []models.Payment
	var total int64

	r.db.Model(&models.Payment{}).Where("organization_id = ?", orgID).Count(&total)

	if err := r.db.Where("organization_id = ?", orgID).Limit(limit).Offset(offset).Find(&payments).Error; err != nil {
		return nil, 0, err
	}
	return payments, total, nil
}

func (r *PaymentRepoPG) GetByID(id uuid.UUID) (*models.Payment, error) {
	payment := &models.Payment{}
	if err := r.db.Where("id = ?", id).First(payment).Error; err != nil {
		return nil, err
	}
	return payment, nil
}

func (r *PaymentRepoPG) UpdateByID(id uuid.UUID, payment *models.Payment) (*models.Payment, error) {
	if err := r.db.Model(&models.Payment{}).Where("id = ?", id).Updates(payment).Error; err != nil {
		return nil, err
	}
	return payment, nil
}

func (r *PaymentRepoPG) DeleteByID(id uuid.UUID) error {
	if err := r.db.Where("id = ?", id).Delete(&models.Payment{}).Error; err != nil {
		return err
	}
	return nil
}

func (r *PaymentRepoPG) GetByTransactionRef(ref string) (*models.Payment, error) {
	payment := &models.Payment{}
	if err := r.db.Where("transaction_ref = ?", ref).First(payment).Error; err != nil {
		return nil, err
	}
	return payment, nil
}
