package repository

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCommerceFulfilmentState     = errors.New("invalid commerce fulfilment state")
	ErrCommerceVerificationFailed  = errors.New("fulfilment verification failed")
	ErrCommerceVerificationLocked  = errors.New("fulfilment verification is temporarily locked")
	ErrCommerceVerificationExpired = errors.New("fulfilment verification code expired")
)

const (
	commerceVerificationMaxAttempts = 5
	commerceVerificationLockTime    = 15 * time.Minute
)

type CommerceStartFulfilmentInput struct {
	Fulfilment     models.CommerceFulfilment
	OrderStatus    string
	ActorUserID    uuid.UUID
	IdempotencyKey string
	Now            time.Time
}

type CommerceCreateDeliveryQuoteInput struct {
	Quote          models.CommerceDeliveryQuote
	ActorUserID    uuid.UUID
	IdempotencyKey string
	Now            time.Time
}

type CommerceDeliveryQuoteDecisionInput struct {
	OrganizationID uuid.UUID
	FulfilmentID   uuid.UUID
	QuoteID        uuid.UUID
	Decision       string
	ActorType      string
	ActorUserID    *uuid.UUID
	Reason         string
	IdempotencyKey string
	Now            time.Time
}

type CommerceAssignRiderInput struct {
	Assignment     models.CommerceRiderAssignment
	ActorUserID    uuid.UUID
	IdempotencyKey string
	Now            time.Time
}

type CommerceVerifyHandoverInput struct {
	OrganizationID uuid.UUID
	FulfilmentID   uuid.UUID
	CandidateHash  []byte
	ActorUserID    uuid.UUID
	IdempotencyKey string
	Now            time.Time
}

type CommerceFulfilmentTransitionInput struct {
	OrganizationID uuid.UUID
	FulfilmentID   uuid.UUID
	ActorType      string
	ActorUserID    *uuid.UUID
	Reason         string
	IdempotencyKey string
	Now            time.Time
}

type CommerceFulfilmentRepository interface {
	StartFulfilment(ctx context.Context, input CommerceStartFulfilmentInput) (*models.CommerceFulfilment, bool, error)
	GetFulfilmentByOrder(ctx context.Context, organizationID, orderID uuid.UUID) (*models.CommerceFulfilment, error)
	ListFulfilmentsByOrderIDs(ctx context.Context, organizationID uuid.UUID, orderIDs []uuid.UUID) ([]models.CommerceFulfilment, error)
	GetFulfilment(ctx context.Context, organizationID, fulfilmentID uuid.UUID) (*models.CommerceFulfilment, error)
	CreateDeliveryQuote(ctx context.Context, input CommerceCreateDeliveryQuoteInput) (*models.CommerceFulfilment, error)
	DecideDeliveryQuote(ctx context.Context, input CommerceDeliveryQuoteDecisionInput) (*models.CommerceFulfilment, error)
	AssignRider(ctx context.Context, input CommerceAssignRiderInput) (*models.CommerceFulfilment, error)
	RecordArrival(ctx context.Context, input CommerceFulfilmentTransitionInput) (*models.CommerceFulfilment, error)
	VerifyHandover(ctx context.Context, input CommerceVerifyHandoverInput) (*models.CommerceFulfilment, error)
	MarkDelivered(ctx context.Context, input CommerceFulfilmentTransitionInput) (*models.CommerceFulfilment, error)
	CompleteFulfilment(ctx context.Context, input CommerceFulfilmentTransitionInput) (*models.CommerceFulfilment, error)
}

type CommerceFulfilmentRepoPG struct {
	db *gorm.DB
}

func NewCommerceFulfilmentRepoPG(db *gorm.DB) *CommerceFulfilmentRepoPG {
	return &CommerceFulfilmentRepoPG{db: db}
}

