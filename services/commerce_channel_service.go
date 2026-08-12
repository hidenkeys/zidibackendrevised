package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type CommerceChannelInbound struct {
	ProviderAccountID string
	ExternalMessageID string
	SenderID          string
	SenderName        string
	MessageType       string
	Text              string
	SelectionID       string
	Latitude          *float64
	Longitude         *float64
	Payload           json.RawMessage
}

type CommerceChannelHandleResult struct {
	Handled   bool
	Duplicate bool
}

type ConfigureCommerceChannelInput struct {
	ProviderAccountID  string
	DisplayPhoneNumber string
	WelcomeMessage     string
	Status             string
}

type CommerceWhatsAppLink struct {
	MerchantSlug        string
	MerchantDisplayName string
	DisplayPhoneNumber  string
	URL                 string
}

type CommerceComplaintListInput struct {
	StoreID *uuid.UUID
	Status  *string
	Limit   int
	Offset  int
}

type UpdateCommerceComplaintInput struct {
	Status     string
	Resolution string
}

type commerceChannelCustomerCart interface {
	ResolveCustomerForChannel(context.Context, uuid.UUID, ResolveCommerceCustomerInput) (*models.CommerceCustomer, bool, error)
	UpdateCustomerForChannel(context.Context, uuid.UUID, uuid.UUID, string, string) (*models.CommerceCustomer, error)
	CreateCartForChannel(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*CommerceCartView, bool, error)
	SetCartItemForChannel(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, int) (*CommerceCartView, error)
}

type commerceChannelOrder interface {
	CheckoutCartForChannel(context.Context, uuid.UUID, uuid.UUID, CheckoutCommerceCartInput) (*models.CommerceOrder, bool, error)
	GetOrderForChannel(context.Context, uuid.UUID, uuid.UUID, string) (*models.CommerceOrder, error)
	SetOrderDestinationForChannel(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, *float64, *float64) (*models.CommerceOrder, error)
}

type commerceChannelPayment interface {
	InitializePaymentForChannel(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, InitializeCommercePaymentInput) (*repository.CommercePaymentSession, bool, error)
}

