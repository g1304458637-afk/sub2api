-- Additive only: opt-in conversion is a separate, audited operation.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS quota_policy VARCHAR(32) NOT NULL DEFAULT 'legacy';
ALTER TABLE groups ADD COLUMN IF NOT EXISTS short_limit_usd NUMERIC(20,10);
ALTER TABLE groups ADD CONSTRAINT groups_quota_policy_valid CHECK (quota_policy IN ('legacy', 'dual_window_v1'));
ALTER TABLE groups ADD CONSTRAINT groups_dual_limits_valid CHECK (quota_policy <> 'dual_window_v1' OR (short_limit_usd IS NOT NULL AND short_limit_usd > 0 AND weekly_limit_usd IS NOT NULL AND weekly_limit_usd > 0));
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS short_window_start TIMESTAMPTZ;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS short_usage_usd NUMERIC(20,10) NOT NULL DEFAULT 0;

-- All dual-window advancement uses the same row lock as settlement and reset.
CREATE FUNCTION campus_advance_dual_windows(p_id BIGINT, p_now TIMESTAMPTZ, p_activate BOOLEAN, p_day_start TIMESTAMPTZ)
RETURNS VOID LANGUAGE plpgsql AS $$
DECLARE
    s user_subscriptions%ROWTYPE;
    policy TEXT;
    short_start TIMESTAMPTZ;
    week_start TIMESTAMPTZ;
    effective_now TIMESTAMPTZ;
    day_start TIMESTAMPTZ;
    month_start TIMESTAMPTZ;
BEGIN
    SELECT * INTO s FROM user_subscriptions WHERE id = p_id AND deleted_at IS NULL FOR UPDATE;
    IF NOT FOUND THEN RETURN; END IF;
    -- Hold policy stable through the caller's settlement transaction.
    -- A policy migration cannot split advancement and charging across policies.
    SELECT quota_policy INTO policy FROM groups WHERE id = s.group_id FOR SHARE;
    IF policy IS DISTINCT FROM 'dual_window_v1' THEN RETURN; END IF;
    -- Late settlements remain accountable in the final valid window.
    effective_now := LEAST(p_now, s.expires_at - INTERVAL '1 microsecond');
    short_start := s.short_window_start;
    week_start := s.weekly_window_start;
    IF p_activate AND short_start IS NULL THEN short_start := effective_now; END IF;
    IF p_activate AND week_start IS NULL THEN week_start := effective_now; END IF;
    IF short_start IS NOT NULL AND effective_now >= short_start + INTERVAL '5 hours' THEN
        short_start := short_start + FLOOR(EXTRACT(EPOCH FROM (effective_now - short_start)) / 18000) * INTERVAL '5 hours';
    END IF;
    IF week_start IS NOT NULL AND effective_now >= week_start + INTERVAL '168 hours' THEN
        week_start := week_start + FLOOR(EXTRACT(EPOCH FROM (effective_now - week_start)) / 604800) * INTERVAL '168 hours';
    END IF;
    -- Statistics keep their existing calendar-day / rolling-30-day semantics.
    -- Neither counter participates in dual-window admission or manual reset.
    day_start := s.daily_window_start;
    month_start := s.monthly_window_start;
    IF p_activate AND day_start IS NULL THEN day_start := p_day_start; END IF;
    IF day_start IS NOT NULL AND p_day_start > day_start AND effective_now >= p_day_start THEN
        day_start := p_day_start;
    END IF;
    IF p_activate AND month_start IS NULL THEN month_start := effective_now; END IF;
    IF month_start IS NOT NULL AND effective_now >= month_start + INTERVAL '720 hours' THEN
        month_start := month_start + FLOOR(EXTRACT(EPOCH FROM (effective_now - month_start)) / 2592000) * INTERVAL '720 hours';
    END IF;
    UPDATE user_subscriptions SET
        daily_usage_usd = CASE WHEN daily_window_start IS DISTINCT FROM day_start THEN 0 ELSE daily_usage_usd END,
        monthly_usage_usd = CASE WHEN monthly_window_start IS DISTINCT FROM month_start THEN 0 ELSE monthly_usage_usd END,
        daily_window_start = day_start, monthly_window_start = month_start,
        short_usage_usd = CASE WHEN short_window_start IS DISTINCT FROM short_start THEN 0 ELSE short_usage_usd END,
        weekly_usage_usd = CASE WHEN weekly_window_start IS DISTINCT FROM week_start THEN 0 ELSE weekly_usage_usd END,
        short_window_start = short_start, weekly_window_start = week_start
    WHERE id = p_id AND (short_window_start IS DISTINCT FROM short_start OR weekly_window_start IS DISTINCT FROM week_start OR daily_window_start IS DISTINCT FROM day_start OR monthly_window_start IS DISTINCT FROM month_start);
END;
$$;

CREATE TABLE subscription_dual_reset_audit (
 id BIGSERIAL PRIMARY KEY,
 subscription_id BIGINT NOT NULL,
 effective_at TIMESTAMPTZ NOT NULL,
 source TEXT NOT NULL,
 card_id BIGINT,
 event_id BIGINT,
 actor_id BIGINT,
 previous_short_usage NUMERIC(20,10) NOT NULL,
 previous_weekly_usage NUMERIC(20,10) NOT NULL,
 previous_short_start TIMESTAMPTZ,
 previous_weekly_start TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX subscription_dual_reset_audit_subscription_idx ON subscription_dual_reset_audit(subscription_id, created_at);

-- Explicit cohort migration audit. Historical quota usage is preserved in place.
CREATE TABLE subscription_quota_policy_audit (
 id BIGSERIAL PRIMARY KEY,
 group_id BIGINT NOT NULL,
 old_policy TEXT NOT NULL,
 new_policy TEXT NOT NULL,
 old_limits JSONB NOT NULL,
 new_limits JSONB NOT NULL,
 actor TEXT NOT NULL,
 affected_subscriptions BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
