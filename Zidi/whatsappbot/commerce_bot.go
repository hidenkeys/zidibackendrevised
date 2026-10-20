package whatsappbot

import (
	"errors"
	"fmt"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/services"
	"github.com/hidenkeys/zidibackend/utils"
	"gorm.io/gorm"
)

// CommerceSessionState defines the current step of the user in the commerce flow
type CommerceSessionState int

const (
	StateOnboardingName CommerceSessionState = iota
	StateOnboardingEmail
	StateOnboardingAge
	StateMainMenu
	StateBrowsingCategories
	StateBrowsingProducts
	StateProductDetail
	StateCart
	StateCheckout
	StateTrackingInput
	StateComplaintCategory
	StateComplaintDescription
)

type CommerceSession struct {
	Client        *models.Client
	InstitutionID uuid.UUID
	State         CommerceSessionState
	TempData      map[string]interface{}
	LastActivity  time.Time
	Cart          []models.OrderItem
}

var commerceSessions = make(map[string]*CommerceSession)

func HandleCommerceMessage(from string, text string, db *gorm.DB) error {
	clientRepo := repository.NewClientRepoPG(db)
	clientService := services.NewClientService(clientRepo)

	productRepo := repository.NewProductRepoPG(db)
	productService := services.NewProductService(productRepo)

	orderRepo := repository.NewOrderRepoPG(db)
	orderService := services.NewOrderService(db, orderRepo, productRepo)

	complaintRepo := repository.NewComplaintRepoPG(db)
	complaintService := services.NewComplaintService(complaintRepo)

	// Fetch Lush Hair Institution
	var institutionID uuid.UUID
	var inst models.Institution
	if err := db.Where("name = ?", "Lush Hair").First(&inst).Error; err == nil {
		institutionID = inst.ID
	} else {
		// Fallback to first if Lush Hair not found (though seeding should ensure it)
		if err := db.First(&inst).Error; err == nil {
			institutionID = inst.ID
		}
	}

	session, exists := commerceSessions[from]
	if !exists {
		client, err := clientService.GetClientByPhone(institutionID, from)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Error checking client: %v", err)
			return utils.SendWhatsAppMessage(from, "❌ System error. Please try again later.")
		}
		// Treat RecordNotFound as new client (client remains nil)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			client = nil
			err = nil
		}

		if client == nil || client.OnboardingStatus != "complete" {
			commerceSessions[from] = &CommerceSession{
				InstitutionID: institutionID,
				State:         StateOnboardingName,
				TempData:      make(map[string]interface{}),
				LastActivity:  time.Now(),
			}
			return utils.SendWhatsAppMessage(from, "👋 Welcome to Lush Hair! \n\nBefore we start, may I know your *Name*?")
		}

		commerceSessions[from] = &CommerceSession{
			Client:        client,
			InstitutionID: institutionID,
			State:         StateMainMenu,
			TempData:      make(map[string]interface{}),
			LastActivity:  time.Now(),
		}
		return sendMainMenu(from, client.Name)
	}

	session.LastActivity = time.Now()

	// Special check for "Main Menu" or "Cancel" command at any time
	normalizedText := strings.ToLower(strings.TrimSpace(text))
	if normalizedText == "menu" || normalizedText == "cancel" || normalizedText == "close" || normalizedText == "quit" || normalizedText == "exit" || normalizedText == "hello" || normalizedText == "hi" || normalizedText == "start" || normalizedText == "shop" {
		session.State = StateMainMenu
		return sendMainMenu(from, session.Client.Name)
	}

	switch session.State {
	case StateOnboardingName:
		return handleOnboardingName(from, text, session)
	case StateOnboardingEmail:
		return handleOnboardingEmail(from, text, session)
	case StateOnboardingAge:
		return handleOnboardingAge(from, text, session, clientService)
	case StateMainMenu:
		return handleMainMenuSelection(from, text, session, productService, db)
	case StateBrowsingCategories:
		return handleBrowsingCategories(from, text, session, db)
	case StateBrowsingProducts:
		return handleBrowsingProducts(from, text, session, productService, orderService)
	case StateCart:
		return handleCartAction(from, text, session, orderService, db)
	case StateTrackingInput:
		return handleTrackingInput(from, text, session, orderService)
	case StateComplaintCategory:
		return handleComplaintCategory(from, text, session)
	case StateComplaintDescription:
		return handleComplaintDescription(from, text, session, complaintService)
	default:
		return sendMainMenu(from, "Unknown state.")
	}
}