func (r *CommerceFulfilmentRepoPG) StartFulfilment(ctx context.Context, input CommerceStartFulfilmentInput) (*models.CommerceFulfilment, bool, error) {
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.CommerceFulfilment
		err := tx.Where("organization_id = ? AND order_id = ?", input.Fulfilment.OrganizationID, input.Fulfilment.OrderID).First(&existing).Error
		if err == nil {
			if existing.Mode != input.Fulfilment.Mode || !sameCommerceOptionalString(existing.DestinationAddress, input.Fulfilment.DestinationAddress) ||
				!sameCommerceOptionalFloat(existing.DestinationLatitude, input.Fulfilment.DestinationLatitude) ||
				!sameCommerceOptionalFloat(existing.DestinationLongitude, input.Fulfilment.DestinationLongitude) {
				return ErrCommerceConflict
			}
			input.Fulfilment.ID = existing.ID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var order models.CommerceOrder
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND id = ?", input.Fulfilment.OrganizationID, input.Fulfilment.OrderID).
			First(&order).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommerceNotFound
		}
		if err != nil {
			return err
		}
		err = tx.Where("organization_id = ? AND order_id = ?", input.Fulfilment.OrganizationID, input.Fulfilment.OrderID).First(&existing).Error
		if err == nil {
			if existing.Mode != input.Fulfilment.Mode || !sameCommerceOptionalString(existing.DestinationAddress, input.Fulfilment.DestinationAddress) ||
				!sameCommerceOptionalFloat(existing.DestinationLatitude, input.Fulfilment.DestinationLatitude) ||
				!sameCommerceOptionalFloat(existing.DestinationLongitude, input.Fulfilment.DestinationLongitude) {
				return ErrCommerceConflict
			}
			input.Fulfilment.ID = existing.ID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if order.Status != models.CommerceOrderStatusReady || order.StoreID != input.Fulfilment.StoreID ||
			order.CustomerID != input.Fulfilment.CustomerID || order.FulfilmentMode != input.Fulfilment.Mode {
			return ErrCommerceFulfilmentState
		}

		if err := tx.Create(&input.Fulfilment).Error; err != nil {
			return err
		}
		if err := transitionCommerceFulfilmentOrder(tx, &order, input.OrderStatus, commerceOrderEventForFulfilmentStart(input.Fulfilment.Mode),
			"fulfilment started for "+input.Fulfilment.Mode, input.IdempotencyKey+":order", models.CommerceOrderActorUser, &input.ActorUserID, input.Now); err != nil {
			return err
		}
		if err := createCommerceFulfilmentEvent(tx, &input.Fulfilment, models.CommerceFulfilmentEventStarted, nil,
			input.Fulfilment.Status, models.CommerceFulfilmentActorUser, &input.ActorUserID, "order is ready for fulfilment", input.IdempotencyKey, nil, input.Now); err != nil {
			return err
		}
		if input.Fulfilment.Mode != models.FulfilmentModeMerchantRider {
			if err := createCommerceFulfilmentOutbox(tx, &input.Fulfilment, models.CommerceOutboxTopicFulfilmentReady,
				input.IdempotencyKey+":ready", map[string]interface{}{"mode": input.Fulfilment.Mode}, input.Now); err != nil {
				return err
			}
		}
		created = true
		return nil
	})
	if err != nil {
		mapped := mapCommerceFulfilmentWriteError("start commerce fulfilment", err)
		if errors.Is(mapped, ErrCommerceConflict) {
			existing, lookupErr := r.GetFulfilmentByOrder(ctx, input.Fulfilment.OrganizationID, input.Fulfilment.OrderID)
			if lookupErr == nil && existing.Mode == input.Fulfilment.Mode &&
				sameCommerceOptionalString(existing.DestinationAddress, input.Fulfilment.DestinationAddress) &&
				sameCommerceOptionalFloat(existing.DestinationLatitude, input.Fulfilment.DestinationLatitude) &&
				sameCommerceOptionalFloat(existing.DestinationLongitude, input.Fulfilment.DestinationLongitude) {
				return existing, false, nil
			}
		}
		return nil, false, mapped
	}
	fulfilment, err := r.GetFulfilment(ctx, input.Fulfilment.OrganizationID, input.Fulfilment.ID)
	return fulfilment, created, err
}

func (r *CommerceFulfilmentRepoPG) GetFulfilmentByOrder(ctx context.Context, organizationID, orderID uuid.UUID) (*models.CommerceFulfilment, error) {
	var item models.CommerceFulfilment
	err := commerceFulfilmentQuery(r.db.WithContext(ctx)).
		Where("commerce_fulfilments.organization_id = ? AND commerce_fulfilments.order_id = ?", organizationID, orderID).
		First(&item).Error
	return mapCommerceFulfilmentRead(&item, err)
}

func (r *CommerceFulfilmentRepoPG) ListFulfilmentsByOrderIDs(ctx context.Context, organizationID uuid.UUID, orderIDs []uuid.UUID) ([]models.CommerceFulfilment, error) {
	if len(orderIDs) == 0 {
		return []models.CommerceFulfilment{}, nil
	}
	var items []models.CommerceFulfilment
	err := commerceFulfilmentQuery(r.db.WithContext(ctx)).
		Where("commerce_fulfilments.organization_id = ? AND commerce_fulfilments.order_id IN ?", organizationID, orderIDs).
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list commerce fulfilments by order: %w", err)
	}
	return items, nil
}

func (r *CommerceFulfilmentRepoPG) GetFulfilment(ctx context.Context, organizationID, fulfilmentID uuid.UUID) (*models.CommerceFulfilment, error) {
	var item models.CommerceFulfilment
	err := commerceFulfilmentQuery(r.db.WithContext(ctx)).
		Where("commerce_fulfilments.organization_id = ? AND commerce_fulfilments.id = ?", organizationID, fulfilmentID).
		First(&item).Error
	return mapCommerceFulfilmentRead(&item, err)
}

