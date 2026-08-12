package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCommercePaymentState        = errors.New("invalid commerce payment state")
	ErrCommercePaymentExpired      = errors.New("commerce payment window expired")
	ErrCommercePaymentInitializing = errors.New("commerce payment is initializing")
)

type CommercePaymentSession struct {
	Invoice *models.CommerceInvoice
	Payment *models.CommercePaymentTransaction
}

type CommercePaymentPreparationInput struct {
	PaymentID         uuid.UUID
	InvoiceID         uuid.UUID
	OrganizationID    uuid.UUID
	OrderID           uuid.UUID
	InvoiceNumber     string
	Provider          string
	ProviderReference string
	IdempotencyKey    string
	PayerEmail        string
	Now               time.Time
}

type CommercePaymentPreparation struct {
	Session *CommercePaymentSession
	Created bool
	Expired bool
}

type CommercePaymentInitializationInput struct {
	OrganizationID   uuid.UUID
	PaymentID        uuid.UUID
	AuthorizationURL string
	AccessCode       string
	ProviderResponse json.RawMessage
	InitializedAt    time.Time
}

type CommerceWebhookInput struct {
	ID                uuid.UUID
	Provider          string
	EventKey          string
	EventType         string
	ProviderReference string
	Payload           json.RawMessage
	ReceivedAt        time.Time
}

type CommerceWebhookReceipt struct {
	Event     *models.CommercePaymentWebhookEvent
	Payment   *models.CommercePaymentTransaction
	Duplicate bool
	Unknown   bool
}

type CommercePaymentVerificationInput struct {
	WebhookID             uuid.UUID
	Provider              string
	Reference             string
	ProviderTransactionID string
	Status                string
	AmountMinor           int64
	Currency              string
	PaidAt                *time.Time
	ProviderResponse      json.RawMessage
}

type CommercePaymentVerificationResult struct {
	Session *CommercePaymentSession
	Outcome string
}

type CommercePaymentRepository interface {
	PreparePayment(ctx context.Context, input CommercePaymentPreparationInput) (*CommercePaymentPreparation, error)
	CompleteInitialization(ctx context.Context, input CommercePaymentInitializationInput) (*CommercePaymentSession, error)
	FailInitialization(ctx context.Context, organizationID, paymentID uuid.UUID, reason string, providerResponse json.RawMessage) error
	GetInvoice(ctx context.Context, organizationID, orderID uuid.UUID) (*models.CommerceInvoice, error)
	GetPayment(ctx context.Context, organizationID, paymentID uuid.UUID) (*CommercePaymentSession, error)
	ExpirePendingPayments(ctx context.Context, now time.Time, limit int) (int, error)
	BeginWebhook(ctx context.Context, input CommerceWebhookInput) (*CommerceWebhookReceipt, error)
	IgnoreWebhook(ctx context.Context, webhookID uuid.UUID, reason string) error
	ApplyVerification(ctx context.Context, input CommercePaymentVerificationInput) (*CommercePaymentVerificationResult, error)
}

type CommercePaymentRepoPG struct {
	db *gorm.DB
}

func NewCommercePaymentRepoPG(db *gorm.DB) *CommercePaymentRepoPG {
	return &CommercePaymentRepoPG{db: db}
}

