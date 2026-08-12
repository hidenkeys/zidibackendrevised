ALTER TABLE commerce_orders
    ADD COLUMN IF NOT EXISTS customer_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS customer_phone TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS customer_email TEXT;

CREATE INDEX IF NOT EXISTS idx_commerce_orders_operational_search
    ON commerce_orders (organization_id, store_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_commerce_orders_customer_name_search
    ON commerce_orders (organization_id, LOWER(customer_name));

CREATE INDEX IF NOT EXISTS idx_commerce_orders_customer_phone_search
    ON commerce_orders (organization_id, customer_phone);

ALTER TABLE commerce_store_fulfilment_modes
    ADD COLUMN IF NOT EXISTS customer_pays BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS pricing_mode TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS fixed_fee_minor BIGINT,
    ADD COLUMN IF NOT EXISTS quote_provider TEXT,
    ADD COLUMN IF NOT EXISTS disclaimer TEXT NOT NULL DEFAULT '';

UPDATE commerce_store_fulfilment_modes
SET customer_pays = TRUE,
    pricing_mode = CASE WHEN pricing_mode = 'none' THEN 'manual' ELSE pricing_mode END,
    disclaimer = CASE
        WHEN disclaimer = '' THEN 'Delivery is arranged separately. The customer pays the delivery fee after accepting the estimate.'
        ELSE disclaimer
    END
WHERE mode = 'merchant_rider';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_commerce_fulfilment_pricing_mode') THEN
        ALTER TABLE commerce_store_fulfilment_modes
            ADD CONSTRAINT chk_commerce_fulfilment_pricing_mode
            CHECK (pricing_mode IN ('none', 'fixed', 'manual', 'provider'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_commerce_fulfilment_fixed_fee') THEN
        ALTER TABLE commerce_store_fulfilment_modes
            ADD CONSTRAINT chk_commerce_fulfilment_fixed_fee
            CHECK (
                (pricing_mode = 'fixed' AND fixed_fee_minor IS NOT NULL AND fixed_fee_minor >= 0)
                OR (pricing_mode <> 'fixed' AND (fixed_fee_minor IS NULL OR fixed_fee_minor >= 0))
            );
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_outbox_id_organization
    ON commerce_outbox_events (id, organization_id);

CREATE TABLE IF NOT EXISTS commerce_email_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    customer_id UUID NOT NULL,
    order_id UUID NOT NULL,
    outbox_event_id UUID NOT NULL,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    html_body TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'skipped')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commerce_email_customer_tenant
        FOREIGN KEY (customer_id, organization_id)
        REFERENCES commerce_customers(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_email_order_tenant
        FOREIGN KEY (order_id, organization_id)
        REFERENCES commerce_orders(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_email_outbox_tenant
        FOREIGN KEY (outbox_event_id, organization_id)
        REFERENCES commerce_outbox_events(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_email_outbox_event
    ON commerce_email_messages (organization_id, outbox_event_id);
CREATE INDEX IF NOT EXISTS idx_commerce_email_delivery
    ON commerce_email_messages (status, available_at, created_at)
    WHERE status IN ('pending', 'failed');
