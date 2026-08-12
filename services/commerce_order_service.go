package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

const commercePaymentReservationLifetime = 30 * time.Minute

type CheckoutCommerceCartInput struct {
	CartID         uuid.UUID
	FulfilmentMode string
	IdempotencyKey string
}

type CommerceOrderListInput struct {
	StoreID    *uuid.UUID
	CustomerID *uuid.UUID
	Status     *string
	Statuses   []string
	Limit      int
	Offset     int
}

type TransitionCommerceOrderInput struct {
	Status         string
	Reason         string
	IdempotencyKey string
}

type CommerceOrderService struct {
	repo           repository.CommerceOrderRepository
	customerRepo   repository.CommerceCustomerRepository
	cartRepo       repository.CommerceCartRepository
	foundationRepo repository.CommerceFoundationRepository
}

func NewCommerceOrderService(repo repository.CommerceOrderRepository, customerRepo repository.CommerceCustomerRepository, cartRepo repository.CommerceCartRepository, foundationRepo repository.CommerceFoundationRepository) *CommerceOrderService {
	return &CommerceOrderService{repo: repo, customerRepo: customerRepo, cartRepo: cartRepo, foundationRepo: foundationRepo}
}

func (s *CommerceOrderService) CheckoutCart(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, input CheckoutCommerceCartInput) (*models.CommerceOrder, bool, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, false, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, false, err
	}
	actorUserID := actor.UserID
	return s.checkoutCart(ctx, organizationID, nil, storeScope(actor), models.CommerceOrderActorUser, &actorUserID, input)
}

func (s *CommerceOrderService) CheckoutCartForChannel(ctx context.Context, organizationID, customerID uuid.UUID, input CheckoutCommerceCartInput) (*models.CommerceOrder, bool, error) {
	if organizationID == uuid.Nil || customerID == uuid.Nil {
		return nil, false, ErrCommerceForbidden
	}
	return s.checkoutCart(ctx, organizationID, &customerID, nil, models.CommerceOrderActorChannel, nil, input)
}

func (s *CommerceOrderService) checkoutCart(ctx context.Context, organizationID uuid.UUID, expectedCustomerID, assignedUserID *uuid.UUID, actorType string, actorUserID *uuid.UUID, input CheckoutCommerceCartInput) (*models.CommerceOrder, bool, error) {
	checkoutKey := strings.TrimSpace(input.IdempotencyKey)
	fulfilmentMode := strings.ToLower(strings.TrimSpace(input.FulfilmentMode))
	if input.CartID == uuid.Nil || len(checkoutKey) < 8 || len(checkoutKey) > 200 {
		return nil, false, fmt.Errorf("%w: cart and an idempotency key between 8 and 200 characters are required", ErrCommerceValidation)
	}
	if !isCommerceFulfilmentMode(fulfilmentMode) {
		return nil, false, fmt.Errorf("%w: unsupported fulfilment mode", ErrCommerceValidation)
	}
	if existing, err := s.repo.GetOrderByCheckoutKey(ctx, organizationID, checkoutKey); err == nil {
		if existing.CartID != input.CartID || existing.FulfilmentMode != fulfilmentMode {
			return nil, false, repository.ErrCommerceConflict
		}
		if expectedCustomerID != nil && existing.CustomerID != *expectedCustomerID {
			return nil, false, ErrCommerceForbidden
		}
		if _, err := s.foundationRepo.GetStore(ctx, organizationID, existing.StoreID, assignedUserID); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	} else if err != nil && !errors.Is(err, repository.ErrCommerceNotFound) {
		return nil, false, err
	}

	cart, err := s.cartRepo.GetActiveCart(ctx, organizationID, input.CartID)
	if err != nil {
		return nil, false, err
	}
	customer, err := s.customerRepo.GetCustomer(ctx, organizationID, cart.CustomerID)
	if err != nil {
		return nil, false, err
	}
	if customer.Status != models.CommerceStatusActive {
		return nil, false, repository.ErrCommerceNotFound
	}
	if expectedCustomerID != nil && cart.CustomerID != *expectedCustomerID {
		return nil, false, ErrCommerceForbidden
	}
	store, err := s.foundationRepo.GetStore(ctx, organizationID, cart.StoreID, assignedUserID)
	if err != nil {
		return nil, false, err
	}
	if !storeSupportsCommerceFulfilment(store, fulfilmentMode) {
		return nil, false, fmt.Errorf("%w: fulfilment mode is not enabled for this store", ErrCommerceValidation)
	}

	now := time.Now().UTC()
	orderID := uuid.New()
	return s.repo.CheckoutCart(ctx, repository.CommerceCheckoutInput{
		OrderID:          orderID,
		OrganizationID:   organizationID,
		CartID:           cart.ID,
		OrderNumber:      commerceOrderNumber(now, orderID),
		CheckoutKey:      checkoutKey,
		FulfilmentMode:   fulfilmentMode,
		PaymentExpiresAt: now.Add(commercePaymentReservationLifetime),
		ActorType:        actorType,
		ActorUserID:      actorUserID,
	})
}

