package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/services"
)

func (s Server) StartCommerceFulfilment(c *fiber.Ctx, orderID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.StartCommerceFulfilmentJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	item, created, err := s.commerceFulfilmentService.StartFulfilment(c.UserContext(), actor, request.OrganizationId, orderID, services.StartCommerceFulfilmentInput{
		DestinationAddress: optionalString(request.DestinationAddress), DestinationLatitude: request.DestinationLatitude,
		DestinationLongitude: request.DestinationLongitude, IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return commerceError(c, err)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return c.Status(status).JSON(commerceFulfilmentResponse(item))
}

func (s Server) GetCommerceOrderFulfilment(c *fiber.Ctx, orderID uuid.UUID, params api.GetCommerceOrderFulfilmentParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	item, err := s.commerceFulfilmentService.GetOrderFulfilment(c.UserContext(), actor, params.OrganizationId, orderID)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceFulfilmentResponse(item))
}

func (s Server) CreateCommerceDeliveryQuote(c *fiber.Ctx, fulfilmentID api.CommerceFulfilmentId) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.CreateCommerceDeliveryQuoteJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	item, err := s.commerceFulfilmentService.CreateDeliveryQuote(c.UserContext(), actor, request.OrganizationId, fulfilmentID, services.CreateCommerceDeliveryQuoteInput{
		Source: string(request.Source), Provider: optionalString(request.Provider), EstimatedFeeMinor: request.EstimatedFeeMinor,
		DistanceMeters: request.DistanceMeters, DurationSeconds: request.DurationSeconds, ExpiresAt: request.ExpiresAt,
		IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceFulfilmentResponse(item))
}

func (s Server) DecideCommerceDeliveryQuote(c *fiber.Ctx, fulfilmentID api.CommerceFulfilmentId, quoteID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.DecideCommerceDeliveryQuoteJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	item, err := s.commerceFulfilmentService.DecideDeliveryQuote(c.UserContext(), actor, request.OrganizationId, fulfilmentID, quoteID, services.DecideCommerceDeliveryQuoteInput{
		Decision: string(request.Decision), Reason: optionalString(request.Reason), IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceFulfilmentResponse(item))
}

func (s Server) AssignCommerceRider(c *fiber.Ctx, fulfilmentID api.CommerceFulfilmentId) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.AssignCommerceRiderJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	item, err := s.commerceFulfilmentService.AssignRider(c.UserContext(), actor, request.OrganizationId, fulfilmentID, services.AssignCommerceRiderInput{
		Source: string(request.Source), Provider: optionalString(request.Provider), ProviderAssignmentID: optionalString(request.ProviderAssignmentId),
		RiderName: request.RiderName, RiderPhone: request.RiderPhone, VehicleDescription: optionalString(request.VehicleDescription),
		TrackingURL: optionalString(request.TrackingUrl), IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceFulfilmentResponse(item))
}

func (s Server) VerifyCommerceFulfilmentHandover(c *fiber.Ctx, fulfilmentID api.CommerceFulfilmentId) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.VerifyCommerceFulfilmentHandoverJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	item, err := s.commerceFulfilmentService.VerifyHandover(c.UserContext(), actor, request.OrganizationId, fulfilmentID, services.VerifyCommerceHandoverInput{
		VerificationCode: request.VerificationCode, IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceFulfilmentResponse(item))
}

func (s Server) ResendCommerceFulfilmentHandoverCode(c *fiber.Ctx, fulfilmentID api.CommerceFulfilmentId) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.TransitionCommerceFulfilmentRequest
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	item, err := s.commerceFulfilmentService.ResendHandoverCode(c.UserContext(), actor, request.OrganizationId, fulfilmentID, services.TransitionCommerceFulfilmentInput{
		Reason: optionalString(request.Reason), IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceFulfilmentResponse(item))
}

func (s Server) RecordCommerceFulfilmentArrival(c *fiber.Ctx, fulfilmentID api.CommerceFulfilmentId) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.TransitionCommerceFulfilmentRequest
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	item, err := s.commerceFulfilmentService.RecordArrival(c.UserContext(), actor, request.OrganizationId, fulfilmentID, services.TransitionCommerceFulfilmentInput{
		Reason: optionalString(request.Reason), IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceFulfilmentResponse(item))
}

func (s Server) MarkCommerceFulfilmentDelivered(c *fiber.Ctx, fulfilmentID api.CommerceFulfilmentId) error {
	return s.transitionCommerceFulfilment(c, fulfilmentID, false)
}

func (s Server) CompleteCommerceFulfilment(c *fiber.Ctx, fulfilmentID api.CommerceFulfilmentId) error {
	return s.transitionCommerceFulfilment(c, fulfilmentID, true)
}

func (s Server) transitionCommerceFulfilment(c *fiber.Ctx, fulfilmentID uuid.UUID, complete bool) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.TransitionCommerceFulfilmentRequest
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	input := services.TransitionCommerceFulfilmentInput{Reason: optionalString(request.Reason), IdempotencyKey: request.IdempotencyKey}
	var item *models.CommerceFulfilment
	if complete {
		item, err = s.commerceFulfilmentService.CompleteFulfilment(c.UserContext(), actor, request.OrganizationId, fulfilmentID, input)
	} else {
		item, err = s.commerceFulfilmentService.MarkDelivered(c.UserContext(), actor, request.OrganizationId, fulfilmentID, input)
	}
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceFulfilmentResponse(item))
}

