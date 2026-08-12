package services

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/payments"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/utils"
)

type commercePaymentRepoStub struct {
	mu                sync.Mutex
	sessions          map[uuid.UUID]*repository.CommercePaymentSession
	paymentByKey      map[string]uuid.UUID
	prepareCalls      int
	beginWebhookCalls int
	ignoreCalls       int
	applyInput        repository.CommercePaymentVerificationInput
	webhookReceipt    *repository.CommerceWebhookReceipt
}

func (s *commercePaymentRepoStub) PreparePayment(_ context.Context, input repository.CommercePaymentPreparationInput) (*repository.CommercePaymentPreparation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initialize()
	s.prepareCalls++
	if existingID, ok := s.paymentByKey[input.OrganizationID.String()+":"+input.IdempotencyKey]; ok {
		session := s.sessions[existingID]
		if session != nil && session.Payment.Status == models.CommercePaymentStatusFailed && session.Payment.AuthorizationURL == nil {
			session.Payment.Status = models.CommercePaymentStatusInitializing
			session.Payment.FailureReason = ""
			return &repository.CommercePaymentPreparation{Session: cloneCommercePaymentSession(session), Created: true}, nil
		}
		return &repository.CommercePaymentPreparation{Session: cloneCommercePaymentSession(session)}, nil
	}
	invoice := &models.CommerceInvoice{
		ID: input.InvoiceID, OrganizationID: input.OrganizationID, OrderID: input.OrderID,
		InvoiceNumber: input.InvoiceNumber, Status: models.CommerceInvoiceStatusIssued,
		Currency: "NGN", TotalMinor: 420000, IssuedAt: input.Now,
	}
	payment := &models.CommercePaymentTransaction{
		ID: input.PaymentID, OrganizationID: input.OrganizationID, OrderID: input.OrderID,
		InvoiceID: input.InvoiceID, Provider: input.Provider, ProviderReference: input.ProviderReference,
		IdempotencyKey: input.IdempotencyKey, PayerEmail: input.PayerEmail,
		Status: models.CommercePaymentStatusInitializing, Currency: "NGN", AmountMinor: 420000,
		ExpiresAt: input.Now.Add(30 * time.Minute), ProviderResponse: json.RawMessage(`{}`),
	}
	session := &repository.CommercePaymentSession{Invoice: invoice, Payment: payment}
	s.sessions[payment.ID] = session
	s.paymentByKey[input.OrganizationID.String()+":"+input.IdempotencyKey] = payment.ID
	return &repository.CommercePaymentPreparation{Session: cloneCommercePaymentSession(session), Created: true}, nil
}

func (s *commercePaymentRepoStub) CompleteInitialization(_ context.Context, input repository.CommercePaymentInitializationInput) (*repository.CommercePaymentSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[input.PaymentID]
	if session == nil || session.Payment.OrganizationID != input.OrganizationID {
		return nil, repository.ErrCommerceNotFound
	}
	session.Payment.Status = models.CommercePaymentStatusPending
	session.Payment.AuthorizationURL = &input.AuthorizationURL
	session.Payment.ProviderResponse = input.ProviderResponse
	initializedAt := input.InitializedAt
	session.Payment.InitializedAt = &initializedAt
	return cloneCommercePaymentSession(session), nil
}

func (s *commercePaymentRepoStub) FailInitialization(_ context.Context, organizationID, paymentID uuid.UUID, reason string, _ json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session := s.sessions[paymentID]; session != nil && session.Payment.OrganizationID == organizationID {
		session.Payment.Status = models.CommercePaymentStatusFailed
		session.Payment.FailureReason = reason
	}
	return nil
}

func (s *commercePaymentRepoStub) GetInvoice(_ context.Context, organizationID, orderID uuid.UUID) (*models.CommerceInvoice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, session := range s.sessions {
		if session.Invoice.OrganizationID == organizationID && session.Invoice.OrderID == orderID {
			invoice := *session.Invoice
			return &invoice, nil
		}
	}
	return nil, repository.ErrCommerceNotFound
}

