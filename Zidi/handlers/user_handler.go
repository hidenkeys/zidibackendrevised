package handlers

import (
	"context"
	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidibackend/api"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"golang.org/x/crypto/bcrypt"
	"net/http"
)

func (s Server) GetUsersByOrganization(c *fiber.Ctx, organizationId openapi_types.UUID, params api.GetUsersByOrganizationParams) error {
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
	limit := 10
	offset := 0

	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	response, err := s.usrService.GetUserByOrganizationID(context.Background(), organizationId, limit, offset)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusCreated).JSON(response)
}

func (s Server) GetUserByEmail(c *fiber.Ctx, params api.GetUserByEmailParams) error {
	response, err := s.usrService.GetUserByEmail(context.Background(), string(params.Email))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(response)
}

func (s Server) GetUsers(c *fiber.Ctx, params api.GetUsersParams) error {
	limit := 10
	offset := 0

	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	response, err := s.usrService.GetAllUsers(context.Background(), limit, offset)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(response)
}

func (s Server) CreateUser(c *fiber.Ctx) error {
	var reqBody api.CreateUserRequestBody

	// Parse request body
	if err := c.BodyParser(&reqBody); err != nil {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid request body",
		})
	}

	// Validate passwords match
	if reqBody.Password != reqBody.ConfirmPassword {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Passwords do not match",
		})
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(reqBody.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   "Failed to hash password",
		})
	}

	// Create user struct with hashed password
	newUser := api.User{
		Firstname:      reqBody.FirstName,
		Lastname:       reqBody.LastName,
		Email:          reqBody.Email,
		Address:        reqBody.Address,
		Role:           reqBody.Role,
		OrganizationId: openapi_types.UUID(reqBody.OrganizationId),
		Password:       string(hashedPassword), // Save hashed password
	}

	_, err = s.orgService.GetOrganizationByID(context.Background(), newUser.OrganizationId)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   "organization not found",
		})
	}

	// Call user service to save user
	response, err := s.usrService.CreateUser(context.Background(), newUser)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	return c.Status(http.StatusCreated).JSON(response)

}

func (s Server) DeleteUser(c *fiber.Ctx, userId openapi_types.UUID) error {
	err := s.usrService.DeleteUser(context.Background(), userId)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusNoContent).JSON(fiber.Map{
		"message": "User deleted",
	})
}

func (s Server) GetUserById(c *fiber.Ctx, userId openapi_types.UUID) error {
	response, err := s.usrService.GetUserByID(context.Background(), userId)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(response)
}

func (s Server) UpdateUser(c *fiber.Ctx, userId openapi_types.UUID) error {
	user := new(api.User)
	if err := c.BodyParser(user); err != nil {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid request body",
		})
	}
	response, err := s.usrService.UpdateUser(context.Background(), userId, *user)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	return c.Status(http.StatusOK).JSON(response)
}

func (s Server) UpdateUserPassword(c *fiber.Ctx, userId openapi_types.UUID) error {
	password := new(api.UpdateUserPasswordJSONRequestBody)

	// Parse request body
	if err := c.BodyParser(password); err != nil {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Invalid request body",
		})
	}

	// Validate request fields
	if password.OldPassword == "" || password.NewPassword == "" {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Both old and new passwords are required",
		})
	}

	// Fetch user from the database
	user, err := s.usrService.GetUserByID(context.Background(), userId)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(api.Error{
			ErrorCode: "404",
			Message:   "User not found",
		})
	}

	// Compare old password with stored hash
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password.OldPassword)); err != nil {
		return c.Status(http.StatusUnauthorized).JSON(api.Error{
			ErrorCode: "401",
			Message:   "Incorrect old password",
		})
	}

	// Hash the new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   "Failed to hash new password",
		})
	}

	// Update password in database
	err = s.usrService.UpdatePassword(context.Background(), userId, string(hashedPassword))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   "Failed to update password",
		})
	}

	// Return success response
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "Password updated successfully",
	})
}
