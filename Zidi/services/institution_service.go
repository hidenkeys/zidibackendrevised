package services

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type InstitutionService struct {
	institutionRepo repository.InstitutionRepository
}

func NewInstitutionService(institutionRepo repository.InstitutionRepository) *InstitutionService {
	return &InstitutionService{institutionRepo: institutionRepo}
}

func (s *InstitutionService) CreateInstitution(name, contactName, contactPhone string) (*models.Institution, error) {
	inst := &models.Institution{
		Name:               name,
		ContactPersonName:  contactName,
		ContactPersonPhone: contactPhone,
	}
	return s.institutionRepo.Create(inst)
}

func (s *InstitutionService) GetInstitution(id uuid.UUID) (*models.Institution, error) {
	return s.institutionRepo.GetByID(id)
}
