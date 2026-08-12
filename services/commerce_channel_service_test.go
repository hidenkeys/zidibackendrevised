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
	order     *models.CommerceOrder
	err       error
	cancelled bool
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
func (s *commerceChannelOrderStub) CancelOrderForChannel(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) (*models.CommerceOrder, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.cancelled = true
	copy := *s.order
	copy.Status = models.CommerceOrderStatusCancelled
	return &copy, nil
}

type commerceChannelPaymentStub struct {
	session *repository.CommercePaymentSession
	err     error
	input   InitializeCommercePaymentInput
}

func (s *commerceChannelPaymentStub) InitializePaymentForChannel(_ context.Context, _, _, _ uuid.UUID, input InitializeCommercePaymentInput) (*repository.CommercePaymentSession, bool, error) {
	s.input = input
	if s.err != nil {
		return nil, false, s.err
	}
	if s.session != nil {
		return s.session, false, nil
	}
	link := "https://pay.example/checkout"
	return &repository.CommercePaymentSession{
		Invoice: &models.CommerceInvoice{InvoiceNumber: "INV-100"}, Payment: &models.CommercePaymentTransaction{AuthorizationURL: &link},
	}, true, nil
}

type commerceChannelFulfilmentStub struct{}

func (*commerceChannelFulfilmentStub) PreparePaidOrderForNotification(_ context.Context, organizationID, orderID uuid.UUID) (*models.CommerceFulfilment, error) {
	return &models.CommerceFulfilment{ID: uuid.New(), OrganizationID: organizationID, OrderID: orderID, Mode: models.FulfilmentModeCustomerPickup}, nil
}

