package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/media"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/utils"
)

var currencyPattern = regexp.MustCompile(`^[A-Za-z]{3}$`)

type CreateCommerceCategoryInput struct {
	Name        string
	Slug        string
	Description string
	SortOrder   int
}

type UpdateCommerceCategoryInput struct {
	CreateCommerceCategoryInput
	Status string
}

type CommerceProductVariantInput struct {
	Name       string
	SKU        string
	PriceMinor int64
	Attributes map[string]string
	IsDefault  bool
}

type CommerceProductImageInput struct {
	URL       string
	AltText   string
	SortOrder int
}

type CreateCommerceProductInput struct {
	CategoryID  uuid.UUID
	Name        string
	Slug        string
	Description string
	Currency    string
	Variants    []CommerceProductVariantInput
	Images      []CommerceProductImageInput
}

type UpdateCommerceProductInput struct {
	CategoryID  uuid.UUID
	Name        string
	Slug        string
	Description string
	Currency    string
	Status      string
	Images      []CommerceProductImageInput
}

type UpdateCommerceProductVariantInput struct {
	Name       string
	PriceMinor int64
	Attributes map[string]string
	Status     string
}

type ConfigureCommerceStoreVariantInput struct {
	Enabled            bool
	PriceOverrideMinor *int64
	ReorderThreshold   int
}

type AdjustCommerceInventoryInput struct {
	QuantityDelta int
	Reference     string
	Reason        string
}

type CommerceCatalogueService struct {
	repo           repository.CommerceCatalogueRepository
	foundationRepo repository.CommerceFoundationRepository
	imageUploader  media.ImageUploader
}

func NewCommerceCatalogueService(repo repository.CommerceCatalogueRepository, foundationRepo repository.CommerceFoundationRepository) *CommerceCatalogueService {
	return &CommerceCatalogueService{repo: repo, foundationRepo: foundationRepo}
}

func (s *CommerceCatalogueService) ConfigureImageUploader(uploader media.ImageUploader) {
	s.imageUploader = uploader
}

func (s *CommerceCatalogueService) UploadProductImage(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, fileName string, content []byte) (*media.ImageUpload, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	if _, err := resolveCommerceTenant(actor, requestedOrganizationID); err != nil {
		return nil, err
	}
	if len(content) == 0 || len(content) > 5*1024*1024 {
		return nil, fmt.Errorf("%w: product image must be between 1 byte and 5 MB", ErrCommerceValidation)
	}
	contentType := http.DetectContentType(content)
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" {
		return nil, fmt.Errorf("%w: product image must be JPEG, PNG, or WebP", ErrCommerceValidation)
	}
	if s.imageUploader == nil {
		return nil, errors.New("commerce image uploader is not configured")
	}
	return s.imageUploader.UploadImage(ctx, strings.TrimSpace(fileName), contentType, content)
}

func (s *CommerceCatalogueService) CreateCategory(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, input CreateCommerceCategoryInput) (*models.CommerceCategory, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if _, err := s.foundationRepo.GetMerchantProfile(ctx, organizationID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Name) == "" || !commerceSlugPattern.MatchString(strings.ToLower(strings.TrimSpace(input.Slug))) {
		return nil, fmt.Errorf("%w: category name and a valid slug are required", ErrCommerceValidation)
	}
	if input.SortOrder < 0 {
		return nil, fmt.Errorf("%w: sort order cannot be negative", ErrCommerceValidation)
	}
	category := &models.CommerceCategory{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		Name:           strings.TrimSpace(input.Name),
		Slug:           strings.ToLower(strings.TrimSpace(input.Slug)),
		Description:    strings.TrimSpace(input.Description),
		SortOrder:      input.SortOrder,
		Status:         models.CommerceStatusActive,
	}
	if err := s.repo.CreateCategory(ctx, category); err != nil {
		return nil, err
	}
	return category, nil
}

func (s *CommerceCatalogueService) ListCategories(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID) ([]models.CommerceCategory, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListCategories(ctx, organizationID)
}

