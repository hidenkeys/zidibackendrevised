package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/utils"
)

var (
	ErrCommerceValidation = errors.New("invalid commerce input")
	ErrCommerceForbidden  = errors.New("commerce access denied")
)

var commerceSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type CommerceActor struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	Role           string
}

type CreateCommerceMerchantProfileInput struct {
	Slug            string
	DisplayName     string
	DefaultCurrency string
	Timezone        string
}

type UpdateCommerceMerchantProfileInput struct {
	Slug            string
	DisplayName     string
	DefaultCurrency string
	Timezone        string
	Status          string
}

type CreateCommerceStoreInput struct {
	Code               string
	Name               string
	Address            string
	City               string
	State              string
	CountryCode        string
	Latitude           *float64
	Longitude          *float64
	Timezone           string
	PreparationMinutes int
	Hours              []CommerceStoreHourInput
	FulfilmentModes    []CommerceStoreFulfilmentModeInput
}

type UpdateCommerceStoreInput struct {
	CreateCommerceStoreInput
	Status string
}

type CommerceStoreHourInput struct {
	DayOfWeek   int
	OpenMinute  *int
	CloseMinute *int
	IsClosed    bool
}

type CommerceStoreFulfilmentModeInput struct {
	Mode    string
	Enabled bool
}

type CreateCommerceStaffAssignmentInput struct {
	UserID uuid.UUID
	Role   string
}

type CommerceFoundationService struct {
	repo repository.CommerceFoundationRepository
}

func NewCommerceFoundationService(repo repository.CommerceFoundationRepository) *CommerceFoundationService {
	return &CommerceFoundationService{repo: repo}
}