func (*commerceChannelFulfilmentStub) DecideDeliveryQuoteForCustomer(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, DecideCommerceDeliveryQuoteInput) (*models.CommerceFulfilment, error) {
	return &models.CommerceFulfilment{}, nil
}
func (*commerceChannelFulfilmentStub) DecideDeliveryConfirmationForCustomer(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, string) (*models.CommerceFulfilment, error) {
	return &models.CommerceFulfilment{}, nil
}
func (*commerceChannelFulfilmentStub) RevealVerificationCode(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (string, error) {
	return "123456", nil
}

func TestRiderAssignedNotificationIncludesDeliveryReference(t *testing.T) {
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
	if recipientID != customerID || !strings.Contains(reply.Body, "Delivery reference: 123456") || !strings.Contains(reply.Body, "Test Rider") {
		t.Fatalf("rider notification did not include the delivery reference and rider: recipient=%s body=%q", recipientID, reply.Body)
	}
}

func TestHandoverCodeReminderIncludesExistingProtectedCode(t *testing.T) {
	organizationID, customerID, orderID, fulfilmentID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	service := NewCommerceChannelDeliveryService(
		&commerceChannelRepoStub{}, nil,
		seededCommerceOrderRepo(&models.CommerceOrder{ID: orderID, OrganizationID: organizationID, CustomerID: customerID, OrderNumber: "ZC-200"}),
		&commerceFulfilmentRepoStub{item: &models.CommerceFulfilment{ID: fulfilmentID, OrganizationID: organizationID, CustomerID: customerID, OrderID: orderID}},
		&commerceChannelFulfilmentStub{},
	)
	payload, _ := json.Marshal(map[string]uuid.UUID{"customer_id": customerID, "order_id": orderID, "fulfilment_id": fulfilmentID})
	recipientID, reply, _, err := service.notificationReply(context.Background(), &models.CommerceOutboxEvent{
		OrganizationID: organizationID, Topic: models.CommerceOutboxTopicHandoverCodeReminder, Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recipientID != customerID || !strings.Contains(reply.Body, "your handover code is 123456") {
		t.Fatalf("handover reminder did not include the protected code: recipient=%s body=%q", recipientID, reply.Body)
	}
}

func TestDeliveryConfirmationNotificationIncludesOrderAndDecisionButtons(t *testing.T) {
	organizationID, customerID, orderID, fulfilmentID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	service := NewCommerceChannelDeliveryService(
		&commerceChannelRepoStub{}, nil,
		seededCommerceOrderRepo(&models.CommerceOrder{
			ID: orderID, OrganizationID: organizationID, CustomerID: customerID, OrderNumber: "ZC-300",
			Items: []models.CommerceOrderItem{{ProductName: "Original Milk Tea", Quantity: 2}},
		}),
		&commerceFulfilmentRepoStub{}, &commerceChannelFulfilmentStub{},
	)
	payload, err := json.Marshal(map[string]uuid.UUID{
		"customer_id": customerID, "order_id": orderID, "fulfilment_id": fulfilmentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, reply, _, err := service.notificationReply(context.Background(), &models.CommerceOutboxEvent{
		OrganizationID: organizationID, Topic: models.CommerceOutboxTopicDeliveryConfirmationRequested, Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Body, "ZC-300") || !strings.Contains(reply.Body, "2x Original Milk Tea") || len(reply.Options) != 2 {
		t.Fatalf("unexpected delivery confirmation reply: %#v", reply)
	}
	if reply.Options[0].ID != "delivery:received:"+fulfilmentID.String() || reply.Options[1].ID != "delivery:not_received:"+fulfilmentID.String() {
		t.Fatalf("unexpected delivery confirmation buttons: %#v", reply.Options)
	}
}

func TestParseCommerceDeliveryConfirmation(t *testing.T) {
	fulfilmentID := uuid.New()
	decision, parsedID, ok := parseCommerceDeliveryConfirmation("delivery:received:" + fulfilmentID.String())
	if !ok || decision != models.CommerceDeliveryConfirmationReceived || parsedID != fulfilmentID {
		t.Fatalf("valid confirmation was not parsed: decision=%q id=%s ok=%t", decision, parsedID, ok)
	}
	if _, _, ok := parseCommerceDeliveryConfirmation("delivery:maybe:" + fulfilmentID.String()); ok {
		t.Fatal("unsupported delivery confirmation was accepted")
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

func TestCommerceWhatsAppPaymentEmailMissingLinkReturnsRetryMessage(t *testing.T) {
	organizationID, customerID, orderID, cartID, storeID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	contextValue := commerceConversationContext{
		CartID: &cartID, StoreID: &storeID, OrderID: &orderID,
		FulfilmentMode: models.FulfilmentModeCustomerPickup,
	}
	service := NewCommerceChannelService(
		&commerceChannelRepoStub{}, &commerceFoundationRepoStub{}, &commerceCatalogueRepoStub{},
		&commerceChannelCustomerStub{cartID: cartID},
		&commerceChannelOrderStub{order: &models.CommerceOrder{
			ID: orderID, OrganizationID: organizationID, CustomerID: customerID, StoreID: storeID,
			OrderNumber: "ORD-101", Currency: "NGN", TotalMinor: 420000,
		}},
		&commerceChannelPaymentStub{session: &repository.CommercePaymentSession{
			Invoice: &models.CommerceInvoice{InvoiceNumber: "INV-101"},
			Payment: &models.CommercePaymentTransaction{},
		}},
		&commerceChannelFulfilmentStub{},
	)
	customer := &models.CommerceCustomer{ID: customerID, OrganizationID: organizationID, DisplayName: "Test Customer"}
	conversation := &models.CommerceConversation{ID: uuid.New(), State: models.CommerceConversationStatePaymentEmail}

	encodedContext, _ := json.Marshal(contextValue)
	conversation.Context = encodedContext
	conversation.CurrentIntent = optionalChannelString(models.CommerceConversationIntentOrder)
	state, intent, updatedContext, replies, err := service.processInbound(
		context.Background(),
		&models.CommerceChannelConfiguration{OrganizationID: organizationID},
		customer,
		conversation,
		CommerceChannelInbound{Text: "customer@example.com"},
	)
	if err != nil {
		t.Fatalf("missing payment link should not fail the webhook: %v", err)
	}
	if state != models.CommerceConversationStatePaymentEmail || intent != models.CommerceConversationIntentOrder || len(replies) != 1 {
		t.Fatalf("unexpected retry response: state=%s intent=%s replies=%d", state, intent, len(replies))
	}
	if !strings.Contains(replies[0].Body, "could not create the payment link") || updatedContext.OrderID == nil || *updatedContext.OrderID != orderID {
		t.Fatalf("retry reply/context missing expected details: body=%q context=%+v", replies[0].Body, updatedContext)
	}
}

func TestCommerceWhatsAppExpiredPaymentAsksBeforeRegenerating(t *testing.T) {
	organizationID, customerID, orderID, cartID, storeID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	contextValue := commerceConversationContext{
		CartID: &cartID, StoreID: &storeID, OrderID: &orderID,
		FulfilmentMode: models.FulfilmentModeCustomerPickup,
	}
	payments := &commerceChannelPaymentStub{err: repository.ErrCommercePaymentExpired}
	service := NewCommerceChannelService(
		&commerceChannelRepoStub{}, &commerceFoundationRepoStub{}, &commerceCatalogueRepoStub{},
		&commerceChannelCustomerStub{cartID: cartID},
		&commerceChannelOrderStub{order: &models.CommerceOrder{ID: orderID, StoreID: storeID}},
		payments,
		&commerceChannelFulfilmentStub{},
	)
	customer := &models.CommerceCustomer{ID: customerID, OrganizationID: organizationID, DisplayName: "Test Customer"}
	conversation := &models.CommerceConversation{ID: uuid.New(), State: models.CommerceConversationStatePaymentEmail}
	encodedContext, _ := json.Marshal(contextValue)
	conversation.Context = encodedContext
	conversation.CurrentIntent = optionalChannelString(models.CommerceConversationIntentOrder)

	state, intent, updatedContext, replies, err := service.processInbound(
		context.Background(),
		&models.CommerceChannelConfiguration{OrganizationID: organizationID},
		customer,
		conversation,
		CommerceChannelInbound{Text: "customer@example.com"},
	)
	if err != nil {
		t.Fatalf("expired payment should become a renewal prompt: %v", err)
	}
	if state != models.CommerceConversationStatePaymentRenewal || intent != models.CommerceConversationIntentOrder || len(replies) != 1 {
		t.Fatalf("unexpected renewal prompt: state=%s intent=%s replies=%d", state, intent, len(replies))
	}
	if updatedContext.OrderID == nil || *updatedContext.OrderID != orderID || updatedContext.PaymentEmail != "customer@example.com" {
		t.Fatalf("renewal context missing order/email: %+v", updatedContext)
	}
	if len(replies[0].Options) != 2 || payments.input.RenewExpired {
		t.Fatalf("renewal prompt/options or initial renewal flag wrong: reply=%+v input=%+v", replies[0], payments.input)
	}
}

func TestCommerceWhatsAppExpiredPaymentDecisionCanRenewOrCancel(t *testing.T) {
	organizationID, customerID, orderID := uuid.New(), uuid.New(), uuid.New()
	email := "customer@example.com"
	contextValue, _ := json.Marshal(commerceConversationContext{OrderID: &orderID, PaymentEmail: email})
	conversation := &models.CommerceConversation{
		ID: uuid.New(), State: models.CommerceConversationStatePaymentRenewal,
		CurrentIntent: optionalChannelString(models.CommerceConversationIntentOrder), Context: contextValue,
	}
	payments := &commerceChannelPaymentStub{}
	orderStub := &commerceChannelOrderStub{order: &models.CommerceOrder{ID: orderID, CustomerID: customerID, Status: models.CommerceOrderStatusPaymentExpired}}
	service := NewCommerceChannelService(&commerceChannelRepoStub{}, &commerceFoundationRepoStub{}, &commerceCatalogueRepoStub{}, &commerceChannelCustomerStub{}, orderStub, payments, &commerceChannelFulfilmentStub{})
	service.now = func() time.Time { return time.Date(2026, 8, 12, 19, 0, 0, 0, time.UTC) }
	customer := &models.CommerceCustomer{ID: customerID, OrganizationID: organizationID, Email: &email}

	state, _, _, replies, err := service.processInbound(
		context.Background(),
		&models.CommerceChannelConfiguration{OrganizationID: organizationID},
		customer,
		conversation,
		CommerceChannelInbound{SelectionID: "payment:renew"},
	)
	if err != nil || state != models.CommerceConversationStateIntent || len(replies) != 1 || !payments.input.RenewExpired {
		t.Fatalf("renew decision did not create a fresh link: state=%s replies=%+v input=%+v err=%v", state, replies, payments.input, err)
	}

	payments = &commerceChannelPaymentStub{}
	orderStub = &commerceChannelOrderStub{order: &models.CommerceOrder{ID: orderID, CustomerID: customerID, Status: models.CommerceOrderStatusPaymentExpired}}
	service = NewCommerceChannelService(&commerceChannelRepoStub{}, &commerceFoundationRepoStub{}, &commerceCatalogueRepoStub{}, &commerceChannelCustomerStub{}, orderStub, payments, &commerceChannelFulfilmentStub{})
	state, _, updatedContext, replies, err := service.processInbound(
		context.Background(),
		&models.CommerceChannelConfiguration{OrganizationID: organizationID, WelcomeMessage: "Welcome"},
		customer,
		conversation,
		CommerceChannelInbound{SelectionID: "payment:cancel"},
	)
	if err != nil || state != models.CommerceConversationStateIntent || !orderStub.cancelled || len(replies) != 2 {
		t.Fatalf("cancel decision did not cancel order: state=%s cancelled=%t replies=%+v err=%v", state, orderStub.cancelled, replies, err)
	}
	if updatedContext.OrderID != nil || updatedContext.PaymentEmail != "" {
		t.Fatalf("cancel decision did not clear context: %+v", updatedContext)
	}
}

func TestCommerceWhatsAppOrderPromptRestartsStaleQuantitySession(t *testing.T) {
	organizationID, customerID, storeID, variantID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	contextValue, _ := json.Marshal(commerceConversationContext{
		StoreID: &storeID, VariantID: &variantID,
		OptionKind: "product", OptionIDs: []uuid.UUID{variantID},
	})
	service := NewCommerceChannelService(&commerceChannelRepoStub{}, &commerceFoundationRepoStub{}, &commerceCatalogueRepoStub{}, &commerceChannelCustomerStub{}, &commerceChannelOrderStub{}, &commerceChannelPaymentStub{}, &commerceChannelFulfilmentStub{})
	conversation := &models.CommerceConversation{
		ID: uuid.New(), OrganizationID: organizationID, CustomerID: customerID,
		State: models.CommerceConversationStateQuantity, CurrentIntent: optionalChannelString(models.CommerceConversationIntentOrder),
		Context: contextValue,
	}

	state, intent, updatedContext, replies, err := service.processInbound(
		context.Background(),
		&models.CommerceChannelConfiguration{OrganizationID: organizationID},
		&models.CommerceCustomer{ID: customerID, OrganizationID: organizationID},
		conversation,
		CommerceChannelInbound{Text: "Hi Zidi, I would like to place an order from Bing Chun Nigeria."},
	)
	if err != nil {
		t.Fatalf("restart prompt failed: %v", err)
	}
	if state != models.CommerceConversationStateLocation || intent != models.CommerceConversationIntentOrder || len(replies) != 1 {
		t.Fatalf("prompt did not restart order flow: state=%s intent=%s replies=%d", state, intent, len(replies))
	}
	if updatedContext.StoreID != nil || updatedContext.VariantID != nil || !strings.Contains(replies[0].Body, "Starting a fresh order") {
		t.Fatalf("stale context was not cleared: context=%+v reply=%q", updatedContext, replies[0].Body)
	}
}

func TestCommerceWhatsAppCancelClearsPrePaymentSession(t *testing.T) {
	organizationID, customerID, storeID, variantID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	contextValue, _ := json.Marshal(commerceConversationContext{StoreID: &storeID, VariantID: &variantID})
	service := NewCommerceChannelService(&commerceChannelRepoStub{}, &commerceFoundationRepoStub{}, &commerceCatalogueRepoStub{}, &commerceChannelCustomerStub{}, &commerceChannelOrderStub{}, &commerceChannelPaymentStub{}, &commerceChannelFulfilmentStub{})
	conversation := &models.CommerceConversation{
		ID: uuid.New(), OrganizationID: organizationID, CustomerID: customerID,
		State: models.CommerceConversationStateQuantity, CurrentIntent: optionalChannelString(models.CommerceConversationIntentOrder),
		Context: contextValue,
	}

	state, intent, updatedContext, replies, err := service.processInbound(
		context.Background(),
		&models.CommerceChannelConfiguration{OrganizationID: organizationID, WelcomeMessage: "Welcome"},
		&models.CommerceCustomer{ID: customerID, OrganizationID: organizationID},
		conversation,
		CommerceChannelInbound{Text: "cancel"},
	)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if state != models.CommerceConversationStateIntent || intent != "" || len(replies) != 2 {
		t.Fatalf("cancel did not return to main menu: state=%s intent=%s replies=%d", state, intent, len(replies))
	}
	if updatedContext.StoreID != nil || updatedContext.VariantID != nil || !strings.Contains(replies[0].Body, "ended the current pre-payment session") {
		t.Fatalf("cancel did not clear context: context=%+v firstReply=%q", updatedContext, replies[0].Body)
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
