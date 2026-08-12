package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/payments"
	"github.com/hidenkeys/zidibackend/repository"
)

var (
	ErrCommerceWebhookUnauthorized        = errors.New("commerce payment webhook signature is invalid")
	ErrCommercePaymentProviderUnavailable = errors.New("commerce payment provider is unavailable")
)

type InitializeCommercePaymentInput struct {
	Provider       string
	PayerEmail     string
	IdempotencyKey string
}

type CommercePaymentWebhookResult struct {
	Outcome   string
	Duplicate bool
}

type CommercePaymentService struct {
	repo            repository.CommercePaymentRepository
	orderRepo       repository.CommerceOrderRepository
	customerRepo    repository.CommerceCustomerRepository
	foundationRepo  repository.CommerceFoundationRepository
	providers       *payments.Registry
	defaultProvider string
	callbackURL     string
	fulfilment      interface {
		PreparePaidOrderForNotification(context.Context, uuid.UUID, uuid.UUID) (*models.CommerceFulfilment, error)
	}
	now func() time.Time
}

func (s *CommercePaymentService) SetFulfilmentService(fulfilment interface {
	PreparePaidOrderForNotification(context.Context, uuid.UUID, uuid.UUID) (*models.CommerceFulfilment, error)
}) {
	s.fulfilment = fulfilment
}

func NewCommercePaymentService(
	repo repository.CommercePaymentRepository,
	orderRepo repository.CommerceOrderRepository,
	customerRepo repository.CommerceCustomerRepository,
	foundationRepo repository.CommerceFoundationRepository,
	providers *payments.Registry,
	defaultProvider string,
	callbackURL string,
) *CommercePaymentService {
	return &CommercePaymentService{
		repo: repo, orderRepo: orderRepo, customerRepo: customerRepo, foundationRepo: foundationRepo,
		providers: providers, defaultProvider: strings.ToLower(strings.TrimSpace(defaultProvider)),
		callbackURL: strings.TrimSpace(callbackURL), now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *CommercePaymentService) InitializePayment(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, orderID uuid.UUID, input InitializeCommercePaymentInput) (*repository.CommercePaymentSession, bool, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, false, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, false, err
	}
	return s.initializePayment(ctx, organizationID, nil, storeScope(actor), orderID, input)
}

func (s *CommercePaymentService) InitializePaymentForChannel(ctx context.Context, organizationID, customerID, orderID uuid.UUID, input InitializeCommercePaymentInput) (*repository.CommercePaymentSession, bool, error) {
	if organizationID == uuid.Nil || customerID == uuid.Nil || orderID == uuid.Nil {
		return nil, false, ErrCommerceForbidden
	}
	return s.initializePayment(ctx, organizationID, &customerID, nil, orderID, input)
}

