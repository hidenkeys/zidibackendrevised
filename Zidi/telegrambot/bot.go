package telegrambot

import (
	"bytes"
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

	"github.com/hidenkeys/zidibackend/utils"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	tele "gopkg.in/telebot.v3"
	"gorm.io/gorm"
)

type createOrgaization struct {
	Name       string
	CouponCode string
}
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

var sessions = make(map[int64]*Session)
var phoneRegex = regexp.MustCompile(`^0\d{10}$`)

// awaitingCampaignName tracks users who need to provide a campaign name after clarification
var awaitingCampaignName = make(map[int64]bool)

// awaitingCampaignSelection tracks users who need to pick among multiple matching campaigns.
// Map: userID -> shortCode -> campaignID
var awaitingCampaignSelection = make(map[int64]map[string]uuid.UUID)

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

// extractCampaignName extracts the campaign name from a natural user message
func extractCampaignName(text string) string {
	normalized := strings.ToLower(strings.TrimSpace(text))
	for _, phrase := range fillerPhrases {
		normalized = strings.ReplaceAll(normalized, phrase, " ")
	}
	normalized = strings.Join(strings.Fields(normalized), " ")
	return strings.TrimSpace(normalized)
}

func campaignShortCode(id uuid.UUID) string {
	s := id.String()
	if len(s) >= 8 {
		return s[:8]
	}
	return s
}

// findCampaignsByName searches for active campaigns by name using fuzzy matching
func findCampaignsByName(db *gorm.DB, campaignName string) ([]models.Campaign, error) {
	var campaigns []models.Campaign

	if err := db.Where("LOWER(campaign_name) = LOWER(?) AND status = ?", campaignName, "active").Find(&campaigns).Error; err != nil {
		return nil, err
	}
	if len(campaigns) > 0 {
		return campaigns, nil
	}

	searchPattern := "%" + campaignName + "%"
	if err := db.Where("LOWER(campaign_name) LIKE LOWER(?) AND status = ?", searchPattern, "active").Find(&campaigns).Error; err != nil {
		return nil, err
	}

	return campaigns, nil
}

func startCampaignByName(c tele.Context, db *gorm.DB, campaignName string) error {
	userID := c.Sender().ID
	if campaignName == "" {
		awaitingCampaignName[userID] = true
		return c.Send("👋 Welcome! Which campaign are you joining today?\n\nPlease reply with the campaign name.")
	}

	campaigns, err := findCampaignsByName(db, campaignName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			awaitingCampaignName[userID] = true
			return c.Send("❌ I couldn't find that campaign. Which campaign are you joining today?\n\nPlease reply with the campaign name.")
		}
		return c.Send("❌ An error occurred while looking up the campaign.")
	}

	if len(campaigns) == 0 {
		awaitingCampaignName[userID] = true
		return c.Send("❌ I couldn't find that campaign. Which campaign are you joining today?\n\nPlease reply with the campaign name.")
	}

	if len(campaigns) > 1 {
		pending := make(map[string]uuid.UUID)
		var b strings.Builder
		b.WriteString("I found multiple campaigns with that name. Please reply with the campaign code:\n\n")
		for i, camp := range campaigns {
			code := campaignShortCode(camp.ID)
			pending[code] = camp.ID
			b.WriteString(fmt.Sprintf("%d) %s (%s)\n", i+1, camp.CampaignName, code))
		}
		awaitingCampaignSelection[userID] = pending
		return c.Send(b.String())
	}

	campaign := campaigns[0]
	log.Printf("✅ Found campaign '%s' (ID: %s) for user %d", campaign.CampaignName, campaign.ID.String(), userID)
	return startCampaignByID(c, db, campaign.ID)
}

