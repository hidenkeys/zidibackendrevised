package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/fulfilment"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

var ErrCommerceDeliveryProviderUnavailable = errors.New("delivery quote provider unavailable")

const commerceFulfilmentCodeLifetime = 48 * time.Hour

var commerceVerificationCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

type StartCommerceFulfilmentInput struct {
	DestinationAddress   string
	DestinationLatitude  *float64
	DestinationLongitude *float64
	IdempotencyKey       string
}

type CreateCommerceDeliveryQuoteInput struct {
	Source            string
	Provider          string
	EstimatedFeeMinor *int64
	DistanceMeters    *int
	DurationSeconds   *int
	ExpiresAt         *time.Time
	IdempotencyKey    string
}

type DecideCommerceDeliveryQuoteInput struct {
	Decision       string
	Reason         string
	IdempotencyKey string
}

type AssignCommerceRiderInput struct {
	Source               string
	Provider             string
	ProviderAssignmentID string
	RiderName            string
	RiderPhone           string
	VehicleDescription   string
	TrackingURL          string
	IdempotencyKey       string
}

type VerifyCommerceHandoverInput struct {
	VerificationCode string
	IdempotencyKey   string
}

type TransitionCommerceFulfilmentInput struct {
	Reason         string
	IdempotencyKey string
}

type CommerceFulfilmentService struct {
	repo           repository.CommerceFulfilmentRepository
	orderRepo      repository.CommerceOrderRepository
	foundationRepo repository.CommerceFoundationRepository
	providers      *fulfilment.Registry
	codes          *fulfilment.CodeManager
}

func NewCommerceFulfilmentService(repo repository.CommerceFulfilmentRepository, orderRepo repository.CommerceOrderRepository, foundationRepo repository.CommerceFoundationRepository, providers *fulfilment.Registry, codes *fulfilment.CodeManager) *CommerceFulfilmentService {
	return &CommerceFulfilmentService{repo: repo, orderRepo: orderRepo, foundationRepo: foundationRepo, providers: providers, codes: codes}
}

func (s *CommerceFulfilmentService) StartFulfilment(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, orderID uuid.UUID, input StartCommerceFulfilmentInput) (*models.CommerceFulfilment, bool, error) {
	organizationID, err := s.authorize(ctx, actor, requestedOrganizationID, orderID, uuid.Nil)
	if err != nil {
		return nil, false, err
	}
	key, err := validateCommerceFulfilmentKey(orderID, input.IdempotencyKey)
	if err != nil {
		return nil, false, err
	}
	order, err := s.orderRepo.GetOrder(ctx, organizationID, orderID)
	if err != nil {
		return nil, false, err
	}
	store, err := s.foundationRepo.GetStore(ctx, organizationID, order.StoreID, storeScope(actor))
	if err != nil {
		return nil, false, err
	}
	destinationAddress := input.DestinationAddress
	destinationLatitude := input.DestinationLatitude
	destinationLongitude := input.DestinationLongitude
	if strings.TrimSpace(destinationAddress) == "" && order.DestinationAddress != nil {
		destinationAddress = *order.DestinationAddress
		destinationLatitude = order.DestinationLatitude
		destinationLongitude = order.DestinationLongitude
	}
	destination, err := validateCommerceDestination(order.FulfilmentMode, destinationAddress, destinationLatitude, destinationLongitude)
	if err != nil {
		return nil, false, err
	}
	if s.codes == nil {
		return nil, false, errors.New("fulfilment code manager is not configured")
	}
	protectedCode, err := s.codes.Generate()
	if err != nil {
		return nil, false, err
	}
	now := time.Now().UTC()
	status := models.CommerceFulfilmentStatusReadyForPickup
	orderStatus := models.CommerceOrderStatusReadyForPickup
	if order.FulfilmentMode == models.FulfilmentModeCustomerRider {
		orderStatus = models.CommerceOrderStatusFulfilmentPending
	}
	if order.FulfilmentMode == models.FulfilmentModeMerchantRider {
		status = models.CommerceFulfilmentStatusAwaitingQuote
		orderStatus = models.CommerceOrderStatusFulfilmentPending
	}
	item := models.CommerceFulfilment{
		ID: uuid.New(), OrganizationID: organizationID, OrderID: order.ID, StoreID: order.StoreID, CustomerID: order.CustomerID,
		Mode: order.FulfilmentMode, Status: status, PickupAddress: formatCommerceStoreAddress(store),
		PickupLatitude: store.Latitude, PickupLongitude: store.Longitude, DestinationAddress: destination,
		DestinationLatitude: destinationLatitude, DestinationLongitude: destinationLongitude,
		VerificationCodeHash: protectedCode.Hash, VerificationCodeCiphertext: protectedCode.Ciphertext,
		VerificationCodeExpiresAt: now.Add(commerceFulfilmentCodeLifetime), Version: 1,
	}
	return s.repo.StartFulfilment(ctx, repository.CommerceStartFulfilmentInput{
		Fulfilment: item, OrderStatus: orderStatus, ActorUserID: actor.UserID, IdempotencyKey: key, Now: now,
	})
}

