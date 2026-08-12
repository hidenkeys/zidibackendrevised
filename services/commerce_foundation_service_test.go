package services

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/utils"
)

type commerceFoundationRepoStub struct {
	mu                 sync.Mutex
	listOrganizationID uuid.UUID
	listAssignedUserID *uuid.UUID
	createdStore       *models.CommerceStore
	createdHours       []models.CommerceStoreHour
	createdModes       []models.CommerceStoreFulfilmentMode
	updatedProfile     *models.CommerceMerchantProfile
	userRole           string
	listStores         []models.CommerceStore
	storeModes         []models.CommerceStoreFulfilmentMode
}

func (s *commerceFoundationRepoStub) OrganizationExists(context.Context, uuid.UUID) (bool, error) {
	return true, nil
}

func (s *commerceFoundationRepoStub) UserRoleInOrganization(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	if s.userRole == "" {
		return "", repository.ErrCommerceNotFound
	}
	return s.userRole, nil
}

func (s *commerceFoundationRepoStub) CreateMerchantProfile(context.Context, *models.CommerceMerchantProfile) error {
	return nil
}

func (s *commerceFoundationRepoStub) UpdateMerchantProfile(_ context.Context, profile *models.CommerceMerchantProfile) error {
	s.updatedProfile = profile
	return nil
}

func (s *commerceFoundationRepoStub) GetMerchantProfile(_ context.Context, organizationID uuid.UUID) (*models.CommerceMerchantProfile, error) {
	return &models.CommerceMerchantProfile{ID: uuid.New(), OrganizationID: organizationID, DefaultCurrency: "NGN"}, nil
}

func (s *commerceFoundationRepoStub) CreateStore(_ context.Context, store *models.CommerceStore, hours []models.CommerceStoreHour, modes []models.CommerceStoreFulfilmentMode) error {
	s.createdStore = store
	s.createdHours = hours
	s.createdModes = modes
	store.Hours = hours
	store.FulfilmentModes = modes
	return nil
}

func (s *commerceFoundationRepoStub) UpdateStore(_ context.Context, store *models.CommerceStore, hours []models.CommerceStoreHour, modes []models.CommerceStoreFulfilmentMode) error {
	s.createdStore = store
	s.createdHours = hours
	s.createdModes = modes
	store.Hours = hours
	store.FulfilmentModes = modes
	return nil
}

func (s *commerceFoundationRepoStub) ListStores(_ context.Context, organizationID uuid.UUID, assignedUserID *uuid.UUID) ([]models.CommerceStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listOrganizationID = organizationID
	s.listAssignedUserID = assignedUserID
	return s.listStores, nil
}

func (s *commerceFoundationRepoStub) GetStore(_ context.Context, organizationID, storeID uuid.UUID, assignedUserID *uuid.UUID) (*models.CommerceStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listOrganizationID = organizationID
	s.listAssignedUserID = assignedUserID
	return &models.CommerceStore{ID: storeID, OrganizationID: organizationID, FulfilmentModes: s.storeModes}, nil
}

func (s *commerceFoundationRepoStub) CreateStaffAssignment(context.Context, *models.CommerceStaffStoreAssignment) error {
	return nil
}

func (s *commerceFoundationRepoStub) ListStaffAssignments(context.Context, uuid.UUID, uuid.UUID) ([]models.CommerceStaffStoreAssignment, error) {
	return []models.CommerceStaffStoreAssignment{}, nil
}

func TestMerchantAdminCannotSelectAnotherTenant(t *testing.T) {
	repo := &commerceFoundationRepoStub{}
	service := NewCommerceFoundationService(repo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: uuid.New(), Role: utils.RoleMerchantAdmin}
	otherOrganizationID := uuid.New()

	_, err := service.ListStores(context.Background(), actor, &otherOrganizationID)
	if !errors.Is(err, ErrCommerceForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}
	if repo.listOrganizationID != uuid.Nil {
		t.Fatal("repository was called after a cross-tenant request")
	}
}

func TestPlatformAdminCanSelectAnotherTenant(t *testing.T) {
	repo := &commerceFoundationRepoStub{}
	service := NewCommerceFoundationService(repo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: uuid.New(), Role: utils.RolePlatformAdmin}
	targetOrganizationID := uuid.New()

	if _, err := service.ListStores(context.Background(), actor, &targetOrganizationID); err != nil {
		t.Fatalf("list target tenant stores: %v", err)
	}
	if repo.listOrganizationID != targetOrganizationID {
		t.Fatalf("expected tenant %s, got %s", targetOrganizationID, repo.listOrganizationID)
	}
}

