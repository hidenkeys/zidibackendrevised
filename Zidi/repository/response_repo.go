package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
)

type ResponseRepository interface {
	GetResponsesByQuestion(questionID uuid.UUID, limit, offset int) ([]models.Response, int64, error)
	CreateResponse(response *models.Response) (*models.Response, error)
	GetResponseCountByQuestion(questionID uuid.UUID) (int64, error)
}
