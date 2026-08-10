package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s Server) GetQuestionsQuestionIdResponses(c *fiber.Ctx, questionId openapi_types.UUID, params api.GetQuestionsQuestionIdResponsesParams) error {
	if questionId == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid question ID",
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

	responses, err := s.responseService.GetResponsesByQuestion(context.Background(), questionId, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"Message": "Success",
		"Data":    responses,
	})
}

func (s Server) PostQuestionsQuestionIdResponses(c *fiber.Ctx, questionId openapi_types.UUID) error {
	if questionId == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid question ID",
		})
	}

	newResponse := new(api.Response)
	if err := c.BodyParser(newResponse); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   err.Error(),
		})
	}

	newResponse.QuestionId = questionId

	createdResponse, err := s.responseService.CreateResponse(context.Background(), *newResponse)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(createdResponse)
}
