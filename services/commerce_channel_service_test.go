package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type commerceChannelRepoStub struct {
	complaints    []models.CommerceComplaint
	publicChannel *repository.CommercePublicChannel
	publicErr     error
}

func (s *commerceChannelRepoStub) GetChannelConfigurationByProviderAccount(context.Context, string, string) (*models.CommerceChannelConfiguration, error) {
	return nil, repository.ErrCommerceNotFound
}
func (s *commerceChannelRepoStub) GetChannelConfiguration(context.Context, uuid.UUID, string) (*models.CommerceChannelConfiguration, error) {
	return nil, repository.ErrCommerceNotFound
}
func (s *commerceChannelRepoStub) GetActiveChannelByMerchantSlug(context.Context, string, string) (*repository.CommercePublicChannel, error) {
	if s.publicErr != nil {
		return nil, s.publicErr
	}
	if s.publicChannel == nil {
		return nil, repository.ErrCommerceNotFound
	}
	return s.publicChannel, nil
}
func (s *commerceChannelRepoStub) UpsertChannelConfiguration(context.Context, *models.CommerceChannelConfiguration) (*models.CommerceChannelConfiguration, error) {
	return nil, nil
}
func (s *commerceChannelRepoStub) ClaimInboundMessage(context.Context, *models.CommerceChannelConfiguration, uuid.UUID, repository.CommerceInboundChannelMessage, time.Time) (*repository.CommerceConversationClaim, error) {
	return nil, nil
}
func (s *commerceChannelRepoStub) CompleteInboundMessage(context.Context, repository.CommerceConversationCompletion) error {
	return nil
}
func (s *commerceChannelRepoStub) FailInboundMessage(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, time.Time) error {
	return nil
}
func (s *commerceChannelRepoStub) ClaimOutboundMessages(context.Context, int, time.Time) ([]models.CommerceChannelMessage, error) {
	return nil, nil
}
func (s *commerceChannelRepoStub) MarkOutboundMessageSent(context.Context, uuid.UUID, string, time.Time) error {
	return nil
}
func (s *commerceChannelRepoStub) MarkOutboundMessageFailed(context.Context, uuid.UUID, string, time.Time) error {
	return nil
}
func (s *commerceChannelRepoStub) CreateComplaint(_ context.Context, complaint *models.CommerceComplaint) error {
	s.complaints = append(s.complaints, *complaint)
	return nil
}
func (s *commerceChannelRepoStub) GetComplaint(context.Context, uuid.UUID, uuid.UUID) (*models.CommerceComplaint, error) {
	return nil, repository.ErrCommerceNotFound
}
func (s *commerceChannelRepoStub) ListComplaints(context.Context, uuid.UUID, repository.CommerceComplaintFilter) ([]models.CommerceComplaint, int64, error) {
	return nil, 0, nil
}
func (s *commerceChannelRepoStub) UpdateComplaint(context.Context, uuid.UUID, uuid.UUID, repository.CommerceComplaintUpdate) (*models.CommerceComplaint, error) {
	return nil, nil
}
func (s *commerceChannelRepoStub) ClaimCustomerOutboxEvents(context.Context, int, time.Time) ([]models.CommerceOutboxEvent, error) {
	return nil, nil
}
func (s *commerceChannelRepoStub) QueueOutboxNotification(context.Context, *models.CommerceOutboxEvent, uuid.UUID, repository.CommerceChannelReply, *repository.CommerceEmailNotification, time.Time) error {
	return nil
}
func (s *commerceChannelRepoStub) MarkOutboxEventFailed(context.Context, uuid.UUID, string, time.Time) error {
	return nil
}
func (s *commerceChannelRepoStub) ClaimEmailMessages(context.Context, int, time.Time) ([]models.CommerceEmailMessage, error) {
	return nil, nil
}
func (s *commerceChannelRepoStub) MarkEmailMessageSent(context.Context, uuid.UUID, time.Time) error {
	return nil
}
func (s *commerceChannelRepoStub) MarkEmailMessageFailed(context.Context, uuid.UUID, string, time.Time) error {
	return nil
}

type commerceChannelCustomerStub struct {
	cartID uuid.UUID
}