// StartBot initializes and runs the Telegram bot with a database connection
func StartBot(db *gorm.DB) {
	bot, err := tele.NewBot(tele.Settings{
		Token:  os.Getenv("TELEGRAM_API_KEY"),
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		log.Fatal("❌ Error initializing bot: ", err)
	}

	log.Println("🚀 Telegram Bot is running...")

	bot.Handle("/start", func(c tele.Context) error {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ Panic in /start handler (user %d): %v", c.Sender().ID, r)
				_ = c.Send("❌ Sorry—something went wrong on our side. Please try again.")
			}
		}()

		args := c.Args()
		if len(args) == 0 {
			awaitingCampaignName[c.Sender().ID] = true
			return c.Send("👋 Welcome! Which campaign are you joining today?\n\nPlease reply with the campaign name.")
		}

		// Join all args to support multi-word campaign names
		campaignArg := strings.Join(args, " ")

		// First, try to parse as UUID (legacy support)
		if campaignID, err := uuid.Parse(campaignArg); err == nil {
			return startCampaignByID(c, db, campaignID)
		}

		// Otherwise, treat as campaign name
		campaignName := extractCampaignName(campaignArg)
		if err := startCampaignByName(c, db, campaignName); err != nil {
			log.Printf("❌ Error in /start handler (user %d): %v", c.Sender().ID, err)
			_ = c.Send("❌ Sorry—something went wrong on our side. Please try again.")
			return err
		}
		return nil
	})

	// Pass the db to handleResponses
	bot.Handle(tele.OnText, func(c tele.Context) error {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ Panic in OnText handler (user %d): %v", c.Sender().ID, r)
				_ = c.Send("❌ Sorry—something went wrong on our side. Please try again.")
			}
		}()

		// Check if user is awaiting to provide a campaign name
		userID := c.Sender().ID

		if pending, ok := awaitingCampaignSelection[userID]; ok {
			code := strings.ToLower(strings.TrimSpace(c.Text()))
			if campaignID, exists := pending[code]; exists {
				delete(awaitingCampaignSelection, userID)
				if err := startCampaignByID(c, db, campaignID); err != nil {
					log.Printf("❌ Error starting campaign by code (user %d): %v", userID, err)
					_ = c.Send("❌ Sorry—something went wrong on our side. Please try again.")
					return err
				}
				return nil
			}
			return c.Send("❌ Invalid campaign code. Please reply with one of the codes shown.")
		}

		if awaitingCampaignName[userID] {
			delete(awaitingCampaignName, userID)
			campaignName := extractCampaignName(c.Text())
			if campaignName == "" {
				awaitingCampaignName[userID] = true
				return c.Send("❌ I couldn't understand the campaign name. Please try again with just the campaign name.")
			}
			if err := startCampaignByName(c, db, campaignName); err != nil {
				log.Printf("❌ Error starting campaign by name (user %d): %v", userID, err)
				_ = c.Send("❌ Sorry—something went wrong on our side. Please try again.")
				return err
			}
			return nil
		}
		if err := handleResponses(c, db); err != nil {
			log.Printf("❌ Error handling response (user %d): %v", userID, err)
			_ = c.Send("❌ Sorry—something went wrong on our side. Please try again.")
			return err
		}
		return nil
	})

	bot.Start()
}

