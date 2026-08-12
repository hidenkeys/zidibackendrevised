package services

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	commercefulfilment "github.com/hidenkeys/zidibackend/fulfilment"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/utils"
)

type commerceFulfilmentRepoStub struct {
	item          *models.CommerceFulfilment
	startInput    repository.CommerceStartFulfilmentInput
	quoteInput    repository.CommerceCreateDeliveryQuoteInput
	handoverInput repository.CommerceVerifyHandoverInput
	arrivalInput  repository.CommerceFulfilmentTransitionInput
	decisionInput repository.CommerceDeliveryQuoteDecisionInput
	reminderInput repository.CommerceFulfilmentTransitionInput
	operationErr  error
}

func (s *commerceFulfilmentRepoStub) StartFulfilment(_ context.Context, input repository.CommerceStartFulfilmentInput) (*models.CommerceFulfilment, bool, error) {
	s.startInput = input
	s.item = cloneCommerceFulfilment(&input.Fulfilment)
	return cloneCommerceFulfilment(s.item), true, s.operationErr
}

func (s *commerceFulfilmentRepoStub) GetFulfilmentByOrder(_ context.Context, organizationID, orderID uuid.UUID) (*models.CommerceFulfilment, error) {
	if s.item == nil || s.item.OrganizationID != organizationID || s.item.OrderID != orderID {
		return nil, repository.ErrCommerceNotFound
	}
	return cloneCommerceFulfilment(s.item), nil
}

func (s *commerceFulfilmentRepoStub) ListFulfilmentsByOrderIDs(_ context.Context, organizationID uuid.UUID, orderIDs []uuid.UUID) ([]models.CommerceFulfilment, error) {
	if s.item == nil || s.item.OrganizationID != organizationID {
		return []models.CommerceFulfilment{}, nil
	}
	for _, orderID := range orderIDs {
		if orderID == s.item.OrderID {
			return []models.CommerceFulfilment{*cloneCommerceFulfilment(s.item)}, nil
		}
	}
	return []models.CommerceFulfilment{}, nil
}

func (s *commerceFulfilmentRepoStub) GetFulfilment(_ context.Context, organizationID, fulfilmentID uuid.UUID) (*models.CommerceFulfilment, error) {
	if s.item == nil || s.item.OrganizationID != organizationID || s.item.ID != fulfilmentID {
		return nil, repository.ErrCommerceNotFound
	}
	return cloneCommerceFulfilment(s.item), nil
}

func (s *commerceFulfilmentRepoStub) CreateDeliveryQuote(_ context.Context, input repository.CommerceCreateDeliveryQuoteInput) (*models.CommerceFulfilment, error) {
	s.quoteInput = input
	return cloneCommerceFulfilment(s.item), s.operationErr
}

func (s *commerceFulfilmentRepoStub) DecideDeliveryQuote(_ context.Context, input repository.CommerceDeliveryQuoteDecisionInput) (*models.CommerceFulfilment, error) {
	s.decisionInput = input
	return cloneCommerceFulfilment(s.item), s.operationErr
}

func (s *commerceFulfilmentRepoStub) AssignRider(context.Context, repository.CommerceAssignRiderInput) (*models.CommerceFulfilment, error) {
	return cloneCommerceFulfilment(s.item), s.operationErr
}

func (s *commerceFulfilmentRepoStub) QueueHandoverCodeReminder(_ context.Context, input repository.CommerceFulfilmentTransitionInput) (*models.CommerceFulfilment, error) {
	s.reminderInput = input
	return cloneCommerceFulfilment(s.item), s.operationErr
}

func (s *commerceFulfilmentRepoStub) RecordArrival(_ context.Context, input repository.CommerceFulfilmentTransitionInput) (*models.CommerceFulfilment, error) {
	s.arrivalInput = input
	return cloneCommerceFulfilment(s.item), s.operationErr
}

func (s *commerceFulfilmentRepoStub) VerifyHandover(_ context.Context, input repository.CommerceVerifyHandoverInput) (*models.CommerceFulfilment, error) {
	s.handoverInput = input
	return cloneCommerceFulfilment(s.item), s.operationErr
}