func (s *CommerceCatalogueService) UpdateCategory(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, categoryID uuid.UUID, input UpdateCommerceCategoryInput) (*models.CommerceCategory, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if categoryID == uuid.Nil || strings.TrimSpace(input.Name) == "" || !commerceSlugPattern.MatchString(strings.ToLower(strings.TrimSpace(input.Slug))) || input.SortOrder < 0 {
		return nil, fmt.Errorf("%w: category, name, valid slug, and non-negative sort order are required", ErrCommerceValidation)
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if !isCommerceActiveStatus(status) {
		return nil, fmt.Errorf("%w: status must be active or inactive", ErrCommerceValidation)
	}
	category := &models.CommerceCategory{
		ID: categoryID, OrganizationID: organizationID, Name: strings.TrimSpace(input.Name),
		Slug: strings.ToLower(strings.TrimSpace(input.Slug)), Description: strings.TrimSpace(input.Description),
		SortOrder: input.SortOrder, Status: status,
	}
	if err := s.repo.UpdateCategory(ctx, category); err != nil {
		return nil, err
	}
	return category, nil
}

func (s *CommerceCatalogueService) CreateProduct(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, input CreateCommerceProductInput) (*models.CommerceProduct, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if err := validateProduct(input); err != nil {
		return nil, err
	}

	product := &models.CommerceProduct{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		CategoryID:     input.CategoryID,
		Name:           strings.TrimSpace(input.Name),
		Slug:           strings.ToLower(strings.TrimSpace(input.Slug)),
		Description:    strings.TrimSpace(input.Description),
		Currency:       strings.ToUpper(strings.TrimSpace(input.Currency)),
		Status:         models.CommerceStatusActive,
		Variants:       make([]models.CommerceProductVariant, 0, len(input.Variants)),
		Images:         make([]models.CommerceProductImage, 0, len(input.Images)),
	}
	for _, item := range input.Variants {
		attributesInput := item.Attributes
		if attributesInput == nil {
			attributesInput = map[string]string{}
		}
		attributes, err := json.Marshal(attributesInput)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid variant attributes", ErrCommerceValidation)
		}
		product.Variants = append(product.Variants, models.CommerceProductVariant{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			ProductID:      product.ID,
			Name:           strings.TrimSpace(item.Name),
			SKU:            strings.ToUpper(strings.TrimSpace(item.SKU)),
			PriceMinor:     item.PriceMinor,
			Attributes:     attributes,
			IsDefault:      item.IsDefault,
			Status:         models.CommerceStatusActive,
		})
	}
	for _, item := range input.Images {
		product.Images = append(product.Images, models.CommerceProductImage{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			ProductID:      product.ID,
			URL:            strings.TrimSpace(item.URL),
			AltText:        strings.TrimSpace(item.AltText),
			SortOrder:      item.SortOrder,
		})
	}
	if err := s.repo.CreateProduct(ctx, product); err != nil {
		return nil, err
	}
	return product, nil
}

func (s *CommerceCatalogueService) ListProducts(ctx context.Context, actor CommerceActor, requestedOrganizationID, categoryID *uuid.UUID, limit, offset int) ([]models.CommerceProduct, int64, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, 0, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, 0, err
	}
	limit, offset = commercePagination(limit, offset)
	return s.repo.ListProducts(ctx, organizationID, categoryID, limit, offset)
}

func (s *CommerceCatalogueService) GetProduct(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, productID uuid.UUID) (*models.CommerceProduct, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	return s.repo.GetProduct(ctx, organizationID, productID)
}

