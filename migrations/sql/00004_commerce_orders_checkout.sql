CREATE TABLE IF NOT EXISTS commerce_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    cart_id UUID NOT NULL,
    customer_id UUID NOT NULL,
    store_id UUID NOT NULL,
    order_number TEXT NOT NULL,
    checkout_key TEXT NOT NULL CHECK (LENGTH(checkout_key) BETWEEN 8 AND 200),
    fulfilment_mode TEXT NOT NULL CHECK (fulfilment_mode IN ('customer_pickup', 'customer_rider', 'merchant_rider')),
    status TEXT NOT NULL CHECK (status IN (
        'draft', 'pending_payment', 'paid', 'processing', 'ready',
        'fulfilment_pending', 'ready_for_pickup', 'out_for_delivery',
        'delivered', 'completed', 'payment_failed', 'payment_expired',
        'cancelled', 'refunded'
    )),
    currency CHAR(3) NOT NULL,
    subtotal_minor BIGINT NOT NULL CHECK (subtotal_minor >= 0),
    discount_minor BIGINT NOT NULL DEFAULT 0 CHECK (discount_minor >= 0),
    delivery_fee_minor BIGINT NOT NULL DEFAULT 0 CHECK (delivery_fee_minor >= 0),
    total_minor BIGINT NOT NULL CHECK (total_minor >= 0),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    payment_expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_commerce_order_total CHECK (
        discount_minor <= subtotal_minor
        AND total_minor = subtotal_minor - discount_minor + delivery_fee_minor
    ),
    CONSTRAINT fk_commerce_order_cart_tenant
        FOREIGN KEY (cart_id, organization_id)
        REFERENCES commerce_carts(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_order_customer_tenant
        FOREIGN KEY (customer_id, organization_id)
        REFERENCES commerce_customers(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_order_store_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_orders_id_organization
    ON commerce_orders (id, organization_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_orders_checkout_key
    ON commerce_orders (organization_id, checkout_key);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_orders_cart
    ON commerce_orders (organization_id, cart_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_orders_number
    ON commerce_orders (organization_id, order_number);
CREATE INDEX IF NOT EXISTS idx_commerce_orders_store_status
    ON commerce_orders (organization_id, store_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_commerce_orders_customer
    ON commerce_orders (organization_id, customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_commerce_orders_payment_expiry
    ON commerce_orders (organization_id, status, payment_expires_at)
    WHERE status IN ('pending_payment', 'payment_failed');

CREATE TABLE IF NOT EXISTS commerce_order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    order_id UUID NOT NULL,
    product_id UUID NOT NULL,
    variant_id UUID NOT NULL,
    inventory_reservation_id UUID NOT NULL,
    product_name TEXT NOT NULL,
    variant_name TEXT NOT NULL,
    sku TEXT NOT NULL,
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(attributes) = 'object'),
    primary_image_url TEXT,
    quantity INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 100),
    unit_price_minor BIGINT NOT NULL CHECK (unit_price_minor >= 0),
    line_total_minor BIGINT NOT NULL CHECK (line_total_minor >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_commerce_order_item_total CHECK (line_total_minor = unit_price_minor * quantity),
    CONSTRAINT fk_commerce_order_item_order_tenant
        FOREIGN KEY (order_id, organization_id)
        REFERENCES commerce_orders(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_order_item_product_tenant
        FOREIGN KEY (product_id, organization_id)
        REFERENCES commerce_products(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_order_item_variant_tenant
        FOREIGN KEY (variant_id, organization_id)
        REFERENCES commerce_product_variants(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_order_item_reservation_tenant
        FOREIGN KEY (inventory_reservation_id, organization_id)
        REFERENCES commerce_inventory_reservations(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_order_item_variant
    ON commerce_order_items (organization_id, order_id, variant_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_order_item_reservation
    ON commerce_order_items (organization_id, inventory_reservation_id);
CREATE INDEX IF NOT EXISTS idx_commerce_order_items_order
    ON commerce_order_items (organization_id, order_id, created_at);

CREATE TABLE IF NOT EXISTS commerce_order_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    order_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN (
        'order_created', 'payment_initiated', 'payment_confirmed', 'payment_failed',
        'payment_expired', 'order_processing', 'order_ready', 'fulfilment_pending',
        'ready_for_pickup', 'out_for_delivery', 'order_delivered', 'order_completed',
        'order_cancelled', 'order_refunded', 'customer_notified', 'rider_requested',
        'rider_assigned', 'order_picked_up'
    )),
    from_status TEXT CHECK (from_status IS NULL OR from_status IN (
        'draft', 'pending_payment', 'paid', 'processing', 'ready',
        'fulfilment_pending', 'ready_for_pickup', 'out_for_delivery',
        'delivered', 'completed', 'payment_failed', 'payment_expired',
        'cancelled', 'refunded'
    )),
    to_status TEXT NOT NULL CHECK (to_status IN (
        'draft', 'pending_payment', 'paid', 'processing', 'ready',
        'fulfilment_pending', 'ready_for_pickup', 'out_for_delivery',
        'delivered', 'completed', 'payment_failed', 'payment_expired',
        'cancelled', 'refunded'
    )),
    actor_type TEXT NOT NULL CHECK (actor_type IN ('system', 'user', 'payment', 'channel')),
    actor_user_id UUID,
    reason TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    idempotency_key TEXT NOT NULL CHECK (LENGTH(idempotency_key) BETWEEN 8 AND 250),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commerce_order_event_order_tenant
        FOREIGN KEY (order_id, organization_id)
        REFERENCES commerce_orders(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_order_event_actor
        FOREIGN KEY (actor_user_id)
        REFERENCES users(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_order_event_idempotency
    ON commerce_order_events (organization_id, order_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_commerce_order_events_history
    ON commerce_order_events (organization_id, order_id, created_at ASC);