func (s *CommerceFulfilmentService) GetOrderFulfilment(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, orderID uuid.UUID) (*models.CommerceFulfilment, error) {
	organizationID, err := s.authorize(ctx, actor, requestedOrganizationID, orderID, uuid.Nil)
	if err != nil {
		return nil, err
	}
	return s.repo.GetFulfilmentByOrder(ctx, organizationID, orderID)
}

func (s *CommerceFulfilmentService) ListOrderFulfilments(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, orders []models.CommerceOrder) (map[uuid.UUID]*models.CommerceFulfilment, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	orderIDs := make([]uuid.UUID, 0, len(orders))
	validatedStores := make(map[uuid.UUID]struct{})
	for _, order := range orders {
		if order.OrganizationID != organizationID || order.ID == uuid.Nil || order.StoreID == uuid.Nil {
			return nil, ErrCommerceForbidden
		}
		if _, validated := validatedStores[order.StoreID]; !validated {
			if _, storeErr := s.foundationRepo.GetStore(ctx, organizationID, order.StoreID, storeScope(actor)); storeErr != nil {
				return nil, storeErr
			}
			validatedStores[order.StoreID] = struct{}{}
		}
		orderIDs = append(orderIDs, order.ID)
	}
	items, err := s.repo.ListFulfilmentsByOrderIDs(ctx, organizationID, orderIDs)
	if err != nil {
		return nil, err
	}
	byOrderID := make(map[uuid.UUID]*models.CommerceFulfilment, len(items))
	for i := range items {
		byOrderID[items[i].OrderID] = &items[i]
	}
	return byOrderID, nil
}