func (s *commerceFulfilmentRepoStub) MarkDelivered(context.Context, repository.CommerceFulfilmentTransitionInput) (*models.CommerceFulfilment, error) {
	return cloneCommerceFulfilment(s.item), s.operationErr
}

func (s *commerceFulfilmentRepoStub) CompleteFulfilment(context.Context, repository.CommerceFulfilmentTransitionInput) (*models.CommerceFulfilment, error) {
	return cloneCommerceFulfilment(s.item), s.operationErr
}

type deliveryQuoteProviderStub struct {
	request commercefulfilment.DeliveryQuoteRequest
	quote   *commercefulfilment.DeliveryQuote
	err     error
}

type deniedCommerceFoundationRepoStub struct {
	commerceFoundationRepoStub
}

func (s *deniedCommerceFoundationRepoStub) GetStore(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) (*models.CommerceStore, error) {
	return nil, repository.ErrCommerceNotFound
}

func (s *deliveryQuoteProviderStub) Name() string { return "test-provider" }

func (s *deliveryQuoteProviderStub) Quote(_ context.Context, request commercefulfilment.DeliveryQuoteRequest) (*commercefulfilment.DeliveryQuote, error) {
	s.request = request
	return s.quote, s.err
}

func TestStartCommercePickupFulfilmentCreatesProtectedCode(t *testing.T) {
	organizationID, orderID, storeID, customerID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	orderRepo := seededCommerceOrderRepo(&models.CommerceOrder{
		ID: orderID, OrganizationID: organizationID, StoreID: storeID, CustomerID: customerID,
		Status: models.CommerceOrderStatusReady, FulfilmentMode: models.FulfilmentModeCustomerPickup,
	})
	repo := &commerceFulfilmentRepoStub{}
	manager := testCommerceCodeManager(t)
	service := NewCommerceFulfilmentService(repo, orderRepo, &commerceFoundationRepoStub{}, commercefulfilment.NewRegistry(), manager)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}

	item, created, err := service.StartFulfilment(context.Background(), actor, nil, orderID, StartCommerceFulfilmentInput{IdempotencyKey: "fulfilment-start-001"})
	if err != nil || !created {
		t.Fatalf("start pickup fulfilment: created=%v err=%v", created, err)
	}
	if item.Status != models.CommerceFulfilmentStatusReadyForPickup || repo.startInput.OrderStatus != models.CommerceOrderStatusReadyForPickup {
		t.Fatalf("pickup started in the wrong state: item=%+v input=%+v", item, repo.startInput)
	}
	if len(item.VerificationCodeHash) == 0 || len(item.VerificationCodeCiphertext) == 0 || !item.VerificationCodeExpiresAt.After(time.Now().Add(47*time.Hour)) {
		t.Fatal("pickup did not receive a protected, expiring verification code")
	}
	code, err := service.RevealVerificationCode(context.Background(), organizationID, customerID, item.ID)
	if err != nil || !commerceVerificationCodePattern.MatchString(code) || !manager.Verify(code, item.VerificationCodeHash) {
		t.Fatalf("customer code reveal failed: code=%q err=%v", code, err)
	}
}

func TestMerchantRiderFulfilmentRequiresDestination(t *testing.T) {
	organizationID, orderID := uuid.New(), uuid.New()
	orderRepo := seededCommerceOrderRepo(&models.CommerceOrder{
		ID: orderID, OrganizationID: organizationID, StoreID: uuid.New(), CustomerID: uuid.New(),
		Status: models.CommerceOrderStatusReady, FulfilmentMode: models.FulfilmentModeMerchantRider,
	})
	service := NewCommerceFulfilmentService(&commerceFulfilmentRepoStub{}, orderRepo, &commerceFoundationRepoStub{}, commercefulfilment.NewRegistry(), testCommerceCodeManager(t))
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}

	_, _, err := service.StartFulfilment(context.Background(), actor, nil, orderID, StartCommerceFulfilmentInput{IdempotencyKey: "fulfilment-start-002"})
	if !errors.Is(err, ErrCommerceValidation) {
		t.Fatalf("expected destination validation error, got %v", err)
	}
}

