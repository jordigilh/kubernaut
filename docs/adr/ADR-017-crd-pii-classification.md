# ADR-017: InvestigationSession CRD PII Classification and Data Retention

**Status:** Accepted
**Date:** 2026-05-04
**Deciders:** AF team
**NIST Controls:** AC-4, AU-11, DM-1, DM-2, SI-12

## Context

The `InvestigationSession` CRD persists session metadata to etcd via the Kubernetes API.
In a FedRAMP environment, all data stores containing Personally Identifiable Information
(PII) must have a documented classification, retention policy, and minimization strategy.

## PII Classification

| CRD Field | PII Category | Sensitivity | Justification |
|-----------|-------------|-------------|---------------|
| `spec.createdBy.username` | Direct identifier | Moderate | OIDC `sub` claim; maps to SSO identity |
| `spec.createdBy.groups` | Indirect identifier | Low | Role-based group names (e.g., `sre-team`) |
| `spec.joinMode` | Non-PII | None | Enum value (Creator, SharedLink, RBAC) |
| `spec.a2aTaskID` | Non-PII | None | Opaque task identifier for A2A protocol reconnection |
| `status.kaCorrelationID` | Non-PII | None | Opaque correlation token |
| `status.phase` | Non-PII | None | Enum state value |
| `status.message` | Potential PII | Low | May echo parts of user query in error messages |
| `metadata.labels` | Indirect identifier | Low | Contains username for label-selector filtering |

## Data Retention Policy

| Tier | Retention | Mechanism | Rationale |
|------|-----------|-----------|-----------|
| Active sessions | Indefinite while Active | No TTL | Required for session continuity |
| Completed/Failed/Cancelled | 30 days (default) | TTL controller (PR4) with `status.completedAt` + configurable TTL | FedRAMP AU-11: minimum 30-day audit retention |
| Archived sessions | 90 days | CRD soft-delete + DataStorage archival (PR7) | Extended retention for compliance review |

The TTL controller (implemented in PR4) garbage-collects terminal sessions based on
`status.completedAt` + a configurable duration (default: 30 days, override via
`--session-ttl` flag or `SESSION_TTL_DAYS` env var).

## Data Minimization (DM-1)

1. **Event data is NOT stored in the CRD.** LLM conversation events (tool calls,
   responses, intermediate reasoning) are held in-memory only and discarded on pod
   restart. This is by design (ADR-005) to avoid etcd bloat and to minimize the PII
   surface.

2. **The `spec.a2aTaskID` field** stores a task correlation identifier, not
   user-originated content. No free-text user query is persisted in the CRD spec.

3. **Username labels** use the OIDC `sub` claim (opaque identifier) rather than
   display names when available.

## Access Controls (AC-4)

- CRD access is scoped via Kubernetes RBAC (`config/rbac/role.yaml`).
- Only the AF service account can create/update/delete `InvestigationSession` resources.
- Users access session data exclusively through the AF API, which enforces:
  - JWT authentication (issuer verification, audience check, CEL claim mapping)
  - Owner-based filtering (users see only sessions they created or joined)
  - No direct etcd access is possible from application pods (OCP network policy)

## Encryption

- **At rest**: etcd encryption is managed by the OCP cluster operator (`EncryptionConfig`).
  AF does not implement application-level encryption of CRD fields.
- **In transit**: All K8s API communication uses mTLS (service account token + TLS).

## Consequences

- Operators MUST ensure etcd encryption is enabled in the OCP cluster configuration.
- The TTL controller MUST be deployed alongside AF to enforce retention policy.
- Compliance auditors can verify retention by querying:
  `kubectl get investigationsessions --field-selector status.phase=Completed -o json`
  and checking `status.completedAt` timestamps.
- Future work (PR7) will add DataStorage archival for long-term audit trail retention.