func (s *commercePaymentRepoStub) GetPayment(_ context.Context, organizationID, paymentID uuid.UUID) (*repository.CommercePaymentSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[paymentID]
	if session == nil || session.Payment.OrganizationID != organizationID {
		return nil, repository.ErrCommerceNotFound
	}
	return cloneCommercePaymentSession(session), nil
}

func (s *commercePaymentRepoStub) ExpirePendingPayments(context.Context, time.Time, int) (int, error) {
	return 0, nil
}

func (s *commercePaymentRepoStub) BeginWebhook(_ context.Context, input repository.CommerceWebhookInput) (*repository.CommerceWebhookReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beginWebhookCalls++
	if s.webhookReceipt != nil {
		return s.webhookReceipt, nil
	}
	return &repository.CommerceWebhookReceipt{Event: &models.CommercePaymentWebhookEvent{
		ID: input.ID, Provider: input.Provider, EventKey: input.EventKey,
		EventType: input.EventType, ProviderReference: input.ProviderReference,
		Status: models.CommerceWebhookStatusReceived,
	}}, nil
}

func (s *commercePaymentRepoStub) IgnoreWebhook(context.Context, uuid.UUID, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ignoreCalls++
	return nil
}

func (s *commercePaymentRepoStub) ApplyVerification(_ context.Context, input repository.CommercePaymentVerificationInput) (*repository.CommercePaymentVerificationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyInput = input
	return &repository.CommercePaymentVerificationResult{Outcome: models.CommercePaymentStatusSucceeded}, nil
}

func (s *commercePaymentRepoStub) initialize() {
	if s.sessions == nil {
		s.sessions = make(map[uuid.UUID]*repository.CommercePaymentSession)
	}
	if s.paymentByKey == nil {
		s.paymentByKey = make(map[string]uuid.UUID)
	}
}

type commercePaymentProviderStub struct {
	mu                sync.Mutex
	validSignature    bool
	initializeCalls   int
	initializeErr     error
	verifyCalls       int
	initializeRequest payments.InitializeRequest
	event             *payments.WebhookEvent
	verification      *payments.Verification
}

func (s *commercePaymentProviderStub) Name() string            { return payments.PaystackProviderName }
func (s *commercePaymentProviderStub) SignatureHeader() string { return "x-paystack-signature" }
func (s *commercePaymentProviderStub) VerifyWebhook([]byte, string) bool {
	return s.validSignature
}
func (s *commercePaymentProviderStub) Initialize(_ context.Context, request payments.InitializeRequest) (*payments.Initialization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initializeCalls++
	s.initializeRequest = request
	if s.initializeErr != nil {
		return nil, s.initializeErr
	}
	return &payments.Initialization{
		Reference: request.Reference, AuthorizationURL: "https://checkout.paystack.test/session",
		AccessCode: "access", ProviderResponse: []byte(`{"reference":"ok"}`),
	}, nil
}
func (s *commercePaymentProviderStub) ParseWebhook(body []byte) (*payments.WebhookEvent, error) {
	if s.event != nil {
		return s.event, nil
	}
	return &payments.WebhookEvent{
		Key: "charge.success:123", Type: "charge.success", Reference: "ZCP-payment",
		ProviderTransactionID: "123", Payload: append([]byte(nil), body...),
	}, nil
}
func (s *commercePaymentProviderStub) Verify(context.Context, string) (*payments.Verification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.verifyCalls++
	if s.verification != nil {
		return s.verification, nil
	}
	return &payments.Verification{
		Reference: "ZCP-payment", ProviderTransactionID: "123", Status: "success",
		AmountMinor: 420000, Currency: "NGN", ProviderResponse: []byte(`{"status":"success"}`),
	}, nil
}

