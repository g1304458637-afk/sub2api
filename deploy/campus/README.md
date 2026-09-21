# Campus local deployments

One backend/website implementation; canonical IDs are `muc` and `hubu`.
Build website and backend with the same brand. `Dockerfile --build-arg BRAND=hubu`
selects the HUBU website. Runtime `BRAND=hubu` selects the matching API routes,
connect-code namespace, public metadata and education policy. An unknown brand
fails closed; mismatched website/backend metadata is rejected.

Copy each example outside the repository, generate separate database/admin/JWT
secrets, and protect each private file with mode 0600. Example local commands:

```sh
docker compose -p campus-muc --env-file /private/tmp/muc.env -f deploy/campus/compose.local.yml up --build
docker compose -p campus-hubu --env-file /private/tmp/hubu.env -f deploy/campus/compose.local.yml up --build
```

Compose project names isolate networks and all four named volumes. Only the
backend binds a loopback host port; neither DB nor Redis is published. Keep
project names, ports, secrets, database and download volumes distinct. Payments,
provider accounts and education verification are unconfigured by default; do not
copy production configuration or credentials into these local environments.
Feature switches use the existing shared settings table independently in each DB.

For source development, run the backend with these environment values and host
DB/Redis connection values, then run the website with `BRAND=muc` (or `hubu`) and
`VITE_DEV_PROXY_TARGET` pointing to that backend. Both use the exact same 292
migration files. No brand-specific schema, billing, reset or media code exists.

Desktop uses the same canonical `OPENCODE_CHANNEL`/`BRAND`, plus its existing
`<BRAND>_GATEWAY_URL`, `_WEBSITE_URL` (the brand download page), `_MANIFEST_URL`,
`_UPDATE_FEED_URL`, `_DOWNLOAD_BASE_URL`. Set `CAMPUS_LOCAL_BUILD=1` for loopback
HTTP. HUBU has no official site/feed yet; release builds require explicit URLs.
MUC retains its legacy defaults. Never infer a feed from saved credentials.

API paths are `/api/v1/{brand}/connect-code` and `/api/v1/{brand}/exchange`.
Code payloads carry `brand` and `audience={brand}:desktop`; Redis stores a hash of
codes with a 60-second lifetime. Old in-flight codes without identity are rejected
once during this upgrade: request a fresh code. Existing MUC API keys are retained.
The historical `/v1/muc/reset-with-card/:id` is a shared Desktop transport contract
for both brands, not a separate MUC billing implementation.

MUC education policy remains `muc.edu.cn`. HUBU requires an explicit operator
policy before enabling verification; a test domain is not an official policy.
All payment, quota, wallet, reward, research and media flags retain their shared
Core implementations and separate per-deployment settings.

This template is for local validation, not production release or DNS changes.