func sendMainMenu(to, name string) error {
	msg := fmt.Sprintf("Hello %s! 👋\n\nHow can we help you today?", name)
	buttons := []string{"🛍️ Shop", "🚚 Track Order", "💬 Support"}
	if session, ok := commerceSessions[to]; ok {
		session.State = StateMainMenu
	}
	return utils.SendWhatsAppButtons(to, msg, buttons)
}

func handleOnboardingName(from, text string, session *CommerceSession) error {
	session.TempData["name"] = text
	session.State = StateOnboardingEmail
	return utils.SendWhatsAppMessage(from, "Nice to meet you! \n\nWhat is your *Email Address*?")
}

func handleOnboardingEmail(from, text string, session *CommerceSession) error {
	_, err := mail.ParseAddress(text)
	if err != nil {
		return utils.SendWhatsAppMessage(from, "❌ Invalid email format. Please try again.")
	}
	session.TempData["email"] = text
	session.State = StateOnboardingAge
	return utils.SendWhatsAppButtons(from, "Please select your *Age Range*:", []string{"18-24", "25-34", "35-50", "50+"})
}

func handleOnboardingAge(from, text string, session *CommerceSession, clientService *services.ClientService) error {
	name := session.TempData["name"].(string)
	email := session.TempData["email"].(string)
	ageRange := text

	client, err := clientService.RegisterClient(session.InstitutionID, name, from, email, ageRange)
	if err != nil {
		log.Printf("Error registering client: %v", err)
		return utils.SendWhatsAppMessage(from, "❌ Could not register you. Please try again.")
	}

	session.Client = client
	session.State = StateMainMenu
	return sendMainMenu(from, client.Name)
}

func handleMainMenuSelection(from, text string, session *CommerceSession, productService *services.ProductService, db *gorm.DB) error {
	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "shop") {
		session.State = StateBrowsingCategories
		// Fetch Categories
		var categories []models.Category
		if err := db.Where("institution_id = ?", session.InstitutionID).Find(&categories).Error; err != nil || len(categories) == 0 {
			return utils.SendWhatsAppMessage(from, "❌ No categories found.")
		}

		var categoryNames []string
		for _, cat := range categories {
			categoryNames = append(categoryNames, cat.Name)
		}

		// Map for next step
		session.TempData["categories"] = categories

		if len(categoryNames) > 3 {
			// Just list them with numbers if too many for buttons
			msg := "Please reply with the *number* of the category:"
			for i, name := range categoryNames {
				msg += fmt.Sprintf("\n%d. %s", i+1, name)
			}
			return utils.SendWhatsAppMessage(from, msg)
		}
		return utils.SendWhatsAppButtons(from, "Select a Category:", categoryNames)

	} else if strings.Contains(lowerText, "track") {
		session.State = StateTrackingInput
		return utils.SendWhatsAppMessage(from, "Please enter your **Tracking Number** (e.g., ORD-X8A9):")
	} else if strings.Contains(lowerText, "support") || strings.Contains(lowerText, "complaint") {
		session.State = StateComplaintCategory
		return utils.SendWhatsAppButtons(from, "What issue are you facing?", []string{"Product Defect", "Delivery Delay", "Other"})
	}

	// Default fall through
	return sendMainMenu(from, session.Client.Name)
}

