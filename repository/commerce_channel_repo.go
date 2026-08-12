package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrCommerceConversationBusy = errors.New("commerce conversation is already processing a message")

type CommerceInboundChannelMessage struct {
	ExternalMessageID string
	SenderID          string
	MessageType       string
	Body              string
	Payload           json.RawMessage
}

type CommerceConversationClaim struct {
	Conversation *models.CommerceConversation
	Message      *models.CommerceChannelMessage
	Duplicate    bool
}

type CommerceChannelReplyOption struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type CommerceChannelReply struct {
	Body     string
	Options  []CommerceChannelReplyOption
	ImageURL string
}

type CommerceEmailNotification struct {
	OrderID   uuid.UUID
	Recipient string
	Subject   string
	HTMLBody  string
}

type CommerceConversationCompletion struct {
	OrganizationID uuid.UUID
	ConversationID uuid.UUID
	MessageID      uuid.UUID
	State          string
	CurrentIntent  *string
	Context        json.RawMessage
	Replies        []CommerceChannelReply
	Now            time.Time
}

type CommerceComplaintFilter struct {
	StoreID        *uuid.UUID
	Status         *string
	AssignedUserID *uuid.UUID
	Limit          int
	Offset         int
}

type CommerceComplaintUpdate struct {
	Status     string
	Resolution *string
	ResolvedAt *time.Time
}

type CommercePublicChannel struct {
	MerchantSlug        string
	MerchantDisplayName string
	Configuration       models.CommerceChannelConfiguration
}

type CommerceChannelRepository interface {
	GetChannelConfigurationByProviderAccount(ctx context.Context, channel, providerAccountID string) (*models.CommerceChannelConfiguration, error)
	GetChannelConfiguration(ctx context.Context, organizationID uuid.UUID, channel string) (*models.CommerceChannelConfiguration, error)
	GetActiveChannelByMerchantSlug(ctx context.Context, merchantSlug, channel string) (*CommercePublicChannel, error)
	UpsertChannelConfiguration(ctx context.Context, configuration *models.CommerceChannelConfiguration) (*models.CommerceChannelConfiguration, error)
	ClaimInboundMessage(ctx context.Context, configuration *models.CommerceChannelConfiguration, customerID uuid.UUID, input CommerceInboundChannelMessage, now time.Time) (*CommerceConversationClaim, error)
	CompleteInboundMessage(ctx context.Context, input CommerceConversationCompletion) error
	FailInboundMessage(ctx context.Context, organizationID, conversationID, messageID uuid.UUID, reason string, now time.Time) error
	ClaimOutboundMessages(ctx context.Context, limit int, now time.Time) ([]models.CommerceChannelMessage, error)
	MarkOutboundMessageSent(ctx context.Context, messageID uuid.UUID, providerMessageID string, now time.Time) error
	MarkOutboundMessageFailed(ctx context.Context, messageID uuid.UUID, reason string, retryAt time.Time) error
	CreateComplaint(ctx context.Context, complaint *models.CommerceComplaint) error
	GetComplaint(ctx context.Context, organizationID, complaintID uuid.UUID) (*models.CommerceComplaint, error)
	ListComplaints(ctx context.Context, organizationID uuid.UUID, filter CommerceComplaintFilter) ([]models.CommerceComplaint, int64, error)
	UpdateComplaint(ctx context.Context, organizationID, complaintID uuid.UUID, update CommerceComplaintUpdate) (*models.CommerceComplaint, error)
	ClaimCustomerOutboxEvents(ctx context.Context, limit int, now time.Time) ([]models.CommerceOutboxEvent, error)
	QueueOutboxNotification(ctx context.Context, event *models.CommerceOutboxEvent, customerID uuid.UUID, reply CommerceChannelReply, email *CommerceEmailNotification, now time.Time) error
	MarkOutboxEventFailed(ctx context.Context, eventID uuid.UUID, reason string, retryAt time.Time) error
	ClaimEmailMessages(ctx context.Context, limit int, now time.Time) ([]models.CommerceEmailMessage, error)
	MarkEmailMessageSent(ctx context.Context, messageID uuid.UUID, now time.Time) error
	MarkEmailMessageFailed(ctx context.Context, messageID uuid.UUID, reason string, retryAt time.Time) error
}

type CommerceChannelRepoPG struct{ db *gorm.DB }

func NewCommerceChannelRepoPG(db *gorm.DB) *CommerceChannelRepoPG {
	return &CommerceChannelRepoPG{db: db}
}

