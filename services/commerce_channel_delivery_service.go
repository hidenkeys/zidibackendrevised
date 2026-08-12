package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"
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
	emailSender    messaging.EmailSender
	now            func() time.Time
}

func NewCommerceChannelDeliveryService(repo repository.CommerceChannelRepository, sender messaging.WhatsAppSender, orderRepo repository.CommerceOrderRepository, fulfilmentRepo repository.CommerceFulfilmentRepository, fulfilment commerceChannelFulfilment, emailSenders ...messaging.EmailSender) *CommerceChannelDeliveryService {
	service := &CommerceChannelDeliveryService{
		repo: repo, sender: sender, orderRepo: orderRepo, fulfilmentRepo: fulfilmentRepo,
		fulfilment: fulfilment, now: func() time.Time { return time.Now().UTC() },
	}
	if len(emailSenders) > 0 {
		service.emailSender = emailSenders[0]
	}
	return service
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
		customerID, reply, email, buildErr := s.notificationReply(ctx, event)
		if buildErr == nil {
			buildErr = s.repo.QueueOutboxNotification(ctx, event, customerID, reply, email, s.now().UTC())
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
		buttons, imageURL, parseErr := commerceOutboundContent(message.Payload)
		if parseErr != nil {
			_ = s.repo.MarkOutboundMessageFailed(ctx, message.ID, parseErr.Error(), commerceChannelRetryAt(now, message.Attempts))
			continue
		}
		providerID, sendErr := s.sender.Send(ctx, messaging.WhatsAppOutboundMessage{
			PhoneNumberID: configuration.ProviderAccountID, To: message.RecipientID, Body: message.Body, Buttons: buttons, ImageURL: imageURL,
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
	if s.emailSender != nil {
		emails, emailErr := s.repo.ClaimEmailMessages(ctx, limit, s.now().UTC())
		if emailErr != nil {
			return processed, emailErr
		}
		for index := range emails {
			email := &emails[index]
			sendErr := s.emailSender.Send(ctx, messaging.EmailMessage{To: email.Recipient, Subject: email.Subject, HTMLBody: email.HTMLBody})
			if sendErr != nil {
				_ = s.repo.MarkEmailMessageFailed(ctx, email.ID, sendErr.Error(), commerceChannelRetryAt(now, email.Attempts))
				continue
			}
			if err := s.repo.MarkEmailMessageSent(ctx, email.ID, s.now().UTC()); err != nil {
				return processed, err
			}
			processed++
		}
	}
	return processed, nil
}

func (s *CommerceChannelDeliveryService) notificationReply(ctx context.Context, event *models.CommerceOutboxEvent) (uuid.UUID, repository.CommerceChannelReply, *repository.CommerceEmailNotification, error) {
	var payload struct {
		CustomerID   uuid.UUID `json:"customer_id"`
		OrderID      uuid.UUID `json:"order_id"`
		FulfilmentID uuid.UUID `json:"fulfilment_id"`
		QuoteID      uuid.UUID `json:"quote_id"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.CustomerID == uuid.Nil {
		return uuid.Nil, repository.CommerceChannelReply{}, nil, errors.New("commerce notification payload has no customer")
	}
	order, err := s.orderRepo.GetOrder(ctx, event.OrganizationID, payload.OrderID)
	if err != nil {
		return uuid.Nil, repository.CommerceChannelReply{}, nil, err
	}
	if order.CustomerID != payload.CustomerID {
		return uuid.Nil, repository.CommerceChannelReply{}, nil, ErrCommerceForbidden
	}
	email := commerceOrderEmail(order, event.Topic)

	switch event.Topic {
	case models.CommerceOutboxTopicPaymentCustomer:
		return payload.CustomerID, repository.CommerceChannelReply{Body: fmt.Sprintf("Payment confirmed for order %s. The store can now begin processing it.", order.OrderNumber)}, email, nil
	case models.CommerceOutboxTopicFulfilmentReady:
		code, err := s.fulfilment.RevealVerificationCode(ctx, event.OrganizationID, payload.CustomerID, payload.FulfilmentID)
		if err != nil {
			return uuid.Nil, repository.CommerceChannelReply{}, nil, err
		}
		return payload.CustomerID, repository.CommerceChannelReply{Body: fmt.Sprintf("Order %s is ready. Your handover code is %s. Share it only when you receive the order.", order.OrderNumber, code)}, email, nil
	case models.CommerceOutboxTopicDeliveryQuoteAvailable:
		item, err := s.fulfilmentRepo.GetFulfilment(ctx, event.OrganizationID, payload.FulfilmentID)
		if err != nil {
			return uuid.Nil, repository.CommerceChannelReply{}, nil, err
		}
		var quote *models.CommerceDeliveryQuote
		for index := range item.Quotes {
			if item.Quotes[index].ID == payload.QuoteID {
				quote = &item.Quotes[index]
				break
			}
		}
		if quote == nil || quote.Status != models.CommerceDeliveryQuoteStatusQuoted {
			return uuid.Nil, repository.CommerceChannelReply{}, nil, repository.ErrCommerceFulfilmentState
		}
		eta := ""
		if quote.DurationSeconds != nil && *quote.DurationSeconds > 0 {
			eta = fmt.Sprintf(" Estimated arrival: about %d minutes.", (*quote.DurationSeconds+59)/60)
		}
		return payload.CustomerID, repository.CommerceChannelReply{
			Body: fmt.Sprintf("Delivery for order %s is estimated at %s %s, paid directly to the rider.%s Continue with delivery?", order.OrderNumber, quote.Currency, formatCommerceMinor(quote.EstimatedFeeMinor), eta),
			Options: []repository.CommerceChannelReplyOption{
				{ID: "quote:accepted:" + item.ID.String() + ":" + quote.ID.String(), Title: "Use delivery"},
				{ID: "quote:rejected:" + item.ID.String() + ":" + quote.ID.String(), Title: "Pickup instead"},
			},
		}, email, nil
	case models.CommerceOutboxTopicRiderAssigned:
		item, err := s.fulfilmentRepo.GetFulfilment(ctx, event.OrganizationID, payload.FulfilmentID)
		if err != nil {
			return uuid.Nil, repository.CommerceChannelReply{}, nil, err
		}
		code, err := s.fulfilment.RevealVerificationCode(ctx, event.OrganizationID, payload.CustomerID, payload.FulfilmentID)
		if err != nil {
			return uuid.Nil, repository.CommerceChannelReply{}, nil, err
		}
		for index := len(item.RiderAssignments) - 1; index >= 0; index-- {
			assignment := item.RiderAssignments[index]
			if assignment.Status == models.CommerceRiderStatusAssigned {
				body := fmt.Sprintf("%s has been assigned to order %s. Rider phone: %s. Your handover code is %s. Share it only when you receive the order.", assignment.RiderName, order.OrderNumber, assignment.RiderPhone, code)
				if assignment.TrackingURL != nil {
					body += " Track: " + *assignment.TrackingURL
				}
				return payload.CustomerID, repository.CommerceChannelReply{Body: body}, email, nil
			}
		}
		return uuid.Nil, repository.CommerceChannelReply{}, nil, repository.ErrCommerceFulfilmentState
	case models.CommerceOutboxTopicHandoverCodeReminder:
		code, err := s.fulfilment.RevealVerificationCode(ctx, event.OrganizationID, payload.CustomerID, payload.FulfilmentID)
		if err != nil {
			return uuid.Nil, repository.CommerceChannelReply{}, nil, err
		}
		return payload.CustomerID, repository.CommerceChannelReply{
			Body: fmt.Sprintf("Reminder for order %s: your handover code is %s. Share it only when you receive the order.", order.OrderNumber, code),
		}, email, nil
	case models.CommerceOutboxTopicOutForDelivery:
		return payload.CustomerID, repository.CommerceChannelReply{Body: fmt.Sprintf("Order %s is now out for delivery.", order.OrderNumber)}, email, nil
	case models.CommerceOutboxTopicFulfilmentDelivered:
		return payload.CustomerID, repository.CommerceChannelReply{Body: fmt.Sprintf("Order %s has been delivered. Thank you.", order.OrderNumber)}, email, nil
	default:
		return uuid.Nil, repository.CommerceChannelReply{}, nil, fmt.Errorf("unsupported customer notification topic %q", event.Topic)
	}
}

func commerceOrderEmail(order *models.CommerceOrder, topic string) *repository.CommerceEmailNotification {
	if order == nil || order.CustomerEmail == nil || strings.TrimSpace(*order.CustomerEmail) == "" {
		return nil
	}
	status := "Order update"
	switch topic {
	case models.CommerceOutboxTopicPaymentCustomer:
		status = "Payment confirmed"
	case models.CommerceOutboxTopicFulfilmentReady:
		status = "Order ready"
	case models.CommerceOutboxTopicDeliveryQuoteAvailable:
		status = "Delivery quote available"
	case models.CommerceOutboxTopicRiderAssigned:
		status = "Rider assigned"
	case models.CommerceOutboxTopicHandoverCodeReminder:
		status = "Handover code reminder"
	case models.CommerceOutboxTopicOutForDelivery:
		status = "Out for delivery"
	case models.CommerceOutboxTopicFulfilmentDelivered:
		status = "Order delivered"
	}
	name := strings.TrimSpace(order.CustomerName)
	if name == "" {
		name = "Customer"
	}
	body := fmt.Sprintf("<p>Hello %s,</p><p><strong>%s</strong> for order <strong>%s</strong>.</p><p>Total: %s %s</p><p>Thank you for ordering with us.</p>",
		html.EscapeString(name), html.EscapeString(status), html.EscapeString(order.OrderNumber), html.EscapeString(order.Currency), html.EscapeString(formatCommerceMinor(order.TotalMinor)))
	return &repository.CommerceEmailNotification{
		OrderID: order.ID, Recipient: strings.TrimSpace(*order.CustomerEmail),
		Subject: status + " - " + order.OrderNumber, HTMLBody: body,
	}
}

func commerceOutboundContent(payload json.RawMessage) ([]messaging.WhatsAppButton, string, error) {
	var object struct {
		Buttons  []repository.CommerceChannelReplyOption `json:"buttons"`
		ImageURL string                                  `json:"image_url"`
	}
	if len(payload) == 0 {
		return nil, "", nil
	}
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, "", fmt.Errorf("decode outbound WhatsApp options: %w", err)
	}
	buttons := make([]messaging.WhatsAppButton, 0, len(object.Buttons))
	for _, option := range object.Buttons {
		buttons = append(buttons, messaging.WhatsAppButton{ID: option.ID, Title: option.Title})
	}
	return buttons, strings.TrimSpace(object.ImageURL), nil
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