type commerceChannelFulfilment interface {
	PreparePaidOrderForNotification(context.Context, uuid.UUID, uuid.UUID) (*models.CommerceFulfilment, error)
	DecideDeliveryQuoteForCustomer(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, DecideCommerceDeliveryQuoteInput) (*models.CommerceFulfilment, error)
	DecideDeliveryConfirmationForCustomer(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, string) (*models.CommerceFulfilment, error)
	RevealVerificationCode(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (string, error)
}

type CommerceChannelService struct {
	repo           repository.CommerceChannelRepository
	foundationRepo repository.CommerceFoundationRepository
	catalogueRepo  repository.CommerceCatalogueRepository
	customers      commerceChannelCustomerCart
	orders         commerceChannelOrder
	payments       commerceChannelPayment
	fulfilments    commerceChannelFulfilment
	now            func() time.Time
}

func NewCommerceChannelService(repo repository.CommerceChannelRepository, foundationRepo repository.CommerceFoundationRepository, catalogueRepo repository.CommerceCatalogueRepository, customers commerceChannelCustomerCart, orders commerceChannelOrder, payments commerceChannelPayment, fulfilments commerceChannelFulfilment) *CommerceChannelService {
	return &CommerceChannelService{
		repo: repo, foundationRepo: foundationRepo, catalogueRepo: catalogueRepo,
		customers: customers, orders: orders, payments: payments, fulfilments: fulfilments, now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *CommerceChannelService) HandleInbound(ctx context.Context, input CommerceChannelInbound) (*CommerceChannelHandleResult, error) {
	providerAccountID := strings.TrimSpace(input.ProviderAccountID)
	senderID := normalizeChannelIdentifier(input.SenderID)
	messageID := strings.TrimSpace(input.ExternalMessageID)
	if providerAccountID == "" || senderID == "" || messageID == "" {
		return nil, fmt.Errorf("%w: provider account, sender, and message identifiers are required", ErrCommerceValidation)
	}
	configuration, err := s.repo.GetChannelConfigurationByProviderAccount(ctx, models.CommerceChannelWhatsApp, providerAccountID)
	if errors.Is(err, repository.ErrCommerceNotFound) {
		return &CommerceChannelHandleResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	if configuration.Status != models.CommerceStatusActive {
		return &CommerceChannelHandleResult{Handled: true}, nil
	}
	customer, _, err := s.customers.ResolveCustomerForChannel(ctx, configuration.OrganizationID, ResolveCommerceCustomerInput{
		Channel: models.CommerceIdentityChannelWhatsApp, Identifier: senderID,
		DisplayName: strings.TrimSpace(input.SenderName), Verified: true,
	})
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	claim, err := s.repo.ClaimInboundMessage(ctx, configuration, customer.ID, repository.CommerceInboundChannelMessage{
		ExternalMessageID: messageID, SenderID: senderID, MessageType: strings.TrimSpace(input.MessageType),
		Body: strings.TrimSpace(input.Text), Payload: input.Payload,
	}, now)
	if err != nil {
		return nil, err
	}
	if claim.Duplicate {
		return &CommerceChannelHandleResult{Handled: true, Duplicate: true}, nil
	}

	state, intent, conversationContext, replies, err := s.processInbound(ctx, configuration, customer, claim.Conversation, input)
	if err != nil {
		_ = s.repo.FailInboundMessage(ctx, configuration.OrganizationID, claim.Conversation.ID, claim.Message.ID, err.Error(), s.now().UTC())
		return nil, err
	}
	encodedContext, err := json.Marshal(conversationContext)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CompleteInboundMessage(ctx, repository.CommerceConversationCompletion{
		OrganizationID: configuration.OrganizationID, ConversationID: claim.Conversation.ID, MessageID: claim.Message.ID,
		State: state, CurrentIntent: optionalChannelString(intent), Context: encodedContext, Replies: replies, Now: s.now().UTC(),
	}); err != nil {
		return nil, err
	}
	return &CommerceChannelHandleResult{Handled: true}, nil
}

func (s *CommerceChannelService) ConfigureWhatsApp(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, input ConfigureCommerceChannelInput) (*models.CommerceChannelConfiguration, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if _, err := s.foundationRepo.GetMerchantProfile(ctx, organizationID); err != nil {
		return nil, err
	}
	providerAccountID := strings.TrimSpace(input.ProviderAccountID)
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = models.CommerceStatusActive
	}
	if len(providerAccountID) < 3 || len(providerAccountID) > 200 || (status != models.CommerceStatusActive && status != models.CommerceStatusInactive) {
		return nil, fmt.Errorf("%w: a provider account and valid status are required", ErrCommerceValidation)
	}
	welcome := strings.TrimSpace(input.WelcomeMessage)
	displayPhoneNumber := strings.TrimSpace(input.DisplayPhoneNumber)
	if len(welcome) > 1000 || len(displayPhoneNumber) > 40 {
		return nil, fmt.Errorf("%w: channel display values are too long", ErrCommerceValidation)
	}
	if displayPhoneNumber != "" {
		if _, ok := commerceWhatsAppLinkPhone(displayPhoneNumber); !ok {
			return nil, fmt.Errorf("%w: display phone number must use international format", ErrCommerceValidation)
		}
	}
	return s.repo.UpsertChannelConfiguration(ctx, &models.CommerceChannelConfiguration{
		ID: uuid.New(), OrganizationID: organizationID, Channel: models.CommerceChannelWhatsApp,
		ProviderAccountID: providerAccountID, DisplayPhoneNumber: optionalChannelString(displayPhoneNumber),
		WelcomeMessage: welcome, Status: status,
	})
}

func (s *CommerceChannelService) GetWhatsAppConfiguration(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID) (*models.CommerceChannelConfiguration, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	return s.repo.GetChannelConfiguration(ctx, organizationID, models.CommerceChannelWhatsApp)
}

func (s *CommerceChannelService) ResolvePublicWhatsAppLink(ctx context.Context, merchantSlug string) (*CommerceWhatsAppLink, error) {
	slug := strings.ToLower(strings.TrimSpace(merchantSlug))
	if !commerceSlugPattern.MatchString(slug) {
		return nil, repository.ErrCommerceNotFound
	}
	channel, err := s.repo.GetActiveChannelByMerchantSlug(ctx, slug, models.CommerceChannelWhatsApp)
	if err != nil {
		return nil, err
	}
	if channel.Configuration.DisplayPhoneNumber == nil {
		return nil, repository.ErrCommerceNotFound
	}
	displayPhoneNumber := strings.TrimSpace(*channel.Configuration.DisplayPhoneNumber)
	linkPhone, ok := commerceWhatsAppLinkPhone(displayPhoneNumber)
	if !ok {
		return nil, repository.ErrCommerceNotFound
	}
	return &CommerceWhatsAppLink{
		MerchantSlug:        channel.MerchantSlug,
		MerchantDisplayName: channel.MerchantDisplayName,
		DisplayPhoneNumber:  displayPhoneNumber,
		URL:                 "https://wa.me/" + linkPhone + "?text=Hi",
	}, nil
}

func (s *CommerceChannelService) ListComplaints(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, input CommerceComplaintListInput) ([]models.CommerceComplaint, int64, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, 0, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, 0, err
	}
	if input.StoreID != nil {
		if _, err := s.foundationRepo.GetStore(ctx, organizationID, *input.StoreID, storeScope(actor)); err != nil {
			return nil, 0, err
		}
	}
	if input.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*input.Status))
		if !isCommerceComplaintStatus(status) {
			return nil, 0, fmt.Errorf("%w: invalid complaint status", ErrCommerceValidation)
		}
		input.Status = &status
	}
	limit, offset := commercePagination(input.Limit, input.Offset)
	return s.repo.ListComplaints(ctx, organizationID, repository.CommerceComplaintFilter{
		StoreID: input.StoreID, Status: input.Status, AssignedUserID: storeScope(actor), Limit: limit, Offset: offset,
	})
}

