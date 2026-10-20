package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type TransactionRepoPG struct {
	db *gorm.DB
}

func NewTransactionRepoPG(db *gorm.DB) *TransactionRepoPG {
	return &TransactionRepoPG{db: db}
}

func (r *TransactionRepoPG) Create(transaction *models.Transaction) (*models.Transaction, error) {
	err := r.db.Create(transaction)
	if err.Error != nil {
		return nil, err.Error
	}
	return transaction, nil
}

func (r *TransactionRepoPG) GetAll(limit, offset int) ([]models.Transaction, int64, error) {
	var transactions []models.Transaction
	var total int64

	r.db.Model(&models.Transaction{}).Count(&total)
	if err := r.db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&transactions).Error; err != nil {
		return nil, 0, err
	}
	return transactions, total, nil
}

func (r *TransactionRepoPG) GetByID(id uuid.UUID) (*models.Transaction, error) {
	transaction := &models.Transaction{}
	err := r.db.Where("id = ?", id).First(transaction)
	if err.Error != nil {
		return nil, err.Error
	}
	return transaction, nil
}

func (r *TransactionRepoPG) GetByReference(reference string) (*models.Transaction, error) {
	transaction := &models.Transaction{}
	err := r.db.Where("reference = ?", reference).First(transaction)
	if err.Error != nil {
		return nil, err.Error
	}
	return transaction, nil
}

func (r *TransactionRepoPG) GetByOrganization(orgID uuid.UUID, limit, offset int) ([]models.Transaction, int64, error) {
	var transactions []models.Transaction
	var total int64

	r.db.Model(&models.Transaction{}).Where("organization_id = ?", orgID).Count(&total)
	if err := r.db.Where("organization_id = ?", orgID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions).Error; err != nil {
		return nil, 0, err
	}
	return transactions, total, nil
}

func (r *TransactionRepoPG) UpdateStatus(id uuid.UUID, status string) (*models.Transaction, error) {
	transaction := &models.Transaction{}
	err := r.db.Model(transaction).Where("id = ?", id).Update("status", status)
	if err.Error != nil {
		return nil, err.Error
	}
	return transaction, nil
}
