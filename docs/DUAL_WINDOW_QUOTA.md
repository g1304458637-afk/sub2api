# Campus subscription dual windows

Implemented policy: `dual_window_v1`. Legacy groups retain their original quota behavior.

## Contract

- One subscription row owns both budgets, shared across its API keys.
- Short window: 5 hours. Weekly window: 168 hours. Both must have positive limits.
- Either exhausted window blocks subscription billing; existing explicit wallet fallback remains independent.
- First admission starts an uninitialized window. Status reads do not activate it. Expired windows advance by whole periods from their existing anchors. Unused budget never stacks.
- Settlement uses the window current at settlement time, under the same subscription row lock as reset. An in-flight request settled after reset consumes the new budget. Existing request-id deduplication remains authoritative.
- Daily and rolling-30-day counters remain statistics only. The immutable usage ledger is not deleted. API key / user platform safety limits remain separate controls.
- A reset card is usable at any usage level and resets both quota windows to its effective time. It does not credit a wallet, change prices or extend subscription expiry. An old client without `X-Quota-Contract: 2` cannot spend a card on a dual-window subscription.
- Newly created batch reset events freeze contract version 2; historical events retain weekly-only scope. The additive reset audit records old short/weekly usage and anchors.
- Upgrade preserves absolute usage, anchors and expiry. Both new limits must be at least their previous values, and policies must match. The percentage preview is an estimate; settlement during payment can change the final remaining value.
- Renewal during an active term extends validity without replenishment. An expired subscription's new paid term starts fresh windows.
- User quota surfaces show remaining percentages. A positive value below 1% displays `<1%`; missing data displays `—`. A subscription expiring before its next window boundary does not promise an automatic recovery.

## Schema and opt-in migration

Migration 244 is additive. It creates short-window columns, policy configuration, a locking advancement function, and audit tables; it does not automatically convert any group.

`psql ... -v group_ids=11,12,13 -v apply=0 -f scripts/migrate-dual-quota.sql` previews explicitly selected groups and rolls back. Review actual campus group IDs first; the IDs above are examples, not production targets.

With `apply=1`, the script copies each selected legacy group's positive daily limit into its 5-hour limit and retains its weekly limit. Existing subscription usage/anchors are untouched; new short columns start NULL/0. The change is audited and repeated execution does not grant additional quota. Restart/invalidate auth and subscription caches after an approved production switch.

Deploy the dual-window-capable backend before switching policies. Deploy matching website/client builds before allowing users to consume dual resets. MUC and HUBU deploy independently: admin.wuxuexi.top uses /srv/sub2api, while hubu.wuxuexi.top uses /srv/sub2api-hubu, with separate databases, caches, brand configuration and desktop download manifests. Rollback must retain a dual-window-capable backend for any converted groups; do not silently revert them to daily/monthly enforcement.

## Limits of this change

Usage weights remain the existing Sub2API model pricing/settlement system. This reproduces the chosen short-plus-week product behavior, not OpenAI's private pricing algorithm. Admission does not reserve an unknown final token cost: concurrent accepted requests may exceed the limit on settlement, after which future requests are blocked.
