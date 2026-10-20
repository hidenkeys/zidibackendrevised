package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var (
	ErrCommerceNotFound = errors.New("commerce resource not found")
	ErrCommerceConflict = errors.New("commerce resource already exists")
)

type CommerceFoundationRepository interface {
	OrganizationExists(ctx context.Context, organizationID uuid.UUID) (bool, error)
	UserRoleInOrganization(ctx context.Context, organizationID, userID uuid.UUID) (string, error)
	CreateMerchantProfile(ctx context.Context, profile *models.CommerceMerchantProfile) error
	UpdateMerchantProfile(ctx context.Context, profile *models.CommerceMerchantProfile) error
	GetMerchantProfile(ctx context.Context, organizationID uuid.UUID) (*models.CommerceMerchantProfile, error)
	CreateStore(ctx context.Context, store *models.CommerceStore, hours []models.CommerceStoreHour, modes []models.CommerceStoreFulfilmentMode) error
	UpdateStore(ctx context.Context, store *models.CommerceStore, hours []models.CommerceStoreHour, modes []models.CommerceStoreFulfilmentMode) error
	ListStores(ctx context.Context, organizationID uuid.UUID, assignedUserID *uuid.UUID) ([]models.CommerceStore, error)
	GetStore(ctx context.Context, organizationID, storeID uuid.UUID, assignedUserID *uuid.UUID) (*models.CommerceStore, error)
	CreateStaffAssignment(ctx context.Context, assignment *models.CommerceStaffStoreAssignment) error
	ListStaffAssignments(ctx context.Context, organizationID, storeID uuid.UUID) ([]models.CommerceStaffStoreAssignment, error)
}

type CommerceFoundationRepoPG struct {
	db *gorm.DB
}

func NewCommerceFoundationRepoPG(db *gorm.DB) *CommerceFoundationRepoPG {
	return &CommerceFoundationRepoPG{db: db}
}

func (r *CommerceFoundationRepoPG) OrganizationExists(ctx context.Context, organizationID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.Organization{}).Where("id = ?", organizationID).Count(&count).Error
	return count > 0, err
}

func (r *CommerceFoundationRepoPG) UserRoleInOrganization(ctx context.Context, organizationID, userID uuid.UUID) (string, error) {
	var user models.User
	err := r.db.WithContext(ctx).
		Select("role").
		Where("id = ? AND organization_id = ?", userID, organizationID).
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrCommerceNotFound
	}
	if err != nil {
		return "", fmt.Errorf("find organization user: %w", err)
	}
	return user.Role, nil
}

func (r *CommerceFoundationRepoPG) CreateMerchantProfile(ctx context.Context, profile *models.CommerceMerchantProfile) error {
	if err := r.db.WithContext(ctx).Create(profile).Error; err != nil {
		return mapCommerceWriteError("create merchant profile", err)
	}
	return nil
}

func (r *CommerceFoundationRepoPG) UpdateMerchantProfile(ctx context.Context, profile *models.CommerceMerchantProfile) error {
	result := r.db.WithContext(ctx).Model(&models.CommerceMerchantProfile{}).
		Where("id = ? AND organization_id = ?", profile.ID, profile.OrganizationID).
		Updates(map[string]interface{}{
			"slug": profile.Slug, "display_name": profile.DisplayName, "default_currency": profile.DefaultCurrency,
			"timezone": profile.Timezone, "status": profile.Status,
		})
	if result.Error != nil {
		return mapCommerceWriteError("update merchant profile", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrCommerceNotFound
	}
	return nil
}

func (r *CommerceFoundationRepoPG) GetMerchantProfile(ctx context.Context, organizationID uuid.UUID) (*models.CommerceMerchantProfile, error) {
	var profile models.CommerceMerchantProfile
	err := r.db.WithContext(ctx).Where("organization_id = ?", organizationID).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get merchant profile: %w", err)
	}
	return &profile, nil
}

func (r *CommerceFoundationRepoPG) CreateStore(ctx context.Context, store *models.CommerceStore, hours []models.CommerceStoreHour, modes []models.CommerceStoreFulfilmentMode) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(store).Error; err != nil {
			return err
		}
		for index := range hours {
			hours[index].OrganizationID = store.OrganizationID
			hours[index].StoreID = store.ID
		}
		if len(hours) > 0 {
			if err := tx.Create(&hours).Error; err != nil {
				return err
			}
		}
		for index := range modes {
			modes[index].OrganizationID = store.OrganizationID
			modes[index].StoreID = store.ID
		}
		if len(modes) > 0 {
			if err := tx.Create(&modes).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return mapCommerceWriteError("create store", err)
	}
	store.Hours = hours
	store.FulfilmentModes = modes
	return nil
}

