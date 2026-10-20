package services

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/utils"
)

type commerceCatalogueRepoStub struct {
	mu                sync.Mutex
	categoryCreated   *models.CommerceCategory
	productCreated    *models.CommerceProduct
	configuredItem    *models.CommerceStoreCatalogueItem
	inventory         models.CommerceInventoryLevel
	reservationsByKey map[string]*models.CommerceInventoryReservation
	reservationByID   map[uuid.UUID]*models.CommerceInventoryReservation
	adjustmentsByRef  map[string]repository.InventoryAdjustment
	storeCatalogue    []repository.CommerceStoreCatalogueEntry
	variant           *models.CommerceProductVariant
}

func (s *commerceCatalogueRepoStub) CreateCategory(_ context.Context, category *models.CommerceCategory) error {
	s.categoryCreated = category
	return nil
}

func (s *commerceCatalogueRepoStub) UpdateCategory(_ context.Context, category *models.CommerceCategory) error {
	s.categoryCreated = category
	return nil
}

func (s *commerceCatalogueRepoStub) ListCategories(context.Context, uuid.UUID) ([]models.CommerceCategory, error) {
	return []models.CommerceCategory{}, nil
}

func (s *commerceCatalogueRepoStub) CreateProduct(_ context.Context, product *models.CommerceProduct) error {
	s.productCreated = product
	return nil
}

func (s *commerceCatalogueRepoStub) UpdateProduct(_ context.Context, product *models.CommerceProduct) error {
	s.productCreated = product
	return nil
}

func (s *commerceCatalogueRepoStub) UpdateProductVariant(_ context.Context, variant *models.CommerceProductVariant) error {
	if s.productCreated == nil {
		s.productCreated = &models.CommerceProduct{ID: variant.ProductID, OrganizationID: variant.OrganizationID}
	}
	s.productCreated.Variants = []models.CommerceProductVariant{*variant}
	return nil
}

func (s *commerceCatalogueRepoStub) ListProducts(context.Context, uuid.UUID, *uuid.UUID, int, int) ([]models.CommerceProduct, int64, error) {
	return []models.CommerceProduct{}, 0, nil
}

func (s *commerceCatalogueRepoStub) GetProduct(_ context.Context, organizationID, productID uuid.UUID) (*models.CommerceProduct, error) {
	return &models.CommerceProduct{ID: productID, OrganizationID: organizationID}, nil
}

func (s *commerceCatalogueRepoStub) GetProductVariant(_ context.Context, organizationID, variantID uuid.UUID) (*models.CommerceProductVariant, error) {
	if s.variant != nil {
		copy := *s.variant
		return &copy, nil
	}
	return &models.CommerceProductVariant{ID: variantID, OrganizationID: organizationID}, nil
}

func (s *commerceCatalogueRepoStub) ConfigureStoreCatalogueItem(_ context.Context, item *models.CommerceStoreCatalogueItem, reorderThreshold int) (*models.CommerceStoreCatalogueItem, error) {
	s.configuredItem = item
	s.inventory = models.CommerceInventoryLevel{
		ID:               uuid.New(),
		OrganizationID:   item.OrganizationID,
		StoreID:          item.StoreID,
		VariantID:        item.VariantID,
		ReorderThreshold: reorderThreshold,
		Version:          1,
	}
	return item, nil
}

func (s *commerceCatalogueRepoStub) ListStoreCatalogue(context.Context, uuid.UUID, uuid.UUID) ([]repository.CommerceStoreCatalogueEntry, error) {
	return s.storeCatalogue, nil
}

func (s *commerceCatalogueRepoStub) GetStoreCatalogueEntry(_ context.Context, organizationID, storeID, variantID uuid.UUID) (*repository.CommerceStoreCatalogueEntry, error) {
	for _, entry := range s.storeCatalogue {
		if entry.StoreID == storeID && entry.VariantID == variantID {
			entryCopy := entry
			return &entryCopy, nil
		}
	}
	return nil, repository.ErrCommerceNotFound
}

func (s *commerceCatalogueRepoStub) GetInventoryLevel(_ context.Context, organizationID, storeID, variantID uuid.UUID) (*models.CommerceInventoryLevel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	level := s.inventory
	if level.ID == uuid.Nil {
		return nil, repository.ErrCommerceNotFound
	}
	if level.OrganizationID != organizationID || level.StoreID != storeID || level.VariantID != variantID {
		return nil, repository.ErrCommerceNotFound
	}
	return &level, nil
}

