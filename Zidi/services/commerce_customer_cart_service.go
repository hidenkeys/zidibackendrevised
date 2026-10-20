package services

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

const (
	commerceCartLifetime    = 30 * 24 * time.Hour
	commerceCartMaxQuantity = 100
)

var (
	ErrCommerceCartItemUnavailable = errors.New("commerce cart item is unavailable")
	ErrCommerceCartCurrency        = errors.New("commerce cart currency mismatch")
)

type ResolveCommerceCustomerInput struct {
	Channel     string
	Identifier  string
	DisplayName string
	Email       string
	Verified    bool
}

type CreateCommerceCartInput struct {
	CustomerID uuid.UUID
	StoreID    uuid.UUID
}

type CommerceCartLine struct {
	Item              models.CommerceCartItem
	ProductID         *uuid.UUID
	ProductName       *string
	VariantName       *string
	SKU               *string
	PrimaryImageURL   *string
	UnitPriceMinor    *int64
	LineTotalMinor    *int64
	AvailableQuantity int
	Available         bool
	UnavailableReason *string
}

type CommerceCartView struct {
	Cart          *models.CommerceCart
	Items         []CommerceCartLine
	ItemCount     int
	SubtotalMinor int64
	TotalMinor    int64
	CheckoutReady bool
}

type CommerceCustomerCartService struct {
	customerRepo   repository.CommerceCustomerRepository
	cartRepo       repository.CommerceCartRepository
	catalogueRepo  repository.CommerceCatalogueRepository
	foundationRepo repository.CommerceFoundationRepository
}

func NewCommerceCustomerCartService(customerRepo repository.CommerceCustomerRepository, cartRepo repository.CommerceCartRepository, catalogueRepo repository.CommerceCatalogueRepository, foundationRepo repository.CommerceFoundationRepository) *CommerceCustomerCartService {
	return &CommerceCustomerCartService{
		customerRepo:   customerRepo,
		cartRepo:       cartRepo,
		catalogueRepo:  catalogueRepo,
		foundationRepo: foundationRepo,
	}
}

func (s *CommerceCustomerCartService) ResolveCustomer(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, input ResolveCommerceCustomerInput) (*models.CommerceCustomer, bool, error) {
	if !canManageMerchant(actor.Role) {
		return nil, false, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, false, err
	}
	return s.resolveCustomer(ctx, organizationID, input)
}

// ResolveCustomerForChannel supports trusted channel adapters such as the WhatsApp webhook.
func (s *CommerceCustomerCartService) ResolveCustomerForChannel(ctx context.Context, organizationID uuid.UUID, input ResolveCommerceCustomerInput) (*models.CommerceCustomer, bool, error) {
	if organizationID == uuid.Nil {
		return nil, false, fmt.Errorf("%w: organization is required", ErrCommerceValidation)
	}
	return s.resolveCustomer(ctx, organizationID, input)
}

func (s *CommerceCustomerCartService) GetCustomer(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, customerID uuid.UUID) (*models.CommerceCustomer, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if customerID == uuid.Nil {
		return nil, fmt.Errorf("%w: customer is required", ErrCommerceValidation)
	}
	return s.customerRepo.GetCustomer(ctx, organizationID, customerID)
}

func (s *CommerceCustomerCartService) CreateCart(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, input CreateCommerceCartInput) (*CommerceCartView, bool, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, false, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, false, err
	}
	return s.createCart(ctx, organizationID, input, storeScope(actor))
}

// CreateCartForChannel is restricted to the customer identity already resolved by
// a trusted channel adapter. The customer cannot create or mutate another cart.
func (s *CommerceCustomerCartService) CreateCartForChannel(ctx context.Context, organizationID, customerID, storeID uuid.UUID) (*CommerceCartView, bool, error) {
	if organizationID == uuid.Nil || customerID == uuid.Nil || storeID == uuid.Nil {
		return nil, false, ErrCommerceForbidden
	}
	return s.createCart(ctx, organizationID, CreateCommerceCartInput{CustomerID: customerID, StoreID: storeID}, nil)
}

