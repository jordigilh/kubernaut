# BR-PLATFORM-007: Helm Chart Minimal-Configuration Onboarding

**Business Requirement ID**: BR-PLATFORM-007
**Category**: Platform
**Priority**: P2
**Target Version**: V1.5
**Status**: 🟡 Proposed
**Date**: 2026-07-25 (lean reconstruction 2026-07-29 — see Document Note below)

---

## Document Note

This document is a lean reconstruction of an earlier, more detailed version that was lost (never
committed to git, deleted during a rebase's untracked-file cleanup on 2026-07-29). No decision
below changed as a result of the rewrite — it is grounded in the surviving implementation plan
(`.cursor/plans/dd-platform-006_full_implementation_10d3769d.plan.md`) and this project's
conversation record, with the iterative alternatives/version history omitted for brevity.

---

## Business Need

### Problem Statement

`charts/kubernaut/values.yaml` exposes 404 configurable fields (post-PR #1755) as a single flat
example file. A new user deploying Kubernaut to a vanilla Kubernetes cluster cannot tell, from the
file alone, which handful of fields they must actually set versus which ~397 already have a safe,
working default. This inflates perceived complexity, slows first-deployment time, and increases
support burden — directly working against fast time-to-value for a new install.

Separately, several `enabled`-style toggles gate security- or stability-relevant controls
(NetworkPolicy, audit-log tamper-evidence, JWT replay protection, rate limiting, transport
encryption) that this project already claims as enforced controls (FedRAMP AC-4/SC-8/AU-9,
`AGENTS.md`), yet ship as a silent, self-service opt-out with no compensating benefit once the
underlying capability has no real reason to be disabled.

### Impact

- New users cannot distinguish required fields from safely-defaulted ones without reading the
  chart's templates directly.
- A self-service disable toggle exists for several controls this project already commits to
  enforcing, undermining that commitment.
- Local ("vanilla Kubernetes") and Fleet-Federation topologies have no distinct minimal on-ramp —
  a Fleet user must still wade through every local-only field, and vice versa.

---

## Business Objective

Reduce the Helm chart's user-facing configuration surface to the minimum required for a working
deployment — on both the "local" (vanilla Kubernetes) and "Fleet Federation" topologies — without
losing any configuration capability for users who need it, and close the small number of
security/stability-relevant opt-out gaps identified during this effort.

### Success Criteria

1. The shipped `values.yaml` example shows only mandatory fields (fields with no safe default) and
   feature-enable toggles (fields that turn an optional capability on/off) — every other field
   keeps its schema-declared default and is documented in the README, not shown in the example.
2. NetworkPolicy, and four security-relevant toggles (rate limiting, JWT replay-cache, audit-log
   HMAC hash-chaining, DataStorage↔Valkey transport encryption), render unconditionally, removing
   the self-service opt-out for each once its operational prerequisites exist.
3. A dedicated `values-fleet.yaml` overlay gives Fleet Federation its own minimal on-ramp, distinct
   from the local-topology default.
4. Zero capability loss for every field not covered by Criterion 2's deliberate exceptions — every
   trimmed field remains settable via `--set` or an overlay file.
5. No backward-compatibility guarantee is required (pre-GA); the bar is "a fresh `helm install`
   produces a valid, working deployment," not "renders identically to before."

---

## Functional Requirements

- **FR-1**: Consolidate the 12 services' identical `pdb.{enabled,maxUnavailable}` blocks into a
  single `global.podDefaults.pdb` default, overridable per service.
- **FR-2**: Add `global.defaultResources` as a schema-declared fallback-of-last-resort (does not
  remove any of the 14 services' existing, intentionally distinct `resources` blocks).
- **FR-3**: Remove the global `networkPolicies.enabled` toggle and all 14 per-service
  `networkPolicies.<service>.enabled` toggles — NetworkPolicy becomes an unconditional resource.
- **FR-4**: Remove two dead/non-configurable schema fields: `tls.interService.enabled` (never read
  by any template — inter-service TLS is unconditionally mandatory via #753/#1683) and
  `gateway.config.cors.allowedMethods` (hardcoded 1:1 to the REST route surface).
- **FR-5**: Make four security-relevant toggles unconditional, gated on their prerequisite
  capabilities (FR-6/FR-7) where needed: `datastorage.config.server.rateLimit.enabled`,
  `apifrontend.config.auth.replayCache.enabled` (no new prerequisite), `datastorage.config.
  auditHashKey.enabled` (needs FR-6), `datastorage.config.redis.tls.enabled` (needs FR-7).
- **FR-6**: Auto-generate the audit-HMAC key Secret via a `pre-install`/`pre-upgrade` hook
  (extending the existing `tls-cert-job.yaml` pattern), exists-only idempotency (never rotates),
  with `helm.sh/resource-policy: keep` so the key survives a `helm uninstall`/`helm install` cycle
  and preserves AU-9 hash-chain continuity.
- **FR-7**: Add native server-side TLS to the in-chart Valkey Deployment (extending the same
  hook's per-service cert loop), disabling the plaintext port, with TLS-aware `readinessProbe`/
  `livenessProbe`.
- **FR-8**: Trim `values.yaml` to mandatory fields + feature-enable toggles; document every
  defaulted field in the README's configuration reference instead.
- **FR-9**: Default `kubernautAgent.llmProfileRef` to `"primary"` instead of an unconditional
  `fail()`, matching the universal existing convention across this repo's own usage.
- **FR-10**: Derive `fleetmetadatacache.enabled` from `global.fleet.enabled` +
  `global.fleet.backend` instead of requiring a separate, independently-defaulted toggle; fail
  loudly only on an explicit, contradictory override.
- **FR-11**: Require `apifrontend.enabled=true` whenever `kubernautAgent.interactive.enabled=true`
  (APIFrontend is currently the sole consumer of KubernautAgent's interactive MCP endpoint), and
  whenever `console.enabled=true` (Console's nginx sidecar has no other backend).
- **FR-12**: Add a `values-fleet.yaml` overlay covering Fleet-Federation-specific fields.

---

## Non-Goals

- Does not require backward compatibility with pre-GA deployments — a reinstall
  (`helm uninstall` + `helm install`), not an in-place `helm upgrade`, is the accepted upgrade path
  for any breaking default/shape change this BR introduces.
- Does not remove `postgresql`/`valkey` from the chart — both remain in-chart, enabled-by-default
  convenience deployments with an existing BYO escape hatch (`enabled: false` + `.host`); removing
  them entirely was considered and declined (see DD-PLATFORM-006).
- Does not change `console.ingress.enabled`'s default — that default is owned by BR-PLATFORM-009
  (Gateway/APIFrontend Ingress Parity), reviewed and deferred to, see DD-PLATFORM-006 Decision
  Area 9.
- Does not change Operator (`kubernaut-operator`) code or CRDs — chart-only.

---

## Related Decisions

- **Implemented by**: DD-PLATFORM-006 (Helm Chart Configuration Surface Reduction) — the full
  design and PR-by-PR implementation plan for every Functional Requirement above.
- **Interacts with**: BR-PLATFORM-009 (Helm Chart Gateway/APIFrontend Ingress Parity) — see
  DD-PLATFORM-006 Decision Area 9.
- **Tracked in**: [Issue #1743](https://github.com/jordigilh/kubernaut/issues/1743).

---

**Document Status**: 🟡 Proposed
**Priority**: P2
