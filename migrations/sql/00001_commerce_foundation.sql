CREATE TABLE IF NOT EXISTS commerce_merchant_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    slug TEXT NOT NULL,
    display_name TEXT NOT NULL,
    default_currency CHAR(3) NOT NULL,
    timezone TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_merchant_profiles_organization
    ON commerce_merchant_profiles (organization_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_merchant_profiles_slug
    ON commerce_merchant_profiles (LOWER(slug)) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS commerce_stores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    address TEXT NOT NULL,
    city TEXT NOT NULL,
    state TEXT NOT NULL,
    country_code CHAR(2) NOT NULL,
    latitude NUMERIC(10, 7),
    longitude NUMERIC(10, 7),
    timezone TEXT NOT NULL,
    preparation_minutes INTEGER NOT NULL DEFAULT 15 CHECK (preparation_minutes BETWEEN 0 AND 1440),
    status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chk_commerce_store_latitude CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),
    CONSTRAINT chk_commerce_store_longitude CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_stores_org_code
    ON commerce_stores (organization_id, LOWER(code)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_stores_id_organization
    ON commerce_stores (id, organization_id);
CREATE INDEX IF NOT EXISTS idx_commerce_stores_org_status
    ON commerce_stores (organization_id, status) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS commerce_store_hours (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    store_id UUID NOT NULL,
    day_of_week SMALLINT NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    open_minute SMALLINT CHECK (open_minute BETWEEN 0 AND 1439),
    close_minute SMALLINT CHECK (close_minute BETWEEN 1 AND 1440),
    is_closed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chk_commerce_store_hours_range CHECK (
        (is_closed = TRUE AND open_minute IS NULL AND close_minute IS NULL)
        OR
        (is_closed = FALSE AND open_minute IS NOT NULL AND close_minute IS NOT NULL AND close_minute > open_minute)
    ),
    CONSTRAINT fk_commerce_store_hours_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_store_hours_day
    ON commerce_store_hours (organization_id, store_id, day_of_week) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_commerce_store_hours_lookup
    ON commerce_store_hours (organization_id, store_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS commerce_store_fulfilment_modes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    store_id UUID NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('customer_pickup', 'customer_rider', 'merchant_rider')),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_commerce_store_fulfilment_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_store_fulfilment_mode
    ON commerce_store_fulfilment_modes (organization_id, store_id, mode) WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_id_organization
    ON users (id, organization_id);

CREATE TABLE IF NOT EXISTS commerce_staff_store_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    store_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('store_manager', 'store_staff')),
    status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_commerce_staff_store_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE CASCADE,
    CONSTRAINT fk_commerce_staff_user_tenant
        FOREIGN KEY (user_id, organization_id)
        REFERENCES users(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_staff_store_assignment
    ON commerce_staff_store_assignments (organization_id, store_id, user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_commerce_staff_store_user
    ON commerce_staff_store_assignments (organization_id, user_id, status) WHERE deleted_at IS NULL;
