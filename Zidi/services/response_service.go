package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type ResponseService struct {
	responseRepo repository.ResponseRepository
}

func NewResponseService(responseRepo repository.ResponseRepository) *ResponseService {
	return &ResponseService{responseRepo: responseRepo}
}

func (s *ResponseService) CreateResponse(ctx context.Context, req api.Response) (*api.Response, error) {
	newResponse := &models.Response{
		ID:         req.Id,
		CustomerID: req.CustomerId,
		QuestionID: req.QuestionId,
		Answer:     req.Answer,
	}

	response, err := s.responseRepo.CreateResponse(newResponse)
	if err != nil {
		return nil, err
	}

	return mapToAPIResponse(response), nil
}

func (s *ResponseService) GetResponsesByQuestion(ctx context.Context, questionID uuid.UUID, limit, offset int) ([]api.Response, error) {
	responses, _, err := s.responseRepo.GetResponsesByQuestion(questionID, limit, offset)
	if err != nil {
		return nil, err
	}

	var finalResponses []api.Response
	for _, response := range responses {
		finalResponses = append(finalResponses, *mapToAPIResponse(&response))
	}

	return finalResponses, nil
}

// Helper function to convert models.Response to api.Response
func mapToAPIResponse(response *models.Response) *api.Response {
	return &api.Response{
		Id:         response.ID,
		CustomerId: response.CustomerID,
		QuestionId: response.QuestionID,
		Answer:     response.Answer,
	}
}
