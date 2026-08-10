package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

type QuestionRepository interface {
	GetQuestionsByCampaign(campaignID uuid.UUID, limit, offset int) ([]models.Question, int64, error)
	CreateQuestions(question []*models.Question) ([]*models.Question, error)
	DeleteQuestion(questionID uuid.UUID) error
}