func (r *CommerceFoundationRepoPG) UpdateStore(ctx context.Context, store *models.CommerceStore, hours []models.CommerceStoreHour, modes []models.CommerceStoreFulfilmentMode) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.CommerceStore{}).
			Where("id = ? AND organization_id = ?", store.ID, store.OrganizationID).
			Updates(map[string]interface{}{
				"code": store.Code, "name": store.Name, "address": store.Address, "city": store.City, "state": store.State,
				"country_code": store.CountryCode, "latitude": store.Latitude, "longitude": store.Longitude,
				"timezone": store.Timezone, "preparation_minutes": store.PreparationMinutes, "status": store.Status,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrCommerceNotFound
		}
		if err := tx.Where("organization_id = ? AND store_id = ?", store.OrganizationID, store.ID).Delete(&models.CommerceStoreHour{}).Error; err != nil {
			return err
		}
		if err := tx.Where("organization_id = ? AND store_id = ?", store.OrganizationID, store.ID).Delete(&models.CommerceStoreFulfilmentMode{}).Error; err != nil {
			return err
		}
		for i := range hours {
			hours[i].OrganizationID = store.OrganizationID
			hours[i].StoreID = store.ID
		}
		if len(hours) > 0 {
			if err := tx.Create(&hours).Error; err != nil {
				return err
			}
		}
		for i := range modes {
			modes[i].OrganizationID = store.OrganizationID
			modes[i].StoreID = store.ID
		}
		if len(modes) > 0 {
			if err := tx.Create(&modes).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return mapCommerceWriteError("update store", err)
	}
	store.Hours = hours
	store.FulfilmentModes = modes
	return nil
}

func (r *CommerceFoundationRepoPG) ListStores(ctx context.Context, organizationID uuid.UUID, assignedUserID *uuid.UUID) ([]models.CommerceStore, error) {
	query := r.storeQuery(ctx).Where("commerce_stores.organization_id = ?", organizationID)
	if assignedUserID != nil {
		query = query.Joins(`
			JOIN commerce_staff_store_assignments assignments
			  ON assignments.store_id = commerce_stores.id
			 AND assignments.organization_id = commerce_stores.organization_id
			 AND assignments.user_id = ?
			 AND assignments.status = ?
			 AND assignments.deleted_at IS NULL
		`, *assignedUserID, models.CommerceStatusActive)
	}

	var stores []models.CommerceStore
	if err := query.Order("commerce_stores.name ASC").Find(&stores).Error; err != nil {
		return nil, fmt.Errorf("list stores: %w", err)
	}
	return stores, nil
}

func (r *CommerceFoundationRepoPG) GetStore(ctx context.Context, organizationID, storeID uuid.UUID, assignedUserID *uuid.UUID) (*models.CommerceStore, error) {
	query := r.storeQuery(ctx).
		Where("commerce_stores.organization_id = ? AND commerce_stores.id = ?", organizationID, storeID)
	if assignedUserID != nil {
		query = query.Joins(`
			JOIN commerce_staff_store_assignments assignments
			  ON assignments.store_id = commerce_stores.id
			 AND assignments.organization_id = commerce_stores.organization_id
			 AND assignments.user_id = ?
			 AND assignments.status = ?
			 AND assignments.deleted_at IS NULL
		`, *assignedUserID, models.CommerceStatusActive)
	}

	var store models.CommerceStore
	err := query.First(&store).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get store: %w", err)
	}
	return &store, nil
}

func (r *CommerceFoundationRepoPG) CreateStaffAssignment(ctx context.Context, assignment *models.CommerceStaffStoreAssignment) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var storeCount int64
		if err := tx.Model(&models.CommerceStore{}).
			Where("id = ? AND organization_id = ?", assignment.StoreID, assignment.OrganizationID).
			Count(&storeCount).Error; err != nil {
			return err
		}
		if storeCount == 0 {
			return ErrCommerceNotFound
		}

		var userCount int64
		if err := tx.Model(&models.User{}).
			Where("id = ? AND organization_id = ?", assignment.UserID, assignment.OrganizationID).
			Count(&userCount).Error; err != nil {
			return err
		}
		if userCount == 0 {
			return ErrCommerceNotFound
		}
		return tx.Create(assignment).Error
	})
	if err != nil {
		return mapCommerceWriteError("create store staff assignment", err)
	}
	return nil
}

func (r *CommerceFoundationRepoPG) ListStaffAssignments(ctx context.Context, organizationID, storeID uuid.UUID) ([]models.CommerceStaffStoreAssignment, error) {
	var assignments []models.CommerceStaffStoreAssignment
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND store_id = ?", organizationID, storeID).
		Order("created_at ASC").
		Find(&assignments).Error
	if err != nil {
		return nil, fmt.Errorf("list store staff assignments: %w", err)
	}
	return assignments, nil
}

func (r *CommerceFoundationRepoPG) storeQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&models.CommerceStore{}).
		Preload("Hours", func(db *gorm.DB) *gorm.DB { return db.Order("day_of_week ASC") }).
		Preload("FulfilmentModes", func(db *gorm.DB) *gorm.DB { return db.Order("mode ASC") })
}

func mapCommerceWriteError(action string, err error) error {
	if errors.Is(err, ErrCommerceNotFound) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%s: %w", action, ErrCommerceConflict)
	}
	return fmt.Errorf("%s: %w", action, err)
}