func (s *CommerceChannelService) UpdateComplaint(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, complaintID uuid.UUID, input UpdateCommerceComplaintInput) (*models.CommerceComplaint, error) {
	if !canAccessCommerce(actor.Role) || complaintID == uuid.Nil {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.GetComplaint(ctx, organizationID, complaintID)
	if err != nil {
		return nil, err
	}
	if item.StoreID == nil && storeScope(actor) != nil {
		return nil, ErrCommerceForbidden
	}
	if item.StoreID != nil {
		if _, err := s.foundationRepo.GetStore(ctx, organizationID, *item.StoreID, storeScope(actor)); err != nil {
			return nil, err
		}
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	resolution := strings.TrimSpace(input.Resolution)
	if !isCommerceComplaintStatus(status) || len(resolution) > 4000 {
		return nil, fmt.Errorf("%w: invalid complaint update", ErrCommerceValidation)
	}
	var resolvedAt *time.Time
	if status == models.CommerceComplaintStatusResolved || status == models.CommerceComplaintStatusClosed {
		if resolution == "" {
			return nil, fmt.Errorf("%w: a resolution is required", ErrCommerceValidation)
		}
		now := s.now().UTC()
		resolvedAt = &now
	} else {
		resolution = ""
	}
	return s.repo.UpdateComplaint(ctx, organizationID, complaintID, repository.CommerceComplaintUpdate{
		Status: status, Resolution: optionalChannelString(resolution), ResolvedAt: resolvedAt,
	})
}

type commerceConversationContext struct {
	StoreID              *uuid.UUID  `json:"store_id,omitempty"`
	CartID               *uuid.UUID  `json:"cart_id,omitempty"`
	VariantID            *uuid.UUID  `json:"variant_id,omitempty"`
	OrderID              *uuid.UUID  `json:"order_id,omitempty"`
	ComplaintOrderID     *uuid.UUID  `json:"complaint_order_id,omitempty"`
	ComplaintStoreID     *uuid.UUID  `json:"complaint_store_id,omitempty"`
	FulfilmentMode       string      `json:"fulfilment_mode,omitempty"`
	DestinationAddress   string      `json:"destination_address,omitempty"`
	DestinationLatitude  *float64    `json:"destination_latitude,omitempty"`
	DestinationLongitude *float64    `json:"destination_longitude,omitempty"`
	OptionKind           string      `json:"option_kind,omitempty"`
	OptionIDs            []uuid.UUID `json:"option_ids,omitempty"`
}

func (s *CommerceChannelService) processInbound(ctx context.Context, configuration *models.CommerceChannelConfiguration, customer *models.CommerceCustomer, conversation *models.CommerceConversation, input CommerceChannelInbound) (string, string, commerceConversationContext, []repository.CommerceChannelReply, error) {
	state := conversation.State
	intent := ""
	if conversation.CurrentIntent != nil {
		intent = *conversation.CurrentIntent
	}
	conversationContext := commerceConversationContext{}
	if len(conversation.Context) > 0 {
		_ = json.Unmarshal(conversation.Context, &conversationContext)
	}
	text := strings.TrimSpace(input.Text)
	selection := strings.TrimSpace(input.SelectionID)
	command := strings.ToLower(text)
	if selection != "" {
		command = strings.ToLower(selection)
	}
	if decision, fulfilmentID, quoteID, ok := parseCommerceQuoteDecision(command); ok {
		if s.fulfilments == nil {
			return state, intent, conversationContext, nil, errors.New("commerce fulfilment service is unavailable")
		}
		if _, err := s.fulfilments.DecideDeliveryQuoteForCustomer(ctx, configuration.OrganizationID, customer.ID, fulfilmentID, quoteID, DecideCommerceDeliveryQuoteInput{
			Decision: decision, Reason: "customer decision received through WhatsApp",
			IdempotencyKey: "wa-quote:" + decision + ":" + quoteID.String(),
		}); err != nil {
			return state, intent, conversationContext, nil, err
		}
		if decision == models.CommerceDeliveryQuoteStatusRejected {
			code, revealErr := s.fulfilments.RevealVerificationCode(ctx, configuration.OrganizationID, customer.ID, fulfilmentID)
			if revealErr != nil {
				return state, intent, conversationContext, nil, revealErr
			}
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Delivery was declined. Your order is now ready for pickup. Your handover code is " + code + ". Share it only when you receive the order."}}, nil
		}
		return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Delivery quote accepted. The store will arrange your rider and keep you updated here."}}, nil
	}
	if decision, fulfilmentID, ok := parseCommerceDeliveryConfirmation(command); ok {
		if s.fulfilments == nil {
			return state, intent, conversationContext, nil, errors.New("commerce fulfilment service is unavailable")
		}
		if _, err := s.fulfilments.DecideDeliveryConfirmationForCustomer(ctx, configuration.OrganizationID, customer.ID, fulfilmentID, decision,
			"wa-delivery-confirmation:"+decision+":"+fulfilmentID.String()); err != nil {
			return state, intent, conversationContext, nil, err
		}
		if decision == models.CommerceDeliveryConfirmationReceived {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Thank you. Your delivery has been confirmed and the order is complete."}}, nil
		}
		return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "We have recorded that your order has not arrived. The store will follow up with the rider."}}, nil
	}

	if isCommerceMenuCommand(command) {
		return models.CommerceConversationStateIntent, "", commerceConversationContext{}, []repository.CommerceChannelReply{s.intentReply(configuration)}, nil
	}
	if state == "" || state == models.CommerceConversationStateWelcome {
		return models.CommerceConversationStateIntent, "", commerceConversationContext{}, []repository.CommerceChannelReply{s.intentReply(configuration)}, nil
	}

	switch state {
	case models.CommerceConversationStateIntent:
		switch parseCommerceIntent(command) {
		case models.CommerceConversationIntentOrder:
			return models.CommerceConversationStateLocation, models.CommerceConversationIntentOrder, commerceConversationContext{}, []repository.CommerceChannelReply{{
				Body:    "Share your WhatsApp location to use the nearest open store, or choose List stores.",
				Options: []repository.CommerceChannelReplyOption{{ID: "stores:list", Title: "List stores"}},
			}}, nil
		case models.CommerceConversationIntentTrackOrder:
			return models.CommerceConversationStateOrderID, models.CommerceConversationIntentTrackOrder, commerceConversationContext{}, []repository.CommerceChannelReply{{Body: "Enter your order number or order ID."}}, nil
		case models.CommerceConversationIntentComplaint:
			return models.CommerceConversationStateComplaintOrder, models.CommerceConversationIntentComplaint, commerceConversationContext{}, []repository.CommerceChannelReply{{Body: "Enter the related order number, or reply Skip if there is no order."}}, nil
		default:
			return state, intent, conversationContext, []repository.CommerceChannelReply{s.intentReply(configuration)}, nil
		}
	case models.CommerceConversationStateLocation:
		stores, err := s.availableStores(ctx, configuration.OrganizationID, input.Latitude, input.Longitude)
		if err != nil {
			return state, intent, conversationContext, nil, err
		}
		if len(stores) == 0 {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "No open store with available products can take an order right now. Please try again later."}}, nil
		}
		if input.Latitude != nil && input.Longitude != nil {
			conversationContext.StoreID = &stores[0].ID
			return s.categoryStep(ctx, configuration.OrganizationID, conversationContext, "We selected "+stores[0].Name+", the nearest available store.")
		}
		if command == "stores:list" || command == "list" || command == "list stores" {
			conversationContext.OptionKind = "store"
			conversationContext.OptionIDs = commerceStoreIDs(stores)
			return models.CommerceConversationStateStore, intent, conversationContext, []repository.CommerceChannelReply{commerceStoreListReply(stores)}, nil
		}
		return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Share a location or reply List stores."}}, nil
	case models.CommerceConversationStateStore:
		storeID, ok := selectCommerceOption(command, "store", conversationContext)
		if !ok {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Choose a store using its number."}}, nil
		}
		stores, err := s.availableStores(ctx, configuration.OrganizationID, nil, nil)
		if err != nil {
			return state, intent, conversationContext, nil, err
		}
		if !commerceStoreIncluded(stores, storeID) {
			return models.CommerceConversationStateLocation, intent, commerceConversationContext{}, []repository.CommerceChannelReply{{Body: "That store is closed or unavailable now. Share a location or choose List stores again."}}, nil
		}
		conversationContext.StoreID = &storeID
		return s.categoryStep(ctx, configuration.OrganizationID, conversationContext, "")
	case models.CommerceConversationStateCategory:
		categoryID, ok := selectCommerceOption(command, "category", conversationContext)
		if !ok || conversationContext.StoreID == nil {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Choose a category using its number."}}, nil
		}
		entries, err := s.availableCatalogue(ctx, configuration.OrganizationID, *conversationContext.StoreID)
		if err != nil {
			return state, intent, conversationContext, nil, err
		}
		products := make([]repository.CommerceStoreCatalogueEntry, 0)
		for _, entry := range entries {
			if entry.CategoryID == categoryID {
				products = append(products, entry)
			}
		}
		if len(products) == 0 {
			return s.categoryStep(ctx, configuration.OrganizationID, conversationContext, "That category is no longer available.")
		}
		conversationContext.OptionKind = "product"
		conversationContext.OptionIDs = commerceVariantIDs(products)
		return models.CommerceConversationStateProduct, intent, conversationContext, commerceProductListReplies(products), nil
	case models.CommerceConversationStateProduct:
		variantID, ok := selectCommerceOption(command, "product", conversationContext)
		if !ok {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Choose a product using its number."}}, nil
		}
		conversationContext.VariantID = &variantID
		return models.CommerceConversationStateQuantity, intent, conversationContext, []repository.CommerceChannelReply{{Body: "How many would you like? Enter a quantity from 1 to 100."}}, nil
	case models.CommerceConversationStateQuantity:
		quantity, err := strconv.Atoi(command)
		if err != nil || quantity < 1 || quantity > commerceCartMaxQuantity || conversationContext.StoreID == nil || conversationContext.VariantID == nil {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Enter a quantity from 1 to 100."}}, nil
		}
		cart, _, err := s.customers.CreateCartForChannel(ctx, configuration.OrganizationID, customer.ID, *conversationContext.StoreID)
		if err != nil {
			return state, intent, conversationContext, nil, err
		}
		cart, err = s.customers.SetCartItemForChannel(ctx, configuration.OrganizationID, customer.ID, cart.Cart.ID, *conversationContext.VariantID, quantity)
		if err != nil {
			if errors.Is(err, ErrCommerceCartItemUnavailable) {
				return s.categoryStep(ctx, configuration.OrganizationID, conversationContext, "That product is no longer available in the requested quantity.")
			}
			return state, intent, conversationContext, nil, err
		}
		conversationContext.CartID = &cart.Cart.ID
		conversationContext.VariantID = nil
		return models.CommerceConversationStateCart, intent, conversationContext, []repository.CommerceChannelReply{commerceCartReply(cart)}, nil
	case models.CommerceConversationStateCart:
		if command == "cart:add" || command == "add" || command == "add more" {
			return s.categoryStep(ctx, configuration.OrganizationID, conversationContext, "")
		}
		if command == "cart:checkout" || command == "checkout" {
			if conversationContext.StoreID == nil || conversationContext.CartID == nil {
				return models.CommerceConversationStateLocation, intent, commerceConversationContext{}, []repository.CommerceChannelReply{{Body: "Your cart is no longer available. Choose a store to start again."}}, nil
			}
			store, err := s.foundationRepo.GetStore(ctx, configuration.OrganizationID, *conversationContext.StoreID, nil)
			if err != nil {
				return state, intent, conversationContext, nil, err
			}
			return models.CommerceConversationStateFulfilment, intent, conversationContext, []repository.CommerceChannelReply{commerceFulfilmentReply(store)}, nil
		}
		return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Choose Add more or Checkout.", Options: commerceCartActions()}}, nil
	case models.CommerceConversationStateFulfilment:
		mode := parseCommerceFulfilment(command)
		if conversationContext.StoreID == nil || !s.storeSupportsMode(ctx, configuration.OrganizationID, *conversationContext.StoreID, mode) {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Choose one of the fulfilment options shown."}}, nil
		}
		conversationContext.FulfilmentMode = mode
		if mode == models.FulfilmentModeMerchantRider {
			store, err := s.foundationRepo.GetStore(ctx, configuration.OrganizationID, *conversationContext.StoreID, nil)
			if err != nil {
				return state, intent, conversationContext, nil, err
			}
			disclaimer := commerceDeliveryDisclaimer(store)
			return models.CommerceConversationStateDeliveryAddress, intent, conversationContext, []repository.CommerceChannelReply{{Body: disclaimer + "\n\nEnter the full delivery address."}}, nil
		}
		return s.collectCustomerDetailsOrCheckout(ctx, configuration, customer, conversation.ID, intent, conversationContext)
	case models.CommerceConversationStateDeliveryAddress:
		if len(text) < 5 || len(text) > 500 {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Enter a full delivery address between 5 and 500 characters."}}, nil
		}
		conversationContext.DestinationAddress = text
		conversationContext.DestinationLatitude = input.Latitude
		conversationContext.DestinationLongitude = input.Longitude
		return s.collectCustomerDetailsOrCheckout(ctx, configuration, customer, conversation.ID, intent, conversationContext)
	case models.CommerceConversationStateCustomerName:
		if len(text) < 2 || len(text) > 200 {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Enter the customer name for this order (2 to 200 characters)."}}, nil
		}
		customer, err := s.customers.UpdateCustomerForChannel(ctx, configuration.OrganizationID, customer.ID, text, "")
		if err != nil {
			return state, intent, conversationContext, nil, err
		}
		return s.collectCustomerDetailsOrCheckout(ctx, configuration, customer, conversation.ID, intent, conversationContext)
	case models.CommerceConversationStatePaymentEmail:
		if !validCommerceChannelEmail(text) {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Enter a valid email address for the payment receipt."}}, nil
		}
		customer, err := s.customers.UpdateCustomerForChannel(ctx, configuration.OrganizationID, customer.ID, "", text)
		if err != nil {
			return state, intent, conversationContext, nil, err
		}
		if conversationContext.OrderID != nil {
			return s.initializeChannelPayment(ctx, configuration.OrganizationID, customer.ID, *conversationContext.OrderID, text, intent, conversationContext)
		}
		return s.checkoutAndPayment(ctx, configuration, customer, conversation.ID, intent, conversationContext)
	case models.CommerceConversationStateOrderID:
		order, err := s.orders.GetOrderForChannel(ctx, configuration.OrganizationID, customer.ID, text)
		if errors.Is(err, repository.ErrCommerceNotFound) {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "We could not find that order for this WhatsApp number. Check the order number and try again."}}, nil
		}
		if err != nil {
			return state, intent, conversationContext, nil, err
		}
		return models.CommerceConversationStateIntent, "", commerceConversationContext{}, []repository.CommerceChannelReply{{
			Body: fmt.Sprintf("Order %s is %s. Total: %s %s.", order.OrderNumber, commerceStatusLabel(order.Status), order.Currency, formatCommerceMinor(order.TotalMinor)),
		}, s.intentReply(configuration)}, nil
	case models.CommerceConversationStateComplaintOrder:
		if command == "skip" || command == "none" {
			return models.CommerceConversationStateComplaintDescription, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Describe the issue in as much detail as possible."}}, nil
		}
		order, err := s.orders.GetOrderForChannel(ctx, configuration.OrganizationID, customer.ID, text)
		if errors.Is(err, repository.ErrCommerceNotFound) {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "We could not find that order. Enter it again, or reply Skip."}}, nil
		}
		if err != nil {
			return state, intent, conversationContext, nil, err
		}
		conversationContext.ComplaintOrderID = &order.ID
		conversationContext.ComplaintStoreID = &order.StoreID
		return models.CommerceConversationStateComplaintDescription, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Describe the issue in as much detail as possible."}}, nil
	case models.CommerceConversationStateComplaintDescription:
		if len(text) < 3 || len(text) > 4000 {
			return state, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Please provide between 3 and 4000 characters."}}, nil
		}
		complaint := &models.CommerceComplaint{
			ID: uuid.New(), OrganizationID: configuration.OrganizationID, CustomerID: customer.ID,
			OrderID: conversationContext.ComplaintOrderID, StoreID: conversationContext.ComplaintStoreID,
			ConversationID: &conversation.ID, Category: "other", Description: text, Status: models.CommerceComplaintStatusOpen,
		}
		if err := s.repo.CreateComplaint(ctx, complaint); err != nil {
			return state, intent, conversationContext, nil, err
		}
		return models.CommerceConversationStateIntent, "", commerceConversationContext{}, []repository.CommerceChannelReply{{Body: "Your complaint has been recorded as " + complaint.ID.String() + ". Our support team can now review it."}, s.intentReply(configuration)}, nil
	default:
		return models.CommerceConversationStateIntent, "", commerceConversationContext{}, []repository.CommerceChannelReply{s.intentReply(configuration)}, nil
	}
}

