//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/commerceonboarding"
	commercefulfilment "github.com/hidenkeys/zidibackend/fulfilment"
	"github.com/hidenkeys/zidibackend/messaging"
	"github.com/hidenkeys/zidibackend/migrations"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/payments"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/services"
	"github.com/hidenkeys/zidibackend/utils"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	testProviderAccountID = "e2e-whatsapp-account"
	testSenderID          = "2348000000000"
	testWebhookSignature  = "valid-e2e-signature"
)

var integrationDB *gorm.DB

type legacyUser struct {
	gorm.Model
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Email          string    `gorm:"unique;not null"`
	OrganizationID uuid.UUID
}

func (legacyUser) TableName() string { return "users" }

func TestMain(m *testing.M) {
	dsn := strings.TrimSpace(os.Getenv("COMMERCE_TEST_DATABASE_URL"))
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "COMMERCE_TEST_DATABASE_URL is required; run make commerce-e2e")
		os.Exit(2)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		fmt.Fprintf(os.Stderr, "open commerce E2E database: %v\n", err)
		os.Exit(2)
	}
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto").Error; err != nil {
		fmt.Fprintf(os.Stderr, "create pgcrypto extension: %v\n", err)
		os.Exit(2)
	}
	if err := db.AutoMigrate(&models.Organization{}, &legacyUser{}); err != nil {
		fmt.Fprintf(os.Stderr, "create legacy prerequisite schema: %v\n", err)
		os.Exit(2)
	}
	legacyOrganizationID := uuid.New()
	if err := db.Create(&models.Organization{ID: legacyOrganizationID, Email: "legacy-schema@example.com"}).Error; err != nil {
		fmt.Fprintf(os.Stderr, "create legacy schema organization: %v\n", err)
		os.Exit(2)
	}
	if err := db.Create(&legacyUser{ID: uuid.New(), Email: "legacy-schema-user@example.com", OrganizationID: legacyOrganizationID}).Error; err != nil {
		fmt.Fprintf(os.Stderr, "create legacy schema user: %v\n", err)
		os.Exit(2)
	}
	if err := migrations.PrepareLegacySchema(context.Background(), db); err != nil {
		fmt.Fprintf(os.Stderr, "prepare legacy commerce schema: %v\n", err)
		os.Exit(2)
	}
	var convertedOrganizationID string
	if err := db.Raw("SELECT organization_id::text FROM users WHERE email = ?", "legacy-schema-user@example.com").Scan(&convertedOrganizationID).Error; err != nil || convertedOrganizationID != legacyOrganizationID.String() {
		fmt.Fprintf(os.Stderr, "validate legacy organization UUID conversion: got=%s err=%v\n", convertedOrganizationID, err)
		os.Exit(2)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		fmt.Fprintf(os.Stderr, "complete user schema migration: %v\n", err)
		os.Exit(2)
	}
	if err := migrations.Run(context.Background(), db); err != nil {
		fmt.Fprintf(os.Stderr, "run commerce migrations: %v\n", err)
		os.Exit(2)
	}
	if err := migrations.Run(context.Background(), db); err != nil {
		fmt.Fprintf(os.Stderr, "rerun commerce migrations idempotently: %v\n", err)
		os.Exit(2)
	}
	integrationDB = db

	code := m.Run()
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
	os.Exit(code)
}

type e2ePaymentProvider struct {
	mu       sync.Mutex
	requests map[string]payments.InitializeRequest
	verifies int
}

func newE2EPaymentProvider() *e2ePaymentProvider {
	return &e2ePaymentProvider{requests: make(map[string]payments.InitializeRequest)}
}

func (p *e2ePaymentProvider) Name() string            { return "e2epay" }
func (p *e2ePaymentProvider) SignatureHeader() string { return "x-e2e-signature" }

func (p *e2ePaymentProvider) Initialize(_ context.Context, request payments.InitializeRequest) (*payments.Initialization, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests[request.Reference] = request
	return &payments.Initialization{
		Reference:        request.Reference,
		AuthorizationURL: "https://payments.invalid/authorize/" + request.Reference,
		AccessCode:       "e2e-access-code",
		ProviderResponse: []byte(`{"initialized":true}`),
	}, nil
}

func (p *e2ePaymentProvider) Verify(_ context.Context, reference string) (*payments.Verification, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	request, ok := p.requests[reference]
	if !ok {
		return nil, payments.ErrProviderRejected
	}
	p.verifies++
	paidAt := time.Now().UTC()
	return &payments.Verification{
		Reference: reference, ProviderTransactionID: "e2e-transaction-" + reference,
		Status: "success", AmountMinor: request.AmountMinor, Currency: request.Currency,
		PaidAt: &paidAt, ProviderResponse: []byte(`{"status":"success"}`),
	}, nil
}

func (p *e2ePaymentProvider) VerifyWebhook(_ []byte, signature string) bool {
	return signature == testWebhookSignature
}

