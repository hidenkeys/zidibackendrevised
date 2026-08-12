package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/utils"
)

type commerceOrderRepoStub struct {
	mu               sync.Mutex
	orders           map[uuid.UUID]*models.CommerceOrder
	checkoutByKey    map[string]uuid.UUID
	eventByKey       map[string]string
	checkoutTemplate *models.CommerceOrder
	checkoutErr      error
	checkoutInput    repository.CommerceCheckoutInput
	assignedUserID   *uuid.UUID
	listFilter       repository.CommerceOrderListFilter
}

func (s *commerceOrderRepoStub) CheckoutCart(_ context.Context, input repository.CommerceCheckoutInput) (*models.CommerceOrder, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkoutInput = input
	if s.checkoutErr != nil {
		return nil, false, s.checkoutErr
	}
	s.initialize()
	key := input.OrganizationID.String() + ":" + input.CheckoutKey
	if orderID, exists := s.checkoutByKey[key]; exists {
		return cloneCommerceOrder(s.orders[orderID]), false, nil
	}
	order := &models.CommerceOrder{
		ID: input.OrderID, OrganizationID: input.OrganizationID, CartID: input.CartID,
		OrderNumber: input.OrderNumber, CheckoutKey: input.CheckoutKey, FulfilmentMode: input.FulfilmentMode,
		Status: models.CommerceOrderStatusPendingPayment, Currency: "NGN", Version: 1,
		PaymentExpiresAt: input.PaymentExpiresAt, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if s.checkoutTemplate != nil {
		template := cloneCommerceOrder(s.checkoutTemplate)
		template.ID = input.OrderID
		template.OrganizationID = input.OrganizationID
		template.CartID = input.CartID
		template.OrderNumber = input.OrderNumber
		template.CheckoutKey = input.CheckoutKey
		template.FulfilmentMode = input.FulfilmentMode
		template.Status = models.CommerceOrderStatusPendingPayment
		template.PaymentExpiresAt = input.PaymentExpiresAt
		order = template
	}
	s.orders[order.ID] = cloneCommerceOrder(order)
	s.checkoutByKey[key] = order.ID
	return cloneCommerceOrder(order), true, nil
}

func (s *commerceOrderRepoStub) GetOrder(_ context.Context, organizationID, orderID uuid.UUID) (*models.CommerceOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initialize()
	order := s.orders[orderID]
	if order == nil || order.OrganizationID != organizationID {
		return nil, repository.ErrCommerceNotFound
	}
	return cloneCommerceOrder(order), nil
}

func (s *commerceOrderRepoStub) GetOrderByCheckoutKey(_ context.Context, organizationID uuid.UUID, checkoutKey string) (*models.CommerceOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initialize()
	orderID, exists := s.checkoutByKey[organizationID.String()+":"+checkoutKey]
	if !exists {
		return nil, repository.ErrCommerceNotFound
	}
	return cloneCommerceOrder(s.orders[orderID]), nil
}

func (s *commerceOrderRepoStub) GetOrderByNumber(_ context.Context, organizationID uuid.UUID, orderNumber string) (*models.CommerceOrder, error) {
	for _, order := range s.orders {
		if order.OrganizationID == organizationID && order.OrderNumber == orderNumber {
			copy := *order
			return &copy, nil
		}
	}
	return nil, repository.ErrCommerceNotFound
}

func (s *commerceOrderRepoStub) SetOrderDestination(_ context.Context, organizationID, customerID, orderID uuid.UUID, address string, latitude, longitude *float64) (*models.CommerceOrder, error) {
	order, err := s.GetOrder(context.Background(), organizationID, orderID)
	if err != nil || order.CustomerID != customerID {
		return nil, repository.ErrCommerceNotFound
	}
	order.DestinationAddress = &address
	order.DestinationLatitude = latitude
	order.DestinationLongitude = longitude
	return order, nil
}

func (s *commerceOrderRepoStub) ListOrders(_ context.Context, organizationID uuid.UUID, assignedUserID *uuid.UUID, filter repository.CommerceOrderListFilter) ([]models.CommerceOrder, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assignedUserID = assignedUserID
	s.listFilter = filter
	s.initialize()
	orders := make([]models.CommerceOrder, 0)
	for _, order := range s.orders {
		if order.OrganizationID == organizationID {
			orders = append(orders, *cloneCommerceOrder(order))
		}
	}
	return orders, int64(len(orders)), nil
}

func (s *commerceOrderRepoStub) TransitionOrder(_ context.Context, input repository.CommerceOrderTransitionInput) (*models.CommerceOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initialize()
	order := s.orders[input.OrderID]
	if order == nil || order.OrganizationID != input.OrganizationID {
		return nil, repository.ErrCommerceNotFound
	}
	eventKey := input.OrganizationID.String() + ":" + input.OrderID.String() + ":" + input.IdempotencyKey
	if target, exists := s.eventByKey[eventKey]; exists {
		if target != input.ToStatus {
			return nil, repository.ErrCommerceConflict
		}
		return cloneCommerceOrder(order), nil
	}
	if !input.Allowed || order.Status != input.FromStatus || input.FromStatus == input.ToStatus {
		return nil, repository.ErrCommerceOrderTransition
	}
	fromStatus := order.Status
	order.Status = input.ToStatus
	order.Version++
	order.Events = append(order.Events, models.CommerceOrderEvent{
		ID: uuid.New(), OrganizationID: order.OrganizationID, OrderID: order.ID, EventType: input.EventType,
		FromStatus: &fromStatus, ToStatus: input.ToStatus, ActorType: input.ActorType,
		ActorUserID: input.ActorUserID, Reason: input.Reason, IdempotencyKey: input.IdempotencyKey, CreatedAt: time.Now().UTC(),
	})
	s.eventByKey[eventKey] = input.ToStatus
	return cloneCommerceOrder(order), nil
}

func (s *commerceOrderRepoStub) initialize() {
	if s.orders == nil {
		s.orders = make(map[uuid.UUID]*models.CommerceOrder)
	}
	if s.checkoutByKey == nil {
		s.checkoutByKey = make(map[string]uuid.UUID)
	}
	if s.eventByKey == nil {
		s.eventByKey = make(map[string]string)
	}
}

func TestCommerceCheckoutCreatesPendingOrderWithServerOwnedValues(t *testing.T) {
	organizationID, customerID, storeID := uuid.New(), uuid.New(), uuid.New()
	cartRepo, cart := seededActiveCommerceCart(t, organizationID, customerID, storeID)
	orderRepo := &commerceOrderRepoStub{checkoutTemplate: &models.CommerceOrder{
		CustomerID: customerID, StoreID: storeID, Currency: "NGN", SubtotalMinor: 840000, TotalMinor: 840000,
		Items: []models.CommerceOrderItem{{ID: uuid.New(), ProductID: uuid.New(), VariantID: uuid.New(), Quantity: 2, UnitPriceMinor: 420000, LineTotalMinor: 840000}},
	}}
	foundationRepo := &commerceFoundationRepoStub{storeModes: []models.CommerceStoreFulfilmentMode{{Mode: models.FulfilmentModeCustomerPickup, Enabled: true}}}
	service := NewCommerceOrderService(orderRepo, seededCommerceCustomerRepo(organizationID, customerID), cartRepo, foundationRepo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}
	before := time.Now().UTC()

	order, created, err := service.CheckoutCart(context.Background(), actor, nil, CheckoutCommerceCartInput{
		CartID: cart.ID, FulfilmentMode: models.FulfilmentModeCustomerPickup, IdempotencyKey: "checkout-request-001",
	})
	if err != nil || !created {
		t.Fatalf("checkout: created=%v err=%v", created, err)
	}
	if order.Status != models.CommerceOrderStatusPendingPayment || order.TotalMinor != 840000 || order.Items[0].UnitPriceMinor != 420000 {
		t.Fatalf("checkout did not return the authoritative order snapshot: %+v", order)
	}
	if !orderRepo.checkoutInput.PaymentExpiresAt.After(before.Add(29*time.Minute)) || orderRepo.checkoutInput.ActorUserID == nil || *orderRepo.checkoutInput.ActorUserID != actor.UserID {
		t.Fatalf("checkout did not assign the server reservation deadline and actor: %+v", orderRepo.checkoutInput)
	}
	if orderRepo.checkoutInput.OrderNumber == "" || orderRepo.checkoutInput.CheckoutKey != "checkout-request-001" {
		t.Fatalf("checkout did not assign server order identity: %+v", orderRepo.checkoutInput)
	}
}

func TestCommerceCheckoutRetryResolvesAfterCartConversion(t *testing.T) {
	organizationID, storeID, cartID := uuid.New(), uuid.New(), uuid.New()
	orderID := uuid.New()
	orderRepo := &commerceOrderRepoStub{
		orders: map[uuid.UUID]*models.CommerceOrder{orderID: {
			ID: orderID, OrganizationID: organizationID, CartID: cartID, StoreID: storeID,
			CheckoutKey: "checkout-request-002", FulfilmentMode: models.FulfilmentModeCustomerPickup,
		}},
		checkoutByKey: map[string]uuid.UUID{organizationID.String() + ":checkout-request-002": orderID},
	}
	service := NewCommerceOrderService(orderRepo, &commerceCustomerRepoStub{}, &commerceCartRepoStub{}, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}

	order, created, err := service.CheckoutCart(context.Background(), actor, nil, CheckoutCommerceCartInput{
		CartID: cartID, FulfilmentMode: models.FulfilmentModeCustomerPickup, IdempotencyKey: "checkout-request-002",
	})
	if err != nil || created || order.ID != orderID {
		t.Fatalf("converted cart retry was not idempotent: order=%+v created=%v err=%v", order, created, err)
	}
}

func TestConcurrentCommerceCheckoutRetriesCreateOneOrder(t *testing.T) {
	organizationID, customerID, storeID := uuid.New(), uuid.New(), uuid.New()
	cartRepo, cart := seededActiveCommerceCart(t, organizationID, customerID, storeID)
	orderRepo := &commerceOrderRepoStub{}
	foundationRepo := &commerceFoundationRepoStub{storeModes: []models.CommerceStoreFulfilmentMode{{Mode: models.FulfilmentModeCustomerPickup, Enabled: true}}}
	service := NewCommerceOrderService(orderRepo, seededCommerceCustomerRepo(organizationID, customerID), cartRepo, foundationRepo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}

	type result struct {
		order   *models.CommerceOrder
		created bool
		err     error
	}
	results := make(chan result, 2)
	var waitGroup sync.WaitGroup
	for range 2 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			order, created, err := service.CheckoutCart(context.Background(), actor, nil, CheckoutCommerceCartInput{
				CartID: cart.ID, FulfilmentMode: models.FulfilmentModeCustomerPickup, IdempotencyKey: "checkout-request-concurrent",
			})
			results <- result{order: order, created: created, err: err}
		}()
	}
	waitGroup.Wait()
	close(results)

	createdCount := 0
	var orderID uuid.UUID
	for item := range results {
		if item.err != nil {
			t.Fatalf("concurrent checkout failed: %v", item.err)
		}
		if item.created {
			createdCount++
		}
		if orderID == uuid.Nil {
			orderID = item.order.ID
		} else if item.order.ID != orderID {
			t.Fatalf("checkout retries returned different orders: %s and %s", orderID, item.order.ID)
		}
	}
	if createdCount != 1 {
		t.Fatalf("expected exactly one created checkout, got %d", createdCount)
	}
}