func (s *CommerceCustomerCartService) createCart(ctx context.Context, organizationID uuid.UUID, input CreateCommerceCartInput, assignedUserID *uuid.UUID) (*CommerceCartView, bool, error) {
	if input.CustomerID == uuid.Nil || input.StoreID == uuid.Nil {
		return nil, false, fmt.Errorf("%w: customer and store are required", ErrCommerceValidation)
	}
	if _, err := s.customerRepo.GetCustomer(ctx, organizationID, input.CustomerID); err != nil {
		return nil, false, err
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, input.StoreID, assignedUserID); err != nil {
		return nil, false, err
	}
	profile, err := s.foundationRepo.GetMerchantProfile(ctx, organizationID)
	if err != nil {
		return nil, false, err
	}

	now := time.Now().UTC()
	cart, created, err := s.cartRepo.GetOrCreateActiveCart(ctx, &models.CommerceCart{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		CustomerID:     input.CustomerID,
		StoreID:        input.StoreID,
		Currency:       strings.ToUpper(strings.TrimSpace(profile.DefaultCurrency)),
		Status:         models.CommerceCartStatusActive,
		Version:        1,
		ExpiresAt:      now.Add(commerceCartLifetime),
	})
	if err != nil {
		return nil, false, err
	}
	view, err := s.hydrateCart(ctx, cart)
	return view, created, err
}

func (s *CommerceCustomerCartService) GetCart(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, cartID uuid.UUID) (*CommerceCartView, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	return s.getCart(ctx, organizationID, cartID, nil, storeScope(actor))
}

func (s *CommerceCustomerCartService) GetCartForChannel(ctx context.Context, organizationID, customerID, cartID uuid.UUID) (*CommerceCartView, error) {
	if organizationID == uuid.Nil || customerID == uuid.Nil || cartID == uuid.Nil {
		return nil, ErrCommerceForbidden
	}
	return s.getCart(ctx, organizationID, cartID, &customerID, nil)
}

func (s *CommerceCustomerCartService) getCart(ctx context.Context, organizationID, cartID uuid.UUID, expectedCustomerID, assignedUserID *uuid.UUID) (*CommerceCartView, error) {
	cart, err := s.cartRepo.GetActiveCart(ctx, organizationID, cartID)
	if err != nil {
		return nil, err
	}
	if expectedCustomerID != nil && cart.CustomerID != *expectedCustomerID {
		return nil, ErrCommerceForbidden
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, cart.StoreID, assignedUserID); err != nil {
		return nil, err
	}
	return s.hydrateCart(ctx, cart)
}

func (s *CommerceCustomerCartService) SetCartItem(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, cartID, variantID uuid.UUID, quantity int) (*CommerceCartView, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	return s.setCartItem(ctx, organizationID, nil, cartID, variantID, quantity, storeScope(actor))
}

func (s *CommerceCustomerCartService) SetCartItemForChannel(ctx context.Context, organizationID, customerID, cartID, variantID uuid.UUID, quantity int) (*CommerceCartView, error) {
	if organizationID == uuid.Nil || customerID == uuid.Nil {
		return nil, ErrCommerceForbidden
	}
	return s.setCartItem(ctx, organizationID, &customerID, cartID, variantID, quantity, nil)
}

func (s *CommerceCustomerCartService) setCartItem(ctx context.Context, organizationID uuid.UUID, expectedCustomerID *uuid.UUID, cartID, variantID uuid.UUID, quantity int, assignedUserID *uuid.UUID) (*CommerceCartView, error) {
	if cartID == uuid.Nil || variantID == uuid.Nil || quantity < 1 || quantity > commerceCartMaxQuantity {
		return nil, fmt.Errorf("%w: cart, variant, and quantity between 1 and %d are required", ErrCommerceValidation, commerceCartMaxQuantity)
	}
	cart, err := s.cartRepo.GetActiveCart(ctx, organizationID, cartID)
	if err != nil {
		return nil, err
	}
	if expectedCustomerID != nil && cart.CustomerID != *expectedCustomerID {
		return nil, ErrCommerceForbidden
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, cart.StoreID, assignedUserID); err != nil {
		return nil, err
	}
	entry, err := s.catalogueRepo.GetStoreCatalogueEntry(ctx, organizationID, cart.StoreID, variantID)
	if err != nil {
		if errors.Is(err, repository.ErrCommerceNotFound) {
			return nil, ErrCommerceCartItemUnavailable
		}
		return nil, err
	}
	if !entry.Enabled || entry.AvailableQuantity < quantity {
		return nil, ErrCommerceCartItemUnavailable
	}
	if !strings.EqualFold(entry.ProductCurrency, cart.Currency) {
		return nil, ErrCommerceCartCurrency
	}

	cart, err = s.cartRepo.SetCartItem(ctx, organizationID, cartID, variantID, quantity)
	if err != nil {
		return nil, err
	}
	return s.hydrateCart(ctx, cart)
}