func (r *CommercePaymentRepoPG) PreparePayment(ctx context.Context, input CommercePaymentPreparationInput) (*CommercePaymentPreparation, error) {
	result := &CommercePaymentPreparation{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order models.CommerceOrder
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND id = ?", input.OrganizationID, input.OrderID).
			First(&order).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommerceNotFound
		}
		if err != nil {
			return err
		}

		var existing models.CommercePaymentTransaction
		existingErr := tx.Where("organization_id = ? AND idempotency_key = ?", input.OrganizationID, input.IdempotencyKey).
			First(&existing).Error
		if existingErr == nil && existing.OrderID != input.OrderID {
			return ErrCommerceConflict
		}
		if existingErr != nil && !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}

		if (order.Status == models.CommerceOrderStatusPendingPayment || order.Status == models.CommerceOrderStatusPaymentFailed) && !order.PaymentExpiresAt.After(input.Now) {
			if err := expireCommerceOrderPayment(tx, &order, input.Now); err != nil {
				return err
			}
			result.Expired = true
			return nil
		}
		if existingErr == nil {
			session, err := getCommercePaymentSession(tx, input.OrganizationID, existing.ID)
			if err != nil {
				return err
			}
			result.Session = session
			return nil
		}
		if order.Status != models.CommerceOrderStatusPendingPayment && order.Status != models.CommerceOrderStatusPaymentFailed {
			return ErrCommercePaymentState
		}

		invoice, err := getOrCreateCommerceInvoice(tx, &order, input.InvoiceID, input.InvoiceNumber, input.Now)
		if err != nil {
			return err
		}
		payment := &models.CommercePaymentTransaction{
			ID: input.PaymentID, OrganizationID: input.OrganizationID, OrderID: order.ID,
			InvoiceID: invoice.ID, Provider: input.Provider, ProviderReference: input.ProviderReference,
			IdempotencyKey: input.IdempotencyKey, PayerEmail: input.PayerEmail,
			Status: models.CommercePaymentStatusInitializing, Currency: order.Currency,
			AmountMinor: order.TotalMinor, FailureReason: "", ProviderResponse: json.RawMessage(`{}`),
			ExpiresAt: order.PaymentExpiresAt,
		}
		if err := tx.Create(payment).Error; err != nil {
			return err
		}
		result.Session = &CommercePaymentSession{Invoice: invoice, Payment: payment}
		result.Created = true
		return nil
	})
	if err != nil {
		return nil, mapCommercePaymentWriteError("prepare commerce payment", err)
	}
	return result, nil
}

func (r *CommercePaymentRepoPG) CompleteInitialization(ctx context.Context, input CommercePaymentInitializationInput) (*CommercePaymentSession, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var paymentLookup models.CommercePaymentTransaction
		err := tx.
			Where("organization_id = ? AND id = ?", input.OrganizationID, input.PaymentID).
			First(&paymentLookup).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommerceNotFound
		}
		if err != nil {
			return err
		}
		var order models.CommerceOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND id = ?", paymentLookup.OrganizationID, paymentLookup.OrderID).
			First(&order).Error; err != nil {
			return err
		}
		var payment models.CommercePaymentTransaction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND id = ?", input.OrganizationID, input.PaymentID).
			First(&payment).Error; err != nil {
			return err
		}
		if payment.Status == models.CommercePaymentStatusPending || payment.Status == models.CommercePaymentStatusSucceeded {
			return nil
		}
		if payment.Status != models.CommercePaymentStatusInitializing {
			return ErrCommercePaymentState
		}
		now := input.InitializedAt.UTC()
		result := tx.Model(&payment).Updates(map[string]interface{}{
			"status": models.CommercePaymentStatusPending, "authorization_url": input.AuthorizationURL,
			"access_code": nullableCommercePaymentString(input.AccessCode), "provider_response": input.ProviderResponse,
			"initialized_at": now, "updated_at": now,
		})
		if result.Error != nil {
			return result.Error
		}

		fromStatus := order.Status
		if fromStatus != models.CommerceOrderStatusPendingPayment && fromStatus != models.CommerceOrderStatusPaymentFailed {
			return ErrCommercePaymentState
		}
		if fromStatus == models.CommerceOrderStatusPaymentFailed {
			if err := tx.Model(&order).Updates(map[string]interface{}{
				"status":  models.CommerceOrderStatusPendingPayment,
				"version": gorm.Expr("version + 1"), "updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		return createCommercePaymentOrderEvent(tx, &payment, models.CommerceOrderEventPaymentInitiated, fromStatus, models.CommerceOrderStatusPendingPayment, "payment initialized", "initiated", now)
	})
	if err != nil {
		return nil, mapCommercePaymentWriteError("complete commerce payment initialization", err)
	}
	return r.GetPayment(ctx, input.OrganizationID, input.PaymentID)
}