// startCampaignByID starts the campaign flow for a given campaign UUID
func startCampaignByID(c tele.Context, db *gorm.DB, campaignID uuid.UUID) error {
	var campaign models.Campaign
	if err := db.Where("id = ? AND status = ?", campaignID, "active").First(&campaign).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Send("❌ Campaign not found or not active.")
		}
		return c.Send("❌ An error occurred while fetching the campaign.")
	}

	var questions []models.Question
	if err := db.Where("campaign_id = ?", campaignID).Order("created_at asc").Find(&questions).Error; err != nil {
		return c.Send("❌ Failed to fetch questions.")
	}

	user := c.Sender()

	// Check if the customer already exists for the same campaign
	var existingCustomer models.Customer
	q := db.Select("id").Where("phone = ? AND campaign_id = ?", user.Recipient(), campaign.ID).Limit(1).Find(&existingCustomer)
	if q.Error != nil {
		return c.Send("❌ An error occurred while checking your registration. Please try again later.")
	}
	if q.RowsAffected > 0 {
		return c.Send("✅ You have already registered for this campaign. Stay tuned for the next one!")
	}

	step := 1
	if len(questions) == 0 {
		step = 2
	}

	sessions[user.ID] = &Session{
		CampaignID:     campaign.ID,
		OrganizationID: campaign.OrganizationID,
		Amount:         campaign.Amount,
		Customer: models.Customer{
			Status:     "inactive",
			CampaignID: campaign.ID,
			Channel:    "telegram",
		},
		Step:            step,
		Questions:       questions,
		CurrentQuestion: 0,
		Responses:       []models.Response{},
	}

	// Start with questions if available, otherwise ask for personal details
	if len(questions) > 0 {
		firstQuestion := questions[0]

		// Parse options if present
		options, err := parseOptions(firstQuestion.Options)
		if err != nil {
			log.Println("❌ Error parsing options:", err)
			return c.Send("❌ Error retrieving question options. Please try again later.")
		}

		// Send question with buttons if there are options
		if len(options) > 0 && firstQuestion.Type == "multiple_choice" {
			btns := createOptionButtons(options)
			return c.Send(fmt.Sprintf("%s\n\n📋 %s", campaign.WelcomeMessage, firstQuestion.Text), &tele.ReplyMarkup{ReplyKeyboard: btns})
		}

		// Otherwise, just send the question
		return c.Send(fmt.Sprintf("%s\n\n📋 %s", campaign.WelcomeMessage, firstQuestion.Text))
	}

	// No questions, go straight to personal details
	return c.Send(fmt.Sprintf("%s\n\nLet's get started by gathering your details.\nWhat's your first name?", campaign.WelcomeMessage))
}

func parseOptions(optionsJSON []byte) ([]string, error) {
	var options []string
	if err := json.Unmarshal(optionsJSON, &options); err != nil {
		return nil, err
	}
	return options, nil
}

// Helper to create option buttons
func createOptionButtons(options []string) [][]tele.ReplyButton {
	var buttons [][]tele.ReplyButton
	for _, option := range options {
		btn := tele.ReplyButton{Text: option}
		buttons = append(buttons, []tele.ReplyButton{btn}) // One button per row
	}
	return buttons
}

