ALTER TABLE commerce_fulfilments
    DROP CONSTRAINT IF EXISTS commerce_fulfilments_status_check;

ALTER TABLE commerce_fulfilments
    ADD CONSTRAINT commerce_fulfilments_status_check CHECK (status IN (
        'preparing', 'ready_for_pickup', 'awaiting_quote', 'awaiting_customer_confirmation',
        'rider_requested', 'rider_assigned', 'out_for_delivery',
        'awaiting_delivery_confirmation', 'delivery_issue', 'delivered',
        'completed', 'cancelled'
    ));

ALTER TABLE commerce_fulfilments
    ADD COLUMN IF NOT EXISTS expected_delivery_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS delivery_confirmation_requested_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS delivery_confirmation_deadline_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS delivery_confirmation_status TEXT;

ALTER TABLE commerce_fulfilments
    DROP CONSTRAINT IF EXISTS chk_commerce_delivery_confirmation_status;

ALTER TABLE commerce_fulfilments
    ADD CONSTRAINT chk_commerce_delivery_confirmation_status CHECK (
        delivery_confirmation_status IS NULL OR delivery_confirmation_status IN (
            'pending', 'received', 'not_received', 'manual', 'unanswered'
        )
    );

CREATE INDEX IF NOT EXISTS idx_commerce_delivery_confirmation_deadline
    ON commerce_fulfilments (delivery_confirmation_deadline_at)
    WHERE status = 'awaiting_delivery_confirmation';
