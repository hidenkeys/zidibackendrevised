package handlers

import (
	"bytes"
	"context"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/middleware"
	"github.com/hidenkeys/zidibackend/utils"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"golang.org/x/crypto/bcrypt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"time"
)

type createOrgaization struct {
	Name     string
	Email    string
	Password string
}

const (
	letterBytes    = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	numberBytes    = "0123456789"
	symbolBytes    = "!@#$%^&*()-_=+[]{}|;:,.<>?/`~"
	allChars       = letterBytes + numberBytes + symbolBytes
	passwordLength = 16
)

// GeneratePassword generates a 16-character password with letters, numbers, and symbols.
func GeneratePassword() string {
	rand.Seed(time.Now().UnixNano())

	// Ensure at least one character from each category
	password := []byte{
		letterBytes[rand.Intn(len(letterBytes))], // At least one letter
		numberBytes[rand.Intn(len(numberBytes))], // At least one number
		symbolBytes[rand.Intn(len(symbolBytes))], // At least one symbol
	}

	// Fill the remaining length randomly
	for i := len(password); i < passwordLength; i++ {
		password = append(password, allChars[rand.Intn(len(allChars))])
	}

	// Shuffle the password to avoid predictable patterns
	rand.Shuffle(len(password), func(i, j int) {
		password[i], password[j] = password[j], password[i]
	})

	return string(password)
}

func (s Server) CreateOrganization(c *fiber.Ctx) error {
	userClaims, ok := c.Locals("user").(middleware.UserClaims)
	if !ok {
		return c.Status(http.StatusUnauthorized).JSON(api.Error{
			ErrorCode: "401",
			Message:   "Unauthorized - Invalid token",
		})
	}

	if userClaims.Role != "zidi" {
		return c.Status(http.StatusUnauthorized).JSON(api.Error{
			ErrorCode: "401",
			Message:   "Unauthorized - Invalid token",
		})
	}
	organization := new(api.Organization)

	if err := c.BodyParser(organization); err != nil {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid request body",
		})
	}

	// Create the organization
	orgResponse, err := s.orgService.CreateOrganization(context.Background(), *organization)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	password := GeneratePassword()
	// Create an admin user for the organization
	defaultUser := api.User{
		Firstname:      "Admin",
		Lastname:       "User",
		Email:          organization.Email, // Assuming organization email is provided
		Password:       password,           // Default password (should be changed later)
		OrganizationId: orgResponse.Id,     // Assign newly created org ID
		Role:           "admin",
	}

	// Hash the password before storing it
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultUser.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   "Failed to encrypt password",
		})
	}
	defaultUser.Password = string(hashedPassword)

	userResponse, err := s.usrService.CreateUser(context.Background(), defaultUser)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	tmp := createOrgaization{
		Name:     organization.CompanyName,
		Email:    string(defaultUser.Email),
		Password: password,
	}

	tmpl, err := template.ParseFiles("zidi-onboarding-email.html")
	if err != nil {
		log.Fatalf("Error loading template: %v", err)
	}

	// Parse the template with the receipt data
	var tpl bytes.Buffer
	if err := tmpl.Execute(&tpl, tmp); err != nil {
		log.Fatalf("Error executing template: %v", err)
	}

	// Convert parsed template to a string
	createBody := tpl.String()

	err = utils.SendEmail00(string(defaultUser.Email), "Welcome to Zidi", createBody)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	fmt.Println(userResponse.Id.String(), orgResponse.Id.String())

	// Generate a JWT Token
	token, err := utils.GenerateJWTToken(userResponse.Id.String(), orgResponse.Id.String(), userResponse.Role)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   "Failed to generate authentication token",
		})
	}

	// Return response with token
	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"organization": orgResponse,
		"user":         userResponse,
		"token":        token,
	})
}

