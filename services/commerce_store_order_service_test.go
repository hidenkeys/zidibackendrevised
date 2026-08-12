package services

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/utils"
)

type commerceStoreOrderOperationsStub struct {
	order       *models.CommerceOrder
	listInput   CommerceOrderListInput
	transitions []TransitionCommerceOrderInput
}

func (s *commerceStoreOrderOperationsStub) GetOrder(_ context.Context, _ CommerceActor, _ *uuid.UUID, orderID uuid.UUID) (*models.CommerceOrder, error) {
	if s.order == nil || s.order.ID != orderID {
		return nil, repository.ErrCommerceNotFound
	}
	return cloneCommerceOrder(s.order), nil
}

func (s *commerceStoreOrderOperationsStub) ListOrders(_ context.Context, _ CommerceActor, _ *uuid.UUID, input CommerceOrderListInput) ([]models.CommerceOrder, int64, error) {
	s.listInput = input
	if s.order == nil {
		return nil, 0, nil
	}
	return []models.CommerceOrder{*cloneCommerceOrder(s.order)}, 1, nil
}

func (s *commerceStoreOrderOperationsStub) TransitionOrder(_ context.Context, _ CommerceActor, _ *uuid.UUID, orderID uuid.UUID, input TransitionCommerceOrderInput) (*models.CommerceOrder, error) {
	if s.order == nil || s.order.ID != orderID {
		return nil, repository.ErrCommerceNotFound
	}
	if !isValidCommerceOrderTransition(s.order.Status, input.Status, s.order.FulfilmentMode) {
		return nil, repository.ErrCommerceOrderTransition
	}
	s.transitions = append(s.transitions, input)
	s.order.Status = input.Status
	s.order.Version++
	return cloneCommerceOrder(s.order), nil
}

type commerceStoreFulfilmentOperationsStub struct {
	orders     *commerceStoreOrderOperationsStub
	fulfilment *models.CommerceFulfilment
	starts     []StartCommerceFulfilmentInput
}

func (s *commerceStoreFulfilmentOperationsStub) StartFulfilment(_ context.Context, _ CommerceActor, _ *uuid.UUID, orderID uuid.UUID, input StartCommerceFulfilmentInput) (*models.CommerceFulfilment, bool, error) {
	if s.orders.order == nil || s.orders.order.ID != orderID {
		return nil, false, repository.ErrCommerceNotFound
	}
	if s.orders.order.Status != models.CommerceOrderStatusReady {
		return nil, false, repository.ErrCommerceFulfilmentState
	}
	s.starts = append(s.starts, input)
	status := models.CommerceFulfilmentStatusReadyForPickup
	orderStatus := models.CommerceOrderStatusReadyForPickup
	if s.orders.order.FulfilmentMode != models.FulfilmentModeCustomerPickup {
		status = models.CommerceFulfilmentStatusAwaitingQuote
		orderStatus = models.CommerceOrderStatusFulfilmentPending
	}
	s.orders.order.Status = orderStatus
	s.fulfilment = &models.CommerceFulfilment{
		ID: uuid.New(), OrganizationID: s.orders.order.OrganizationID, OrderID: orderID,
		StoreID: s.orders.order.StoreID, CustomerID: s.orders.order.CustomerID,
		Mode: s.orders.order.FulfilmentMode, Status: status,
	}
	return s.fulfilment, true, nil
}

func (s *commerceStoreFulfilmentOperationsStub) GetOrderFulfilment(_ context.Context, _ CommerceActor, _ *uuid.UUID, orderID uuid.UUID) (*models.CommerceFulfilment, error) {
	if s.fulfilment == nil || s.fulfilment.OrderID != orderID {
		return nil, repository.ErrCommerceNotFound
	}
	return s.fulfilment, nil
}

func (s *commerceStoreFulfilmentOperationsStub) ListOrderFulfilments(_ context.Context, _ CommerceActor, _ *uuid.UUID, orders []models.CommerceOrder) (map[uuid.UUID]*models.CommerceFulfilment, error) {
	items := make(map[uuid.UUID]*models.CommerceFulfilment)
	if s.fulfilment == nil {
		return items, nil
	}
	for _, order := range orders {
		if order.ID == s.fulfilment.OrderID {
			items[order.ID] = s.fulfilment
		}
	}
	return items, nil
}

