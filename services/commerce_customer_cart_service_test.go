package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/utils"
)

type commerceCustomerRepoStub struct {
	mu         sync.Mutex
	customers  map[uuid.UUID]*models.CommerceCustomer
	identities map[string]uuid.UUID
}

func (s *commerceCustomerRepoStub) ResolveCustomerIdentity(_ context.Context, customer *models.CommerceCustomer, identity *models.CommerceCustomerIdentity) (*models.CommerceCustomer, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.customers == nil {
		s.customers = make(map[uuid.UUID]*models.CommerceCustomer)
		s.identities = make(map[string]uuid.UUID)
	}
	key := fmt.Sprintf("%s:%s:%s", identity.OrganizationID, identity.Channel, identity.NormalizedIdentifier)
	if customerID, exists := s.identities[key]; exists {
		return cloneCommerceCustomer(s.customers[customerID]), false, nil
	}
	identity.CustomerID = customer.ID
	identity.CreatedAt = time.Now().UTC()
	customer.CreatedAt = identity.CreatedAt
	customer.UpdatedAt = identity.CreatedAt
	customer.Identities = []models.CommerceCustomerIdentity{*identity}
	s.customers[customer.ID] = cloneCommerceCustomer(customer)
	s.identities[key] = customer.ID
	return cloneCommerceCustomer(customer), true, nil
}

func (s *commerceCustomerRepoStub) GetCustomer(_ context.Context, organizationID, customerID uuid.UUID) (*models.CommerceCustomer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	customer := s.customers[customerID]
	if customer == nil || customer.OrganizationID != organizationID {
		return nil, repository.ErrCommerceNotFound
	}
	return cloneCommerceCustomer(customer), nil
}

func (s *commerceCustomerRepoStub) UpdateCustomerProfile(_ context.Context, organizationID, customerID uuid.UUID, displayName string, email *string) (*models.CommerceCustomer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	customer := s.customers[customerID]
	if customer == nil || customer.OrganizationID != organizationID {
		return nil, repository.ErrCommerceNotFound
	}
	if displayName != "" {
		customer.DisplayName = displayName
	}
	if email != nil {
		customer.Email = email
	}
	return cloneCommerceCustomer(customer), nil
}

type commerceCartRepoStub struct {
	mu       sync.Mutex
	carts    map[uuid.UUID]*models.CommerceCart
	activeBy map[string]uuid.UUID
}

func (s *commerceCartRepoStub) GetOrCreateActiveCart(_ context.Context, cart *models.CommerceCart) (*models.CommerceCart, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.carts == nil {
		s.carts = make(map[uuid.UUID]*models.CommerceCart)
		s.activeBy = make(map[string]uuid.UUID)
	}
	key := commerceCartStubKey(cart.OrganizationID, cart.CustomerID, cart.StoreID)
	if existingID, exists := s.activeBy[key]; exists {
		return cloneCommerceCart(s.carts[existingID]), false, nil
	}
	now := time.Now().UTC()
	cart.CreatedAt = now
	cart.UpdatedAt = now
	cart.Items = []models.CommerceCartItem{}
	s.carts[cart.ID] = cloneCommerceCart(cart)
	s.activeBy[key] = cart.ID
	return cloneCommerceCart(cart), true, nil
}

func (s *commerceCartRepoStub) GetActiveCart(_ context.Context, organizationID, cartID uuid.UUID) (*models.CommerceCart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cart := s.carts[cartID]
	if cart == nil || cart.OrganizationID != organizationID || cart.Status != models.CommerceCartStatusActive || !cart.ExpiresAt.After(time.Now()) {
		return nil, repository.ErrCommerceNotFound
	}
	return cloneCommerceCart(cart), nil
}

