# ADR-007: CRD Spec Immutability Enforcement

**Status:** Accepted
**Date:** 2026-05-03
**Deciders:** AF team
**Source:** #56 design

## Context

InvestigationSession spec fields (remediationRequestRef, a2aTaskID, userIdentity, joinMode) must not change after creation. Mutation would break session identity, enable session hijacking, and corrupt audit trails.

## Decision

Enforce spec immutability via:
- **Production (OCP):** ValidatingAdmissionPolicy with CEL expression `oldObject.spec == object.spec`, deployed by kubernaut-operator
- **Development:** Convention only (no admission webhook; tests verify immutability via integration tests)

## Consequences

- Production has cryptographic enforcement (K8s admission control)
- Development has zero infrastructure overhead (no webhook server to run)
- ValidatingAdmissionPolicy requires K8s 1.30+ (OCP 4.17+) — acceptable for v1.5 target
- CEL-based policy is declarative (no custom webhook code to maintain)
- If target cluster is older than 1.30, fallback to convention + integration tests

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| Custom ValidatingWebhook | Requires running a webhook server; operational complexity for a simple equality check |
| Controller-side rejection | Race condition: controller may not reconcile before next read; not immediate |
| Convention only (everywhere) | No enforcement in production; security risk (malicious actor could mutate spec) |
| CEL in CRD schema (x-kubernetes-validations) | CRD-level CEL cannot compare oldObject; only per-field validation |