func (r *CommercePaymentRepoPG) FailInitialization(ctx context.Context, organizationID, paymentID uuid.UUID, reason string, providerResponse json.RawMessage) error {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&models.CommercePaymentTransaction{}).
		Where("organization_id = ? AND id = ? AND status = ?", organizationID, paymentID, models.CommercePaymentStatusInitializing).
		Updates(map[string]interface{}{
			"status": models.CommercePaymentStatusFailed, "failure_reason": truncateCommercePaymentReason(reason),
			"provider_response": providerResponse, "failed_at": now, "updated_at": now,
		})
	if result.Error != nil {
		return mapCommercePaymentWriteError("fail commerce payment initialization", result.Error)
	}
	return nil
}

func (r *CommercePaymentRepoPG) GetInvoice(ctx context.Context, organizationID, orderID uuid.UUID) (*models.CommerceInvoice, error) {
	var invoice models.CommerceInvoice
	err := r.db.WithContext(ctx).
		Preload("Items", func(query *gorm.DB) *gorm.DB { return query.Order("created_at ASC") }).
		Where("organization_id = ? AND order_id = ?", organizationID, orderID).
		First(&invoice).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get commerce invoice: %w", err)
	}
	return &invoice, nil
}

func (r *CommercePaymentRepoPG) GetPayment(ctx context.Context, organizationID, paymentID uuid.UUID) (*CommercePaymentSession, error) {
	return getCommercePaymentSession(r.db.WithContext(ctx), organizationID, paymentID)
}

func (r *CommercePaymentRepoPG) ExpirePendingPayments(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	expired := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var orders []models.CommerceOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND payment_expires_at <= ?", []string{models.CommerceOrderStatusPendingPayment, models.CommerceOrderStatusPaymentFailed}, now.UTC()).
			Order("payment_expires_at ASC").Limit(limit).Find(&orders).Error; err != nil {
			return err
		}
		for index := range orders {
			if err := expireCommerceOrderPayment(tx, &orders[index], now.UTC()); err != nil {
				return err
			}
			expired++
		}
		return nil
	})
	if err != nil {
		return 0, mapCommercePaymentWriteError("expire pending commerce payments", err)
	}
	return expired, nil
}

func (r *CommercePaymentRepoPG) BeginWebhook(ctx context.Context, input CommerceWebhookInput) (*CommerceWebhookReceipt, error) {
	receipt := &CommerceWebhookReceipt{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.CommercePaymentWebhookEvent
		err := tx.Where("provider = ? AND event_key = ?", input.Provider, input.EventKey).First(&existing).Error
		if err == nil {
			receipt.Event = &existing
			receipt.Duplicate = true
			if existing.PaymentID != nil && existing.OrganizationID != nil {
				var payment models.CommercePaymentTransaction
				if err := tx.Where("organization_id = ? AND id = ?", *existing.OrganizationID, *existing.PaymentID).First(&payment).Error; err == nil {
					receipt.Payment = &payment
				}
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var payment models.CommercePaymentTransaction
		paymentErr := tx.Where("provider = ? AND provider_reference = ?", input.Provider, input.ProviderReference).
			First(&payment).Error
		event := &models.CommercePaymentWebhookEvent{
			ID: input.ID, Provider: input.Provider, EventKey: input.EventKey,
			EventType: input.EventType, ProviderReference: input.ProviderReference,
			Status: models.CommerceWebhookStatusReceived, FailureReason: "", Payload: input.Payload,
			ReceivedAt: input.ReceivedAt.UTC(), UpdatedAt: input.ReceivedAt.UTC(),
		}
		if errors.Is(paymentErr, gorm.ErrRecordNotFound) {
			now := input.ReceivedAt.UTC()
			event.Status = models.CommerceWebhookStatusIgnored
			event.FailureReason = "payment reference is not owned by ZidiCommerce"
			event.ProcessedAt = &now
			receipt.Unknown = true
		} else if paymentErr != nil {
			return paymentErr
		} else {
			event.OrganizationID = &payment.OrganizationID
			event.OrderID = &payment.OrderID
			event.PaymentID = &payment.ID
			receipt.Payment = &payment
		}
		if err := tx.Create(event).Error; err != nil {
			return err
		}
		receipt.Event = event
		return nil
	})
	if err != nil {
		return nil, mapCommercePaymentWriteError("record commerce payment webhook", err)
	}
	return receipt, nil
}

func (r *CommercePaymentRepoPG) IgnoreWebhook(ctx context.Context, webhookID uuid.UUID, reason string) error {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&models.CommercePaymentWebhookEvent{}).
		Where("id = ? AND status = ?", webhookID, models.CommerceWebhookStatusReceived).
		Updates(map[string]interface{}{
			"status": models.CommerceWebhookStatusIgnored, "failure_reason": truncateCommercePaymentReason(reason),
			"processed_at": now, "updated_at": now,
		})
	if result.Error != nil {
		return mapCommercePaymentWriteError("ignore commerce payment webhook", result.Error)
	}
	return nil
}

