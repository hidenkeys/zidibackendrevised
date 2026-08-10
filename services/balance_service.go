package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type BalanceService struct {
	balanceRepo  repository.BalanceRepository
	campaignRepo repository.CampaignRepository
}

func NewBalanceService(balanceRepo repository.BalanceRepository, campaignRepo repository.CampaignRepository) *BalanceService {
	return &BalanceService{
		balanceRepo:  balanceRepo,
		campaignRepo: campaignRepo,
	}
}
func (s *BalanceService) CreateBalance(ctx context.Context, req *api.Balance) (*api.Balance, error) {
	// Check if a balance already exists for this CampaignId
	existingBalance, err := s.balanceRepo.GetBalanceByCampaign(req.CampaignId)
	if err != nil {
		return nil, err
	}
	if existingBalance != nil {
		// Option 1: return the existing balance
		return mapToAPIBalance(existingBalance), nil

		// Option 2: return an error
		// return nil, fmt.Errorf("balance already exists for campaign ID %s", req.CampaignId.String())
	}

	// Create a new balance since none exists
	newBalance := &models.Balance{
		CampaignId:      req.CampaignId,
		StartingBalance: float64(req.StartingBalance),
		Amount:          float64(req.Amount),
	}

	createdBalance, err := s.balanceRepo.CreateBalance(newBalance)
	if err != nil {
		return nil, err
	}

	return mapToAPIBalance(createdBalance), nil
}

func (s *BalanceService) GetBalanceByCampaign(ctx context.Context, campaignId uuid.UUID) (*api.Balance, error) {
	balance, err := s.balanceRepo.GetBalanceByCampaign(campaignId)
	if err != nil {
		return nil, err
	}
	if balance == nil {
		return nil, nil
	}

	campaign, err := s.campaignRepo.GetByID(campaignId)
	if err != nil {
		return nil, err
	}

	bal := mapToAPIBalance(balance)
	if campaign != nil {
		bal.CampaignName = &campaign.CampaignName
	}

	return bal, nil
}

func (s *BalanceService) GetAllBalances(ctx context.Context, limit, offset int) ([]api.Balance, error) {
	balances, err := s.balanceRepo.GetAllBalances(limit, offset)
	if err != nil {
		return nil, err
	}

	var finalBalances []api.Balance
	for _, balance := range balances {
		campaign, err := s.campaignRepo.GetByID(balance.CampaignId)
		if err != nil {
			return nil, err
		}

		campaignName := ""
		if campaign != nil {
			campaignName = campaign.CampaignName
		}

		bal := mapToAPIBalance(&balance)
		bal.CampaignName = &campaignName

		finalBalances = append(finalBalances, *bal)
	}

	return finalBalances, nil
}

func (s *BalanceService) UpdateBalance(ctx context.Context, campaignId uuid.UUID, amount float64) (*api.Balance, error) {
	updatedBalance, err := s.balanceRepo.UpdateBalance(campaignId, &models.Balance{Amount: amount})
	if err != nil {
		return nil, err
	}

	//updatedBalance, err := s.balanceRepo.GetByCampaignID(campaignId)
	//if err != nil {
	//	return nil, err
	//}

	return mapToAPIBalance(updatedBalance), nil
}

func mapToAPIBalance(balance *models.Balance) *api.Balance {
	return &api.Balance{
		Id:              balance.ID,
		CampaignId:      balance.CampaignId,
		StartingBalance: float32(balance.StartingBalance),
		Amount:          float32(balance.Amount),
	}
}
