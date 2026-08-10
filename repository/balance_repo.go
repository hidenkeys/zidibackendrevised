package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

type BalanceRepository interface {
	CreateBalance(balance *models.Balance) (*models.Balance, error)
	GetBalanceByCampaign(campaignId uuid.UUID) (*models.Balance, error)
	GetAllBalances(limit, offset int) ([]models.Balance, error)
	UpdateBalance(campaignId uuid.UUID, balance *models.Balance) (*models.Balance, error)
}