func (r *CommercePaymentRepoPG) ApplyVerification(ctx context.Context, input CommercePaymentVerificationInput) (*CommercePaymentVerificationResult, error) {
	result := &CommercePaymentVerificationResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var event models.CommercePaymentWebhookEvent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND provider = ?", input.WebhookID, input.Provider).
			First(&event).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommerceNotFound
		}
		if err != nil {
			return err
		}
		if event.PaymentID == nil || event.OrganizationID == nil {
			result.Outcome = models.CommerceWebhookStatusIgnored
			return nil
		}

		var paymentLookup models.CommercePaymentTransaction
		if err := tx.Where("organization_id = ? AND id = ?", *event.OrganizationID, *event.PaymentID).
			First(&paymentLookup).Error; err != nil {
			return err
		}
		var order models.CommerceOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND id = ?", paymentLookup.OrganizationID, paymentLookup.OrderID).
			First(&order).Error; err != nil {
			return err
		}
		var payment models.CommercePaymentTransaction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND id = ?", *event.OrganizationID, *event.PaymentID).
			First(&payment).Error; err != nil {
			return err
		}
		if event.Status == models.CommerceWebhookStatusProcessed || event.Status == models.CommerceWebhookStatusIgnored || event.Status == models.CommerceWebhookStatusFailed {
			session, err := getCommercePaymentSession(tx, payment.OrganizationID, payment.ID)
			if err != nil {
				return err
			}
			result.Session = session
			result.Outcome = event.Status
			return nil
		}

		now := time.Now().UTC()
		if input.Reference != payment.ProviderReference || input.AmountMinor != payment.AmountMinor || !strings.EqualFold(input.Currency, payment.Currency) {
			reason := "verified payment reference, amount, or currency did not match the local transaction"
			if err := markCommercePaymentReviewRequired(tx, &event, &payment, input, reason, now); err != nil {
				return err
			}
			result.Outcome = models.CommercePaymentStatusReviewRequired
			return nil
		}
		if input.Status != "success" {
			if payment.Status == models.CommercePaymentStatusSucceeded {
				if err := completeCommerceWebhook(tx, &event, models.CommerceWebhookStatusIgnored, "payment was already confirmed", now); err != nil {
					return err
				}
				result.Outcome = models.CommerceWebhookStatusIgnored
				return nil
			}
			if err := applyFailedCommercePayment(tx, &event, &payment, &order, input, now); err != nil {
				return err
			}
			result.Outcome = models.CommercePaymentStatusFailed
			return nil
		}
		if payment.Status == models.CommercePaymentStatusSucceeded {
			if err := completeCommerceWebhook(tx, &event, models.CommerceWebhookStatusProcessed, "", now); err != nil {
				return err
			}
			result.Outcome = models.CommercePaymentStatusSucceeded
			return nil
		}

		if order.Status != models.CommerceOrderStatusPendingPayment && order.Status != models.CommerceOrderStatusPaymentFailed {
			reason := "verified payment arrived after the order left the payable state"
			if err := markCommercePaymentReviewRequired(tx, &event, &payment, input, reason, now); err != nil {
				return err
			}
			result.Outcome = models.CommercePaymentStatusReviewRequired
			return nil
		}
		if err := tx.SavePoint("before_payment_inventory").Error; err != nil {
			return err
		}
		if err := commitCommercePaymentInventory(tx, &order, payment.ID, now); err != nil {
			if errors.Is(err, ErrCommerceInventoryUnavailable) || errors.Is(err, ErrCommerceReservationState) {
				if rollbackErr := tx.RollbackTo("before_payment_inventory").Error; rollbackErr != nil {
					return rollbackErr
				}
				reason := "verified payment could not be matched to available reserved inventory"
				if reviewErr := markCommercePaymentReviewRequired(tx, &event, &payment, input, reason, now); reviewErr != nil {
					return reviewErr
				}
				result.Outcome = models.CommercePaymentStatusReviewRequired
				return nil
			}
			return err
		}

		providerTransactionID := nullableCommercePaymentString(input.ProviderTransactionID)
		confirmedAt := now
		if input.PaidAt != nil {
			confirmedAt = input.PaidAt.UTC()
		}
		if err := tx.Model(&payment).Updates(map[string]interface{}{
			"status": models.CommercePaymentStatusSucceeded, "provider_transaction_id": providerTransactionID,
			"provider_response": input.ProviderResponse, "failure_reason": "",
			"confirmed_at": confirmedAt, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.CommerceInvoice{}).
			Where("organization_id = ? AND id = ?", payment.OrganizationID, payment.InvoiceID).
			Updates(map[string]interface{}{
				"status": models.CommerceInvoiceStatusPaid, "paid_at": confirmedAt, "updated_at": now,
			}).Error; err != nil {
			return err
		}
		fromStatus := order.Status
		if err := tx.Model(&order).Updates(map[string]interface{}{
			"status": models.CommerceOrderStatusPaid, "version": gorm.Expr("version + 1"), "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := createCommercePaymentOrderEvent(tx, &payment, models.CommerceOrderEventPaymentConfirmed, fromStatus, models.CommerceOrderStatusPaid, "payment verified by provider", "confirmed", now); err != nil {
			return err
		}
		if err := createCommercePaymentOutboxEvents(tx, &payment, &order, now); err != nil {
			return err
		}
		if err := completeCommerceWebhook(tx, &event, models.CommerceWebhookStatusProcessed, "", now); err != nil {
			return err
		}
		result.Outcome = models.CommercePaymentStatusSucceeded
		result.Session, _ = getCommercePaymentSession(tx, payment.OrganizationID, payment.ID)
		return nil
	})
	if err != nil {
		return nil, mapCommercePaymentWriteError("apply commerce payment verification", err)
	}
	if result.Session == nil && input.WebhookID != uuid.Nil {
		var event models.CommercePaymentWebhookEvent
		if err := r.db.WithContext(ctx).Where("id = ?", input.WebhookID).First(&event).Error; err == nil && event.PaymentID != nil && event.OrganizationID != nil {
			result.Session, _ = r.GetPayment(ctx, *event.OrganizationID, *event.PaymentID)
		}
	}
	return result, nil
}