func (r *CommerceFulfilmentRepoPG) CreateDeliveryQuote(ctx context.Context, input CommerceCreateDeliveryQuoteInput) (*models.CommerceFulfilment, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.CommerceDeliveryQuote
		err := tx.Where("organization_id = ? AND fulfilment_id = ? AND idempotency_key = ?", input.Quote.OrganizationID, input.Quote.FulfilmentID, input.IdempotencyKey).
			First(&existing).Error
		if err == nil {
			if existing.EstimatedFeeMinor != input.Quote.EstimatedFeeMinor || existing.Currency != input.Quote.Currency ||
				existing.Source != input.Quote.Source || !sameCommerceOptionalString(existing.Provider, input.Quote.Provider) {
				return ErrCommerceConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		fulfilment, err := lockCommerceFulfilment(tx, input.Quote.OrganizationID, input.Quote.FulfilmentID)
		if err != nil {
			return err
		}
		err = tx.Where("organization_id = ? AND fulfilment_id = ? AND idempotency_key = ?", input.Quote.OrganizationID, input.Quote.FulfilmentID, input.IdempotencyKey).
			First(&existing).Error
		if err == nil {
			if existing.EstimatedFeeMinor != input.Quote.EstimatedFeeMinor || existing.Currency != input.Quote.Currency ||
				existing.Source != input.Quote.Source || !sameCommerceOptionalString(existing.Provider, input.Quote.Provider) {
				return ErrCommerceConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if fulfilment.Mode != models.FulfilmentModeMerchantRider || fulfilment.Status != models.CommerceFulfilmentStatusAwaitingQuote {
			return ErrCommerceFulfilmentState
		}
		if err := tx.Create(&input.Quote).Error; err != nil {
			return err
		}
		from := fulfilment.Status
		if err := updateCommerceFulfilmentStatus(tx, fulfilment, models.CommerceFulfilmentStatusAwaitingCustomerConfirmation, input.Now, nil); err != nil {
			return err
		}
		metadata := map[string]interface{}{"quote_id": input.Quote.ID, "source": input.Quote.Source, "estimated_fee_minor": input.Quote.EstimatedFeeMinor, "currency": input.Quote.Currency}
		if err := createCommerceFulfilmentEvent(tx, fulfilment, models.CommerceFulfilmentEventQuoteCreated, &from,
			models.CommerceFulfilmentStatusAwaitingCustomerConfirmation, models.CommerceFulfilmentActorUser, &input.ActorUserID,
			"delivery estimate is ready for customer confirmation", input.IdempotencyKey, metadata, input.Now); err != nil {
			return err
		}
		return createCommerceFulfilmentOutbox(tx, fulfilment, models.CommerceOutboxTopicDeliveryQuoteAvailable,
			input.IdempotencyKey+":notify", metadata, input.Now)
	})
	if err != nil {
		return nil, mapCommerceFulfilmentWriteError("create commerce delivery quote", err)
	}
	return r.GetFulfilment(ctx, input.Quote.OrganizationID, input.Quote.FulfilmentID)
}

func (r *CommerceFulfilmentRepoPG) DecideDeliveryQuote(ctx context.Context, input CommerceDeliveryQuoteDecisionInput) (*models.CommerceFulfilment, error) {
	quoteExpired := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if event, err := getCommerceFulfilmentEventByKey(tx, input.OrganizationID, input.FulfilmentID, input.IdempotencyKey); err == nil {
			if (input.Decision == models.CommerceDeliveryQuoteStatusAccepted && event.EventType != models.CommerceFulfilmentEventQuoteAccepted) ||
				(input.Decision == models.CommerceDeliveryQuoteStatusRejected && event.EventType != models.CommerceFulfilmentEventQuoteRejected) {
				return ErrCommerceConflict
			}
			return nil
		} else if !errors.Is(err, ErrCommerceNotFound) {
			return err
		}

		fulfilment, err := lockCommerceFulfilment(tx, input.OrganizationID, input.FulfilmentID)
		if err != nil {
			return err
		}
		if event, eventErr := getCommerceFulfilmentEventByKey(tx, input.OrganizationID, input.FulfilmentID, input.IdempotencyKey); eventErr == nil {
			if (input.Decision == models.CommerceDeliveryQuoteStatusAccepted && event.EventType != models.CommerceFulfilmentEventQuoteAccepted) ||
				(input.Decision == models.CommerceDeliveryQuoteStatusRejected && event.EventType != models.CommerceFulfilmentEventQuoteRejected) {
				return ErrCommerceConflict
			}
			return nil
		} else if !errors.Is(eventErr, ErrCommerceNotFound) {
			return eventErr
		}
		if fulfilment.Mode != models.FulfilmentModeMerchantRider || fulfilment.Status != models.CommerceFulfilmentStatusAwaitingCustomerConfirmation {
			return ErrCommerceFulfilmentState
		}
		var quote models.CommerceDeliveryQuote
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND fulfilment_id = ? AND id = ?", input.OrganizationID, input.FulfilmentID, input.QuoteID).
			First(&quote).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommerceNotFound
		}
		if err != nil {
			return err
		}
		if quote.Status != models.CommerceDeliveryQuoteStatusQuoted {
			return ErrCommerceFulfilmentState
		}
		if quote.ExpiresAt != nil && !quote.ExpiresAt.After(input.Now) {
			from := fulfilment.Status
			if err := tx.Model(&quote).Updates(map[string]interface{}{"status": models.CommerceDeliveryQuoteStatusExpired, "updated_at": input.Now}).Error; err != nil {
				return err
			}
			if err := updateCommerceFulfilmentStatus(tx, fulfilment, models.CommerceFulfilmentStatusAwaitingQuote, input.Now, nil); err != nil {
				return err
			}
			if err := createCommerceFulfilmentEvent(tx, fulfilment, models.CommerceFulfilmentEventQuoteExpired, &from,
				models.CommerceFulfilmentStatusAwaitingQuote, models.CommerceFulfilmentActorSystem, nil,
				"delivery quote expired before customer confirmation", input.IdempotencyKey, map[string]interface{}{"quote_id": quote.ID}, input.Now); err != nil {
				return err
			}
			quoteExpired = true
			return nil
		}

		from := fulfilment.Status
		target := models.CommerceFulfilmentStatusReadyForPickup
		eventType := models.CommerceFulfilmentEventQuoteRejected
		quoteUpdates := map[string]interface{}{"status": models.CommerceDeliveryQuoteStatusRejected, "rejected_at": input.Now, "updated_at": input.Now}
		if input.Decision == models.CommerceDeliveryQuoteStatusAccepted {
			target = models.CommerceFulfilmentStatusRiderRequested
			eventType = models.CommerceFulfilmentEventQuoteAccepted
			quoteUpdates = map[string]interface{}{"status": models.CommerceDeliveryQuoteStatusAccepted, "accepted_at": input.Now, "fee_status": models.CommerceDeliveryFeeStatusDue, "updated_at": input.Now}
		}
		if err := tx.Model(&quote).Updates(quoteUpdates).Error; err != nil {
			return err
		}
		if err := updateCommerceFulfilmentStatus(tx, fulfilment, target, input.Now, nil); err != nil {
			return err
		}
		metadata := map[string]interface{}{"quote_id": quote.ID, "decision": input.Decision}
		if err := createCommerceFulfilmentEvent(tx, fulfilment, eventType, &from, target, input.ActorType, input.ActorUserID,
			input.Reason, input.IdempotencyKey, metadata, input.Now); err != nil {
			return err
		}
		if input.Decision == models.CommerceDeliveryQuoteStatusAccepted {
			orderActorType := models.CommerceOrderActorUser
			if input.ActorType == models.CommerceFulfilmentActorCustomer {
				orderActorType = models.CommerceOrderActorChannel
			}
			return createCommerceOrderAuditEvent(tx, fulfilment, models.CommerceOrderEventRiderRequested,
				models.CommerceOrderStatusFulfilmentPending, orderActorType, input.ActorUserID, "customer accepted delivery estimate", input.IdempotencyKey+":order", metadata, input.Now)
		}
		orderActorType := models.CommerceOrderActorUser
		if input.ActorType == models.CommerceFulfilmentActorCustomer {
			orderActorType = models.CommerceOrderActorChannel
		}
		var order models.CommerceOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND id = ?", fulfilment.OrganizationID, fulfilment.OrderID).First(&order).Error; err != nil {
			return err
		}
		if order.Status != models.CommerceOrderStatusFulfilmentPending {
			return ErrCommerceFulfilmentState
		}
		if err := tx.Model(&order).Updates(map[string]interface{}{
			"fulfilment_mode": models.FulfilmentModeCustomerPickup,
			"status":          models.CommerceOrderStatusReadyForPickup,
			"version":         gorm.Expr("version + 1"), "updated_at": input.Now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(fulfilment).Updates(map[string]interface{}{
			"mode": models.FulfilmentModeCustomerPickup, "destination_address": nil,
			"destination_latitude": nil, "destination_longitude": nil, "updated_at": input.Now,
		}).Error; err != nil {
			return err
		}
		return createCommerceOrderAuditEvent(tx, fulfilment, models.CommerceOrderEventReadyForPickup,
			models.CommerceOrderStatusReadyForPickup, orderActorType, input.ActorUserID, "customer declined delivery and switched to pickup", input.IdempotencyKey+":order", metadata, input.Now)
	})
	if err != nil {
		return nil, mapCommerceFulfilmentWriteError("decide commerce delivery quote", err)
	}
	if quoteExpired {
		return nil, ErrCommerceFulfilmentState
	}
	return r.GetFulfilment(ctx, input.OrganizationID, input.FulfilmentID)
}

