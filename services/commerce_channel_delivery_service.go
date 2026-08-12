package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/messaging"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type CommerceChannelDeliveryService struct {
	repo           repository.CommerceChannelRepository
	sender         messaging.WhatsAppSender
	orderRepo      repository.CommerceOrderRepository
	fulfilmentRepo repository.CommerceFulfilmentRepository
	fulfilment     commerceChannelFulfilment
	now            func() time.Time
}

func NewCommerceChannelDeliveryService(repo repository.CommerceChannelRepository, sender messaging.WhatsAppSender, orderRepo repository.CommerceOrderRepository, fulfilmentRepo repository.CommerceFulfilmentRepository, fulfilment commerceChannelFulfilment) *CommerceChannelDeliveryService {
	return &CommerceChannelDeliveryService{
		repo: repo, sender: sender, orderRepo: orderRepo, fulfilmentRepo: fulfilmentRepo,
		fulfilment: fulfilment, now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *CommerceChannelDeliveryService) DispatchOnce(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	processed := 0
	now := s.now().UTC()
	events, err := s.repo.ClaimCustomerOutboxEvents(ctx, limit, now)
	if err != nil {
		return processed, err
	}
	for index := range events {
		event := &events[index]
		customerID, reply, buildErr := s.notificationReply(ctx, event)
		if buildErr == nil {
			buildErr = s.repo.QueueOutboxNotification(ctx, event, customerID, reply, s.now().UTC())
		}
		if buildErr != nil {
			_ = s.repo.MarkOutboxEventFailed(ctx, event.ID, buildErr.Error(), commerceChannelRetryAt(now, event.Attempts))
			continue
		}
		processed++
	}

	messages, err := s.repo.ClaimOutboundMessages(ctx, limit, s.now().UTC())
	if err != nil {
		return processed, err
	}
	for index := range messages {
		message := &messages[index]
		configuration, lookupErr := s.repo.GetChannelConfiguration(ctx, message.OrganizationID, models.CommerceChannelWhatsApp)
		if lookupErr == nil && configuration.ID != message.ChannelConfigurationID {
			lookupErr = repository.ErrCommerceNotFound
		}
		if lookupErr == nil && configuration.Status != models.CommerceStatusActive {
			lookupErr = errors.New("commerce WhatsApp channel is inactive")
		}
		if lookupErr != nil {
			_ = s.repo.MarkOutboundMessageFailed(ctx, message.ID, lookupErr.Error(), commerceChannelRetryAt(now, message.Attempts))
			continue
		}
		buttons, parseErr := commerceOutboundButtons(message.Payload)
		if parseErr != nil {
			_ = s.repo.MarkOutboundMessageFailed(ctx, message.ID, parseErr.Error(), commerceChannelRetryAt(now, message.Attempts))
			continue
		}
		providerID, sendErr := s.sender.Send(ctx, messaging.WhatsAppOutboundMessage{
			PhoneNumberID: configuration.ProviderAccountID, To: message.RecipientID, Body: message.Body, Buttons: buttons,
		})
		if sendErr != nil {
			_ = s.repo.MarkOutboundMessageFailed(ctx, message.ID, sendErr.Error(), commerceChannelRetryAt(now, message.Attempts))
			continue
		}
		if err := s.repo.MarkOutboundMessageSent(ctx, message.ID, providerID, s.now().UTC()); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (s *CommerceChannelDeliveryService) notificationReply(ctx context.Context, event *models.CommerceOutboxEvent) (uuid.UUID, repository.CommerceChannelReply, error) {
	var payload struct {
		CustomerID   uuid.UUID `json:"customer_id"`
		OrderID      uuid.UUID `json:"order_id"`
		FulfilmentID uuid.UUID `json:"fulfilment_id"`
		QuoteID      uuid.UUID `json:"quote_id"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.CustomerID == uuid.Nil {
		return uuid.Nil, repository.CommerceChannelReply{}, errors.New("commerce notification payload has no customer")
	}
	order, err := s.orderRepo.GetOrder(ctx, event.OrganizationID, payload.OrderID)
	if err != nil {
		return uuid.Nil, repository.CommerceChannelReply{}, err
	}
	if order.CustomerID != payload.CustomerID {
		return uuid.Nil, repository.CommerceChannelReply{}, ErrCommerceForbidden
	}

	switch event.Topic {
	case models.CommerceOutboxTopicPaymentCustomer:
		return payload.CustomerID, repository.CommerceChannelReply{Body: fmt.Sprintf("Payment confirmed for order %s. The store can now begin processing it.", order.OrderNumber)}, nil
	case models.CommerceOutboxTopicFulfilmentReady:
		code, err := s.fulfilment.RevealVerificationCode(ctx, event.OrganizationID, payload.CustomerID, payload.FulfilmentID)
		if err != nil {
			return uuid.Nil, repository.CommerceChannelReply{}, err
		}
		return payload.CustomerID, repository.CommerceChannelReply{Body: fmt.Sprintf("Order %s is ready. Your handover code is %s. Share it only when you receive the order.", order.OrderNumber, code)}, nil
	case models.CommerceOutboxTopicDeliveryQuoteAvailable:
		item, err := s.fulfilmentRepo.GetFulfilment(ctx, event.OrganizationID, payload.FulfilmentID)
		if err != nil {
			return uuid.Nil, repository.CommerceChannelReply{}, err
		}
		var quote *models.CommerceDeliveryQuote
		for index := range item.Quotes {
			if item.Quotes[index].ID == payload.QuoteID {
				quote = &item.Quotes[index]
				break
			}
		}
		if quote == nil || quote.Status != models.CommerceDeliveryQuoteStatusQuoted {
			return uuid.Nil, repository.CommerceChannelReply{}, repository.ErrCommerceFulfilmentState
		}
		return payload.CustomerID, repository.CommerceChannelReply{
			Body: fmt.Sprintf("Delivery for order %s is estimated at %s %s, paid directly to the rider. Accept this quote?", order.OrderNumber, quote.Currency, formatCommerceMinor(quote.EstimatedFeeMinor)),
			Options: []repository.CommerceChannelReplyOption{
				{ID: "quote:accepted:" + item.ID.String() + ":" + quote.ID.String(), Title: "Accept"},
				{ID: "quote:rejected:" + item.ID.String() + ":" + quote.ID.String(), Title: "Reject"},
			},
		}, nil
	case models.CommerceOutboxTopicRiderAssigned:
		item, err := s.fulfilmentRepo.GetFulfilment(ctx, event.OrganizationID, payload.FulfilmentID)
		if err != nil {
			return uuid.Nil, repository.CommerceChannelReply{}, err
		}
		for index := len(item.RiderAssignments) - 1; index >= 0; index-- {
			assignment := item.RiderAssignments[index]
			if assignment.Status == models.CommerceRiderStatusAssigned {
				body := fmt.Sprintf("%s has been assigned to order %s. Rider phone: %s.", assignment.RiderName, order.OrderNumber, assignment.RiderPhone)
				if assignment.TrackingURL != nil {
					body += " Track: " + *assignment.TrackingURL
				}
				return payload.CustomerID, repository.CommerceChannelReply{Body: body}, nil
			}
		}
		return uuid.Nil, repository.CommerceChannelReply{}, repository.ErrCommerceFulfilmentState
	case models.CommerceOutboxTopicOutForDelivery:
		return payload.CustomerID, repository.CommerceChannelReply{Body: fmt.Sprintf("Order %s is now out for delivery.", order.OrderNumber)}, nil
	case models.CommerceOutboxTopicFulfilmentDelivered:
		return payload.CustomerID, repository.CommerceChannelReply{Body: fmt.Sprintf("Order %s has been delivered. Thank you.", order.OrderNumber)}, nil
	default:
		return uuid.Nil, repository.CommerceChannelReply{}, fmt.Errorf("unsupported customer notification topic %q", event.Topic)
	}
}

func commerceOutboundButtons(payload json.RawMessage) ([]messaging.WhatsAppButton, error) {
	var object struct {
		Buttons []repository.CommerceChannelReplyOption `json:"buttons"`
	}
	if len(payload) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, fmt.Errorf("decode outbound WhatsApp options: %w", err)
	}
	buttons := make([]messaging.WhatsAppButton, 0, len(object.Buttons))
	for _, option := range object.Buttons {
		buttons = append(buttons, messaging.WhatsAppButton{ID: option.ID, Title: option.Title})
	}
	return buttons, nil
}

func commerceChannelRetryAt(now time.Time, attempts int) time.Time {
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Second * time.Duration(1<<minCommerceChannelInt(attempts-1, 6))
	return now.Add(delay)
}

func minCommerceChannelInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
