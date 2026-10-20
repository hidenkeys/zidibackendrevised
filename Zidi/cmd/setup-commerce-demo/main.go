package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/commerceonboarding"
	"github.com/hidenkeys/zidibackend/config"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/utils"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const demoOrganizationID = "67fa2b87-15c9-4f4b-a2db-c3b1da019211"

func main() {
	_ = godotenv.Load()
	organizationID := uuid.MustParse(envOrDefault("BING_CHUN_ORGANIZATION_ID", demoOrganizationID))
	email := strings.ToLower(envOrDefault("COMMERCE_DEMO_EMAIL", "commerce.demo@zidihq.test"))
	password := strings.TrimSpace(os.Getenv("COMMERCE_DEMO_PASSWORD"))
	if len(password) < 12 {
		log.Fatal("COMMERCE_DEMO_PASSWORD must contain at least 12 characters")
	}

	config.ConnectDatabase()
	config.MigrateDatabase()
	if err := ensureDemoIdentity(config.DB, organizationID, email, password); err != nil {
		log.Fatal(err)
	}

	merchantConfig, err := commerceonboarding.Load("config/merchants/bing-chun-nigeria.json")
	if err != nil {
		log.Fatal(err)
	}
	report, err := commerceonboarding.Apply(context.Background(), config.DB, merchantConfig)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf(
		"demo ready: organization=%s email=%s stores=%d categories=%d products=%d variants=%d inventory_created=%d inventory_preserved=%d\n",
		organizationID, email, report.Stores, report.Categories, report.Products, report.Variants,
		report.InventoryRowsCreated, report.InventoryRowsPreserved,
	)
}

func ensureDemoIdentity(db *gorm.DB, organizationID uuid.UUID, email, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash demo password: %w", err)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		organization := models.Organization{ID: organizationID}
		if err := tx.Where("id = ?", organizationID).FirstOrCreate(&organization, models.Organization{
			ID: organizationID, Email: email, CompanyName: "Bing Chun Nigeria Commerce Demo",
			ContactPersonName: "Local Demo", ContactPersonPhone: "+2348000000000",
			Address: "Lagos, Nigeria", Industry: "Food and beverage", CompanySize: 7,
		}).Error; err != nil {
			return fmt.Errorf("create demo organization: %w", err)
		}

		var user models.User
		err := tx.Where("email = ?", email).First(&user).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return fmt.Errorf("find demo user: %w", err)
		}
		if err == gorm.ErrRecordNotFound {
			user = models.User{ID: uuid.New(), Email: email}
		}
		user.FirstName = "Commerce"
		user.LastName = "Demo"
		user.Password = string(hash)
		user.Role = utils.RoleMerchantAdmin
		user.OrganizationID = organizationID
		if user.CreatedAt.IsZero() {
			err = tx.Create(&user).Error
		} else {
			err = tx.Save(&user).Error
		}
		if err != nil {
			return fmt.Errorf("upsert demo user: %w", err)
		}
		return nil
	})
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
