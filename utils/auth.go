package utils

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"gorm.io/gorm"
)

const (
	RolePlatformAdmin       = "platform_admin"
	RoleLegacyPlatformAdmin = "zidi"
	RoleMerchantAdmin       = "merchant_admin"
	RoleLegacyMerchantAdmin = "admin"
	RoleStoreManager        = "store_manager"
	RoleStoreStaff          = "store_staff"
	RoleUser                = "user"

	jwtIssuer = "zidi"
)

var ErrJWTSecretMissing = errors.New("JWT_SECRET must be set to at least 32 characters")

type Claims struct {
	UserID         string `json:"user_id"`
	Role           string `json:"role"`
	OrganizationID string `json:"organization_id"`
	jwt.RegisteredClaims
}

func LoadJWTSecret() ([]byte, error) {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if len(secret) < 32 {
		return nil, ErrJWTSecretMissing
	}
	return []byte(secret), nil
}

func GenerateJWTToken(userID, orgID, role string) (string, error) {
	secret, err := LoadJWTSecret()
	if err != nil {
		return "", err
	}
	if _, err := uuid.Parse(userID); err != nil {
		return "", fmt.Errorf("invalid user id: %w", err)
	}
	if _, err := uuid.Parse(orgID); err != nil {
		return "", fmt.Errorf("invalid organization id: %w", err)
	}

	now := time.Now().UTC()
	claims := Claims{
		UserID:         userID,
		OrganizationID: orgID,
		Role:           strings.ToLower(strings.TrimSpace(role)),
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			Issuer:    jwtIssuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(72 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func RevokeToken(db *gorm.DB, tokenID string, expiry time.Time) error {
	if strings.TrimSpace(tokenID) == "" {
		return errors.New("token id is required")
	}
	return db.Create(&models.Token{Token: tokenID, ExpiresAt: expiry}).Error
}

func IsTokenRevoked(db *gorm.DB, tokenID string) (bool, error) {
	if tokenID == "" {
		return false, nil
	}
	var count int64
	err := db.Model(&models.Token{}).Where("token = ?", tokenID).Count(&count).Error
	return count > 0, err
}

func IsPlatformRole(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	return role == RolePlatformAdmin || role == RoleLegacyPlatformAdmin
}

func IsMerchantAdminRole(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	return role == RoleMerchantAdmin || role == RoleLegacyMerchantAdmin
}

func IsStoreRole(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	return role == RoleStoreManager || role == RoleStoreStaff
}

func IsKnownRole(role string) bool {
	return IsPlatformRole(role) || IsMerchantAdminRole(role) || IsStoreRole(role) || strings.EqualFold(role, RoleUser)
}
