package handlers

import (
	"context"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s Server) GetAllCustomers(c *fiber.Ctx, params api.GetAllCustomersParams) error {
	limit := 10
	offset := 0

	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	response, count, err := s.customerService.GetAllCustomers(context.Background(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"Message": "Success",
		"Count":   count,
		"Data":    response,
	})
}

func (s Server) CreateCustomer(c *fiber.Ctx) error {
	newCustomer := new(api.Customer)
	if err := c.BodyParser(newCustomer); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   err.Error(),
		})
	}

	createdCustomer, err := s.customerService.CreateCustomer(context.Background(), *newCustomer)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(createdCustomer)
}

func (s Server) GetCustomersByOrganization(c *fiber.Ctx, params api.GetCustomersByOrganizationParams) error {
	//userClaims, ok := c.Locals("user").(middleware.UserClaims)
	//if !ok {
	//	return c.Status(http.StatusUnauthorized).JSON(api.Error{
	//		ErrorCode: "401",
	//		Message:   "Unauthorized - Invalid token",
	//	})
	//}
	//
	//organizationUUID, err := uuid.Parse(userClaims.OrganizationID)
	//if err != nil {
	//	return c.Status(http.StatusBadRequest).JSON(api.Error{
	//		ErrorCode: "400",
	//		Message:   "Invalid organization ID format",
	//	})
	//}

	limit := 10
	offset := 0

	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	customers, count, err := s.customerService.GetCustomersByOrganization(context.Background(), params.OrganizationId, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"Message": "Success",
		"Count":   count,
		"Data":    customers,
	})
}

func (s Server) GetCustomersByCampaign(c *fiber.Ctx, params api.GetCustomersByCampaignParams) error {
	limit := 10
	offset := 0

	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	customers, count, err := s.customerService.GetCustomersByCampaign(context.Background(), params.CampaignId, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"Message": "Success",
		"Count":   count,
		"Data":    customers,
	})
}

func (s Server) DeleteCustomer(c *fiber.Ctx, id openapi_types.UUID) error {
	if id == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid customer ID",
		})
	}

	err := s.customerService.DeleteCustomer(context.Background(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(api.Error{
			ErrorCode: "404",
			Message:   "Customer not found",
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (s Server) GetCustomerById(c *fiber.Ctx, id openapi_types.UUID) error {
	if id == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid customer ID",
		})
	}

	customer, err := s.customerService.GetCustomerByID(context.Background(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(api.Error{
			ErrorCode: "404",
			Message:   "Customer not found",
		})
	}

	return c.Status(fiber.StatusOK).JSON(customer)
}

func (s Server) UpdateCustomer(c *fiber.Ctx, id openapi_types.UUID) error {
	if id == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid customer ID",
		})
	}

	updateRequest := new(api.Customer)
	if err := c.BodyParser(updateRequest); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   err.Error(),
		})
	}

	updatedCustomer, err := s.customerService.UpdateCustomer(context.Background(), id, updateRequest)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(updatedCustomer)
}