func (s *CommerceOrderService) GetOrderForChannel(ctx context.Context, organizationID, customerID uuid.UUID, orderReference string) (*models.CommerceOrder, error) {
	if organizationID == uuid.Nil || customerID == uuid.Nil || strings.TrimSpace(orderReference) == "" {
		return nil, ErrCommerceForbidden
	}
	var order *models.CommerceOrder
	var err error
	if orderID, parseErr := uuid.Parse(strings.TrimSpace(orderReference)); parseErr == nil {
		order, err = s.repo.GetOrder(ctx, organizationID, orderID)
	} else {
		order, err = s.repo.GetOrderByNumber(ctx, organizationID, orderReference)
	}
	if err != nil {
		return nil, err
	}
	if order.CustomerID != customerID {
		return nil, repository.ErrCommerceNotFound
	}
	return order, nil
}

func (s *CommerceOrderService) SetOrderDestinationForChannel(ctx context.Context, organizationID, customerID, orderID uuid.UUID, address string, latitude, longitude *float64) (*models.CommerceOrder, error) {
	address = strings.TrimSpace(address)
	if organizationID == uuid.Nil || customerID == uuid.Nil || orderID == uuid.Nil || len(address) < 5 || len(address) > 500 {
		return nil, fmt.Errorf("%w: a delivery address between 5 and 500 characters is required", ErrCommerceValidation)
	}
	if (latitude == nil) != (longitude == nil) || (latitude != nil && (*latitude < -90 || *latitude > 90 || *longitude < -180 || *longitude > 180)) {
		return nil, fmt.Errorf("%w: destination coordinates must be a valid pair", ErrCommerceValidation)
	}
	return s.repo.SetOrderDestination(ctx, organizationID, customerID, orderID, address, latitude, longitude)
}

func (s *CommerceOrderService) GetOrder(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, orderID uuid.UUID) (*models.CommerceOrder, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if orderID == uuid.Nil {
		return nil, fmt.Errorf("%w: order is required", ErrCommerceValidation)
	}
	order, err := s.repo.GetOrder(ctx, organizationID, orderID)
	if err != nil {
		return nil, err
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, order.StoreID, storeScope(actor)); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *CommerceOrderService) ListOrders(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, input CommerceOrderListInput) ([]models.CommerceOrder, int64, error) {
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
		if !isCommerceOrderStatus(status) {
			return nil, 0, fmt.Errorf("%w: unknown order status", ErrCommerceValidation)
		}
		input.Status = &status
	}
	if input.Status != nil && len(input.Statuses) > 0 {
		return nil, 0, fmt.Errorf("%w: use either status or statuses", ErrCommerceValidation)
	}
	statuses := make([]string, 0, len(input.Statuses))
	seenStatuses := make(map[string]struct{}, len(input.Statuses))
	for _, candidate := range input.Statuses {
		status := strings.ToLower(strings.TrimSpace(candidate))
		if !isCommerceOrderStatus(status) {
			return nil, 0, fmt.Errorf("%w: unknown order status", ErrCommerceValidation)
		}
		if _, exists := seenStatuses[status]; exists {
			continue
		}
		seenStatuses[status] = struct{}{}
		statuses = append(statuses, status)
	}
	limit, offset := commercePagination(input.Limit, input.Offset)
	return s.repo.ListOrders(ctx, organizationID, storeScope(actor), repository.CommerceOrderListFilter{
		StoreID: input.StoreID, CustomerID: input.CustomerID, Status: input.Status, Statuses: statuses, Limit: limit, Offset: offset,
	})
}