func getOrCreateCommerceInvoice(tx *gorm.DB, order *models.CommerceOrder, invoiceID uuid.UUID, invoiceNumber string, now time.Time) (*models.CommerceInvoice, error) {
	var existing models.CommerceInvoice
	err := tx.Preload("Items", func(query *gorm.DB) *gorm.DB { return query.Order("created_at ASC") }).
		Where("organization_id = ? AND order_id = ?", order.OrganizationID, order.ID).
		First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := tx.Where("organization_id = ? AND id = ?", order.OrganizationID, order.ID).
		Preload("Items", func(query *gorm.DB) *gorm.DB { return query.Order("created_at ASC") }).
		First(order).Error; err != nil {
		return nil, err
	}
	var merchant models.CommerceMerchantProfile
	if err := tx.Where("organization_id = ? AND status = ? AND deleted_at IS NULL", order.OrganizationID, models.CommerceStatusActive).First(&merchant).Error; err != nil {
		return nil, err
	}
	var store models.CommerceStore
	if err := tx.Where("organization_id = ? AND id = ? AND deleted_at IS NULL", order.OrganizationID, order.StoreID).First(&store).Error; err != nil {
		return nil, err
	}
	var customer models.CommerceCustomer
	if err := tx.Where("organization_id = ? AND id = ? AND deleted_at IS NULL", order.OrganizationID, order.CustomerID).First(&customer).Error; err != nil {
		return nil, err
	}
	invoice := &models.CommerceInvoice{
		ID: invoiceID, OrganizationID: order.OrganizationID, OrderID: order.ID,
		StoreID: order.StoreID, CustomerID: order.CustomerID,
		InvoiceNumber: invoiceNumber, Status: models.CommerceInvoiceStatusIssued,
		MerchantName: merchant.DisplayName, StoreName: store.Name,
		StoreAddress: strings.TrimSpace(strings.Join([]string{store.Address, store.City, store.State, store.CountryCode}, ", ")),
		CustomerName: customer.DisplayName, CustomerEmail: customer.Email,
		OrderNumber: order.OrderNumber, FulfilmentMode: order.FulfilmentMode, Currency: order.Currency,
		SubtotalMinor: order.SubtotalMinor, DiscountMinor: order.DiscountMinor,
		DeliveryFeeMinor: order.DeliveryFeeMinor, TotalMinor: order.TotalMinor, IssuedAt: now.UTC(),
		Items: make([]models.CommerceInvoiceItem, 0, len(order.Items)),
	}
	for _, item := range order.Items {
		invoice.Items = append(invoice.Items, models.CommerceInvoiceItem{
			ID: uuid.New(), OrganizationID: order.OrganizationID, InvoiceID: invoice.ID,
			OrderItemID: item.ID, ProductID: item.ProductID, VariantID: item.VariantID,
			ProductName: item.ProductName, VariantName: item.VariantName, SKU: item.SKU,
			Attributes: append(json.RawMessage(nil), item.Attributes...), Quantity: item.Quantity,
			UnitPriceMinor: item.UnitPriceMinor, LineTotalMinor: item.LineTotalMinor,
		})
	}
	if err := tx.Omit("Items").Create(invoice).Error; err != nil {
		return nil, err
	}
	if len(invoice.Items) > 0 {
		if err := tx.Create(&invoice.Items).Error; err != nil {
			return nil, err
		}
	}
	return invoice, nil
}

