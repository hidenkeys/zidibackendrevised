package middleware

import (
	"fmt"
	"github.com/hidenkeys/zidibackend/utils"
	"gorm.io/gorm"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

// UserClaims represents extracted claims from the JWT
type UserClaims struct {
	ID             string `json:"user_id"`
	Role           string `json:"role"`
	OrganizationID string `json:"organization_id"`
}

// Claims structure for JWT parsing
type Claims struct {
	UserID         string `json:"user_id"`
	Role           string `json:"role"`
	OrganizationID string `json:"organization_id"`
	jwt.RegisteredClaims
}

// AllowedRoles defines the roles permitted to access routes
var AllowedRoles = map[string]bool{
	"zidi":  true,
	"admin": true,
	"user":  true,
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

		// Check if the token is revoked
		revoked, err := utils.IsTokenRevoked(db, tokenString)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Error checking token status"})
		}
		if revoked {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Token has been revoked"})
		}

		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secretKey), nil
		})

		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token"})
		}

		claims, ok := token.Claims.(*Claims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token claims"})
		}

		if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Token has expired"})
		}

		if !AllowedRoles[claims.Role] {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Unauthorized role"})
		}

		if len(allowedRoles) > 0 {
			roleAllowed := false
			for _, role := range allowedRoles {
				if claims.Role == role {
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
			Role:           claims.Role,
			OrganizationID: claims.OrganizationID,
		})

		return c.Next()
	}
}