func (s *CommerceChannelService) categoryStep(ctx context.Context, organizationID uuid.UUID, conversationContext commerceConversationContext, prefix string) (string, string, commerceConversationContext, []repository.CommerceChannelReply, error) {
	if conversationContext.StoreID == nil {
		return models.CommerceConversationStateLocation, models.CommerceConversationIntentOrder, conversationContext, []repository.CommerceChannelReply{{Body: "Choose a store first."}}, nil
	}
	entries, err := s.availableCatalogue(ctx, organizationID, *conversationContext.StoreID)
	if err != nil {
		return "", "", conversationContext, nil, err
	}
	categoryNames := make(map[uuid.UUID]string)
	categoryOrder := make([]uuid.UUID, 0)
	for _, entry := range entries {
		if _, exists := categoryNames[entry.CategoryID]; !exists {
			categoryNames[entry.CategoryID] = entry.CategoryName
			categoryOrder = append(categoryOrder, entry.CategoryID)
		}
	}
	if len(categoryOrder) == 0 {
		return models.CommerceConversationStateLocation, models.CommerceConversationIntentOrder, commerceConversationContext{}, []repository.CommerceChannelReply{{Body: "That store has no available products right now. Choose another store."}}, nil
	}
	conversationContext.OptionKind = "category"
	conversationContext.OptionIDs = categoryOrder
	return models.CommerceConversationStateCategory, models.CommerceConversationIntentOrder, conversationContext, []repository.CommerceChannelReply{commerceCategoryListReply(categoryOrder, categoryNames, prefix)}, nil
}

