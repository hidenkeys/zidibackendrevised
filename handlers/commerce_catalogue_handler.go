package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/services"
)

func (s Server) CreateCommerceCategory(c *fiber.Ctx) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.CreateCommerceCategoryJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	category, err := s.commerceCatalogueService.CreateCategory(c.UserContext(), actor, request.OrganizationId, services.CreateCommerceCategoryInput{
		Name:        request.Name,
		Slug:        request.Slug,
		Description: optionalString(request.Description),
		SortOrder:   request.SortOrder,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.Status(http.StatusCreated).JSON(commerceCategoryResponse(category))
}

func (s Server) ListCommerceCategories(c *fiber.Ctx, params api.ListCommerceCategoriesParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	categories, err := s.commerceCatalogueService.ListCategories(c.UserContext(), actor, params.OrganizationId)
	if err != nil {
		return commerceError(c, err)
	}
	response := make([]api.CommerceCategory, 0, len(categories))
	for index := range categories {
		response = append(response, commerceCategoryResponse(&categories[index]))
	}
	return c.JSON(response)
}

func (s Server) UpdateCommerceCategory(c *fiber.Ctx, categoryID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.UpdateCommerceCategoryJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	category, err := s.commerceCatalogueService.UpdateCategory(c.UserContext(), actor, request.OrganizationId, categoryID, services.UpdateCommerceCategoryInput{
		CreateCommerceCategoryInput: services.CreateCommerceCategoryInput{
			Name: request.Name, Slug: request.Slug, Description: optionalString(request.Description), SortOrder: request.SortOrder,
		},
		Status: string(request.Status),
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceCategoryResponse(category))
}

func (s Server) CreateCommerceProduct(c *fiber.Ctx) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.CreateCommerceProductJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	variants := make([]services.CommerceProductVariantInput, 0, len(request.Variants))
	for _, item := range request.Variants {
		attributes := map[string]string{}
		if item.Attributes != nil {
			attributes = *item.Attributes
		}
		variants = append(variants, services.CommerceProductVariantInput{
			Name:       item.Name,
			SKU:        item.Sku,
			PriceMinor: item.PriceMinor,
			Attributes: attributes,
			IsDefault:  item.IsDefault,
		})
	}
	images := make([]services.CommerceProductImageInput, 0)
	if request.Images != nil {
		images = make([]services.CommerceProductImageInput, 0, len(*request.Images))
		for _, item := range *request.Images {
			images = append(images, services.CommerceProductImageInput{
				URL:       item.Url,
				AltText:   optionalString(item.AltText),
				SortOrder: item.SortOrder,
			})
		}
	}
	product, err := s.commerceCatalogueService.CreateProduct(c.UserContext(), actor, request.OrganizationId, services.CreateCommerceProductInput{
		CategoryID:  request.CategoryId,
		Name:        request.Name,
		Slug:        request.Slug,
		Description: optionalString(request.Description),
		Currency:    request.Currency,
		Variants:    variants,
		Images:      images,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.Status(http.StatusCreated).JSON(commerceProductResponse(product))
}

func (s Server) UploadCommerceProductImage(c *fiber.Ctx) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	organizationID, err := optionalCommerceOrganizationID(c.FormValue("organization_id"))
	if err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	fileHeader, err := c.FormFile("file")
	if err != nil || fileHeader.Size < 1 || fileHeader.Size > 5*1024*1024 {
		return commerceError(c, services.ErrCommerceValidation)
	}
	file, err := fileHeader.Open()
	if err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 5*1024*1024+1))
	if err != nil || len(content) > 5*1024*1024 {
		return commerceError(c, services.ErrCommerceValidation)
	}
	result, err := s.commerceCatalogueService.UploadProductImage(c.UserContext(), actor, organizationID, fileHeader.Filename, content)
	if err != nil {
		return commerceError(c, err)
	}
	return c.Status(http.StatusCreated).JSON(api.CommerceProductImageUpload{
		Url: result.URL, PublicId: result.PublicID, Width: result.Width, Height: result.Height, Format: strings.ToLower(result.Format),
	})
}