func (s *CommerceCatalogueService) UpdateProduct(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, productID uuid.UUID, input UpdateCommerceProductInput) (*models.CommerceProduct, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if productID == uuid.Nil || input.CategoryID == uuid.Nil || strings.TrimSpace(input.Name) == "" || !commerceSlugPattern.MatchString(strings.ToLower(strings.TrimSpace(input.Slug))) || !currencyPattern.MatchString(strings.TrimSpace(input.Currency)) {
		return nil, fmt.Errorf("%w: category, product name, valid slug, and currency are required", ErrCommerceValidation)
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if !isCommerceActiveStatus(status) {
		return nil, fmt.Errorf("%w: status must be active or inactive", ErrCommerceValidation)
	}
	if err := validateCommerceProductImages(input.Images); err != nil {
		return nil, err
	}
	product, err := s.repo.GetProduct(ctx, organizationID, productID)
	if err != nil {
		return nil, err
	}
	product.CategoryID = input.CategoryID
	product.Name = strings.TrimSpace(input.Name)
	product.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	product.Description = strings.TrimSpace(input.Description)
	product.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	product.Status = status
	product.Images = make([]models.CommerceProductImage, 0, len(input.Images))
	for _, item := range input.Images {
		product.Images = append(product.Images, models.CommerceProductImage{
			ID: uuid.New(), OrganizationID: organizationID, ProductID: product.ID,
			URL: strings.TrimSpace(item.URL), AltText: strings.TrimSpace(item.AltText), SortOrder: item.SortOrder,
		})
	}
	if err := s.repo.UpdateProduct(ctx, product); err != nil {
		return nil, err
	}
	return product, nil
}

func (s *CommerceCatalogueService) UpdateProductVariant(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, productID, variantID uuid.UUID, input UpdateCommerceProductVariantInput) (*models.CommerceProductVariant, error) {
	if !canManageMerchant(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if productID == uuid.Nil || variantID == uuid.Nil || strings.TrimSpace(input.Name) == "" || input.PriceMinor < 0 || !isCommerceActiveStatus(status) {
		return nil, fmt.Errorf("%w: product, variant, name, non-negative price, and valid status are required", ErrCommerceValidation)
	}
	variant, err := s.repo.GetProductVariant(ctx, organizationID, variantID)
	if err != nil {
		return nil, err
	}
	if variant.ProductID != productID {
		return nil, repository.ErrCommerceNotFound
	}
	attributesInput := input.Attributes
	if attributesInput == nil {
		attributesInput = map[string]string{}
	}
	attributes, err := json.Marshal(attributesInput)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid variant attributes", ErrCommerceValidation)
	}
	variant.Name = strings.TrimSpace(input.Name)
	variant.PriceMinor = input.PriceMinor
	variant.Attributes = attributes
	variant.Status = status
	if err := s.repo.UpdateProductVariant(ctx, variant); err != nil {
		return nil, err
	}
	return variant, nil
}

func (s *CommerceCatalogueService) ConfigureStoreVariant(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, storeID, variantID uuid.UUID, input ConfigureCommerceStoreVariantInput) (*models.CommerceStoreCatalogueItem, *models.CommerceInventoryLevel, error) {
	if !canManageInventory(actor.Role) {
		return nil, nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, storeID, storeScope(actor)); err != nil {
		return nil, nil, err
	}
	if _, err := s.repo.GetProductVariant(ctx, organizationID, variantID); err != nil {
		return nil, nil, err
	}
	if input.PriceOverrideMinor != nil && *input.PriceOverrideMinor < 0 {
		return nil, nil, fmt.Errorf("%w: price override cannot be negative", ErrCommerceValidation)
	}
	if input.ReorderThreshold < 0 {
		return nil, nil, fmt.Errorf("%w: reorder threshold cannot be negative", ErrCommerceValidation)
	}
	item, err := s.repo.ConfigureStoreCatalogueItem(ctx, &models.CommerceStoreCatalogueItem{
		ID:                 uuid.New(),
		OrganizationID:     organizationID,
		StoreID:            storeID,
		VariantID:          variantID,
		Enabled:            input.Enabled,
		PriceOverrideMinor: input.PriceOverrideMinor,
	}, input.ReorderThreshold)
	if err != nil {
		return nil, nil, err
	}
	level, err := s.repo.GetInventoryLevel(ctx, organizationID, storeID, variantID)
	return item, level, err
}

func (s *CommerceCatalogueService) ListStoreCatalogue(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, storeID uuid.UUID) ([]repository.CommerceStoreCatalogueEntry, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, storeID, storeScope(actor)); err != nil {
		return nil, err
	}
	return s.repo.ListStoreCatalogue(ctx, organizationID, storeID)
}

func (s *CommerceCatalogueService) GetInventoryLevel(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, storeID, variantID uuid.UUID) (*models.CommerceInventoryLevel, error) {
	if !canAccessCommerce(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, storeID, storeScope(actor)); err != nil {
		return nil, err
	}
	return s.repo.GetInventoryLevel(ctx, organizationID, storeID, variantID)
}

func (s *CommerceCatalogueService) AdjustInventory(ctx context.Context, actor CommerceActor, requestedOrganizationID *uuid.UUID, storeID, variantID uuid.UUID, input AdjustCommerceInventoryInput) (*models.CommerceInventoryLevel, error) {
	if !canManageInventory(actor.Role) {
		return nil, ErrCommerceForbidden
	}
	organizationID, err := resolveCommerceTenant(actor, requestedOrganizationID)
	if err != nil {
		return nil, err
	}
	if _, err := s.foundationRepo.GetStore(ctx, organizationID, storeID, storeScope(actor)); err != nil {
		return nil, err
	}
	if input.QuantityDelta == 0 || strings.TrimSpace(input.Reference) == "" || strings.TrimSpace(input.Reason) == "" {
		return nil, fmt.Errorf("%w: non-zero quantity delta, reference, and reason are required", ErrCommerceValidation)
	}
	return s.repo.AdjustInventory(ctx, repository.InventoryAdjustment{
		OrganizationID: organizationID,
		StoreID:        storeID,
		VariantID:      variantID,
		QuantityDelta:  input.QuantityDelta,
		Reference:      strings.TrimSpace(input.Reference),
		Reason:         strings.TrimSpace(input.Reason),
		ActorUserID:    actor.UserID,
	})
}

