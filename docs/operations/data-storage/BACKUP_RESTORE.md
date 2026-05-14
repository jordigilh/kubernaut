# Data Storage: Backup and Restore (PROD-L2)

Operational guidance for the Data Storage service backing store and related infrastructure.

## PostgreSQL (audit and catalog data)

Use native PostgreSQL tooling. Replace connection flags, credentials, paths, and object names with values from your environment (Secret, RDS instance, Helm values).

### Full logical backup (`pg_dump` custom format)

```bash
# Full cluster or single-database dump (custom format, parallel restore friendly)
pg_dump "postgresql://${PGUSER}:${PGPASSWORD}@${PGHOST}:${PGPORT}/${PGDATABASE}?sslmode=require" \
  --format=custom \
  --file "/backup/kubernaut-ds-${DATE}.dump" \
  --verbose
```

Restore with `pg_restore` into a clean database (adjust `--jobs` for host capacity):

```bash
pg_restore \
  --dbname="postgresql://${PGUSER}:${PGPASSWORD}@${PGHOST_RESTORE}:${PGPORT}/${PGDATABASE_NEW}?sslmode=require" \
  --verbose \
  --jobs=4 \
  --no-owner \
  "/backup/kubernaut-ds-${DATE}.dump"
```

### Incremental strategy

PostgreSQL does not ship incremental logical backups natively like file-level incrementals.

- **Rolling full + WAL archiving (recommended where RPO \< 24h)**  
  - Enable **continuous archiving** (`archive_mode` / `archive_command`) and retain **WAL**.  
  - Combine periodic **full base backups** (`pg_basebackup`) with WAL segments for **point-in-time recovery (PITR)**.  
  - Restore by recovering a base backup and replaying WAL to the desired timestamp (`recovery_target_time`).

- **Logical “incremental” via smaller dumps**  
  - Narrow `pg_dump` with `--schema=...` / table lists for large-but-rare schemas, alongside regular full dumps.  
  This is **not** a substitute for WAL-based PITR for RPO guarantees.

Operational teams should encode RPO/RTO in runbooks using **WAL archiving + base backup** unless a managed service provides automated PITR (e.g. RDS automated backups).

### Redis DLQ

The Redis-backed **dead-letter queue is ephemeral**. Failed audit writes are transient until replayed into PostgreSQL; there is **no expectation of DLQ backup**. **Replay and durability** come from the database and DLQ retry/drain semantics (DD-009, DD-008).

## Kubernetes-level backup

Use **Velero** (or equivalent) for namespaces, Secrets, ConfigMaps, and Helm release state—not as a substitute for PostgreSQL logical/PITR strategy, but to recover cluster configuration alongside DB restores:

- Backup: scheduled Velero backups of the namespace(s) hosting Data Storage, PostgreSQL operators, ingress, OAuth proxy, etc.
- Restore: coordinated restore—cluster objects first or in lockstep with database restore procedures per environment.

Keep this combined with PostgreSQL dumps or managed-DB snapshots so application data and cluster config stay aligned.