func (s *commerceChannelCustomerStub) ResolveCustomerForChannel(context.Context, uuid.UUID, ResolveCommerceCustomerInput) (*models.CommerceCustomer, bool, error) {
	return nil, false, nil
}
func (s *commerceChannelCustomerStub) UpdateCustomerForChannel(_ context.Context, organizationID, customerID uuid.UUID, displayName, email string) (*models.CommerceCustomer, error) {
	customer := &models.CommerceCustomer{ID: customerID, OrganizationID: organizationID, DisplayName: displayName}
	if email != "" {
		customer.Email = &email
	}
	return customer, nil
}
func (s *commerceChannelCustomerStub) CreateCartForChannel(_ context.Context, organizationID, customerID, storeID uuid.UUID) (*CommerceCartView, bool, error) {
	return &CommerceCartView{Cart: &models.CommerceCart{ID: s.cartID, OrganizationID: organizationID, CustomerID: customerID, StoreID: storeID, Currency: "NGN"}}, true, nil
}
func (s *commerceChannelCustomerStub) SetCartItemForChannel(_ context.Context, organizationID, customerID, cartID, variantID uuid.UUID, quantity int) (*CommerceCartView, error) {
	name := "Tea"
	return &CommerceCartView{
		Cart:  &models.CommerceCart{ID: cartID, OrganizationID: organizationID, CustomerID: customerID, Currency: "NGN"},
		Items: []CommerceCartLine{{Item: models.CommerceCartItem{VariantID: variantID, Quantity: quantity}, ProductName: &name}}, TotalMinor: 420000,
	}, nil
}

type commerceChannelOrderStub struct {
	order *models.CommerceOrder
	err   error
}

func (s *commerceChannelOrderStub) CheckoutCartForChannel(_ context.Context, organizationID, customerID uuid.UUID, input CheckoutCommerceCartInput) (*models.CommerceOrder, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	copy := *s.order
	copy.OrganizationID, copy.CustomerID, copy.CartID, copy.FulfilmentMode = organizationID, customerID, input.CartID, input.FulfilmentMode
	return &copy, true, nil
}
func (s *commerceChannelOrderStub) GetOrderForChannel(context.Context, uuid.UUID, uuid.UUID, string) (*models.CommerceOrder, error) {
	if s.err != nil {
		return nil, s.err
	}
	copy := *s.order
	return &copy, nil
}
func (s *commerceChannelOrderStub) SetOrderDestinationForChannel(_ context.Context, _, _, _ uuid.UUID, address string, latitude, longitude *float64) (*models.CommerceOrder, error) {
	copy := *s.order
	copy.DestinationAddress, copy.DestinationLatitude, copy.DestinationLongitude = &address, latitude, longitude
	return &copy, nil
}

type commerceChannelPaymentStub struct{}

func (*commerceChannelPaymentStub) InitializePaymentForChannel(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, InitializeCommercePaymentInput) (*repository.CommercePaymentSession, bool, error) {
	link := "https://pay.example/checkout"
	return &repository.CommercePaymentSession{
		Invoice: &models.CommerceInvoice{InvoiceNumber: "INV-100"}, Payment: &models.CommercePaymentTransaction{AuthorizationURL: &link},
	}, true, nil
}

type commerceChannelFulfilmentStub struct{}