func (p *e2ePaymentProvider) ParseWebhook(body []byte) (*payments.WebhookEvent, error) {
	var payload struct {
		EventKey  string `json:"event_key"`
		Reference string `json:"reference"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.EventKey == "" || payload.Reference == "" {
		return nil, payments.ErrInvalidWebhook
	}
	return &payments.WebhookEvent{
		Key: payload.EventKey, Type: "charge.success", Reference: payload.Reference,
		ProviderTransactionID: "e2e-transaction-" + payload.Reference, Payload: append([]byte(nil), body...),
	}, nil
}

func (p *e2ePaymentProvider) verifyCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.verifies
}

type e2eWhatsAppSender struct {
	mu       sync.Mutex
	messages []messaging.WhatsAppOutboundMessage
}

func (s *e2eWhatsAppSender) Send(_ context.Context, message messaging.WhatsAppOutboundMessage) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, message)
	return fmt.Sprintf("e2e-message-%d", len(s.messages)), nil
}

func (s *e2eWhatsAppSender) contains(fragment string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, message := range s.messages {
		if strings.Contains(message.Body, fragment) {
			return true
		}
	}
	return false
}

type e2eEnvironment struct {
	db *gorm.DB

	organizationID      uuid.UUID
	otherOrganizationID uuid.UUID
	merchantActor       services.CommerceActor
	otherMerchantActor  services.CommerceActor
	ikejaStaffActor     services.CommerceActor
	lekkiStaffActor     services.CommerceActor
	ikejaStore          models.CommerceStore
	lekkiStore          models.CommerceStore
	fruitTeaCategory    models.CommerceCategory
	variant             models.CommerceProductVariant

	foundationRepo  *repository.CommerceFoundationRepoPG
	catalogueRepo   *repository.CommerceCatalogueRepoPG
	customerRepo    *repository.CommerceCustomerCartRepoPG
	orderRepo       *repository.CommerceOrderRepoPG
	paymentRepo     *repository.CommercePaymentRepoPG
	fulfilmentRepo  *repository.CommerceFulfilmentRepoPG
	channelRepo     *repository.CommerceChannelRepoPG
	customerService *services.CommerceCustomerCartService
	orderService    *services.CommerceOrderService
	paymentService  *services.CommercePaymentService
	fulfilment      *services.CommerceFulfilmentService
	storeOrders     *services.CommerceStoreOrderService
	channel         *services.CommerceChannelService
	delivery        *services.CommerceChannelDeliveryService
	paymentProvider *e2ePaymentProvider
	sender          *e2eWhatsAppSender
	messageSequence int
}

func seedEnvironment(t *testing.T) *e2eEnvironment {
	t.Helper()
	ctx := context.Background()
	if err := integrationDB.Exec("TRUNCATE TABLE users, organizations RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("reset E2E database: %v", err)
	}

	organizationID := uuid.New()
	otherOrganizationID := uuid.New()
	organizations := []models.Organization{
		{ID: organizationID, Email: "bing-e2e@example.com", CompanyName: "Bing Chun E2E"},
		{ID: otherOrganizationID, Email: "other-e2e@example.com", CompanyName: "Other Merchant E2E"},
	}
	if err := integrationDB.Create(&organizations).Error; err != nil {
		t.Fatalf("create E2E organizations: %v", err)
	}

	merchantID, otherMerchantID, ikejaStaffID, lekkiStaffID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	users := []models.User{
		{ID: merchantID, Email: "merchant-e2e@example.com", Role: utils.RoleMerchantAdmin, OrganizationID: organizationID},
		{ID: otherMerchantID, Email: "other-merchant-e2e@example.com", Role: utils.RoleMerchantAdmin, OrganizationID: otherOrganizationID},
		{ID: ikejaStaffID, Email: "ikeja-staff-e2e@example.com", Role: utils.RoleStoreStaff, OrganizationID: organizationID},
		{ID: lekkiStaffID, Email: "lekki-staff-e2e@example.com", Role: utils.RoleStoreStaff, OrganizationID: organizationID},
	}
	if err := integrationDB.Create(&users).Error; err != nil {
		t.Fatalf("create E2E users: %v", err)
	}

	t.Setenv("BING_CHUN_ORGANIZATION_ID", organizationID.String())
	t.Setenv("BING_CHUN_WHATSAPP_PROVIDER_ACCOUNT_ID", testProviderAccountID)
	t.Setenv("BING_CHUN_WHATSAPP_DISPLAY_PHONE_NUMBER", "+2348000000000")
	config, err := commerceonboarding.Load("../config/merchants/bing-chun-nigeria.json")
	if err != nil {
		t.Fatalf("load Bing Chun onboarding: %v", err)
	}
	report, err := commerceonboarding.Apply(ctx, integrationDB, config)
	if err != nil {
		t.Fatalf("apply Bing Chun onboarding: %v", err)
	}
	if report.Stores != 7 || report.Categories != 6 || report.Products != 28 || report.Variants != 28 {
		t.Fatalf("unexpected onboarding summary: %+v", report.Summary)
	}

	var stores []models.CommerceStore
	if err := integrationDB.Preload("Hours").Preload("FulfilmentModes").Where("organization_id = ?", organizationID).Find(&stores).Error; err != nil {
		t.Fatalf("load E2E stores: %v", err)
	}
	var ikeja, lekki models.CommerceStore
	for _, store := range stores {
		switch store.Code {
		case "JARA-IKEJA":
			ikeja = store
		case "OLIVE-LEKKI":
			lekki = store
		}
	}
	if ikeja.ID == uuid.Nil || lekki.ID == uuid.Nil {
		t.Fatal("onboarding did not create both active test stores")
	}
	if err := integrationDB.Model(&models.CommerceStoreHour{}).
		Where("organization_id = ? AND store_id IN ?", organizationID, []uuid.UUID{ikeja.ID, lekki.ID}).
		Updates(map[string]interface{}{"is_closed": false, "open_minute": 0, "close_minute": 1440}).Error; err != nil {
		t.Fatalf("make active E2E stores deterministically open: %v", err)
	}

	var category models.CommerceCategory
	if err := integrationDB.Where("organization_id = ? AND slug = ?", organizationID, "fruit-tea").First(&category).Error; err != nil {
		t.Fatalf("load fruit tea category: %v", err)
	}
	var variant models.CommerceProductVariant
	if err := integrationDB.Where("organization_id = ? AND sku = ?", organizationID, "BCNG-FT-001").First(&variant).Error; err != nil {
		t.Fatalf("load test variant: %v", err)
	}

	assignments := []models.CommerceStaffStoreAssignment{
		{ID: uuid.New(), OrganizationID: organizationID, StoreID: ikeja.ID, UserID: ikejaStaffID, Role: utils.RoleStoreStaff, Status: models.CommerceStatusActive},
		{ID: uuid.New(), OrganizationID: organizationID, StoreID: lekki.ID, UserID: lekkiStaffID, Role: utils.RoleStoreStaff, Status: models.CommerceStatusActive},
	}
	if err := integrationDB.Create(&assignments).Error; err != nil {
		t.Fatalf("create E2E staff assignments: %v", err)
	}

	foundationRepo := repository.NewCommerceFoundationRepoPG(integrationDB)
	catalogueRepo := repository.NewCommerceCatalogueRepoPG(integrationDB)
	customerRepo := repository.NewCommerceCustomerCartRepoPG(integrationDB)
	orderRepo := repository.NewCommerceOrderRepoPG(integrationDB)
	paymentRepo := repository.NewCommercePaymentRepoPG(integrationDB)
	fulfilmentRepo := repository.NewCommerceFulfilmentRepoPG(integrationDB)
	channelRepo := repository.NewCommerceChannelRepoPG(integrationDB)
	customerService := services.NewCommerceCustomerCartService(customerRepo, customerRepo, catalogueRepo, foundationRepo)
	orderService := services.NewCommerceOrderService(orderRepo, customerRepo, customerRepo, foundationRepo)
	paymentProvider := newE2EPaymentProvider()
	paymentService := services.NewCommercePaymentService(
		paymentRepo, orderRepo, customerRepo, foundationRepo, payments.NewRegistry(paymentProvider),
		paymentProvider.Name(), "https://commerce.invalid/payment/callback",
	)
	codeManager, err := commercefulfilment.NewCodeManager([]byte("zidi-commerce-e2e-secret-at-least-32-bytes"))
	if err != nil {
		t.Fatalf("create fulfilment code manager: %v", err)
	}
	fulfilmentService := services.NewCommerceFulfilmentService(
		fulfilmentRepo, orderRepo, foundationRepo, commercefulfilment.NewRegistry(), codeManager,
	)
	storeOrders := services.NewCommerceStoreOrderService(orderService, fulfilmentService)
	channelService := services.NewCommerceChannelService(
		channelRepo, foundationRepo, catalogueRepo, customerService, orderService, paymentService, fulfilmentService,
	)
	sender := &e2eWhatsAppSender{}
	delivery := services.NewCommerceChannelDeliveryService(channelRepo, sender, orderRepo, fulfilmentRepo, fulfilmentService)

	return &e2eEnvironment{
		db: integrationDB, organizationID: organizationID, otherOrganizationID: otherOrganizationID,
		merchantActor:      services.CommerceActor{UserID: merchantID, OrganizationID: organizationID, Role: utils.RoleMerchantAdmin},
		otherMerchantActor: services.CommerceActor{UserID: otherMerchantID, OrganizationID: otherOrganizationID, Role: utils.RoleMerchantAdmin},
		ikejaStaffActor:    services.CommerceActor{UserID: ikejaStaffID, OrganizationID: organizationID, Role: utils.RoleStoreStaff},
		lekkiStaffActor:    services.CommerceActor{UserID: lekkiStaffID, OrganizationID: organizationID, Role: utils.RoleStoreStaff},
		ikejaStore:         ikeja, lekkiStore: lekki, fruitTeaCategory: category, variant: variant,
		foundationRepo: foundationRepo, catalogueRepo: catalogueRepo, customerRepo: customerRepo,
		orderRepo: orderRepo, paymentRepo: paymentRepo, fulfilmentRepo: fulfilmentRepo, channelRepo: channelRepo,
		customerService: customerService, orderService: orderService, paymentService: paymentService,
		fulfilment: fulfilmentService, storeOrders: storeOrders, channel: channelService, delivery: delivery,
		paymentProvider: paymentProvider, sender: sender,
	}
}

func (e *e2eEnvironment) handleInbound(t *testing.T, input services.CommerceChannelInbound) *services.CommerceChannelHandleResult {
	t.Helper()
	e.messageSequence++
	if input.ProviderAccountID == "" {
		input.ProviderAccountID = testProviderAccountID
	}
	if input.SenderID == "" {
		input.SenderID = testSenderID
	}
	if input.SenderName == "" {
		input.SenderName = "E2E Customer"
	}
	if input.MessageType == "" {
		input.MessageType = "text"
	}
	if input.ExternalMessageID == "" {
		input.ExternalMessageID = fmt.Sprintf("e2e-inbound-%03d", e.messageSequence)
	}
	result, err := e.channel.HandleInbound(context.Background(), input)
	if err != nil {
		t.Fatalf("handle inbound message %q: %v", input.Text, err)
	}
	return result
}

func (e *e2eEnvironment) latestOutboundBody(t *testing.T, senderID string) string {
	t.Helper()
	var message models.CommerceChannelMessage
	err := e.db.Where("organization_id = ? AND direction = ? AND recipient_id = ?", e.organizationID, models.CommerceChannelDirectionOutbound, senderID).
		Order("created_at DESC, id DESC").First(&message).Error
	if err != nil {
		t.Fatalf("load latest outbound message: %v", err)
	}
	return message.Body
}

func (e *e2eEnvironment) resolveCustomer(t *testing.T, suffix string) *models.CommerceCustomer {
	t.Helper()
	customer, _, err := e.customerService.ResolveCustomerForChannel(context.Background(), e.organizationID, services.ResolveCommerceCustomerInput{
		Channel: models.CommerceIdentityChannelWhatsApp, Identifier: "+234811000" + suffix,
		DisplayName: "Checkout Customer " + suffix, Email: "customer-" + suffix + "@example.com", Verified: true,
	})
	if err != nil {
		t.Fatalf("resolve customer %s: %v", suffix, err)
	}
	return customer
}

func (e *e2eEnvironment) createCart(t *testing.T, customer *models.CommerceCustomer, quantity int) *services.CommerceCartView {
	t.Helper()
	cart, _, err := e.customerService.CreateCartForChannel(context.Background(), e.organizationID, customer.ID, e.ikejaStore.ID)
	if err != nil {
		t.Fatalf("create cart: %v", err)
	}
	cart, err = e.customerService.SetCartItemForChannel(context.Background(), e.organizationID, customer.ID, cart.Cart.ID, e.variant.ID, quantity)
	if err != nil {
		t.Fatalf("add cart item: %v", err)
	}
	return cart
}

func (e *e2eEnvironment) initializeAndPay(t *testing.T, customer *models.CommerceCustomer, order *models.CommerceOrder, suffix string) *repository.CommercePaymentSession {
	t.Helper()
	session, created, err := e.paymentService.InitializePaymentForChannel(context.Background(), e.organizationID, customer.ID, order.ID, services.InitializeCommercePaymentInput{
		Provider: e.paymentProvider.Name(), PayerEmail: "payer-" + suffix + "@example.com", IdempotencyKey: "payment-" + suffix,
	})
	if err != nil || !created {
		t.Fatalf("initialize payment: created=%v err=%v", created, err)
	}
	body, _ := json.Marshal(map[string]string{"event_key": "event-" + suffix, "reference": session.Payment.ProviderReference})
	result, err := e.paymentService.ProcessWebhook(context.Background(), e.paymentProvider.Name(), body, testWebhookSignature)
	if err != nil || result.Outcome != models.CommercePaymentStatusSucceeded {
		t.Fatalf("process payment webhook: result=%+v err=%v", result, err)
	}
	return session
}

func wrongVerificationCode(actual string) string {
	if actual == "000000" {
		return "999999"
	}
	return "000000"
}

func TestCommerceWhatsAppPickupJourneyE2E(t *testing.T) {
	env := seedEnvironment(t)
	ctx := context.Background()

	env.handleInbound(t, services.CommerceChannelInbound{Text: "Hi"})
	env.handleInbound(t, services.CommerceChannelInbound{Text: "Order"})
	latitude, longitude := 6.592294, 3.3386004
	env.handleInbound(t, services.CommerceChannelInbound{MessageType: "location", Latitude: &latitude, Longitude: &longitude})
	if body := env.latestOutboundBody(t, testSenderID); !strings.Contains(body, "Bingchun Jara Mall") {
		t.Fatalf("nearest-store reply did not select Ikeja: %q", body)
	}
	env.handleInbound(t, services.CommerceChannelInbound{SelectionID: "category:" + env.fruitTeaCategory.ID.String()})
	env.handleInbound(t, services.CommerceChannelInbound{SelectionID: "product:" + env.variant.ID.String()})
	quantityResult := env.handleInbound(t, services.CommerceChannelInbound{Text: "1", ExternalMessageID: "quantity-message"})
	if !quantityResult.Handled {
		t.Fatal("quantity message was not handled")
	}
	duplicate := env.handleInbound(t, services.CommerceChannelInbound{Text: "1", ExternalMessageID: "quantity-message"})
	if !duplicate.Duplicate {
		t.Fatal("duplicate WhatsApp message was processed twice")
	}

	// Simulate the customer closing WhatsApp after building a cart. A new service
	// instance must resume from the persisted conversation and cart state.
	env.channel = services.NewCommerceChannelService(
		env.channelRepo, env.foundationRepo, env.catalogueRepo, env.customerService,
		env.orderService, env.paymentService, env.fulfilment,
	)
	env.handleInbound(t, services.CommerceChannelInbound{SelectionID: "cart:checkout"})
	env.handleInbound(t, services.CommerceChannelInbound{SelectionID: "fulfilment:pickup"})
	env.handleInbound(t, services.CommerceChannelInbound{Text: "customer@example.com"})
	if body := env.latestOutboundBody(t, testSenderID); !strings.Contains(body, "Pay securely here") {
		t.Fatalf("WhatsApp checkout did not return a payment link: %q", body)
	}

	var customer models.CommerceCustomer
	if err := env.db.Joins("JOIN commerce_customer_identities identities ON identities.customer_id = commerce_customers.id").
		Where("commerce_customers.organization_id = ? AND identities.normalized_identifier = ?", env.organizationID, testSenderID).
		First(&customer).Error; err != nil {
		t.Fatalf("load WhatsApp customer: %v", err)
	}
	var order models.CommerceOrder
	if err := env.db.Where("organization_id = ? AND customer_id = ?", env.organizationID, customer.ID).Order("created_at DESC").First(&order).Error; err != nil {
		t.Fatalf("load WhatsApp order: %v", err)
	}
	if order.Status != models.CommerceOrderStatusPendingPayment || order.FulfilmentMode != models.FulfilmentModeCustomerPickup {
		t.Fatalf("unexpected order after WhatsApp checkout: status=%s mode=%s", order.Status, order.FulfilmentMode)
	}
	var payment models.CommercePaymentTransaction
	if err := env.db.Where("organization_id = ? AND order_id = ?", env.organizationID, order.ID).First(&payment).Error; err != nil {
		t.Fatalf("load initialized payment: %v", err)
	}

	// Confirmation is webhook-driven; this deliberately does not call a browser callback.
	webhookBody, _ := json.Marshal(map[string]string{"event_key": "pickup-payment-event", "reference": payment.ProviderReference})
	webhook, err := env.paymentService.ProcessWebhook(ctx, env.paymentProvider.Name(), webhookBody, testWebhookSignature)
	if err != nil || webhook.Outcome != models.CommercePaymentStatusSucceeded || webhook.Duplicate {
		t.Fatalf("first payment webhook: result=%+v err=%v", webhook, err)
	}
	duplicateWebhook, err := env.paymentService.ProcessWebhook(ctx, env.paymentProvider.Name(), webhookBody, testWebhookSignature)
	if err != nil || !duplicateWebhook.Duplicate || env.paymentProvider.verifyCount() != 1 {
		t.Fatalf("duplicate payment webhook was not idempotent: result=%+v verifies=%d err=%v", duplicateWebhook, env.paymentProvider.verifyCount(), err)
	}

	if err := env.db.Preload("Items").Preload("Events").First(&order, "id = ?", order.ID).Error; err != nil {
		t.Fatalf("reload paid order: %v", err)
	}
	if order.Status != models.CommerceOrderStatusPaid || len(order.Items) != 1 || order.Items[0].UnitPriceMinor != env.variant.PriceMinor {
		t.Fatalf("paid order snapshot is incorrect: %+v", order)
	}
	var invoice models.CommerceInvoice
	if err := env.db.Where("order_id = ?", order.ID).First(&invoice).Error; err != nil || invoice.Status != models.CommerceInvoiceStatusPaid {
		t.Fatalf("invoice was not paid: status=%s err=%v", invoice.Status, err)
	}
	var inventory models.CommerceInventoryLevel
	if err := env.db.Where("organization_id = ? AND store_id = ? AND variant_id = ?", env.organizationID, env.ikejaStore.ID, env.variant.ID).First(&inventory).Error; err != nil {
		t.Fatalf("load committed inventory: %v", err)
	}
	if inventory.QuantityOnHand != 24 || inventory.QuantityReserved != 0 {
		t.Fatalf("payment did not commit inventory exactly once: %+v", inventory)
	}
	var paymentOutboxCount int64
	if err := env.db.Model(&models.CommerceOutboxEvent{}).Where("aggregate_id = ? AND topic IN ?", order.ID, []string{
		models.CommerceOutboxTopicPaymentCustomer, models.CommerceOutboxTopicPaymentStore,
	}).Count(&paymentOutboxCount).Error; err != nil || paymentOutboxCount != 2 {
		t.Fatalf("payment notification outbox count=%d err=%v", paymentOutboxCount, err)
	}

	if _, err := env.delivery.DispatchOnce(ctx, 100); err != nil {
		t.Fatalf("dispatch payment confirmation: %v", err)
	}
	if !env.sender.contains("Payment confirmed for order " + order.OrderNumber) {
		t.Fatal("customer did not receive the payment confirmation through the channel delivery boundary")
	}

	views, total, err := env.storeOrders.ListOperationalOrders(ctx, env.ikejaStaffActor, nil, services.CommerceStoreOrderListInput{StoreID: &env.ikejaStore.ID})
	if err != nil || total != 1 || len(views) != 1 || views[0].Order.ID != order.ID {
		t.Fatalf("Ikeja staff queue mismatch: total=%d views=%d err=%v", total, len(views), err)
	}
	if _, _, err := env.storeOrders.ListOperationalOrders(ctx, env.lekkiStaffActor, nil, services.CommerceStoreOrderListInput{StoreID: &env.ikejaStore.ID}); !errors.Is(err, repository.ErrCommerceNotFound) {
		t.Fatalf("staff from another store accessed the order queue: %v", err)
	}
	if _, err := env.orderService.GetOrder(ctx, env.otherMerchantActor, nil, order.ID); !errors.Is(err, repository.ErrCommerceNotFound) {
		t.Fatalf("another merchant accessed the order: %v", err)
	}

	prepared, err := env.storeOrders.MarkPrepared(ctx, env.ikejaStaffActor, nil, order.ID, services.PrepareCommerceStoreOrderInput{IdempotencyKey: "prepare-pickup-order"})
	if err != nil || prepared.Order.Status != models.CommerceOrderStatusReadyForPickup || prepared.Fulfilment == nil {
		t.Fatalf("prepare pickup order: view=%+v err=%v", prepared, err)
	}
	if _, err := env.delivery.DispatchOnce(ctx, 100); err != nil {
		t.Fatalf("dispatch ready notification: %v", err)
	}
	if !env.sender.contains("Your handover code is") {
		t.Fatal("customer did not receive a pickup handover code")
	}

	code, err := env.fulfilment.RevealVerificationCode(ctx, env.organizationID, customer.ID, prepared.Fulfilment.ID)
	if err != nil {
		t.Fatalf("reveal customer pickup code: %v", err)
	}
	if _, err := env.fulfilment.RecordArrival(ctx, env.ikejaStaffActor, nil, prepared.Fulfilment.ID, services.TransitionCommerceFulfilmentInput{IdempotencyKey: "pickup-arrival"}); err != nil {
		t.Fatalf("record customer arrival: %v", err)
	}
	if _, err := env.fulfilment.VerifyHandover(ctx, env.ikejaStaffActor, nil, prepared.Fulfilment.ID, services.VerifyCommerceHandoverInput{
		VerificationCode: wrongVerificationCode(code), IdempotencyKey: "pickup-wrong-code",
	}); !errors.Is(err, repository.ErrCommerceVerificationFailed) {
		t.Fatalf("wrong pickup code was accepted: %v", err)
	}
	completed, err := env.fulfilment.VerifyHandover(ctx, env.ikejaStaffActor, nil, prepared.Fulfilment.ID, services.VerifyCommerceHandoverInput{
		VerificationCode: code, IdempotencyKey: "pickup-correct-code",
	})
	if err != nil || completed.Status != models.CommerceFulfilmentStatusCompleted {
		t.Fatalf("complete secure pickup: fulfilment=%+v err=%v", completed, err)
	}
	if err := env.db.First(&order, "id = ?", order.ID).Error; err != nil || order.Status != models.CommerceOrderStatusCompleted {
		t.Fatalf("pickup did not complete order: status=%s err=%v", order.Status, err)
	}
	if _, err := env.storeOrders.MarkPrepared(ctx, env.ikejaStaffActor, nil, order.ID, services.PrepareCommerceStoreOrderInput{IdempotencyKey: "prepare-completed-order"}); !errors.Is(err, services.ErrCommerceStoreOrderState) {
		t.Fatalf("completed order re-entered preparation: %v", err)
	}

	env.handleInbound(t, services.CommerceChannelInbound{Text: "Track"})
	env.handleInbound(t, services.CommerceChannelInbound{Text: "NOT-A-REAL-ORDER"})
	if body := env.latestOutboundBody(t, testSenderID); !strings.Contains(body, "could not find") {
		t.Fatalf("invalid tracking ID was not rejected: %q", body)
	}
	env.handleInbound(t, services.CommerceChannelInbound{Text: order.OrderNumber})
	var trackingReplyCount int64
	if err := env.db.Model(&models.CommerceChannelMessage{}).
		Where("organization_id = ? AND direction = ? AND recipient_id = ? AND body LIKE ? AND body LIKE ?", env.organizationID, models.CommerceChannelDirectionOutbound, testSenderID, "%"+order.OrderNumber+"%", "%completed%").
		Count(&trackingReplyCount).Error; err != nil || trackingReplyCount != 1 {
		t.Fatalf("tracking did not return the persisted completed status: count=%d err=%v", trackingReplyCount, err)
	}

	env.handleInbound(t, services.CommerceChannelInbound{Text: "Menu"})
	env.handleInbound(t, services.CommerceChannelInbound{Text: "Complaint"})
	env.handleInbound(t, services.CommerceChannelInbound{Text: order.OrderNumber})
	env.handleInbound(t, services.CommerceChannelInbound{Text: "The lid leaked during pickup."})
	var complaint models.CommerceComplaint
	if err := env.db.Where("organization_id = ? AND order_id = ?", env.organizationID, order.ID).First(&complaint).Error; err != nil {
		t.Fatalf("WhatsApp complaint was not persisted: %v", err)
	}
	if complaint.Status != models.CommerceComplaintStatusOpen || complaint.Description != "The lid leaked during pickup." {
		t.Fatalf("unexpected complaint: %+v", complaint)
	}
}

func TestCommerceCheckoutIntegrityE2E(t *testing.T) {
	t.Run("authoritative price and duplicate checkout", func(t *testing.T) {
		env := seedEnvironment(t)
		customer := env.resolveCustomer(t, "1001")
		cart := env.createCart(t, customer, 1)
		newPrice := env.variant.PriceMinor + 25000
		if err := env.db.Model(&models.CommerceProductVariant{}).Where("id = ?", env.variant.ID).Update("price_minor", newPrice).Error; err != nil {
			t.Fatalf("change authoritative price: %v", err)
		}
		input := services.CheckoutCommerceCartInput{CartID: cart.Cart.ID, FulfilmentMode: models.FulfilmentModeCustomerPickup, IdempotencyKey: "checkout-authoritative-price"}
		order, created, err := env.orderService.CheckoutCartForChannel(context.Background(), env.organizationID, customer.ID, input)
		if err != nil || !created || len(order.Items) != 1 || order.Items[0].UnitPriceMinor != newPrice {
			t.Fatalf("authoritative checkout snapshot: created=%v order=%+v err=%v", created, order, err)
		}
		replayed, created, err := env.orderService.CheckoutCartForChannel(context.Background(), env.organizationID, customer.ID, input)
		if err != nil || created || replayed.ID != order.ID {
			t.Fatalf("duplicate checkout was not idempotent: created=%v order=%+v err=%v", created, replayed, err)
		}
	})

	t.Run("unavailable after cart", func(t *testing.T) {
		env := seedEnvironment(t)
		customer := env.resolveCustomer(t, "1002")
		cart := env.createCart(t, customer, 1)
		if err := env.db.Model(&models.CommerceStoreCatalogueItem{}).
			Where("organization_id = ? AND store_id = ? AND variant_id = ?", env.organizationID, env.ikejaStore.ID, env.variant.ID).
			Update("enabled", false).Error; err != nil {
			t.Fatalf("disable item before checkout: %v", err)
		}
		_, _, err := env.orderService.CheckoutCartForChannel(context.Background(), env.organizationID, customer.ID, services.CheckoutCommerceCartInput{
			CartID: cart.Cart.ID, FulfilmentMode: models.FulfilmentModeCustomerPickup, IdempotencyKey: "checkout-disabled-item",
		})
		if !errors.Is(err, repository.ErrCommerceInventoryUnavailable) {
			t.Fatalf("disabled item checkout error=%v", err)
		}
	})

	t.Run("concurrent last unit", func(t *testing.T) {
		env := seedEnvironment(t)
		if err := env.db.Model(&models.CommerceInventoryLevel{}).
			Where("organization_id = ? AND store_id = ? AND variant_id = ?", env.organizationID, env.ikejaStore.ID, env.variant.ID).
			Updates(map[string]interface{}{"quantity_on_hand": 1, "quantity_reserved": 0}).Error; err != nil {
			t.Fatalf("set last inventory unit: %v", err)
		}
		customerA, customerB := env.resolveCustomer(t, "1003"), env.resolveCustomer(t, "1004")
		cartA, cartB := env.createCart(t, customerA, 1), env.createCart(t, customerB, 1)
		type checkoutResult struct {
			order *models.CommerceOrder
			err   error
		}
		start := make(chan struct{})
		results := make(chan checkoutResult, 2)
		checkout := func(customer *models.CommerceCustomer, cart *services.CommerceCartView, key string) {
			<-start
			order, _, err := env.orderService.CheckoutCartForChannel(context.Background(), env.organizationID, customer.ID, services.CheckoutCommerceCartInput{
				CartID: cart.Cart.ID, FulfilmentMode: models.FulfilmentModeCustomerPickup, IdempotencyKey: key,
			})
			results <- checkoutResult{order: order, err: err}
		}
		go checkout(customerA, cartA, "concurrent-checkout-a")
		go checkout(customerB, cartB, "concurrent-checkout-b")
		close(start)
		first, second := <-results, <-results
		successes, unavailable := 0, 0
		for _, result := range []checkoutResult{first, second} {
			if result.err == nil && result.order != nil {
				successes++
			} else if errors.Is(result.err, repository.ErrCommerceInventoryUnavailable) {
				unavailable++
			} else {
				t.Fatalf("unexpected concurrent checkout result: %+v", result)
			}
		}
		if successes != 1 || unavailable != 1 {
			t.Fatalf("last-unit race: successes=%d unavailable=%d", successes, unavailable)
		}
		var level models.CommerceInventoryLevel
		if err := env.db.Where("organization_id = ? AND store_id = ? AND variant_id = ?", env.organizationID, env.ikejaStore.ID, env.variant.ID).First(&level).Error; err != nil {
			t.Fatalf("load inventory after checkout race: %v", err)
		}
		if level.QuantityOnHand != 1 || level.QuantityReserved != 1 || level.AvailableQuantity() != 0 {
			t.Fatalf("last-unit race oversold inventory: %+v", level)
		}
	})

	t.Run("expired order cannot be paid", func(t *testing.T) {
		env := seedEnvironment(t)
		customer := env.resolveCustomer(t, "1005")
		cart := env.createCart(t, customer, 1)
		order, _, err := env.orderService.CheckoutCartForChannel(context.Background(), env.organizationID, customer.ID, services.CheckoutCommerceCartInput{
			CartID: cart.Cart.ID, FulfilmentMode: models.FulfilmentModeCustomerPickup, IdempotencyKey: "checkout-expired-payment",
		})
		if err != nil {
			t.Fatalf("checkout expiring order: %v", err)
		}
		if err := env.db.Model(&models.CommerceOrder{}).Where("id = ?", order.ID).Update("payment_expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
			t.Fatalf("expire order: %v", err)
		}
		_, _, err = env.paymentService.InitializePaymentForChannel(context.Background(), env.organizationID, customer.ID, order.ID, services.InitializeCommercePaymentInput{
			Provider: env.paymentProvider.Name(), PayerEmail: "expired@example.com", IdempotencyKey: "pay-expired-order",
		})
		if !errors.Is(err, repository.ErrCommercePaymentExpired) {
			t.Fatalf("expired order payment error=%v", err)
		}
		var level models.CommerceInventoryLevel
		if err := env.db.Where("organization_id = ? AND store_id = ? AND variant_id = ?", env.organizationID, env.ikejaStore.ID, env.variant.ID).First(&level).Error; err != nil {
			t.Fatalf("load released inventory: %v", err)
		}
		if level.QuantityReserved != 0 || level.QuantityOnHand != 25 {
			t.Fatalf("expired payment did not release reservation: %+v", level)
		}
	})
}

func TestCommerceClosedStoreSelectionE2E(t *testing.T) {
	env := seedEnvironment(t)
	if err := env.db.Model(&models.CommerceStore{}).Where("id = ?", env.ikejaStore.ID).Update("status", models.CommerceStatusInactive).Error; err != nil {
		t.Fatalf("close nearest store: %v", err)
	}
	sender := "2348000000001"
	env.handleInbound(t, services.CommerceChannelInbound{SenderID: sender, Text: "Hi"})
	env.handleInbound(t, services.CommerceChannelInbound{SenderID: sender, Text: "Order"})
	latitude, longitude := 6.592294, 3.3386004
	env.handleInbound(t, services.CommerceChannelInbound{SenderID: sender, MessageType: "location", Latitude: &latitude, Longitude: &longitude})
	if body := env.latestOutboundBody(t, sender); !strings.Contains(body, env.lekkiStore.Name) || strings.Contains(body, env.ikejaStore.Name) {
		t.Fatalf("closed nearest store was selected: %q", body)
	}
}

func TestCommerceRiderFulfilmentE2E(t *testing.T) {
	for _, mode := range []string{models.FulfilmentModeCustomerRider, models.FulfilmentModeMerchantRider} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			env := seedEnvironment(t)
			ctx := context.Background()
			if mode == models.FulfilmentModeMerchantRider {
				if err := env.db.Model(&models.CommerceStoreFulfilmentMode{}).
					Where("organization_id = ? AND store_id = ? AND mode = ?", env.organizationID, env.ikejaStore.ID, mode).
					Update("enabled", true).Error; err != nil {
					t.Fatalf("enable merchant rider for E2E coverage: %v", err)
				}
			}
			customerSuffix := "2001"
			if mode == models.FulfilmentModeMerchantRider {
				customerSuffix = "2002"
			}
			customer := env.resolveCustomer(t, customerSuffix)
			cart := env.createCart(t, customer, 1)
			order, _, err := env.orderService.CheckoutCartForChannel(ctx, env.organizationID, customer.ID, services.CheckoutCommerceCartInput{
				CartID: cart.Cart.ID, FulfilmentMode: mode, IdempotencyKey: "checkout-" + mode,
			})
			if err != nil {
				t.Fatalf("checkout %s: %v", mode, err)
			}
			if mode == models.FulfilmentModeMerchantRider {
				if _, err := env.orderService.SetOrderDestinationForChannel(ctx, env.organizationID, customer.ID, order.ID, "10 E2E Street, Lagos", nil, nil); err != nil {
					t.Fatalf("set merchant-rider destination: %v", err)
				}
			}
			env.initializeAndPay(t, customer, order, mode)
			prepared, err := env.storeOrders.MarkPrepared(ctx, env.ikejaStaffActor, nil, order.ID, services.PrepareCommerceStoreOrderInput{IdempotencyKey: "prepare-" + mode})
			if err != nil || prepared.Fulfilment == nil {
				t.Fatalf("prepare %s order: %+v err=%v", mode, prepared, err)
			}
			fulfilment := prepared.Fulfilment
			if mode == models.FulfilmentModeMerchantRider {
				fee := int64(250000)
				fulfilment, err = env.fulfilment.CreateDeliveryQuote(ctx, env.ikejaStaffActor, nil, fulfilment.ID, services.CreateCommerceDeliveryQuoteInput{
					Source: models.CommerceDeliveryQuoteSourceManual, EstimatedFeeMinor: &fee, IdempotencyKey: "quote-merchant-rider",
				})
				if err != nil || len(fulfilment.Quotes) != 1 {
					t.Fatalf("create merchant-rider quote: %+v err=%v", fulfilment, err)
				}
				fulfilment, err = env.fulfilment.DecideDeliveryQuoteForCustomer(ctx, env.organizationID, customer.ID, fulfilment.ID, fulfilment.Quotes[0].ID, services.DecideCommerceDeliveryQuoteInput{
					Decision: models.CommerceDeliveryQuoteStatusAccepted, IdempotencyKey: "accept-merchant-quote",
				})
				if err != nil {
					t.Fatalf("accept merchant-rider quote: %v", err)
				}
			}
			source := models.CommerceRiderSourceCustomer
			if mode == models.FulfilmentModeMerchantRider {
				source = models.CommerceRiderSourceMerchant
			}
			fulfilment, err = env.fulfilment.AssignRider(ctx, env.ikejaStaffActor, nil, fulfilment.ID, services.AssignCommerceRiderInput{
				Source: source, RiderName: "E2E Rider", RiderPhone: "+2348112223333", TrackingURL: "https://tracking.invalid/e2e",
				IdempotencyKey: "assign-" + mode,
			})
			if err != nil || fulfilment.Status != models.CommerceFulfilmentStatusRiderAssigned {
				t.Fatalf("assign %s rider: %+v err=%v", mode, fulfilment, err)
			}
			code, err := env.fulfilment.RevealVerificationCode(ctx, env.organizationID, customer.ID, fulfilment.ID)
			if err != nil {
				t.Fatalf("reveal %s rider code: %v", mode, err)
			}
			if _, err := env.fulfilment.RecordArrival(ctx, env.ikejaStaffActor, nil, fulfilment.ID, services.TransitionCommerceFulfilmentInput{IdempotencyKey: "arrival-" + mode}); err != nil {
				t.Fatalf("record %s rider arrival: %v", mode, err)
			}
			if _, err := env.fulfilment.VerifyHandover(ctx, env.ikejaStaffActor, nil, fulfilment.ID, services.VerifyCommerceHandoverInput{
				VerificationCode: wrongVerificationCode(code), IdempotencyKey: "wrong-handover-" + mode,
			}); !errors.Is(err, repository.ErrCommerceVerificationFailed) {
				t.Fatalf("wrong %s rider code was accepted: %v", mode, err)
			}
			fulfilment, err = env.fulfilment.VerifyHandover(ctx, env.ikejaStaffActor, nil, fulfilment.ID, services.VerifyCommerceHandoverInput{
				VerificationCode: code, IdempotencyKey: "handover-" + mode,
			})
			if err != nil || fulfilment.Status != models.CommerceFulfilmentStatusOutForDelivery {
				t.Fatalf("verify %s rider handover: %+v err=%v", mode, fulfilment, err)
			}
			fulfilment, err = env.fulfilment.MarkDelivered(ctx, env.ikejaStaffActor, nil, fulfilment.ID, services.TransitionCommerceFulfilmentInput{IdempotencyKey: "delivered-" + mode})
			if err != nil || fulfilment.Status != models.CommerceFulfilmentStatusDelivered {
				t.Fatalf("mark %s delivered: %+v err=%v", mode, fulfilment, err)
			}
			fulfilment, err = env.fulfilment.CompleteFulfilment(ctx, env.ikejaStaffActor, nil, fulfilment.ID, services.TransitionCommerceFulfilmentInput{IdempotencyKey: "complete-" + mode})
			if err != nil || fulfilment.Status != models.CommerceFulfilmentStatusCompleted {
				t.Fatalf("complete %s fulfilment: %+v err=%v", mode, fulfilment, err)
			}
		})
	}
}