func TestCommerceCheckoutFailureLeavesCartActive(t *testing.T) {
	organizationID, customerID, storeID := uuid.New(), uuid.New(), uuid.New()
	cartRepo, cart := seededActiveCommerceCart(t, organizationID, customerID, storeID)
	orderRepo := &commerceOrderRepoStub{checkoutErr: repository.ErrCommerceInventoryUnavailable}
	foundationRepo := &commerceFoundationRepoStub{storeModes: []models.CommerceStoreFulfilmentMode{{Mode: models.FulfilmentModeCustomerPickup, Enabled: true}}}
	service := NewCommerceOrderService(orderRepo, seededCommerceCustomerRepo(organizationID, customerID), cartRepo, foundationRepo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}

	_, _, err := service.CheckoutCart(context.Background(), actor, nil, CheckoutCommerceCartInput{
		CartID: cart.ID, FulfilmentMode: models.FulfilmentModeCustomerPickup, IdempotencyKey: "checkout-request-003",
	})
	if !errors.Is(err, repository.ErrCommerceInventoryUnavailable) {
		t.Fatalf("expected inventory failure, got %v", err)
	}
	active, activeErr := cartRepo.GetActiveCart(context.Background(), organizationID, cart.ID)
	if activeErr != nil || active.Status != models.CommerceCartStatusActive {
		t.Fatalf("failed checkout mutated the cart: cart=%+v err=%v", active, activeErr)
	}
}