func (s *CommerceOrderService) TransitionOrder(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, orderID uuid.UUID, input TransitionCommerceOrderInput) (*models.CommerceOrder, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	targetStatus := strings.ToLower(strings.TrimSpace(input.Status))
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	reason := strings.TrimSpace(input.Reason)
	if orderID == uuid.Nil || len(idempotencyKey) < 8 || len(idempotencyKey) > 200 || len(reason) > 500 || !isCommerceOrderStatus(targetStatus) {
		return nil, fmt.Errorf("%w: order, known status, and an idempotency key between 8 and 200 characters are required", ErrCommerceValidation)
	}
	if !canActorRequestCommerceOrderStatus(actor.Role, targetStatus) {
		return nil, ErrCommerceForbidden
	}
	if targetStatus == models.CommerceOrderStatusCancelled && reason == "" {
		return nil, fmt.Errorf("%w: cancellation reason is required", ErrCommerceValidation)
	}

	order, err := s.repo.GetOrder(ctx, organizationID, orderID)
	if err != nil {
		return nil, err
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, order.StoreID, storeScope(actor)); err != nil {
		return nil, err
	}
	actorUserID := actor.UserID
	return s.repo.TransitionOrder(ctx, repository.CommerceOrderTransitionInput{
		OrganizationID: organizationID,
		OrderID:        order.ID,
		FromStatus:     order.Status,
		ToStatus:       targetStatus,
		EventType:      commerceOrderEventForStatus(targetStatus),
		Reason:         reason,
		IdempotencyKey: idempotencyKey,
		ActorType:      models.CommerceOrderActorUser,
		ActorUserID:    &actorUserID,
		Allowed:        isValidCommerceOrderTransition(order.Status, targetStatus, order.FulfilmentMode),
	})
}

func isValidCommerceOrderTransition(from, to, fulfilmentMode string) bool {
	if from == to {
		return false
	}
	switch from {
	case models.CommerceOrderStatusDraft:
		return to == models.CommerceOrderStatusPendingPayment || to == models.CommerceOrderStatusCancelled
	case models.CommerceOrderStatusPendingPayment:
		return to == models.CommerceOrderStatusPaid || to == models.CommerceOrderStatusPaymentFailed || to == models.CommerceOrderStatusPaymentExpired || to == models.CommerceOrderStatusCancelled
	case models.CommerceOrderStatusPaymentFailed:
		return to == models.CommerceOrderStatusPendingPayment || to == models.CommerceOrderStatusPaymentExpired || to == models.CommerceOrderStatusCancelled
	case models.CommerceOrderStatusPaymentExpired:
		return to == models.CommerceOrderStatusCancelled
	case models.CommerceOrderStatusPaid:
		return to == models.CommerceOrderStatusProcessing || to == models.CommerceOrderStatusRefunded
	case models.CommerceOrderStatusProcessing:
		return to == models.CommerceOrderStatusReady || to == models.CommerceOrderStatusRefunded
	case models.CommerceOrderStatusReady:
		if to == models.CommerceOrderStatusRefunded {
			return true
		}
		if fulfilmentMode == models.FulfilmentModeCustomerPickup {
			return to == models.CommerceOrderStatusReadyForPickup
		}
		return to == models.CommerceOrderStatusFulfilmentPending
	case models.CommerceOrderStatusFulfilmentPending:
		return to == models.CommerceOrderStatusOutForDelivery || to == models.CommerceOrderStatusRefunded
	case models.CommerceOrderStatusReadyForPickup:
		return to == models.CommerceOrderStatusCompleted || to == models.CommerceOrderStatusRefunded
	case models.CommerceOrderStatusOutForDelivery:
		return to == models.CommerceOrderStatusDelivered || to == models.CommerceOrderStatusRefunded
	case models.CommerceOrderStatusDelivered:
		return to == models.CommerceOrderStatusCompleted || to == models.CommerceOrderStatusRefunded
	case models.CommerceOrderStatusCompleted:
		return to == models.CommerceOrderStatusRefunded
	default:
		return false
	}
}