func (s *CommerceCustomerCartService) DeleteCartItem(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, cartID, variantID uuid.UUID) (*CommerceCartView, error) {
	if variantID == uuid.Nil {
		return nil, fmt.Errorf("%w: variant is required", ErrCommerceValidation)
	}
	organizationID, cart, err := s.authorizeCart(ctx, actor, requestedOrganizationID, cartID)
	if err != nil {
		return nil, err
	}
	cart, err = s.cartRepo.DeleteCartItem(ctx, organizationID, cart.ID, variantID)
	if err != nil {
		return nil, err
	}
	return s.hydrateCart(ctx, cart)
}

func (s *CommerceCustomerCartService) ClearCart(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, cartID uuid.UUID) (*CommerceCartView, error) {
	organizationID, cart, err := s.authorizeCart(ctx, actor, requestedOrganizationID, cartID)
	if err != nil {
		return nil, err
	}
	cart, err = s.cartRepo.ClearCart(ctx, organizationID, cart.ID)
	if err != nil {
		return nil, err
	}
	return s.hydrateCart(ctx, cart)
}

func (s *CommerceCustomerCartService) resolveCustomer(ctx context.Context, organizationID uuid.UUID, input ResolveCommerceCustomerInput) (*models.CommerceCustomer, bool, error) {
	if _, err := s.foundationRepo.GetMerchantProfile(ctx, organizationID); err != nil {
		return nil, false, err
	}
	channel, normalizedIdentifier, displayIdentifier, err := normalizeCommerceIdentity(input.Channel, input.Identifier)
	if err != nil {
		return nil, false, err
	}
	email, err := normalizeCommerceCustomerEmail(input.Email)
	if err != nil {
		return nil, false, err
	}
	if channel == models.CommerceIdentityChannelEmail && email == nil {
		value := normalizedIdentifier
		email = &value
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = displayIdentifier
	}
	if len(displayName) > 200 {
		return nil, false, fmt.Errorf("%w: display name is too long", ErrCommerceValidation)
	}

	var verifiedAt *time.Time
	if input.Verified {
		now := time.Now().UTC()
		verifiedAt = &now
	}
	customer := &models.CommerceCustomer{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		DisplayName:    displayName,
		Email:          email,
		Status:         models.CommerceStatusActive,
	}
	identity := &models.CommerceCustomerIdentity{
		ID:                   uuid.New(),
		OrganizationID:       organizationID,
		Channel:              channel,
		NormalizedIdentifier: normalizedIdentifier,
		DisplayIdentifier:    displayIdentifier,
		VerifiedAt:           verifiedAt,
	}
	return s.customerRepo.ResolveCustomerIdentity(ctx, customer, identity)
}