func (s *CommercePaymentService) initializePayment(ctx context.Context, organizationID uuid.UUID, expectedCustomerID, assignedUserID *uuid.UUID, orderID uuid.UUID, input InitializeCommercePaymentInput) (*repository.CommercePaymentSession, bool, error) {
	providerName := strings.ToLower(strings.TrimSpace(input.Provider))
	if providerName == "" {
		providerName = s.defaultProvider
	}
	provider, ok := s.providers.Get(providerName)
	if !ok {
		return nil, false, fmt.Errorf("%w: unsupported provider", ErrCommerceValidation)
	}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if orderID == uuid.Nil || len(idempotencyKey) < 8 || len(idempotencyKey) > 200 {
		return nil, false, fmt.Errorf("%w: order and an idempotency key between 8 and 200 characters are required", ErrCommerceValidation)
	}

	order, err := s.orderRepo.GetOrder(ctx, organizationID, orderID)
	if err != nil {
		return nil, false, err
	}
	if expectedCustomerID != nil && order.CustomerID != *expectedCustomerID {
		return nil, false, ErrCommerceForbidden
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, order.StoreID, assignedUserID); err != nil {
		return nil, false, err
	}
	customer, err := s.customerRepo.GetCustomer(ctx, organizationID, order.CustomerID)
	if err != nil {
		return nil, false, err
	}
	payerEmail, err := commercePaymentEmail(input.PayerEmail, customer.Email)
	if err != nil {
		return nil, false, err
	}

	now := s.now().UTC()
	paymentID, invoiceID := uuid.New(), uuid.New()
	preparation, err := s.repo.PreparePayment(ctx, repository.CommercePaymentPreparationInput{
		PaymentID: paymentID, InvoiceID: invoiceID, OrganizationID: organizationID, OrderID: orderID,
		InvoiceNumber: commerceInvoiceNumber(now, invoiceID), Provider: providerName,
		ProviderReference: commercePaymentReference(paymentID), IdempotencyKey: idempotencyKey,
		PayerEmail: payerEmail, Now: now,
	})
	if err != nil {
		return nil, false, err
	}
	if preparation.Expired {
		return nil, false, repository.ErrCommercePaymentExpired
	}
	if !preparation.Created {
		if preparation.Session.Payment.Status == models.CommercePaymentStatusInitializing {
			return nil, false, repository.ErrCommercePaymentInitializing
		}
		return preparation.Session, false, nil
	}

	initialization, err := provider.Initialize(ctx, payments.InitializeRequest{
		Reference: preparation.Session.Payment.ProviderReference, Email: payerEmail,
		AmountMinor: preparation.Session.Payment.AmountMinor, Currency: preparation.Session.Payment.Currency,
		CallbackURL: s.callbackURL,
		Metadata: map[string]string{
			"organization_id": organizationID.String(), "order_id": orderID.String(),
			"invoice_id": preparation.Session.Invoice.ID.String(), "payment_id": preparation.Session.Payment.ID.String(),
		},
	})
	if err != nil {
		_ = s.repo.FailInitialization(ctx, organizationID, preparation.Session.Payment.ID, err.Error(), json.RawMessage(`{}`))
		return nil, false, fmt.Errorf("%w: %v", ErrCommercePaymentProviderUnavailable, err)
	}
	if initialization.Reference != preparation.Session.Payment.ProviderReference || strings.TrimSpace(initialization.AuthorizationURL) == "" {
		reason := "provider returned inconsistent payment initialization data"
		_ = s.repo.FailInitialization(ctx, organizationID, preparation.Session.Payment.ID, reason, json.RawMessage(`{}`))
		return nil, false, fmt.Errorf("%w: %s", ErrCommercePaymentProviderUnavailable, reason)
	}
	providerResponse := json.RawMessage(initialization.ProviderResponse)
	if !json.Valid(providerResponse) {
		providerResponse = json.RawMessage(`{}`)
	}
	session, err := s.repo.CompleteInitialization(ctx, repository.CommercePaymentInitializationInput{
		OrganizationID: organizationID, PaymentID: preparation.Session.Payment.ID,
		AuthorizationURL: initialization.AuthorizationURL, AccessCode: initialization.AccessCode,
		ProviderResponse: providerResponse, InitializedAt: s.now().UTC(),
	})
	if err != nil {
		return nil, false, err
	}
	return session, true, nil
}

func (s *CommercePaymentService) GetInvoice(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, orderID uuid.UUID) (*models.CommerceInvoice, error) {
	organizationID, order, err := s.authorizeOrder(ctx, actor, requestedOrganizationID, orderID)
	if err != nil {
		return nil, err
	}
	return s.repo.GetInvoice(ctx, organizationID, order.ID)
}

func (s *CommercePaymentService) GetPayment(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, paymentID uuid.UUID) (*repository.CommercePaymentSession, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if paymentID == uuid.Nil {
		return nil, fmt.Errorf("%w: payment is required", ErrCommerceValidation)
	}
	session, err := s.repo.GetPayment(ctx, organizationID, paymentID)
	if err != nil {
		return nil, err
	}
	order, err := s.orderRepo.GetOrder(ctx, organizationID, session.Payment.OrderID)
	if err != nil {
		return nil, err
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, order.StoreID, storeScope(actor)); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *CommercePaymentService) ExpirePendingPayments(ctx context.Context, limit int) (int, error) {
	return s.repo.ExpirePendingPayments(ctx, s.now().UTC(), limit)
}

