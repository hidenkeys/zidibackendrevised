package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/services"
)

const commerceWebhookMaxBodyBytes = 1 << 20

func (s Server) InitializeCommerceOrderPayment(c *fiber.Ctx, orderID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.InitializeCommerceOrderPaymentJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	session, created, err := s.commercePaymentService.InitializePayment(c.UserContext(), actor, request.OrganizationId, orderID, services.InitializeCommercePaymentInput{
		Provider: optionalString(request.Provider), PayerEmail: optionalString(request.PayerEmail),
		IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return commerceError(c, err)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return c.Status(status).JSON(commercePaymentSessionResponse(session))
}

func (s Server) GetCommerceOrderInvoice(c *fiber.Ctx, orderID uuid.UUID, params api.GetCommerceOrderInvoiceParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	invoice, err := s.commercePaymentService.GetInvoice(c.UserContext(), actor, params.OrganizationId, orderID)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceInvoiceResponse(invoice))
}

func (s Server) GetCommercePayment(c *fiber.Ctx, paymentID uuid.UUID, params api.GetCommercePaymentParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	session, err := s.commercePaymentService.GetPayment(c.UserContext(), actor, params.OrganizationId, paymentID)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commercePaymentSessionResponse(session))
}

func (s Server) CommercePaymentWebhook(c *fiber.Ctx, provider string) error {
	if len(c.Body()) > commerceWebhookMaxBodyBytes {
		return c.Status(http.StatusRequestEntityTooLarge).JSON(api.Error{ErrorCode: "413", Message: "Webhook payload is too large"})
	}
	header, ok := s.commercePaymentService.WebhookSignatureHeader(provider)
	if !ok {
		return c.Status(http.StatusBadRequest).JSON(api.Error{ErrorCode: "400", Message: "Unsupported payment provider"})
	}
	result, err := s.commercePaymentService.ProcessWebhook(c.UserContext(), provider, c.Body(), c.Get(header))
	if err != nil {
		switch {
		case errors.Is(err, services.ErrCommerceWebhookUnauthorized):
			return c.Status(http.StatusUnauthorized).JSON(api.Error{ErrorCode: "401", Message: "Invalid payment webhook signature"})
		case errors.Is(err, services.ErrCommerceValidation):
			return c.Status(http.StatusBadRequest).JSON(api.Error{ErrorCode: "400", Message: "Invalid payment webhook"})
		case errors.Is(err, services.ErrCommercePaymentProviderUnavailable):
			log.Printf("commerce payment webhook verification unavailable: %v", err)
			return c.Status(http.StatusBadGateway).JSON(api.Error{ErrorCode: "502", Message: "Payment verification is temporarily unavailable"})
		default:
			log.Printf("commerce payment webhook failed: %v", err)
			return c.Status(http.StatusInternalServerError).JSON(api.Error{ErrorCode: "500", Message: "Payment webhook processing failed"})
		}
	}
	return c.JSON(api.CommercePaymentWebhookResponse{Acknowledged: true, Outcome: result.Outcome, Duplicate: result.Duplicate})
}

func commercePaymentSessionResponse(session *repository.CommercePaymentSession) api.CommercePaymentSession {
	payment := session.Payment
	return api.CommercePaymentSession{
		Invoice: commerceInvoiceResponse(session.Invoice),
		Payment: api.CommercePayment{
			Id: payment.ID, OrganizationId: payment.OrganizationID, OrderId: payment.OrderID,
			InvoiceId: payment.InvoiceID, Provider: payment.Provider,
			ProviderReference: payment.ProviderReference, ProviderTransactionId: payment.ProviderTransactionID,
			Status: api.CommercePaymentStatus(payment.Status), Currency: payment.Currency,
			AmountMinor: payment.AmountMinor, PayerEmail: payment.PayerEmail,
			AuthorizationUrl: payment.AuthorizationURL, ExpiresAt: payment.ExpiresAt,
			InitializedAt: payment.InitializedAt, ConfirmedAt: payment.ConfirmedAt,
			FailureReason: payment.FailureReason, CreatedAt: payment.CreatedAt, UpdatedAt: payment.UpdatedAt,
		},
	}
}

func commerceInvoiceResponse(invoice *models.CommerceInvoice) api.CommerceInvoice {
	items := make([]api.CommerceInvoiceItem, 0, len(invoice.Items))
	for _, item := range invoice.Items {
		attributes := map[string]string{}
		_ = json.Unmarshal(item.Attributes, &attributes)
		items = append(items, api.CommerceInvoiceItem{
			Id: item.ID, OrderItemId: item.OrderItemID, ProductId: item.ProductID,
			VariantId: item.VariantID, ProductName: item.ProductName, VariantName: item.VariantName,
			Sku: item.SKU, Attributes: attributes, Quantity: item.Quantity,
			UnitPriceMinor: item.UnitPriceMinor, LineTotalMinor: item.LineTotalMinor, CreatedAt: item.CreatedAt,
		})
	}
	return api.CommerceInvoice{
		Id: invoice.ID, OrganizationId: invoice.OrganizationID, OrderId: invoice.OrderID,
		StoreId: invoice.StoreID, CustomerId: invoice.CustomerID,
		InvoiceNumber: invoice.InvoiceNumber, Status: api.CommerceInvoiceStatus(invoice.Status),
		MerchantName: invoice.MerchantName, StoreName: invoice.StoreName, StoreAddress: invoice.StoreAddress,
		CustomerName: invoice.CustomerName, CustomerEmail: invoice.CustomerEmail,
		OrderNumber: invoice.OrderNumber, FulfilmentMode: api.CommerceInvoiceFulfilmentMode(invoice.FulfilmentMode),
		Currency: invoice.Currency, SubtotalMinor: invoice.SubtotalMinor, DiscountMinor: invoice.DiscountMinor,
		DeliveryFeeMinor: invoice.DeliveryFeeMinor, TotalMinor: invoice.TotalMinor,
		IssuedAt: invoice.IssuedAt, PaidAt: invoice.PaidAt, VoidedAt: invoice.VoidedAt,
		Items: items, CreatedAt: invoice.CreatedAt, UpdatedAt: invoice.UpdatedAt,
	}
}