func (s *CommerceCustomerCartService) authorizeCart(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, cartID uuid.UUID) (uuid.UUID, *models.CommerceCart, error) {
	if !canAccessCommerce(actor.Role) {
		return uuid.Nil, nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if cartID == uuid.Nil {
		return uuid.Nil, nil, fmt.Errorf("%w: cart is required", ErrCommerceValidation)
	}
	cart, err := s.cartRepo.GetActiveCart(ctx, organizationID, cartID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, cart.StoreID, storeScope(actor)); err != nil {
		return uuid.Nil, nil, err
	}
	return organizationID, cart, nil
}

func (s *CommerceCustomerCartService) hydrateCart(ctx context.Context, cart *models.CommerceCart) (*CommerceCartView, error) {
	entries, err := s.catalogueRepo.ListStoreCatalogue(ctx, cart.OrganizationID, cart.StoreID)
	if err != nil {
		return nil, err
	}
	entryByVariant := make(map[uuid.UUID]repository.CommerceStoreCatalogueEntry, len(entries))
	for _, entry := range entries {
		entryByVariant[entry.VariantID] = entry
	}

	view := &CommerceCartView{
		Cart:          cart,
		Items:         make([]CommerceCartLine, 0, len(cart.Items)),
		CheckoutReady: len(cart.Items) > 0,
	}
	for _, item := range cart.Items {
		line := CommerceCartLine{Item: item}
		entry, exists := entryByVariant[item.VariantID]
		if !exists {
			reason := "product is no longer in this store's active catalogue"
			line.UnavailableReason = &reason
			view.CheckoutReady = false
			view.Items = append(view.Items, line)
			view.ItemCount += item.Quantity
			continue
		}
		if !strings.EqualFold(entry.ProductCurrency, cart.Currency) {
			return nil, ErrCommerceCartCurrency
		}

		unitPrice := entry.EffectivePriceMinor
		lineTotal := unitPrice * int64(item.Quantity)
		productID, productName := entry.ProductID, entry.ProductName
		variantName, sku := entry.VariantName, entry.SKU
		line.ProductID = &productID
		line.ProductName = &productName
		line.VariantName = &variantName
		line.SKU = &sku
		line.PrimaryImageURL = entry.PrimaryImageURL
		line.UnitPriceMinor = &unitPrice
		line.LineTotalMinor = &lineTotal
		line.AvailableQuantity = entry.AvailableQuantity
		line.Available = entry.Enabled && entry.AvailableQuantity >= item.Quantity
		if !line.Available {
			reason := "requested quantity is currently unavailable"
			if !entry.Enabled {
				reason = "product is not currently offered by this store"
			}
			line.UnavailableReason = &reason
			view.CheckoutReady = false
		}
		view.Items = append(view.Items, line)
		view.ItemCount += item.Quantity
		view.SubtotalMinor += lineTotal
	}
	view.TotalMinor = view.SubtotalMinor
	return view, nil
}

func normalizeCommerceIdentity(channel, identifier string) (string, string, string, error) {
	channel = strings.ToLower(strings.TrimSpace(channel))
	displayIdentifier := strings.TrimSpace(identifier)
	if displayIdentifier == "" {
		return "", "", "", fmt.Errorf("%w: identity identifier is required", ErrCommerceValidation)
	}

	switch channel {
	case models.CommerceIdentityChannelWhatsApp, models.CommerceIdentityChannelPhone:
		var digits strings.Builder
		for _, character := range displayIdentifier {
			switch {
			case character >= '0' && character <= '9':
				digits.WriteRune(character)
			case strings.ContainsRune("+ -().", character):
			default:
				return "", "", "", fmt.Errorf("%w: phone identity contains invalid characters", ErrCommerceValidation)
			}
		}
		normalized := digits.String()
		if strings.HasPrefix(normalized, "00") {
			normalized = strings.TrimPrefix(normalized, "00")
		}
		if len(normalized) < 7 || len(normalized) > 15 {
			return "", "", "", fmt.Errorf("%w: phone identity must contain 7 to 15 digits including country code", ErrCommerceValidation)
		}
		return channel, normalized, displayIdentifier, nil
	case models.CommerceIdentityChannelEmail:
		parsed, err := mail.ParseAddress(displayIdentifier)
		if err != nil || !strings.Contains(parsed.Address, "@") {
			return "", "", "", fmt.Errorf("%w: email identity is invalid", ErrCommerceValidation)
		}
		return channel, strings.ToLower(parsed.Address), displayIdentifier, nil
	case models.CommerceIdentityChannelWeb:
		normalized := strings.ToLower(displayIdentifier)
		if len(normalized) < 3 || len(normalized) > 200 {
			return "", "", "", fmt.Errorf("%w: web identity must contain 3 to 200 characters", ErrCommerceValidation)
		}
		return channel, normalized, displayIdentifier, nil
	default:
		return "", "", "", fmt.Errorf("%w: identity channel must be whatsapp, phone, email, or web", ErrCommerceValidation)
	}
}

func normalizeCommerceCustomerEmail(value string) (*string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || !strings.Contains(parsed.Address, "@") {
		return nil, fmt.Errorf("%w: customer email is invalid", ErrCommerceValidation)
	}
	normalized := strings.ToLower(parsed.Address)
	return &normalized, nil
}
