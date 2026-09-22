-- Usage: psql ... -v group_ids=11,12,13 -v apply=0 -f scripts/migrate-dual-quota.sql
-- Default dry run. Set apply=1 only for the explicitly reviewed campus groups.
\set ON_ERROR_STOP on
\if :{?apply}
\else
  \set apply 0
\endif
BEGIN;
CREATE TEMP TABLE quota_migration_targets ON COMMIT DROP AS
 SELECT DISTINCT unnest(string_to_array(:'group_ids', ',')::bigint[]) AS id;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM quota_migration_targets) OR EXISTS (
  SELECT 1 FROM quota_migration_targets t LEFT JOIN groups g ON g.id=t.id
  WHERE g.id IS NULL OR g.deleted_at IS NOT NULL OR g.subscription_type <> 'subscription'
    OR g.quota_policy NOT IN ('legacy','dual_window_v1')
    OR g.daily_limit_usd IS NULL OR g.daily_limit_usd <= 0
    OR g.weekly_limit_usd IS NULL OR g.weekly_limit_usd <= 0
 ) THEN RAISE EXCEPTION 'invalid cohort or missing positive daily/weekly limits'; END IF;
END $$;
SELECT g.id FROM groups g JOIN quota_migration_targets t ON t.id=g.id FOR UPDATE OF g;
INSERT INTO subscription_quota_policy_audit(group_id,old_policy,new_policy,old_limits,new_limits,actor,affected_subscriptions)
 SELECT g.id,g.quota_policy,'dual_window_v1',
 jsonb_build_object('daily',g.daily_limit_usd,'weekly',g.weekly_limit_usd,'monthly',g.monthly_limit_usd,'short',g.short_limit_usd),
 jsonb_build_object('short',g.daily_limit_usd,'weekly',g.weekly_limit_usd),current_user,
 (SELECT count(*) FROM user_subscriptions s WHERE s.group_id=g.id AND s.deleted_at IS NULL)
 FROM groups g JOIN quota_migration_targets t ON t.id=g.id WHERE g.quota_policy='legacy';
-- New short columns are initially NULL/0. Do not modify usage or window anchors.
UPDATE groups g SET quota_policy='dual_window_v1',short_limit_usd=daily_limit_usd,updated_at=NOW()
 FROM quota_migration_targets t WHERE t.id=g.id AND g.quota_policy='legacy';
SELECT g.id,g.name,g.quota_policy,g.short_limit_usd,g.weekly_limit_usd,
 (SELECT count(*) FROM user_subscriptions s WHERE s.group_id=g.id AND s.deleted_at IS NULL) AS subscriptions
 FROM groups g JOIN quota_migration_targets t ON t.id=g.id ORDER BY g.id;
\if :apply
 COMMIT;
\else
 ROLLBACK;
\endif
