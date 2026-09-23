-- Normalize wallet-denominated amounts to CNY at the fixed migration rate.
-- Provider settlement amounts (payment_orders.pay_amount) and model metering
-- records (usage_logs) remain in their source currency. The migration runner
-- applies this file atomically; the contract marker is written last.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM settings
        WHERE key = 'wallet_currency_contract'
          AND value <> 'CNY_V1'
    ) THEN
        RAISE EXCEPTION 'unsupported wallet currency contract; refusing conversion';
    END IF;

    IF NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract') THEN
        IF EXISTS (
            SELECT 1 FROM subscription_plans
            WHERE UPPER(BTRIM(COALESCE(currency, ''))) NOT IN ('', 'USD', 'CNY', 'RMB', 'CNH')
        ) THEN
            RAISE EXCEPTION 'subscription plans contain currencies unsupported by CNY_V1';
        END IF;

        IF EXISTS (
            SELECT 1 FROM subscription_terms
            WHERE UPPER(BTRIM(COALESCE(currency, ''))) NOT IN ('', 'USD', 'CNY', 'RMB', 'CNH')
        ) OR EXISTS (
            SELECT 1 FROM subscription_plan_changes
            WHERE UPPER(BTRIM(COALESCE(currency, ''))) NOT IN ('', 'USD', 'CNY', 'RMB', 'CNH')
        ) THEN
            RAISE EXCEPTION 'subscription price snapshots contain currencies unsupported by CNY_V1';
        END IF;

        IF EXISTS (
            SELECT 1 FROM settings
            WHERE (key IN (
                'default_balance', 'student_verification_reward_amount',
                'affiliate_rebate_per_invitee_cap', 'balance_low_notify_threshold',
                'DAILY_RECHARGE_LIMIT', 'MIN_RECHARGE_AMOUNT', 'MAX_RECHARGE_AMOUNT'
            ) OR key ~ '^auth_source_default_[a-z0-9]+_balance$')
              AND BTRIM(value) <> ''
              AND BTRIM(value) !~ '^[+-]?[0-9]+([.][0-9]+)?$'
        ) THEN
            RAISE EXCEPTION 'wallet amount setting contains a non-numeric value; refusing conversion';
        END IF;
    END IF;
END $$;

-- Snapshot each plan's original denomination before normalizing currencies.
-- Subscription terms, plan changes, and orders need this factor after plans
-- themselves have been converted to CNY.
CREATE TEMPORARY TABLE cny_wallet_plan_rates ON COMMIT DROP AS
SELECT id AS plan_id,
       CASE WHEN UPPER(BTRIM(COALESCE(currency, ''))) IN ('CNY', 'RMB', 'CNH') THEN 1.0 ELSE 6.7 END::numeric AS factor
FROM subscription_plans;
CREATE TEMPORARY TABLE cny_wallet_term_rates ON COMMIT DROP AS
SELECT order_id,
       CASE WHEN UPPER(BTRIM(COALESCE(currency, ''))) IN ('CNY', 'RMB', 'CNH') THEN 1.0 ELSE 6.7 END::numeric AS factor
FROM subscription_terms
WHERE order_id IS NOT NULL;
CREATE TEMPORARY TABLE cny_wallet_plan_change_rates ON COMMIT DROP AS
SELECT id AS plan_change_id,
       CASE WHEN UPPER(BTRIM(COALESCE(currency, ''))) IN ('CNY', 'RMB', 'CNH') THEN 1.0 ELSE 6.7 END::numeric AS factor
FROM subscription_plan_changes;

-- Keep one timestamp for legacy refund audit details, whose JSON amounts were
-- written in the old wallet unit and are converted on read by the ledger.
INSERT INTO settings (key, value)
VALUES ('wallet_currency_cutover_at', clock_timestamp()::text)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

UPDATE users
SET balance = ROUND(balance * 6.7, 8),
    frozen_balance = ROUND(COALESCE(frozen_balance, 0) * 6.7, 8),
    total_recharged = ROUND(COALESCE(total_recharged, 0) * 6.7, 8),
    balance_notify_threshold = CASE
        WHEN balance_notify_threshold IS NOT NULL AND balance_notify_threshold_type = 'fixed'
            THEN ROUND(balance_notify_threshold * 6.7, 8)
        ELSE balance_notify_threshold
    END,
    updated_at = NOW()
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

UPDATE user_affiliates
SET aff_quota = ROUND(aff_quota * 6.7, 8),
    aff_frozen_quota = ROUND(aff_frozen_quota * 6.7, 8),
    aff_history_quota = ROUND(aff_history_quota * 6.7, 8),
    updated_at = NOW()
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

-- Prices already denominated in CNY stay numerically unchanged; legacy blank
-- and USD prices are converted. All plans use CNY after this migration.
UPDATE subscription_plans p
SET price = ROUND(p.price * rates.factor, 2),
    original_price = CASE WHEN p.original_price IS NULL THEN NULL ELSE ROUND(p.original_price * rates.factor, 2) END,
    currency = 'CNY',
    updated_at = NOW()
FROM cny_wallet_plan_rates rates
WHERE p.id = rates.plan_id
  AND NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

-- Term rows carry the denomination captured at purchase time; empty legacy
-- values retain the original USD meaning even if the plan has since changed.
UPDATE subscription_terms t
SET price_paid = ROUND(t.price_paid * CASE WHEN UPPER(BTRIM(COALESCE(t.currency, ''))) IN ('CNY', 'RMB', 'CNH') THEN 1 ELSE 6.7 END, 2),
    currency = 'CNY'
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

