package utils

import (
	"errors"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

func TestGenerateJWTTokenUsesCanonicalClaims(t *testing.T) {
	secret := strings.Repeat("s", 32)
	t.Setenv("JWT_SECRET", secret)
	userID := uuid.NewString()
	organizationID := uuid.NewString()

	signed, err := GenerateJWTToken(userID, organizationID, RoleMerchantAdmin)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	parsed, err := jwt.ParseWithClaims(signed, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("parse generated token: %v", err)
	}
	claims := parsed.Claims.(*Claims)
	if claims.UserID != userID || claims.OrganizationID != organizationID || claims.Role != RoleMerchantAdmin {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.ID == "" || claims.ExpiresAt == nil || claims.Issuer != "zidi" {
		t.Fatalf("missing registered claims: %+v", claims.RegisteredClaims)
	}
}

func TestLoadJWTSecretRejectsMissingOrShortSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "too-short")
	_, err := LoadJWTSecret()
	if !errors.Is(err, ErrJWTSecretMissing) {
		t.Fatalf("expected missing secret error, got %v", err)
	}
}