func getCommercePaymentSession(db *gorm.DB, organizationID, paymentID uuid.UUID) (*CommercePaymentSession, error) {
	var payment models.CommercePaymentTransaction
	err := db.Where("organization_id = ? AND id = ?", organizationID, paymentID).First(&payment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommerceNotFound
	}
	if err != nil {
		return nil, err
	}
	var invoice models.CommerceInvoice
	if err := db.Preload("Items", func(query *gorm.DB) *gorm.DB { return query.Order("created_at ASC") }).
		Where("organization_id = ? AND id = ?", organizationID, payment.InvoiceID).
		First(&invoice).Error; err != nil {
		return nil, err
	}
	return &CommercePaymentSession{Invoice: &invoice, Payment: &payment}, nil
}

func expireCommerceOrderPayment(tx *gorm.DB, order *models.CommerceOrder, now time.Time) error {
	if order.Status == models.CommerceOrderStatusPaymentExpired {
		return nil
	}
	if order.Status != models.CommerceOrderStatusPendingPayment && order.Status != models.CommerceOrderStatusPaymentFailed {
		return ErrCommercePaymentState
	}
	if err := releaseCommerceOrderReservations(tx, order); err != nil {
		return err
	}
	fromStatus := order.Status
	if err := tx.Model(order).Updates(map[string]interface{}{
		"status":  models.CommerceOrderStatusPaymentExpired,
		"version": gorm.Expr("version + 1"), "updated_at": now,
	}).Error; err != nil {
		return err
	}
	if err := tx.Model(&models.CommercePaymentTransaction{}).
		Where("organization_id = ? AND order_id = ? AND status IN ?", order.OrganizationID, order.ID, []string{models.CommercePaymentStatusInitializing, models.CommercePaymentStatusPending}).
		Updates(map[string]interface{}{"status": models.CommercePaymentStatusExpired, "failed_at": now, "failure_reason": "payment window expired", "updated_at": now}).Error; err != nil {
		return err
	}
	if err := tx.Model(&models.CommerceInvoice{}).
		Where("organization_id = ? AND order_id = ? AND status = ?", order.OrganizationID, order.ID, models.CommerceInvoiceStatusIssued).
		Updates(map[string]interface{}{"status": models.CommerceInvoiceStatusVoid, "voided_at": now, "updated_at": now}).Error; err != nil {
		return err
	}
	event := models.CommerceOrderEvent{
		ID: uuid.New(), OrganizationID: order.OrganizationID, OrderID: order.ID,
		EventType: models.CommerceOrderEventPaymentExpired, FromStatus: &fromStatus,
		ToStatus: models.CommerceOrderStatusPaymentExpired, ActorType: models.CommerceOrderActorSystem,
		Reason: "payment window expired", Metadata: json.RawMessage(`{}`),
		IdempotencyKey: "payment-expiry:" + order.ID.String(), CreatedAt: now,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event).Error
}

