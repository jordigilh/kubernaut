# BR-AUDIT-010: Keyed Audit Hash Chain (HMAC-SHA256)

**Business Requirement ID**: BR-AUDIT-010
**Category**: Data Storage Service — Security & Compliance
**Priority**: **P1 (HIGH)** — Closes a HIGH-severity GA readiness gap
**Target Version**: **V1.5**
**Status**: ✅ Implemented (DataStorage) / 🔜 Tracked separately for `kubernaut-operator` (v1.6)
**Date**: June 30, 2026
**Related ADRs**: [ADR-034: Unified Audit Table Design](../architecture/decisions/ADR-034-unified-audit-table-design.md)
**Related BRs**: BR-AUDIT-005 (Core Audit Business Requirement), BR-AUDIT-007 (Tamper-evident audit exports)
**GitHub Issues**: [#1505](https://github.com/jordigilh/kubernaut/issues/1505) (GAP-05)

---

## Business Need

### Problem Statement

DataStorage's `audit_events` table implements a blockchain-style hash chain (SOC2 Gap #9): each row's `event_hash` is `SHA256(previous_event_hash + event_json)`, so tampering with any row breaks the chain for every subsequent event. Before this BR, that hash was **unkeyed**: anyone with read/write access to the PostgreSQL database — including a DBA, a compromised service account with `UPDATE` on `audit_events`, or an attacker who gained database credentials — could tamper with a row and then recompute a self-consistent SHA256 chain for the rows that follow, defeating the tamper-evidence guarantee entirely.

This was identified as **GAP-05 (HIGH severity)** in the GA Readiness Audit (issue #1505): "the hash chain protects against accidental corruption but not against a database-privileged attacker."

### Impact Without This BR

- **FedRAMP AU-9** (Protection of Audit Information) is only partially satisfied: the chain detects *unintentional* corruption but not a deliberate, DB-privileged forgery.
- **SOC2 CC8.1** audit reconstruction cannot fully rely on `event_hash`/`previous_event_hash` alone as proof against a database-level compromise — a necessary caveat for any compliance narrative built on this chain.

---

## Decision: Keyed HMAC-SHA256, Opt-In and Backward Compatible

New audit events are hashed with **HMAC-SHA256** using a key stored **outside the database** (a Kubernetes Secret mounted into the DataStorage pod), instead of plain SHA256. Forging a valid HMAC without that key is computationally infeasible, even for an attacker with full database read/write access — closing the gap that made the previous unkeyed chain forgeable by a DB-privileged actor.

Key design choices:

1. **Per-row `hash_algorithm` column** (migration `013_audit_hash_algorithm.sql`): every row records which algorithm produced its `event_hash` (`sha256-unkeyed` or `hmac-sha256`). This supports a **mixed-algorithm chain**: existing rows keep verifying under the legacy algorithm; only new writes made after the HMAC key is provisioned use `hmac-sha256`. There is no retroactive re-hashing of historical data.
2. **Backward-compatible, opt-in**: when no HMAC key is configured (the default), DataStorage continues using the legacy unkeyed SHA256 algorithm — existing deployments upgrade with zero required action. Operators enable the stronger algorithm by pre-creating a Secret and setting `datastorage.config.auditHashKey.enabled=true` (Helm) — see `charts/kubernaut/README.md`.
3. **Excluded from the hash payload**: `hash_algorithm` itself is *not* included in the JSON that gets hashed (`PrepareEventForHashing` zeroes it, the same treatment as `EventHash`/`PreviousEventHash`/`EventDate`). This keeps pre-GAP-05 hashes verifiable byte-for-byte against their original JSON representation, which never contained this field.
4. **Shared verification logic**: `repository.CalculateHashForVerification(hmacKey, previousHash, event)` is the single source of truth for algorithm-aware verification, used by both the `/api/v1/audit/verify-chain` HTTP handler and the `Export` (compliance export) path. Verifying an `hmac-sha256` event without a configured key fails loudly with an explicit error rather than silently reporting a false "tampered" mismatch.
5. **No chart-managed secret generation**: consistent with the existing `postgresql-secret` / `valkey-secret` convention (`charts/kubernaut/templates/infrastructure/secrets.yaml`), the chart does **not** auto-generate the HMAC key. When `auditHashKey.enabled=true`, Helm validates the Secret exists via `lookup` and fails with an actionable `kubectl create secret` command if missing.

The equivalent wiring for `kubernaut-operator`-managed deployments (which construct their own `DataStorageConfigMap`/`DataStorageDeployment` independently of this Helm chart) is tracked separately: [kubernaut-operator#209](https://github.com/jordigilh/kubernaut-operator/issues/209), milestone v1.6.

### Known Residual Risk (Documented, Not Closed by This BR)

The per-row `hash_algorithm` column is itself ordinary database data. A sufficiently privileged attacker who can rewrite arbitrary rows could, in principle, also rewrite `hash_algorithm` from `hmac-sha256` back to `sha256-unkeyed` and recompute a plain SHA256 hash from scratch (a "downgrade" forgery) — the same fundamental class of risk as an attacker rewriting `event_hash`/`previous_event_hash` directly, which is not blocked by the immutability trigger (migration `012_audit_event_immutability.sql`) because doing so would break existing tamper-*detection* tests that deliberately rely on being able to `UPDATE event_hash` to simulate an attacker (see `test/integration/datastorage/audit_export_integration_test.go`, `test/e2e/datastorage/05_soc2_compliance_test.go`).

Closing this residual risk would require an external, out-of-band anchor (e.g., periodically publishing chain-tip hashes to an external immutable log, or WORM storage) — out of scope for this BR. HMAC-SHA256 still meaningfully raises the bar: it defeats the *straightforward* SHA256-recompute-and-forge attack the unkeyed algorithm was vulnerable to, requiring instead a more complex attack that also depends on hiding evidence of the downgrade itself.

---

## Success Criteria

1. `AuditEventsRepository.Create`/`CreateBatch` stamp `hash_algorithm` and select `hmac-sha256` when an HMAC key is configured (`WithHMACKey`), else `sha256-unkeyed` (backward-compatible default).
2. `repository.CalculateHashForVerification` is algorithm-aware, used identically by `Export` and the verify-chain HTTP handler.
3. `hmac-sha256` verification without a configured key fails with an explicit error, not a false "tampered" result.
4. Pre-GAP-05 events (empty `hash_algorithm`) continue to verify identically to `sha256-unkeyed` events.
5. Helm: `datastorage.config.auditHashKey.enabled=true` wires the ConfigMap (`audit.hashKeySecretsFile`/`hashKeyKey`), mounts the pre-created Secret, and validates its existence at install/upgrade time. Disabled by default.
6. FedRAMP/SOC2 control mapping: **AU-9** (Protection of Audit Information) is materially strengthened for post-rollout events; **CC8.1** reconstruction retains a documented, bounded caveat for the residual downgrade risk above.

---

## Related Documents

- [ADR-034: Unified Audit Table Design](../architecture/decisions/ADR-034-unified-audit-table-design.md)
- [Kubernaut Helm Chart README — Optional: Keyed Audit Hash Chain](../../charts/kubernaut/README.md#optional-keyed-audit-hash-chain-gap-05)

---

**Document Version**: 1.0
**Last Updated**: June 30, 2026
**Maintained By**: Kubernaut Architecture Team
