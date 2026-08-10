package services

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type ProductService struct {
	productRepo repository.ProductRepository
}

func NewProductService(productRepo repository.ProductRepository) *ProductService {
	return &ProductService{productRepo: productRepo}
}

func (s *ProductService) CreateProduct(product *models.Product) (*models.Product, error) {
	return s.productRepo.Create(product)
}

func (s *ProductService) GetProduct(id uuid.UUID) (*models.Product, error) {
	return s.productRepo.GetByID(id)
}

func (s *ProductService) ListProducts(institutionID uuid.UUID, categoryID string, limit, offset int) ([]models.Product, int64, error) {
	return s.productRepo.GetByInstitutionID(institutionID, categoryID, limit, offset)
}

func (s *ProductService) UpdateStock(productID uuid.UUID, quantityChange int) error {
	product, err := s.productRepo.GetByID(productID)
	if err != nil {
		return err
	}
	product.StockQuantity += quantityChange
	_, err = s.productRepo.Update(product)
	return err
}

func (s *ProductService) GetProductBySKU(institutionID uuid.UUID, sku string) (*models.Product, error) {
	return s.productRepo.GetBySKU(institutionID, sku)
}
