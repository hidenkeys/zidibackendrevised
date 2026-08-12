package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/services"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s Server) CheckoutCommerceCart(c *fiber.Ctx) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.CheckoutCommerceCartJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	order, created, err := s.commerceOrderService.CheckoutCart(c.UserContext(), actor, request.OrganizationId, services.CheckoutCommerceCartInput{
		CartID:         request.CartId,
		FulfilmentMode: string(request.FulfilmentMode),
		IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return commerceError(c, err)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return c.Status(status).JSON(commerceOrderResponse(order))
}

func (s Server) ListCommerceOrders(c *fiber.Ctx, params api.ListCommerceOrdersParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	limit, offset := 50, 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var status *string
	if params.Status != nil {
		value := string(*params.Status)
		status = &value
	}
	orders, total, err := s.commerceOrderService.ListOrders(c.UserContext(), actor, params.OrganizationId, services.CommerceOrderListInput{
		StoreID: params.StoreId, CustomerID: params.CustomerId, Status: status, Limit: limit, Offset: offset,
	})
	if err != nil {
		return commerceError(c, err)
	}
	items := make([]api.CommerceOrder, 0, len(orders))
	for index := range orders {
		items = append(items, commerceOrderResponse(&orders[index]))
	}
	return c.JSON(api.CommerceOrderList{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s Server) GetCommerceOrder(c *fiber.Ctx, orderID uuid.UUID, params api.GetCommerceOrderParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	order, err := s.commerceOrderService.GetOrder(c.UserContext(), actor, params.OrganizationId, orderID)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceOrderResponse(order))
}

func (s Server) TransitionCommerceOrder(c *fiber.Ctx, orderID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.TransitionCommerceOrderJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	order, err := s.commerceOrderService.TransitionOrder(c.UserContext(), actor, request.OrganizationId, orderID, services.TransitionCommerceOrderInput{
		Status:         string(request.Status),
		Reason:         optionalString(request.Reason),
		IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceOrderResponse(order))
}

func commerceOrderResponse(order *models.CommerceOrder) api.CommerceOrder {
	items := make([]api.CommerceOrderItem, 0, len(order.Items))
	for _, item := range order.Items {
		attributes := map[string]string{}
		_ = json.Unmarshal(item.Attributes, &attributes)
		items = append(items, api.CommerceOrderItem{
			Id:              item.ID,
			ProductId:       item.ProductID,
			VariantId:       item.VariantID,
			ProductName:     item.ProductName,
			VariantName:     item.VariantName,
			Sku:             item.SKU,
			Attributes:      attributes,
			PrimaryImageUrl: item.PrimaryImageURL,
			Quantity:        item.Quantity,
			UnitPriceMinor:  item.UnitPriceMinor,
			LineTotalMinor:  item.LineTotalMinor,
			CreatedAt:       item.CreatedAt,
		})
	}
	events := make([]api.CommerceOrderEvent, 0, len(order.Events))
	for _, event := range order.Events {
		metadata := map[string]interface{}{}
		_ = json.Unmarshal(event.Metadata, &metadata)
		var fromStatus *api.CommerceOrderStatus
		if event.FromStatus != nil {
			value := api.CommerceOrderStatus(*event.FromStatus)
			fromStatus = &value
		}
		events = append(events, api.CommerceOrderEvent{
			Id:          event.ID,
			EventType:   event.EventType,
			FromStatus:  fromStatus,
			ToStatus:    api.CommerceOrderStatus(event.ToStatus),
			ActorType:   api.CommerceOrderEventActorType(event.ActorType),
			ActorUserId: event.ActorUserID,
			Reason:      event.Reason,
			Metadata:    metadata,
			CreatedAt:   event.CreatedAt,
		})
	}
	return api.CommerceOrder{
		Id:                   order.ID,
		OrganizationId:       order.OrganizationID,
		CartId:               order.CartID,
		CustomerId:           order.CustomerID,
		StoreId:              order.StoreID,
		OrderNumber:          order.OrderNumber,
		CustomerName:         order.CustomerName,
		CustomerPhone:        order.CustomerPhone,
		CustomerEmail:        commerceEmailResponse(order.CustomerEmail),
		FulfilmentMode:       api.CommerceOrderFulfilmentMode(order.FulfilmentMode),
		DestinationAddress:   order.DestinationAddress,
		DestinationLatitude:  order.DestinationLatitude,
		DestinationLongitude: order.DestinationLongitude,
		Status:               api.CommerceOrderStatus(order.Status),
		Currency:             order.Currency,
		SubtotalMinor:        order.SubtotalMinor,
		DiscountMinor:        order.DiscountMinor,
		DeliveryFeeMinor:     order.DeliveryFeeMinor,
		TotalMinor:           order.TotalMinor,
		Version:              order.Version,
		PaymentExpiresAt:     order.PaymentExpiresAt,
		Items:                items,
		Events:               events,
		CreatedAt:            order.CreatedAt,
		UpdatedAt:            order.UpdatedAt,
	}
}

func commerceEmailResponse(value *string) *openapi_types.Email {
	if value == nil {
		return nil
	}
	email := openapi_types.Email(*value)
	return &email
}