func (r *CommerceFulfilmentRepoPG) AssignRider(ctx context.Context, input CommerceAssignRiderInput) (*models.CommerceFulfilment, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.CommerceRiderAssignment
		err := tx.Where("organization_id = ? AND fulfilment_id = ? AND idempotency_key = ?", input.Assignment.OrganizationID, input.Assignment.FulfilmentID, input.IdempotencyKey).
			First(&existing).Error
		if err == nil {
			if existing.RiderPhone != input.Assignment.RiderPhone || existing.RiderName != input.Assignment.RiderName ||
				existing.Source != input.Assignment.Source || !sameCommerceOptionalString(existing.Provider, input.Assignment.Provider) {
				return ErrCommerceConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		fulfilment, err := lockCommerceFulfilment(tx, input.Assignment.OrganizationID, input.Assignment.FulfilmentID)
		if err != nil {
			return err
		}
		err = tx.Where("organization_id = ? AND fulfilment_id = ? AND idempotency_key = ?", input.Assignment.OrganizationID, input.Assignment.FulfilmentID, input.IdempotencyKey).
			First(&existing).Error
		if err == nil {
			if existing.RiderPhone != input.Assignment.RiderPhone || existing.RiderName != input.Assignment.RiderName ||
				existing.Source != input.Assignment.Source || !sameCommerceOptionalString(existing.Provider, input.Assignment.Provider) {
				return ErrCommerceConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		allowed := fulfilment.Mode == models.FulfilmentModeCustomerRider && fulfilment.Status == models.CommerceFulfilmentStatusReadyForPickup && input.Assignment.Source == models.CommerceRiderSourceCustomer
		allowed = allowed || fulfilment.Mode == models.FulfilmentModeMerchantRider && fulfilment.Status == models.CommerceFulfilmentStatusRiderRequested && input.Assignment.Source == models.CommerceRiderSourceMerchant
		if !allowed {
			return ErrCommerceFulfilmentState
		}
		if err := tx.Create(&input.Assignment).Error; err != nil {
			return err
		}
		from := fulfilment.Status
		if err := updateCommerceFulfilmentStatus(tx, fulfilment, models.CommerceFulfilmentStatusRiderAssigned, input.Now, nil); err != nil {
			return err
		}
		metadata := map[string]interface{}{"rider_assignment_id": input.Assignment.ID, "source": input.Assignment.Source}
		if err := createCommerceFulfilmentEvent(tx, fulfilment, models.CommerceFulfilmentEventRiderAssigned, &from,
			models.CommerceFulfilmentStatusRiderAssigned, models.CommerceFulfilmentActorUser, &input.ActorUserID,
			"rider identity recorded before handover", input.IdempotencyKey, metadata, input.Now); err != nil {
			return err
		}
		if err := createCommerceOrderAuditEvent(tx, fulfilment, models.CommerceOrderEventRiderAssigned,
			models.CommerceOrderStatusFulfilmentPending, models.CommerceOrderActorUser, &input.ActorUserID, "rider assigned", input.IdempotencyKey+":order", metadata, input.Now); err != nil {
			return err
		}
		return createCommerceFulfilmentOutbox(tx, fulfilment, models.CommerceOutboxTopicRiderAssigned,
			input.IdempotencyKey+":notify", metadata, input.Now)
	})
	if err != nil {
		return nil, mapCommerceFulfilmentWriteError("assign commerce rider", err)
	}
	return r.GetFulfilment(ctx, input.Assignment.OrganizationID, input.Assignment.FulfilmentID)
}

func (r *CommerceFulfilmentRepoPG) RecordArrival(ctx context.Context, input CommerceFulfilmentTransitionInput) (*models.CommerceFulfilment, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if event, err := getCommerceFulfilmentEventByKey(tx, input.OrganizationID, input.FulfilmentID, input.IdempotencyKey); err == nil {
			if event.EventType != models.CommerceFulfilmentEventCustomerArrived && event.EventType != models.CommerceFulfilmentEventRiderArrived {
				return ErrCommerceConflict
			}
			return nil
		} else if !errors.Is(err, ErrCommerceNotFound) {
			return err
		}
		item, err := lockCommerceFulfilment(tx, input.OrganizationID, input.FulfilmentID)
		if err != nil {
			return err
		}
		if event, eventErr := getCommerceFulfilmentEventByKey(tx, input.OrganizationID, input.FulfilmentID, input.IdempotencyKey); eventErr == nil {
			if event.EventType != models.CommerceFulfilmentEventCustomerArrived && event.EventType != models.CommerceFulfilmentEventRiderArrived {
				return ErrCommerceConflict
			}
			return nil
		} else if !errors.Is(eventErr, ErrCommerceNotFound) {
			return eventErr
		}
		eventType := models.CommerceFulfilmentEventCustomerArrived
		if item.Mode == models.FulfilmentModeCustomerPickup {
			if item.Status != models.CommerceFulfilmentStatusReadyForPickup {
				return ErrCommerceFulfilmentState
			}
		} else {
			if item.Status != models.CommerceFulfilmentStatusRiderAssigned {
				return ErrCommerceFulfilmentState
			}
			var assignment models.CommerceRiderAssignment
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("organization_id = ? AND fulfilment_id = ? AND status = ?", input.OrganizationID, input.FulfilmentID, models.CommerceRiderStatusAssigned).
				First(&assignment).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCommerceFulfilmentState
			}
			if err != nil {
				return err
			}
			if err := tx.Model(&assignment).Updates(map[string]interface{}{
				"status": models.CommerceRiderStatusArrived, "arrived_at": input.Now, "updated_at": input.Now,
			}).Error; err != nil {
				return err
			}
			eventType = models.CommerceFulfilmentEventRiderArrived
		}
		if err := tx.Model(item).Updates(map[string]interface{}{"version": gorm.Expr("version + 1"), "updated_at": input.Now}).Error; err != nil {
			return err
		}
		status := item.Status
		return createCommerceFulfilmentEvent(tx, item, eventType, &status, status, input.ActorType, input.ActorUserID,
			input.Reason, input.IdempotencyKey, map[string]interface{}{"mode": item.Mode}, input.Now)
	})
	if err != nil {
		return nil, mapCommerceFulfilmentWriteError("record commerce fulfilment arrival", err)
	}
	return r.GetFulfilment(ctx, input.OrganizationID, input.FulfilmentID)
}

