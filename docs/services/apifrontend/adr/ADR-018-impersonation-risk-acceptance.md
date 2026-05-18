# ADR-018: Impersonation Privilege Scope and Risk Acceptance

**Status:** Accepted
**Date:** 2026-05-05
**Deciders:** AF team
**NIST Controls:** AC-6, AC-6(1), AU-2

## Context

The AF ServiceAccount requires Kubernetes `impersonate` verb on `users` and `groups`
resources to perform user-scoped queries during triage (e.g., listing pods visible to
the authenticated user). The current `ClusterRole` grants impersonation without
`resourceNames` restrictions, meaning the AF SA can impersonate any user or group in
the cluster.

This was flagged as a FedRAMP AC-6 (Least Privilege) concern in the PR#78 review
(RBAC-1). The three remediation options considered:

- **Option A**: `resourceNames` restriction — too rigid; impersonatable identities are
  dynamic and vary per cluster deployment.
- **Option B**: Admission webhook — significant implementation effort; deferred to a
  follow-up issue.
- **Option C**: Risk acceptance with compensating controls — immediate, documented.

## Decision

We accept Option C: the AF ClusterRole retains unrestricted impersonation, with the
following compensating controls:

### Compensating Controls

1. **Audit trail (AU-2)**: Every impersonation call emits an `impersonation.created`
   audit event (via `audit.Emitter`) containing the source user, target identity, and
   request ID. These events flow to the SOC2-compatible audit trail.

2. **Network isolation**: The AF pod runs in a dedicated namespace with `NetworkPolicy`
   restricting egress to the K8s API server and KA endpoints only. Lateral movement
   from a compromised AF pod is constrained.

3. **Pod security**: The AF deployment uses a restricted SecurityContextConstraint (SCC)
   with `readOnlyRootFilesystem`, `runAsNonRoot`, and no privilege escalation.

4. **Runtime scope**: Impersonation is only exercised within the `ClientFactory` for
   user-scoped triage queries (read-only GET/LIST on pods, events, logs). The AF SA's
   own identity is used for write operations (CRD creation, RR creation).

5. **Monitoring**: Alert on `af_audit_events_total{event_type="impersonation.created"}`
   anomalies (rate spikes, unusual target identities).

## Risk Assessment

| Risk | Likelihood | Impact | Residual Risk |
|------|-----------|--------|---------------|
| Compromised AF pod impersonates cluster-admin | Very Low | Critical | Low — requires pod compromise AND knowledge of admin identities; mitigated by network isolation, audit alerting, and SCC constraints |
| AF impersonates unintended user during triage | Low | Medium | Low — impersonation target comes from validated JWT claims, not user input |
| Audit gap hides impersonation abuse | Very Low | High | Very Low — all impersonation calls emit audit events; audit pipeline has at-least-once delivery |

## Follow-up

- **Issue #XX**: Implement Option B (admission webhook) for runtime impersonation
  validation if FedRAMP assessor requires additional controls beyond risk acceptance.

## NIST 800-53 Mapping

- **AC-6**: Least privilege is addressed via compensating controls rather than
  `resourceNames` restriction.
- **AC-6(1)**: Privileged function (impersonation) is audited and monitored.
- **AU-2**: Impersonation events are included in the audit event catalog.
