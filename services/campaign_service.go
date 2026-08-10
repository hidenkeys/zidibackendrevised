package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type CampaignService struct {
	campaignRepo repository.CampaignRepository
}

func NewCampaignService(campaignRepo repository.CampaignRepository) *CampaignService {
	return &CampaignService{campaignRepo: campaignRepo}
}

func (s *CampaignService) CreateCoupon(ctx context.Context, req *api.Coupon) (*api.Coupon, error) {
	newCoupon := &models.Coupon{
		ID:         req.Id,
		CampaignID: req.CampaignId,
		Code:       req.Code,
		Redeemed:   req.Redeemed,
		RedeemedAt: req.RedeemedAt,
	}

	coupon, err := s.campaignRepo.CreateCoupon(newCoupon)
	if err != nil {
		return nil, err
	}
	finalCoupon := &api.Coupon{
		Id:         coupon.ID,
		Code:       coupon.Code,
		Redeemed:   coupon.Redeemed,
		RedeemedAt: coupon.RedeemedAt,
		CampaignId: coupon.CampaignID,
	}
	return finalCoupon, nil
}

func (s *CampaignService) CreateCampaign(ctx context.Context, req api.Campaign) (*api.Campaign, error) {
	newCampaign := &models.Campaign{
		ID:                    req.Id,
		CampaignName:          req.CampaignName,
		CouponID:              req.CouponId,
		CharacterType:         req.CharacterType,
		CouponLength:          req.CouponLength,
		CouponNumber:          req.CouponNumber,
		OrganizationID:        req.OrganizationId,
		WelcomeMessage:        req.WelcomeMessage,
		QuestionNumber:        req.QuestionNumber,
		Amount:                float64(req.Amount),
		Price:                 float64(req.Price),
		Status:                req.Status,
		CommunicationChannels: req.CommunicationChannels,
		CreatedAt:             time.Now(),
	}

	campaign, err := s.campaignRepo.Create(newCampaign)
	if err != nil {
		return nil, err
	}

	return mapToAPICampaign(campaign), nil
}

func (s *CampaignService) GetAllCampaigns(ctx context.Context, limit, offset int) ([]api.Campaign, int64, error) {
	campaigns, count, err := s.campaignRepo.GetAll(limit, offset)
	if err != nil {
		return nil, count, err
	}

	var finalCampaigns []api.Campaign
	for _, campaign := range campaigns {
		finalCampaigns = append(finalCampaigns, *mapToAPICampaign(&campaign))
	}

	return finalCampaigns, count, nil
}

func (s *CampaignService) GetAllCoupons(ctx context.Context, campaignId uuid.UUID) ([]api.Coupon, error) {
	coupons, err := s.campaignRepo.GetCouponByCampaign(campaignId)
	if err != nil {
		return nil, err
	}
	var finalCoupons []api.Coupon
	for _, coupon := range coupons {
		finalCoupons = append(finalCoupons, api.Coupon{
			Id:         coupon.ID,
			CampaignId: coupon.CampaignID,
			Code:       coupon.Code,
			Redeemed:   coupon.Redeemed,
			RedeemedAt: coupon.RedeemedAt,
		})
	}

	return finalCoupons, nil
}

func (s *CampaignService) GetCampaignsByOrganization(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]api.Campaign, int64, error) {
	campaigns, count, err := s.campaignRepo.GetAllByOrganization(orgID, limit, offset)
	if err != nil {
		return nil, count, err
	}

	var finalCampaigns []api.Campaign
	for _, campaign := range campaigns {
		finalCampaigns = append(finalCampaigns, *mapToAPICampaign(&campaign))
	}

	return finalCampaigns, count, nil
}

func (s *CampaignService) GetCampaignByID(ctx context.Context, id uuid.UUID) (*api.Campaign, error) {
	campaign, err := s.campaignRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	return mapToAPICampaign(campaign), nil
}

func (s *CampaignService) UpdateCampaign(ctx context.Context, id uuid.UUID, req *api.Campaign) (*api.Campaign, error) {
	var createdAt time.Time
	if req.CreatedAt != nil {
		createdAt = *req.CreatedAt
	} else {
		createdAt = time.Now() // or some default value
	}

	updateData := &models.Campaign{
		CampaignName:          req.CampaignName,
		CouponID:              req.CouponId,
		CharacterType:         req.CharacterType,
		CouponLength:          req.CouponLength,
		CouponNumber:          req.CouponNumber,
		OrganizationID:        req.OrganizationId,
		WelcomeMessage:        req.WelcomeMessage,
		QuestionNumber:        req.QuestionNumber,
		Amount:                float64(req.Amount),
		Price:                 float64(req.Price),
		Status:                req.Status,
		CommunicationChannels: req.CommunicationChannels,
		CreatedAt:             createdAt,
	}
	updatedCampaign, err := s.campaignRepo.UpdateByID(id, updateData)
	if err != nil {
		return nil, err
	}
	return mapToAPICampaign(updatedCampaign), nil
}

func (s *CampaignService) DeleteCampaign(ctx context.Context, id uuid.UUID) error {
	return s.campaignRepo.DeleteByID(id)
}

// FindCampaignByName searches for an active campaign by its human-readable name
// Uses fuzzy matching to find campaigns from natural user messages
func (s *CampaignService) FindCampaignByName(ctx context.Context, campaignName string) (*api.Campaign, error) {
	campaign, err := s.campaignRepo.FindByNameFuzzy(campaignName)
	if err != nil {
		return nil, err
	}
	return mapToAPICampaign(campaign), nil
}

// Helper function to convert models.Campaign to api.Campaign
func mapToAPICampaign(campaign *models.Campaign) *api.Campaign {
	return &api.Campaign{
		Id:                    campaign.ID,
		CampaignName:          campaign.CampaignName,
		CouponId:              campaign.CouponID,
		CharacterType:         campaign.CharacterType,
		CouponLength:          campaign.CouponLength,
		CouponNumber:          campaign.CouponNumber,
		OrganizationId:        campaign.OrganizationID,
		WelcomeMessage:        campaign.WelcomeMessage,
		QuestionNumber:        campaign.QuestionNumber,
		Amount:                float32(campaign.Amount),
		Price:                 float32(campaign.Price),
		Status:                campaign.Status,
		CommunicationChannels: campaign.CommunicationChannels,
		CreatedAt:             &campaign.CreatedAt,
	}
}
