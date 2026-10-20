CREATE TABLE IF NOT EXISTS commerce_channel_configurations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    channel TEXT NOT NULL CHECK (channel IN ('whatsapp')),
    provider_account_id TEXT NOT NULL CHECK (LENGTH(provider_account_id) BETWEEN 3 AND 200),
    display_phone_number TEXT,
    welcome_message TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_channel_provider_account
    ON commerce_channel_configurations (channel, provider_account_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_channel_organization
    ON commerce_channel_configurations (organization_id, channel);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_channel_config_id_organization
    ON commerce_channel_configurations (id, organization_id);

CREATE TABLE IF NOT EXISTS commerce_conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    channel_configuration_id UUID NOT NULL,
    customer_id UUID NOT NULL,
    channel TEXT NOT NULL CHECK (channel IN ('whatsapp')),
    external_user_id TEXT NOT NULL CHECK (LENGTH(external_user_id) BETWEEN 3 AND 200),
    state TEXT NOT NULL,
    current_intent TEXT,
    context JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(context) = 'object'),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    processing_message_id UUID,
    locked_until TIMESTAMPTZ,
    last_message_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commerce_conversation_configuration_tenant
        FOREIGN KEY (channel_configuration_id, organization_id)
        REFERENCES commerce_channel_configurations(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_conversation_customer_tenant
        FOREIGN KEY (customer_id, organization_id)
        REFERENCES commerce_customers(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_conversation_identity
    ON commerce_conversations (organization_id, channel, external_user_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_conversation_id_organization
    ON commerce_conversations (id, organization_id);
CREATE INDEX IF NOT EXISTS idx_commerce_conversation_customer
    ON commerce_conversations (organization_id, customer_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS commerce_channel_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    channel_configuration_id UUID NOT NULL,
    conversation_id UUID NOT NULL,
    direction TEXT NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    external_message_id TEXT,
    provider_message_id TEXT,
    sender_id TEXT NOT NULL DEFAULT '',
    recipient_id TEXT NOT NULL DEFAULT '',
    message_type TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(payload) = 'object'),
    status TEXT NOT NULL CHECK (status IN ('received', 'processing', 'processed', 'pending', 'sent', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    processed_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commerce_channel_message_configuration_tenant
        FOREIGN KEY (channel_configuration_id, organization_id)
        REFERENCES commerce_channel_configurations(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_channel_message_conversation_tenant
        FOREIGN KEY (conversation_id, organization_id)
        REFERENCES commerce_conversations(id, organization_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_channel_inbound_message
    ON commerce_channel_messages (channel_configuration_id, external_message_id)
    WHERE direction = 'inbound' AND external_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_commerce_channel_outbound_delivery
    ON commerce_channel_messages (status, available_at, created_at)
    WHERE direction = 'outbound' AND status IN ('pending', 'failed');

CREATE TABLE IF NOT EXISTS commerce_complaints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    customer_id UUID NOT NULL,
    order_id UUID,
    store_id UUID,
    conversation_id UUID,
    category TEXT NOT NULL DEFAULT 'other',
    description TEXT NOT NULL CHECK (LENGTH(description) BETWEEN 3 AND 4000),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'resolved', 'closed')),
    resolution TEXT,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commerce_complaint_customer_tenant
        FOREIGN KEY (customer_id, organization_id)
        REFERENCES commerce_customers(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_complaint_order_tenant
        FOREIGN KEY (order_id, organization_id)
        REFERENCES commerce_orders(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_complaint_store_tenant
        FOREIGN KEY (store_id, organization_id)
        REFERENCES commerce_stores(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_commerce_complaint_conversation_tenant
        FOREIGN KEY (conversation_id, organization_id)
        REFERENCES commerce_conversations(id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_commerce_complaint_resolution CHECK (
        (status IN ('open', 'in_progress') AND resolved_at IS NULL)
        OR (status IN ('resolved', 'closed') AND resolved_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_complaint_id_organization
    ON commerce_complaints (id, organization_id);
CREATE INDEX IF NOT EXISTS idx_commerce_complaint_staff_queue
    ON commerce_complaints (organization_id, store_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_commerce_complaint_customer
    ON commerce_complaints (organization_id, customer_id, created_at DESC);

ALTER TABLE commerce_outbox_events
    ADD COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_error TEXT;

ALTER TABLE commerce_orders
    ADD COLUMN IF NOT EXISTS destination_address TEXT,
    ADD COLUMN IF NOT EXISTS destination_latitude NUMERIC(10,7),
    ADD COLUMN IF NOT EXISTS destination_longitude NUMERIC(10,7);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_commerce_order_destination_coordinates') THEN
        ALTER TABLE commerce_orders ADD CONSTRAINT chk_commerce_order_destination_coordinates CHECK (
            (destination_latitude IS NULL AND destination_longitude IS NULL)
            OR (destination_latitude BETWEEN -90 AND 90 AND destination_longitude BETWEEN -180 AND 180)
        );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_commerce_outbox_customer_delivery
    ON commerce_outbox_events (status, available_at, created_at)
    WHERE status IN ('pending', 'failed');