func (s *commerceCatalogueRepoStub) AdjustInventory(_ context.Context, input repository.InventoryAdjustment) (*models.CommerceInventoryLevel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.adjustmentsByRef == nil {
		s.adjustmentsByRef = make(map[string]repository.InventoryAdjustment)
	}
	if existing, ok := s.adjustmentsByRef[input.Reference]; ok {
		if existing.OrganizationID != input.OrganizationID || existing.StoreID != input.StoreID || existing.VariantID != input.VariantID || existing.QuantityDelta != input.QuantityDelta {
			return nil, repository.ErrCommerceConflict
		}
		level := s.inventory
		return &level, nil
	}
	if s.inventory.QuantityOnHand+input.QuantityDelta < s.inventory.QuantityReserved {
		return nil, repository.ErrCommerceInventoryUnavailable
	}
	s.inventory.QuantityOnHand += input.QuantityDelta
	s.inventory.Version++
	s.adjustmentsByRef[input.Reference] = input
	level := s.inventory
	return &level, nil
}

func TestCommerceMerchantAdminUpdatesCategory(t *testing.T) {
	organizationID := uuid.New()
	repo := &commerceCatalogueRepoStub{}
	service := NewCommerceCatalogueService(repo, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}

	category, err := service.UpdateCategory(context.Background(), actor, nil, uuid.New(), UpdateCommerceCategoryInput{
		CreateCommerceCategoryInput: CreateCommerceCategoryInput{Name: " Milk Tea ", Slug: "milk-tea", SortOrder: 2},
		Status:                      models.CommerceStatusInactive,
	})
	if err != nil {
		t.Fatalf("update category: %v", err)
	}
	if repo.categoryCreated == nil || category.Name != "Milk Tea" || category.Status != models.CommerceStatusInactive {
		t.Fatalf("category update was not persisted: %+v", category)
	}
}

func TestCommerceMerchantAdminUpdatesVariantBasePrice(t *testing.T) {
	organizationID, productID, variantID := uuid.New(), uuid.New(), uuid.New()
	repo := &commerceCatalogueRepoStub{variant: &models.CommerceProductVariant{
		ID: variantID, OrganizationID: organizationID, ProductID: productID, SKU: "TEA-L", IsDefault: true,
	}}
	service := NewCommerceCatalogueService(repo, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}

	variant, err := service.UpdateProductVariant(context.Background(), actor, nil, productID, variantID, UpdateCommerceProductVariantInput{
		Name: "Large", PriceMinor: 420000, Attributes: map[string]string{"size": "large"}, Status: models.CommerceStatusActive,
	})
	if err != nil {
		t.Fatalf("update variant: %v", err)
	}
	if len(repo.productCreated.Variants) != 1 || variant.PriceMinor != 420000 || variant.SKU != "TEA-L" || !variant.IsDefault {
		t.Fatalf("variant update did not preserve identity: %+v", variant)
	}
}

func (s *commerceCatalogueRepoStub) ReserveInventory(_ context.Context, input repository.InventoryReservationRequest) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reservationsByKey == nil {
		s.reservationsByKey = make(map[string]*models.CommerceInventoryReservation)
		s.reservationByID = make(map[uuid.UUID]*models.CommerceInventoryReservation)
	}
	if existing := s.reservationsByKey[input.ReservationKey]; existing != nil {
		if existing.StoreID != input.StoreID || existing.VariantID != input.VariantID || existing.Quantity != input.Quantity {
			return nil, nil, repository.ErrCommerceConflict
		}
		reservationCopy, levelCopy := *existing, s.inventory
		return &reservationCopy, &levelCopy, nil
	}
	if s.inventory.AvailableQuantity() < input.Quantity {
		return nil, nil, repository.ErrCommerceInventoryUnavailable
	}
	reservation := &models.CommerceInventoryReservation{
		ID:             uuid.New(),
		OrganizationID: input.OrganizationID,
		StoreID:        input.StoreID,
		VariantID:      input.VariantID,
		ReservationKey: input.ReservationKey,
		Quantity:       input.Quantity,
		Status:         models.InventoryReservationActive,
		ExpiresAt:      input.ExpiresAt,
	}
	s.inventory.QuantityReserved += input.Quantity
	s.inventory.Version++
	s.reservationsByKey[input.ReservationKey] = reservation
	s.reservationByID[reservation.ID] = reservation
	reservationCopy, levelCopy := *reservation, s.inventory
	return &reservationCopy, &levelCopy, nil
}

