CREATE TABLE IF NOT EXISTS commerce_invoices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    order_id UUID NOT NULL,
    store_id UUID NOT NULL,
    customer_id UUID NOT NULL,
    invoice_number TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('issued', 'paid', 'void')),
    merchant_name TEXT NOT NULL,
    store_name TEXT NOT NULL,
    store_address TEXT NOT NULL,
    customer_name TEXT NOT NULL,
    customer_email TEXT,
    order_number TEXT NOT NULL,
    fulfilment_mode TEXT NOT NULL CHECK (fulfilment_mode IN ('customer_pickup', 'customer_rider', 'merchant_rider')),
    currency CHAR(3) NOT NULL,
    subtotal_minor BIGINT NOT NULL CHECK (subtotal_minor >= 0),
    discount_minor BIGINT NOT NULL DEFAULT 0 CHECK (discount_minor >= 0),
    delivery_fee_minor BIGINT NOT NULL DEFAULT 0 CHECK (delivery_fee_minor >= 0),
    total_minor BIGINT NOT NULL CHECK (total_minor >= 0),
    issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    paid_at TIMESTAMPTZ,
    voided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_commerce_invoice_total CHECK (
        discount_minor <= subtotal_minor
        AND total_minor = subtotal_minor - discount_minor + delivery_fee_minor
    ),
    CONSTRAINT fk_commerce_invoice_order_tenant
        FOREIGN KEY (order_id, organization_id)
        REFERENCES commerce_orders(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_invoice_store_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_invoice_customer_tenant
        FOREIGN KEY (customer_id, organization_id)
        REFERENCES commerce_customers(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_invoices_id_organization
    ON commerce_invoices (id, organization_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_invoice_order
    ON commerce_invoices (organization_id, order_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_invoice_number
    ON commerce_invoices (organization_id, invoice_number);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_order_items_id_organization
    ON commerce_order_items (id, organization_id);

CREATE TABLE IF NOT EXISTS commerce_invoice_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    invoice_id UUID NOT NULL,
    order_item_id UUID NOT NULL,
    product_id UUID NOT NULL,
    variant_id UUID NOT NULL,
    product_name TEXT NOT NULL,
    variant_name TEXT NOT NULL,
    sku TEXT NOT NULL,
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(attributes) = 'object'),
    quantity INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 100),
    unit_price_minor BIGINT NOT NULL CHECK (unit_price_minor >= 0),
    line_total_minor BIGINT NOT NULL CHECK (line_total_minor >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_commerce_invoice_item_total CHECK (line_total_minor = unit_price_minor * quantity),
    CONSTRAINT fk_commerce_invoice_item_invoice_tenant
        FOREIGN KEY (invoice_id, organization_id)
        REFERENCES commerce_invoices(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_invoice_item_order_item_tenant
        FOREIGN KEY (order_item_id, organization_id)
        REFERENCES commerce_order_items(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_invoice_item_product_tenant
        FOREIGN KEY (product_id, organization_id)
        REFERENCES commerce_products(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_invoice_item_variant_tenant
        FOREIGN KEY (variant_id, organization_id)
        REFERENCES commerce_product_variants(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_invoice_item_order_item
    ON commerce_invoice_items (organization_id, invoice_id, order_item_id);
CREATE INDEX IF NOT EXISTS idx_commerce_invoice_items_invoice
    ON commerce_invoice_items (organization_id, invoice_id, created_at);

CREATE TABLE IF NOT EXISTS commerce_payment_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    order_id UUID NOT NULL,
    invoice_id UUID NOT NULL,
    provider TEXT NOT NULL CHECK (LENGTH(provider) BETWEEN 2 AND 50),
    provider_reference TEXT NOT NULL CHECK (LENGTH(provider_reference) BETWEEN 8 AND 200),
    provider_transaction_id TEXT,
    idempotency_key TEXT NOT NULL CHECK (LENGTH(idempotency_key) BETWEEN 8 AND 200),
    payer_email TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN (
        'initializing', 'pending', 'succeeded', 'failed', 'expired', 'review_required'
    )),
    currency CHAR(3) NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
    authorization_url TEXT,
    access_code TEXT,
    failure_reason TEXT NOT NULL DEFAULT '',
    provider_response JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(provider_response) = 'object'),
    expires_at TIMESTAMPTZ NOT NULL,
    initialized_at TIMESTAMPTZ,
    confirmed_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commerce_payment_order_tenant
        FOREIGN KEY (order_id, organization_id)
        REFERENCES commerce_orders(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_payment_invoice_tenant
        FOREIGN KEY (invoice_id, organization_id)
        REFERENCES commerce_invoices(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_payments_id_organization
    ON commerce_payment_transactions (id, organization_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_payment_reference
    ON commerce_payment_transactions (provider, provider_reference);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_payment_provider_transaction
    ON commerce_payment_transactions (provider, provider_transaction_id)
    WHERE provider_transaction_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_payment_idempotency
    ON commerce_payment_transactions (organization_id, idempotency_key);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_active_payment_order
    ON commerce_payment_transactions (organization_id, order_id, provider)
    WHERE status IN ('initializing', 'pending');
CREATE INDEX IF NOT EXISTS idx_commerce_payment_order_history
    ON commerce_payment_transactions (organization_id, order_id, created_at DESC);

CREATE TABLE IF NOT EXISTS commerce_payment_webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID,
    order_id UUID,
    payment_id UUID,
    provider TEXT NOT NULL CHECK (LENGTH(provider) BETWEEN 2 AND 50),
    event_key TEXT NOT NULL CHECK (LENGTH(event_key) BETWEEN 8 AND 250),
    event_type TEXT NOT NULL,
    provider_reference TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('received', 'processed', 'ignored', 'failed')),
    failure_reason TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commerce_webhook_organization
        FOREIGN KEY (organization_id)
        REFERENCES organizations(id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_webhook_order_tenant
        FOREIGN KEY (order_id, organization_id)
        REFERENCES commerce_orders(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_webhook_payment_tenant
        FOREIGN KEY (payment_id, organization_id)
        REFERENCES commerce_payment_transactions(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_commerce_webhook_tenant_links CHECK (
        (organization_id IS NULL AND order_id IS NULL AND payment_id IS NULL)
        OR (organization_id IS NOT NULL AND order_id IS NOT NULL AND payment_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_webhook_event
    ON commerce_payment_webhook_events (provider, event_key);
CREATE INDEX IF NOT EXISTS idx_commerce_webhook_reference
    ON commerce_payment_webhook_events (provider, provider_reference, received_at DESC);

CREATE TABLE IF NOT EXISTS commerce_outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    aggregate_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,
    topic TEXT NOT NULL,
    deduplication_key TEXT NOT NULL CHECK (LENGTH(deduplication_key) BETWEEN 8 AND 250),
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'delivered', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_outbox_deduplication
    ON commerce_outbox_events (organization_id, deduplication_key);
CREATE INDEX IF NOT EXISTS idx_commerce_outbox_pending
    ON commerce_outbox_events (status, available_at, created_at)
    WHERE status IN ('pending', 'failed');