func (r *CommerceChannelRepoPG) GetChannelConfigurationByProviderAccount(ctx context.Context, channel, providerAccountID string) (*models.CommerceChannelConfiguration, error) {
	var item models.CommerceChannelConfiguration
	err := r.db.WithContext(ctx).Where("channel = ? AND provider_account_id = ?", channel, providerAccountID).First(&item).Error
	return commerceChannelConfigurationResult(&item, err)
}

func (r *CommerceChannelRepoPG) GetChannelConfiguration(ctx context.Context, organizationID uuid.UUID, channel string) (*models.CommerceChannelConfiguration, error) {
	var item models.CommerceChannelConfiguration
	err := r.db.WithContext(ctx).Where("organization_id = ? AND channel = ?", organizationID, channel).First(&item).Error
	return commerceChannelConfigurationResult(&item, err)
}

func (r *CommerceChannelRepoPG) GetActiveChannelByMerchantSlug(ctx context.Context, merchantSlug, channel string) (*CommercePublicChannel, error) {
	var merchant models.CommerceMerchantProfile
	err := r.db.WithContext(ctx).
		Where("slug = ? AND status = ?", merchantSlug, models.CommerceStatusActive).
		First(&merchant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get public commerce merchant: %w", err)
	}

	configuration, err := r.GetChannelConfiguration(ctx, merchant.OrganizationID, channel)
	if err != nil {
		return nil, err
	}
	if configuration.Status != models.CommerceStatusActive {
		return nil, ErrCommerceNotFound
	}
	return &CommercePublicChannel{
		MerchantSlug:        merchant.Slug,
		MerchantDisplayName: merchant.DisplayName,
		Configuration:       *configuration,
	}, nil
}