func (s *commerceCartRepoStub) SetCartItem(_ context.Context, organizationID, cartID, variantID uuid.UUID, quantity int) (*models.CommerceCart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cart := s.carts[cartID]
	if cart == nil || cart.OrganizationID != organizationID {
		return nil, repository.ErrCommerceNotFound
	}
	for index := range cart.Items {
		if cart.Items[index].VariantID == variantID {
			cart.Items[index].Quantity = quantity
			cart.Items[index].UpdatedAt = time.Now().UTC()
			cart.Version++
			return cloneCommerceCart(cart), nil
		}
	}
	now := time.Now().UTC()
	cart.Items = append(cart.Items, models.CommerceCartItem{
		ID: uuid.New(), OrganizationID: organizationID, CartID: cartID, VariantID: variantID, Quantity: quantity, CreatedAt: now, UpdatedAt: now,
	})
	cart.Version++
	return cloneCommerceCart(cart), nil
}

func (s *commerceCartRepoStub) DeleteCartItem(_ context.Context, organizationID, cartID, variantID uuid.UUID) (*models.CommerceCart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cart := s.carts[cartID]
	if cart == nil || cart.OrganizationID != organizationID {
		return nil, repository.ErrCommerceNotFound
	}
	items := cart.Items[:0]
	for _, item := range cart.Items {
		if item.VariantID != variantID {
			items = append(items, item)
		}
	}
	cart.Items = items
	cart.Version++
	return cloneCommerceCart(cart), nil
}

func (s *commerceCartRepoStub) ClearCart(_ context.Context, organizationID, cartID uuid.UUID) (*models.CommerceCart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cart := s.carts[cartID]
	if cart == nil || cart.OrganizationID != organizationID {
		return nil, repository.ErrCommerceNotFound
	}
	cart.Items = []models.CommerceCartItem{}
	cart.Version++
	return cloneCommerceCart(cart), nil
}

func TestResolveCommerceCustomerDeduplicatesNormalizedWhatsAppIdentity(t *testing.T) {
	repo := &commerceCustomerRepoStub{}
	service := NewCommerceCustomerCartService(repo, &commerceCartRepoStub{}, &commerceCatalogueRepoStub{}, &commerceFoundationRepoStub{})
	organizationID := uuid.New()

	first, created, err := service.ResolveCustomerForChannel(context.Background(), organizationID, ResolveCommerceCustomerInput{
		Channel: models.CommerceIdentityChannelWhatsApp, Identifier: "+234 803 123 4567", DisplayName: "Ada",
	})
	if err != nil || !created {
		t.Fatalf("resolve first customer: created=%v err=%v", created, err)
	}
	second, created, err := service.ResolveCustomerForChannel(context.Background(), organizationID, ResolveCommerceCustomerInput{
		Channel: models.CommerceIdentityChannelWhatsApp, Identifier: "002348031234567", DisplayName: "Duplicate",
	})
	if err != nil || created {
		t.Fatalf("resolve duplicate customer: created=%v err=%v", created, err)
	}
	if first.ID != second.ID || first.Identities[0].NormalizedIdentifier != "2348031234567" {
		t.Fatalf("WhatsApp identity was not normalized and deduplicated: first=%+v second=%+v", first, second)
	}
}

