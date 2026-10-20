package handlers

import (
	"context"
	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidibackend/api"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"net/http"
)

func (s Server) GetAllPayments(c *fiber.Ctx, params api.GetAllPaymentsParams) error {
	limit := 10
	offset := 0

	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	response, count, err := s.paymentService.GetAllPayments(context.Background(), limit, offset)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "success",
		"data":    response,
		"count":   count,
	})
}

func (s Server) CreatePayment(c *fiber.Ctx) error {
	//TODO implement me
	panic("implement me")
}

func (s Server) GetPaymentsByOrganization(c *fiber.Ctx, organizationId openapi_types.UUID, params api.GetPaymentsByOrganizationParams) error {
	limit := 10
	offset := 0

	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}

	response, count, err := s.paymentService.GetPaymentsByOrganization(context.Background(), organizationId, limit, offset)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "success",
		"count":   count,
		"data":    response,
	})
}

func (s Server) GetPaymentById(c *fiber.Ctx, id openapi_types.UUID) error {
	response, err := s.paymentService.GetPaymentByID(context.Background(), id)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(response)
}

func (s Server) UpdatePaymentById(c *fiber.Ctx, id openapi_types.UUID) error {
	payment := new(api.Payment)
	if err := c.BodyParser(payment); err != nil {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid request body",
		})
	}
	response, err := s.paymentService.UpdatePayment(context.Background(), id, payment)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(response)
}
