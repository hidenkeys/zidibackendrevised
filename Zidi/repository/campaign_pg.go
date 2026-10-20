package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type CampaignRepoPG struct {
	db *gorm.DB
}

func NewCampaignRepoPG(db *gorm.DB) *CampaignRepoPG {
	return &CampaignRepoPG{db: db}
}

func (r *CampaignRepoPG) Create(campaign *models.Campaign) (*models.Campaign, error) {
	err := r.db.Create(campaign)
	if err.Error != nil {
		return nil, err.Error
	}
	return campaign, nil
}

func (r *CampaignRepoPG) CreateCoupon(coupon *models.Coupon) (*models.Coupon, error) {
	err := r.db.Create(coupon)
	if err.Error != nil {
		return nil, err.Error
	}
	return coupon, nil
}

func (r *CampaignRepoPG) GetAll(limit, offset int) ([]models.Campaign, int64, error) {
	var campaigns []models.Campaign
	var total int64

	// Count total campaigns
	r.db.Model(&models.Campaign{}).Count(&total)

	// Retrieve paginated results
	err := r.db.Limit(limit).Offset(offset).Find(&campaigns)
	if err.Error != nil {
		return nil, 0, err.Error
	}
	return campaigns, total, nil
}

func (r *CampaignRepoPG) GetAllByOrganization(orgID uuid.UUID, limit, offset int) ([]models.Campaign, int64, error) {
	var campaigns []models.Campaign
	var total int64

	// Count total campaigns for the organization
	r.db.Model(&models.Campaign{}).Where("organization_id = ?", orgID).Count(&total)

	// Retrieve paginated results
	err := r.db.Where("organization_id = ?", orgID).Limit(limit).Offset(offset).Find(&campaigns)
	if err.Error != nil {
		return nil, 0, err.Error
	}
	return campaigns, total, nil
}

func (r *CampaignRepoPG) GetCouponByCampaign(campaignID uuid.UUID) ([]models.Coupon, error) {
	var coupons []models.Coupon
	err := r.db.Where("campaign_id = ?", campaignID).Find(&coupons)
	if err.Error != nil {
		return nil, err.Error
	}
	return coupons, nil
}

func (r *CampaignRepoPG) GetByID(id uuid.UUID) (*models.Campaign, error) {
	campaign := &models.Campaign{}
	err := r.db.Where("id = ?", id).First(campaign)
	if err.Error != nil {
		return nil, err.Error
	}
	return campaign, nil
}

func (r *CampaignRepoPG) UpdateByID(id uuid.UUID, campaign *models.Campaign) (*models.Campaign, error) {
	if err := r.db.Model(&models.Campaign{}).Where("id = ?", id).Updates(campaign).Error; err != nil {
		return nil, err
	}
	return campaign, nil
}

func (r *CampaignRepoPG) DeleteByID(id uuid.UUID) error {
	err := r.db.Where("id = ?", id).Delete(&models.Campaign{})
	if err.Error != nil {
		return err.Error
	}
	return nil
}

// FindByNameFuzzy searches for active campaigns by name using fuzzy matching
// It normalizes the input and performs a case-insensitive search with LIKE pattern matching
func (r *CampaignRepoPG) FindByNameFuzzy(campaignName string) (*models.Campaign, error) {
	campaign := &models.Campaign{}
	
	// First try exact match (case-insensitive)
	if err := r.db.Where("LOWER(campaign_name) = LOWER(?) AND status = ?", campaignName, "active").First(campaign).Error; err == nil {
		return campaign, nil
	}
	
	// Try LIKE match with the campaign name
	searchPattern := "%" + campaignName + "%"
	if err := r.db.Where("LOWER(campaign_name) LIKE LOWER(?) AND status = ?", searchPattern, "active").First(campaign).Error; err != nil {
		return nil, err
	}
	
	return campaign, nil
}