func (r *CommerceFulfilmentRepoPG) VerifyHandover(ctx context.Context, input CommerceVerifyHandoverInput) (*models.CommerceFulfilment, error) {
	verificationFailed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if event, err := getCommerceFulfilmentEventByKey(tx, input.OrganizationID, input.FulfilmentID, input.IdempotencyKey); err == nil {
			if event.EventType == models.CommerceFulfilmentEventHandoverFailed {
				verificationFailed = true
			} else if event.EventType != models.CommerceFulfilmentEventHandedOver {
				return ErrCommerceConflict
			}
			return nil
		} else if !errors.Is(err, ErrCommerceNotFound) {
			return err
		}

		fulfilment, err := lockCommerceFulfilment(tx, input.OrganizationID, input.FulfilmentID)
		if err != nil {
			return err
		}
		if event, eventErr := getCommerceFulfilmentEventByKey(tx, input.OrganizationID, input.FulfilmentID, input.IdempotencyKey); eventErr == nil {
			if event.EventType == models.CommerceFulfilmentEventHandoverFailed {
				verificationFailed = true
			} else if event.EventType != models.CommerceFulfilmentEventHandedOver {
				return ErrCommerceConflict
			}
			return nil
		} else if !errors.Is(eventErr, ErrCommerceNotFound) {
			return eventErr
		}
		if fulfilment.Status == models.CommerceFulfilmentStatusCompleted || fulfilment.Status == models.CommerceFulfilmentStatusDelivered || fulfilment.Status == models.CommerceFulfilmentStatusOutForDelivery {
			return ErrCommerceFulfilmentState
		}
		if fulfilment.VerificationLockedUntil != nil && fulfilment.VerificationLockedUntil.After(input.Now) {
			return ErrCommerceVerificationLocked
		}
		if !fulfilment.VerificationCodeExpiresAt.After(input.Now) {
			return ErrCommerceVerificationExpired
		}
		if fulfilment.Mode == models.FulfilmentModeCustomerPickup && fulfilment.Status != models.CommerceFulfilmentStatusReadyForPickup {
			return ErrCommerceFulfilmentState
		}
		if fulfilment.Mode != models.FulfilmentModeCustomerPickup && fulfilment.Status != models.CommerceFulfilmentStatusRiderAssigned {
			return ErrCommerceFulfilmentState
		}
		if fulfilment.Mode == models.FulfilmentModeCustomerPickup {
			var arrivalCount int64
			if err := tx.Model(&models.CommerceFulfilmentEvent{}).
				Where("organization_id = ? AND fulfilment_id = ? AND event_type = ?", input.OrganizationID, input.FulfilmentID, models.CommerceFulfilmentEventCustomerArrived).
				Count(&arrivalCount).Error; err != nil {
				return err
			}
			if arrivalCount == 0 {
				return ErrCommerceFulfilmentState
			}
		}

		if !hmac.Equal(input.CandidateHash, fulfilment.VerificationCodeHash) {
			attempts := fulfilment.VerificationAttempts + 1
			updates := map[string]interface{}{"verification_attempts": attempts, "updated_at": input.Now, "version": gorm.Expr("version + 1")}
			if attempts >= commerceVerificationMaxAttempts {
				updates["verification_locked_until"] = input.Now.Add(commerceVerificationLockTime)
				updates["verification_attempts"] = 0
			}
			if err := tx.Model(fulfilment).Updates(updates).Error; err != nil {
				return err
			}
			metadata := map[string]interface{}{"attempt": attempts, "locked": attempts >= commerceVerificationMaxAttempts}
			if err := createCommerceFulfilmentEvent(tx, fulfilment, models.CommerceFulfilmentEventHandoverFailed, &fulfilment.Status,
				fulfilment.Status, models.CommerceFulfilmentActorUser, &input.ActorUserID, "incorrect handover verification code", input.IdempotencyKey, metadata, input.Now); err != nil {
				return err
			}
			verificationFailed = true
			return nil
		}

		from := fulfilment.Status
		fulfilmentUpdates := map[string]interface{}{
			"verified_at": input.Now, "verified_by_user_id": input.ActorUserID,
			"handed_over_at": input.Now, "handed_over_by_user_id": input.ActorUserID,
			"verification_attempts": 0, "verification_locked_until": nil,
		}
		orderStatus := models.CommerceOrderStatusOutForDelivery
		fulfilmentStatus := models.CommerceFulfilmentStatusOutForDelivery
		orderFrom := models.CommerceOrderStatusFulfilmentPending
		if fulfilment.Mode == models.FulfilmentModeCustomerPickup {
			orderStatus = models.CommerceOrderStatusCompleted
			fulfilmentStatus = models.CommerceFulfilmentStatusCompleted
			orderFrom = models.CommerceOrderStatusReadyForPickup
			fulfilmentUpdates["completed_at"] = input.Now
		} else {
			var assignment models.CommerceRiderAssignment
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("organization_id = ? AND fulfilment_id = ? AND status = ?", input.OrganizationID, input.FulfilmentID, models.CommerceRiderStatusArrived).
				First(&assignment).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCommerceFulfilmentState
			}
			if err != nil {
				return err
			}
			if err := tx.Model(&assignment).Updates(map[string]interface{}{"status": models.CommerceRiderStatusPickedUp, "picked_up_at": input.Now, "updated_at": input.Now}).Error; err != nil {
				return err
			}
		}
		if err := updateCommerceFulfilmentStatus(tx, fulfilment, fulfilmentStatus, input.Now, fulfilmentUpdates); err != nil {
			return err
		}
		var order models.CommerceOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", input.OrganizationID, fulfilment.OrderID).First(&order).Error; err != nil {
			return err
		}
		if order.Status != orderFrom {
			return ErrCommerceFulfilmentState
		}
		if err := transitionCommerceFulfilmentOrder(tx, &order, orderStatus, commerceOrderEventForFulfilmentHandover(fulfilment.Mode),
			"handover verified using the order verification code", input.IdempotencyKey+":order", models.CommerceOrderActorUser, &input.ActorUserID, input.Now); err != nil {
			return err
		}
		metadata := map[string]interface{}{"mode": fulfilment.Mode, "handed_over_by_user_id": input.ActorUserID}
		if err := createCommerceFulfilmentEvent(tx, fulfilment, models.CommerceFulfilmentEventHandedOver, &from,
			fulfilmentStatus, models.CommerceFulfilmentActorUser, &input.ActorUserID, "secure handover verification succeeded", input.IdempotencyKey, metadata, input.Now); err != nil {
			return err
		}
		topic := models.CommerceOutboxTopicOutForDelivery
		if fulfilment.Mode == models.FulfilmentModeCustomerPickup {
			topic = models.CommerceOutboxTopicFulfilmentDelivered
		}
		return createCommerceFulfilmentOutbox(tx, fulfilment, topic, input.IdempotencyKey+":notify", metadata, input.Now)
	})
	if err != nil {
		return nil, mapCommerceFulfilmentWriteError("verify commerce fulfilment handover", err)
	}
	if verificationFailed {
		return nil, ErrCommerceVerificationFailed
	}
	return r.GetFulfilment(ctx, input.OrganizationID, input.FulfilmentID)
}