func TestStoreStaffQueriesOnlyAssignedStores(t *testing.T) {
	repo := &commerceFoundationRepoStub{}
	service := NewCommerceFoundationService(repo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: uuid.New(), Role: utils.RoleStoreStaff}

	if _, err := service.ListStores(context.Background(), actor, nil); err != nil {
		t.Fatalf("list assigned stores: %v", err)
	}
	if repo.listAssignedUserID == nil || *repo.listAssignedUserID != actor.UserID {
		t.Fatal("store staff query was not scoped to its active assignments")
	}
}

func TestCommerceMerchantAdminUpdatesBusinessSettings(t *testing.T) {
	organizationID := uuid.New()
	repo := &commerceFoundationRepoStub{}
	service := NewCommerceFoundationService(repo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}

	profile, err := service.UpdateMerchantProfile(context.Background(), actor, nil, UpdateCommerceMerchantProfileInput{
		Slug: "  example-shop  ", DisplayName: " Example Shop ", DefaultCurrency: "ngn",
		Timezone: "Africa/Lagos", Status: models.CommerceStatusActive,
	})
	if err != nil {
		t.Fatalf("update merchant profile: %v", err)
	}
	if repo.updatedProfile == nil || profile.Slug != "example-shop" || profile.DefaultCurrency != "NGN" || profile.DisplayName != "Example Shop" {
		t.Fatalf("profile was not normalized: %+v", profile)
	}
}

func TestCommerceMerchantAdminUpdatesStoreFulfilmentOptions(t *testing.T) {
	organizationID, storeID := uuid.New(), uuid.New()
	repo := &commerceFoundationRepoStub{}
	service := NewCommerceFoundationService(repo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}
	openMinute, closeMinute := 8*60, 20*60

	store, err := service.UpdateStore(context.Background(), actor, nil, storeID, UpdateCommerceStoreInput{
		CreateCommerceStoreInput: CreateCommerceStoreInput{
			Code: " lekki ", Name: "Lekki", Address: "18 Admiralty Way", City: "Lagos", State: "Lagos",
			CountryCode: "ng", Timezone: "Africa/Lagos", PreparationMinutes: 15,
			Hours:           []CommerceStoreHourInput{{DayOfWeek: 1, OpenMinute: &openMinute, CloseMinute: &closeMinute}},
			FulfilmentModes: []CommerceStoreFulfilmentModeInput{{Mode: models.FulfilmentModeCustomerPickup, Enabled: true}},
		},
		Status: models.CommerceStatusActive,
	})
	if err != nil {
		t.Fatalf("update store: %v", err)
	}
	if store.Code != "LEKKI" || len(repo.createdHours) != 1 || len(repo.createdModes) != 1 || repo.createdModes[0].Mode != models.FulfilmentModeCustomerPickup {
		t.Fatalf("store update was not persisted: store=%+v hours=%+v modes=%+v", store, repo.createdHours, repo.createdModes)
	}
}

func TestCreateStoreAppliesTenantToStoreConfiguration(t *testing.T) {
	repo := &commerceFoundationRepoStub{}
	service := NewCommerceFoundationService(repo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: uuid.New(), Role: utils.RoleMerchantAdmin}
	open, closeMinute := 600, 1200

	store, err := service.CreateStore(context.Background(), actor, nil, CreateCommerceStoreInput{
		Code:               " lekki-1 ",
		Name:               "Lekki",
		Address:            "1 Admiralty Way",
		City:               "Lagos",
		State:              "Lagos",
		CountryCode:        "ng",
		Timezone:           "Africa/Lagos",
		PreparationMinutes: 15,
		Hours: []CommerceStoreHourInput{{
			DayOfWeek: 1, OpenMinute: &open, CloseMinute: &closeMinute,
		}},
		FulfilmentModes: []CommerceStoreFulfilmentModeInput{{
			Mode: models.FulfilmentModeCustomerPickup, Enabled: true,
		}},
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if store.OrganizationID != actor.OrganizationID || store.Code != "LEKKI-1" || store.CountryCode != "NG" {
		t.Fatalf("store was not normalized and tenant-scoped: %+v", store)
	}
	if len(repo.createdHours) != 1 || len(repo.createdModes) != 1 {
		t.Fatal("store configuration was not persisted with the store")
	}
}

func TestAssignmentRoleMustMatchUserRole(t *testing.T) {
	repo := &commerceFoundationRepoStub{userRole: utils.RoleStoreStaff}
	service := NewCommerceFoundationService(repo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: uuid.New(), Role: utils.RoleMerchantAdmin}

	_, err := service.AssignStoreStaff(context.Background(), actor, nil, uuid.New(), CreateCommerceStaffAssignmentInput{
		UserID: uuid.New(),
		Role:   utils.RoleStoreManager,
	})
	if !errors.Is(err, ErrCommerceValidation) {
		t.Fatalf("expected role validation error, got %v", err)
	}
}
