package services

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type ComplaintService struct {
	complaintRepo repository.ComplaintRepository
}

func NewComplaintService(complaintRepo repository.ComplaintRepository) *ComplaintService {
	return &ComplaintService{complaintRepo: complaintRepo}
}

func (s *ComplaintService) LodgeComplaint(institutionID, clientID uuid.UUID, category, description string) (*models.Complaint, error) {
	complaint := &models.Complaint{
		InstitutionID: institutionID,
		ClientID:      clientID,
		Category:      category,
		Description:   description,
		Status:        "open",
	}
	return s.complaintRepo.Create(complaint)
}

func (s *ComplaintService) GetClientComplaints(clientID uuid.UUID) ([]models.Complaint, int64, error) {
	return s.complaintRepo.GetByClientID(clientID, 10, 0) // Default limit 10
}
