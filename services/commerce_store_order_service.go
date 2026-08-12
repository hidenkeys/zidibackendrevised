package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

var ErrCommerceStoreOrderState = errors.New("order cannot be prepared from its current state")

var commerceOperationalOrderStatuses = []string{
	models.CommerceOrderStatusPendingPayment,
	models.CommerceOrderStatusPaid,
	models.CommerceOrderStatusProcessing,
	models.CommerceOrderStatusReady,
	models.CommerceOrderStatusFulfilmentPending,
	models.CommerceOrderStatusReadyForPickup,
	models.CommerceOrderStatusOutForDelivery,
	models.CommerceOrderStatusDelivered,
}

type CommerceStoreOrderListInput struct {
	StoreID *uuid.UUID
	Search  string
	Limit   int
	Offset  int
}

type PrepareCommerceStoreOrderInput struct {
	IdempotencyKey string
}

type CommerceStoreOrderView struct {
	Order      *models.CommerceOrder
	Fulfilment *models.CommerceFulfilment
}

type commerceStoreOrderOperations interface {
	GetOrder(context.Context, CommerceActor, *uuid.UUID, uuid.UUID) (*models.CommerceOrder, error)
	ListOrders(context.Context, CommerceActor, *uuid.UUID, CommerceOrderListInput) ([]models.CommerceOrder, int64, error)
	TransitionOrder(context.Context, CommerceActor, *uuid.UUID, uuid.UUID, TransitionCommerceOrderInput) (*models.CommerceOrder, error)
}

type commerceStoreFulfilmentOperations interface {
	StartFulfilment(context.Context, CommerceActor, *uuid.UUID, uuid.UUID, StartCommerceFulfilmentInput) (*models.CommerceFulfilment, bool, error)
	GetOrderFulfilment(context.Context, CommerceActor, *uuid.UUID, uuid.UUID) (*models.CommerceFulfilment, error)
	ListOrderFulfilments(context.Context, CommerceActor, *uuid.UUID, []models.CommerceOrder) (map[uuid.UUID]*models.CommerceFulfilment, error)
}

type CommerceStoreOrderService struct {
	orders      commerceStoreOrderOperations
	fulfilments commerceStoreFulfilmentOperations
}

func NewCommerceStoreOrderService(orders commerceStoreOrderOperations, fulfilments commerceStoreFulfilmentOperations) *CommerceStoreOrderService {
	return &CommerceStoreOrderService{orders: orders, fulfilments: fulfilments}
}

func (s *CommerceStoreOrderService) ListOperationalOrders(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, input CommerceStoreOrderListInput) ([]CommerceStoreOrderView, int64, error) {
	orders, total, err := s.orders.ListOrders(ctx, actor, requestedOrganizationID, CommerceOrderListInput{
		StoreID:  input.StoreID,
		Statuses: commerceOperationalOrderStatuses,
		Search:   input.Search,
		Limit:    input.Limit,
		Offset:   input.Offset,
	})
	if err != nil {
		return nil, 0, err
	}

	fulfilments, err := s.fulfilments.ListOrderFulfilments(ctx, actor, requestedOrganizationID, orders)
	if err != nil {
		return nil, 0, err
	}
	views := make([]CommerceStoreOrderView, 0, len(orders))
	for i := range orders {
		views = append(views, CommerceStoreOrderView{Order: &orders[i], Fulfilment: fulfilments[orders[i].ID]})
	}
	return views, total, nil
}

func (s *CommerceStoreOrderService) MarkPrepared(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, orderID uuid.UUID, input PrepareCommerceStoreOrderInput) (*CommerceStoreOrderView, error) {
	key := strings.TrimSpace(input.IdempotencyKey)
	if orderID == uuid.Nil || len(key) < 8 || len(key) > 170 {
		return nil, fmt.Errorf("%w: order and an idempotency key between 8 and 170 characters are required", ErrCommerceValidation)
	}

	order, err := s.orders.GetOrder(ctx, actor, requestedOrganizationID, orderID)
	if err != nil {
		return nil, err
	}

	switch order.Status {
	case models.CommerceOrderStatusPaid:
		order, err = s.orders.TransitionOrder(ctx, actor, requestedOrganizationID, orderID, TransitionCommerceOrderInput{
			Status: models.CommerceOrderStatusProcessing, IdempotencyKey: key + ":processing",
		})
		if err != nil {
			return nil, err
		}
		fallthrough
	case models.CommerceOrderStatusProcessing:
		order, err = s.orders.TransitionOrder(ctx, actor, requestedOrganizationID, orderID, TransitionCommerceOrderInput{
			Status: models.CommerceOrderStatusReady, IdempotencyKey: key + ":ready",
		})
		if err != nil {
			return nil, err
		}
		fallthrough
	case models.CommerceOrderStatusReady:
		fulfilment, _, startErr := s.fulfilments.StartFulfilment(ctx, actor, requestedOrganizationID, orderID, StartCommerceFulfilmentInput{
			IdempotencyKey: key + ":fulfilment",
		})
		if startErr != nil {
			return nil, startErr
		}
		return s.storeOrderView(ctx, actor, requestedOrganizationID, orderID, fulfilment)
	case models.CommerceOrderStatusFulfilmentPending, models.CommerceOrderStatusReadyForPickup:
		fulfilment, fulfilmentErr := s.fulfilments.GetOrderFulfilment(ctx, actor, requestedOrganizationID, orderID)
		if fulfilmentErr != nil {
			return nil, fulfilmentErr
		}
		return s.storeOrderView(ctx, actor, requestedOrganizationID, orderID, fulfilment)
	default:
		return nil, fmt.Errorf("%w: %s", ErrCommerceStoreOrderState, order.Status)
	}
}

func (s *CommerceStoreOrderService) storeOrderView(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, orderID uuid.UUID, fulfilment *models.CommerceFulfilment) (*CommerceStoreOrderView, error) {
	order, err := s.orders.GetOrder(ctx, actor, requestedOrganizationID, orderID)
	if err != nil {
		return nil, err
	}
	return &CommerceStoreOrderView{Order: order, Fulfilment: fulfilment}, nil
}