func (s *CommerceChannelService) checkoutAndPayment(ctx context.Context, configuration *models.CommerceChannelConfiguration, customer *models.CommerceCustomer, conversationID uuid.UUID, intent string, conversationContext commerceConversationContext) (string, string, commerceConversationContext, []repository.CommerceChannelReply, error) {
	if conversationContext.CartID == nil {
		return models.CommerceConversationStateLocation, intent, commerceConversationContext{}, []repository.CommerceChannelReply{{Body: "Your cart is no longer available. Start the order again."}}, nil
	}
	order, _, err := s.orders.CheckoutCartForChannel(ctx, configuration.OrganizationID, customer.ID, CheckoutCommerceCartInput{
		CartID: *conversationContext.CartID, FulfilmentMode: conversationContext.FulfilmentMode,
		IdempotencyKey: "wa-checkout:" + conversationContext.CartID.String(),
	})
	if err != nil {
		return "", "", conversationContext, nil, err
	}
	if conversationContext.FulfilmentMode == models.FulfilmentModeMerchantRider {
		if _, err := s.orders.SetOrderDestinationForChannel(ctx, configuration.OrganizationID, customer.ID, order.ID, conversationContext.DestinationAddress, conversationContext.DestinationLatitude, conversationContext.DestinationLongitude); err != nil {
			return "", "", conversationContext, nil, err
		}
	}
	conversationContext.OrderID = &order.ID
	return s.initializeChannelPayment(ctx, configuration.OrganizationID, customer.ID, order.ID, *customer.Email, intent, conversationContext)
}