func TestCommerceCheckoutRejectsCrossTenantTarget(t *testing.T) {
	organizationID := uuid.New()
	service := NewCommerceOrderService(&commerceOrderRepoStub{}, &commerceCustomerRepoStub{}, &commerceCartRepoStub{}, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}
	otherOrganizationID := uuid.New()

	_, _, err := service.CheckoutCart(context.Background(), actor, &otherOrganizationID, CheckoutCommerceCartInput{
		CartID: uuid.New(), FulfilmentMode: models.FulfilmentModeCustomerPickup, IdempotencyKey: "checkout-request-004",
	})
	if !errors.Is(err, ErrCommerceForbidden) {
		t.Fatalf("expected cross-tenant checkout to be forbidden, got %v", err)
	}
}

func TestStoreStaffCheckoutUsesAssignedStoreScope(t *testing.T) {
	organizationID, customerID, storeID := uuid.New(), uuid.New(), uuid.New()
	cartRepo, cart := seededActiveCommerceCart(t, organizationID, customerID, storeID)
	foundationRepo := &commerceFoundationRepoStub{storeModes: []models.CommerceStoreFulfilmentMode{{Mode: models.FulfilmentModeCustomerPickup, Enabled: true}}}
	service := NewCommerceOrderService(&commerceOrderRepoStub{}, seededCommerceCustomerRepo(organizationID, customerID), cartRepo, foundationRepo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleStoreStaff}

	if _, _, err := service.CheckoutCart(context.Background(), actor, nil, CheckoutCommerceCartInput{
		CartID: cart.ID, FulfilmentMode: models.FulfilmentModeCustomerPickup, IdempotencyKey: "checkout-request-005",
	}); err != nil {
		t.Fatal(err)
	}
	if foundationRepo.listAssignedUserID == nil || *foundationRepo.listAssignedUserID != actor.UserID {
		t.Fatal("store checkout access was not scoped to the user's store assignment")
	}
}