func commerceFulfilmentResponse(item *models.CommerceFulfilment) api.CommerceFulfilment {
	quotes := make([]api.CommerceDeliveryQuote, 0, len(item.Quotes))
	for _, quote := range item.Quotes {
		quotes = append(quotes, api.CommerceDeliveryQuote{
			Id: quote.ID, Source: api.CommerceDeliveryQuoteSource(quote.Source), Provider: quote.Provider,
			ProviderQuoteId: quote.ProviderQuoteID, Status: api.CommerceDeliveryQuoteStatus(quote.Status),
			PickupAddress: quote.PickupAddress, DestinationAddress: quote.DestinationAddress,
			DistanceMeters: quote.DistanceMeters, DurationSeconds: quote.DurationSeconds,
			EstimatedFeeMinor: quote.EstimatedFeeMinor, Currency: quote.Currency,
			FeePaymentMode: api.CommerceDeliveryQuoteFeePaymentMode(quote.FeePaymentMode), FeeStatus: api.CommerceDeliveryQuoteFeeStatus(quote.FeeStatus),
			ExpiresAt: quote.ExpiresAt, AcceptedAt: quote.AcceptedAt, RejectedAt: quote.RejectedAt,
			CreatedAt: quote.CreatedAt, UpdatedAt: quote.UpdatedAt,
		})
	}
	assignments := make([]api.CommerceRiderAssignment, 0, len(item.RiderAssignments))
	for _, assignment := range item.RiderAssignments {
		assignments = append(assignments, api.CommerceRiderAssignment{
			Id: assignment.ID, Source: api.CommerceRiderAssignmentSource(assignment.Source), Provider: assignment.Provider,
			ProviderAssignmentId: assignment.ProviderAssignmentID, RiderName: assignment.RiderName, RiderPhone: assignment.RiderPhone,
			VehicleDescription: assignment.VehicleDescription, TrackingUrl: assignment.TrackingURL,
			Status: api.CommerceRiderAssignmentStatus(assignment.Status), AssignedByUserId: assignment.AssignedByUserID,
			ArrivedAt: assignment.ArrivedAt, PickedUpAt: assignment.PickedUpAt, DeliveredAt: assignment.DeliveredAt, CreatedAt: assignment.CreatedAt, UpdatedAt: assignment.UpdatedAt,
		})
	}
	events := make([]api.CommerceFulfilmentEvent, 0, len(item.Events))
	for _, event := range item.Events {
		metadata := map[string]interface{}{}
		_ = json.Unmarshal(event.Metadata, &metadata)
		var fromStatus *api.CommerceFulfilmentStatus
		if event.FromStatus != nil {
			value := api.CommerceFulfilmentStatus(*event.FromStatus)
			fromStatus = &value
		}
		events = append(events, api.CommerceFulfilmentEvent{
			Id: event.ID, EventType: event.EventType, FromStatus: fromStatus, ToStatus: api.CommerceFulfilmentStatus(event.ToStatus),
			ActorType: api.CommerceFulfilmentEventActorType(event.ActorType), ActorUserId: event.ActorUserID,
			Reason: event.Reason, Metadata: metadata, CreatedAt: event.CreatedAt,
		})
	}
	return api.CommerceFulfilment{
		Id: item.ID, OrganizationId: item.OrganizationID, OrderId: item.OrderID, StoreId: item.StoreID, CustomerId: item.CustomerID,
		Mode: api.CommerceFulfilmentMode(item.Mode), Status: api.CommerceFulfilmentStatus(item.Status),
		PickupAddress: item.PickupAddress, DestinationAddress: item.DestinationAddress,
		VerificationCodeExpiresAt: item.VerificationCodeExpiresAt, VerifiedAt: item.VerifiedAt, VerifiedByUserId: item.VerifiedByUserID,
		HandedOverAt: item.HandedOverAt, HandedOverByUserId: item.HandedOverByUserID,
		DeliveredAt: item.DeliveredAt, CompletedAt: item.CompletedAt, Version: item.Version,
		Quotes: quotes, RiderAssignments: assignments, Events: events, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}