func (s *CommerceFulfilmentService) CreateDeliveryQuote(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, fulfilmentID uuid.UUID, input CreateCommerceDeliveryQuoteInput) (*models.CommerceFulfilment, error) {
	organizationID, item, err := s.authorizeFulfilment(ctx, actor, requestedOrganizationID, fulfilmentID)
	if err != nil {
		return nil, err
	}
	key, err := validateCommerceFulfilmentKey(fulfilmentID, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if item.Mode != models.FulfilmentModeMerchantRider || item.DestinationAddress == nil {
		return nil, repository.ErrCommerceFulfilmentState
	}
	order, err := s.orderRepo.GetOrder(ctx, organizationID, item.OrderID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	source := strings.ToLower(strings.TrimSpace(input.Source))
	quote := models.CommerceDeliveryQuote{
		ID: uuid.New(), OrganizationID: organizationID, FulfilmentID: item.ID, OrderID: item.OrderID,
		Status: models.CommerceDeliveryQuoteStatusQuoted, PickupAddress: item.PickupAddress,
		PickupLatitude: item.PickupLatitude, PickupLongitude: item.PickupLongitude,
		DestinationAddress: *item.DestinationAddress, DestinationLatitude: item.DestinationLatitude,
		DestinationLongitude: item.DestinationLongitude, Currency: order.Currency,
		FeePaymentMode: models.CommerceDeliveryFeePaymentDirectToRider,
		FeeStatus:      models.CommerceDeliveryFeeStatusNotCollected, RawResponse: json.RawMessage(`{}`),
		IdempotencyKey: key, CreatedByUserID: actor.UserID,
	}
	switch source {
	case models.CommerceDeliveryQuoteSourceManual:
		if input.EstimatedFeeMinor == nil || *input.EstimatedFeeMinor < 0 || !validOptionalCommerceMetric(input.DistanceMeters) || !validOptionalCommerceMetric(input.DurationSeconds) {
			return nil, fmt.Errorf("%w: a non-negative manual fee and valid optional distance and duration are required", ErrCommerceValidation)
		}
		if strings.TrimSpace(input.Provider) != "" {
			return nil, fmt.Errorf("%w: manual quotes cannot name a delivery provider", ErrCommerceValidation)
		}
		if input.ExpiresAt != nil && !input.ExpiresAt.After(now) {
			return nil, fmt.Errorf("%w: quote expiry must be in the future", ErrCommerceValidation)
		}
		quote.Source = source
		quote.EstimatedFeeMinor = *input.EstimatedFeeMinor
		quote.DistanceMeters = input.DistanceMeters
		quote.DurationSeconds = input.DurationSeconds
		quote.ExpiresAt = normalizeFutureCommerceTime(input.ExpiresAt, now)
	case models.CommerceDeliveryQuoteSourceProvider:
		providerName := strings.ToLower(strings.TrimSpace(input.Provider))
		if len(providerName) < 2 || len(providerName) > 50 {
			return nil, fmt.Errorf("%w: provider name must contain 2 to 50 characters", ErrCommerceValidation)
		}
		provider, lookupErr := s.providers.Get(providerName)
		if lookupErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrCommerceDeliveryProviderUnavailable, lookupErr)
		}
		providerQuote, quoteErr := provider.Quote(ctx, fulfilment.DeliveryQuoteRequest{
			Reference: item.OrderID.String(), Currency: order.Currency,
			Pickup:      fulfilment.Location{Address: item.PickupAddress, Latitude: item.PickupLatitude, Longitude: item.PickupLongitude},
			Destination: fulfilment.Location{Address: *item.DestinationAddress, Latitude: item.DestinationLatitude, Longitude: item.DestinationLongitude},
		})
		if quoteErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrCommerceDeliveryProviderUnavailable, quoteErr)
		}
		if providerQuote == nil || providerQuote.EstimatedFeeMinor < 0 || !strings.EqualFold(providerQuote.Currency, order.Currency) ||
			!validOptionalCommerceMetric(providerQuote.DistanceMeters) || !validOptionalCommerceMetric(providerQuote.DurationSeconds) ||
			(providerQuote.ExpiresAt != nil && !providerQuote.ExpiresAt.After(now)) ||
			(len(providerQuote.RawResponse) > 0 && !validCommerceJSONObject(providerQuote.RawResponse)) {
			return nil, fmt.Errorf("%w: provider returned an invalid quote", ErrCommerceDeliveryProviderUnavailable)
		}
		quote.Source = source
		quote.Provider = optionalCommerceFulfilmentString(providerName)
		quote.ProviderQuoteID = optionalCommerceFulfilmentString(providerQuote.ProviderQuoteID)
		quote.EstimatedFeeMinor = providerQuote.EstimatedFeeMinor
		quote.DistanceMeters = providerQuote.DistanceMeters
		quote.DurationSeconds = providerQuote.DurationSeconds
		quote.ExpiresAt = providerQuote.ExpiresAt
		if len(providerQuote.RawResponse) > 0 {
			quote.RawResponse = json.RawMessage(providerQuote.RawResponse)
		}
	default:
		return nil, fmt.Errorf("%w: delivery quote source must be manual or provider", ErrCommerceValidation)
	}
	return s.repo.CreateDeliveryQuote(ctx, repository.CommerceCreateDeliveryQuoteInput{
		Quote: quote, ActorUserID: actor.UserID, IdempotencyKey: key, Now: now,
	})
}

