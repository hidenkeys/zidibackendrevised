package handlers

import (
	"context"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s Server) GetAllBalances(c *fiber.Ctx, params api.GetAllBalancesParams) error {
	limit := 10
	offset := 0

	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}

	balances, err := s.balanceService.GetAllBalances(context.Background(), limit, offset)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(balances)
}

func (s Server) CreateBalance(c *fiber.Ctx) error {
	balance := new(api.Balance)
	if err := c.BodyParser(balance); err != nil {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid request body",
		})
	}

	response, err := s.balanceService.CreateBalance(context.Background(), balance)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusCreated).JSON(response)
}

func (s Server) GetBalanceByCampaign(c *fiber.Ctx, campaignId openapi_types.UUID) error {
	balance, err := s.balanceService.GetBalanceByCampaign(context.Background(), uuid.UUID(campaignId))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	if balance == nil {
		return c.Status(http.StatusNotFound).JSON(api.Error{
			ErrorCode: "404",
			Message:   "Balance not found",
		})
	}

	response, err := s.campaignService.GetCampaignByID(context.Background(), uuid.UUID(campaignId))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(balance, response.CampaignName)
}

func (s Server) UpdateBalance(c *fiber.Ctx, campaignId openapi_types.UUID) error {
	updateRequest := new(api.UpdateBalanceRequest)
	if err := c.BodyParser(updateRequest); err != nil {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid request body",
		})
	}

	ctx := context.Background()
	currentBalance, err := s.balanceService.GetBalanceByCampaign(ctx, uuid.UUID(campaignId))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	if currentBalance == nil {
		return c.Status(http.StatusNotFound).JSON(api.Error{
			ErrorCode: "404",
			Message:   "Balance not found",
		})
	}

	newAmount := currentBalance.Amount - updateRequest.Amount
	if newAmount <= 0 {
		// Leave logic handling for user as requested
		response, err := s.campaignService.GetCampaignByID(context.Background(), currentBalance.CampaignId)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(api.Error{
				ErrorCode: "500",
				Message:   err.Error(),
			})
		}
		response.Status = "inactive"
		_, err = s.campaignService.UpdateCampaign(context.Background(), currentBalance.CampaignId, response)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(api.Error{
				ErrorCode: "500",
				Message:   err.Error(),
			})
		}
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"message":       "Balance depleted",
			"currentAmount": newAmount,
		})
	}

	_, err = s.balanceService.UpdateBalance(ctx, uuid.UUID(campaignId), float64(newAmount))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"campaignId":    campaignId,
		"updatedAmount": newAmount,
	})
}
