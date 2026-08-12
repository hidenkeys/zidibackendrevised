CREATE TABLE IF NOT EXISTS commerce_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_categories_org_slug
    ON commerce_categories (organization_id, LOWER(slug)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_categories_id_organization
    ON commerce_categories (id, organization_id);
CREATE INDEX IF NOT EXISTS idx_commerce_categories_org_status_sort
    ON commerce_categories (organization_id, status, sort_order) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS commerce_products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    category_id UUID NOT NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    currency CHAR(3) NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_commerce_products_category_tenant
        FOREIGN KEY (category_id, organization_id)
        REFERENCES commerce_categories(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_products_org_slug
    ON commerce_products (organization_id, LOWER(slug)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_products_id_organization
    ON commerce_products (id, organization_id);
CREATE INDEX IF NOT EXISTS idx_commerce_products_org_category_status
    ON commerce_products (organization_id, category_id, status) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS commerce_product_variants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    product_id UUID NOT NULL,
    name TEXT NOT NULL,
    sku TEXT NOT NULL,
    price_minor BIGINT NOT NULL CHECK (price_minor >= 0),
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(attributes) = 'object'),
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_commerce_variants_product_tenant
        FOREIGN KEY (product_id, organization_id)
        REFERENCES commerce_products(id, organization_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_variants_org_sku
    ON commerce_product_variants (organization_id, LOWER(sku)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_variants_id_organization
    ON commerce_product_variants (id, organization_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_variants_default
    ON commerce_product_variants (organization_id, product_id) WHERE is_default = TRUE AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_commerce_variants_org_product_status
    ON commerce_product_variants (organization_id, product_id, status) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS commerce_product_images (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    product_id UUID NOT NULL,
    url TEXT NOT NULL,
    alt_text TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_commerce_images_product_tenant
        FOREIGN KEY (product_id, organization_id)
        REFERENCES commerce_products(id, organization_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_product_images_url
    ON commerce_product_images (organization_id, product_id, url) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_commerce_product_images_sort
    ON commerce_product_images (organization_id, product_id, sort_order) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS commerce_store_catalogue_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    store_id UUID NOT NULL,
    variant_id UUID NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    price_override_minor BIGINT CHECK (price_override_minor IS NULL OR price_override_minor >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_commerce_catalogue_store_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE CASCADE,
    CONSTRAINT fk_commerce_catalogue_variant_tenant
        FOREIGN KEY (variant_id, organization_id)
        REFERENCES commerce_product_variants(id, organization_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_store_catalogue_variant
    ON commerce_store_catalogue_items (organization_id, store_id, variant_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_commerce_store_catalogue_enabled
    ON commerce_store_catalogue_items (organization_id, store_id, enabled) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS commerce_inventory_levels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    store_id UUID NOT NULL,
    variant_id UUID NOT NULL,
    quantity_on_hand INTEGER NOT NULL DEFAULT 0,
    quantity_reserved INTEGER NOT NULL DEFAULT 0,
    reorder_threshold INTEGER NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_commerce_inventory_quantities CHECK (
        quantity_on_hand >= 0
        AND quantity_reserved >= 0
        AND quantity_reserved <= quantity_on_hand
        AND reorder_threshold >= 0
    ),
    CONSTRAINT fk_commerce_inventory_store_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE CASCADE,
    CONSTRAINT fk_commerce_inventory_variant_tenant
        FOREIGN KEY (variant_id, organization_id)
        REFERENCES commerce_product_variants(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_inventory_level
    ON commerce_inventory_levels (organization_id, store_id, variant_id);
CREATE INDEX IF NOT EXISTS idx_commerce_inventory_store_available
    ON commerce_inventory_levels (organization_id, store_id, quantity_on_hand, quantity_reserved);

CREATE TABLE IF NOT EXISTS commerce_inventory_reservations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    store_id UUID NOT NULL,
    variant_id UUID NOT NULL,
    reservation_key TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    status TEXT NOT NULL CHECK (status IN ('active', 'committed', 'released', 'expired')),
    expires_at TIMESTAMPTZ NOT NULL,
    committed_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_commerce_reservation_expiry CHECK (expires_at > created_at),
    CONSTRAINT fk_commerce_reservation_store_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE CASCADE,
    CONSTRAINT fk_commerce_reservation_variant_tenant
        FOREIGN KEY (variant_id, organization_id)
        REFERENCES commerce_product_variants(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_inventory_reservation_key
    ON commerce_inventory_reservations (organization_id, reservation_key);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_inventory_reservation_id_organization
    ON commerce_inventory_reservations (id, organization_id);
CREATE INDEX IF NOT EXISTS idx_commerce_inventory_reservation_expiry
    ON commerce_inventory_reservations (organization_id, status, expires_at);

CREATE TABLE IF NOT EXISTS commerce_inventory_movements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    store_id UUID NOT NULL,
    variant_id UUID NOT NULL,
    reservation_id UUID,
    movement_type TEXT NOT NULL CHECK (movement_type IN ('adjustment', 'reservation', 'reservation_commit', 'reservation_release')),
    quantity_on_hand_delta INTEGER NOT NULL DEFAULT 0,
    quantity_reserved_delta INTEGER NOT NULL DEFAULT 0,
    reference TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_by_user_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_commerce_inventory_movement_delta CHECK (
        quantity_on_hand_delta <> 0 OR quantity_reserved_delta <> 0
    ),
    CONSTRAINT fk_commerce_movement_store_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE CASCADE,
    CONSTRAINT fk_commerce_movement_variant_tenant
        FOREIGN KEY (variant_id, organization_id)
        REFERENCES commerce_product_variants(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_movement_reservation_tenant
        FOREIGN KEY (reservation_id, organization_id)
        REFERENCES commerce_inventory_reservations(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_movement_actor
        FOREIGN KEY (created_by_user_id)
        REFERENCES users(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_inventory_movement_reference
    ON commerce_inventory_movements (organization_id, reference);
CREATE INDEX IF NOT EXISTS idx_commerce_inventory_movement_lookup
    ON commerce_inventory_movements (organization_id, store_id, variant_id, created_at DESC);