func handleResponses(c tele.Context, db *gorm.DB) error {
	userID := c.Sender().ID
	session, exists := sessions[userID]
	if !exists {
		// No active session - prompt for campaign name
		awaitingCampaignName[userID] = true
		return c.Send("👋 Welcome! Which campaign are you joining today?\n\nPlease reply with the campaign name, or use /start followed by the campaign name.")
	}

	switch session.Step {
	case 1:
		// Safety: if there are no questions loaded, transition to personal details flow
		if len(session.Questions) == 0 {
			session.Step = 2
			return c.Send("Great! Now let's get your details.\nWhat's your first name?", &tele.ReplyMarkup{RemoveKeyboard: true})
		}

		// Handle campaign questions first
		if session.CurrentQuestion < len(session.Questions) {
			question := session.Questions[session.CurrentQuestion]
			session.Responses = append(session.Responses, models.Response{
				CustomerID: session.Customer.ID,
				QuestionID: question.ID,
				Answer:     c.Text(),
			})

			// If there are more questions, move to the next one
			if session.CurrentQuestion+1 < len(session.Questions) {
				session.CurrentQuestion++
				nextQuestion := session.Questions[session.CurrentQuestion]

				// Parse and show options if applicable
				options, err := parseOptions(nextQuestion.Options)
				if err != nil {
					log.Println("❌ Error parsing options:", err)
					return c.Send("❌ Error retrieving question options. Please try again later.")
				}

				if len(options) > 0 && nextQuestion.Type == "multiple_choice" {
					btns := createOptionButtons(options)
					return c.Send(nextQuestion.Text, &tele.ReplyMarkup{ReplyKeyboard: btns})
				}

				return c.Send(nextQuestion.Text)
			}

			// All questions answered, move to personal details
			session.Step = 2
			return c.Send("Great! Now let's get your details.\nWhat's your first name?", &tele.ReplyMarkup{RemoveKeyboard: true})
		}

	case 2:
		session.Customer.FirstName = c.Text()
		session.Step++
		return c.Send("📛 What's your last name?")

	case 3:
		session.Customer.LastName = c.Text()
		session.Step++
		return c.Send("📧 What's your email address?")

	case 4:
		emailInput := c.Text()
		// Validate email format
		_, err := mail.ParseAddress(emailInput)
		if err != nil {
			// Invalid email format, ask again without incrementing step
			return c.Send("❌ The email address you entered is not valid. Please enter a valid email address.")
		}

		session.Customer.Email = emailInput
		session.Step++
		var coupon models.Coupon

		// Find the first unredeemed coupon for a specific campaign
		if err := db.
			Where("campaign_id = ? AND redeemed = false", session.CampaignID).
			First(&coupon).Error; err != nil {

			if errors.Is(err, gorm.ErrRecordNotFound) {
				return c.Send("❌ No available coupons at the moment.")
			}

			log.Println("❌ Error retrieving coupon:", err)
			return c.Send("❌ An error occurred while fetching a coupon. Please try again later.")
		}

		// Coupon found, send it to the user
		tmp := createOrgaization{
			Name:       session.Customer.FirstName + " " + session.Customer.LastName,
			CouponCode: coupon.Code,
		}

		tmpl, err := template.ParseFiles("Zidi-coupon-code-email-template.html")
		if err != nil {
			log.Printf("Error loading template: %v", err)
			return c.Send("❌ An error occurred while processing your request.")
		}

		// Parse the template with the receipt data
		var tpl bytes.Buffer
		if err := tmpl.Execute(&tpl, tmp); err != nil {
			log.Printf("Error executing template: %v", err)
			return c.Send("❌ An error occurred while processing your request.")
		}

		// Convert parsed template to a string
		createBody := tpl.String()

		err = utils.SendEmail00(session.Customer.Email, "Your Zidi Campaign Coupon Code", createBody)
		if err != nil {
			log.Println("❌ Error sending email:", err)
			return c.Send("❌ An error occurred while sending the coupon code.")
		}
		return c.Send("📞 Please provide your phone number:")

	case 5:
		phoneInput := c.Text()
		// Validate phone number format using regex
		if !phoneRegex.MatchString(phoneInput) {
			return c.Send("❌ Invalid phone number format. Please enter your phone number in this format: 08156579909 (no spaces, no country code).")
		}

		session.Customer.Phone = phoneInput
		session.Step++

		networkKeyboard := &tele.ReplyMarkup{ResizeKeyboard: true}
		btnMTN := networkKeyboard.Text("mtn")
		btnGlo := networkKeyboard.Text("glo")
		btnAirtel := networkKeyboard.Text("airtel")
		btnEtisalat := networkKeyboard.Text("etisalat")
		networkKeyboard.Reply(networkKeyboard.Row(btnMTN, btnGlo), networkKeyboard.Row(btnAirtel, btnEtisalat))

		return c.Send("📶 Which network provider do you use?", networkKeyboard)

	case 6:
		network := strings.ToLower(c.Text())
		validNetworks := map[string]bool{
			"mtn":      true,
			"glo":      true,
			"airtel":   true,
			"etisalat": true,
		}

		if !validNetworks[network] {
			return c.Send("❌ Invalid network. Please select from the options provided.")
		}

		session.Customer.Network = strings.ToUpper(network)
		session.Customer.OrganizationID = session.OrganizationID
		session.Customer.Amount = session.Amount

		// Save customer to DB
		if err := saveCustomer(db, &session.Customer); err != nil {
			log.Println("❌ Error saving customer:", err)
			return c.Send("❌ customer already exists for this campaign.")
		}

		// Save responses if any questions were answered
		if len(session.Responses) > 0 {
			if err := saveResponses(db, session.Responses); err != nil {
				log.Println("❌ Error saving responses:", err)
				return c.Send("❌ An error occurred while saving your responses.")
			}
		}

		// Move to coupon validation step
		session.Step = 7
		clearKeyboard := &tele.ReplyMarkup{RemoveKeyboard: true}
		return c.Send("🎟 Please enter the coupon code sent to your email.", clearKeyboard)

	case 7: // Validate coupon code
		couponCode := c.Text()

		var coupon models.Coupon
		if err := db.Where("code = ? AND campaign_id = ?", couponCode, session.CampaignID).First(&coupon).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return c.Send("❌ Invalid coupon code. Please try again.")
			}
			log.Println("❌ Error retrieving coupon:", err)
			return c.Send("❌ An error occurred while checking the coupon. Please try again later.")
		}

		// Check if the coupon has already been redeemed
		if coupon.Redeemed {
			return c.Send("❌ This coupon has already been redeemed. Please check and try again.")
		}

		// Mark coupon as redeemed
		now := time.Now()
		coupon.Redeemed = true
		coupon.RedeemedAt = &now
		if err := db.Save(&coupon).Error; err != nil {
			log.Println("❌ Error updating coupon:", err)
			return c.Send("❌ An error occurred while redeeming your coupon. Please try again later.")
		}

		airtimeRespose, err := utils.SendAirtime(fmt.Sprintf("%.0f", session.Amount), session.Customer.Network, session.Customer.Phone)
		if err != nil {
			log.Println("❌ Error sending airtime:", err)
			return c.Send("❌ An error occurred while sending your airtime. Please try again later.")
		}

		commissionFloat, err := strconv.ParseFloat(airtimeRespose.Commission, 32)
		if err != nil {
			log.Println("Error converting commission:", err)
			commissionFloat = 0
		}

		// Create transaction directly
		tx := models.Transaction{
			OrganizationID: session.OrganizationID,
			CampaignID:     session.CampaignID,
			CustomerID:     session.Customer.ID,
			Network:        airtimeRespose.Network,
			PhoneNumber:    session.Customer.Phone,
			TxReference:    airtimeRespose.RequestID,
			Status:         airtimeRespose.Status,
			Amount:         session.Amount,
			Type:           "airtime",
			Commisson:      commissionFloat,
		}

		if err := db.Create(&tx).Error; err != nil {
			log.Println("❌ Error creating transaction directly:", err)
			return c.Send("❌ An error occurred while processing your transaction. Please try again later.")
		}

		// Update balance directly
		err = db.Model(&models.Balance{}).
			Where("campaign_id = ?", session.CampaignID).
			Update("amount", gorm.Expr("amount - ?", session.Amount)).Error
		if err != nil {
			log.Println("❌ Error updating balance directly:", err)
		}

		// Update customer status
		err = db.Model(&models.Customer{}).
			Where("id = ?", session.Customer.ID).
			Update("status", "active").Error
		if err != nil {
			log.Println("❌ Error updating customer status:", err)
		}

		// Transaction created successfully
		log.Println("✅ Transaction created:", tx.TxReference)

		// Final success message
		delete(sessions, userID)
		return c.Send(fmt.Sprintf("🎉 Congratulations! Your coupon has been successfully redeemed.\nAmount paid: ₦%.2f\nThank you for participating!\n\n👉 Follow @zidibot on Instagram, X & TikTok to join our next survey and win again!", session.Amount))
	}

	return nil
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
