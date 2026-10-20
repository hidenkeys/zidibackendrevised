package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type TransactionService struct {
	transactionRepo repository.TransactionRepository
}

func NewTransactionService(transactionRepo repository.TransactionRepository) *TransactionService {
	return &TransactionService{transactionRepo: transactionRepo}
}

func (s *TransactionService) CreateTransaction(ctx context.Context, req *api.TransactionInput) (*api.Transaction, error) {
	newTransaction := &models.Transaction{
		ID:             uuid.New(),
		OrganizationID: req.OrganizationId,
		CampaignID:     req.CampaignId,
		Amount:         float64(req.Amount),
		Type:           string(req.Type),
		Commisson:      float64(req.Commisson),
		Network:        req.Network,
		PhoneNumber:    req.PhoneNumber,
		CustomerID:     req.CustomerId,
		TxReference:    req.TxReference,
		Status:         string(req.Status),
	}

	transaction, err := s.transactionRepo.Create(newTransaction)
	if err != nil {
		return nil, err
	}

	return mapToAPITransaction(transaction), nil
}

func (s *TransactionService) GetAllTransactions(ctx context.Context, limit, offset int) ([]api.Transaction, int64, error) {
	transactions, count, err := s.transactionRepo.GetAll(limit, offset)
	if err != nil {
		return nil, count, err
	}

	var finalTransactions []api.Transaction
	for _, transaction := range transactions {
		finalTransactions = append(finalTransactions, *mapToAPITransaction(&transaction))
	}

	return finalTransactions, count, nil
}

func (s *TransactionService) GetTransactionsByOrganization(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]api.Transaction, int64, error) {
	transactions, count, err := s.transactionRepo.GetByOrganization(orgID, limit, offset)
	if err != nil {
		return nil, count, err
	}

	var finalTransactions []api.Transaction
	for _, transaction := range transactions {
		finalTransactions = append(finalTransactions, *mapToAPITransaction(&transaction))
	}

	return finalTransactions, count, nil
}

func (s *TransactionService) GetTransactionByID(ctx context.Context, id uuid.UUID) (*api.Transaction, error) {
	transaction, err := s.transactionRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return mapToAPITransaction(transaction), nil
}

func (s *TransactionService) GetTransactionByReference(ctx context.Context, reference string) (*api.Transaction, error) {
	transaction, err := s.transactionRepo.GetByReference(reference)
	if err != nil {
		return nil, err
	}
	return mapToAPITransaction(transaction), nil
}

func (s *TransactionService) UpdateTransactionStatus(ctx context.Context, id uuid.UUID, status string) (*api.Transaction, error) {
	updatedTransaction, err := s.transactionRepo.UpdateStatus(id, status)
	if err != nil {
		return nil, err
	}
	return mapToAPITransaction(updatedTransaction), nil
}

// Helper function to convert models.Transaction to api.Transaction
func mapToAPITransaction(transaction *models.Transaction) *api.Transaction {
	return &api.Transaction{
		Id:             transaction.ID,
		OrganizationId: transaction.OrganizationID,
		CampaignId:     transaction.CampaignID,
		Amount:         float32(transaction.Amount),
		Type:           api.TransactionType(transaction.Type),
		CustomerId:     transaction.CustomerID,
		Commisson:      float32(transaction.Commisson),
		Network:        transaction.Network,
		PhoneNumber:    transaction.PhoneNumber,
		TxReference:    transaction.TxReference,
		Status:         api.TransactionStatus(transaction.Status),
	}
}
