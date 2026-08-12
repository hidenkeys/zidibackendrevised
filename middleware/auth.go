package middleware

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/utils"
	"gorm.io/gorm"
)

// UserClaims represents extracted claims from the JWT
type UserClaims struct {
	ID             string `json:"user_id"`
	Role           string `json:"role"`
	OrganizationID string `json:"organization_id"`
	TokenID        string `json:"token_id"`
	ExpiresAt      time.Time
}

// AllowedRoles defines the roles permitted to access routes
var AllowedRoles = map[string]bool{
	utils.RolePlatformAdmin:       true,
	utils.RoleLegacyPlatformAdmin: true,
	utils.RoleMerchantAdmin:       true,
	utils.RoleLegacyMerchantAdmin: true,
	utils.RoleStoreManager:        true,
	utils.RoleStoreStaff:          true,
	utils.RoleUser:                true,
}

func AuthMiddleware(db *gorm.DB, secretKey string, allowedRoles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing token"})
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token format"})
		}

		token, err := jwt.ParseWithClaims(tokenString, &utils.Claims{}, func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secretKey), nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))

		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token"})
		}

		claims, ok := token.Claims.(*utils.Claims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token claims"})
		}

		if claims.Issuer != "zidi" || claims.ID == "" || claims.UserID == "" || claims.OrganizationID == "" || claims.ExpiresAt == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token claims"})
		}
		if _, err := uuid.Parse(claims.UserID); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token claims"})
		}
		if _, err := uuid.Parse(claims.OrganizationID); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token claims"})
		}

		revoked, err := utils.IsTokenRevoked(db, claims.ID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Error checking token status"})
		}
		if revoked {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Token has been revoked"})
		}

		role := strings.ToLower(strings.TrimSpace(claims.Role))
		if !AllowedRoles[role] {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Unauthorized role"})
		}

		if len(allowedRoles) > 0 {
			roleAllowed := false
			for _, allowedRole := range allowedRoles {
				if strings.EqualFold(role, allowedRole) {
					roleAllowed = true
					break
				}
			}
			if !roleAllowed {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Access denied"})
			}
		}

		c.Locals("user", UserClaims{
			ID:             claims.UserID,
			Role:           role,
			OrganizationID: claims.OrganizationID,
			TokenID:        claims.ID,
			ExpiresAt:      claims.ExpiresAt.Time,
		})

		return c.Next()
	}
}

func CurrentUser(c *fiber.Ctx) (UserClaims, bool) {
	claims, ok := c.Locals("user").(UserClaims)
	return claims, ok
}
