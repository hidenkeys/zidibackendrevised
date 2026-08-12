package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/services"
)

func (s Server) ListCommerceStoreOrders(c *fiber.Ctx, params api.ListCommerceStoreOrdersParams) error {
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

	views, total, err := s.commerceStoreOrderService.ListOperationalOrders(c.UserContext(), actor, params.OrganizationId, services.CommerceStoreOrderListInput{
		StoreID: params.StoreId,
		Search:  optionalString(params.Search),
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return commerceError(c, err)
	}
	items := make([]api.CommerceStoreOrder, 0, len(views))
	for i := range views {
		items = append(items, commerceStoreOrderResponse(&views[i]))
	}
	return c.JSON(api.CommerceStoreOrderList{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s Server) MarkCommerceStoreOrderPrepared(c *fiber.Ctx, orderID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.MarkCommerceStoreOrderPreparedJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	view, err := s.commerceStoreOrderService.MarkPrepared(c.UserContext(), actor, request.OrganizationId, orderID, services.PrepareCommerceStoreOrderInput{
		IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceStoreOrderResponse(view))
}

func commerceStoreOrderResponse(view *services.CommerceStoreOrderView) api.CommerceStoreOrder {
	response := api.CommerceStoreOrder{Order: commerceOrderResponse(view.Order)}
	if view.Fulfilment != nil {
		fulfilment := commerceFulfilmentResponse(view.Fulfilment)
		response.Fulfilment = &fulfilment
	}
	return response
}
