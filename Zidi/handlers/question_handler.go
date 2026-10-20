package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s Server) GetCampaignsCampaignIdQuestions(c *fiber.Ctx, campaignId openapi_types.UUID, params api.GetCampaignsCampaignIdQuestionsParams) error {
	if campaignId == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid campaign ID",
		})
	}
	limit := 10
	offset := 0

	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}

	questions, err := s.questionService.GetQuestionsByCampaign(context.Background(), campaignId, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"Message": "Success",
		"Data":    questions,
	})
}

func (s Server) PostCampaignsCampaignIdQuestions(c *fiber.Ctx, campaignId openapi_types.UUID) error {
	if campaignId == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid campaign ID",
		})
	}

	var newQuestions []api.Question
	if err := c.BodyParser(&newQuestions); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   err.Error(),
		})
	}

	for i := range newQuestions {
		newQuestions[i].CampaignId = campaignId
	}

	createdQuestion, err := s.questionService.CreateQuestions(context.Background(), newQuestions)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(createdQuestion)
}

func (s Server) DeleteQuestionsQuestionId(c *fiber.Ctx, questionId openapi_types.UUID) error {
	if questionId == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid question ID",
		})
	}

	err := s.questionService.DeleteQuestion(context.Background(), questionId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(api.Error{
			ErrorCode: "404",
			Message:   "Question not found",
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}