func (r *CommerceFulfilmentRepoPG) MarkDelivered(ctx context.Context, input CommerceFulfilmentTransitionInput) (*models.CommerceFulfilment, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if event, err := getCommerceFulfilmentEventByKey(tx, input.OrganizationID, input.FulfilmentID, input.IdempotencyKey); err == nil {
			if event.EventType != models.CommerceFulfilmentEventDelivered {
				return ErrCommerceConflict
			}
			return nil
		} else if !errors.Is(err, ErrCommerceNotFound) {
			return err
		}
		fulfilment, err := lockCommerceFulfilment(tx, input.OrganizationID, input.FulfilmentID)
		if err != nil {
			return err
		}
		if event, eventErr := getCommerceFulfilmentEventByKey(tx, input.OrganizationID, input.FulfilmentID, input.IdempotencyKey); eventErr == nil {
			if event.EventType != models.CommerceFulfilmentEventDelivered {
				return ErrCommerceConflict
			}
			return nil
		} else if !errors.Is(eventErr, ErrCommerceNotFound) {
			return eventErr
		}
		if fulfilment.Mode == models.FulfilmentModeCustomerPickup || fulfilment.Status != models.CommerceFulfilmentStatusOutForDelivery {
			return ErrCommerceFulfilmentState
		}
		var assignment models.CommerceRiderAssignment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND fulfilment_id = ? AND status = ?", input.OrganizationID, input.FulfilmentID, models.CommerceRiderStatusPickedUp).First(&assignment).Error; err != nil {
			return ErrCommerceFulfilmentState
		}
		if err := tx.Model(&assignment).Updates(map[string]interface{}{"status": models.CommerceRiderStatusDelivered, "delivered_at": input.Now, "updated_at": input.Now}).Error; err != nil {
			return err
		}
		from := fulfilment.Status
		if err := updateCommerceFulfilmentStatus(tx, fulfilment, models.CommerceFulfilmentStatusDelivered, input.Now, map[string]interface{}{"delivered_at": input.Now}); err != nil {
			return err
		}
		var order models.CommerceOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", input.OrganizationID, fulfilment.OrderID).First(&order).Error; err != nil {
			return err
		}
		if order.Status != models.CommerceOrderStatusOutForDelivery {
			return ErrCommerceFulfilmentState
		}
		if err := transitionCommerceFulfilmentOrder(tx, &order, models.CommerceOrderStatusDelivered, models.CommerceOrderEventDelivered,
			input.Reason, input.IdempotencyKey+":order", models.CommerceOrderActorUser, input.ActorUserID, input.Now); err != nil {
			return err
		}
		if err := createCommerceFulfilmentEvent(tx, fulfilment, models.CommerceFulfilmentEventDelivered, &from,
			models.CommerceFulfilmentStatusDelivered, input.ActorType, input.ActorUserID, input.Reason, input.IdempotencyKey, nil, input.Now); err != nil {
			return err
		}
		return createCommerceFulfilmentOutbox(tx, fulfilment, models.CommerceOutboxTopicFulfilmentDelivered, input.IdempotencyKey+":notify", nil, input.Now)
	})
	if err != nil {
		return nil, mapCommerceFulfilmentWriteError("mark commerce fulfilment delivered", err)
	}
	return r.GetFulfilment(ctx, input.OrganizationID, input.FulfilmentID)
}

