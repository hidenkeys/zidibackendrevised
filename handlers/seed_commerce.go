package handlers

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/utils"
)

func (s *Server) SeedCommerceData() {
	// 1. Seed Institution (Lush Hair)
	// Check if exists
	var institution models.Institution
	instName := "Lush Hair"
	if err := s.db.Where("name = ?", instName).First(&institution).Error; err != nil {
		institution = models.Institution{
			Name:               instName,
			LogoURL:            "https://res.cloudinary.com/dkmv44eg9/image/upload/v1769492011/LUSH_LOGO_j76mqv.avif",
			ContactPersonName:  "Letima",
			ContactPersonPhone: "08156572209",
			WelcomeMessage:     "Welcome to Lush Hair! We help you shine.",
		}
		if err := s.db.Create(&institution).Error; err != nil {
			log.Printf("❌ Failed to seed institution: %v", err)
			return
		}
		log.Println("✅ Seeded Institution: Lush Hair")
	}

	// 2. Seed Categories
	categories := []struct {
		Name        string
		Description string
		Products    []string
	}{
		{
			Name:        "Hair Care",
			Description: "Shampoos, conditioners, and creams",
			Products:    []string{"Hair Cream", "Shampoo"},
		},
		{
			Name:        "Hair Extensions",
			Description: "Premium hair extensions",
			Products:    []string{"Curly Braids", "Dancing Curls", "Kinky Twist"},
		},
		{
			Name:        "Hair Breakage Treatment",
			Description: "Treatments for healthy hair",
			Products:    []string{"Rejuvinating Shampoo", "Detangling Conditioner", "Deep Conditioner", "Leave in Treatment", "Indian Herb Hair"},
		},
	}

	for _, catData := range categories {
		var category models.Category
		if err := s.db.Where("institution_id = ? AND name = ?", institution.ID, catData.Name).First(&category).Error; err != nil {
			category = models.Category{
				InstitutionID: institution.ID,
				Name:          catData.Name,
				Description:   catData.Description,
			}
			s.db.Create(&category)
			log.Printf("✅ Seeded Category: %s", catData.Name)
		} else {
			// Category exists, we have the ID to use
		}

		// 3. Seed Products
		for _, prodName := range catData.Products {
			var product models.Product
			if err := s.db.Where("institution_id = ? AND name = ?", institution.ID, prodName).First(&product).Error; err != nil {
				price := 5000.0                                                                                   // Default price
				imageURL := "https://res.cloudinary.com/dkmv44eg9/image/upload/v1769492132/hair-care_qp2wng.webp" // Default (Hair Care)

				if catData.Name == "Hair Extensions" {
					price = 15000.0
					imageURL = "https://res.cloudinary.com/dkmv44eg9/image/upload/v1769492023/product-image_tliaog.webp"
				} else if catData.Name == "Hair Breakage Treatment" {
					imageURL = "https://res.cloudinary.com/dkmv44eg9/image/upload/v1769492130/hair-breakage_wai55c.webp"
				}

				product = models.Product{
					InstitutionID: institution.ID,
					CategoryID:    category.ID,
					Name:          prodName,
					Description:   prodName + " - High Quality",
					Price:         price,
					SKU:           prodName[:3] + "-" + uuid.New().String()[:4],
					StockQuantity: 8,
					ImageURL:      imageURL,
				}
				s.db.Create(&product)
				log.Printf("✅ Seeded Product: %s", prodName)
			} else {
				// Update stock and image if exists
				product.StockQuantity = 8
				// Update Image URL too to ensure latest is used
				if catData.Name == "Hair Extensions" {
					product.ImageURL = "https://res.cloudinary.com/dkmv44eg9/image/upload/v1769492023/product-image_tliaog.webp"
				} else if catData.Name == "Hair Breakage Treatment" {
					product.ImageURL = "https://res.cloudinary.com/dkmv44eg9/image/upload/v1769492130/hair-breakage_wai55c.webp"
				} else {
					product.ImageURL = "https://res.cloudinary.com/dkmv44eg9/image/upload/v1769492132/hair-care_qp2wng.webp"
				}
				s.db.Save(&product)
			}
		}
	}

	// Send Email to Contact Person
	// "let the whatsappbot link be sent to the contact person emailon creation of the organization"
	// We do this every time seed runs? No, maybe just if we created it or just always for demo.
	// Let's do it always for this session to ensure they get it.

	// Construct Link
	// "https://wa.me/<PhoneNumber>"
	// Assuming bot phone number is known or environmentally set.
	// Using generic link or the one from `server.go` logic.
	// "2348085105382" from server.go
	botLink := "https://wa.me/2348085105382?text=Hello"

	emailBody := fmt.Sprintf("Hello %s,<br><br>Your Commerce Organization <b>%s</b> is ready.<br>You can start selling on WhatsApp here: <a href='%s'>%s</a>", institution.ContactPersonName, institution.Name, botLink, botLink)

	// Hardcoded for Pilot
	contactEmail := "letimapro23@gmail.com"
	if err := utils.SendEmail00(contactEmail, "Your Commerce Bot is Ready!", emailBody); err != nil {
		log.Printf("❌ Failed to send welcome email: %v", err)
	} else {
		log.Println("✅ Sent welcome email to", contactEmail)
	}
}
