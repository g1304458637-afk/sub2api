# Legacy production migration baseline

An old application image can have `DEPLOY_COMMIT=unknown`. Do not replace that
field with a guessed Git commit. Use the deployment dispatch input
`legacy_migration_baseline` only when a historical commit's complete migration
filename/checksum set matches the live `schema_migrations` table exactly.

The verifier rejects missing, extra, duplicate or changed entries and invalid
commit identifiers. SSH/SQL failures stop the workflow. The usual pending SQL
audit still runs. The baseline is migration provenance only, not application
provenance. The actual old image digest and live ledger digest are checked again
before deployment, and the old application commit remains `unknown` in rollback
metadata. A fresh root-only DB/config backup is created before application state
changes. Prior independently restored backups remain available.

For the September 2026 legacy MUC upgrade, baseline
`9a0d68d1670a08fd0e04af951858ffae74424a0a` matched all 290 live checksums. This is
evidence for selecting the input, not a hardcoded exemption; each run verifies it
again. Future releases use the actual canonical deployed commit automatically.
HUBU has no production deployment.

The generated legacy digests and migration-baseline SHA travel in the same
workflow run's `migration-proof` artifact, not job outputs (GitHub can suppress
arbitrary hashes as potential secrets). A separate `build-metadata` artifact
preserves the image build timestamp. These artifacts contain no database rows,
environment files or credentials. Deployment validates their formats and compares
legacy digests to fresh server observations before changing app state.

Validation: `python3 -m unittest discover -s scripts -p test_verify_migration_ledger.py`.