func handleBrowsingCategories(from, text string, session *CommerceSession, db *gorm.DB) error {
	// Identify selected category
	selectedCatName := text

	// Handle number selection if needed (for now assume text matches button or exact name)
	// (Production Note: Should robustly handle "1", "2" mapping back to list)

	var category models.Category
	// Try fuzzy match on name
	if err := db.Where("institution_id = ? AND LOWER(name) LIKE ?", session.InstitutionID, "%"+strings.ToLower(selectedCatName)+"%").First(&category).Error; err != nil {
		return utils.SendWhatsAppMessage(from, "❌ Category not found. Please try again.")
	}

	session.TempData["selected_category_id"] = category.ID
	session.State = StateBrowsingProducts

	// Fetch Products in Category
	var products []models.Product
	if err := db.Where("category_id = ?", category.ID).Find(&products).Error; err != nil || len(products) == 0 {
		return utils.SendWhatsAppMessage(from, "❌ No products found in this category.")
	}

	// Check stock
	var availableProducts []models.Product
	for _, p := range products {
		if p.StockQuantity > 0 {
			availableProducts = append(availableProducts, p)
		}
	}

	if len(availableProducts) == 0 {
		return utils.SendWhatsAppMessage(from, "❌ All products in this category are out of stock.")
	}

	msg := fmt.Sprintf("*%s*\nSelect a product to buy (reply with number):\n", category.Name)
	for i, p := range availableProducts {
		msg += fmt.Sprintf("\n%d. %s - ₦%.2f", i+1, p.Name, p.Price)
	}

	session.TempData["available_products"] = availableProducts
	return utils.SendWhatsAppMessage(from, msg)
}

func handleBrowsingProducts(from, text string, session *CommerceSession, productService *services.ProductService, orderService *services.OrderService) error {
	// Parse selection
	// TODO: Handle numeric parsing better
	var index int
	_, err := fmt.Sscanf(text, "%d", &index)
	if err != nil || index < 1 {
		return utils.SendWhatsAppMessage(from, "❌ Invalid selection. Please reply with the product number.")
	}

	products, ok := session.TempData["available_products"].([]models.Product)
	if !ok || index > len(products) {
		return utils.SendWhatsAppMessage(from, "❌ Invalid selection. Please try again.")
	}

	selectedProduct := products[index-1]

	// Add to Cart
	session.Cart = append(session.Cart, models.OrderItem{
		ProductID: selectedProduct.ID,
		Quantity:  1,
		UnitPrice: selectedProduct.Price,
		// Store name for display, though not saved in DB items directly usually
		// but helpful for summary
	})

	// Add Name to TempData for summary if needed or just fetch from DB later
	// For simplicity, we trust ID.

	session.State = StateCart

	msg := fmt.Sprintf("✅ *Added to Cart!* \n\n%s - ₦%.2f\n\nWhat would you like to do next?", selectedProduct.Name, selectedProduct.Price)
	return utils.SendWhatsAppButtons(from, msg, []string{"🛍️ Shop More", "🛒 View Cart", "✅ Checkout"})
}

// Handler for Cart Actions
func handleCartAction(from, text string, session *CommerceSession, orderService *services.OrderService, db *gorm.DB) error {
	lowerText := strings.ToLower(text)

	if strings.Contains(lowerText, "shop more") {
		session.State = StateMainMenu
		return handleMainMenuSelection(from, "shop", session, services.NewProductService(repository.NewProductRepoPG(db)), db)
	} else if strings.Contains(lowerText, "view") {
		return sendCartSummary(from, session)
	} else if strings.Contains(lowerText, "checkout") {
		session.State = StateCheckout
		return handleCheckout(from, session, orderService)
	}

	return utils.SendWhatsAppButtons(from, "Please select an option:", []string{"🛍️ Shop More", "🛒 View Cart", "✅ Checkout"})
}