func commitCommercePaymentInventory(tx *gorm.DB, order *models.CommerceOrder, paymentID uuid.UUID, now time.Time) error {
	var items []models.CommerceOrderItem
	if err := tx.Where("organization_id = ? AND order_id = ?", order.OrganizationID, order.ID).
		Order("variant_id ASC").Find(&items).Error; err != nil {
		return err
	}
	if len(items) == 0 {
		return ErrCommerceReservationState
	}
	sort.Slice(items, func(i, j int) bool { return items[i].VariantID.String() < items[j].VariantID.String() })
	for _, item := range items {
		var reservation models.CommerceInventoryReservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND id = ?", order.OrganizationID, item.InventoryReservationID).
			First(&reservation).Error; err != nil {
			return err
		}
		if reservation.Quantity != item.Quantity || reservation.StoreID != order.StoreID || reservation.VariantID != item.VariantID {
			return ErrCommerceReservationState
		}
		if reservation.Status == models.InventoryReservationCommitted {
			continue
		}
		updates := map[string]interface{}{
			"quantity_on_hand": gorm.Expr("quantity_on_hand - ?", item.Quantity),
			"version":          gorm.Expr("version + 1"), "updated_at": now,
		}
		query := tx.Model(&models.CommerceInventoryLevel{}).
			Where("organization_id = ? AND store_id = ? AND variant_id = ?", order.OrganizationID, order.StoreID, item.VariantID)
		reservedDelta := 0
		switch reservation.Status {
		case models.InventoryReservationActive:
			query = query.Where("quantity_on_hand >= ? AND quantity_reserved >= ?", item.Quantity, item.Quantity)
			updates["quantity_reserved"] = gorm.Expr("quantity_reserved - ?", item.Quantity)
			reservedDelta = -item.Quantity
		case models.InventoryReservationReleased, models.InventoryReservationExpired:
			query = query.Where("quantity_on_hand - quantity_reserved >= ?", item.Quantity)
		default:
			return ErrCommerceReservationState
		}
		inventoryResult := query.Updates(updates)
		if inventoryResult.Error != nil {
			return inventoryResult.Error
		}
		if inventoryResult.RowsAffected != 1 {
			return ErrCommerceInventoryUnavailable
		}
		if err := tx.Model(&reservation).Updates(map[string]interface{}{
			"status": models.InventoryReservationCommitted, "committed_at": now,
			"released_at": nil, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		movement := models.CommerceInventoryMovement{
			ID: uuid.New(), OrganizationID: order.OrganizationID, StoreID: order.StoreID,
			VariantID: item.VariantID, ReservationID: &reservation.ID,
			MovementType:        models.InventoryMovementReservationCommit,
			QuantityOnHandDelta: -item.Quantity, QuantityReservedDelta: reservedDelta,
			Reference: "payment:" + paymentID.String() + ":reservation:" + reservation.ID.String() + ":commit",
			Reason:    "inventory committed after verified payment",
		}
		if err := tx.Create(&movement).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyFailedCommercePayment(tx *gorm.DB, event *models.CommercePaymentWebhookEvent, payment *models.CommercePaymentTransaction, order *models.CommerceOrder, input CommercePaymentVerificationInput, now time.Time) error {
	providerTransactionID := nullableCommercePaymentString(input.ProviderTransactionID)
	reason := "provider verification returned payment status " + input.Status
	if err := tx.Model(payment).Updates(map[string]interface{}{
		"status": models.CommercePaymentStatusFailed, "provider_transaction_id": providerTransactionID,
		"provider_response": input.ProviderResponse, "failure_reason": reason,
		"failed_at": now, "updated_at": now,
	}).Error; err != nil {
		return err
	}
	if order.Status == models.CommerceOrderStatusPendingPayment {
		fromStatus := order.Status
		if err := tx.Model(order).Updates(map[string]interface{}{
			"status":  models.CommerceOrderStatusPaymentFailed,
			"version": gorm.Expr("version + 1"), "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := createCommercePaymentOrderEvent(tx, payment, models.CommerceOrderEventPaymentFailed, fromStatus, models.CommerceOrderStatusPaymentFailed, reason, "failed", now); err != nil {
			return err
		}
	}
	return completeCommerceWebhook(tx, event, models.CommerceWebhookStatusProcessed, "", now)
}

func markCommercePaymentReviewRequired(tx *gorm.DB, event *models.CommercePaymentWebhookEvent, payment *models.CommercePaymentTransaction, input CommercePaymentVerificationInput, reason string, now time.Time) error {
	if err := tx.Model(payment).Updates(map[string]interface{}{
		"status":                  models.CommercePaymentStatusReviewRequired,
		"provider_transaction_id": nullableCommercePaymentString(input.ProviderTransactionID),
		"provider_response":       input.ProviderResponse, "failure_reason": reason, "updated_at": now,
	}).Error; err != nil {
		return err
	}
	return completeCommerceWebhook(tx, event, models.CommerceWebhookStatusFailed, reason, now)
}

func completeCommerceWebhook(tx *gorm.DB, event *models.CommercePaymentWebhookEvent, status, reason string, now time.Time) error {
	return tx.Model(event).Updates(map[string]interface{}{
		"status": status, "failure_reason": truncateCommercePaymentReason(reason),
		"processed_at": now, "updated_at": now,
	}).Error
}

func createCommercePaymentOrderEvent(tx *gorm.DB, payment *models.CommercePaymentTransaction, eventType, fromStatus, toStatus, reason, suffix string, now time.Time) error {
	event := models.CommerceOrderEvent{
		ID: uuid.New(), OrganizationID: payment.OrganizationID, OrderID: payment.OrderID,
		EventType: eventType, FromStatus: &fromStatus, ToStatus: toStatus,
		ActorType: models.CommerceOrderActorPayment, Reason: reason, Metadata: json.RawMessage(`{}`),
		IdempotencyKey: "payment:" + payment.ID.String() + ":" + suffix, CreatedAt: now,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event).Error
}

func createCommercePaymentOutboxEvents(tx *gorm.DB, payment *models.CommercePaymentTransaction, order *models.CommerceOrder, now time.Time) error {
	payload, err := json.Marshal(map[string]string{
		"organization_id": payment.OrganizationID.String(), "order_id": payment.OrderID.String(),
		"invoice_id": payment.InvoiceID.String(), "payment_id": payment.ID.String(),
		"store_id": order.StoreID.String(), "customer_id": order.CustomerID.String(),
	})
	if err != nil {
		return err
	}
	for _, topic := range []string{models.CommerceOutboxTopicPaymentCustomer, models.CommerceOutboxTopicPaymentStore} {
		event := models.CommerceOutboxEvent{
			ID: uuid.New(), OrganizationID: payment.OrganizationID, AggregateType: "commerce_order",
			AggregateID: payment.OrderID, Topic: topic,
			DeduplicationKey: "payment:" + payment.ID.String() + ":" + topic,
			Payload:          payload, Status: models.CommerceOutboxStatusPending, AvailableAt: now,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event).Error; err != nil {
			return err
		}
	}
	return nil
}

func nullableCommercePaymentString(value string) interface{} {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func truncateCommercePaymentReason(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1000 {
		return value[:1000]
	}
	return value
}

func mapCommercePaymentWriteError(action string, err error) error {
	if errors.Is(err, ErrCommerceNotFound) || errors.Is(err, ErrCommerceConflict) ||
		errors.Is(err, ErrCommerceInventoryUnavailable) || errors.Is(err, ErrCommerceReservationState) ||
		errors.Is(err, ErrCommercePaymentState) || errors.Is(err, ErrCommercePaymentExpired) ||
		errors.Is(err, ErrCommercePaymentInitializing) {
		return err
	}
	return mapCommerceWriteError(action, err)
}
