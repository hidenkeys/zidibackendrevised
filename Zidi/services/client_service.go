package services

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type ClientService struct {
	clientRepo repository.ClientRepository
}

func NewClientService(clientRepo repository.ClientRepository) *ClientService {
	return &ClientService{clientRepo: clientRepo}
}

func (s *ClientService) RegisterClient(institutionID uuid.UUID, name, phone, email, ageRange string) (*models.Client, error) {
	// Check if already exists
	existing, _ := s.clientRepo.GetByPhone(institutionID, phone)
	if existing != nil {
		return existing, nil
	}

	client := &models.Client{
		InstitutionID:    institutionID,
		Name:             name,
		Phone:            phone,
		Email:            email,
		AgeRange:         ageRange,
		OnboardingStatus: "complete",
	}
	return s.clientRepo.Create(client)
}

func (s *ClientService) GetClientByPhone(institutionID uuid.UUID, phone string) (*models.Client, error) {
	return s.clientRepo.GetByPhone(institutionID, phone)
}