UPDATE subscription_plan_changes c
SET old_price_snapshot = CASE WHEN old_price_snapshot IS NULL THEN NULL ELSE ROUND(old_price_snapshot * CASE WHEN UPPER(BTRIM(COALESCE(c.currency, ''))) IN ('CNY', 'RMB', 'CNH') THEN 1 ELSE 6.7 END, 2) END,
    new_price_snapshot = ROUND(new_price_snapshot * CASE WHEN UPPER(BTRIM(COALESCE(c.currency, ''))) IN ('CNY', 'RMB', 'CNH') THEN 1 ELSE 6.7 END, 2),
    unused_credit = ROUND(unused_credit * CASE WHEN UPPER(BTRIM(COALESCE(c.currency, ''))) IN ('CNY', 'RMB', 'CNH') THEN 1 ELSE 6.7 END, 2),
    prorated_charge = ROUND(prorated_charge * CASE WHEN UPPER(BTRIM(COALESCE(c.currency, ''))) IN ('CNY', 'RMB', 'CNH') THEN 1 ELSE 6.7 END, 2),
    amount_due = ROUND(amount_due * CASE WHEN UPPER(BTRIM(COALESCE(c.currency, ''))) IN ('CNY', 'RMB', 'CNH') THEN 1 ELSE 6.7 END, 2),
    currency = 'CNY',
    updated_at = NOW()
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

-- payment_orders.amount is the wallet/plan amount; pay_amount is the provider
-- settlement amount and must not change. For plan orders, use the plan's
-- denomination; old balance ledger amounts are USD regardless of provider.
UPDATE payment_orders o
SET amount = ROUND(o.amount * CASE
        WHEN o.order_type = 'subscription' THEN COALESCE((
            SELECT rates.factor
            FROM cny_wallet_term_rates rates WHERE rates.order_id = o.id
        ), (
            SELECT rates.factor
            FROM cny_wallet_plan_rates rates WHERE rates.plan_id = o.plan_id
        ), 6.7)
        WHEN o.order_type = 'plan_change' THEN COALESCE((
            SELECT rates.factor
            FROM cny_wallet_plan_change_rates rates WHERE rates.plan_change_id = o.plan_change_id
        ), 6.7)
        ELSE 6.7
    END, 2),
    refund_amount = ROUND(o.refund_amount * CASE
        WHEN o.order_type = 'subscription' THEN COALESCE((
            SELECT rates.factor
            FROM cny_wallet_term_rates rates WHERE rates.order_id = o.id
        ), (
            SELECT rates.factor
            FROM cny_wallet_plan_rates rates WHERE rates.plan_id = o.plan_id
        ), 6.7)
        WHEN o.order_type = 'plan_change' THEN COALESCE((
            SELECT rates.factor
            FROM cny_wallet_plan_change_rates rates WHERE rates.plan_change_id = o.plan_change_id
        ), 6.7)
        ELSE 6.7
    END, 2),
    provider_snapshot = COALESCE(provider_snapshot, '{}'::jsonb) ||
        jsonb_build_object('amount_currency', 'CNY', 'wallet_usd_to_cny_rate', 6.7),
    updated_at = NOW()
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

-- Convert existing credits and historical wallet ledgers to their fixed-rate
-- CNY equivalent. This keeps ledger totals comparable with the new balance.
UPDATE redeem_codes
SET value = ROUND(value * 6.7, 8)
WHERE type IN ('balance', 'admin_balance', 'affiliate_balance', 'reward_grant')
  AND NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

UPDATE reward_grants
SET amount = ROUND(amount * 6.7, 8)
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

UPDATE user_affiliate_ledger
SET amount = ROUND(amount * 6.7, 8),
    balance_after = CASE WHEN balance_after IS NULL THEN NULL ELSE ROUND(balance_after * 6.7, 8) END,
    aff_quota_after = CASE WHEN aff_quota_after IS NULL THEN NULL ELSE ROUND(aff_quota_after * 6.7, 8) END,
    aff_frozen_quota_after = CASE WHEN aff_frozen_quota_after IS NULL THEN NULL ELSE ROUND(aff_frozen_quota_after * 6.7, 8) END,
    aff_history_quota_after = CASE WHEN aff_history_quota_after IS NULL THEN NULL ELSE ROUND(aff_history_quota_after * 6.7, 8) END,
    updated_at = NOW()
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

UPDATE promo_codes
SET bonus_amount = ROUND(bonus_amount * 6.7, 8), updated_at = NOW()
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

UPDATE promo_code_usages
SET bonus_amount = ROUND(bonus_amount * 6.7, 8)
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

UPDATE research_applications
SET reward_amount = ROUND(reward_amount::numeric * 6.7, 2)::double precision, updated_at = NOW()
WHERE reward_amount IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

UPDATE settings
SET value = ROUND(BTRIM(value)::numeric * 6.7, 8)::text, updated_at = NOW()
WHERE (key IN (
        'default_balance', 'student_verification_reward_amount',
        'affiliate_rebate_per_invitee_cap', 'balance_low_notify_threshold',
        'DAILY_RECHARGE_LIMIT', 'MIN_RECHARGE_AMOUNT', 'MAX_RECHARGE_AMOUNT'
      ) OR key ~ '^auth_source_default_[a-z0-9]+_balance$')
  AND BTRIM(value) ~ '^[+-]?[0-9]+([.][0-9]+)?$'
  AND NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

-- New accounting and display behavior is fixed at this rate; the old
-- subscription-only conversion option is retired.
INSERT INTO settings (key, value)
VALUES
    ('BALANCE_RECHARGE_MULTIPLIER', '1'),
    ('USD_TO_CNY_DISPLAY_RATE', '6.7'),
    ('SUBSCRIPTION_USD_TO_CNY_RATE', '0')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wallet_currency_contract');

INSERT INTO settings (key, value)
VALUES ('wallet_currency_contract', 'CNY_V1')
ON CONFLICT (key) DO NOTHING;