func (s *CommerceCatalogueService) ReserveInventoryForCheckout(ctx context.Context, organizationID, storeID, variantID uuid.UUID, reservationKey string, quantity int, expiresAt time.Time) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error) {
	if organizationID == uuid.Nil || storeID == uuid.Nil || variantID == uuid.Nil || strings.TrimSpace(reservationKey) == "" || quantity < 1 {
		return nil, nil, fmt.Errorf("%w: valid tenant, store, variant, reservation key, and quantity are required", ErrCommerceValidation)
	}
	now := time.Now().UTC()
	if !expiresAt.After(now) || expiresAt.After(now.Add(24*time.Hour)) {
		return nil, nil, fmt.Errorf("%w: reservation expiry must be within the next 24 hours", ErrCommerceValidation)
	}
	if _, err := s.repo.ExpireInventoryReservations(ctx, organizationID, storeID, variantID, now, 100); err != nil {
		return nil, nil, err
	}
	return s.repo.ReserveInventory(ctx, repository.InventoryReservationRequest{
		OrganizationID: organizationID,
		StoreID:        storeID,
		VariantID:      variantID,
		ReservationKey: strings.TrimSpace(reservationKey),
		Quantity:       quantity,
		ExpiresAt:      expiresAt.UTC(),
	})
}

func (s *CommerceCatalogueService) CommitInventoryReservation(ctx context.Context, organizationID, reservationID uuid.UUID) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error) {
	if organizationID == uuid.Nil || reservationID == uuid.Nil {
		return nil, nil, fmt.Errorf("%w: tenant and reservation are required", ErrCommerceValidation)
	}
	return s.repo.CommitInventoryReservation(ctx, organizationID, reservationID)
}

func (s *CommerceCatalogueService) ReleaseInventoryReservation(ctx context.Context, organizationID, reservationID uuid.UUID, expired bool) (*models.CommerceInventoryReservation, *models.CommerceInventoryLevel, error) {
	if organizationID == uuid.Nil || reservationID == uuid.Nil {
		return nil, nil, fmt.Errorf("%w: tenant and reservation are required", ErrCommerceValidation)
	}
	return s.repo.ReleaseInventoryReservation(ctx, organizationID, reservationID, expired)
}

func canManageInventory(role string) bool {
	return canManageMerchant(role) || strings.EqualFold(role, utils.RoleStoreManager)
}

func commercePagination(limit, offset int) (int, int) {
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func validateProduct(input CreateCommerceProductInput) error {
	if input.CategoryID == uuid.Nil || strings.TrimSpace(input.Name) == "" || !commerceSlugPattern.MatchString(strings.ToLower(strings.TrimSpace(input.Slug))) {
		return fmt.Errorf("%w: category, product name, and a valid slug are required", ErrCommerceValidation)
	}
	if !currencyPattern.MatchString(strings.TrimSpace(input.Currency)) {
		return fmt.Errorf("%w: currency must be a 3-letter ISO code", ErrCommerceValidation)
	}
	if len(input.Variants) == 0 {
		return fmt.Errorf("%w: at least one product variant is required", ErrCommerceValidation)
	}
	defaultCount := 0
	skus := make(map[string]struct{}, len(input.Variants))
	for _, item := range input.Variants {
		sku := strings.ToUpper(strings.TrimSpace(item.SKU))
		if strings.TrimSpace(item.Name) == "" || sku == "" || item.PriceMinor < 0 {
			return fmt.Errorf("%w: each variant requires a name, SKU, and non-negative price", ErrCommerceValidation)
		}
		if _, exists := skus[sku]; exists {
			return fmt.Errorf("%w: duplicate variant SKU", ErrCommerceValidation)
		}
		skus[sku] = struct{}{}
		if item.IsDefault {
			defaultCount++
		}
	}
	if defaultCount != 1 {
		return fmt.Errorf("%w: exactly one default variant is required", ErrCommerceValidation)
	}
	return validateCommerceProductImages(input.Images)
}

func validateCommerceProductImages(images []CommerceProductImageInput) error {
	imageURLs := make(map[string]struct{}, len(images))
	for _, item := range images {
		parsed, err := url.ParseRequestURI(strings.TrimSpace(item.URL))
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || item.SortOrder < 0 {
			return fmt.Errorf("%w: each image requires a public HTTPS URL and non-negative sort order", ErrCommerceValidation)
		}
		if _, exists := imageURLs[parsed.String()]; exists {
			return fmt.Errorf("%w: duplicate product image URL", ErrCommerceValidation)
		}
		imageURLs[parsed.String()] = struct{}{}
	}
	return nil
}