func (s *commerceCatalogueRepoStub) CommitInventoryReservation(_ context.Context, organizationID, reservationID uuid.UUID) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reservation := s.reservationByID[reservationID]
	if reservation == nil || reservation.OrganizationID != organizationID {
		return nil, nil, repository.ErrCommerceNotFound
	}
	if reservation.Status == models.InventoryReservationCommitted {
		reservationCopy, levelCopy := *reservation, s.inventory
		return &reservationCopy, &levelCopy, nil
	}
	if reservation.Status != models.InventoryReservationActive {
		return nil, nil, repository.ErrCommerceReservationState
	}
	s.inventory.QuantityOnHand -= reservation.Quantity
	s.inventory.QuantityReserved -= reservation.Quantity
	s.inventory.Version++
	reservation.Status = models.InventoryReservationCommitted
	reservationCopy, levelCopy := *reservation, s.inventory
	return &reservationCopy, &levelCopy, nil
}

func (s *commerceCatalogueRepoStub) ReleaseInventoryReservation(_ context.Context, organizationID, reservationID uuid.UUID, expired bool) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reservation := s.reservationByID[reservationID]
	if reservation == nil || reservation.OrganizationID != organizationID {
		return nil, nil, repository.ErrCommerceNotFound
	}
	targetStatus := models.InventoryReservationReleased
	if expired {
		targetStatus = models.InventoryReservationExpired
	}
	if reservation.Status == targetStatus {
		reservationCopy, levelCopy := *reservation, s.inventory
		return &reservationCopy, &levelCopy, nil
	}
	if reservation.Status != models.InventoryReservationActive {
		return nil, nil, repository.ErrCommerceReservationState
	}
	s.inventory.QuantityReserved -= reservation.Quantity
	s.inventory.Version++
	reservation.Status = targetStatus
	reservationCopy, levelCopy := *reservation, s.inventory
	return &reservationCopy, &levelCopy, nil
}

func (s *commerceCatalogueRepoStub) ExpireInventoryReservations(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time, int) (int, error) {
	return 0, nil
}

func TestCategoryCreationRejectsCrossTenantTarget(t *testing.T) {
	catalogueRepo := &commerceCatalogueRepoStub{}
	service := NewCommerceCatalogueService(catalogueRepo, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: uuid.New(), Role: utils.RoleMerchantAdmin}
	otherOrganizationID := uuid.New()

	_, err := service.CreateCategory(context.Background(), actor, &otherOrganizationID, CreateCommerceCategoryInput{Name: "Tea", Slug: "tea"})
	if !errors.Is(err, ErrCommerceForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}
	if catalogueRepo.categoryCreated != nil {
		t.Fatal("category repository was called for a cross-tenant request")
	}
}