func (s *CommercePaymentService) ProcessWebhook(ctx context.Context, providerName string, body []byte, signature string) (*CommercePaymentWebhookResult, error) {
	provider, ok := s.providers.Get(providerName)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported provider", ErrCommerceValidation)
	}
	if !provider.VerifyWebhook(body, signature) {
		return nil, ErrCommerceWebhookUnauthorized
	}
	event, err := provider.ParseWebhook(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCommerceValidation, err)
	}
	payload := json.RawMessage(event.Payload)
	if !json.Valid(payload) {
		return nil, fmt.Errorf("%w: webhook payload is not valid JSON", ErrCommerceValidation)
	}
	receipt, err := s.repo.BeginWebhook(ctx, repository.CommerceWebhookInput{
		ID: uuid.New(), Provider: provider.Name(), EventKey: event.Key, EventType: event.Type,
		ProviderReference: event.Reference, Payload: payload, ReceivedAt: s.now().UTC(),
	})
	if err != nil {
		return nil, err
	}
	if receipt.Unknown || (receipt.Duplicate && receipt.Event.Status != models.CommerceWebhookStatusReceived) {
		return &CommercePaymentWebhookResult{Outcome: receipt.Event.Status, Duplicate: receipt.Duplicate}, nil
	}
	if !isCommercePaymentEvent(event.Type) {
		if err := s.repo.IgnoreWebhook(ctx, receipt.Event.ID, "event type does not change a commerce payment"); err != nil {
			return nil, err
		}
		return &CommercePaymentWebhookResult{Outcome: models.CommerceWebhookStatusIgnored, Duplicate: receipt.Duplicate}, nil
	}

	verification, err := provider.Verify(ctx, event.Reference)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCommercePaymentProviderUnavailable, err)
	}
	providerResponse := json.RawMessage(verification.ProviderResponse)
	if !json.Valid(providerResponse) {
		providerResponse = json.RawMessage(`{}`)
	}
	result, err := s.repo.ApplyVerification(ctx, repository.CommercePaymentVerificationInput{
		WebhookID: receipt.Event.ID, Provider: provider.Name(), Reference: verification.Reference,
		ProviderTransactionID: verification.ProviderTransactionID, Status: verification.Status,
		AmountMinor: verification.AmountMinor, Currency: verification.Currency,
		PaidAt: verification.PaidAt, ProviderResponse: providerResponse,
	})
	if err != nil {
		return nil, err
	}
	if result.Outcome == models.CommercePaymentStatusSucceeded && result.Session != nil && s.fulfilment != nil {
		if _, err := s.fulfilment.PreparePaidOrderForNotification(ctx, result.Session.Payment.OrganizationID, result.Session.Payment.OrderID); err != nil {
			return nil, fmt.Errorf("prepare paid order fulfilment: %w", err)
		}
	}
	return &CommercePaymentWebhookResult{Outcome: result.Outcome, Duplicate: receipt.Duplicate}, nil
}

func (s *CommercePaymentService) WebhookSignatureHeader(providerName string) (string, bool) {
	provider, ok := s.providers.Get(providerName)
	if !ok {
		return "", false
	}
	return provider.SignatureHeader(), true
}

func (s *CommercePaymentService) authorizeOrder(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, orderID uuid.UUID) (uuid.UUID, *models.CommerceOrder, error) {
	if !canAccessCommerce(actor.Role) {
		return uuid.Nil, nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if orderID == uuid.Nil {
		return uuid.Nil, nil, fmt.Errorf("%w: order is required", ErrCommerceValidation)
	}
	order, err := s.orderRepo.GetOrder(ctx, organizationID, orderID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, order.StoreID, storeScope(actor)); err != nil {
		return uuid.Nil, nil, err
	}
	return organizationID, order, nil
}

func commercePaymentEmail(input string, customerEmail *string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" && customerEmail != nil {
		value = strings.TrimSpace(*customerEmail)
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || !strings.Contains(parsed.Address, "@") || len(parsed.Address) > 320 {
		return "", fmt.Errorf("%w: a valid payer email is required", ErrCommerceValidation)
	}
	return strings.ToLower(parsed.Address), nil
}

func isCommercePaymentEvent(eventType string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "charge.success", "charge.failed":
		return true
	default:
		return false
	}
}

func commercePaymentReference(paymentID uuid.UUID) string {
	return "ZCP-" + strings.ToUpper(paymentID.String())
}

func commerceInvoiceNumber(now time.Time, invoiceID uuid.UUID) string {
	return "INV-" + now.UTC().Format("20060102") + "-" + strings.ToUpper(invoiceID.String()[:8])
}
