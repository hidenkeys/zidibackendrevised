package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/middleware"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/services"
)

func (s Server) CreateCommerceMerchantProfile(c *fiber.Ctx) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.CreateCommerceMerchantProfileJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	profile, err := s.commerceFoundationService.CreateMerchantProfile(c.UserContext(), actor, request.OrganizationId, services.CreateCommerceMerchantProfileInput{
		Slug:            request.Slug,
		DisplayName:     request.DisplayName,
		DefaultCurrency: request.DefaultCurrency,
		Timezone:        request.Timezone,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.Status(http.StatusCreated).JSON(commerceMerchantProfileResponse(profile))
}

func (s Server) GetCommerceMerchantProfile(c *fiber.Ctx, params api.GetCommerceMerchantProfileParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	profile, err := s.commerceFoundationService.GetMerchantProfile(c.UserContext(), actor, params.OrganizationId)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceMerchantProfileResponse(profile))
}

func (s Server) UpdateCommerceMerchantProfile(c *fiber.Ctx) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.UpdateCommerceMerchantProfileJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	profile, err := s.commerceFoundationService.UpdateMerchantProfile(c.UserContext(), actor, request.OrganizationId, services.UpdateCommerceMerchantProfileInput{
		Slug: request.Slug, DisplayName: request.DisplayName, DefaultCurrency: request.DefaultCurrency,
		Timezone: request.Timezone, Status: string(request.Status),
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceMerchantProfileResponse(profile))
}

func (s Server) CreateCommerceStore(c *fiber.Ctx) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.CreateCommerceStoreJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}

	hours := make([]services.CommerceStoreHourInput, 0)
	if request.Hours != nil {
		hours = make([]services.CommerceStoreHourInput, 0, len(*request.Hours))
		for _, item := range *request.Hours {
			hours = append(hours, services.CommerceStoreHourInput{
				DayOfWeek:   item.DayOfWeek,
				OpenMinute:  item.OpenMinute,
				CloseMinute: item.CloseMinute,
				IsClosed:    item.IsClosed,
			})
		}
	}
	modes := make([]services.CommerceStoreFulfilmentModeInput, 0)
	if request.FulfilmentModes != nil {
		modes = make([]services.CommerceStoreFulfilmentModeInput, 0, len(*request.FulfilmentModes))
		for _, item := range *request.FulfilmentModes {
			modes = append(modes, services.CommerceStoreFulfilmentModeInput{
				Mode:    string(item.Mode),
				Enabled: item.Enabled,
			})
		}
	}

	store, err := s.commerceFoundationService.CreateStore(c.UserContext(), actor, request.OrganizationId, services.CreateCommerceStoreInput{
		Code:               request.Code,
		Name:               request.Name,
		Address:            request.Address,
		City:               request.City,
		State:              request.State,
		CountryCode:        request.CountryCode,
		Latitude:           request.Latitude,
		Longitude:          request.Longitude,
		Timezone:           request.Timezone,
		PreparationMinutes: request.PreparationMinutes,
		Hours:              hours,
		FulfilmentModes:    modes,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.Status(http.StatusCreated).JSON(commerceStoreResponse(store))
}

func (s Server) ListCommerceStores(c *fiber.Ctx, params api.ListCommerceStoresParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	stores, err := s.commerceFoundationService.ListStores(c.UserContext(), actor, params.OrganizationId)
	if err != nil {
		return commerceError(c, err)
	}
	response := make([]api.CommerceStore, 0, len(stores))
	for index := range stores {
		response = append(response, commerceStoreResponse(&stores[index]))
	}
	return c.JSON(response)
}

func (s Server) GetCommerceStore(c *fiber.Ctx, storeID uuid.UUID, params api.GetCommerceStoreParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	store, err := s.commerceFoundationService.GetStore(c.UserContext(), actor, params.OrganizationId, storeID)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceStoreResponse(store))
}