func optionalCommerceOrganizationID(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (s Server) ListCommerceProducts(c *fiber.Ctx, params api.ListCommerceProductsParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	limit, offset := 50, 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	products, total, err := s.commerceCatalogueService.ListProducts(c.UserContext(), actor, params.OrganizationId, params.CategoryId, limit, offset)
	if err != nil {
		return commerceError(c, err)
	}
	items := make([]api.CommerceProduct, 0, len(products))
	for index := range products {
		items = append(items, commerceProductResponse(&products[index]))
	}
	return c.JSON(api.CommerceProductList{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s Server) GetCommerceProduct(c *fiber.Ctx, productID uuid.UUID, params api.GetCommerceProductParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	product, err := s.commerceCatalogueService.GetProduct(c.UserContext(), actor, params.OrganizationId, productID)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceProductResponse(product))
}

func (s Server) UpdateCommerceProduct(c *fiber.Ctx, productID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.UpdateCommerceProductJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	images := make([]services.CommerceProductImageInput, 0)
	if request.Images != nil {
		images = make([]services.CommerceProductImageInput, 0, len(*request.Images))
		for _, item := range *request.Images {
			images = append(images, services.CommerceProductImageInput{URL: item.Url, AltText: optionalString(item.AltText), SortOrder: item.SortOrder})
		}
	}
	product, err := s.commerceCatalogueService.UpdateProduct(c.UserContext(), actor, request.OrganizationId, productID, services.UpdateCommerceProductInput{
		CategoryID: request.CategoryId, Name: request.Name, Slug: request.Slug,
		Description: optionalString(request.Description), Currency: request.Currency, Status: string(request.Status), Images: images,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceProductResponse(product))
}

func (s Server) UpdateCommerceProductVariant(c *fiber.Ctx, productID, variantID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.UpdateCommerceProductVariantJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	attributes := map[string]string{}
	if request.Attributes != nil {
		attributes = *request.Attributes
	}
	variant, err := s.commerceCatalogueService.UpdateProductVariant(c.UserContext(), actor, request.OrganizationId, productID, variantID, services.UpdateCommerceProductVariantInput{
		Name: request.Name, PriceMinor: request.PriceMinor, Attributes: attributes, Status: string(request.Status),
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceProductVariantResponse(variant))
}

func (s Server) ConfigureCommerceStoreVariant(c *fiber.Ctx, storeID, variantID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.ConfigureCommerceStoreVariantJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	item, level, err := s.commerceCatalogueService.ConfigureStoreVariant(c.UserContext(), actor, request.OrganizationId, storeID, variantID, services.ConfigureCommerceStoreVariantInput{
		Enabled:            request.Enabled,
		PriceOverrideMinor: request.PriceOverrideMinor,
		ReorderThreshold:   request.ReorderThreshold,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(api.CommerceStoreVariantConfiguration{
		Id:                 item.ID,
		OrganizationId:     item.OrganizationID,
		StoreId:            item.StoreID,
		VariantId:          item.VariantID,
		Enabled:            item.Enabled,
		PriceOverrideMinor: item.PriceOverrideMinor,
		Inventory:          commerceInventoryLevelResponse(level),
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	})
}

func (s Server) ListCommerceStoreCatalogue(c *fiber.Ctx, storeID uuid.UUID, params api.ListCommerceStoreCatalogueParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	entries, err := s.commerceCatalogueService.ListStoreCatalogue(c.UserContext(), actor, params.OrganizationId, storeID)
	if err != nil {
		return commerceError(c, err)
	}
	response := make([]api.CommerceStoreCatalogueItem, 0, len(entries))
	for _, entry := range entries {
		response = append(response, commerceStoreCatalogueItemResponse(entry))
	}
	return c.JSON(response)
}

func (s Server) GetCommerceInventoryLevel(c *fiber.Ctx, storeID, variantID uuid.UUID, params api.GetCommerceInventoryLevelParams) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	level, err := s.commerceCatalogueService.GetInventoryLevel(c.UserContext(), actor, params.OrganizationId, storeID, variantID)
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceInventoryLevelResponse(level))
}

func (s Server) AdjustCommerceInventory(c *fiber.Ctx, storeID, variantID uuid.UUID) error {
	actor, err := commerceActor(c)
	if err != nil {
		return commerceError(c, err)
	}
	var request api.AdjustCommerceInventoryJSONRequestBody
	if err := c.BodyParser(&request); err != nil {
		return commerceError(c, services.ErrCommerceValidation)
	}
	level, err := s.commerceCatalogueService.AdjustInventory(c.UserContext(), actor, request.OrganizationId, storeID, variantID, services.AdjustCommerceInventoryInput{
		QuantityDelta: request.QuantityDelta,
		Reference:     request.Reference,
		Reason:        request.Reason,
	})
	if err != nil {
		return commerceError(c, err)
	}
	return c.JSON(commerceInventoryLevelResponse(level))
}

func commerceCategoryResponse(category *models.CommerceCategory) api.CommerceCategory {
	return api.CommerceCategory{
		Id:             category.ID,
		OrganizationId: category.OrganizationID,
		Name:           category.Name,
		Slug:           category.Slug,
		Description:    category.Description,
		SortOrder:      category.SortOrder,
		Status:         api.CommerceCategoryStatus(category.Status),
		CreatedAt:      category.CreatedAt,
		UpdatedAt:      category.UpdatedAt,
	}
}

func commerceProductResponse(product *models.CommerceProduct) api.CommerceProduct {
	variants := make([]api.CommerceProductVariant, 0, len(product.Variants))
	for _, item := range product.Variants {
		variants = append(variants, commerceProductVariantResponse(&item))
	}
	images := make([]api.CommerceProductImage, 0, len(product.Images))
	for _, item := range product.Images {
		images = append(images, api.CommerceProductImage{
			Id:        item.ID,
			Url:       item.URL,
			AltText:   item.AltText,
			SortOrder: item.SortOrder,
		})
	}
	return api.CommerceProduct{
		Id:             product.ID,
		OrganizationId: product.OrganizationID,
		CategoryId:     product.CategoryID,
		Name:           product.Name,
		Slug:           product.Slug,
		Description:    product.Description,
		Currency:       product.Currency,
		Status:         api.CommerceProductStatus(product.Status),
		Variants:       variants,
		Images:         images,
		CreatedAt:      product.CreatedAt,
		UpdatedAt:      product.UpdatedAt,
	}
}

func commerceProductVariantResponse(item *models.CommerceProductVariant) api.CommerceProductVariant {
	attributes := map[string]string{}
	_ = json.Unmarshal(item.Attributes, &attributes)
	return api.CommerceProductVariant{
		Id: item.ID, ProductId: item.ProductID, Name: item.Name, Sku: item.SKU, PriceMinor: item.PriceMinor,
		Attributes: attributes, IsDefault: item.IsDefault, Status: api.CommerceProductVariantStatus(item.Status),
	}
}

func commerceStoreCatalogueItemResponse(entry repository.CommerceStoreCatalogueEntry) api.CommerceStoreCatalogueItem {
	attributes := map[string]string{}
	_ = json.Unmarshal(entry.Attributes, &attributes)
	return api.CommerceStoreCatalogueItem{
		StoreId:             entry.StoreID,
		CategoryId:          entry.CategoryID,
		CategoryName:        entry.CategoryName,
		ProductId:           entry.ProductID,
		ProductName:         entry.ProductName,
		ProductDescription:  entry.ProductDescription,
		Currency:            entry.ProductCurrency,
		VariantId:           entry.VariantID,
		VariantName:         entry.VariantName,
		Sku:                 entry.SKU,
		Attributes:          attributes,
		BasePriceMinor:      entry.BasePriceMinor,
		PriceOverrideMinor:  entry.PriceOverrideMinor,
		EffectivePriceMinor: entry.EffectivePriceMinor,
		PrimaryImageUrl:     entry.PrimaryImageURL,
		Enabled:             entry.Enabled,
		QuantityOnHand:      entry.QuantityOnHand,
		QuantityReserved:    entry.QuantityReserved,
		AvailableQuantity:   entry.AvailableQuantity,
		ReorderThreshold:    entry.ReorderThreshold,
		Available:           entry.Enabled && entry.AvailableQuantity > 0,
	}
}

func commerceInventoryLevelResponse(level *models.CommerceInventoryLevel) api.CommerceInventoryLevel {
	return api.CommerceInventoryLevel{
		Id:                level.ID,
		OrganizationId:    level.OrganizationID,
		StoreId:           level.StoreID,
		VariantId:         level.VariantID,
		QuantityOnHand:    level.QuantityOnHand,
		QuantityReserved:  level.QuantityReserved,
		AvailableQuantity: level.AvailableQuantity(),
		ReorderThreshold:  level.ReorderThreshold,
		Version:           level.Version,
		CreatedAt:         level.CreatedAt,
		UpdatedAt:         level.UpdatedAt,
	}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