func (r *CommerceFulfilmentRepoPG) CompleteFulfilment(ctx context.Context, input CommerceFulfilmentTransitionInput) (*models.CommerceFulfilment, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if event, err := getCommerceFulfilmentEventByKey(tx, input.OrganizationID, input.FulfilmentID, input.IdempotencyKey); err == nil {
			if event.EventType != models.CommerceFulfilmentEventCompleted {
				return ErrCommerceConflict
			}
			return nil
		} else if !errors.Is(err, ErrCommerceNotFound) {
			return err
		}
		fulfilment, err := lockCommerceFulfilment(tx, input.OrganizationID, input.FulfilmentID)
		if err != nil {
			return err
		}
		if event, eventErr := getCommerceFulfilmentEventByKey(tx, input.OrganizationID, input.FulfilmentID, input.IdempotencyKey); eventErr == nil {
			if event.EventType != models.CommerceFulfilmentEventCompleted {
				return ErrCommerceConflict
			}
			return nil
		} else if !errors.Is(eventErr, ErrCommerceNotFound) {
			return eventErr
		}
		if fulfilment.Status != models.CommerceFulfilmentStatusDelivered {
			return ErrCommerceFulfilmentState
		}
		from := fulfilment.Status
		if err := updateCommerceFulfilmentStatus(tx, fulfilment, models.CommerceFulfilmentStatusCompleted, input.Now, map[string]interface{}{"completed_at": input.Now}); err != nil {
			return err
		}
		var order models.CommerceOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", input.OrganizationID, fulfilment.OrderID).First(&order).Error; err != nil {
			return err
		}
		if order.Status != models.CommerceOrderStatusDelivered {
			return ErrCommerceFulfilmentState
		}
		if err := transitionCommerceFulfilmentOrder(tx, &order, models.CommerceOrderStatusCompleted, models.CommerceOrderEventCompleted,
			input.Reason, input.IdempotencyKey+":order", models.CommerceOrderActorUser, input.ActorUserID, input.Now); err != nil {
			return err
		}
		return createCommerceFulfilmentEvent(tx, fulfilment, models.CommerceFulfilmentEventCompleted, &from,
			models.CommerceFulfilmentStatusCompleted, input.ActorType, input.ActorUserID, input.Reason, input.IdempotencyKey, nil, input.Now)
	})
	if err != nil {
		return nil, mapCommerceFulfilmentWriteError("complete commerce fulfilment", err)
	}
	return r.GetFulfilment(ctx, input.OrganizationID, input.FulfilmentID)
}

func commerceFulfilmentQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&models.CommerceFulfilment{}).
		Preload("Quotes", func(query *gorm.DB) *gorm.DB { return query.Order("created_at ASC") }).
		Preload("RiderAssignments", func(query *gorm.DB) *gorm.DB { return query.Order("created_at ASC") }).
		Preload("Events", func(query *gorm.DB) *gorm.DB { return query.Order("created_at ASC") })
}

func mapCommerceFulfilmentRead(item *models.CommerceFulfilment, err error) (*models.CommerceFulfilment, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get commerce fulfilment: %w", err)
	}
	return item, nil
}

func lockCommerceFulfilment(tx *gorm.DB, organizationID, fulfilmentID uuid.UUID) (*models.CommerceFulfilment, error) {
	var item models.CommerceFulfilment
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", organizationID, fulfilmentID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	return &item, err
}