func TestManualDeliveryQuoteUsesDirectToRiderFeeMode(t *testing.T) {
	organizationID, orderID, fulfilmentID := uuid.New(), uuid.New(), uuid.New()
	destination := "15 Admiralty Way, Lagos"
	repo := &commerceFulfilmentRepoStub{item: &models.CommerceFulfilment{
		ID: fulfilmentID, OrganizationID: organizationID, OrderID: orderID, StoreID: uuid.New(),
		Mode: models.FulfilmentModeMerchantRider, Status: models.CommerceFulfilmentStatusAwaitingQuote,
		PickupAddress: "Store, Lagos", DestinationAddress: &destination,
	}}
	orderRepo := seededCommerceOrderRepo(&models.CommerceOrder{ID: orderID, OrganizationID: organizationID, StoreID: repo.item.StoreID, Currency: "NGN"})
	service := NewCommerceFulfilmentService(repo, orderRepo, &commerceFoundationRepoStub{}, commercefulfilment.NewRegistry(), testCommerceCodeManager(t))
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}
	fee := int64(250000)

	_, err := service.CreateDeliveryQuote(context.Background(), actor, nil, fulfilmentID, CreateCommerceDeliveryQuoteInput{
		Source: models.CommerceDeliveryQuoteSourceManual, EstimatedFeeMinor: &fee, IdempotencyKey: "delivery-quote-001",
	})
	if err != nil {
		t.Fatal(err)
	}
	quote := repo.quoteInput.Quote
	if quote.EstimatedFeeMinor != fee || quote.Currency != "NGN" || quote.FeePaymentMode != models.CommerceDeliveryFeePaymentDirectToRider || quote.FeeStatus != models.CommerceDeliveryFeeStatusNotCollected {
		t.Fatalf("manual quote does not preserve the V1 fee contract: %+v", quote)
	}
}