func (*commerceChannelFulfilmentStub) DecideDeliveryQuoteForCustomer(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, DecideCommerceDeliveryQuoteInput) (*models.CommerceFulfilment, error) {
	return &models.CommerceFulfilment{}, nil
}
func (*commerceChannelFulfilmentStub) RevealVerificationCode(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (string, error) {
	return "123456", nil
}

func TestRiderAssignedNotificationIncludesCustomerHandoverCode(t *testing.T) {
	organizationID, customerID, orderID, fulfilmentID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	orderRepo := seededCommerceOrderRepo(&models.CommerceOrder{
		ID: orderID, OrganizationID: organizationID, CustomerID: customerID, OrderNumber: "ZC-100",
	})
	fulfilmentRepo := &commerceFulfilmentRepoStub{item: &models.CommerceFulfilment{
		ID: fulfilmentID, OrganizationID: organizationID, CustomerID: customerID, OrderID: orderID,
		Status: models.CommerceFulfilmentStatusRiderAssigned,
		RiderAssignments: []models.CommerceRiderAssignment{{
			RiderName: "Test Rider", RiderPhone: "+2348112223333", Status: models.CommerceRiderStatusAssigned,
		}},
	}}
	service := NewCommerceChannelDeliveryService(
		&commerceChannelRepoStub{}, nil, orderRepo, fulfilmentRepo, &commerceChannelFulfilmentStub{},
	)
	payload, err := json.Marshal(map[string]uuid.UUID{
		"customer_id": customerID, "order_id": orderID, "fulfilment_id": fulfilmentID,
	})
	if err != nil {
		t.Fatal(err)
	}

	recipientID, reply, _, err := service.notificationReply(context.Background(), &models.CommerceOutboxEvent{
		OrganizationID: organizationID, Topic: models.CommerceOutboxTopicRiderAssigned, Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recipientID != customerID || !strings.Contains(reply.Body, "Your handover code is 123456") || !strings.Contains(reply.Body, "Share it only when you receive the order") {
		t.Fatalf("rider notification did not securely deliver the handover code: recipient=%s body=%q", recipientID, reply.Body)
	}
}

func TestCommerceWhatsAppOrderFlowUsesCoreServicesAndPersistsState(t *testing.T) {
	organizationID, customerID, storeID, categoryID, variantID, cartID, orderID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	openMinute, closeMinute := 0, 1440
	store := models.CommerceStore{
		ID: storeID, OrganizationID: organizationID, Name: "Lekki", Address: "Admiralty Way", Timezone: "Africa/Lagos", Status: models.CommerceStatusActive,
		Hours: []models.CommerceStoreHour{{DayOfWeek: int(time.Monday), OpenMinute: &openMinute, CloseMinute: &closeMinute}},
	}
	foundation := &commerceFoundationRepoStub{listStores: []models.CommerceStore{store}, storeModes: []models.CommerceStoreFulfilmentMode{{Mode: models.FulfilmentModeCustomerPickup, Enabled: true}}}
	catalogue := &commerceCatalogueRepoStub{storeCatalogue: []repository.CommerceStoreCatalogueEntry{{
		StoreID: storeID, CategoryID: categoryID, CategoryName: "Tea", ProductName: "Lemon Tea", ProductCurrency: "NGN",
		VariantID: variantID, VariantName: "Regular", EffectivePriceMinor: 420000, Enabled: true, AvailableQuantity: 10,
	}}}
	service := NewCommerceChannelService(&commerceChannelRepoStub{}, foundation, catalogue, &commerceChannelCustomerStub{cartID: cartID}, &commerceChannelOrderStub{order: &models.CommerceOrder{
		ID: orderID, StoreID: storeID, OrderNumber: "ORD-100", Currency: "NGN", TotalMinor: 420000,
	}}, &commerceChannelPaymentStub{}, &commerceChannelFulfilmentStub{})
	service.now = func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC) }
	configuration := &models.CommerceChannelConfiguration{ID: uuid.New(), OrganizationID: organizationID, WelcomeMessage: "Welcome"}
	email := "customer@example.com"
	customer := &models.CommerceCustomer{ID: customerID, OrganizationID: organizationID, DisplayName: "Test Customer", Email: &email}
	conversation := &models.CommerceConversation{ID: uuid.New(), State: models.CommerceConversationStateIntent, Context: json.RawMessage(`{}`)}

	steps := []CommerceChannelInbound{
		{SelectionID: "intent:order"}, {Text: "list"}, {SelectionID: "store:" + storeID.String()},
		{SelectionID: "category:" + categoryID.String()}, {SelectionID: "product:" + variantID.String()},
		{Text: "2"}, {SelectionID: "cart:checkout"}, {SelectionID: "fulfilment:pickup"},
	}
	for _, step := range steps {
		state, intent, conversationContext, replies, err := service.processInbound(context.Background(), configuration, customer, conversation, step)
		if err != nil {
			t.Fatalf("process state %s: %v", conversation.State, err)
		}
		encoded, _ := json.Marshal(conversationContext)
		conversation.State, conversation.Context, conversation.CurrentIntent = state, encoded, optionalChannelString(intent)
		if len(replies) == 0 {
			t.Fatalf("state %s produced no customer reply", state)
		}
	}
	if conversation.State != models.CommerceConversationStateIntent || !strings.Contains(string(conversation.Context), "{}") {
		t.Fatalf("checkout did not complete back to the intent menu: state=%s context=%s", conversation.State, conversation.Context)
	}
}

func TestCommerceWhatsAppTrackingRejectsUnknownOrder(t *testing.T) {
	service := &CommerceChannelService{orders: &commerceChannelOrderStub{err: repository.ErrCommerceNotFound}}
	configuration := &models.CommerceChannelConfiguration{OrganizationID: uuid.New()}
	customer := &models.CommerceCustomer{ID: uuid.New()}
	conversation := &models.CommerceConversation{State: models.CommerceConversationStateOrderID, Context: json.RawMessage(`{}`)}
	state, _, _, replies, err := service.processInbound(context.Background(), configuration, customer, conversation, CommerceChannelInbound{Text: "ORD-MISSING"})
	if err != nil || state != models.CommerceConversationStateOrderID || len(replies) != 1 || !strings.Contains(replies[0].Body, "could not find") {
		t.Fatalf("invalid tracking response: state=%s replies=%+v err=%v", state, replies, err)
	}
}

func TestCommerceWhatsAppComplaintCreatesTenantScopedSupportRecord(t *testing.T) {
	repo := &commerceChannelRepoStub{}
	service := &CommerceChannelService{repo: repo}
	organizationID, customerID, conversationID, orderID, storeID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	contextValue, _ := json.Marshal(commerceConversationContext{ComplaintOrderID: &orderID, ComplaintStoreID: &storeID})
	conversation := &models.CommerceConversation{ID: conversationID, State: models.CommerceConversationStateComplaintDescription, Context: contextValue}
	state, _, _, _, err := service.processInbound(context.Background(), &models.CommerceChannelConfiguration{OrganizationID: organizationID}, &models.CommerceCustomer{ID: customerID}, conversation, CommerceChannelInbound{Text: "The item delivered was damaged."})
	if err != nil || state != models.CommerceConversationStateIntent || len(repo.complaints) != 1 {
		t.Fatalf("create complaint: state=%s count=%d err=%v", state, len(repo.complaints), err)
	}
	item := repo.complaints[0]
	if item.OrganizationID != organizationID || item.CustomerID != customerID || item.OrderID == nil || *item.OrderID != orderID {
		t.Fatalf("complaint lost tenant/customer/order ownership: %+v", item)
	}
}

func TestCommerceWhatsAppDoesNotRecommendClosedStore(t *testing.T) {
	organizationID, openStoreID, closedStoreID := uuid.New(), uuid.New(), uuid.New()
	openMinute, closeMinute := 0, 1440
	stores := []models.CommerceStore{
		{ID: closedStoreID, OrganizationID: organizationID, Status: models.CommerceStatusActive, Timezone: "Africa/Lagos", Hours: []models.CommerceStoreHour{{DayOfWeek: int(time.Monday), IsClosed: true}}},
		{ID: openStoreID, OrganizationID: organizationID, Status: models.CommerceStatusActive, Timezone: "Africa/Lagos", Hours: []models.CommerceStoreHour{{DayOfWeek: int(time.Monday), OpenMinute: &openMinute, CloseMinute: &closeMinute}}},
	}
	catalogue := &commerceCatalogueRepoStub{storeCatalogue: []repository.CommerceStoreCatalogueEntry{{Enabled: true, AvailableQuantity: 1}}}
	service := &CommerceChannelService{foundationRepo: &commerceFoundationRepoStub{listStores: stores}, catalogueRepo: catalogue}
	service.now = func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC) }
	available, err := service.availableStores(context.Background(), organizationID, nil, nil)
	if err != nil || len(available) != 1 || available[0].ID != openStoreID {
		t.Fatalf("closed-store filtering failed: stores=%+v err=%v", available, err)
	}
}

