package config

import (
	"context"
	"log"
	"os"

	"github.com/hidenkeys/zidibackend/migrations"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func ConnectDatabase() {
	dsn := os.Getenv("DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	DB = db
	log.Println("Connected to database")
}

func MigrateDatabase() {
	if err := migrations.PrepareLegacySchema(context.Background(), DB); err != nil {
		log.Fatal("Legacy schema preparation failed:", err)
	}
	err := DB.AutoMigrate(&models.Organization{}, &models.User{}, &models.Campaign{}, &models.Customer{}, &models.Question{}, &models.Response{}, &models.Coupon{}, &models.Payment{}, &models.Token{}, &models.Transaction{}, &models.Balance{}, &models.Institution{}, &models.Client{}, &models.Product{}, &models.Category{}, &models.Order{}, &models.OrderItem{}, &models.OrderTracking{}, &models.Complaint{}, &models.CommercePayment{})
	if err != nil {
		log.Fatal("Migration failed:", err)
	}
	if err := migrations.Run(context.Background(), DB); err != nil {
		log.Fatal("Versioned migration failed:", err)
	}
	log.Println("✅ Database migration successful")
}
