package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type ProductRepoPG struct {
	db *gorm.DB
}

func NewProductRepoPG(db *gorm.DB) *ProductRepoPG {
	return &ProductRepoPG{db: db}
}

func (r *ProductRepoPG) Create(product *models.Product) (*models.Product, error) {
	if err := r.db.Create(product).Error; err != nil {
		return nil, err
	}
	return product, nil
}

func (r *ProductRepoPG) GetByID(id uuid.UUID) (*models.Product, error) {
	var product models.Product
	if err := r.db.Where("id = ?", id).First(&product).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *ProductRepoPG) GetByInstitutionID(institutionID uuid.UUID, categoryID string, limit, offset int) ([]models.Product, int64, error) {
	var products []models.Product
	var total int64

	query := r.db.Model(&models.Product{}).Where("institution_id = ?", institutionID)
	if categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Limit(limit).Offset(offset).Find(&products).Error; err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

func (r *ProductRepoPG) Update(product *models.Product) (*models.Product, error) {
	if err := r.db.Save(product).Error; err != nil {
		return nil, err
	}
	return product, nil
}

func (r *ProductRepoPG) Delete(id uuid.UUID) error {
	if err := r.db.Where("id = ?", id).Delete(&models.Product{}).Error; err != nil {
		return err
	}
	return nil
}

func (r *ProductRepoPG) GetBySKU(institutionID uuid.UUID, sku string) (*models.Product, error) {
	var product models.Product
	if err := r.db.Where("institution_id = ? AND sku = ?", institutionID, sku).First(&product).Error; err != nil {
		return nil, err
	}
	return &product, nil
}
