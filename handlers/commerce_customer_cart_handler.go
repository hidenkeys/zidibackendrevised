package handlers

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/services"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s Server) ResolveCommerceCustomer(c *fiber.Ctx) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.ResolveCommerceCustomerJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	email := ""
	if request.Email != nil {
		email = string(*request.Email)
	}
	customer, created, err := s.commerceCustomerCartService.ResolveCustomer(c.UserContext(), actor, request.OrganizationId, services.ResolveCommerceCustomerInput{
		Channel:     string(request.Channel),
		Identifier:  request.Identifier,
		DisplayName: optionalString(request.DisplayName),
		Email:       email,
	})
	if err != nil {
		return commerceError(c, err)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return c.Status(status).JSON(commerceCustomerResponse(customer))
}

func (s Server) GetCommerceCustomer(c *fiber.Ctx, customerID uuid.UUID, params api.GetCommerceCustomerParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	customer, err := s.commerceCustomerCartService.GetCustomer(c.UserContext(), actor, params.OrganizationId, customerID)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceCustomerResponse(customer))
}

func (s Server) CreateCommerceCart(c *fiber.Ctx) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.CreateCommerceCartJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	view, created, err := s.commerceCustomerCartService.CreateCart(c.UserContext(), actor, request.OrganizationId, services.CreateCommerceCartInput{
		CustomerID: request.CustomerId,
		StoreID:    request.StoreId,
	})
	if err != nil {
		return commerceError(c, err)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return c.Status(status).JSON(commerceCartResponse(view))
}

func (s Server) GetCommerceCart(c *fiber.Ctx, cartID uuid.UUID, params api.GetCommerceCartParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	view, err := s.commerceCustomerCartService.GetCart(c.UserContext(), actor, params.OrganizationId, cartID)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceCartResponse(view))
}

func (s Server) SetCommerceCartItem(c *fiber.Ctx, cartID, variantID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.SetCommerceCartItemJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	view, err := s.commerceCustomerCartService.SetCartItem(c.UserContext(), actor, request.OrganizationId, cartID, variantID, request.Quantity)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceCartResponse(view))
}

func (s Server) DeleteCommerceCartItem(c *fiber.Ctx, cartID, variantID uuid.UUID, params api.DeleteCommerceCartItemParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	view, err := s.commerceCustomerCartService.DeleteCartItem(c.UserContext(), actor, params.OrganizationId, cartID, variantID)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceCartResponse(view))
}

func (s Server) ClearCommerceCart(c *fiber.Ctx, cartID uuid.UUID, params api.ClearCommerceCartParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	view, err := s.commerceCustomerCartService.ClearCart(c.UserContext(), actor, params.OrganizationId, cartID)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceCartResponse(view))
}

func commerceCustomerResponse(customer *models.CommerceCustomer) api.CommerceCustomer {
	identities := make([]api.CommerceCustomerIdentity, 0, len(customer.Identities))
	for _, identity := range customer.Identities {
		identities = append(identities, api.CommerceCustomerIdentity{
			Id:         identity.ID,
			Channel:    api.CommerceCustomerIdentityChannel(identity.Channel),
			Identifier: identity.DisplayIdentifier,
			Verified:   identity.VerifiedAt != nil,
			VerifiedAt: identity.VerifiedAt,
			CreatedAt:  identity.CreatedAt,
		})
	}
	var email *openapi_types.Email
	if customer.Email != nil {
		value := openapi_types.Email(*customer.Email)
		email = &value
	}
	return api.CommerceCustomer{
		Id:             customer.ID,
		OrganizationId: customer.OrganizationID,
		DisplayName:    customer.DisplayName,
		Email:          email,
		Status:         api.CommerceCustomerStatus(customer.Status),
		Identities:     identities,
		CreatedAt:      customer.CreatedAt,
		UpdatedAt:      customer.UpdatedAt,
	}
}

func commerceCartResponse(view *services.CommerceCartView) api.CommerceCart {
	items := make([]api.CommerceCartItem, 0, len(view.Items))
	for _, line := range view.Items {
		items = append(items, api.CommerceCartItem{
			Id:                line.Item.ID,
			ProductId:         line.ProductID,
			ProductName:       line.ProductName,
			VariantId:         line.Item.VariantID,
			VariantName:       line.VariantName,
			Sku:               line.SKU,
			PrimaryImageUrl:   line.PrimaryImageURL,
			Quantity:          line.Item.Quantity,
			UnitPriceMinor:    line.UnitPriceMinor,
			LineTotalMinor:    line.LineTotalMinor,
			AvailableQuantity: line.AvailableQuantity,
			Available:         line.Available,
			UnavailableReason: line.UnavailableReason,
			CreatedAt:         line.Item.CreatedAt,
			UpdatedAt:         line.Item.UpdatedAt,
		})
	}
	return api.CommerceCart{
		Id:             view.Cart.ID,
		OrganizationId: view.Cart.OrganizationID,
		CustomerId:     view.Cart.CustomerID,
		StoreId:        view.Cart.StoreID,
		Currency:       view.Cart.Currency,
		Status:         api.CommerceCartStatus(view.Cart.Status),
		Version:        view.Cart.Version,
		ExpiresAt:      view.Cart.ExpiresAt,
		Items:          items,
		ItemCount:      view.ItemCount,
		SubtotalMinor:  view.SubtotalMinor,
		TotalMinor:     view.TotalMinor,
		CheckoutReady:  view.CheckoutReady,
		CreatedAt:      view.Cart.CreatedAt,
		UpdatedAt:      view.Cart.UpdatedAt,
	}
}