func (s *CommerceFulfilmentService) DecideDeliveryQuote(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, fulfilmentID, quoteID uuid.UUID, input DecideCommerceDeliveryQuoteInput) (*models.CommerceFulfilment, error) {
	organizationID, _, err := s.authorizeFulfilment(ctx, actor, requestedOrganizationID, fulfilmentID)
	if err != nil {
		return nil, err
	}
	key, err := validateCommerceFulfilmentKey(fulfilmentID, input.IdempotencyKey)
	if err != nil || quoteID == uuid.Nil {
		return nil, fmt.Errorf("%w: quote and an idempotency key between 8 and 200 characters are required", ErrCommerceValidation)
	}
	decision := strings.ToLower(strings.TrimSpace(input.Decision))
	if decision != models.CommerceDeliveryQuoteStatusAccepted && decision != models.CommerceDeliveryQuoteStatusRejected {
		return nil, fmt.Errorf("%w: quote decision must be accepted or rejected", ErrCommerceValidation)
	}
	actorUserID := actor.UserID
	return s.repo.DecideDeliveryQuote(ctx, repository.CommerceDeliveryQuoteDecisionInput{
		OrganizationID: organizationID, FulfilmentID: fulfilmentID, QuoteID: quoteID, Decision: decision,
		ActorType: models.CommerceFulfilmentActorUser, ActorUserID: &actorUserID, Reason: commerceFulfilmentReason(input.Reason, "customer delivery quote decision recorded"),
		IdempotencyKey: key, Now: time.Now().UTC(),
	})
}