func TestResolvePublicWhatsAppLinkUsesTenantChannelConfiguration(t *testing.T) {
	displayPhoneNumber := "+234 (800) 123-4567"
	service := &CommerceChannelService{repo: &commerceChannelRepoStub{publicChannel: &repository.CommercePublicChannel{
		MerchantSlug:        "bing-chun-nigeria",
		MerchantDisplayName: "Bing Chun Nigeria",
		Configuration: models.CommerceChannelConfiguration{
			Channel: models.CommerceChannelWhatsApp, DisplayPhoneNumber: &displayPhoneNumber, Status: models.CommerceStatusActive,
		},
	}}}

	link, err := service.ResolvePublicWhatsAppLink(context.Background(), "BING-CHUN-NIGERIA")
	if err != nil {
		t.Fatalf("resolve public WhatsApp link: %v", err)
	}
	if link.MerchantSlug != "bing-chun-nigeria" || link.MerchantDisplayName != "Bing Chun Nigeria" || link.DisplayPhoneNumber != displayPhoneNumber {
		t.Fatalf("unexpected public channel metadata: %+v", link)
	}
	if link.URL != "https://wa.me/2348001234567?text=Hi" {
		t.Fatalf("unexpected WhatsApp URL: %s", link.URL)
	}
}

func TestResolvePublicWhatsAppLinkHidesInvalidChannelConfiguration(t *testing.T) {
	invalidPhoneNumber := "0800-123-4567"
	service := &CommerceChannelService{repo: &commerceChannelRepoStub{publicChannel: &repository.CommercePublicChannel{
		MerchantSlug:  "merchant-a",
		Configuration: models.CommerceChannelConfiguration{DisplayPhoneNumber: &invalidPhoneNumber, Status: models.CommerceStatusActive},
	}}}

	_, err := service.ResolvePublicWhatsAppLink(context.Background(), "merchant-a")
	if err != repository.ErrCommerceNotFound {
		t.Fatalf("expected not found for an invalid public phone, got %v", err)
	}
}

func TestCommerceQuoteDecisionParserRejectsInvalidIDs(t *testing.T) {
	_, _, _, ok := parseCommerceQuoteDecision("quote:accepted:not-a-uuid:also-bad")
	if ok {
		t.Fatal("invalid quote identifiers were accepted")
	}
}