func TestProviderQuoteUsesRegisteredProviderWithoutFabricatedFallback(t *testing.T) {
	organizationID, orderID, fulfilmentID := uuid.New(), uuid.New(), uuid.New()
	destination := "Victoria Island, Lagos"
	repo := &commerceFulfilmentRepoStub{item: &models.CommerceFulfilment{
		ID: fulfilmentID, OrganizationID: organizationID, OrderID: orderID, StoreID: uuid.New(),
		Mode: models.FulfilmentModeMerchantRider, Status: models.CommerceFulfilmentStatusAwaitingQuote,
		PickupAddress: "Lekki, Lagos", DestinationAddress: &destination,
	}}
	orderRepo := seededCommerceOrderRepo(&models.CommerceOrder{ID: orderID, OrganizationID: organizationID, StoreID: repo.item.StoreID, Currency: "NGN"})
	provider := &deliveryQuoteProviderStub{quote: &commercefulfilment.DeliveryQuote{ProviderQuoteID: "quote-123", EstimatedFeeMinor: 180000, Currency: "NGN"}}
	service := NewCommerceFulfilmentService(repo, orderRepo, &commerceFoundationRepoStub{}, commercefulfilment.NewRegistry(provider), testCommerceCodeManager(t))
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}

	_, err := service.CreateDeliveryQuote(context.Background(), actor, nil, fulfilmentID, CreateCommerceDeliveryQuoteInput{
		Source: models.CommerceDeliveryQuoteSourceProvider, Provider: provider.Name(), IdempotencyKey: "delivery-quote-002",
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.request.Destination.Address != destination || repo.quoteInput.Quote.EstimatedFeeMinor != 180000 || repo.quoteInput.Quote.Provider == nil {
		t.Fatalf("registered provider quote was not preserved: request=%+v quote=%+v", provider.request, repo.quoteInput.Quote)
	}

	_, err = service.CreateDeliveryQuote(context.Background(), actor, nil, fulfilmentID, CreateCommerceDeliveryQuoteInput{
		Source: models.CommerceDeliveryQuoteSourceProvider, Provider: "not-configured", IdempotencyKey: "delivery-quote-003",
	})
	if !errors.Is(err, ErrCommerceDeliveryProviderUnavailable) {
		t.Fatalf("expected unavailable provider error, got %v", err)
	}
}

func TestCustomerQuoteDecisionRequiresMatchingCustomerIdentity(t *testing.T) {
	organizationID, customerID, fulfilmentID, quoteID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := &commerceFulfilmentRepoStub{item: &models.CommerceFulfilment{
		ID: fulfilmentID, OrganizationID: organizationID, CustomerID: customerID,
		Mode: models.FulfilmentModeMerchantRider, Status: models.CommerceFulfilmentStatusAwaitingCustomerConfirmation,
	}}
	service := NewCommerceFulfilmentService(repo, &commerceOrderRepoStub{}, &commerceFoundationRepoStub{}, commercefulfilment.NewRegistry(), testCommerceCodeManager(t))
	input := DecideCommerceDeliveryQuoteInput{Decision: models.CommerceDeliveryQuoteStatusAccepted, IdempotencyKey: "customer-quote-decision-001"}

	if _, err := service.DecideDeliveryQuoteForCustomer(context.Background(), organizationID, uuid.New(), fulfilmentID, quoteID, input); !errors.Is(err, ErrCommerceForbidden) {
		t.Fatalf("expected mismatched customer to be denied, got %v", err)
	}
	if _, err := service.DecideDeliveryQuoteForCustomer(context.Background(), organizationID, customerID, fulfilmentID, quoteID, input); err != nil {
		t.Fatal(err)
	}
	if repo.decisionInput.ActorType != models.CommerceFulfilmentActorCustomer || repo.decisionInput.ActorUserID != nil {
		t.Fatalf("customer decision was not attributed to the customer channel: %+v", repo.decisionInput)
	}
}

func TestHandoverHashesCodeAndFulfilmentStatesCannotUseGenericOrderEndpoint(t *testing.T) {
	organizationID, orderID, fulfilmentID := uuid.New(), uuid.New(), uuid.New()
	manager := testCommerceCodeManager(t)
	repo := &commerceFulfilmentRepoStub{item: &models.CommerceFulfilment{
		ID: fulfilmentID, OrganizationID: organizationID, OrderID: orderID, StoreID: uuid.New(),
		Mode: models.FulfilmentModeCustomerPickup, Status: models.CommerceFulfilmentStatusReadyForPickup,
	}}
	orderRepo := seededCommerceOrderRepo(&models.CommerceOrder{ID: orderID, OrganizationID: organizationID, StoreID: repo.item.StoreID})
	service := NewCommerceFulfilmentService(repo, orderRepo, &commerceFoundationRepoStub{}, commercefulfilment.NewRegistry(), manager)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleStoreStaff}

	_, err := service.VerifyHandover(context.Background(), actor, nil, fulfilmentID, VerifyCommerceHandoverInput{VerificationCode: "123456", IdempotencyKey: "handover-attempt-001"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(repo.handoverInput.CandidateHash, manager.Hash("123456")) || bytes.Contains(repo.handoverInput.CandidateHash, []byte("123456")) {
		t.Fatal("handover code was not converted to its keyed verification hash")
	}
	for _, status := range []string{models.CommerceOrderStatusReadyForPickup, models.CommerceOrderStatusOutForDelivery, models.CommerceOrderStatusDelivered, models.CommerceOrderStatusCompleted} {
		if canActorRequestCommerceOrderStatus(actor.Role, status) {
			t.Fatalf("generic order endpoint can bypass fulfilment-owned status %q", status)
		}
	}
}

func TestResendHandoverCodeAuthorizesStaffAndQueuesProtectedReminder(t *testing.T) {
	organizationID, orderID, fulfilmentID, storeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := &commerceFulfilmentRepoStub{item: &models.CommerceFulfilment{
		ID: fulfilmentID, OrganizationID: organizationID, OrderID: orderID, StoreID: storeID,
		Mode: models.FulfilmentModeCustomerRider, Status: models.CommerceFulfilmentStatusRiderAssigned,
	}}
	service := NewCommerceFulfilmentService(repo, seededCommerceOrderRepo(&models.CommerceOrder{
		ID: orderID, OrganizationID: organizationID, StoreID: storeID,
	}), &commerceFoundationRepoStub{}, commercefulfilment.NewRegistry(), testCommerceCodeManager(t))
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleStoreStaff}

	_, err := service.ResendHandoverCode(context.Background(), actor, nil, fulfilmentID, TransitionCommerceFulfilmentInput{
		IdempotencyKey: "handover-reminder-001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.reminderInput.FulfilmentID != fulfilmentID || repo.reminderInput.ActorUserID == nil || *repo.reminderInput.ActorUserID != actor.UserID {
		t.Fatalf("handover reminder was not attributed correctly: %+v", repo.reminderInput)
	}
}

func TestHandoverRejectsUnauthorizedStaffIncorrectCodeAndCompletedState(t *testing.T) {
	organizationID, orderID, fulfilmentID, storeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	item := &models.CommerceFulfilment{
		ID: fulfilmentID, OrganizationID: organizationID, OrderID: orderID, StoreID: storeID,
		Mode: models.FulfilmentModeCustomerPickup, Status: models.CommerceFulfilmentStatusReadyForPickup,
	}
	orderRepo := seededCommerceOrderRepo(&models.CommerceOrder{ID: orderID, OrganizationID: organizationID, StoreID: storeID})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleStoreStaff}
	input := VerifyCommerceHandoverInput{VerificationCode: "123456", IdempotencyKey: "handover-edge-001"}

	deniedRepo := &commerceFulfilmentRepoStub{item: item}
	deniedService := NewCommerceFulfilmentService(deniedRepo, orderRepo, &deniedCommerceFoundationRepoStub{}, commercefulfilment.NewRegistry(), testCommerceCodeManager(t))
	if _, err := deniedService.VerifyHandover(context.Background(), actor, nil, fulfilmentID, input); !errors.Is(err, repository.ErrCommerceNotFound) {
		t.Fatalf("expected unassigned staff to be denied, got %v", err)
	}
	if deniedRepo.handoverInput.FulfilmentID != uuid.Nil {
		t.Fatal("handover repository was called for unauthorized staff")
	}

	incorrectRepo := &commerceFulfilmentRepoStub{item: item, operationErr: repository.ErrCommerceVerificationFailed}
	incorrectService := NewCommerceFulfilmentService(incorrectRepo, orderRepo, &commerceFoundationRepoStub{}, commercefulfilment.NewRegistry(), testCommerceCodeManager(t))
	if _, err := incorrectService.VerifyHandover(context.Background(), actor, nil, fulfilmentID, input); !errors.Is(err, repository.ErrCommerceVerificationFailed) {
		t.Fatalf("expected incorrect verification error, got %v", err)
	}

	completedRepo := &commerceFulfilmentRepoStub{item: item, operationErr: repository.ErrCommerceFulfilmentState}
	completedService := NewCommerceFulfilmentService(completedRepo, orderRepo, &commerceFoundationRepoStub{}, commercefulfilment.NewRegistry(), testCommerceCodeManager(t))
	if _, err := completedService.VerifyHandover(context.Background(), actor, nil, fulfilmentID, input); !errors.Is(err, repository.ErrCommerceFulfilmentState) {
		t.Fatalf("expected already-completed state error, got %v", err)
	}
}

func testCommerceCodeManager(t *testing.T) *commercefulfilment.CodeManager {
	t.Helper()
	manager, err := commercefulfilment.NewCodeManager(bytes.Repeat([]byte{0x5c}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func cloneCommerceFulfilment(item *models.CommerceFulfilment) *models.CommerceFulfilment {
	if item == nil {
		return nil
	}
	copy := *item
	copy.VerificationCodeHash = append([]byte(nil), item.VerificationCodeHash...)
	copy.VerificationCodeCiphertext = append([]byte(nil), item.VerificationCodeCiphertext...)
	copy.Quotes = append([]models.CommerceDeliveryQuote(nil), item.Quotes...)
	copy.RiderAssignments = append([]models.CommerceRiderAssignment(nil), item.RiderAssignments...)
	copy.Events = append([]models.CommerceFulfilmentEvent(nil), item.Events...)
	return &copy
}