// DecideDeliveryQuoteForCustomer is the channel-safe boundary used after a customer
// identity has been authenticated by WhatsApp or another customer-facing channel.
func (s *CommerceFulfilmentService) DecideDeliveryQuoteForCustomer(ctx context.Context, organizationID, customerID, fulfilmentID, quoteID uuid.UUID, input DecideCommerceDeliveryQuoteInput) (*models.CommerceFulfilment, error) {
	if organizationID == uuid.Nil || customerID == uuid.Nil || fulfilmentID == uuid.Nil || quoteID == uuid.Nil {
		return nil, ErrCommerceForbidden
	}
	item, err := s.repo.GetFulfilment(ctx, organizationID, fulfilmentID)
	if err != nil {
		return nil, err
	}
	if item.CustomerID != customerID {
		return nil, ErrCommerceForbidden
	}
	key, err := validateCommerceFulfilmentKey(fulfilmentID, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	decision := strings.ToLower(strings.TrimSpace(input.Decision))
	if decision != models.CommerceDeliveryQuoteStatusAccepted && decision != models.CommerceDeliveryQuoteStatusRejected {
		return nil, fmt.Errorf("%w: quote decision must be accepted or rejected", ErrCommerceValidation)
	}
	return s.repo.DecideDeliveryQuote(ctx, repository.CommerceDeliveryQuoteDecisionInput{
		OrganizationID: organizationID, FulfilmentID: fulfilmentID, QuoteID: quoteID, Decision: decision,
		ActorType: models.CommerceFulfilmentActorCustomer, Reason: commerceFulfilmentReason(input.Reason, "customer delivery quote decision recorded"),
		IdempotencyKey: key, Now: time.Now().UTC(),
	})
}

func (s *CommerceFulfilmentService) AssignRider(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, fulfilmentID uuid.UUID, input AssignCommerceRiderInput) (*models.CommerceFulfilment, error) {
	organizationID, item, err := s.authorizeFulfilment(ctx, actor, requestedOrganizationID, fulfilmentID)
	if err != nil {
		return nil, err
	}
	key, err := validateCommerceFulfilmentKey(fulfilmentID, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	source := strings.ToLower(strings.TrimSpace(input.Source))
	expectedSource := models.CommerceRiderSourceMerchant
	if item.Mode == models.FulfilmentModeCustomerRider {
		expectedSource = models.CommerceRiderSourceCustomer
	}
	riderName := strings.TrimSpace(input.RiderName)
	riderPhone := strings.TrimSpace(input.RiderPhone)
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	providerAssignmentID := strings.TrimSpace(input.ProviderAssignmentID)
	if source != expectedSource || riderName == "" || len(riderName) > 200 || !validCommerceRiderPhone(riderPhone) || len(riderPhone) > 40 {
		return nil, fmt.Errorf("%w: rider source, name, and phone are required and must match the fulfilment mode", ErrCommerceValidation)
	}
	if len(provider) > 50 || len(providerAssignmentID) > 200 {
		return nil, fmt.Errorf("%w: rider provider identifiers are too long", ErrCommerceValidation)
	}
	if len(strings.TrimSpace(input.VehicleDescription)) > 200 {
		return nil, fmt.Errorf("%w: vehicle description is too long", ErrCommerceValidation)
	}
	trackingURL := strings.TrimSpace(input.TrackingURL)
	if len(trackingURL) > 2000 {
		return nil, fmt.Errorf("%w: tracking URL is too long", ErrCommerceValidation)
	}
	if trackingURL != "" {
		parsed, parseErr := url.ParseRequestURI(trackingURL)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, fmt.Errorf("%w: tracking URL must use HTTP or HTTPS", ErrCommerceValidation)
		}
	}
	now := time.Now().UTC()
	assignment := models.CommerceRiderAssignment{
		ID: uuid.New(), OrganizationID: organizationID, FulfilmentID: item.ID, OrderID: item.OrderID, StoreID: item.StoreID,
		Source: source, Provider: optionalCommerceFulfilmentString(provider),
		ProviderAssignmentID: optionalCommerceFulfilmentString(providerAssignmentID),
		RiderName:            riderName, RiderPhone: riderPhone,
		VehicleDescription: optionalCommerceFulfilmentString(input.VehicleDescription), TrackingURL: optionalCommerceFulfilmentString(trackingURL),
		Status: models.CommerceRiderStatusAssigned, IdempotencyKey: key, AssignedByUserID: actor.UserID,
	}
	return s.repo.AssignRider(ctx, repository.CommerceAssignRiderInput{Assignment: assignment, ActorUserID: actor.UserID, IdempotencyKey: key, Now: now})
}

func (s *CommerceFulfilmentService) VerifyHandover(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, fulfilmentID uuid.UUID, input VerifyCommerceHandoverInput) (*models.CommerceFulfilment, error) {
	organizationID, _, err := s.authorizeFulfilment(ctx, actor, requestedOrganizationID, fulfilmentID)
	if err != nil {
		return nil, err
	}
	key, err := validateCommerceFulfilmentKey(fulfilmentID, input.IdempotencyKey)
	if err != nil || !commerceVerificationCodePattern.MatchString(strings.TrimSpace(input.VerificationCode)) {
		return nil, fmt.Errorf("%w: a six-digit verification code and an idempotency key between 8 and 200 characters are required", ErrCommerceValidation)
	}
	if s.codes == nil {
		return nil, errors.New("fulfilment code manager is not configured")
	}
	return s.repo.VerifyHandover(ctx, repository.CommerceVerifyHandoverInput{
		OrganizationID: organizationID, FulfilmentID: fulfilmentID,
		CandidateHash: s.codes.Hash(strings.TrimSpace(input.VerificationCode)), ActorUserID: actor.UserID,
		IdempotencyKey: key, Now: time.Now().UTC(),
	})
}

func (s *CommerceFulfilmentService) RecordArrival(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, fulfilmentID uuid.UUID, input TransitionCommerceFulfilmentInput) (*models.CommerceFulfilment, error) {
	organizationID, item, err := s.authorizeFulfilment(ctx, actor, requestedOrganizationID, fulfilmentID)
	if err != nil {
		return nil, err
	}
	key, err := validateCommerceFulfilmentKey(fulfilmentID, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	actorUserID := actor.UserID
	fallback := "customer arrival recorded before handover verification"
	if item.Mode != models.FulfilmentModeCustomerPickup {
		fallback = "rider arrival recorded before handover verification"
	}
	return s.repo.RecordArrival(ctx, repository.CommerceFulfilmentTransitionInput{
		OrganizationID: organizationID, FulfilmentID: fulfilmentID, ActorType: models.CommerceFulfilmentActorUser,
		ActorUserID: &actorUserID, Reason: commerceFulfilmentReason(input.Reason, fallback), IdempotencyKey: key, Now: time.Now().UTC(),
	})
}

func (s *CommerceFulfilmentService) MarkDelivered(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, fulfilmentID uuid.UUID, input TransitionCommerceFulfilmentInput) (*models.CommerceFulfilment, error) {
	organizationID, _, err := s.authorizeFulfilment(ctx, actor, requestedOrganizationID, fulfilmentID)
	if err != nil {
		return nil, err
	}
	key, err := validateCommerceFulfilmentKey(fulfilmentID, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	actorUserID := actor.UserID
	return s.repo.MarkDelivered(ctx, repository.CommerceFulfilmentTransitionInput{
		OrganizationID: organizationID, FulfilmentID: fulfilmentID, ActorType: models.CommerceFulfilmentActorUser,
		ActorUserID: &actorUserID, Reason: commerceFulfilmentReason(input.Reason, "delivery confirmed"), IdempotencyKey: key, Now: time.Now().UTC(),
	})
}

func (s *CommerceFulfilmentService) CompleteFulfilment(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, fulfilmentID uuid.UUID, input TransitionCommerceFulfilmentInput) (*models.CommerceFulfilment, error) {
	organizationID, _, err := s.authorizeFulfilment(ctx, actor, requestedOrganizationID, fulfilmentID)
	if err != nil {
		return nil, err
	}
	key, err := validateCommerceFulfilmentKey(fulfilmentID, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	actorUserID := actor.UserID
	return s.repo.CompleteFulfilment(ctx, repository.CommerceFulfilmentTransitionInput{
		OrganizationID: organizationID, FulfilmentID: fulfilmentID, ActorType: models.CommerceFulfilmentActorUser,
		ActorUserID: &actorUserID, Reason: commerceFulfilmentReason(input.Reason, "delivery fulfilment completed"), IdempotencyKey: key, Now: time.Now().UTC(),
	})
}

// RevealVerificationCode is an internal boundary for a future authenticated customer channel.
// Staff-facing HTTP handlers deliberately never expose the plaintext code.
func (s *CommerceFulfilmentService) RevealVerificationCode(ctx context.Context, organizationID, customerID, fulfilmentID uuid.UUID) (string, error) {
	if organizationID == uuid.Nil || customerID == uuid.Nil || fulfilmentID == uuid.Nil {
		return "", ErrCommerceForbidden
	}
	item, err := s.repo.GetFulfilment(ctx, organizationID, fulfilmentID)
	if err != nil {
		return "", err
	}
	if item.CustomerID != customerID || item.Status == models.CommerceFulfilmentStatusCompleted || item.Status == models.CommerceFulfilmentStatusCancelled {
		return "", ErrCommerceForbidden
	}
	if !item.VerificationCodeExpiresAt.After(time.Now().UTC()) {
		return "", repository.ErrCommerceVerificationExpired
	}
	if s.codes == nil {
		return "", errors.New("fulfilment code manager is not configured")
	}
	return s.codes.Reveal(item.VerificationCodeCiphertext)
}

func (s *CommerceFulfilmentService) authorize(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, orderID, storeID uuid.UUID) (uuid.UUID, error) {
	if !canAccessCommerce(actor.Role) {
		return uuid.Nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return uuid.Nil, err
	}
	if orderID != uuid.Nil {
		order, err := s.orderRepo.GetOrder(ctx, organizationID, orderID)
		if err != nil {
			return uuid.Nil, err
		}
		storeID = order.StoreID
	}
	if storeID == uuid.Nil {
		return uuid.Nil, ErrCommerceValidation
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, storeID, storeScope(actor)); err != nil {
		return uuid.Nil, err
	}
	return organizationID, nil
}

func (s *CommerceFulfilmentService) authorizeFulfilment(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, fulfilmentID uuid.UUID) (uuid.UUID, *models.CommerceFulfilment, error) {
	if fulfilmentID == uuid.Nil || !canAccessCommerce(actor.Role) {
		return uuid.Nil, nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	item, err := s.repo.GetFulfilment(ctx, organizationID, fulfilmentID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, item.StoreID, storeScope(actor)); err != nil {
		return uuid.Nil, nil, err
	}
	return organizationID, item, nil
}

func validateCommerceFulfilmentKey(resourceID uuid.UUID, key string) (string, error) {
	key = strings.TrimSpace(key)
	if resourceID == uuid.Nil || len(key) < 8 || len(key) > 200 {
		return "", fmt.Errorf("%w: resource and an idempotency key between 8 and 200 characters are required", ErrCommerceValidation)
	}
	return key, nil
}

func validateCommerceDestination(mode, address string, latitude, longitude *float64) (*string, error) {
	address = strings.TrimSpace(address)
	if len(address) > 500 {
		return nil, fmt.Errorf("%w: destination address is too long", ErrCommerceValidation)
	}
	if (latitude == nil) != (longitude == nil) || (latitude != nil && (*latitude < -90 || *latitude > 90 || *longitude < -180 || *longitude > 180)) {
		return nil, fmt.Errorf("%w: destination coordinates must be a valid latitude/longitude pair", ErrCommerceValidation)
	}
	if mode == models.FulfilmentModeMerchantRider && address == "" {
		return nil, fmt.Errorf("%w: destination address is required for merchant-arranged delivery", ErrCommerceValidation)
	}
	return optionalCommerceFulfilmentString(address), nil
}

func formatCommerceStoreAddress(store *models.CommerceStore) string {
	parts := []string{strings.TrimSpace(store.Address), strings.TrimSpace(store.City), strings.TrimSpace(store.State), strings.TrimSpace(store.CountryCode)}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}
	return strings.Join(result, ", ")
}

func validOptionalCommerceMetric(value *int) bool { return value == nil || *value >= 0 }

func validCommerceJSONObject(value []byte) bool {
	var object map[string]interface{}
	return json.Unmarshal(value, &object) == nil && object != nil
}

func normalizeFutureCommerceTime(value *time.Time, now time.Time) *time.Time {
	if value == nil || !value.After(now) {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}

func validCommerceRiderPhone(value string) bool {
	value = strings.TrimSpace(value)
	digits := 0
	for index, character := range value {
		if character >= '0' && character <= '9' {
			digits++
			continue
		}
		if character == '+' && index == 0 {
			continue
		}
		if character != ' ' && character != '-' && character != '(' && character != ')' {
			return false
		}
	}
	return digits >= 7 && digits <= 15
}

func optionalCommerceFulfilmentString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func commerceFulfilmentReason(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if len(value) > 500 {
		return value[:500]
	}
	return value
}
