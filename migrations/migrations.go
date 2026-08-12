package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

//go:embed sql/*.sql
var migrationFiles embed.FS

type migration struct {
	Version  int64
	Name     string
	SQL      string
	Checksum string
}

type appliedMigration struct {
	Version  int64  `gorm:"column:version"`
	Checksum string `gorm:"column:checksum"`
}

func Run(ctx context.Context, db *gorm.DB) error {
	migrations, err := load()
	if err != nil {
		return err
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS zidi_schema_migrations (
				version BIGINT PRIMARY KEY,
				name TEXT NOT NULL,
				checksum TEXT NOT NULL,
				applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)
		`).Error; err != nil {
			return fmt.Errorf("create migration table: %w", err)
		}

		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "zidi_schema_migrations").Error; err != nil {
			return fmt.Errorf("acquire migration lock: %w", err)
		}

		var applied []appliedMigration
		if err := tx.Table("zidi_schema_migrations").Find(&applied).Error; err != nil {
			return fmt.Errorf("read applied migrations: %w", err)
		}
		checksums := make(map[int64]string, len(applied))
		for _, item := range applied {
			checksums[item.Version] = item.Checksum
		}

		for _, item := range migrations {
			if checksum, ok := checksums[item.Version]; ok {
				if checksum != item.Checksum {
					return fmt.Errorf("migration %d checksum changed after it was applied", item.Version)
				}
				continue
			}

			if err := tx.Exec(item.SQL).Error; err != nil {
				return fmt.Errorf("apply migration %d (%s): %w", item.Version, item.Name, err)
			}
			if err := tx.Exec(
				"INSERT INTO zidi_schema_migrations (version, name, checksum) VALUES (?, ?, ?)",
				item.Version,
				item.Name,
				item.Checksum,
			).Error; err != nil {
				return fmt.Errorf("record migration %d: %w", item.Version, err)
			}
		}

		return nil
	})
}

func load() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "sql")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	items := make([]migration, 0, len(entries))
	versions := make(map[int64]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 || strings.TrimSuffix(parts[1], ".sql") == "" {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || version < 1 {
			return nil, fmt.Errorf("invalid migration version in %q", entry.Name())
		}
		if _, exists := versions[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %d", version)
		}

		contents, err := migrationFiles.ReadFile("sql/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		if strings.TrimSpace(string(contents)) == "" {
			return nil, fmt.Errorf("migration %q is empty", entry.Name())
		}
		digest := sha256.Sum256(contents)
		items = append(items, migration{
			Version:  version,
			Name:     strings.TrimSuffix(parts[1], ".sql"),
			SQL:      string(contents),
			Checksum: hex.EncodeToString(digest[:]),
		})
		versions[version] = struct{}{}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Version < items[j].Version })
	return items, nil
}
