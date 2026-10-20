CREATE TABLE IF NOT EXISTS commerce_customers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    display_name TEXT NOT NULL DEFAULT '',
    email TEXT,
    status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_customers_id_organization
    ON commerce_customers (id, organization_id);
CREATE INDEX IF NOT EXISTS idx_commerce_customers_organization_status
    ON commerce_customers (organization_id, status, created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS commerce_customer_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    customer_id UUID NOT NULL,
    channel TEXT NOT NULL CHECK (channel IN ('whatsapp', 'phone', 'email', 'web')),
    normalized_identifier TEXT NOT NULL CHECK (LENGTH(normalized_identifier) BETWEEN 3 AND 320),
    display_identifier TEXT NOT NULL,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_commerce_customer_identity_tenant
        FOREIGN KEY (customer_id, organization_id)
        REFERENCES commerce_customers(id, organization_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_customer_identity
    ON commerce_customer_identities (organization_id, channel, normalized_identifier)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_commerce_customer_identity_customer
    ON commerce_customer_identities (organization_id, customer_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS commerce_carts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    customer_id UUID NOT NULL,
    store_id UUID NOT NULL,
    currency CHAR(3) NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'converted', 'abandoned', 'expired')),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commerce_cart_customer_tenant
        FOREIGN KEY (customer_id, organization_id)
        REFERENCES commerce_customers(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_cart_store_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_carts_id_organization
    ON commerce_carts (id, organization_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_active_cart_customer_store
    ON commerce_carts (organization_id, customer_id, store_id) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_commerce_carts_customer_status
    ON commerce_carts (organization_id, customer_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_commerce_carts_expiry
    ON commerce_carts (organization_id, status, expires_at) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS commerce_cart_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    cart_id UUID NOT NULL,
    variant_id UUID NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commerce_cart_item_cart_tenant
        FOREIGN KEY (cart_id, organization_id)
        REFERENCES commerce_carts(id, organization_id) ON DELETE CASCADE,
    CONSTRAINT fk_commerce_cart_item_variant_tenant
        FOREIGN KEY (variant_id, organization_id)
        REFERENCES commerce_product_variants(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_cart_item_variant
    ON commerce_cart_items (organization_id, cart_id, variant_id);
CREATE INDEX IF NOT EXISTS idx_commerce_cart_items_cart
    ON commerce_cart_items (organization_id, cart_id, created_at);
