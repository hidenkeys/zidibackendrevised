package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type UserRepoPG struct {
	db *gorm.DB
}

func NewUserRepoPG(db *gorm.DB) UserRepository {
	return &UserRepoPG{db: db}
}

func (r *UserRepoPG) Create(user *models.User) (*models.User, error) {
	err := r.db.Create(user).Error
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *UserRepoPG) CreateWithStoreAssignment(ctx context.Context, user *models.User, assignment *models.CommerceStaffStoreAssignment) (*models.User, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var store models.CommerceStore
		if err := tx.Where("id = ? AND organization_id = ? AND status = ?", assignment.StoreID, assignment.OrganizationID, models.CommerceStatusActive).First(&store).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCommerceNotFound
			}
			return fmt.Errorf("find store for staff assignment: %w", err)
		}
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		assignment.UserID = user.ID
		return tx.Create(assignment).Error
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *UserRepoPG) GetByID(id uuid.UUID) (*models.User, error) {
	user := &models.User{}
	err := r.db.Where("id = ?", id).First(user).Error
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *UserRepoPG) GetByEmail(email string) (*models.User, error) {
	user := &models.User{}
	if err := r.db.Where("email = ?", email).First(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

func (r *UserRepoPG) UpdateByID(id uuid.UUID, user *models.User) (*models.User, error) {
	err := r.db.Model(&models.User{}).Where("id = ?", id).Updates(user).Error
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *UserRepoPG) DeleteByID(id uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&models.CommerceStaffStoreAssignment{}).
			Where("user_id = ? AND deleted_at IS NULL", id).
			Updates(map[string]interface{}{
				"status":     models.CommerceStatusInactive,
				"deleted_at": now,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("deactivate staff store assignments: %w", err)
		}
		if err := tx.Delete(&models.User{}, "id = ?", id).Error; err != nil {
			return fmt.Errorf("delete user: %w", err)
		}
		return nil
	})
}

// ✅ Updated GetAll with Pagination
func (r *UserRepoPG) GetAll(limit, offset int) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	// Count total users
	if err := r.db.Model(&models.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply limit and offset
	err := r.db.Limit(limit).Offset(offset).Find(&users).Error
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// ✅ Updated GetAllByOrganizationID with Pagination
func (r *UserRepoPG) GetAllByOrganizationID(orgID uuid.UUID, limit, offset int) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	// Count total users in the organization
	if err := r.db.Model(&models.User{}).Where("organization_id = ?", orgID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply limit and offset
	err := r.db.Where("organization_id = ?", orgID).Limit(limit).Offset(offset).Find(&users).Error
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *UserRepoPG) UpdatePasswordByID(id uuid.UUID, password string) error {
	err := r.db.Model(&models.User{}).Where("id = ?", id).Update("password", password).Error
	if err != nil {
		return err
	}
	return nil
}
