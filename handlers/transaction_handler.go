package handlers

import (
	"context"
	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidibackend/api"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"net/http"
)

func (s Server) ListTransactions(c *fiber.Ctx, params api.ListTransactionsParams) error {
	// Pagination settings
	limit := 10
	offset := 0

	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}

	// Check for organizationId
	if params.OrganizationId != nil {
		transactions, count, err := s.transactionService.GetTransactionsByOrganization(context.Background(), *params.OrganizationId, limit, offset)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(api.Error{
				ErrorCode: "500",
				Message:   err.Error(),
			})
		}
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"transactions": transactions,
			"count":        count,
		})
	}

	// Fetch the list of transactions
	transactions, count, err := s.transactionService.GetAllTransactions(context.Background(), limit, offset)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"transactions": transactions,
		"count":        count,
	})
}

func (s Server) CreateTransaction(c *fiber.Ctx) error {
	// Parse the request body into a Transaction struct
	transaction := new(api.TransactionInput)
	if err := c.BodyParser(transaction); err != nil {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid request body",
		})
	}

	// Validate the data (you may want to add more validation here)
	if transaction.Amount <= 0 {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Amount must be greater than zero",
		})
	}

	// Create the transaction in the database
	newTransaction, err := s.transactionService.CreateTransaction(context.Background(), transaction)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(http.StatusCreated).JSON(newTransaction)
}

func (s Server) GetTransaction(c *fiber.Ctx, transactionId openapi_types.UUID) error {
	// Fetch the transaction by ID
	transaction, err := s.transactionService.GetTransactionByID(context.Background(), transactionId)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(api.Error{
			ErrorCode: "404",
			Message:   "Transaction not found",
		})
	}

	return c.Status(http.StatusOK).JSON(transaction)
}

func (s Server) UpdateTransactionStatus(c *fiber.Ctx, transactionId openapi_types.UUID) error {
	// Parse the request body to get the new status
	type UpdateStatusRequest struct {
		Status string `json:"status"`
	}

	statusRequest := new(UpdateStatusRequest)
	if err := c.BodyParser(statusRequest); err != nil {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid request body",
		})
	}

	// Ensure the status is valid (you can add more statuses as necessary)
	validStatuses := []string{"pending", "completed", "failed"}
	isValidStatus := false
	for _, status := range validStatuses {
		if status == statusRequest.Status {
			isValidStatus = true
			break
		}
	}
	if !isValidStatus {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid status provided",
		})
	}

	// Update the transaction status
	updatedTransaction, err := s.transactionService.UpdateTransactionStatus(context.Background(), transactionId, statusRequest.Status)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(http.StatusOK).JSON(updatedTransaction)
}
