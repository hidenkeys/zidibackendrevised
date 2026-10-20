package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/services"
	"github.com/hidenkeys/zidibackend/utils"
	"gorm.io/gorm"

	//openapi_types "github.com/oapi-codegen/runtime/types"
	"net/http"
	//"os"
	"strconv"
	"strings"
)

type ppayment struct {
	Name            string
	Amount          string
	CampaignName    string
	TelegramBotLink string
	WhatsAppLink    string
	HasTelegram     bool
	HasWhatsApp     bool
}

type Server struct {
	db                          *gorm.DB
	orgService                  *services.OrganizationService
	usrService                  *services.UserService
	campaignService             *services.CampaignService
	customerService             *services.CustomerService
	questionService             *services.QuestionService
	responseService             *services.ResponseService
	paymentService              *services.PaymentService
	transactionService          *services.TransactionService
	balanceService              *services.BalanceService
	orderService                *services.OrderService
	commerceFoundationService   *services.CommerceFoundationService
	commerceCatalogueService    *services.CommerceCatalogueService
	commerceCustomerCartService *services.CommerceCustomerCartService
	commerceOrderService        *services.CommerceOrderService
	commercePaymentService      *services.CommercePaymentService
	commerceFulfilmentService   *services.CommerceFulfilmentService
	commerceStoreOrderService   *services.CommerceStoreOrderService
	commerceChannelService      *services.CommerceChannelService
}