func TestCommerceOrderTransitionGraph(t *testing.T) {
	tests := []struct {
		name, from, to, mode string
		valid                bool
	}{
		{"payment confirms", "pending_payment", "paid", "customer_pickup", true},
		{"cannot skip payment", "pending_payment", "processing", "customer_pickup", false},
		{"paid processes", "paid", "processing", "customer_pickup", true},
		{"pickup becomes ready for pickup", "ready", "ready_for_pickup", "customer_pickup", true},
		{"pickup cannot enter delivery", "ready", "fulfilment_pending", "customer_pickup", false},
		{"delivery enters fulfilment", "ready", "fulfilment_pending", "merchant_rider", true},
		{"delivery cannot become pickup", "ready", "ready_for_pickup", "merchant_rider", false},
		{"delivery completes in order", "out_for_delivery", "delivered", "merchant_rider", true},
		{"completed cannot restart", "completed", "processing", "customer_pickup", false},
		{"terminal cancellation", "cancelled", "pending_payment", "customer_pickup", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isValidCommerceOrderTransition(test.from, test.to, test.mode); got != test.valid {
				t.Fatalf("transition %s -> %s validity=%v, want %v", test.from, test.to, got, test.valid)
			}
		})
	}
}

func TestStoreStaffCannotSetPaymentOrCancellationStatus(t *testing.T) {
	organizationID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	orderRepo := seededCommerceOrderRepo(&models.CommerceOrder{
		ID: orderID, OrganizationID: organizationID, StoreID: storeID, Status: models.CommerceOrderStatusPendingPayment, FulfilmentMode: models.FulfilmentModeCustomerPickup,
	})
	service := NewCommerceOrderService(orderRepo, nil, nil, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleStoreStaff}

	for _, status := range []string{models.CommerceOrderStatusPaid, models.CommerceOrderStatusCancelled} {
		_, err := service.TransitionOrder(context.Background(), actor, nil, orderID, TransitionCommerceOrderInput{Status: status, Reason: "test", IdempotencyKey: "transition-" + status})
		if !errors.Is(err, ErrCommerceForbidden) {
			t.Fatalf("expected %s to be forbidden for store staff, got %v", status, err)
		}
	}
}

