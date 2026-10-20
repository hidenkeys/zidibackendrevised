CREATE TABLE IF NOT EXISTS commerce_fulfilments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    order_id UUID NOT NULL,
    store_id UUID NOT NULL,
    customer_id UUID NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('customer_pickup', 'customer_rider', 'merchant_rider')),
    status TEXT NOT NULL CHECK (status IN (
        'ready_for_pickup', 'awaiting_quote', 'awaiting_customer_confirmation',
        'rider_requested', 'rider_assigned', 'out_for_delivery', 'delivered',
        'completed', 'cancelled'
    )),
    pickup_address TEXT NOT NULL,
    pickup_latitude NUMERIC(10,7),
    pickup_longitude NUMERIC(10,7),
    destination_address TEXT,
    destination_latitude NUMERIC(10,7),
    destination_longitude NUMERIC(10,7),
    verification_code_hash BYTEA NOT NULL CHECK (OCTET_LENGTH(verification_code_hash) = 32),
    verification_code_ciphertext BYTEA NOT NULL CHECK (OCTET_LENGTH(verification_code_ciphertext) >= 28),
    verification_attempts INTEGER NOT NULL DEFAULT 0 CHECK (verification_attempts >= 0),
    verification_locked_until TIMESTAMPTZ,
    verification_code_expires_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ,
    verified_by_user_id UUID,
    handed_over_at TIMESTAMPTZ,
    handed_over_by_user_id UUID,
    delivered_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_commerce_fulfilment_pickup_coordinates CHECK (
        (pickup_latitude IS NULL AND pickup_longitude IS NULL)
        OR (pickup_latitude BETWEEN -90 AND 90 AND pickup_longitude BETWEEN -180 AND 180)
    ),
    CONSTRAINT chk_commerce_fulfilment_destination_coordinates CHECK (
        (destination_latitude IS NULL AND destination_longitude IS NULL)
        OR (destination_latitude BETWEEN -90 AND 90 AND destination_longitude BETWEEN -180 AND 180)
    ),
    CONSTRAINT chk_commerce_fulfilment_destination CHECK (
        mode <> 'merchant_rider' OR destination_address IS NOT NULL
    ),
    CONSTRAINT fk_commerce_fulfilment_order_tenant
        FOREIGN KEY (order_id, organization_id)
        REFERENCES commerce_orders(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_fulfilment_store_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_fulfilment_customer_tenant
        FOREIGN KEY (customer_id, organization_id)
        REFERENCES commerce_customers(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_fulfilments_id_organization
    ON commerce_fulfilments (id, organization_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_fulfilment_order
    ON commerce_fulfilments (organization_id, order_id);
CREATE INDEX IF NOT EXISTS idx_commerce_fulfilment_store_status
    ON commerce_fulfilments (organization_id, store_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS commerce_delivery_quotes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    fulfilment_id UUID NOT NULL,
    order_id UUID NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('manual', 'provider')),
    provider TEXT,
    provider_quote_id TEXT,
    status TEXT NOT NULL CHECK (status IN ('quoted', 'accepted', 'rejected', 'expired')),
    pickup_address TEXT NOT NULL,
    pickup_latitude NUMERIC(10,7),
    pickup_longitude NUMERIC(10,7),
    destination_address TEXT NOT NULL,
    destination_latitude NUMERIC(10,7),
    destination_longitude NUMERIC(10,7),
    distance_meters INTEGER CHECK (distance_meters IS NULL OR distance_meters >= 0),
    duration_seconds INTEGER CHECK (duration_seconds IS NULL OR duration_seconds >= 0),
    estimated_fee_minor BIGINT NOT NULL CHECK (estimated_fee_minor >= 0),
    currency CHAR(3) NOT NULL,
    fee_payment_mode TEXT NOT NULL DEFAULT 'direct_to_rider'
        CHECK (fee_payment_mode IN ('direct_to_rider', 'zidi_collected')),
    fee_status TEXT NOT NULL DEFAULT 'not_collected'
        CHECK (fee_status IN ('not_collected', 'due', 'paid_external', 'paid')),
    raw_response JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(raw_response) = 'object'),
    idempotency_key TEXT NOT NULL CHECK (LENGTH(idempotency_key) BETWEEN 8 AND 200),
    created_by_user_id UUID NOT NULL,
    expires_at TIMESTAMPTZ,
    accepted_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commerce_delivery_quote_fulfilment_tenant
        FOREIGN KEY (fulfilment_id, organization_id)
        REFERENCES commerce_fulfilments(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_delivery_quote_order_tenant
        FOREIGN KEY (order_id, organization_id)
        REFERENCES commerce_orders(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_commerce_delivery_quote_provider CHECK (
        (source = 'manual' AND provider IS NULL)
        OR (source = 'provider' AND provider IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_delivery_quotes_id_organization
    ON commerce_delivery_quotes (id, organization_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_delivery_quote_idempotency
    ON commerce_delivery_quotes (organization_id, fulfilment_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_commerce_delivery_quote_history
    ON commerce_delivery_quotes (organization_id, fulfilment_id, created_at DESC);

CREATE TABLE IF NOT EXISTS commerce_rider_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    fulfilment_id UUID NOT NULL,
    order_id UUID NOT NULL,
    store_id UUID NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('customer', 'merchant')),
    provider TEXT,
    provider_assignment_id TEXT,
    rider_name TEXT NOT NULL,
    rider_phone TEXT NOT NULL,
    vehicle_description TEXT,
    tracking_url TEXT,
    status TEXT NOT NULL CHECK (status IN ('assigned', 'arrived', 'picked_up', 'delivered', 'cancelled')),
    idempotency_key TEXT NOT NULL CHECK (LENGTH(idempotency_key) BETWEEN 8 AND 200),
    assigned_by_user_id UUID NOT NULL,
    arrived_at TIMESTAMPTZ,
    picked_up_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commerce_rider_assignment_fulfilment_tenant
        FOREIGN KEY (fulfilment_id, organization_id)
        REFERENCES commerce_fulfilments(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_rider_assignment_order_tenant
        FOREIGN KEY (order_id, organization_id)
        REFERENCES commerce_orders(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_rider_assignment_store_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_rider_assignments_id_organization
    ON commerce_rider_assignments (id, organization_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_rider_assignment_idempotency
    ON commerce_rider_assignments (organization_id, fulfilment_id, idempotency_key);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_active_rider_assignment
    ON commerce_rider_assignments (organization_id, fulfilment_id)
    WHERE status IN ('assigned', 'arrived', 'picked_up');

CREATE TABLE IF NOT EXISTS commerce_fulfilment_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    fulfilment_id UUID NOT NULL,
    order_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    from_status TEXT,
    to_status TEXT NOT NULL,
    actor_type TEXT NOT NULL CHECK (actor_type IN ('user', 'system', 'customer', 'provider')),
    actor_user_id UUID,
    reason TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    idempotency_key TEXT NOT NULL CHECK (LENGTH(idempotency_key) BETWEEN 8 AND 200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commerce_fulfilment_event_fulfilment_tenant
        FOREIGN KEY (fulfilment_id, organization_id)
        REFERENCES commerce_fulfilments(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_fulfilment_event_order_tenant
        FOREIGN KEY (order_id, organization_id)
        REFERENCES commerce_orders(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_fulfilment_event_idempotency
    ON commerce_fulfilment_events (organization_id, fulfilment_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_commerce_fulfilment_event_history
    ON commerce_fulfilment_events (organization_id, fulfilment_id, created_at ASC);
