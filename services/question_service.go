package services

import (
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/datatypes"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type QuestionService struct {
	questionRepo repository.QuestionRepository
	responseRepo repository.ResponseRepository
}

func NewQuestionService(questionRepo repository.QuestionRepository, responseRepo repository.ResponseRepository) *QuestionService {
	return &QuestionService{
		questionRepo: questionRepo,
		responseRepo: responseRepo,
	}
}

func (s *QuestionService) CreateQuestions(ctx context.Context, reqs []api.Question) ([]api.Question, error) {
	var createdQuestions []api.Question
	var newQuestions []*models.Question

	for _, req := range reqs {
		var optionsJSON datatypes.JSON
		if req.Options != nil {
			jsonData, err := json.Marshal(req.Options)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal options: %w", err)
			}
			optionsJSON = datatypes.JSON(jsonData)
		}

		newQuestion := &models.Question{
			ID:         req.Id,
			CampaignID: req.CampaignId,
			Text:       req.Text,
			Type:       string(req.Type),
			Options:    optionsJSON,
		}

		newQuestions = append(newQuestions, newQuestion)
	}

	// Bulk insert questions into the database
	createdQuestionsModel, err := s.questionRepo.CreateQuestions(newQuestions)
	if err != nil {
		return nil, err
	}

	// Convert models to API responses
	for _, question := range createdQuestionsModel {
		createdQuestions = append(createdQuestions, *mapToAPIQuestion(question))
	}

	return createdQuestions, nil
}

func (s *QuestionService) GetQuestionsByCampaign(ctx context.Context, campaignID uuid.UUID, limit, offset int) ([]api.Question, error) {
	questions, _, err := s.questionRepo.GetQuestionsByCampaign(campaignID, limit, offset)
	if err != nil {
		return nil, err
	}

	var finalQuestions []api.Question
	for _, question := range questions {
		count64, _ := s.responseRepo.GetResponseCountByQuestion(question.ID)
		count := int(count64) // Convert int64 to int
		q := mapToAPIQuestion(&question)
		q.ResponseCount = count

		finalQuestions = append(finalQuestions, *q)
	}

	return finalQuestions, nil
}

//func (s *QuestionService) GetQuestionByID(ctx context.Context, id uuid.UUID) (*api.Question, error) {
//	question, err := s.questionRepo.(id)
//	if err != nil {
//		return nil, err
//	}
//
//	return mapToAPIQuestion(&question), nil
//}

func (s *QuestionService) DeleteQuestion(ctx context.Context, id uuid.UUID) error {
	return s.questionRepo.DeleteQuestion(id)
}

// Helper function to convert models.Question to api.Question
func mapToAPIQuestion(question *models.Question) *api.Question {
	var options []string
	if question.Options != nil {
		_ = json.Unmarshal(question.Options, &options) // Convert JSONB to []string
	}

	return &api.Question{
		Id:         question.ID,
		CampaignId: question.CampaignID,
		Text:       question.Text,
		Type:       api.QuestionType(question.Type),
		Options:    &options,
	}
}
