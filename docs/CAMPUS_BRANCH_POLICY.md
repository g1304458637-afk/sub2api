+# Campus branch policy

The long-lived product branches are:

- `muc-main` for MUC production and release candidates.
- `hubu-main` for HUBU build, test, local, and staging work until HUBU has an approved production environment.

Both branches are cut from the same reviewed Shared Core commit. Product differences belong in BrandConfig, deployment configuration, and isolated release metadata.

Short-lived work must use `feat/*`, `fix/*`, `chore/*`, or `release/*`, then be merged and deleted. Do not recreate `subscription-v1`, `backend-complete`, `muc-harness`, `final-frontend`, `status-contract`, `auto-update`, or `hubu-local` as long-lived branches.

Production deployment requires an exact 40-character Git SHA. MUC production accepts only `muc-main`. HUBU has no production deployment path until its official gateway, update feed, domain, signing policy, and environment are approved.