func TestCommerceOrderTransitionRetryRemainsIdempotentAfterAdvancement(t *testing.T) {
	organizationID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	orderRepo := seededCommerceOrderRepo(&models.CommerceOrder{
		ID: orderID, OrganizationID: organizationID, StoreID: storeID, Status: models.CommerceOrderStatusPaid, FulfilmentMode: models.FulfilmentModeCustomerPickup, Version: 1,
	})
	service := NewCommerceOrderService(orderRepo, nil, nil, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleStoreStaff}

	if _, err := service.TransitionOrder(context.Background(), actor, nil, orderID, TransitionCommerceOrderInput{Status: models.CommerceOrderStatusProcessing, IdempotencyKey: "transition-processing"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.TransitionOrder(context.Background(), actor, nil, orderID, TransitionCommerceOrderInput{Status: models.CommerceOrderStatusReady, IdempotencyKey: "transition-ready"}); err != nil {
		t.Fatal(err)
	}
	order, err := service.TransitionOrder(context.Background(), actor, nil, orderID, TransitionCommerceOrderInput{Status: models.CommerceOrderStatusProcessing, IdempotencyKey: "transition-processing"})
	if err != nil || order.Status != models.CommerceOrderStatusReady || len(order.Events) != 2 {
		t.Fatalf("transition retry was not idempotent after advancement: order=%+v err=%v", order, err)
	}
}

func TestStoreOrderListUsesAssignedStoreScope(t *testing.T) {
	orderRepo := &commerceOrderRepoStub{}
	service := NewCommerceOrderService(orderRepo, nil, nil, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: uuid.New(), Role: utils.RoleStoreManager}

	if _, _, err := service.ListOrders(context.Background(), actor, nil, CommerceOrderListInput{Limit: 20}); err != nil {
		t.Fatal(err)
	}
	if orderRepo.assignedUserID == nil || *orderRepo.assignedUserID != actor.UserID || orderRepo.listFilter.Limit != 20 {
		t.Fatalf("order list was not scoped to assigned stores: user=%v filter=%+v", orderRepo.assignedUserID, orderRepo.listFilter)
	}
}

func seededActiveCommerceCart(t *testing.T, organizationID, customerID, storeID uuid.UUID) (*commerceCartRepoStub, *models.CommerceCart) {
	t.Helper()
	repo := &commerceCartRepoStub{}
	cart, _, err := repo.GetOrCreateActiveCart(context.Background(), &models.CommerceCart{
		ID: uuid.New(), OrganizationID: organizationID, CustomerID: customerID, StoreID: storeID,
		Currency: "NGN", Status: models.CommerceCartStatusActive, Version: 1, ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, cart
}

func seededCommerceOrderRepo(order *models.CommerceOrder) *commerceOrderRepoStub {
	return &commerceOrderRepoStub{orders: map[uuid.UUID]*models.CommerceOrder{order.ID: cloneCommerceOrder(order)}}
}

func cloneCommerceOrder(order *models.CommerceOrder) *models.CommerceOrder {
	copy := *order
	copy.Items = append([]models.CommerceOrderItem(nil), order.Items...)
	copy.Events = append([]models.CommerceOrderEvent(nil), order.Events...)
	return &copy
}
