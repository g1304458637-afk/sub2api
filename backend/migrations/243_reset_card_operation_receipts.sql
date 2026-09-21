-- Durable receipts and cancellation fences outlive the generic idempotency TTL.
-- The receipt is committed atomically with card consumption and weekly reset.
CREATE TABLE IF NOT EXISTS subscription_reset_operations (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash TEXT NOT NULL,
    subscription_id BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('pending', 'succeeded', 'cancelled')),
    result JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, key_hash)
);
