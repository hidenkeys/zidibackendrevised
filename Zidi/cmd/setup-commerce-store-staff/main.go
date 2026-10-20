package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/config"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/utils"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func main() {
	_ = godotenv.Load()
	organizationID, err := uuid.Parse(requiredEnvironment("BING_CHUN_ORGANIZATION_ID"))
	if err != nil {
		log.Fatal("BING_CHUN_ORGANIZATION_ID must be a valid UUID")
	}
	email := strings.ToLower(requiredEnvironment("COMMERCE_STAFF_EMAIL"))
	storeCode := strings.ToUpper(requiredEnvironment("COMMERCE_STAFF_STORE_CODE"))
	password := strings.TrimSpace(os.Getenv("COMMERCE_STAFF_PASSWORD"))
	generated := false
	if password == "" {
		password, err = generatePassword()
		if err != nil {
			log.Fatal(err)
		}
		generated = true
	}
	if len(password) < 12 {
		log.Fatal("COMMERCE_STAFF_PASSWORD must contain at least 12 characters")
	}

	config.ConnectDatabase()
	user, store, err := provision(config.DB, organizationID, email, password, storeCode)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("staff_ready organization=%s user=%s email=%s role=%s store=%s store_code=%s hours=24/7\n",
		organizationID, user.ID, user.Email, user.Role, store.ID, store.Code)
	if generated {
		fmt.Printf("temporary_password=%s\n", password)
	}
}

func provision(db *gorm.DB, organizationID uuid.UUID, email, password, storeCode string) (*models.User, *models.CommerceStore, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, fmt.Errorf("hash staff password: %w", err)
	}

	var user models.User
	var store models.CommerceStore
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization models.Organization
		if err := tx.Where("id = ?", organizationID).First(&organization).Error; err != nil {
			return fmt.Errorf("find organization: %w", err)
		}
		if err := tx.Where("organization_id = ? AND code = ?", organizationID, storeCode).First(&store).Error; err != nil {
			return fmt.Errorf("find store %s: %w", storeCode, err)
		}

		findErr := tx.Unscoped().Where("LOWER(email) = ?", email).First(&user).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find staff user: %w", findErr)
		}
		if findErr == nil && user.OrganizationID != organizationID {
			return fmt.Errorf("staff email already belongs to another organization")
		}
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			user = models.User{ID: uuid.New(), Email: email, OrganizationID: organizationID}
		}
		user.FirstName = "Bing Chun"
		user.LastName = "Store Staff"
		user.Password = string(hash)
		user.Role = utils.RoleStoreStaff
		user.OrganizationID = organizationID
		user.DeletedAt = gorm.DeletedAt{}
		if user.CreatedAt.IsZero() {
			if err := tx.Create(&user).Error; err != nil {
				return fmt.Errorf("create staff user: %w", err)
			}
		} else if err := tx.Unscoped().Save(&user).Error; err != nil {
			return fmt.Errorf("update staff user: %w", err)
		}

		var assignment models.CommerceStaffStoreAssignment
		assignmentErr := tx.Unscoped().Where(
			"organization_id = ? AND store_id = ? AND user_id = ?",
			organizationID, store.ID, user.ID,
		).First(&assignment).Error
		if assignmentErr != nil && !errors.Is(assignmentErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find staff assignment: %w", assignmentErr)
		}
		if errors.Is(assignmentErr, gorm.ErrRecordNotFound) {
			assignment = models.CommerceStaffStoreAssignment{
				ID: uuid.New(), OrganizationID: organizationID, StoreID: store.ID, UserID: user.ID,
			}
		}
		assignment.Role = utils.RoleStoreStaff
		assignment.Status = models.CommerceStatusActive
		assignment.DeletedAt = gorm.DeletedAt{}
		if assignment.CreatedAt.IsZero() {
			if err := tx.Create(&assignment).Error; err != nil {
				return fmt.Errorf("create staff assignment: %w", err)
			}
		} else if err := tx.Unscoped().Save(&assignment).Error; err != nil {
			return fmt.Errorf("update staff assignment: %w", err)
		}

		if err := tx.Model(&store).Updates(map[string]interface{}{
			"status": models.CommerceStatusActive, "updated_at": gorm.Expr("NOW()"),
		}).Error; err != nil {
			return fmt.Errorf("activate store: %w", err)
		}
		for day := 0; day < 7; day++ {
			if err := tx.Exec(`
				INSERT INTO commerce_store_hours
					(id, organization_id, store_id, day_of_week, open_minute, close_minute, is_closed, created_at, updated_at)
				VALUES (?, ?, ?, ?, 0, 1440, FALSE, NOW(), NOW())
				ON CONFLICT (organization_id, store_id, day_of_week) WHERE deleted_at IS NULL
				DO UPDATE SET open_minute = 0, close_minute = 1440, is_closed = FALSE, updated_at = NOW()
			`, uuid.New(), organizationID, store.ID, day).Error; err != nil {
				return fmt.Errorf("set 24/7 store hours for day %d: %w", day, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &user, &store, nil
}

func requiredEnvironment(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		log.Fatalf("%s is required", key)
	}
	return value
}

func generatePassword() (string, error) {
	buffer := make([]byte, 18)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate staff password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