func (s *CommerceChannelService) collectCustomerDetailsOrCheckout(ctx context.Context, configuration *models.CommerceChannelConfiguration, customer *models.CommerceCustomer, conversationID uuid.UUID, intent string, conversationContext commerceConversationContext) (string, string, commerceConversationContext, []repository.CommerceChannelReply, error) {
	if strings.TrimSpace(customer.DisplayName) == "" || strings.EqualFold(strings.TrimSpace(customer.DisplayName), normalizeChannelIdentifier(conversationPhone(customer))) {
		return models.CommerceConversationStateCustomerName, intent, conversationContext, []repository.CommerceChannelReply{{Body: "What name should we put on the order?"}}, nil
	}
	if customer.Email == nil || strings.TrimSpace(*customer.Email) == "" {
		return models.CommerceConversationStatePaymentEmail, intent, conversationContext, []repository.CommerceChannelReply{{Body: "Enter an email address for your order confirmation and receipt."}}, nil
	}
	return s.checkoutAndPayment(ctx, configuration, customer, conversationID, intent, conversationContext)
}

func conversationPhone(customer *models.CommerceCustomer) string {
	for _, identity := range customer.Identities {
		if identity.Channel == models.CommerceIdentityChannelWhatsApp || identity.Channel == models.CommerceIdentityChannelPhone {
			return identity.NormalizedIdentifier
		}
	}
	return ""
}

func (s *CommerceChannelService) initializeChannelPayment(ctx context.Context, organizationID, customerID, orderID uuid.UUID, payerEmail, intent string, conversationContext commerceConversationContext) (string, string, commerceConversationContext, []repository.CommerceChannelReply, error) {
	session, _, err := s.payments.InitializePaymentForChannel(ctx, organizationID, customerID, orderID, InitializeCommercePaymentInput{
		PayerEmail: payerEmail, IdempotencyKey: "wa-payment:" + orderID.String(),
	})
	if err != nil {
		return "", "", conversationContext, nil, err
	}
	if session == nil || session.Payment == nil || session.Payment.AuthorizationURL == nil {
		return "", "", conversationContext, nil, fmt.Errorf("%w: payment link is unavailable", ErrCommercePaymentProviderUnavailable)
	}
	return models.CommerceConversationStateIntent, "", commerceConversationContext{}, []repository.CommerceChannelReply{{
		Body: fmt.Sprintf("Invoice %s is ready. Pay securely here: %s", session.Invoice.InvoiceNumber, *session.Payment.AuthorizationURL),
	}}, nil
}

func (s *CommerceChannelService) availableStores(ctx context.Context, organizationID uuid.UUID, latitude, longitude *float64) ([]models.CommerceStore, error) {
	stores, err := s.foundationRepo.ListStores(ctx, organizationID, nil)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	available := make([]models.CommerceStore, 0, len(stores))
	for _, store := range stores {
		if store.Status != models.CommerceStatusActive || !commerceStoreOpenAt(store, now) {
			continue
		}
		if latitude != nil && longitude != nil && (store.Latitude == nil || store.Longitude == nil) {
			continue
		}
		entries, err := s.availableCatalogue(ctx, organizationID, store.ID)
		if err != nil {
			return nil, err
		}
		if len(entries) > 0 {
			available = append(available, store)
		}
	}
	if latitude != nil && longitude != nil {
		sort.SliceStable(available, func(i, j int) bool {
			return commerceStoreDistance(available[i], *latitude, *longitude) < commerceStoreDistance(available[j], *latitude, *longitude)
		})
	}
	return available, nil
}

func (s *CommerceChannelService) availableCatalogue(ctx context.Context, organizationID, storeID uuid.UUID) ([]repository.CommerceStoreCatalogueEntry, error) {
	entries, err := s.catalogueRepo.ListStoreCatalogue(ctx, organizationID, storeID)
	if err != nil {
		return nil, err
	}
	available := make([]repository.CommerceStoreCatalogueEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Enabled && entry.AvailableQuantity > 0 {
			available = append(available, entry)
		}
	}
	return available, nil
}

func (s *CommerceChannelService) storeSupportsMode(ctx context.Context, organizationID, storeID uuid.UUID, mode string) bool {
	if !isCommerceFulfilmentMode(mode) {
		return false
	}
	store, err := s.foundationRepo.GetStore(ctx, organizationID, storeID, nil)
	return err == nil && storeSupportsCommerceFulfilment(store, mode)
}

func (s *CommerceChannelService) intentReply(configuration *models.CommerceChannelConfiguration) repository.CommerceChannelReply {
	message := strings.TrimSpace(configuration.WelcomeMessage)
	if message == "" {
		message = "Welcome. How can we help?"
	}
	return repository.CommerceChannelReply{Body: message, Options: []repository.CommerceChannelReplyOption{
		{ID: "intent:order", Title: "Order"}, {ID: "intent:track", Title: "Track order"}, {ID: "intent:complaint", Title: "Complaint"},
	}}
}

