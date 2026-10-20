package migrations

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// PrepareLegacySchema normalizes columns created by early GORM inference before
// the versioned commerce migrations add tenant-aware foreign keys.
func PrepareLegacySchema(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}

	var dataType string
	result := db.WithContext(ctx).Raw(`
		SELECT data_type
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'users'
		  AND column_name = 'organization_id'
	`).Scan(&dataType)
	if result.Error != nil {
		return fmt.Errorf("inspect users.organization_id: %w", result.Error)
	}
	if result.RowsAffected == 0 || dataType == "uuid" {
		return nil
	}
	var conversion string
	switch dataType {
	case "text", "character varying":
		conversion = "organization_id::text::uuid"
	case "bytea":
		conversion = `CASE
			WHEN organization_id IS NULL THEN NULL
			WHEN octet_length(organization_id) = 16 THEN encode(organization_id, 'hex')::uuid
			ELSE convert_from(organization_id, 'UTF8')::uuid
		END`
	default:
		return fmt.Errorf("users.organization_id has unsupported database type %q", dataType)
	}

	statement := "ALTER TABLE users ALTER COLUMN organization_id TYPE uuid USING (" + conversion + ")"
	if err := db.WithContext(ctx).Exec(statement).Error; err != nil {
		return fmt.Errorf("convert users.organization_id to uuid: %w", err)
	}
	return nil
}
