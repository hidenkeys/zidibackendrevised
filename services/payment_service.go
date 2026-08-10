package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type PaymentService struct {
	paymentRepo repository.PaymentRepository
}

func NewPaymentService(paymentRepo repository.PaymentRepository) *PaymentService {
	return &PaymentService{paymentRepo: paymentRepo}
}

func (s *PaymentService) CreatePayment(ctx context.Context, req *api.Payment) (*api.Payment, error) {
	newPayment := &models.Payment{
		ID:             req.Id,
		OrganizationID: req.OrganizationId,
		CampaignID:     req.CampaignId,
		Amount:         float64(req.Amount),
		Status:         string(req.Status),
		TransactionRef: req.TransactionRef,
		TransactionID:  req.TransactionId,
	}

	payment, err := s.paymentRepo.Create(newPayment)
	if err != nil {
		return nil, err
	}

	return mapToAPIPayment(payment), nil
}

func (s *PaymentService) GetAllPayments(ctx context.Context, limit, offset int) ([]api.Payment, int64, error) {
	payments, count, err := s.paymentRepo.GetAll(limit, offset)
	if err != nil {
		return nil, count, err
	}

	var finalPayments []api.Payment
	for _, payment := range payments {
		finalPayments = append(finalPayments, *mapToAPIPayment(&payment))
	}

	return finalPayments, count, nil
}

func (s *PaymentService) GetPaymentsByOrganization(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]api.Payment, int64, error) {
	payments, count, err := s.paymentRepo.GetAllByOrganization(orgID, limit, offset)
	if err != nil {
		return nil, count, err
	}

	var finalPayments []api.Payment
	for _, payment := range payments {
		finalPayments = append(finalPayments, *mapToAPIPayment(&payment))
	}

	return finalPayments, count, nil
}

func (s *PaymentService) GetPaymentByID(ctx context.Context, id uuid.UUID) (*api.Payment, error) {
	payment, err := s.paymentRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	return mapToAPIPayment(payment), nil
}

func (s *PaymentService) UpdatePayment(ctx context.Context, id uuid.UUID, req *api.Payment) (*api.Payment, error) {
	updateData := &models.Payment{
		OrganizationID: req.OrganizationId,
		CampaignID:     req.CampaignId,
		Amount:         float64(req.Amount),
		Status:         string(req.Status),
		TransactionRef: req.TransactionRef,
		TransactionID:  req.TransactionId,
	}

	updatedPayment, err := s.paymentRepo.UpdateByID(id, updateData)
	if err != nil {
		return nil, err
	}

	return mapToAPIPayment(updatedPayment), nil
}

func (s *PaymentService) GetPaymentByTransactionRef(ctx context.Context, ref string) (*api.Payment, error) {
	payment, err := s.paymentRepo.GetByTransactionRef(ref)
	if err != nil {
		return nil, err
	}

	return mapToAPIPayment(payment), nil
}

// Helper function to convert models.Payment to api.Payment
func mapToAPIPayment(payment *models.Payment) *api.Payment {
	return &api.Payment{
		Id:             payment.ID,
		OrganizationId: payment.OrganizationID,
		CampaignId:     payment.CampaignID,
		Amount:         float32(payment.Amount),
		Status:         api.PaymentStatus(payment.Status),
		TransactionRef: payment.TransactionRef,
		TransactionId:  payment.TransactionID,
	}
}