func (s Server) UpdateCommerceStore(c *fiber.Ctx, storeID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.UpdateCommerceStoreJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	hours := commerceStoreHourInputs(request.Hours)
	modes := commerceStoreModeInputs(request.FulfilmentModes)
	store, err := s.commerceFoundationService.UpdateStore(c.UserContext(), actor, request.OrganizationId, storeID, services.UpdateCommerceStoreInput{
		CreateCommerceStoreInput: services.CreateCommerceStoreInput{
			Code: request.Code, Name: request.Name, Address: request.Address, City: request.City, State: request.State,
			CountryCode: request.CountryCode, Latitude: request.Latitude, Longitude: request.Longitude, Timezone: request.Timezone,
			PreparationMinutes: request.PreparationMinutes, Hours: hours, FulfilmentModes: modes,
		},
		Status: string(request.Status),
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceStoreResponse(store))
}

func (s Server) AssignCommerceStoreStaff(c *fiber.Ctx, storeID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.AssignCommerceStoreStaffJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	assignment, err := s.commerceFoundationService.AssignStoreStaff(c.UserContext(), actor, request.OrganizationId, storeID, services.CreateCommerceStaffAssignmentInput{
		UserID: request.UserId,
		Role:   string(request.Role),
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.Status(http.StatusCreated).JSON(commerceStaffAssignmentResponse(assignment))
}

func (s Server) ListCommerceStoreStaff(c *fiber.Ctx, storeID uuid.UUID, params api.ListCommerceStoreStaffParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	assignments, err := s.commerceFoundationService.ListStoreStaff(c.UserContext(), actor, params.OrganizationId, storeID)
	if err != nil {
		return commerceError(c, err)
	}
	response := make([]api.CommerceStaffAssignment, 0, len(assignments))
	for index := range assignments {
		response = append(response, commerceStaffAssignmentResponse(&assignments[index]))
	}
	return c.JSON(response)
}

func commerceActor(c *fiber.Ctx) (services.CommerceActor, error) {
	claims, ok := middleware.CurrentUser(c)
	if !ok {
		return services.CommerceActor{}, services.ErrCommerceForbidden
	}
	userID, err := uuid.Parse(claims.ID)
	if err != nil {
		return services.CommerceActor{}, services.ErrCommerceForbidden
	}
	organizationID, err := uuid.Parse(claims.OrganizationID)
	if err != nil {
		return services.CommerceActor{}, services.ErrCommerceForbidden
	}
	return services.CommerceActor{UserID: userID, OrganizationID: organizationID, Role: claims.Role}, nil
}

func commerceMerchantProfileResponse(profile *models.CommerceMerchantProfile) api.CommerceMerchantProfile {
	return api.CommerceMerchantProfile{
		Id:              profile.ID,
		OrganizationId:  profile.OrganizationID,
		Slug:            profile.Slug,
		DisplayName:     profile.DisplayName,
		DefaultCurrency: profile.DefaultCurrency,
		Timezone:        profile.Timezone,
		Status:          api.CommerceMerchantProfileStatus(profile.Status),
		CreatedAt:       profile.CreatedAt,
		UpdatedAt:       profile.UpdatedAt,
	}
}

func commerceStoreResponse(store *models.CommerceStore) api.CommerceStore {
	hours := make([]api.CommerceStoreHour, 0, len(store.Hours))
	for _, item := range store.Hours {
		hours = append(hours, api.CommerceStoreHour{
			Id:          item.ID,
			DayOfWeek:   item.DayOfWeek,
			OpenMinute:  item.OpenMinute,
			CloseMinute: item.CloseMinute,
			IsClosed:    item.IsClosed,
		})
	}
	modes := make([]api.CommerceStoreFulfilmentMode, 0, len(store.FulfilmentModes))
	for _, item := range store.FulfilmentModes {
		modes = append(modes, api.CommerceStoreFulfilmentMode{
			Id:      item.ID,
			Mode:    api.CommerceStoreFulfilmentModeMode(item.Mode),
			Enabled: item.Enabled,
		})
	}
	return api.CommerceStore{
		Id:                 store.ID,
		OrganizationId:     store.OrganizationID,
		Code:               store.Code,
		Name:               store.Name,
		Address:            store.Address,
		City:               store.City,
		State:              store.State,
		CountryCode:        store.CountryCode,
		Latitude:           store.Latitude,
		Longitude:          store.Longitude,
		Timezone:           store.Timezone,
		PreparationMinutes: store.PreparationMinutes,
		Status:             api.CommerceStoreStatus(store.Status),
		Hours:              hours,
		FulfilmentModes:    modes,
		CreatedAt:          store.CreatedAt,
		UpdatedAt:          store.UpdatedAt,
	}
}

func commerceStaffAssignmentResponse(assignment *models.CommerceStaffStoreAssignment) api.CommerceStaffAssignment {
	return api.CommerceStaffAssignment{
		Id:             assignment.ID,
		OrganizationId: assignment.OrganizationID,
		StoreId:        assignment.StoreID,
		UserId:         assignment.UserID,
		Role:           api.CommerceStaffAssignmentRole(assignment.Role),
		Status:         api.CommerceStaffAssignmentStatus(assignment.Status),
		CreatedAt:      assignment.CreatedAt,
		UpdatedAt:      assignment.UpdatedAt,
	}
}

func commerceStoreHourInputs(items *[]api.CommerceStoreHourInput) []services.CommerceStoreHourInput {
	if items == nil {
		return []services.CommerceStoreHourInput{}
	}
	result := make([]services.CommerceStoreHourInput, 0, len(*items))
	for _, item := range *items {
		result = append(result, services.CommerceStoreHourInput{
			DayOfWeek: item.DayOfWeek, OpenMinute: item.OpenMinute, CloseMinute: item.CloseMinute, IsClosed: item.IsClosed,
		})
	}
	return result
}

func commerceStoreModeInputs(items *[]api.CommerceStoreFulfilmentModeInput) []services.CommerceStoreFulfilmentModeInput {
	if items == nil {
		return []services.CommerceStoreFulfilmentModeInput{}
	}
	result := make([]services.CommerceStoreFulfilmentModeInput, 0, len(*items))
	for _, item := range *items {
		result = append(result, services.CommerceStoreFulfilmentModeInput{Mode: string(item.Mode), Enabled: item.Enabled})
	}
	return result
}

func commerceError(c *fiber.Ctx, err error) error {
	status := http.StatusInternalServerError
	code := "500"
	message := "Internal server error"
	switch {
	case errors.Is(err, services.ErrCommerceValidation):
		status, code, message = http.StatusBadRequest, "400", err.Error()
	case errors.Is(err, services.ErrCommerceForbidden):
		status, code, message = http.StatusForbidden, "403", "Commerce access denied"
	case errors.Is(err, repository.ErrCommerceNotFound):
		status, code, message = http.StatusNotFound, "404", "Commerce resource not found"
	case errors.Is(err, repository.ErrCommerceConflict):
		status, code, message = http.StatusConflict, "409", "Commerce resource already exists"
	case errors.Is(err, repository.ErrCommerceInventoryUnavailable):
		status, code, message = http.StatusConflict, "409", "Requested inventory is unavailable"
	case errors.Is(err, repository.ErrCommerceReservationState):
		status, code, message = http.StatusConflict, "409", "Inventory reservation cannot make that transition"
	case errors.Is(err, repository.ErrCommerceCartInactive):
		status, code, message = http.StatusConflict, "409", "Cart is no longer active"
	case errors.Is(err, services.ErrCommerceCartItemUnavailable):
		status, code, message = http.StatusConflict, "409", "Cart item is unavailable"
	case errors.Is(err, services.ErrCommerceCartCurrency):
		status, code, message = http.StatusConflict, "409", "Cart item currency does not match the cart"
	case errors.Is(err, repository.ErrCommerceCheckoutEmptyCart):
		status, code, message = http.StatusConflict, "409", "Cart is empty"
	case errors.Is(err, repository.ErrCommerceOrderTransition):
		status, code, message = http.StatusConflict, "409", "Order status transition is not allowed"
	case errors.Is(err, services.ErrCommerceStoreOrderState):
		status, code, message = http.StatusConflict, "409", "Order cannot be prepared from its current state"
	case errors.Is(err, repository.ErrCommercePaymentExpired):
		status, code, message = http.StatusConflict, "409", "Payment window has expired"
	case errors.Is(err, repository.ErrCommercePaymentInitializing):
		status, code, message = http.StatusConflict, "409", "Payment initialization is already in progress"
	case errors.Is(err, repository.ErrCommercePaymentState):
		status, code, message = http.StatusConflict, "409", "Order is not in a payable state"
	case errors.Is(err, repository.ErrCommerceFulfilmentState):
		status, code, message = http.StatusConflict, "409", "Fulfilment transition is not allowed"
	case errors.Is(err, repository.ErrCommerceVerificationFailed):
		status, code, message = http.StatusConflict, "409", "Verification code is incorrect"
	case errors.Is(err, repository.ErrCommerceVerificationLocked):
		status, code, message = http.StatusConflict, "409", "Verification is temporarily locked; try again later"
	case errors.Is(err, repository.ErrCommerceVerificationExpired):
		status, code, message = http.StatusConflict, "409", "Verification code has expired"
	case errors.Is(err, services.ErrCommercePaymentProviderUnavailable):
		status, code, message = http.StatusBadGateway, "502", "Payment provider is temporarily unavailable"
	case errors.Is(err, services.ErrCommerceDeliveryProviderUnavailable):
		status, code, message = http.StatusBadGateway, "502", "Delivery quote provider is temporarily unavailable"
	default:
		log.Printf("commerce request failed: %v", err)
	}
	return c.Status(status).JSON(api.Error{ErrorCode: code, Message: message})
}
