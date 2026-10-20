package repository

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

type questionPG struct {
	db *gorm.DB
}

func NewQuestionRepoPG(db *gorm.DB) QuestionRepository {
	return &questionPG{db: db}
}

func (r *questionPG) GetQuestionsByCampaign(campaignID uuid.UUID, limit, offset int) ([]models.Question, int64, error) {
	var questions []models.Question
	var total int64

	// Count total questions for the campaign
	if err := r.db.Model(&models.Question{}).Where("campaign_id = ?", campaignID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply limit and offset for pagination
	err := r.db.Where("campaign_id = ?", campaignID).Limit(limit).Offset(offset).Find(&questions).Error
	if err != nil {
		return nil, 0, err
	}

	return questions, total, nil
}

func (r *questionPG) CreateQuestions(questions []*models.Question) ([]*models.Question, error) {
	// Use GORM to batch insert multiple questions
	err := r.db.Create(&questions).Error
	if err != nil {
		return nil, err
	}

	fmt.Println("Questions inserted successfully:", questions)
	return questions, nil
}

func (r *questionPG) DeleteQuestion(questionID uuid.UUID) error {
	err := r.db.Where("id = ?", questionID).Delete(&models.Question{}).Error
	if err != nil {
		return err
	}
	return nil
}
