package repository

import (
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type responsePG struct {
	db *gorm.DB
}

func NewResponseRepoPG(db *gorm.DB) ResponseRepository {
	return &responsePG{db: db}
}

func (r *responsePG) GetResponsesByQuestion(questionID uuid.UUID, limit, offset int) ([]models.Response, int64, error) {
	var responses []models.Response
	var total int64

	// Count total responses for the question
	if err := r.db.Model(&models.Response{}).Where("question_id = ?", questionID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply limit and offset for pagination
	err := r.db.Where("question_id = ?", questionID).Limit(limit).Offset(offset).Find(&responses).Error
	if err != nil {
		return nil, 0, err
	}

	return responses, total, nil
}

func (r *responsePG) CreateResponse(response *models.Response) (*models.Response, error) {
	err := r.db.Create(response).Error
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (r *responsePG) GetResponseCountByQuestion(questionID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Model(&models.Response{}).Where("question_id = ?", questionID).Count(&count).Error
	return count, err
}