func updateCommerceFulfilmentStatus(tx *gorm.DB, item *models.CommerceFulfilment, status string, now time.Time, additional map[string]interface{}) error {
	updates := map[string]interface{}{"status": status, "version": gorm.Expr("version + 1"), "updated_at": now}
	for key, value := range additional {
		updates[key] = value
	}
	result := tx.Model(item).Where("status = ?", item.Status).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCommerceFulfilmentState
	}
	item.Status = status
	return nil
}

func transitionCommerceFulfilmentOrder(tx *gorm.DB, order *models.CommerceOrder, targetStatus, eventType, reason, idempotencyKey, actorType string, actorUserID *uuid.UUID, now time.Time) error {
	from := order.Status
	result := tx.Model(order).Where("status = ?", from).Updates(map[string]interface{}{
		"status": targetStatus, "version": gorm.Expr("version + 1"), "updated_at": now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCommerceFulfilmentState
	}
	event := models.CommerceOrderEvent{
		ID: uuid.New(), OrganizationID: order.OrganizationID, OrderID: order.ID,
		EventType: eventType, FromStatus: &from, ToStatus: targetStatus, ActorType: actorType,
		ActorUserID: actorUserID, Reason: reason, Metadata: json.RawMessage(`{}`), IdempotencyKey: idempotencyKey, CreatedAt: now,
	}
	return tx.Create(&event).Error
}

func createCommerceOrderAuditEvent(tx *gorm.DB, fulfilment *models.CommerceFulfilment, eventType, status, actorType string, actorUserID *uuid.UUID, reason, idempotencyKey string, metadata interface{}, now time.Time) error {
	payload, err := marshalCommerceFulfilmentMetadata(metadata)
	if err != nil {
		return err
	}
	event := models.CommerceOrderEvent{
		ID: uuid.New(), OrganizationID: fulfilment.OrganizationID, OrderID: fulfilment.OrderID,
		EventType: eventType, FromStatus: &status, ToStatus: status, ActorType: actorType,
		ActorUserID: actorUserID, Reason: reason, Metadata: payload, IdempotencyKey: idempotencyKey, CreatedAt: now,
	}
	return tx.Create(&event).Error
}

func createCommerceFulfilmentEvent(tx *gorm.DB, fulfilment *models.CommerceFulfilment, eventType string, fromStatus *string, toStatus, actorType string, actorUserID *uuid.UUID, reason, idempotencyKey string, metadata interface{}, now time.Time) error {
	payload, err := marshalCommerceFulfilmentMetadata(metadata)
	if err != nil {
		return err
	}
	event := models.CommerceFulfilmentEvent{
		ID: uuid.New(), OrganizationID: fulfilment.OrganizationID, FulfilmentID: fulfilment.ID, OrderID: fulfilment.OrderID,
		EventType: eventType, FromStatus: fromStatus, ToStatus: toStatus, ActorType: actorType, ActorUserID: actorUserID,
		Reason: reason, Metadata: payload, IdempotencyKey: idempotencyKey, CreatedAt: now,
	}
	return tx.Create(&event).Error
}

func createCommerceFulfilmentOutbox(tx *gorm.DB, fulfilment *models.CommerceFulfilment, topic, deduplicationKey string, metadata interface{}, now time.Time) error {
	payloadMap := map[string]interface{}{
		"organization_id": fulfilment.OrganizationID, "fulfilment_id": fulfilment.ID,
		"order_id": fulfilment.OrderID, "store_id": fulfilment.StoreID, "customer_id": fulfilment.CustomerID,
		"mode": fulfilment.Mode,
	}
	if extra, ok := metadata.(map[string]interface{}); ok {
		for key, value := range extra {
			payloadMap[key] = value
		}
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return err
	}
	event := models.CommerceOutboxEvent{
		ID: uuid.New(), OrganizationID: fulfilment.OrganizationID, AggregateType: "commerce_fulfilment", AggregateID: fulfilment.ID,
		Topic: topic, DeduplicationKey: deduplicationKey, Payload: payload, Status: models.CommerceOutboxStatusPending, AvailableAt: now,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event).Error
}

func marshalCommerceFulfilmentMetadata(value interface{}) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage(`{}`), nil
	}
	payload, err := json.Marshal(value)
	return json.RawMessage(payload), err
}

func getCommerceFulfilmentEventByKey(tx *gorm.DB, organizationID, fulfilmentID uuid.UUID, key string) (*models.CommerceFulfilmentEvent, error) {
	var event models.CommerceFulfilmentEvent
	err := tx.Where("organization_id = ? AND fulfilment_id = ? AND idempotency_key = ?", organizationID, fulfilmentID, key).First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	return &event, err
}

func commerceOrderEventForFulfilmentStart(mode string) string {
	if mode == models.FulfilmentModeCustomerPickup {
		return models.CommerceOrderEventReadyForPickup
	}
	return models.CommerceOrderEventFulfilmentPending
}

func commerceOrderEventForFulfilmentHandover(mode string) string {
	if mode == models.FulfilmentModeCustomerPickup {
		return models.CommerceOrderEventCompleted
	}
	return models.CommerceOrderEventOutForDelivery
}

func sameCommerceOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameCommerceOptionalFloat(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func mapCommerceFulfilmentWriteError(action string, err error) error {
	if errors.Is(err, ErrCommerceNotFound) || errors.Is(err, ErrCommerceConflict) ||
		errors.Is(err, ErrCommerceFulfilmentState) || errors.Is(err, ErrCommerceVerificationFailed) ||
		errors.Is(err, ErrCommerceVerificationLocked) || errors.Is(err, ErrCommerceVerificationExpired) {
		return err
	}
	return mapCommerceWriteError(action, err)
}