func commerceStoreOpenAt(store models.CommerceStore, at time.Time) bool {
	location, err := time.LoadLocation(store.Timezone)
	if err != nil {
		return false
	}
	local := at.In(location)
	if len(store.Hours) == 0 {
		return true
	}
	minute := local.Hour()*60 + local.Minute()
	for _, hours := range store.Hours {
		if hours.DayOfWeek == int(local.Weekday()) {
			return !hours.IsClosed && hours.OpenMinute != nil && hours.CloseMinute != nil && minute >= *hours.OpenMinute && minute < *hours.CloseMinute
		}
	}
	return false
}

func commerceStoreDistance(store models.CommerceStore, latitude, longitude float64) float64 {
	if store.Latitude == nil || store.Longitude == nil {
		return math.MaxFloat64
	}
	const earthRadius = 6371000.0
	lat1, lat2 := latitude*math.Pi/180, *store.Latitude*math.Pi/180
	deltaLat := (*store.Latitude - latitude) * math.Pi / 180
	deltaLng := (*store.Longitude - longitude) * math.Pi / 180
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(deltaLng/2)*math.Sin(deltaLng/2)
	return earthRadius * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func commerceStoreIDs(stores []models.CommerceStore) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(stores))
	for _, store := range stores {
		ids = append(ids, store.ID)
	}
	return ids
}

func commerceVariantIDs(entries []repository.CommerceStoreCatalogueEntry) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.VariantID)
	}
	return ids
}

func commerceStoreIncluded(stores []models.CommerceStore, storeID uuid.UUID) bool {
	for _, store := range stores {
		if store.ID == storeID {
			return true
		}
	}
	return false
}

func commerceStoreListReply(stores []models.CommerceStore) repository.CommerceChannelReply {
	lines := []string{"Choose an open store:"}
	for index, store := range stores {
		lines = append(lines, fmt.Sprintf("%d. %s - %s", index+1, store.Name, store.Address))
	}
	return repository.CommerceChannelReply{Body: strings.Join(lines, "\n"), Options: commerceUUIDOptions("store", commerceStoreIDs(stores), func(index int) string { return stores[index].Name })}
}

func commerceCategoryListReply(ids []uuid.UUID, names map[uuid.UUID]string, prefix string) repository.CommerceChannelReply {
	lines := []string{}
	if strings.TrimSpace(prefix) != "" {
		lines = append(lines, strings.TrimSpace(prefix))
	}
	lines = append(lines, "Choose a category:")
	for index, id := range ids {
		lines = append(lines, fmt.Sprintf("%d. %s", index+1, names[id]))
	}
	return repository.CommerceChannelReply{Body: strings.Join(lines, "\n"), Options: commerceUUIDOptions("category", ids, func(index int) string { return names[ids[index]] })}
}

func commerceProductListReply(entries []repository.CommerceStoreCatalogueEntry) repository.CommerceChannelReply {
	lines := []string{"Choose a product:"}
	for index, entry := range entries {
		name := entry.ProductName
		if entry.VariantName != "" && !strings.EqualFold(entry.VariantName, "default") {
			name += " - " + entry.VariantName
		}
		lines = append(lines, fmt.Sprintf("%d. %s (%s %s)", index+1, name, entry.ProductCurrency, formatCommerceMinor(entry.EffectivePriceMinor)))
	}
	return repository.CommerceChannelReply{Body: strings.Join(lines, "\n"), Options: commerceUUIDOptions("product", commerceVariantIDs(entries), func(index int) string { return entries[index].ProductName })}
}

func commerceProductListReplies(entries []repository.CommerceStoreCatalogueEntry) []repository.CommerceChannelReply {
	replies := make([]repository.CommerceChannelReply, 0, minCommerceChannelInt(len(entries), 10)+1)
	for index, entry := range entries {
		if index >= 10 || entry.PrimaryImageURL == nil || strings.TrimSpace(*entry.PrimaryImageURL) == "" {
			continue
		}
		name := entry.ProductName
		if entry.VariantName != "" && !strings.EqualFold(entry.VariantName, "default") {
			name += " - " + entry.VariantName
		}
		replies = append(replies, repository.CommerceChannelReply{
			Body:     fmt.Sprintf("%d. %s - %s %s", index+1, name, entry.ProductCurrency, formatCommerceMinor(entry.EffectivePriceMinor)),
			ImageURL: strings.TrimSpace(*entry.PrimaryImageURL),
		})
	}
	return append(replies, commerceProductListReply(entries))
}

func commerceUUIDOptions(kind string, ids []uuid.UUID, title func(int) string) []repository.CommerceChannelReplyOption {
	if len(ids) > 3 {
		return nil
	}
	options := make([]repository.CommerceChannelReplyOption, 0, len(ids))
	for index, id := range ids {
		options = append(options, repository.CommerceChannelReplyOption{ID: kind + ":" + id.String(), Title: truncateChannelTitle(title(index))})
	}
	return options
}

func commerceCartReply(view *CommerceCartView) repository.CommerceChannelReply {
	lines := []string{"Cart:"}
	for _, line := range view.Items {
		name := "Item"
		if line.ProductName != nil {
			name = *line.ProductName
		}
		lines = append(lines, fmt.Sprintf("- %s x%d", name, line.Item.Quantity))
	}
	lines = append(lines, fmt.Sprintf("Total: %s %s", view.Cart.Currency, formatCommerceMinor(view.TotalMinor)))
	return repository.CommerceChannelReply{Body: strings.Join(lines, "\n"), Options: commerceCartActions()}
}

func commerceCartActions() []repository.CommerceChannelReplyOption {
	return []repository.CommerceChannelReplyOption{{ID: "cart:add", Title: "Add more"}, {ID: "cart:checkout", Title: "Checkout"}}
}