func TestCommerceCustomerIdentityIsTenantScoped(t *testing.T) {
	repo := &commerceCustomerRepoStub{}
	service := NewCommerceCustomerCartService(repo, &commerceCartRepoStub{}, &commerceCatalogueRepoStub{}, &commerceFoundationRepoStub{})

	first, _, err := service.ResolveCustomerForChannel(context.Background(), uuid.New(), ResolveCommerceCustomerInput{Channel: "whatsapp", Identifier: "+2348031234567"})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := service.ResolveCustomerForChannel(context.Background(), uuid.New(), ResolveCommerceCustomerInput{Channel: "whatsapp", Identifier: "+2348031234567"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("the same channel identity was shared across tenants")
	}
}

func TestStoreStaffCannotResolveArbitraryCommerceCustomers(t *testing.T) {
	service := NewCommerceCustomerCartService(&commerceCustomerRepoStub{}, &commerceCartRepoStub{}, &commerceCatalogueRepoStub{}, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: uuid.New(), Role: utils.RoleStoreStaff}

	_, _, err := service.ResolveCustomer(context.Background(), actor, nil, ResolveCommerceCustomerInput{Channel: "whatsapp", Identifier: "+2348031234567"})
	if !errors.Is(err, ErrCommerceForbidden) {
		t.Fatalf("expected store staff customer resolution to be forbidden, got %v", err)
	}
}

func TestCommerceCartIsIdempotentPerCustomerAndStore(t *testing.T) {
	organizationID, customerID, storeID := uuid.New(), uuid.New(), uuid.New()
	customerRepo := seededCommerceCustomerRepo(organizationID, customerID)
	cartRepo := &commerceCartRepoStub{}
	service := NewCommerceCustomerCartService(customerRepo, cartRepo, &commerceCatalogueRepoStub{}, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}

	first, created, err := service.CreateCart(context.Background(), actor, nil, CreateCommerceCartInput{CustomerID: customerID, StoreID: storeID})
	if err != nil || !created {
		t.Fatalf("create first cart: created=%v err=%v", created, err)
	}
	second, created, err := service.CreateCart(context.Background(), actor, nil, CreateCommerceCartInput{CustomerID: customerID, StoreID: storeID})
	if err != nil || created {
		t.Fatalf("resolve active cart: created=%v err=%v", created, err)
	}
	if first.Cart.ID != second.Cart.ID || first.Cart.Currency != "NGN" {
		t.Fatalf("active cart was not reused with merchant currency: first=%+v second=%+v", first.Cart, second.Cart)
	}

	other, created, err := service.CreateCart(context.Background(), actor, nil, CreateCommerceCartInput{CustomerID: customerID, StoreID: uuid.New()})
	if err != nil || !created || other.Cart.ID == first.Cart.ID {
		t.Fatalf("a different store did not receive a distinct cart: created=%v err=%v", created, err)
	}
}

func TestCartUsesCurrentServerPriceAndRecalculatesAfterPriceChange(t *testing.T) {
	organizationID, customerID, storeID, variantID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	customerRepo := seededCommerceCustomerRepo(organizationID, customerID)
	cartRepo := &commerceCartRepoStub{}
	catalogueRepo := &commerceCatalogueRepoStub{storeCatalogue: []repository.CommerceStoreCatalogueEntry{{
		StoreID: storeID, ProductID: uuid.New(), ProductName: "Fruit Tea", ProductCurrency: "NGN", VariantID: variantID, VariantName: "Regular", SKU: "TEA-1", EffectivePriceMinor: 420000, Enabled: true, AvailableQuantity: 5,
	}}}
	service := NewCommerceCustomerCartService(customerRepo, cartRepo, catalogueRepo, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}
	view, _, err := service.CreateCart(context.Background(), actor, nil, CreateCommerceCartInput{CustomerID: customerID, StoreID: storeID})
	if err != nil {
		t.Fatal(err)
	}

	view, err = service.SetCartItem(context.Background(), actor, nil, view.Cart.ID, variantID, 2)
	if err != nil {
		t.Fatalf("set cart item: %v", err)
	}
	if view.SubtotalMinor != 840000 || view.TotalMinor != 840000 || !view.CheckoutReady {
		t.Fatalf("server total is not based on current catalogue price: %+v", view)
	}

	catalogueRepo.storeCatalogue[0].EffectivePriceMinor = 450000
	view, err = service.GetCart(context.Background(), actor, nil, view.Cart.ID)
	if err != nil {
		t.Fatalf("get repriced cart: %v", err)
	}
	if view.TotalMinor != 900000 || view.Items[0].UnitPriceMinor == nil || *view.Items[0].UnitPriceMinor != 450000 {
		t.Fatalf("cart did not recalculate using the changed server price: %+v", view)
	}
}

func TestCartRejectsUnavailableOrDifferentStoreVariant(t *testing.T) {
	organizationID, customerID, cartStoreID := uuid.New(), uuid.New(), uuid.New()
	variantID := uuid.New()
	customerRepo := seededCommerceCustomerRepo(organizationID, customerID)
	cartRepo := &commerceCartRepoStub{}
	catalogueRepo := &commerceCatalogueRepoStub{storeCatalogue: []repository.CommerceStoreCatalogueEntry{{
		StoreID: uuid.New(), ProductCurrency: "NGN", VariantID: variantID, Enabled: true, AvailableQuantity: 10,
	}}}
	service := NewCommerceCustomerCartService(customerRepo, cartRepo, catalogueRepo, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}
	view, _, err := service.CreateCart(context.Background(), actor, nil, CreateCommerceCartInput{CustomerID: customerID, StoreID: cartStoreID})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.SetCartItem(context.Background(), actor, nil, view.Cart.ID, variantID, 1)
	if !errors.Is(err, ErrCommerceCartItemUnavailable) {
		t.Fatalf("expected a cross-store variant to be unavailable, got %v", err)
	}
	stored, _ := cartRepo.GetActiveCart(context.Background(), organizationID, view.Cart.ID)
	if len(stored.Items) != 0 {
		t.Fatal("unavailable item was persisted in the cart")
	}
}

func TestExistingCartBecomesNotReadyWhenInventoryChanges(t *testing.T) {
	organizationID, customerID, storeID, variantID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	customerRepo := seededCommerceCustomerRepo(organizationID, customerID)
	cartRepo := &commerceCartRepoStub{}
	catalogueRepo := &commerceCatalogueRepoStub{storeCatalogue: []repository.CommerceStoreCatalogueEntry{{
		StoreID: storeID, ProductID: uuid.New(), ProductName: "Tea", ProductCurrency: "NGN", VariantID: variantID, VariantName: "Cup", EffectivePriceMinor: 300000, Enabled: true, AvailableQuantity: 2,
	}}}
	service := NewCommerceCustomerCartService(customerRepo, cartRepo, catalogueRepo, &commerceFoundationRepoStub{})
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleMerchantAdmin}
	view, _, _ := service.CreateCart(context.Background(), actor, nil, CreateCommerceCartInput{CustomerID: customerID, StoreID: storeID})
	view, err := service.SetCartItem(context.Background(), actor, nil, view.Cart.ID, variantID, 2)
	if err != nil {
		t.Fatal(err)
	}
	catalogueRepo.storeCatalogue[0].AvailableQuantity = 1

	view, err = service.GetCart(context.Background(), actor, nil, view.Cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.CheckoutReady || view.Items[0].Available || view.Items[0].UnavailableReason == nil {
		t.Fatalf("cart did not reflect current inventory: %+v", view)
	}
}

func TestStoreCartAccessUsesAssignedStoreScope(t *testing.T) {
	organizationID, customerID, storeID := uuid.New(), uuid.New(), uuid.New()
	foundationRepo := &commerceFoundationRepoStub{}
	service := NewCommerceCustomerCartService(seededCommerceCustomerRepo(organizationID, customerID), &commerceCartRepoStub{}, &commerceCatalogueRepoStub{}, foundationRepo)
	actor := CommerceActor{UserID: uuid.New(), OrganizationID: organizationID, Role: utils.RoleStoreStaff}

	if _, _, err := service.CreateCart(context.Background(), actor, nil, CreateCommerceCartInput{CustomerID: customerID, StoreID: storeID}); err != nil {
		t.Fatal(err)
	}
	if foundationRepo.listAssignedUserID == nil || *foundationRepo.listAssignedUserID != actor.UserID {
		t.Fatal("store staff cart access was not scoped to an active store assignment")
	}
}

func seededCommerceCustomerRepo(organizationID, customerID uuid.UUID) *commerceCustomerRepoStub {
	return &commerceCustomerRepoStub{customers: map[uuid.UUID]*models.CommerceCustomer{
		customerID: {ID: customerID, OrganizationID: organizationID, DisplayName: "Customer", Status: models.CommerceStatusActive},
	}, identities: make(map[string]uuid.UUID)}
}

func cloneCommerceCustomer(customer *models.CommerceCustomer) *models.CommerceCustomer {
	copy := *customer
	copy.Identities = append([]models.CommerceCustomerIdentity(nil), customer.Identities...)
	return &copy
}

func cloneCommerceCart(cart *models.CommerceCart) *models.CommerceCart {
	copy := *cart
	copy.Items = append([]models.CommerceCartItem(nil), cart.Items...)
	return &copy
}

func commerceCartStubKey(organizationID, customerID, storeID uuid.UUID) string {
	return organizationID.String() + ":" + customerID.String() + ":" + storeID.String()
}