func TestCreateProductNormalizesSKUAndKeepsMinorUnitPrice(t *testing.T) {
	catalogueRepo := &commerceCatalogueRepoStub{}
	service := NewCommerceCatalogueService(catalogueRepo, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: uuid.New(), Role: utils.RoleMerchantAdmin}

	product, err := service.CreateProduct(context.Background(), actor, nil, CreateCommerceProductInput{
		CategoryID: uuid.New(),
		Name:       "Strawberry Lemon Tea",
		Slug:       "strawberry-lemon-tea",
		Currency:   "ngn",
		Variants: []CommerceProductVariantInput{{
			Name: "Regular", SKU: " bc-fruit-001 ", PriceMinor: 420000, Attributes: map[string]string{"size": "regular"}, IsDefault: true,
		}},
		Images: []CommerceProductImageInput{{URL: "https://example.com/tea.png", SortOrder: 0}},
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	if product.OrganizationID != actor.OrganizationID || product.Currency != "NGN" {
		t.Fatalf("product was not tenant-scoped and normalized: %+v", product)
	}
	if len(product.Variants) != 1 || product.Variants[0].SKU != "BC-FRUIT-001" || product.Variants[0].PriceMinor != 420000 {
		t.Fatalf("variant price or SKU was changed incorrectly: %+v", product.Variants)
	}
	var attributes map[string]string
	if err := json.Unmarshal(product.Variants[0].Attributes, &attributes); err != nil || attributes["size"] != "regular" {
		t.Fatalf("variant attributes were not persisted as JSON: %s", product.Variants[0].Attributes)
	}
}

func TestCreateProductRequiresExactlyOneDefaultVariant(t *testing.T) {
	service := NewCommerceCatalogueService(&commerceCatalogueRepoStub{}, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: uuid.New(), Role: utils.RoleMerchantAdmin}

	_, err := service.CreateProduct(context.Background(), actor, nil, CreateCommerceProductInput{
		CategoryID: uuid.New(), Name: "Tea", Slug: "tea", Currency: "NGN",
		Variants: []CommerceProductVariantInput{{Name: "Small", SKU: "SMALL", PriceMinor: 1000}},
	})
	if !errors.Is(err, ErrCommerceValidation) {
		t.Fatalf("expected default variant validation error, got %v", err)
	}
}

func TestStoreManagerConfigurationIsScopedToAssignedStore(t *testing.T) {
	foundationRepo := &commerceFoundationRepoStub{}
	catalogueRepo := &commerceCatalogueRepoStub{}
	service := NewCommerceCatalogueService(catalogueRepo, foundationRepo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: uuid.New(), Role: utils.RoleStoreManager}
	storeID, variantID := uuid.New(), uuid.New()

	_, _, err := service.ConfigureStoreVariant(context.Background(), actor, nil, storeID, variantID, ConfigureCommerceStoreVariantInput{Enabled: true, ReorderThreshold: 5})
	if err != nil {
		t.Fatalf("configure assigned store item: %v", err)
	}
	if foundationRepo.listAssignedUserID == nil || *foundationRepo.listAssignedUserID != actor.UserID {
		t.Fatal("store manager configuration did not require its active store assignment")
	}
	if catalogueRepo.configuredItem == nil || catalogueRepo.configuredItem.StoreID != storeID {
		t.Fatal("store catalogue item was not configured")
	}
}

func TestStoreStaffCannotAdjustInventory(t *testing.T) {
	service := NewCommerceCatalogueService(&commerceCatalogueRepoStub{}, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: uuid.New(), Role: utils.RoleStoreStaff}

	_, err := service.AdjustInventory(context.Background(), actor, nil, uuid.New(), uuid.New(), AdjustCommerceInventoryInput{QuantityDelta: 1, Reference: "stock-1", Reason: "delivery"})
	if !errors.Is(err, ErrCommerceForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestInventoryReservationIsIdempotent(t *testing.T) {
	organizationID, storeID, variantID := uuid.New(), uuid.New(), uuid.New()
	catalogueRepo := &commerceCatalogueRepoStub{inventory: models.CommerceInventoryLevel{
		ID: uuid.New(), OrganizationID: organizationID, StoreID: storeID, VariantID: variantID, QuantityOnHand: 2,
	}}
	service := NewCommerceCatalogueService(catalogueRepo, &commerceFoundationRepoStub{})
	expiresAt := time.Now().Add(time.Hour)

	first, _, err := service.ReserveInventoryForCheckout(context.Background(), organizationID, storeID, variantID, "cart-123", 1, expiresAt)
	if err != nil {
		t.Fatalf("first reservation: %v", err)
	}
	second, level, err := service.ReserveInventoryForCheckout(context.Background(), organizationID, storeID, variantID, "cart-123", 1, expiresAt)
	if err != nil {
		t.Fatalf("retry reservation: %v", err)
	}
	if first.ID != second.ID || level.QuantityReserved != 1 {
		t.Fatalf("reservation retry was not idempotent: first=%s second=%s reserved=%d", first.ID, second.ID, level.QuantityReserved)
	}
}

func TestInventoryAdjustmentIsIdempotent(t *testing.T) {
	organizationID, storeID, variantID := uuid.New(), uuid.New(), uuid.New()
	catalogueRepo := &commerceCatalogueRepoStub{inventory: models.CommerceInventoryLevel{
		ID: uuid.New(), OrganizationID: organizationID, StoreID: storeID, VariantID: variantID,
	}}
	foundationRepo := &commerceFoundationRepoStub{}
	service := NewCommerceCatalogueService(catalogueRepo, foundationRepo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}
	input := AdjustCommerceInventoryInput{QuantityDelta: 5, Reference: "delivery-123", Reason: "supplier delivery"}

	if _, err := service.AdjustInventory(context.Background(), actor, nil, storeID, variantID, input); err != nil {
		t.Fatalf("first adjustment: %v", err)
	}
	level, err := service.AdjustInventory(context.Background(), actor, nil, storeID, variantID, input)
	if err != nil {
		t.Fatalf("retry adjustment: %v", err)
	}
	if level.QuantityOnHand != 5 {
		t.Fatalf("idempotent adjustment changed stock twice: %d", level.QuantityOnHand)
	}
}

func TestCommittedReservationDecrementsStockOnce(t *testing.T) {
	organizationID, storeID, variantID := uuid.New(), uuid.New(), uuid.New()
	catalogueRepo := &commerceCatalogueRepoStub{inventory: models.CommerceInventoryLevel{
		ID: uuid.New(), OrganizationID: organizationID, StoreID: storeID, VariantID: variantID, QuantityOnHand: 5,
	}}
	service := NewCommerceCatalogueService(catalogueRepo, &commerceFoundationRepoStub{})
	reservation, _, err := service.ReserveInventoryForCheckout(context.Background(), organizationID, storeID, variantID, "checkout-1", 2, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("reserve inventory: %v", err)
	}
	if _, _, err := service.CommitInventoryReservation(context.Background(), organizationID, reservation.ID); err != nil {
		t.Fatalf("commit inventory: %v", err)
	}
	committed, level, err := service.CommitInventoryReservation(context.Background(), organizationID, reservation.ID)
	if err != nil {
		t.Fatalf("retry inventory commit: %v", err)
	}
	if committed.Status != models.InventoryReservationCommitted || level.QuantityOnHand != 3 || level.QuantityReserved != 0 {
		t.Fatalf("unexpected committed inventory state: reservation=%s on_hand=%d reserved=%d", committed.Status, level.QuantityOnHand, level.QuantityReserved)
	}
}

func TestReleasedReservationRestoresAvailability(t *testing.T) {
	organizationID, storeID, variantID := uuid.New(), uuid.New(), uuid.New()
	catalogueRepo := &commerceCatalogueRepoStub{inventory: models.CommerceInventoryLevel{
		ID: uuid.New(), OrganizationID: organizationID, StoreID: storeID, VariantID: variantID, QuantityOnHand: 3,
	}}
	service := NewCommerceCatalogueService(catalogueRepo, &commerceFoundationRepoStub{})
	reservation, _, err := service.ReserveInventoryForCheckout(context.Background(), organizationID, storeID, variantID, "checkout-2", 2, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("reserve inventory: %v", err)
	}
	released, level, err := service.ReleaseInventoryReservation(context.Background(), organizationID, reservation.ID, false)
	if err != nil {
		t.Fatalf("release inventory: %v", err)
	}
	if released.Status != models.InventoryReservationReleased || level.QuantityOnHand != 3 || level.AvailableQuantity() != 3 {
		t.Fatalf("unexpected released inventory state: reservation=%s on_hand=%d available=%d", released.Status, level.QuantityOnHand, level.AvailableQuantity())
	}
}

func TestConcurrentReservationsCannotOversellFinalUnit(t *testing.T) {
	organizationID, storeID, variantID := uuid.New(), uuid.New(), uuid.New()
	catalogueRepo := &commerceCatalogueRepoStub{inventory: models.CommerceInventoryLevel{
		ID: uuid.New(), OrganizationID: organizationID, StoreID: storeID, VariantID: variantID, QuantityOnHand: 1,
	}}
	service := NewCommerceCatalogueService(catalogueRepo, &commerceFoundationRepoStub{})
	expiresAt := time.Now().Add(time.Hour)
	errorsChannel := make(chan error, 2)
	var waitGroup sync.WaitGroup

	for _, key := range []string{"cart-a", "cart-b"} {
		waitGroup.Add(1)
		go func(reservationKey string) {
			defer waitGroup.Done()
			_, _, err := service.ReserveInventoryForCheckout(context.Background(), organizationID, storeID, variantID, reservationKey, 1, expiresAt)
			errorsChannel <- err
		}(key)
	}
	waitGroup.Wait()
	close(errorsChannel)

	successes, unavailable := 0, 0
	for err := range errorsChannel {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, repository.ErrCommerceInventoryUnavailable):
			unavailable++
		default:
			t.Fatalf("unexpected reservation error: %v", err)
		}
	}
	if successes != 1 || unavailable != 1 {
		t.Fatalf("expected one reservation and one rejection, got successes=%d unavailable=%d", successes, unavailable)
	}
}