func (r *CommerceChannelRepoPG) UpsertChannelConfiguration(ctx context.Context, item *models.CommerceChannelConfiguration) (*models.CommerceChannelConfiguration, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO commerce_channel_configurations
				(id, organization_id, channel, provider_account_id, display_phone_number, welcome_message, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
			ON CONFLICT (organization_id, channel)
			DO UPDATE SET provider_account_id = EXCLUDED.provider_account_id,
			              display_phone_number = EXCLUDED.display_phone_number,
			              welcome_message = EXCLUDED.welcome_message,
			              status = EXCLUDED.status,
			              updated_at = NOW()
		`, item.ID, item.OrganizationID, item.Channel, item.ProviderAccountID, item.DisplayPhoneNumber, item.WelcomeMessage, item.Status).Error
	})
	if err != nil {
		return nil, mapCommerceWriteError("configure commerce channel", err)
	}
	return r.GetChannelConfiguration(ctx, item.OrganizationID, item.Channel)
}

func (r *CommerceChannelRepoPG) ClaimInboundMessage(ctx context.Context, configuration *models.CommerceChannelConfiguration, customerID uuid.UUID, input CommerceInboundChannelMessage, now time.Time) (*CommerceConversationClaim, error) {
	claim := &CommerceConversationClaim{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		conversationID := uuid.New()
		if err := tx.Exec(`
			INSERT INTO commerce_conversations
				(id, organization_id, channel_configuration_id, customer_id, channel, external_user_id, state, context, version, last_message_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, '{}'::jsonb, 1, ?, ?, ?)
			ON CONFLICT (organization_id, channel, external_user_id) DO NOTHING
		`, conversationID, configuration.OrganizationID, configuration.ID, customerID, configuration.Channel, input.SenderID, models.CommerceConversationStateWelcome, now, now, now).Error; err != nil {
			return err
		}

		var conversation models.CommerceConversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND channel = ? AND external_user_id = ?", configuration.OrganizationID, configuration.Channel, input.SenderID).
			First(&conversation).Error; err != nil {
			return err
		}
		if conversation.CustomerID != customerID || conversation.ChannelConfigurationID != configuration.ID {
			return ErrCommerceConflict
		}

		message := models.CommerceChannelMessage{
			ID: uuid.New(), OrganizationID: configuration.OrganizationID, ChannelConfigurationID: configuration.ID,
			ConversationID: conversation.ID, Direction: models.CommerceChannelDirectionInbound,
			ExternalMessageID: stringPointer(input.ExternalMessageID), SenderID: input.SenderID,
			MessageType: input.MessageType, Body: input.Body, Payload: validJSONObjectOrEmpty(input.Payload),
			Status: models.CommerceChannelMessageStatusReceived, AvailableAt: now,
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&message)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var existing models.CommerceChannelMessage
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("channel_configuration_id = ? AND external_message_id = ? AND direction = ?", configuration.ID, input.ExternalMessageID, models.CommerceChannelDirectionInbound).
				First(&existing).Error; err != nil {
				return err
			}
			if existing.Status == models.CommerceChannelMessageStatusProcessed {
				claim.Duplicate = true
				claim.Conversation = &conversation
				return nil
			}
			message = existing
		}
		if conversation.LockedUntil != nil && conversation.LockedUntil.After(now) && conversation.ProcessingMessageID != nil {
			return ErrCommerceConversationBusy
		}

		lockedUntil := now.Add(30 * time.Second)
		if err := tx.Model(&models.CommerceConversation{}).Where("id = ? AND organization_id = ?", conversation.ID, configuration.OrganizationID).Updates(map[string]interface{}{
			"processing_message_id": message.ID, "locked_until": lockedUntil, "last_message_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&message).Updates(map[string]interface{}{"status": models.CommerceChannelMessageStatusProcessing, "updated_at": now}).Error; err != nil {
			return err
		}
		conversation.ProcessingMessageID = &message.ID
		conversation.LockedUntil = &lockedUntil
		claim.Conversation = &conversation
		claim.Message = &message
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim inbound commerce message: %w", err)
	}
	return claim, nil
}

func (r *CommerceChannelRepoPG) CompleteInboundMessage(ctx context.Context, input CommerceConversationCompletion) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.CommerceConversation{}).
			Where("id = ? AND organization_id = ? AND processing_message_id = ?", input.ConversationID, input.OrganizationID, input.MessageID).
			Updates(map[string]interface{}{
				"state": input.State, "current_intent": input.CurrentIntent, "context": validJSONObjectOrEmpty(input.Context),
				"version": gorm.Expr("version + 1"), "processing_message_id": nil, "locked_until": nil,
				"last_message_at": input.Now, "updated_at": input.Now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCommerceConversationBusy
		}
		if err := tx.Model(&models.CommerceChannelMessage{}).
			Where("id = ? AND conversation_id = ?", input.MessageID, input.ConversationID).
			Updates(map[string]interface{}{"status": models.CommerceChannelMessageStatusProcessed, "processed_at": input.Now, "updated_at": input.Now}).Error; err != nil {
			return err
		}
		for _, reply := range input.Replies {
			payload, messageType, err := commerceReplyPayload(reply)
			if err != nil {
				return err
			}
			message := models.CommerceChannelMessage{
				ID: uuid.New(), OrganizationID: input.OrganizationID, ConversationID: input.ConversationID,
				Direction: models.CommerceChannelDirectionOutbound, RecipientID: "", MessageType: messageType,
				Body: reply.Body, Payload: payload, Status: models.CommerceChannelMessageStatusPending, AvailableAt: input.Now,
			}
			if err := tx.Raw("SELECT channel_configuration_id, external_user_id FROM commerce_conversations WHERE id = ? AND organization_id = ?", input.ConversationID, input.OrganizationID).
				Row().Scan(&message.ChannelConfigurationID, &message.RecipientID); err != nil {
				return err
			}
			if err := tx.Create(&message).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *CommerceChannelRepoPG) FailInboundMessage(ctx context.Context, organizationID, conversationID, messageID uuid.UUID, reason string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.CommerceConversation{}).Where("id = ? AND organization_id = ? AND processing_message_id = ?", conversationID, organizationID, messageID).
			Updates(map[string]interface{}{"processing_message_id": nil, "locked_until": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&models.CommerceChannelMessage{}).Where("id = ? AND conversation_id = ?", messageID, conversationID).
			Updates(map[string]interface{}{"status": models.CommerceChannelMessageStatusFailed, "last_error": truncateCommerceChannelError(reason), "updated_at": now}).Error
	})
}

func (r *CommerceChannelRepoPG) ClaimOutboundMessages(ctx context.Context, limit int, now time.Time) ([]models.CommerceChannelMessage, error) {
	var items []models.CommerceChannelMessage
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		staleBefore := now.Add(-2 * time.Minute)
		if err := tx.Model(&models.CommerceChannelMessage{}).
			Where("direction = ? AND status = ? AND locked_at < ?", models.CommerceChannelDirectionOutbound, models.CommerceChannelMessageStatusProcessing, staleBefore).
			Updates(map[string]interface{}{"status": models.CommerceChannelMessageStatusFailed, "locked_at": nil, "available_at": now, "last_error": "stale delivery lease recovered", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("direction = ? AND status IN ? AND available_at <= ? AND attempts < 8", models.CommerceChannelDirectionOutbound, []string{models.CommerceChannelMessageStatusPending, models.CommerceChannelMessageStatusFailed}, now).
			Order("created_at ASC").Limit(limit).Find(&items).Error; err != nil {
			return err
		}
		for index := range items {
			if err := tx.Model(&models.CommerceChannelMessage{}).Where("id = ?", items[index].ID).
				Updates(map[string]interface{}{"status": models.CommerceChannelMessageStatusProcessing, "attempts": gorm.Expr("attempts + 1"), "locked_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
			items[index].Status = models.CommerceChannelMessageStatusProcessing
			items[index].Attempts++
			items[index].LockedAt = &now
		}
		return nil
	})
	return items, err
}

func (r *CommerceChannelRepoPG) MarkOutboundMessageSent(ctx context.Context, messageID uuid.UUID, providerMessageID string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.CommerceChannelMessage{}).Where("id = ? AND status = ?", messageID, models.CommerceChannelMessageStatusProcessing).
		Updates(map[string]interface{}{"status": models.CommerceChannelMessageStatusSent, "provider_message_id": nullableString(providerMessageID), "processed_at": now, "locked_at": nil, "last_error": nil, "updated_at": now}).Error
}

func (r *CommerceChannelRepoPG) MarkOutboundMessageFailed(ctx context.Context, messageID uuid.UUID, reason string, retryAt time.Time) error {
	return r.db.WithContext(ctx).Model(&models.CommerceChannelMessage{}).Where("id = ? AND status = ?", messageID, models.CommerceChannelMessageStatusProcessing).
		Updates(map[string]interface{}{"status": models.CommerceChannelMessageStatusFailed, "available_at": retryAt, "locked_at": nil, "last_error": truncateCommerceChannelError(reason), "updated_at": time.Now().UTC()}).Error
}

func (r *CommerceChannelRepoPG) CreateComplaint(ctx context.Context, complaint *models.CommerceComplaint) error {
	if err := r.db.WithContext(ctx).Create(complaint).Error; err != nil {
		return mapCommerceWriteError("create commerce complaint", err)
	}
	return nil
}

func (r *CommerceChannelRepoPG) GetComplaint(ctx context.Context, organizationID, complaintID uuid.UUID) (*models.CommerceComplaint, error) {
	var item models.CommerceComplaint
	err := r.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, complaintID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	return &item, err
}

func (r *CommerceChannelRepoPG) ListComplaints(ctx context.Context, organizationID uuid.UUID, filter CommerceComplaintFilter) ([]models.CommerceComplaint, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.CommerceComplaint{}).Where("commerce_complaints.organization_id = ?", organizationID)
	if filter.StoreID != nil {
		query = query.Where("commerce_complaints.store_id = ?", *filter.StoreID)
	}
	if filter.Status != nil {
		query = query.Where("commerce_complaints.status = ?", *filter.Status)
	}
	if filter.AssignedUserID != nil {
		query = query.Joins(`JOIN commerce_staff_store_assignments assignments
			ON assignments.organization_id = commerce_complaints.organization_id
			AND assignments.store_id = commerce_complaints.store_id
			AND assignments.user_id = ? AND assignments.status = ? AND assignments.deleted_at IS NULL`, *filter.AssignedUserID, models.CommerceStatusActive)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.CommerceComplaint
	err := query.Order("commerce_complaints.created_at DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&items).Error
	return items, total, err
}

func (r *CommerceChannelRepoPG) UpdateComplaint(ctx context.Context, organizationID, complaintID uuid.UUID, update CommerceComplaintUpdate) (*models.CommerceComplaint, error) {
	values := map[string]interface{}{"status": update.Status, "resolution": update.Resolution, "resolved_at": update.ResolvedAt, "updated_at": time.Now().UTC()}
	result := r.db.WithContext(ctx).Model(&models.CommerceComplaint{}).Where("organization_id = ? AND id = ?", organizationID, complaintID).Updates(values)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrCommerceNotFound
	}
	return r.GetComplaint(ctx, organizationID, complaintID)
}

func (r *CommerceChannelRepoPG) ClaimCustomerOutboxEvents(ctx context.Context, limit int, now time.Time) ([]models.CommerceOutboxEvent, error) {
	topics := []string{
		models.CommerceOutboxTopicPaymentCustomer, models.CommerceOutboxTopicFulfilmentReady,
		models.CommerceOutboxTopicDeliveryQuoteAvailable, models.CommerceOutboxTopicRiderAssigned,
		models.CommerceOutboxTopicHandoverCodeReminder,
		models.CommerceOutboxTopicOutForDelivery, models.CommerceOutboxTopicDeliveryConfirmationRequested,
		models.CommerceOutboxTopicFulfilmentDelivered,
	}
	var items []models.CommerceOutboxEvent
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		staleBefore := now.Add(-2 * time.Minute)
		if err := tx.Model(&models.CommerceOutboxEvent{}).Where("status = ? AND locked_at < ? AND topic IN ?", models.CommerceOutboxStatusProcessing, staleBefore, topics).
			Updates(map[string]interface{}{"status": models.CommerceOutboxStatusFailed, "locked_at": nil, "available_at": now, "last_error": "stale notification lease recovered", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status IN ? AND available_at <= ? AND attempts < 8 AND topic IN ?", []string{models.CommerceOutboxStatusPending, models.CommerceOutboxStatusFailed}, now, topics).
			Order("created_at ASC").Limit(limit).Find(&items).Error; err != nil {
			return err
		}
		for index := range items {
			if err := tx.Model(&models.CommerceOutboxEvent{}).Where("id = ?", items[index].ID).Updates(map[string]interface{}{
				"status": models.CommerceOutboxStatusProcessing, "attempts": gorm.Expr("attempts + 1"), "locked_at": now, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			items[index].Attempts++
		}
		return nil
	})
	return items, err
}

func (r *CommerceChannelRepoPG) QueueOutboxNotification(ctx context.Context, event *models.CommerceOutboxEvent, customerID uuid.UUID, reply CommerceChannelReply, email *CommerceEmailNotification, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var configuration models.CommerceChannelConfiguration
		if err := tx.Where("organization_id = ? AND channel = ? AND status = ?", event.OrganizationID, models.CommerceChannelWhatsApp, models.CommerceStatusActive).First(&configuration).Error; err != nil {
			return err
		}
		var identity models.CommerceCustomerIdentity
		if err := tx.Where("organization_id = ? AND customer_id = ? AND channel = ? AND verified_at IS NOT NULL", event.OrganizationID, customerID, models.CommerceIdentityChannelWhatsApp).First(&identity).Error; err != nil {
			return err
		}
		conversationID := uuid.New()
		if err := tx.Exec(`INSERT INTO commerce_conversations
			(id, organization_id, channel_configuration_id, customer_id, channel, external_user_id, state, context, version, last_message_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, '{}'::jsonb, 1, ?, ?, ?)
			ON CONFLICT (organization_id, channel, external_user_id) DO NOTHING`, conversationID, event.OrganizationID, configuration.ID, customerID,
			models.CommerceChannelWhatsApp, identity.NormalizedIdentifier, models.CommerceConversationStateIntent, now, now, now).Error; err != nil {
			return err
		}
		var conversation models.CommerceConversation
		if err := tx.Where("organization_id = ? AND channel = ? AND external_user_id = ?", event.OrganizationID, models.CommerceChannelWhatsApp, identity.NormalizedIdentifier).First(&conversation).Error; err != nil {
			return err
		}
		payload, messageType, err := commerceReplyPayload(reply)
		if err != nil {
			return err
		}
		var payloadObject map[string]interface{}
		_ = json.Unmarshal(payload, &payloadObject)
		payloadObject["outbox_event_id"] = event.ID.String()
		payload, _ = json.Marshal(payloadObject)
		message := models.CommerceChannelMessage{
			ID: uuid.New(), OrganizationID: event.OrganizationID, ChannelConfigurationID: configuration.ID, ConversationID: conversation.ID,
			Direction: models.CommerceChannelDirectionOutbound, RecipientID: identity.NormalizedIdentifier,
			MessageType: messageType, Body: reply.Body, Payload: payload, Status: models.CommerceChannelMessageStatusPending, AvailableAt: now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if email != nil && strings.TrimSpace(email.Recipient) != "" {
			emailMessage := models.CommerceEmailMessage{
				ID: uuid.New(), OrganizationID: event.OrganizationID, CustomerID: customerID,
				OrderID: email.OrderID, OutboxEventID: event.ID, Recipient: strings.TrimSpace(email.Recipient),
				Subject: strings.TrimSpace(email.Subject), HTMLBody: email.HTMLBody,
				Status: models.CommerceEmailStatusPending, AvailableAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&emailMessage).Error; err != nil {
				return err
			}
		}
		result := tx.Model(&models.CommerceOutboxEvent{}).Where("id = ? AND status = ?", event.ID, models.CommerceOutboxStatusProcessing).Updates(map[string]interface{}{
			"status": models.CommerceOutboxStatusDelivered, "processed_at": now, "locked_at": nil, "last_error": nil, "updated_at": now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCommerceConflict
		}
		return nil
	})
}

func (r *CommerceChannelRepoPG) ClaimEmailMessages(ctx context.Context, limit int, now time.Time) ([]models.CommerceEmailMessage, error) {
	var items []models.CommerceEmailMessage
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		staleBefore := now.Add(-2 * time.Minute)
		if err := tx.Model(&models.CommerceEmailMessage{}).
			Where("status = ? AND locked_at < ?", models.CommerceEmailStatusProcessing, staleBefore).
			Updates(map[string]interface{}{"status": models.CommerceEmailStatusFailed, "locked_at": nil, "available_at": now, "last_error": "stale email lease recovered", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND available_at <= ? AND attempts < 8", []string{models.CommerceEmailStatusPending, models.CommerceEmailStatusFailed}, now).
			Order("created_at ASC").Limit(limit).Find(&items).Error; err != nil {
			return err
		}
		for index := range items {
			if err := tx.Model(&models.CommerceEmailMessage{}).Where("id = ?", items[index].ID).
				Updates(map[string]interface{}{"status": models.CommerceEmailStatusProcessing, "attempts": gorm.Expr("attempts + 1"), "locked_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
			items[index].Attempts++
		}
		return nil
	})
	return items, err
}

func (r *CommerceChannelRepoPG) MarkEmailMessageSent(ctx context.Context, messageID uuid.UUID, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&models.CommerceEmailMessage{}).
		Where("id = ? AND status = ?", messageID, models.CommerceEmailStatusProcessing).
		Updates(map[string]interface{}{"status": models.CommerceEmailStatusSent, "sent_at": now, "locked_at": nil, "last_error": nil, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCommerceConflict
	}
	return nil
}

func (r *CommerceChannelRepoPG) MarkEmailMessageFailed(ctx context.Context, messageID uuid.UUID, reason string, retryAt time.Time) error {
	return r.db.WithContext(ctx).Model(&models.CommerceEmailMessage{}).
		Where("id = ? AND status = ?", messageID, models.CommerceEmailStatusProcessing).
		Updates(map[string]interface{}{"status": models.CommerceEmailStatusFailed, "available_at": retryAt, "locked_at": nil, "last_error": truncateCommerceChannelError(reason), "updated_at": time.Now().UTC()}).Error
}

func (r *CommerceChannelRepoPG) MarkOutboxEventFailed(ctx context.Context, eventID uuid.UUID, reason string, retryAt time.Time) error {
	return r.db.WithContext(ctx).Model(&models.CommerceOutboxEvent{}).Where("id = ? AND status = ?", eventID, models.CommerceOutboxStatusProcessing).Updates(map[string]interface{}{
		"status": models.CommerceOutboxStatusFailed, "available_at": retryAt, "locked_at": nil,
		"last_error": truncateCommerceChannelError(reason), "updated_at": time.Now().UTC(),
	}).Error
}

func commerceChannelConfigurationResult(item *models.CommerceChannelConfiguration, err error) (*models.CommerceChannelConfiguration, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get commerce channel configuration: %w", err)
	}
	return item, nil
}

func commerceReplyPayload(reply CommerceChannelReply) (json.RawMessage, string, error) {
	messageType := "text"
	payload := map[string]interface{}{}
	if strings.TrimSpace(reply.ImageURL) != "" {
		messageType = "image"
		payload["image_url"] = strings.TrimSpace(reply.ImageURL)
	} else if len(reply.Options) > 0 {
		messageType = "interactive"
		payload["buttons"] = reply.Options
	}
	encoded, err := json.Marshal(payload)
	return encoded, messageType, err
}

func validJSONObjectOrEmpty(value json.RawMessage) json.RawMessage {
	var object map[string]interface{}
	if len(value) == 0 || json.Unmarshal(value, &object) != nil || object == nil {
		return json.RawMessage(`{}`)
	}
	return value
}

func stringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func nullableString(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func truncateCommerceChannelError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1000 {
		return value[:1000]
	}
	return value
}