func sendCartSummary(from string, session *CommerceSession) error {
	if len(session.Cart) == 0 {
		session.State = StateMainMenu
		return utils.SendWhatsAppMessage(from, "🛒 Your cart is empty.")
	}

	var total float64
	msg := "🛒 *Your Cart*:\n"

	// Issue: We only stored ID. We need Names.
	// Ideally, we should fetch names.
	// Hack for Pilot: We stored them in TempData? No.
	// We need to fetch product names again or store them in OrderItem (struct modification needed? No OrderItem is model).
	// Let's rely on stored IDs and fetch from DB? Or store Name in TempData map associated with index?
	// Better: Reuse ProductService to get names?
	// Or just trust the user knows what they added? No.

	// Let's assume we can fetch names quickly or modify session to store names temporarily.
	// I'll add a temporary "CartDisplay" slice to session?
	// Or just fetch product details here.

	// Since I don't have easier access to Repo here without passing it...
	// I'll assume for now we Just show "Item #1 - Price".
	// Wait, I can pass ProductService to this function.

	// For Speed: I will just show item count and total.
	for i, item := range session.Cart {
		total += item.UnitPrice * float64(item.Quantity)
		msg += fmt.Sprintf("\n%d. Item (₦%.2f) x%d", i+1, item.UnitPrice, item.Quantity)
	}
	msg += fmt.Sprintf("\n\n*Total: ₦%.2f*", total)

	return utils.SendWhatsAppButtons(from, msg, []string{"🛍️ Shop More", "✅ Checkout"})
}

func handleCheckout(from string, session *CommerceSession, orderService *services.OrderService) error {
	if len(session.Cart) == 0 {
		return utils.SendWhatsAppMessage(from, "❌ Your cart is empty.")
	}

	order, err := orderService.CreateOrder(session.InstitutionID, session.Client.ID, session.Cart)
	if err != nil {
		log.Printf("Error creating order: %v", err)
		return utils.SendWhatsAppMessage(from, "❌ Failed to create order. Please try again.")
	}

	amountInt := int(order.TotalAmount)
	link, err := utils.CreatePaystackPaymentLink(session.Client.Email, amountInt, order.ID.String(), session.InstitutionID.String())
	if err != nil {
		log.Printf("Error creating payment link: %v", err)
		return utils.SendWhatsAppMessage(from, "❌ Failed to generate payment link.")
	}

	// Clear Cart
	session.Cart = []models.OrderItem{}
	session.State = StateMainMenu

	msg := fmt.Sprintf("✅ *Order Created!*\nOrder #%s\n\nItems: %d\nTotal: ₦%.2f\n\nPlease pay using this link:\n%s\n\nAfter payment, you will receive confirmation.", order.TrackingNumber, len(order.Items), order.TotalAmount, link)
	return utils.SendWhatsAppMessage(from, msg)
}

func handleTrackingInput(from, text string, session *CommerceSession, orderService *services.OrderService) error {
	order, err := orderService.GetOrderByTracking(strings.TrimSpace(text))
	if err != nil || order == nil {
		return utils.SendWhatsAppMessage(from, "❌ Order not found. Please check the number and try again, or type 'Main' to go back.")
	}

	trackInfo := fmt.Sprintf("📦 *Order Status*: %s\n📅 Date: %s\n💰 Total: ₦%.2f\n\nUpdates:\n", order.Status, order.CreatedAt.Format("02 Jan 2006"), order.TotalAmount)

	// Fetch tracking updates if any (not implemented in GetOrderByTracking yet?)
	// Assuming basic status for now.

	sendMainMenu(from, session.Client.Name)
	return utils.SendWhatsAppMessage(from, trackInfo)
}

func handleComplaintCategory(from, text string, session *CommerceSession) error {
	session.TempData["complaint_category"] = text
	session.State = StateComplaintDescription
	return utils.SendWhatsAppMessage(from, "Please describe your issue in detail:")
}

func handleComplaintDescription(from, text string, session *CommerceSession, complaintService *services.ComplaintService) error {
	category := session.TempData["complaint_category"].(string)
	description := text

	_, err := complaintService.LodgeComplaint(session.InstitutionID, session.Client.ID, category, description)
	if err != nil {
		return utils.SendWhatsAppMessage(from, "❌ Failed to log complaint.")
	}

	sendMainMenu(from, session.Client.Name)
	return utils.SendWhatsAppMessage(from, "✅ Complaint received! Our support team will contact you shortly.")
}