func TestCommercePaymentInitializationUsesAuthoritativeOrderAmountAndIsIdempotent(t *testing.T) {
	organizationID, orderID, customerID, storeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	orderRepo := seededCommerceOrderRepo(&models.CommerceOrder{
		ID: orderID, OrganizationID: organizationID, CustomerID: customerID, StoreID: storeID,
		Status: models.CommerceOrderStatusPendingPayment, Currency: "NGN", TotalMinor: 420000,
		PaymentExpiresAt: time.Now().Add(30 * time.Minute),
	})
	email := "customer@example.com"
	customerRepo := seededCommerceCustomerRepo(organizationID, customerID)
	customerRepo.customers[customerID].Email = &email
	paymentRepo := &commercePaymentRepoStub{}
	provider := &commercePaymentProviderStub{}
	service := NewCommercePaymentService(paymentRepo, orderRepo, customerRepo, &commerceFoundationRepoStub{}, payments.NewRegistry(provider), "paystack", "https://shop.example.com/payment-return")
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}

	first, created, err := service.InitializePayment(context.Background(), actor, nil, orderID, InitializeCommercePaymentInput{IdempotencyKey: "payment-request-001"})
	if err != nil || !created {
		t.Fatalf("initialize payment: created=%v err=%v", created, err)
	}
	second, createdAgain, err := service.InitializePayment(context.Background(), actor, nil, orderID, InitializeCommercePaymentInput{IdempotencyKey: "payment-request-001"})
	if err != nil || createdAgain {
		t.Fatalf("idempotent retry: created=%v err=%v", createdAgain, err)
	}
	if first.Payment.ID != second.Payment.ID || provider.initializeCalls != 1 {
		t.Fatalf("retry duplicated provider initialization: first=%s second=%s calls=%d", first.Payment.ID, second.Payment.ID, provider.initializeCalls)
	}
	if provider.initializeRequest.AmountMinor != 420000 || provider.initializeRequest.Currency != "NGN" || provider.initializeRequest.Email != email {
		t.Fatalf("provider did not receive authoritative payment data: %+v", provider.initializeRequest)
	}
	if first.Payment.Status != models.CommercePaymentStatusPending || first.Payment.AuthorizationURL == nil {
		t.Fatalf("payment was not finalized locally: %+v", first.Payment)
	}
}

func TestCommercePaymentInitializationRetriesFailedSessionWithoutLink(t *testing.T) {
	organizationID, orderID, customerID, storeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	orderRepo := seededCommerceOrderRepo(&models.CommerceOrder{
		ID: orderID, OrganizationID: organizationID, CustomerID: customerID, StoreID: storeID,
		Status: models.CommerceOrderStatusPendingPayment, Currency: "NGN", TotalMinor: 420000,
		PaymentExpiresAt: time.Now().Add(30 * time.Minute),
	})
	email := "customer@example.com"
	customerRepo := seededCommerceCustomerRepo(organizationID, customerID)
	customerRepo.customers[customerID].Email = &email
	paymentRepo := &commercePaymentRepoStub{}
	provider := &commercePaymentProviderStub{initializeErr: errors.New("temporary provider failure")}
	service := NewCommercePaymentService(paymentRepo, orderRepo, customerRepo, &commerceFoundationRepoStub{}, payments.NewRegistry(provider), "paystack", "")
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}

	first, _, err := service.InitializePayment(context.Background(), actor, nil, orderID, InitializeCommercePaymentInput{IdempotencyKey: "payment-retry-001"})
	if !errors.Is(err, ErrCommercePaymentProviderUnavailable) || first != nil {
		t.Fatalf("expected initial provider failure, session=%v err=%v", first, err)
	}
	provider.initializeErr = nil

	second, created, err := service.InitializePayment(context.Background(), actor, nil, orderID, InitializeCommercePaymentInput{IdempotencyKey: "payment-retry-001"})
	if err != nil || !created {
		t.Fatalf("retry failed payment: created=%v err=%v", created, err)
	}
	if second.Payment.AuthorizationURL == nil || provider.initializeCalls != 2 {
		t.Fatalf("failed initialization was not retried: second=%s link=%v calls=%d", second.Payment.ID, second.Payment.AuthorizationURL, provider.initializeCalls)
	}
}