func (s *CommerceFoundationService) CreateMerchantProfile(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, input CreateCommerceMerchantProfileInput) (*models.CommerceMerchantProfile, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if err := validateMerchantProfile(input); err != nil {
		return nil, err
	}
	exists, err := s.repo.OrganizationExists(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("check organization: %w", err)
	}
	if !exists {
		return nil, repository.ErrCommerceNotFound
	}

	profile := &models.CommerceMerchantProfile{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		Slug:            strings.ToLower(strings.TrimSpace(input.Slug)),
		DisplayName:     strings.TrimSpace(input.DisplayName),
		DefaultCurrency: strings.ToUpper(strings.TrimSpace(input.DefaultCurrency)),
		Timezone:        strings.TrimSpace(input.Timezone),
		Status:          models.CommerceStatusActive,
	}
	if err := s.repo.CreateMerchantProfile(ctx, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (s *CommerceFoundationService) GetMerchantProfile(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID) (*models.CommerceMerchantProfile, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	return s.repo.GetMerchantProfile(ctx, organizationID)
}

func (s *CommerceFoundationService) UpdateMerchantProfile(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, input UpdateCommerceMerchantProfileInput) (*models.CommerceMerchantProfile, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	profileInput := CreateCommerceMerchantProfileInput{Slug: input.Slug, DisplayName: input.DisplayName, DefaultCurrency: input.DefaultCurrency, Timezone: input.Timezone}
	if err := validateMerchantProfile(profileInput); err != nil {
		return nil, err
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if !isCommerceActiveStatus(status) {
		return nil, fmt.Errorf("%w: status must be active or inactive", ErrCommerceValidation)
	}
	profile, err := s.repo.GetMerchantProfile(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	profile.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	profile.DisplayName = strings.TrimSpace(input.DisplayName)
	profile.DefaultCurrency = strings.ToUpper(strings.TrimSpace(input.DefaultCurrency))
	profile.Timezone = strings.TrimSpace(input.Timezone)
	profile.Status = status
	if err := s.repo.UpdateMerchantProfile(ctx, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (s *CommerceFoundationService) CreateStore(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, input CreateCommerceStoreInput) (*models.CommerceStore, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.GetMerchantProfile(ctx, organizationID); err != nil {
		return nil, err
	}
	if err := validateStore(input); err != nil {
		return nil, err
	}

	store := &models.CommerceStore{
		ID:                 uuid.New(),
		OrganizationID:     organizationID,
		Code:               strings.ToUpper(strings.TrimSpace(input.Code)),
		Name:               strings.TrimSpace(input.Name),
		Address:            strings.TrimSpace(input.Address),
		City:               strings.TrimSpace(input.City),
		State:              strings.TrimSpace(input.State),
		CountryCode:        strings.ToUpper(strings.TrimSpace(input.CountryCode)),
		Latitude:           input.Latitude,
		Longitude:          input.Longitude,
		Timezone:           strings.TrimSpace(input.Timezone),
		PreparationMinutes: input.PreparationMinutes,
		Status:             models.CommerceStatusActive,
	}
	hours := make([]models.CommerceStoreHour, 0, len(input.Hours))
	for _, item := range input.Hours {
		hours = append(hours, models.CommerceStoreHour{
			ID:          uuid.New(),
			DayOfWeek:   item.DayOfWeek,
			OpenMinute:  item.OpenMinute,
			CloseMinute: item.CloseMinute,
			IsClosed:    item.IsClosed,
		})
	}
	modes := make([]models.CommerceStoreFulfilmentMode, 0, len(input.FulfilmentModes))
	for _, item := range input.FulfilmentModes {
		modes = append(modes, models.CommerceStoreFulfilmentMode{
			ID:      uuid.New(),
			Mode:    strings.ToLower(strings.TrimSpace(item.Mode)),
			Enabled: item.Enabled,
		})
	}
	if err := s.repo.CreateStore(ctx, store, hours, modes); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *CommerceFoundationService) ListStores(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID) ([]models.CommerceStore, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListStores(ctx, organizationID, storeScope(actor))
}

func (s *CommerceFoundationService) GetStore(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, storeID uuid.UUID) (*models.CommerceStore, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	return s.repo.GetStore(ctx, organizationID, storeID, storeScope(actor))
}

func (s *CommerceFoundationService) UpdateStore(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, storeID uuid.UUID, input UpdateCommerceStoreInput) (*models.CommerceStore, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if storeID == uuid.Nil {
		return nil, fmt.Errorf("%w: store is required", ErrCommerceValidation)
	}
	if err := validateStore(input.CreateCommerceStoreInput); err != nil {
		return nil, err
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if !isCommerceActiveStatus(status) {
		return nil, fmt.Errorf("%w: status must be active or inactive", ErrCommerceValidation)
	}
	store, err := s.repo.GetStore(ctx, organizationID, storeID, nil)
	if err != nil {
		return nil, err
	}
	store.Code = strings.ToUpper(strings.TrimSpace(input.Code))
	store.Name = strings.TrimSpace(input.Name)
	store.Address = strings.TrimSpace(input.Address)
	store.City = strings.TrimSpace(input.City)
	store.State = strings.TrimSpace(input.State)
	store.CountryCode = strings.ToUpper(strings.TrimSpace(input.CountryCode))
	store.Latitude = input.Latitude
	store.Longitude = input.Longitude
	store.Timezone = strings.TrimSpace(input.Timezone)
	store.PreparationMinutes = input.PreparationMinutes
	store.Status = status
	hours, modes := commerceStoreRelations(organizationID, store.ID, input.Hours, input.FulfilmentModes)
	if err := s.repo.UpdateStore(ctx, store, hours, modes); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *CommerceFoundationService) AssignStoreStaff(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, storeID uuid.UUID, input CreateCommerceStaffAssignmentInput) (*models.CommerceStaffStoreAssignment, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	role := strings.ToLower(strings.TrimSpace(input.Role))
	if role != utils.RoleStoreManager && role != utils.RoleStoreStaff {
		return nil, fmt.Errorf("%w: role must be store_manager or store_staff", ErrCommerceValidation)
	}
	userRole, err := s.repo.UserRoleInOrganization(ctx, organizationID, input.UserID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(userRole, role) {
		return nil, fmt.Errorf("%w: user's organization role must match the store assignment role", ErrCommerceValidation)
	}

	assignment := &models.CommerceStaffStoreAssignment{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		StoreID:        storeID,
		UserID:         input.UserID,
		Role:           role,
		Status:         models.CommerceStatusActive,
	}
	if err := s.repo.CreateStaffAssignment(ctx, assignment); err != nil {
		return nil, err
	}
	return assignment, nil
}

func (s *CommerceFoundationService) ListStoreStaff(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, storeID uuid.UUID) ([]models.CommerceStaffStoreAssignment, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.GetStore(ctx, organizationID, storeID, storeScope(actor)); err != nil {
		return nil, err
	}
	return s.repo.ListStaffAssignments(ctx, organizationID, storeID)
}

func resolveCommerceTenant(actor CommerceActor, requested *uuid.UUID) (uuid.UUID, error) {
	if actor.UserID == uuid.Nil || actor.OrganizationID == uuid.Nil || !utils.IsKnownRole(actor.Role) {
		return uuid.Nil, ErrCommerceForbidden
	}
	if requested == nil || *requested == uuid.Nil {
		return actor.OrganizationID, nil
	}
	if utils.IsPlatformRole(actor.Role) {
		return *requested, nil
	}
	if *requested != actor.OrganizationID {
		return uuid.Nil, ErrCommerceForbidden
	}
	return actor.OrganizationID, nil
}

func storeScope(actor CommerceActor) *uuid.UUID {
	if utils.IsStoreRole(actor.Role) {
		userID := actor.UserID
		return &userID
	}
	return nil
}

func canManageMerchant(role string) bool {
	return utils.IsPlatformRole(role) || utils.IsMerchantAdminRole(role)
}

func canAccessCommerce(role string) bool {
	return canManageMerchant(role) || utils.IsStoreRole(role)
}

func validateMerchantProfile(input CreateCommerceMerchantProfileInput) error {
	slug := strings.ToLower(strings.TrimSpace(input.Slug))
	if !commerceSlugPattern.MatchString(slug) {
		return fmt.Errorf("%w: slug must contain lowercase letters, numbers, and single hyphens", ErrCommerceValidation)
	}
	if strings.TrimSpace(input.DisplayName) == "" {
		return fmt.Errorf("%w: display name is required", ErrCommerceValidation)
	}
	if len(strings.TrimSpace(input.DefaultCurrency)) != 3 {
		return fmt.Errorf("%w: default currency must be a 3-letter ISO code", ErrCommerceValidation)
	}
	if strings.TrimSpace(input.Timezone) == "" {
		return fmt.Errorf("%w: timezone is required", ErrCommerceValidation)
	}
	return nil
}

func validateStore(input CreateCommerceStoreInput) error {
	if strings.TrimSpace(input.Code) == "" || strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("%w: store code and name are required", ErrCommerceValidation)
	}
	if strings.TrimSpace(input.Address) == "" || strings.TrimSpace(input.City) == "" || strings.TrimSpace(input.State) == "" {
		return fmt.Errorf("%w: store address, city, and state are required", ErrCommerceValidation)
	}
	if len(strings.TrimSpace(input.CountryCode)) != 2 || strings.TrimSpace(input.Timezone) == "" {
		return fmt.Errorf("%w: country code and timezone are required", ErrCommerceValidation)
	}
	if input.PreparationMinutes < 0 || input.PreparationMinutes > 1440 {
		return fmt.Errorf("%w: preparation minutes must be between 0 and 1440", ErrCommerceValidation)
	}
	if input.Latitude != nil && (*input.Latitude < -90 || *input.Latitude > 90) {
		return fmt.Errorf("%w: latitude must be between -90 and 90", ErrCommerceValidation)
	}
	if input.Longitude != nil && (*input.Longitude < -180 || *input.Longitude > 180) {
		return fmt.Errorf("%w: longitude must be between -180 and 180", ErrCommerceValidation)
	}

	days := make(map[int]struct{}, len(input.Hours))
	for _, item := range input.Hours {
		if item.DayOfWeek < 0 || item.DayOfWeek > 6 {
			return fmt.Errorf("%w: day of week must be between 0 and 6", ErrCommerceValidation)
		}
		if _, exists := days[item.DayOfWeek]; exists {
			return fmt.Errorf("%w: store hours contain a duplicate day", ErrCommerceValidation)
		}
		days[item.DayOfWeek] = struct{}{}
		if item.IsClosed {
			if item.OpenMinute != nil || item.CloseMinute != nil {
				return fmt.Errorf("%w: closed days cannot have opening times", ErrCommerceValidation)
			}
			continue
		}
		if item.OpenMinute == nil || item.CloseMinute == nil || *item.OpenMinute < 0 || *item.CloseMinute > 1440 || *item.CloseMinute <= *item.OpenMinute {
			return fmt.Errorf("%w: open days require a valid opening and closing minute", ErrCommerceValidation)
		}
	}

	modes := make(map[string]struct{}, len(input.FulfilmentModes))
	for _, item := range input.FulfilmentModes {
		mode := strings.ToLower(strings.TrimSpace(item.Mode))
		if mode != models.FulfilmentModeCustomerPickup && mode != models.FulfilmentModeCustomerRider && mode != models.FulfilmentModeMerchantRider {
			return fmt.Errorf("%w: unsupported fulfilment mode", ErrCommerceValidation)
		}
		if _, exists := modes[mode]; exists {
			return fmt.Errorf("%w: duplicate fulfilment mode", ErrCommerceValidation)
		}
		modes[mode] = struct{}{}
	}
	return nil
}

func commerceStoreRelations(organizationID, storeID uuid.UUID, hourInputs []CommerceStoreHourInput, modeInputs []CommerceStoreFulfilmentModeInput) ([]models.CommerceStoreHour, []models.CommerceStoreFulfilmentMode) {
	hours := make([]models.CommerceStoreHour, 0, len(hourInputs))
	for _, item := range hourInputs {
		hours = append(hours, models.CommerceStoreHour{
			ID: uuid.New(), OrganizationID: organizationID, StoreID: storeID, DayOfWeek: item.DayOfWeek,
			OpenMinute: item.OpenMinute, CloseMinute: item.CloseMinute, IsClosed: item.IsClosed,
		})
	}
	modes := make([]models.CommerceStoreFulfilmentMode, 0, len(modeInputs))
	for _, item := range modeInputs {
		modes = append(modes, models.CommerceStoreFulfilmentMode{
			ID: uuid.New(), OrganizationID: organizationID, StoreID: storeID,
			Mode: strings.ToLower(strings.TrimSpace(item.Mode)), Enabled: item.Enabled,
		})
	}
	return hours, modes
}

func isCommerceActiveStatus(status string) bool {
	return status == models.CommerceStatusActive || status == models.CommerceStatusInactive
}
