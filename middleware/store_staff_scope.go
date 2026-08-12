package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidibackend/utils"
)

func StoreStaffScope() fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, ok := CurrentUser(c)
		if !ok || !strings.EqualFold(claims.Role, utils.RoleStoreStaff) {
			return c.Next()
		}
		if storeStaffRouteAllowed(c.Method(), c.Path()) {
			return c.Next()
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Store staff can only access assigned store orders"})
	}
}

func storeStaffRouteAllowed(method, path string) bool {
	if method == fiber.MethodPost && path == "/api/v1/auth/logout" {
		return true
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if method == fiber.MethodGet && len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "commerce" && parts[3] == "store-orders" {
		return true
	}
	if method != fiber.MethodPost || len(parts) < 6 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "commerce" {
		return false
	}
	if parts[3] == "store-orders" && len(parts) == 6 && parts[5] == "prepared" {
		return true
	}
	if parts[3] != "fulfilments" {
		return false
	}
	action := strings.Join(parts[5:], "/")
	switch action {
	case "quotes", "rider-assignments", "handover", "handover-code/reminders", "arrival", "delivered", "delivery-confirmation-requests", "complete":
		return true
	default:
		return false
	}
}