func (s Server) GetOrganizations(c *fiber.Ctx, params api.GetOrganizationsParams) error {
	limit := 10
	offset := 0

	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	response, err := s.orgService.GetAllOrganizations(context.Background(), limit, offset)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(response)
}

func (s Server) DeleteOrganization(c *fiber.Ctx, organizationId openapi_types.UUID) error {

	//userClaims, ok := c.Locals("user").(middleware.UserClaims)
	//if !ok {
	//	return c.Status(http.StatusUnauthorized).JSON(api.Error{
	//		ErrorCode: "401",
	//		Message:   "Unauthorized - Invalid token",
	//	})
	//}
	//
	//organizationUUID, err := uuid.Parse(userClaims.OrganizationID)
	//if err != nil {
	//	return c.Status(http.StatusBadRequest).JSON(api.Error{
	//		ErrorCode: "400",
	//		Message:   "Invalid organization ID format",
	//	})
	//}

	err := s.orgService.DeleteOrganization(context.Background(), organizationId)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusNoContent).JSON(nil)
}

func (s Server) GetOrganizationById(c *fiber.Ctx, organizationId openapi_types.UUID) error {
	response, err := s.orgService.GetOrganizationByID(context.Background(), organizationId)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(response)
}

func (s Server) UpdateOrganization(c *fiber.Ctx, organizationId openapi_types.UUID) error {
	//userClaims, ok := c.Locals("user").(middleware.UserClaims)
	//if !ok {
	//	return c.Status(http.StatusUnauthorized).JSON(api.Error{
	//		ErrorCode: "401",
	//		Message:   "Unauthorized - Invalid token",
	//	})
	//}
	//
	//organizationUUID, err := uuid.Parse(userClaims.OrganizationID)
	//if err != nil {
	//	return c.Status(http.StatusBadRequest).JSON(api.Error{
	//		ErrorCode: "400",
	//		Message:   "Invalid organization ID format",
	//	})
	//}
	organization := new(api.Organization)
	if err := c.BodyParser(organization); err != nil {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid request body",
		})
	}
	response, err := s.orgService.UpdateOrganization(context.Background(), organizationId, *organization)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(response)
}

func (s Server) GetOrganizationByName(c *fiber.Ctx, params api.GetOrganizationByNameParams) error {
	limit := 10
	offset := 0

	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	response, err := s.orgService.GetOrganizationByName(context.Background(), params.Name, limit, offset)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(response)
}

// SeedDefaultOrganization ensures a default "Zidi" organization exists
func (s Server) SeedDefaultOrganization() {
	orgName := "Zidi"
	defaultEmail := "admin@zidi.com"

	// Check if organization already exists
	existingOrgs, err := s.orgService.GetOrganizationByName(context.Background(), orgName, 1, 0)
	if err != nil {
		log.Println("Error checking existing organizations:", err)
		return
	}

	if len(existingOrgs) > 0 {
		log.Println("Zidi organization already exists, skipping seed.")
		return
	}

	// Create the organization
	newOrg := api.Organization{
		CompanyName: orgName,
		Email:       openapi_types.Email(defaultEmail),
	}
	orgResponse, err := s.orgService.CreateOrganization(context.Background(), newOrg)
	if err != nil {
		log.Println("Error creating Zidi organization:", err)
		return
	}

	// Create default admin user for the organization
	defaultUser := api.User{
		Firstname:      "Admin",
		Lastname:       "User",
		Email:          openapi_types.Email(defaultEmail),
		Password:       "ChangeMe123!",
		OrganizationId: orgResponse.Id,
		Role:           "zidi", // Role set to "zidi"
	}

	// Hash password before saving
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultUser.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Println("Error encrypting password:", err)
		return
	}
	defaultUser.Password = string(hashedPassword)

	_, err = s.usrService.CreateUser(context.Background(), defaultUser)
	if err != nil {
		log.Println("Error creating admin user for Zidi:", err)
		return
	}

	log.Println("Successfully seeded Zidi organization and default admin user.")
}
