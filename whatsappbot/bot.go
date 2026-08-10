package whatsappbot

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/mail"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/utils"
	"gorm.io/gorm"
)

type Session struct {
	CampaignID      uuid.UUID
	OrganizationID  uuid.UUID
	Amount          float64
	Customer        models.Customer
	Step            int
	Questions       []models.Question
	CurrentQuestion int
	Responses       []models.Response
}

type createOrgaization struct {
	Name       string
	CouponCode string
}

var sessions = make(map[string]*Session)
var processedMessages = make(map[string]bool)
var phoneRegex = regexp.MustCompile(`^0\d{10}$`)

// awaitingCampaignName tracks users who need to provide a campaign name after clarification
var awaitingCampaignName = make(map[string]bool)

// awaitingCampaignSelection tracks users who need to pick among multiple matching campaigns.
// Map: user phone -> shortCode -> campaignID
var awaitingCampaignSelection = make(map[string]map[string]uuid.UUID)

// Filler phrases to strip from user messages when extracting campaign name
var fillerPhrases = []string{
	"hi i am here for the",
	"hi i'm here for the",
	"hello i am here for the",
	"hello i'm here for the",
	"i am here for the",
	"i'm here for the",
	"hi i am here for",
	"hi i'm here for",
	"hello i am here for",
	"hello i'm here for",
	"i am here for",
	"i'm here for",
	"here for the",
	"here for",
	"joining the",
	"joining",
	"for the",
	"the",
	"campaign",
	"hi",
	"hello",
	"hey",
}

// WhatsApp API structures

func VerifyWebhookSignature(payload []byte, signature string) bool {
	// Meta signs webhooks with the App Secret (not the verify token)
	secret := os.Getenv("WHATSAPP_APP_SECRET")
	if secret == "" {
		log.Println("⚠️ WHATSAPP_APP_SECRET not set; rejecting signed webhook")
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	// Remove 'sha256=' prefix if present
	signature = strings.TrimPrefix(signature, "sha256=")

	return hmac.Equal([]byte(expectedSignature), []byte(signature))
}

func HandleWebhook(payload utils.WebhookPayload, db *gorm.DB) error {
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field == "messages" {
				for _, message := range change.Value.Messages {
					err := handleMessage(message.From, message, db)
					if err != nil {
						log.Printf("❌ Error handling message from %s: %v", message.From, err)
						if sendErr := utils.SendWhatsAppMessage(message.From, "❌ Sorry—something went wrong on our side. Please try again."); sendErr != nil {
							log.Printf("❌ Failed to send WhatsApp error message to %s: %v", message.From, sendErr)
						}
					}
				}
			}
		}
	}
	return nil
}