func TestCommercePaymentInitializationRejectsCrossTenantOrder(t *testing.T) {
	organizationID := uuid.New()
	service := NewCommercePaymentService(&commercePaymentRepoStub{}, &commerceOrderRepoStub{}, &commerceCustomerRepoStub{}, &commerceFoundationRepoStub{}, payments.NewRegistry(&commercePaymentProviderStub{}), "paystack", "")
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}
	otherOrganizationID := uuid.New()

	_, _, err := service.InitializePayment(context.Background(), actor, &otherOrganizationID, uuid.New(), InitializeCommercePaymentInput{IdempotencyKey: "payment-request-002", PayerEmail: "customer@example.com"})
	if !errors.Is(err, ErrCommerceForbidden) {
		t.Fatalf("expected cross-tenant payment to be forbidden, got %v", err)
	}
}

func TestCommerceWebhookRejectsInvalidSignatureBeforePersistence(t *testing.T) {
	repo := &commercePaymentRepoStub{}
	provider := &commercePaymentProviderStub{validSignature: false}
	service := NewCommercePaymentService(repo, nil, nil, nil, payments.NewRegistry(provider), "paystack", "")

	_, err := service.ProcessWebhook(context.Background(), "paystack", []byte(`{"event":"charge.success"}`), "invalid")
	if !errors.Is(err, ErrCommerceWebhookUnauthorized) {
		t.Fatalf("expected unauthorized webhook, got %v", err)
	}
	if repo.beginWebhookCalls != 0 || provider.verifyCalls != 0 {
		t.Fatal("invalid webhook reached persistence or provider verification")
	}
}

func TestCommerceWebhookUsesServerVerificationNotPayloadMoney(t *testing.T) {
	repo := &commercePaymentRepoStub{}
	provider := &commercePaymentProviderStub{
		validSignature: true,
		event: &payments.WebhookEvent{
			Key: "charge.success:456", Type: "charge.success", Reference: "ZCP-payment",
			ProviderTransactionID: "456", Payload: []byte(`{"event":"charge.success","data":{"reference":"ZCP-payment","amount":1}}`),
		},
		verification: &payments.Verification{
			Reference: "ZCP-payment", ProviderTransactionID: "456", Status: "success",
			AmountMinor: 420000, Currency: "NGN", ProviderResponse: []byte(`{"status":"success"}`),
		},
	}
	service := NewCommercePaymentService(repo, nil, nil, nil, payments.NewRegistry(provider), "paystack", "")
	body := []byte(`{"event":"charge.success","data":{"reference":"ZCP-payment","amount":1}}`)

	result, err := service.ProcessWebhook(context.Background(), "paystack", body, "valid")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != models.CommercePaymentStatusSucceeded || repo.applyInput.AmountMinor != 420000 || provider.verifyCalls != 1 {
		t.Fatalf("webhook payload was trusted over verification: result=%+v applied=%+v", result, repo.applyInput)
	}
}

func TestCommerceWebhookDuplicateProcessedEventSkipsProviderVerification(t *testing.T) {
	eventID := uuid.New()
	repo := &commercePaymentRepoStub{webhookReceipt: &repository.CommerceWebhookReceipt{
		Event:     &models.CommercePaymentWebhookEvent{ID: eventID, Status: models.CommerceWebhookStatusProcessed},
		Duplicate: true,
	}}
	provider := &commercePaymentProviderStub{validSignature: true}
	service := NewCommercePaymentService(repo, nil, nil, nil, payments.NewRegistry(provider), "paystack", "")

	result, err := service.ProcessWebhook(context.Background(), "paystack", []byte(`{"event":"charge.success"}`), "valid")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Duplicate || result.Outcome != models.CommerceWebhookStatusProcessed || provider.verifyCalls != 0 {
		t.Fatalf("processed duplicate was not short-circuited: result=%+v calls=%d", result, provider.verifyCalls)
	}
}

func cloneCommercePaymentSession(session *repository.CommercePaymentSession) *repository.CommercePaymentSession {
	invoice, payment := *session.Invoice, *session.Payment
	invoice.Items = append([]models.CommerceInvoiceItem(nil), session.Invoice.Items...)
	return &repository.CommercePaymentSession{Invoice: &invoice, Payment: &payment}
}
