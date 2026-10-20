package main

import (
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
	email := strings.ToLower(requiredEnvironment("COMMERCE_ADMIN_EMAIL"))
	password := strings.TrimSpace(requiredEnvironment("COMMERCE_ADMIN_PASSWORD"))
	if len(password) < 12 {
		log.Fatal("COMMERCE_ADMIN_PASSWORD must contain at least 12 characters")
	}

	config.ConnectDatabase()
	user, err := upsertAdmin(config.DB, organizationID, email, password)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("admin_ready organization=%s user=%s email=%s role=%s\n", organizationID, user.ID, user.Email, user.Role)
}

func upsertAdmin(db *gorm.DB, organizationID uuid.UUID, email, password string) (*models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash admin password: %w", err)
	}

	var user models.User
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization models.Organization
		if err := tx.Where("id = ?", organizationID).First(&organization).Error; err != nil {
			return fmt.Errorf("find organization: %w", err)
		}

		findErr := tx.Unscoped().Where("LOWER(email) = ?", email).First(&user).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find admin user: %w", findErr)
		}
		if findErr == nil && user.OrganizationID != organizationID {
			return fmt.Errorf("admin email already belongs to another organization")
		}
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			user = models.User{ID: uuid.New(), Email: email, OrganizationID: organizationID}
		}
		user.FirstName = "Bing Chun"
		user.LastName = "Admin"
		user.Password = string(hash)
		user.Role = utils.RoleMerchantAdmin
		user.OrganizationID = organizationID
		user.DeletedAt = gorm.DeletedAt{}
		if user.CreatedAt.IsZero() {
			return tx.Create(&user).Error
		}
		return tx.Unscoped().Save(&user).Error
	})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func requiredEnvironment(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		log.Fatalf("%s is required", key)
	}
	return value
}