type PaystackWebhookPayload struct {
	Event string `json:"event"`
	Data  struct {
		ID              int    `json:"id"`
		Amount          int    `json:"amount"`
		Currency        string `json:"currency"`
		TransactionRef  string `json:"reference"`
		Channel         string `json:"channel"`
		GatewayResponse string `json:"gateway_response"`
		IPAddress       string `json:"ip_address"`
		CreatedAt       string `json:"created_at"`
		Status          string `json:"status"`
		Customer        struct {
			ID        int    `json:"id"`
			Email     string `json:"email"`
			Phone     string `json:"phone"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		} `json:"customer"`
		Metadata struct {
			CampaignID     string `json:"campaign_id"`
			OrganizationID string `json:"organization_id"`
		} `json:"metadata"`
	} `json:"data"`
}

type FlutterwaveWebhookPayload struct {
	Event string `json:"event"`
	Data  struct {
		ID                int    `json:"id"`
		TxRef             string `json:"tx_ref"`
		FlwRef            string `json:"flw_ref"`
		Amount            int    `json:"amount"`
		Currency          string `json:"currency"`
		ChargedAmount     int    `json:"charged_amount"`
		AppFee            int    `json:"app_fee"`
		MerchantFee       int    `json:"merchant_fee"`
		ProcessorResponse string `json:"processor_response"`
		AuthModel         string `json:"auth_model"`
		IP                string `json:"ip"`
		Narration         string `json:"narration"`
		Status            string `json:"status"`
		PaymentType       string `json:"payment_type"`
		CreatedAt         string `json:"created_at"`
		AccountID         int    `json:"account_id"`
		Customer          struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			PhoneNumber string `json:"phone_number"`
			Email       string `json:"email"`
			CreatedAt   string `json:"created_at"`
		} `json:"customer"`
	} `json:"data"`
	Meta struct {
		CheckoutInitAddress     string `json:"__CheckoutInitAddress"`
		CampaignID              string `json:"campaign_id"`
		OrganizationID          string `json:"organization_id"`
		OriginatorAccountNumber string `json:"originatoraccountnumber"`
		OriginatorName          string `json:"originatorname"`
		BankName                string `json:"bankname"`
		OriginatorAmount        string `json:"originatoramount"`
	} `json:"meta_data"`
}

func VerifyPaystackSignature(body []byte, headerSignature, secretKey string) bool {
	mac := hmac.New(sha512.New, []byte(secretKey))
	mac.Write(body)
	expectedMAC := mac.Sum(nil)
	expectedSignature := hex.EncodeToString(expectedMAC)

	return hmac.Equal([]byte(expectedSignature), []byte(headerSignature))
}

func (s Server) PostFlutterwaveWebhook(c *fiber.Ctx) error {
	signature := c.Get("x-paystack-signature")
	ctx := context.Background()
	if signature == "" {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "Missing signature header"})
	}

	// Use your secret key from env or config
	secretKey := os.Getenv("PAYSTACK_SK")

	// Verify signature
	if !VerifyPaystackSignature(c.Body(), signature, secretKey) {
		fmt.Println("i reached here 2")
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid signature"})
	}

	// Parse payload
	var payload PaystackWebhookPayload
	if err := json.Unmarshal(c.Body(), &payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON payload"})
	}
	fmt.Printf("Received Paystack Webhook: %+v\n", payload)

	transactionRef := payload.Data.TransactionRef
	if transactionRef == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Missing transaction reference"})
	}

	// 🆕 Check if transaction has already been processed
	existingPayment, err := s.paymentService.GetPaymentByTransactionRef(ctx, transactionRef)
	if err == nil && existingPayment != nil {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"message": "Transaction already processed",
		})
	}

	// Verify transaction
	isVerified, err := utils.VerifyPaystackTransaction(transactionRef)
	if err != nil {
		log.Printf("❌ Paystack Verification Error: %v", err)
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Transaction verification error"})
	}
	if !isVerified {
		log.Printf("❌ Paystack Transaction Not Verified: %s", transactionRef)
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Transaction verification failed"})
	}
	log.Printf("✅ Paystack Transaction Verified: %s", transactionRef)

	// Check if Metadata.CampaignID (used as generic ref) corresponds to an Order
	orderIDStr := payload.Data.Metadata.CampaignID
	if orderID, err := uuid.Parse(orderIDStr); err == nil {
		// Check if it's an order
		// FIX: Use GetOrder instead of GetOrderByID
		if order, err := s.orderService.GetOrder(orderID); err == nil && order != nil {
			// Found an Order! Process as Commerce Payment
			if payload.Data.Status == "success" {
				// Verify amount if needed (payload.Data.Amount is kobo)
				// order.TotalAmount is in Naira? Check model. Usually stored as Major unit. Paystack is Minor.
				// Assuming order.TotalAmount is Major (Naira).
				// Verify amount (Paystack sends kobo/minor units) as float64
				amountPaid := float64(payload.Data.Amount) / 100
				log.Printf("💰 Order Payment Check: OrderTotal=%.2f Paid=%.2f", order.TotalAmount, amountPaid)

				// Allow small float difference or exact match
				if amountPaid >= order.TotalAmount-0.01 {
					// Process Payment
					err := s.orderService.ProcessOrderPayment(
						order.ID,
						amountPaid,
						payload.Data.TransactionRef,
						payload.Data.Customer.Email,
						"paystack",
					)

					if err != nil {
						log.Printf("❌ Failed to process order payment: %v", err)
						return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to process payment"})
					}

					return c.Status(http.StatusOK).JSON(fiber.Map{"message": "Order processed successfully"})
				} else {
					log.Printf("❌ Order Payment Amount Mismatch. Expected: %.2f, Got: %.2f", order.TotalAmount, float64(payload.Data.Amount)/100)
				}
			}
			return c.Status(http.StatusOK).JSON(fiber.Map{"message": "Order payment webhook received"})
		}
	}

	// Extract campaign ID (Legacy Flow)
	campaignID, err := uuid.Parse(payload.Data.Metadata.CampaignID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid campaign ID"})
	}

	campaign, err := s.campaignService.GetCampaignByID(ctx, campaignID)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Campaign not found"})
	}

	fmt.Println(payload)

	if payload.Data.Status == "success" && float32(payload.Data.Amount)/100 == campaign.Price {

		fmt.Printf("Transaction %s verified. Processing payment...\n", transactionRef)

		balance := api.Balance{
			CampaignId:      campaignID,
			StartingBalance: float32(payload.Data.Amount) / 100,
			Amount:          float32(payload.Data.Amount) / 100,
		}
		if _, err := s.balanceService.CreateBalance(ctx, &balance); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create balance"})
		}

		tokens := utils.GenerateTokens(campaign.CharacterType, campaign.CouponLength, campaign.CouponNumber)

		for _, token := range tokens {
			coupon := api.Coupon{
				CampaignId: campaignID,
				Code:       token,
				Redeemed:   false,
			}
			if _, err := s.campaignService.CreateCoupon(ctx, &coupon); err != nil {
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to store tokens"})
			}
		}

		payment := api.Payment{
			TransactionId:  strconv.Itoa(payload.Data.ID),
			CampaignId:     campaignID,
			Amount:         float32(payload.Data.Amount) / 100,
			Status:         "successful",
			OrganizationId: campaign.OrganizationId,
			TransactionRef: transactionRef,
		}
		if _, err := s.paymentService.CreatePayment(ctx, &payment); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save payment"})
		}

		campaign.Status = "active"
		fmt.Println("campaign : ", campaign)
		if _, err := s.campaignService.UpdateCampaign(ctx, campaignID, campaign); err != nil {

			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update campaign status"})
		}

		response, err := s.orgService.GetOrganizationByID(ctx, campaign.OrganizationId)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(api.Error{
				ErrorCode: "500",
				Message:   err.Error(),
			})
		}

		// Generate links based on communication channels
		// Use human-readable campaign name instead of internal IDs
		campaignNameEncoded := strings.ReplaceAll(campaign.CampaignName, " ", "%20")
		telegramLink := "https://t.me/zidipromobot?start=" + campaignNameEncoded
		whatsappLink := "https://wa.me/2348085105382?text=Hi%20I%20am%20here%20for%20the%20" + campaignNameEncoded + "%20campaign"

		// Check which channels are enabled
		channels := strings.ToLower(campaign.CommunicationChannels)
		hasTelegram := strings.Contains(channels, "telegram")
		hasWhatsApp := strings.Contains(channels, "whatsapp")

		tmp := ppayment{
			Name:            response.ContactPersonName,
			Amount:          strconv.Itoa(int(campaign.Amount) * 100),
			CampaignName:    campaign.CampaignName,
			TelegramBotLink: telegramLink,
			WhatsAppLink:    whatsappLink,
			HasTelegram:     hasTelegram,
			HasWhatsApp:     hasWhatsApp,
		}

		tmpl, err := template.ParseFiles("Zidi-payment-successful-email-template.html")
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(api.Error{
				ErrorCode: "500",
				Message:   "Template load error: " + err.Error(),
			})
		}

		var tpl bytes.Buffer
		if err := tmpl.Execute(&tpl, tmp); err != nil {
			log.Fatalf("Error executing template: %v", err)
		}

		createBody := tpl.String()

		err = utils.SendEmail00(string(response.Email), "Complete your "+campaign.CampaignName+" Campaign Payment", createBody)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(api.Error{
				ErrorCode: "500",
				Message:   err.Error(),
			})
		}

		return c.Status(http.StatusOK).JSON(fiber.Map{
			"message": "Payment processed successfully, campaign activated, tokens generated",
			"tokens":  tokens,
		})
	}

	fmt.Printf("Transaction %s not successful. Skipping...\n", transactionRef)
	return c.Status(http.StatusOK).JSON(fiber.Map{"message": "Webhook received"})

}

func NewServer(db *gorm.DB, balanceService *services.BalanceService, transactionService *services.TransactionService, orgService *services.OrganizationService, usrService *services.UserService, campaignService *services.CampaignService, customerService *services.CustomerService, questionService *services.QuestionService, responseService *services.ResponseService, paymentService *services.PaymentService, orderService *services.OrderService, commerceFoundationService *services.CommerceFoundationService, commerceCatalogueService *services.CommerceCatalogueService, commerceCustomerCartService *services.CommerceCustomerCartService, commerceOrderService *services.CommerceOrderService, commercePaymentService *services.CommercePaymentService, commerceFulfilmentService *services.CommerceFulfilmentService, commerceStoreOrderService *services.CommerceStoreOrderService, commerceChannelService *services.CommerceChannelService) *Server {
	return &Server{
		db:                          db,
		balanceService:              balanceService,
		transactionService:          transactionService,
		orgService:                  orgService,
		usrService:                  usrService,
		campaignService:             campaignService,
		customerService:             customerService,
		questionService:             questionService,
		responseService:             responseService,
		paymentService:              paymentService,
		orderService:                orderService,
		commerceFoundationService:   commerceFoundationService,
		commerceCatalogueService:    commerceCatalogueService,
		commerceCustomerCartService: commerceCustomerCartService,
		commerceOrderService:        commerceOrderService,
		commercePaymentService:      commercePaymentService,
		commerceFulfilmentService:   commerceFulfilmentService,
		commerceStoreOrderService:   commerceStoreOrderService,
		commerceChannelService:      commerceChannelService,
	}
}

//func (s Server) PostFlutterwaveWebhook(c *fiber.Ctx) error {
//	// Read request body
//	body := c.Body()
//
//	// Verify request signature
//	signature := c.Get("verif-hash")
//	secretHash := os.Getenv("FLW_SECRET_HASH") // Ensure this is set in your .env file
//
//	if signature != secretHash {
//		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid signature"})
//	}
//
//	// Parse JSON payload
//	var payload FlutterwaveWebhookPayload
//	if err := json.Unmarshal(body, &payload); err != nil {
//		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON payload"})
//	}
//	// Log received webhook for debugging
//	fmt.Printf("Received Flutterwave Webhook: %+v\n", payload)
//
//	// Verify transaction ID
//	transactionID := strconv.Itoa(payload.Data.ID)
//	if transactionID == "" {
//		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid transaction ID"})
//	}
//
//	// Verify transaction via Flutterwave API
//	isVerified, err := utils.VerifyFlutterwaveTransaction(payload.Data.ID)
//	if err != nil || !isVerified {
//		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Transaction verification failed"})
//	}
//
//	// Extract campaign ID from metadata
//	campaignIDStr := payload.Meta.CampaignID
//	if campaignIDStr == "" {
//		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Missing campaign ID"})
//	}
//
//	campaignID, err := uuid.Parse(campaignIDStr)
//	if err != nil {
//		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid campaign ID"})
//	}
//
//	// Retrieve campaign details
//	ctx := context.Background()
//
//	campaign, err := s.campaignService.GetCampaignByID(ctx, campaignID)
//	if err != nil {
//		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Campaign not found"})
//	}
//
//	if payload.Data.Status == "successful" && float32(payload.Data.Amount) == campaign.Price {
//		fmt.Printf("Transaction %s verified. Processing payment...\n", transactionID)
//
//		fmt.Println("5")
//		// Extract metadata from webhook
//		//meta, ok := payload.Data.Meta.(map[string]interface{})
//		//if !ok {
//		//	return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid metadata format"})
//		//}
//		// Create balance record
//
//		balance := api.Balance{
//			CampaignId:      campaignID,
//			StartingBalance: float32(payload.Data.Amount),
//			Amount:          float32(payload.Data.Amount),
//		}
//		if _, err := s.balanceService.CreateBalance(ctx, &balance); err != nil {
//			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create balance record"})
//		}
//
//		// Generate tokens
//		tokens := utils.GenerateTokens(campaign.CharacterType, campaign.CouponLength, campaign.CouponNumber)
//
//		// Store tokens in the database
//		for _, token := range tokens {
//			coupon := api.Coupon{
//				CampaignId: campaignID,
//				Code:       token,
//				Redeemed:   false,
//			}
//			_, err := s.campaignService.CreateCoupon(ctx, &coupon)
//			if err != nil {
//				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to store tokens"})
//			}
//		}
//
//		// Save payment record
//		payment := api.Payment{
//			TransactionId:  transactionID,
//			CampaignId:     campaignID,
//			Amount:         float32(payload.Data.Amount),
//			Status:         "successful",
//			OrganizationId: campaign.OrganizationId,
//			TransactionRef: payload.Data.TxRef,
//		}
//
//		if _, err := s.paymentService.CreatePayment(ctx, &payment); err != nil {
//			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save payment record"})
//		}
//		// Update campaign status to "active"
//		campaign.Status = "active"
//		if err, _ := s.campaignService.UpdateCampaign(ctx, campaignID, campaign); err != nil {
//			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update campaign status"})
//		}
//
//		response, err := s.orgService.GetOrganizationByID(context.Background(), campaign.OrganizationId)
//		if err != nil {
//			return c.Status(http.StatusInternalServerError).JSON(api.Error{
//				ErrorCode: "500",
//				Message:   err.Error(),
//			})
//		}
//
//		link := "https://t.me/zidipromobot?start=" + campaignID.String()
//		tmp := ppayment{
//			Name:            response.ContactPersonName,
//			Amount:          strconv.Itoa(int(campaign.Amount) * 100),
//			CampaignName:    campaign.CampaignName,
//			TelegramBotLink: link, // Updated to use Flutterwave link
//		}
//
//		tmpl, err := template.ParseFiles("Zidi-payment-successful-email-template.html")
//		if err != nil {
//			return c.Status(http.StatusInternalServerError).JSON(api.Error{
//				ErrorCode: "500",
//				Message:   "Template load error: " + err.Error(),
//			})
//		}
//
//		var tpl bytes.Buffer
//		if err := tmpl.Execute(&tpl, tmp); err != nil {
//			log.Fatalf("Error executing template: %v", err)
//		}
//
//		createBody := tpl.String()
//
//		err = utils.SendEmail0(string(response.Email), "Complete your "+campaign.CampaignName+" Campaign Payment", createBody)
//		if err != nil {
//			return c.Status(http.StatusInternalServerError).JSON(api.Error{
//				ErrorCode: "500",
//				Message:   err.Error(),
//			})
//		}
//
//		return c.Status(http.StatusOK).JSON(fiber.Map{
//			"message": "Payment processed successfully, campaign activated, tokens generated",
//			"tokens":  tokens,
//		})
//	}
//
//	fmt.Printf("Transaction %s failed or pending. Skipping...\n", transactionID)
//	return c.Status(http.StatusOK).JSON(fiber.Map{"message": "Webhook received"})
//}