func TestCommerceStoreOrderMarkPreparedRunsOwnedWorkflow(t *testing.T) {
	organizationID := uuid.New()
	orderOperations := &commerceStoreOrderOperationsStub{order: &models.CommerceOrder{
		ID: uuid.New(), OrganizationID: organizationID, StoreID: uuid.New(), CustomerID: uuid.New(),
		Status: models.CommerceOrderStatusPaid, FulfilmentMode: models.FulfilmentModeCustomerPickup,
		CreatedAt: time.Now().UTC(),
	}}
	fulfilmentOperations := &commerceStoreFulfilmentOperationsStub{orders: orderOperations}
	service := NewCommerceStoreOrderService(orderOperations, fulfilmentOperations)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleStoreStaff}

	view, err := service.MarkPrepared(context.Background(), actor, nil, orderOperations.order.ID, PrepareCommerceStoreOrderInput{IdempotencyKey: "prepare-request-001"})
	if err != nil {
		t.Fatalf("mark prepared: %v", err)
	}
	if view.Order.Status != models.CommerceOrderStatusReadyForPickup || view.Fulfilment == nil {
		t.Fatalf("expected ready pickup order and fulfilment, got status=%s fulfilment=%v", view.Order.Status, view.Fulfilment)
	}
	if len(orderOperations.transitions) != 2 || orderOperations.transitions[0].Status != models.CommerceOrderStatusProcessing || orderOperations.transitions[1].Status != models.CommerceOrderStatusReady {
		t.Fatalf("unexpected transitions: %+v", orderOperations.transitions)
	}
	if len(fulfilmentOperations.starts) != 1 || fulfilmentOperations.starts[0].IdempotencyKey != "prepare-request-001:fulfilment" {
		t.Fatalf("unexpected fulfilment starts: %+v", fulfilmentOperations.starts)
	}
}

func TestCommerceStoreOrderMarkPreparedRetryReturnsExistingFulfilment(t *testing.T) {
	organizationID := uuid.New()
	orderOperations := &commerceStoreOrderOperationsStub{order: &models.CommerceOrder{
		ID: uuid.New(), OrganizationID: organizationID, StoreID: uuid.New(), CustomerID: uuid.New(),
		Status: models.CommerceOrderStatusPaid, FulfilmentMode: models.FulfilmentModeCustomerPickup,
	}}
	fulfilmentOperations := &commerceStoreFulfilmentOperationsStub{orders: orderOperations}
	service := NewCommerceStoreOrderService(orderOperations, fulfilmentOperations)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleStoreStaff}
	input := PrepareCommerceStoreOrderInput{IdempotencyKey: "prepare-request-002"}

	first, err := service.MarkPrepared(context.Background(), actor, nil, orderOperations.order.ID, input)
	if err != nil {
		t.Fatalf("first mark prepared: %v", err)
	}
	second, err := service.MarkPrepared(context.Background(), actor, nil, orderOperations.order.ID, input)
	if err != nil {
		t.Fatalf("retry mark prepared: %v", err)
	}
	if second.Fulfilment == nil || second.Fulfilment.ID != first.Fulfilment.ID || len(orderOperations.transitions) != 2 || len(fulfilmentOperations.starts) != 1 {
		t.Fatalf("retry was not idempotent: first=%+v second=%+v transitions=%d starts=%d", first, second, len(orderOperations.transitions), len(fulfilmentOperations.starts))
	}
}

func TestCommerceStoreOrderMarkPreparedRejectsInvalidState(t *testing.T) {
	organizationID := uuid.New()
	orderOperations := &commerceStoreOrderOperationsStub{order: &models.CommerceOrder{
		ID: uuid.New(), OrganizationID: organizationID, Status: models.CommerceOrderStatusDelivered,
		FulfilmentMode: models.FulfilmentModeMerchantRider,
	}}
	fulfilmentOperations := &commerceStoreFulfilmentOperationsStub{orders: orderOperations}
	service := NewCommerceStoreOrderService(orderOperations, fulfilmentOperations)

	_, err := service.MarkPrepared(context.Background(), CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleStoreStaff}, nil, orderOperations.order.ID, PrepareCommerceStoreOrderInput{IdempotencyKey: "prepare-request-003"})
	if !errors.Is(err, ErrCommerceStoreOrderState) {
		t.Fatalf("expected invalid store order state, got %v", err)
	}
	if len(orderOperations.transitions) != 0 || len(fulfilmentOperations.starts) != 0 {
		t.Fatalf("invalid state mutated order: transitions=%d starts=%d", len(orderOperations.transitions), len(fulfilmentOperations.starts))
	}
}

func TestCommerceStoreOrderListUsesOperationalStatuses(t *testing.T) {
	organizationID := uuid.New()
	orderOperations := &commerceStoreOrderOperationsStub{order: &models.CommerceOrder{
		ID: uuid.New(), OrganizationID: organizationID, StoreID: uuid.New(), Status: models.CommerceOrderStatusPaid,
	}}
	service := NewCommerceStoreOrderService(orderOperations, &commerceStoreFulfilmentOperationsStub{orders: orderOperations})
	storeID := orderOperations.order.StoreID

	views, total, err := service.ListOperationalOrders(context.Background(), CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleStoreStaff}, nil, CommerceStoreOrderListInput{StoreID: &storeID, Limit: 25})
	if err != nil || total != 1 || len(views) != 1 {
		t.Fatalf("list operational orders: total=%d views=%d err=%v", total, len(views), err)
	}
	if !reflect.DeepEqual(orderOperations.listInput.Statuses, commerceOperationalOrderStatuses) || orderOperations.listInput.StoreID == nil || *orderOperations.listInput.StoreID != storeID {
		t.Fatalf("unexpected operational filter: %+v", orderOperations.listInput)
	}
}