func handleMessage(from string, message struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Text      *struct {
		Body string `json:"body"`
	} `json:"text,omitempty"`
	Interactive *struct {
		Type        string `json:"type"`
		ButtonReply *struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"button_reply,omitempty"`
	} `json:"interactive,omitempty"`
	Type string `json:"type"`
}, db *gorm.DB) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ Panic while handling message from %s: %v", from, r)
			if sendErr := utils.SendWhatsAppMessage(from, "❌ Sorry—something went wrong on our side. Please try again."); sendErr != nil {
				log.Printf("❌ Failed to send WhatsApp error message to %s after panic: %v", from, sendErr)
			}
		}
	}()

	// Check if we've already processed this message
	if processedMessages[message.ID] {
		log.Printf("⚠️ Message %s already processed, skipping", message.ID)
		return nil
	}
	processedMessages[message.ID] = true

	// Clean up old processed messages (keep only last 1000)
	if len(processedMessages) > 1000 {
		processedMessages = make(map[string]bool)
	}

	var text string
	if message.Text != nil {
		text = message.Text.Body
	} else if message.Interactive != nil && message.Interactive.ButtonReply != nil {
		text = message.Interactive.ButtonReply.Title
	}

	// Ignore empty messages
	if text == "" {
		log.Println("⚠️ Received empty message, ignoring")
		return nil
	}

	log.Printf("📨 Message from %s: %s (type: %s)", from, text, message.Type)

	// Check if user is in an active COMMERCE session
	if _, exists := commerceSessions[from]; exists {
		return HandleCommerceMessage(from, text, db)
	}

	// Check if user triggers Commerce Flow explicitly or via Hello (Priority for Pilot)
	if strings.EqualFold(text, "hello") || strings.EqualFold(text, "hi") || strings.EqualFold(text, "shop") {
		// For the pilot, "Hello" starts Commerce Onboarding if not in a campaign
		// We can change this logic later to support both
		return HandleCommerceMessage(from, text, db)
	}

	// Check if user is in an active session - handle their responses
	if _, exists := sessions[from]; exists {
		return handleResponses(from, text, db)
	}

	// Check if user is awaiting to pick one of multiple matching campaigns
	if pending, ok := awaitingCampaignSelection[from]; ok {
		code := strings.ToLower(strings.TrimSpace(text))
		if campaignID, exists := pending[code]; exists {
			delete(awaitingCampaignSelection, from)
			return startCampaign(from, campaignID, db)
		}
		return utils.SendWhatsAppMessage(from, "❌ Invalid campaign code. Please reply with one of the codes shown.")
	}

	// Check if user is awaiting to provide a campaign name after clarification
	if awaitingCampaignName[from] {
		delete(awaitingCampaignName, from)
		campaignName := extractCampaignName(text)
		if campaignName == "" {
			return utils.SendWhatsAppMessage(from, "❌ I couldn't understand the campaign name. Please try again with just the campaign name.")
		}
		return startCampaignByName(from, campaignName, db)
	}

	// Try to extract campaign name from natural message
	// Expected format: "Hi I am here for the Joy campaign" or similar
	campaignName := extractCampaignName(text)

	if campaignName != "" {
		log.Printf("🔍 Extracted campaign name: '%s' from message: '%s'", campaignName, text)
		if err := startCampaignByName(from, campaignName, db); err != nil {
			log.Printf("❌ Error starting campaign by name for user %s: %v", from, err)
			return err
		}
		return nil
	}

	// Legacy support: Handle start command with UUID (for backwards compatibility)
	if strings.HasPrefix(strings.ToLower(text), "start ") {
		parts := strings.Split(text, " ")
		if len(parts) >= 2 {
			// Check if it's a UUID (legacy format)
			if campaignID, err := uuid.Parse(parts[1]); err == nil {
				return startCampaign(from, campaignID, db)
			}
			// Otherwise treat the rest as campaign name
			campaignName = strings.Join(parts[1:], " ")
			return startCampaignByName(from, campaignName, db)
		}
	}

	// If we can't identify the campaign, ask for clarification
	awaitingCampaignName[from] = true
	return utils.SendWhatsAppMessage(from, "👋 Welcome! Which campaign are you joining today?\n\nPlease reply with just the campaign name.")
}

func startLatestCampaign(from string, db *gorm.DB) error {
	var latestCampaign models.Campaign
	if err := db.Where("status = ?", "active").Order("created_at desc").First(&latestCampaign).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.SendWhatsAppMessage(from, "❌ No active campaigns found.")
		}
		return utils.SendWhatsAppMessage(from, "❌ An error occurred while fetching the latest campaign.")
	}

	return startCampaign(from, latestCampaign.ID, db)
}

// extractCampaignName extracts the campaign name from a natural user message
// It strips common filler phrases like "Hi I am here for the" and "campaign"
func extractCampaignName(text string) string {
	// Normalize: lowercase and trim
	normalized := strings.ToLower(strings.TrimSpace(text))

	// Remove filler phrases (process longer phrases first to avoid partial matches)
	for _, phrase := range fillerPhrases {
		normalized = strings.ReplaceAll(normalized, phrase, " ")
	}

	// Clean up extra whitespace
	normalized = strings.Join(strings.Fields(normalized), " ")
	normalized = strings.TrimSpace(normalized)

	return normalized
}

func campaignShortCode(id uuid.UUID) string {
	s := id.String()
	if len(s) >= 8 {
		return s[:8]
	}
	return s
}

// startCampaignByName finds a campaign by its human-readable name and starts the flow
func startCampaignByName(from string, campaignName string, db *gorm.DB) error {
	if campaignName == "" {
		awaitingCampaignName[from] = true
		return utils.SendWhatsAppMessage(from, "👋 Welcome! Which campaign are you joining today?\n\nPlease reply with just the campaign name.")
	}

	// Search for campaign(s) by name (fuzzy match)
	var campaigns []models.Campaign

	// First try exact match (case-insensitive)
	if err := db.Where("LOWER(campaign_name) = LOWER(?) AND status = ?", campaignName, "active").Find(&campaigns).Error; err != nil {
		return utils.SendWhatsAppMessage(from, "❌ An error occurred while looking up the campaign. Please try again.")
	}

	// If no exact matches, try LIKE match
	if len(campaigns) == 0 {
		searchPattern := "%" + campaignName + "%"
		if err := db.Where("LOWER(campaign_name) LIKE LOWER(?) AND status = ?", searchPattern, "active").Find(&campaigns).Error; err != nil {
			return utils.SendWhatsAppMessage(from, "❌ An error occurred while looking up the campaign. Please try again.")
		}
	}

	if len(campaigns) == 0 {
		awaitingCampaignName[from] = true
		return utils.SendWhatsAppMessage(from, "❌ I couldn't find a campaign with that name. Which campaign are you joining today?\n\nPlease reply with the campaign name.")
	}

	if len(campaigns) > 1 {
		options := make([]string, 0, len(campaigns))
		pending := make(map[string]uuid.UUID)
		var b strings.Builder
		b.WriteString("I found multiple campaigns with that name. Please reply with the campaign code:\n\n")
		for i, c := range campaigns {
			code := campaignShortCode(c.ID)
			pending[code] = c.ID
			b.WriteString(fmt.Sprintf("%d) %s (%s)\n", i+1, c.CampaignName, code))
			if len(options) < 3 {
				options = append(options, code)
			}
		}
		awaitingCampaignSelection[from] = pending
		if len(options) > 0 {
			return utils.SendWhatsAppButtons(from, b.String(), options)
		}
		return utils.SendWhatsAppMessage(from, b.String())
	}

	campaign := campaigns[0]
	log.Printf("✅ Found campaign '%s' (ID: %s) for user %s", campaign.CampaignName, campaign.ID.String(), from)
	return startCampaign(from, campaign.ID, db)
}

func startCampaign(from string, campaignID uuid.UUID, db *gorm.DB) error {
	log.Printf("🚀 Starting campaign %s for user %s", campaignID.String(), from)

	var campaign models.Campaign
	if err := db.Where("id = ? AND status = ?", campaignID, "active").First(&campaign).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("❌ Campaign %s not found or not active", campaignID.String())
			return utils.SendWhatsAppMessage(from, "❌ Campaign not found or not active.")
		}
		log.Printf("❌ Error fetching campaign %s: %v", campaignID.String(), err)
		return utils.SendWhatsAppMessage(from, "❌ An error occurred while fetching the campaign.")
	}

	var questions []models.Question
	if err := db.Where("campaign_id = ?", campaignID).Order("created_at asc").Find(&questions).Error; err != nil {
		log.Printf("❌ Error fetching questions for campaign %s: %v", campaignID.String(), err)
		return utils.SendWhatsAppMessage(from, "❌ Failed to fetch questions.")
	}

	log.Printf("📝 Found %d questions for campaign %s", len(questions), campaign.CampaignName)

	// Check if customer already exists
	var existingCustomer models.Customer
	q := db.Select("id").Where("phone = ? AND campaign_id = ?", from, campaign.ID).Limit(1).Find(&existingCustomer)
	if q.Error != nil {
		return utils.SendWhatsAppMessage(from, "❌ An error occurred while checking your registration. Please try again.")
	}
	if q.RowsAffected > 0 {
		return utils.SendWhatsAppMessage(from, "✅ You have already registered for this campaign. Stay tuned for the next one!")
	}

	step := 1
	if len(questions) == 0 {
		step = 2
	}

	sessions[from] = &Session{
		CampaignID:     campaign.ID,
		OrganizationID: campaign.OrganizationID,
		Amount:         campaign.Amount,
		Customer: models.Customer{
			Status:     "inactive",
			CampaignID: campaign.ID,
			Phone:      from,
			Channel:    "whatsapp",
		},
		Step:            step,
		Questions:       questions,
		CurrentQuestion: 0,
		Responses:       []models.Response{},
	}

	// Start with questions if available, otherwise ask for personal details
	if len(questions) > 0 {
		firstQuestion := questions[0]

		options, err := parseOptions(firstQuestion.Options)
		if err != nil {
			log.Println("❌ Error parsing options:", err)
			return utils.SendWhatsAppMessage(from, "❌ Error retrieving question options. Please try again later.")
		}

		if len(options) > 0 && firstQuestion.Type == "multiple_choice" {
			log.Printf("📤 Sending welcome message with buttons to %s", from)
			return utils.SendWhatsAppButtons(from, fmt.Sprintf("%s\n\n📝 %s", campaign.WelcomeMessage, firstQuestion.Text), options)
		}

		log.Printf("📤 Sending welcome message with first question to %s", from)
		return utils.SendWhatsAppMessage(from, fmt.Sprintf("%s\n\n📝 %s", campaign.WelcomeMessage, firstQuestion.Text))
	}

	// No questions, go straight to personal details
	log.Printf("📤 Sending welcome message (no questions) to %s", from)
	return utils.SendWhatsAppMessage(from, fmt.Sprintf("%s\n\nLet's get started by gathering your details.\nWhat's your first name?", campaign.WelcomeMessage))
}

func handleResponses(from, text string, db *gorm.DB) error {
	session, exists := sessions[from]
	if !exists {
		// No active session - try to find campaign by name
		campaignName := extractCampaignName(text)
		if campaignName != "" {
			return startCampaignByName(from, campaignName, db)
		}
		awaitingCampaignName[from] = true
		return utils.SendWhatsAppMessage(from, "👋 Welcome! Which campaign are you joining today?\n\nPlease reply with just the campaign name.")
	}

	switch session.Step {
	case 1:
		// Safety: if there are no questions loaded, transition to personal details flow
		if len(session.Questions) == 0 {
			session.Step = 2
			return utils.SendWhatsAppMessage(from, "Great! Now let's get your details.\nWhat's your first name?")
		}

		// Handle campaign questions first
		if session.CurrentQuestion < len(session.Questions) {
			question := session.Questions[session.CurrentQuestion]
			session.Responses = append(session.Responses, models.Response{
				CustomerID: session.Customer.ID,
				QuestionID: question.ID,
				Answer:     text,
			})

			if session.CurrentQuestion+1 < len(session.Questions) {
				session.CurrentQuestion++
				nextQuestion := session.Questions[session.CurrentQuestion]

				options, err := parseOptions(nextQuestion.Options)
				if err != nil {
					log.Println("❌ Error parsing options:", err)
					return utils.SendWhatsAppMessage(from, "❌ Error retrieving question options. Please try again later.")
				}

				if len(options) > 0 && nextQuestion.Type == "multiple_choice" {
					return utils.SendWhatsAppButtons(from, nextQuestion.Text, options)
				}

				return utils.SendWhatsAppMessage(from, nextQuestion.Text)
			}

			// All questions answered, move to personal details
			session.Step = 2
			return utils.SendWhatsAppMessage(from, "Great! Now let's get your details.\nWhat's your first name?")
		}

	case 2:
		session.Customer.FirstName = text
		session.Step++
		return utils.SendWhatsAppMessage(from, "📛 What's your last name?")

	case 3:
		session.Customer.LastName = text
		session.Step++
		return utils.SendWhatsAppMessage(from, "📧 What's your email address?")

	case 4:
		_, err := mail.ParseAddress(text)
		if err != nil {
			return utils.SendWhatsAppMessage(from, "❌ The email address you entered is not valid. Please enter a valid email address.")
		}

		session.Customer.Email = text
		session.Step++

		// Send coupon code via email
		var coupon models.Coupon
		if err := db.Where("campaign_id = ? AND redeemed = false", session.CampaignID).First(&coupon).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return utils.SendWhatsAppMessage(from, "❌ No available coupons at the moment.")
			}
			log.Println("❌ Error retrieving coupon:", err)
			return utils.SendWhatsAppMessage(from, "❌ An error occurred while fetching a coupon. Please try again later.")
		}

		tmp := createOrgaization{
			Name:       session.Customer.FirstName + " " + session.Customer.LastName,
			CouponCode: coupon.Code,
		}

		tmpl, err := template.ParseFiles("Zidi-coupon-code-email-template.html")
		if err != nil {
			log.Printf("Error loading template: %v", err)
			return utils.SendWhatsAppMessage(from, "❌ An error occurred while processing your request.")
		}

		var tpl bytes.Buffer
		if err := tmpl.Execute(&tpl, tmp); err != nil {
			log.Printf("Error executing template: %v", err)
			return utils.SendWhatsAppMessage(from, "❌ An error occurred while processing your request.")
		}

		err = utils.SendEmail00(session.Customer.Email, "Your Zidi Campaign Coupon Code", tpl.String())
		if err != nil {
			log.Println("❌ Error sending email:", err)
			return utils.SendWhatsAppMessage(from, "❌ An error occurred while sending the coupon code.")
		}

		return utils.SendWhatsAppMessage(from, "📞 Please provide your phone number:")

	case 5:
		if !phoneRegex.MatchString(text) {
			return utils.SendWhatsAppMessage(from, "❌ Invalid phone number format. Please enter your 11-digit phone number in this format: 08156579909 (no spaces, no country code).")
		}

		session.Customer.Phone = text
		session.Step++

		// WhatsApp allows max 3 buttons per message; send ETISALAT in a second message
		if err := utils.SendWhatsAppButtons(from, "📶 Which network provider do you use?", []string{"MTN", "GLO", "AIRTEL"}); err != nil {
			return err
		}
		return utils.SendWhatsAppButtons(from, "More options:", []string{"ETISALAT"})

	case 6:
		network := strings.ToLower(text)
		// Accept 9mobile as alias for Etisalat
		if network == "9mobile" || network == "9 mobile" {
			network = "etisalat"
		}
		validNetworks := map[string]bool{
			"mtn":      true,
			"glo":      true,
			"airtel":   true,
			"etisalat": true,
		}

		if !validNetworks[network] {
			return utils.SendWhatsAppMessage(from, "❌ Invalid network. Please select from the options provided.")
		}

		session.Customer.Network = strings.ToUpper(network)
		session.Customer.OrganizationID = session.OrganizationID
		session.Customer.Amount = session.Amount

		if err := saveCustomer(db, &session.Customer); err != nil {
			log.Println("❌ Error saving customer:", err)
			return utils.SendWhatsAppMessage(from, "❌ Customer already exists for this campaign.")
		}

		// Save responses if any questions were answered
		if len(session.Responses) > 0 {
			if err := saveResponses(db, session.Responses); err != nil {
				log.Println("❌ Error saving responses:", err)
				return utils.SendWhatsAppMessage(from, "❌ An error occurred while saving your responses.")
			}
		}

		session.Step = 7
		return utils.SendWhatsAppMessage(from, "🎟 Please enter the coupon code sent to your email.")

	case 7:
		couponCode := text

		var coupon models.Coupon
		if err := db.Where("code = ? AND campaign_id = ?", couponCode, session.CampaignID).First(&coupon).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return utils.SendWhatsAppMessage(from, "❌ Invalid coupon code. Please try again.")
			}
			log.Println("❌ Error retrieving coupon:", err)
			return utils.SendWhatsAppMessage(from, "❌ An error occurred while checking the coupon. Please try again later.")
		}

		if coupon.Redeemed {
			return utils.SendWhatsAppMessage(from, "❌ This coupon has already been redeemed. Please check and try again.")
		}

		now := time.Now()
		coupon.Redeemed = true
		coupon.RedeemedAt = &now
		if err := db.Save(&coupon).Error; err != nil {
			log.Println("❌ Error updating coupon:", err)
			return utils.SendWhatsAppMessage(from, "❌ An error occurred while redeeming your coupon. Please try again later.")
		}

		airtimeResponse, err := utils.SendAirtime(fmt.Sprintf("%.0f", session.Amount), session.Customer.Network, session.Customer.Phone)
		if err != nil {
			log.Println("❌ Error sending airtime:", err)
			return utils.SendWhatsAppMessage(from, "❌ An error occurred while sending your airtime. Please try again later.")
		}

		commissionFloat, err := strconv.ParseFloat(airtimeResponse.Commission, 32)
		if err != nil {
			log.Println("Error converting commission:", err)
			commissionFloat = 0
		}

		tx := models.Transaction{
			OrganizationID: session.OrganizationID,
			CampaignID:     session.CampaignID,
			CustomerID:     session.Customer.ID,
			Network:        airtimeResponse.Network,
			PhoneNumber:    session.Customer.Phone,
			TxReference:    airtimeResponse.RequestID,
			Status:         airtimeResponse.Status,
			Amount:         session.Amount,
			Type:           "airtime",
			Commisson:      commissionFloat,
		}

		if err := db.Create(&tx).Error; err != nil {
			log.Println("❌ Error creating transaction:", err)
			return utils.SendWhatsAppMessage(from, "❌ An error occurred while processing your transaction. Please try again later.")
		}

		err = db.Model(&models.Balance{}).
			Where("campaign_id = ?", session.CampaignID).
			Update("amount", gorm.Expr("amount - ?", session.Amount)).Error
		if err != nil {
			log.Println("❌ Error updating balance:", err)
		}

		err = db.Model(&models.Customer{}).
			Where("id = ?", session.Customer.ID).
			Update("status", "active").Error
		if err != nil {
			log.Println("❌ Error updating customer status:", err)
		}

		log.Println("✅ Transaction created:", tx.TxReference)

		delete(sessions, from)
		return utils.SendWhatsAppMessage(from, fmt.Sprintf("🎉 Congratulations! Your coupon has been successfully redeemed.\nAmount paid: ₦%.2f\nThank you for participating!\n\n👉 Follow @zidibot on Instagram, X & TikTok to join our next survey and win again!", session.Amount))
	}

	return nil
}

func parseOptions(optionsJSON []byte) ([]string, error) {
	var options []string
	if err := json.Unmarshal(optionsJSON, &options); err != nil {
		return nil, err
	}
	return options, nil
}

func saveCustomer(db *gorm.DB, customer *models.Customer) error {
	var existingCustomer models.Customer
	if err := db.Where("phone = ? AND campaign_id = ?", customer.Phone, customer.CampaignID).First(&existingCustomer).Error; err == nil {
		return fmt.Errorf("customer already exists for this campaign")
	}
	return db.Create(customer).Error
}

func saveResponses(db *gorm.DB, responses []models.Response) error {
	return db.Create(&responses).Error
}
