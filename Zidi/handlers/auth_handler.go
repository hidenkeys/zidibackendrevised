package handlers

import (
	"context"
	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/middleware"
	"github.com/hidenkeys/zidibackend/utils"
	"golang.org/x/crypto/bcrypt"
	"net/http"
)

func (s Server) LoginUser(c *fiber.Ctx) error {
	var req api.LoginUserJSONRequestBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	if req.Email == "" || req.Password == "" {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Email or Password is required",
		})
	}

	user, err := s.usrService.GetUserByEmail(context.Background(), string(req.Email))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return c.Status(http.StatusUnauthorized).JSON(api.Error{
			ErrorCode: "401",
			Message:   "Invalid credentials",
		})
	}

	token, err := utils.GenerateJWTToken(user.Id.String(), user.OrganizationId.String(), user.Role)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	user.Password = ""

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"token":   token,
		"user":    user,
	})
}

func (s Server) LogoutUser(c *fiber.Ctx) error {
	claims, ok := middleware.CurrentUser(c)
	if !ok || claims.TokenID == "" || claims.ExpiresAt.IsZero() {
		return c.Status(http.StatusUnauthorized).JSON(api.Error{
			ErrorCode: "401",
			Message:   "Invalid token claims",
		})
	}

	if err := utils.RevokeToken(s.db, claims.TokenID, claims.ExpiresAt); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   "Could not log out",
		})
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "Successfully logged out",
	})
}

func (s Server) SuperuserLogin(c *fiber.Ctx) error {
	var req api.LoginUserJSONRequestBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}
	if req.Email == "" || req.Password == "" {
		return c.Status(http.StatusBadRequest).JSON(api.Error{
			ErrorCode: "400",
			Message:   "Email or Password is required",
		})
	}

	user, err := s.usrService.GetUserByEmail(context.Background(), string(req.Email))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	if !utils.IsPlatformRole(user.Role) {
		return c.Status(http.StatusUnauthorized).JSON(api.Error{
			ErrorCode: "401",
			Message:   "Insufficient privileges",
		})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return c.Status(http.StatusUnauthorized).JSON(api.Error{
			ErrorCode: "401",
			Message:   "Invalid credentials",
		})
	}

	token, err := utils.GenerateJWTToken(user.Id.String(), user.OrganizationId.String(), user.Role)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(api.Error{
			ErrorCode: "500",
			Message:   err.Error(),
		})
	}

	user.Password = ""

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"token":   token,
		"user":    user,
	})
}