func commerceFulfilmentReply(store *models.CommerceStore) repository.CommerceChannelReply {
	options := make([]repository.CommerceChannelReplyOption, 0, 3)
	lines := []string{"Choose how to receive the order."}
	for _, mode := range store.FulfilmentModes {
		if !mode.Enabled {
			continue
		}
		switch mode.Mode {
		case models.FulfilmentModeCustomerPickup:
			options = append(options, repository.CommerceChannelReplyOption{ID: "fulfilment:pickup", Title: "Pickup"})
		case models.FulfilmentModeCustomerRider:
			options = append(options, repository.CommerceChannelReplyOption{ID: "fulfilment:own_rider", Title: "Send my rider"})
		case models.FulfilmentModeMerchantRider:
			options = append(options, repository.CommerceChannelReplyOption{ID: "fulfilment:delivery", Title: "Delivery"})
			if disclaimer := strings.TrimSpace(mode.Disclaimer); disclaimer != "" {
				lines = append(lines, "Delivery: "+disclaimer)
			}
		}
	}
	return repository.CommerceChannelReply{Body: strings.Join(lines, "\n"), Options: options}
}

func commerceDeliveryDisclaimer(store *models.CommerceStore) string {
	for _, mode := range store.FulfilmentModes {
		if mode.Mode == models.FulfilmentModeMerchantRider && mode.Enabled {
			if value := strings.TrimSpace(mode.Disclaimer); value != "" {
				return value
			}
			if mode.CustomerPays {
				return "Delivery is arranged separately. You will pay the delivery fee after accepting the estimate."
			}
		}
	}
	return "Enter the delivery address so the store can arrange delivery."
}

func selectCommerceOption(value, kind string, conversationContext commerceConversationContext) (uuid.UUID, bool) {
	if strings.HasPrefix(value, kind+":") {
		id, err := uuid.Parse(strings.TrimPrefix(value, kind+":"))
		if err == nil {
			for _, allowed := range conversationContext.OptionIDs {
				if allowed == id && conversationContext.OptionKind == kind {
					return id, true
				}
			}
		}
	}
	index, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || index < 1 || index > len(conversationContext.OptionIDs) || conversationContext.OptionKind != kind {
		return uuid.Nil, false
	}
	return conversationContext.OptionIDs[index-1], true
}

func parseCommerceIntent(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "1", "order", "shop", "intent:order":
		return models.CommerceConversationIntentOrder
	case "2", "track", "track order", "intent:track":
		return models.CommerceConversationIntentTrackOrder
	case "3", "complaint", "support", "intent:complaint":
		return models.CommerceConversationIntentComplaint
	default:
		return ""
	}
}

func parseCommerceFulfilment(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "pickup", "fulfilment:pickup":
		return models.FulfilmentModeCustomerPickup
	case "my rider", "own rider", "fulfilment:own_rider":
		return models.FulfilmentModeCustomerRider
	case "delivery", "store delivery", "fulfilment:delivery":
		return models.FulfilmentModeMerchantRider
	default:
		return ""
	}
}

func parseCommerceQuoteDecision(value string) (string, uuid.UUID, uuid.UUID, bool) {
	parts := strings.Split(strings.TrimSpace(strings.ToLower(value)), ":")
	if len(parts) != 4 || parts[0] != "quote" || (parts[1] != models.CommerceDeliveryQuoteStatusAccepted && parts[1] != models.CommerceDeliveryQuoteStatusRejected) {
		return "", uuid.Nil, uuid.Nil, false
	}
	fulfilmentID, fulfilmentErr := uuid.Parse(parts[2])
	quoteID, quoteErr := uuid.Parse(parts[3])
	return parts[1], fulfilmentID, quoteID, fulfilmentErr == nil && quoteErr == nil
}

func parseCommerceDeliveryConfirmation(value string) (string, uuid.UUID, bool) {
	parts := strings.Split(strings.TrimSpace(strings.ToLower(value)), ":")
	if len(parts) != 3 || parts[0] != "delivery" || (parts[1] != models.CommerceDeliveryConfirmationReceived && parts[1] != models.CommerceDeliveryConfirmationNotReceived) {
		return "", uuid.Nil, false
	}
	fulfilmentID, err := uuid.Parse(parts[2])
	return parts[1], fulfilmentID, err == nil
}

func isCommerceMenuCommand(value string) bool {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "menu", "main menu", "cancel", "start over":
		return true
	default:
		return false
	}
}

func isCommerceComplaintStatus(value string) bool {
	switch value {
	case models.CommerceComplaintStatusOpen, models.CommerceComplaintStatusInProgress, models.CommerceComplaintStatusResolved, models.CommerceComplaintStatusClosed:
		return true
	default:
		return false
	}
}

func validCommerceChannelEmail(value string) bool {
	parsed, err := mail.ParseAddress(strings.TrimSpace(value))
	return err == nil && strings.Contains(parsed.Address, "@") && len(parsed.Address) <= 320
}

func normalizeChannelIdentifier(value string) string {
	var builder strings.Builder
	for _, character := range strings.TrimSpace(value) {
		if character >= '0' && character <= '9' {
			builder.WriteRune(character)
		}
	}
	return strings.TrimPrefix(builder.String(), "00")
}

func commerceWhatsAppLinkPhone(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "+") {
		return "", false
	}
	var digits strings.Builder
	for _, character := range strings.TrimPrefix(value, "+") {
		switch {
		case character >= '0' && character <= '9':
			digits.WriteRune(character)
		case character == ' ' || character == '-' || character == '(' || character == ')' || character == '.':
			continue
		default:
			return "", false
		}
	}
	normalized := digits.String()
	return normalized, len(normalized) >= 8 && len(normalized) <= 15 && normalized[0] != '0'
}

func commerceStatusLabel(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "_", " ")
}

func formatCommerceMinor(value int64) string { return fmt.Sprintf("%d.%02d", value/100, value%100) }

func truncateChannelTitle(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 20 {
		return value[:20]
	}
	return value
}

func optionalChannelString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
