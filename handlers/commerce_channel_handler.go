package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/services"
)

func (s Server) ConfigureCommerceWhatsApp(c *fiber.Ctx) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.ConfigureCommerceWhatsAppJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	item, err := s.commerceChannelService.ConfigureWhatsApp(c.UserContext(), actor, request.OrganizationId, services.ConfigureCommerceChannelInput{
		ProviderAccountID: request.ProviderAccountId, DisplayPhoneNumber: optionalString(request.DisplayPhoneNumber),
		WelcomeMessage: optionalString(request.WelcomeMessage), Status: stringValue(request.Status),
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceChannelConfigurationResponse(item))
}

func (s Server) GetCommerceWhatsAppConfiguration(c *fiber.Ctx, params api.GetCommerceWhatsAppConfigurationParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	item, err := s.commerceChannelService.GetWhatsAppConfiguration(c.UserContext(), actor, params.OrganizationId)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceChannelConfigurationResponse(item))
}

func (s Server) GetPublicCommerceWhatsAppLink(c *fiber.Ctx, merchantSlug string) error {
	item, err := s.commerceChannelService.ResolvePublicWhatsAppLink(c.UserContext(), merchantSlug)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(api.CommerceWhatsAppLink{
		MerchantSlug:        item.MerchantSlug,
		MerchantDisplayName: item.MerchantDisplayName,
		DisplayPhoneNumber:  item.DisplayPhoneNumber,
		Url:                 item.URL,
	})
}

func (s Server) ListCommerceComplaints(c *fiber.Ctx, params api.ListCommerceComplaintsParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var status *string
	if params.Status != nil {
		value := string(*params.Status)
		status = &value
	}
	items, total, err := s.commerceChannelService.ListComplaints(c.UserContext(), actor, params.OrganizationId, services.CommerceComplaintListInput{
		StoreID: params.StoreId, Status: status, Limit: optionalInt(params.Limit), Offset: optionalInt(params.Offset),
	})
	if err != nil {
		return commerceError(c, err)
	}
	response := make([]api.CommerceComplaint, 0, len(items))
	for index := range items {
		response = append(response, commerceComplaintResponse(&items[index]))
	}
	return c.JSON(api.CommerceComplaintList{Items: response, Total: total})
}

func (s Server) UpdateCommerceComplaint(c *fiber.Ctx, complaintID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.UpdateCommerceComplaintJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	item, err := s.commerceChannelService.UpdateComplaint(c.UserContext(), actor, request.OrganizationId, complaintID, services.UpdateCommerceComplaintInput{
		Status: string(request.Status), Resolution: optionalString(request.Resolution),
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceComplaintResponse(item))
}

func commerceChannelConfigurationResponse(item *models.CommerceChannelConfiguration) api.CommerceChannelConfiguration {
	return api.CommerceChannelConfiguration{
		Id: item.ID, OrganizationId: item.OrganizationID, Channel: api.CommerceChannelConfigurationChannel(item.Channel),
		ProviderAccountId: item.ProviderAccountID, DisplayPhoneNumber: item.DisplayPhoneNumber, WelcomeMessage: item.WelcomeMessage,
		Status: api.CommerceChannelConfigurationStatus(item.Status), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func commerceComplaintResponse(item *models.CommerceComplaint) api.CommerceComplaint {
	return api.CommerceComplaint{
		Id: item.ID, OrganizationId: item.OrganizationID, CustomerId: item.CustomerID, OrderId: item.OrderID,
		StoreId: item.StoreID, ConversationId: item.ConversationID, Category: item.Category, Description: item.Description,
		Status: api.CommerceComplaintStatus(item.Status), Resolution: item.Resolution, ResolvedAt: item.ResolvedAt,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func optionalInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