func canActorRequestCommerceOrderStatus(role, status string) bool {
	switch status {
	case models.CommerceOrderStatusProcessing,
		models.CommerceOrderStatusReady:
		return canAccessCommerce(role)
	case models.CommerceOrderStatusCancelled:
		return canManageMerchant(role)
	default:
		return false
	}
}

func commerceOrderEventForStatus(status string) string {
	switch status {
	case models.CommerceOrderStatusPendingPayment:
		return models.CommerceOrderEventPaymentInitiated
	case models.CommerceOrderStatusPaid:
		return models.CommerceOrderEventPaymentConfirmed
	case models.CommerceOrderStatusPaymentFailed:
		return models.CommerceOrderEventPaymentFailed
	case models.CommerceOrderStatusPaymentExpired:
		return models.CommerceOrderEventPaymentExpired
	case models.CommerceOrderStatusProcessing:
		return models.CommerceOrderEventProcessing
	case models.CommerceOrderStatusReady:
		return models.CommerceOrderEventReady
	case models.CommerceOrderStatusFulfilmentPending:
		return models.CommerceOrderEventFulfilmentPending
	case models.CommerceOrderStatusReadyForPickup:
		return models.CommerceOrderEventReadyForPickup
	case models.CommerceOrderStatusOutForDelivery:
		return models.CommerceOrderEventOutForDelivery
	case models.CommerceOrderStatusDelivered:
		return models.CommerceOrderEventDelivered
	case models.CommerceOrderStatusCompleted:
		return models.CommerceOrderEventCompleted
	case models.CommerceOrderStatusCancelled:
		return models.CommerceOrderEventCancelled
	case models.CommerceOrderStatusRefunded:
		return models.CommerceOrderEventRefunded
	default:
		return models.CommerceOrderEventCreated
	}
}

func isCommerceOrderStatus(status string) bool {
	switch status {
	case models.CommerceOrderStatusDraft,
		models.CommerceOrderStatusPendingPayment,
		models.CommerceOrderStatusPaid,
		models.CommerceOrderStatusProcessing,
		models.CommerceOrderStatusReady,
		models.CommerceOrderStatusFulfilmentPending,
		models.CommerceOrderStatusReadyForPickup,
		models.CommerceOrderStatusOutForDelivery,
		models.CommerceOrderStatusDelivered,
		models.CommerceOrderStatusCompleted,
		models.CommerceOrderStatusPaymentFailed,
		models.CommerceOrderStatusPaymentExpired,
		models.CommerceOrderStatusCancelled,
		models.CommerceOrderStatusRefunded:
		return true
	default:
		return false
	}
}

func isCommerceFulfilmentMode(mode string) bool {
	return mode == models.FulfilmentModeCustomerPickup || mode == models.FulfilmentModeCustomerRider || mode == models.FulfilmentModeMerchantRider
}

func storeSupportsCommerceFulfilment(store *models.CommerceStore, mode string) bool {
	for _, item := range store.FulfilmentModes {
		if item.Mode == mode && item.Enabled {
			return true
		}
	}
	return false
}

func commerceOrderNumber(now time.Time, orderID uuid.UUID) string {
	prefix := strings.ToUpper(strings.ReplaceAll(orderID.String()[:8], "-", ""))
	return "ZC-" + now.UTC().Format("20060102") + "-" + prefix
}
