# BR-PLATFORM-010: Helm Chart Schema-Level Input Validation (+ ADR-068 OAuth2 Enforcement)

**Business Requirement ID**: BR-PLATFORM-010
**Category**: Platform
**Priority**: P1
**Target Version**: V1.5
**Status**: Approved
**Date**: 2026-08-06

---

## Business Need

### Problem Statement

The Kubernaut Helm chart (`charts/kubernaut/`) enforces most of its cross-field and
mandatory-field constraints at **template-render time**, via `{{- fail "..." }}` guards scattered
across 12 template files (19 call sites, per the full-inventory `git grep "{{- fail"` triage
performed for Issue #1984). This has two consequences:

1. **Late failure detection**: `fail()` only fires during `helm template`/`helm install`, after
   Helm has already parsed and started rendering the chart — later than a `values.schema.json`
   rejection, which fails before any template logic runs. `helm lint --strict` and CI tooling that
   validates values files against the schema (without a full template render) cannot catch these
   violations at all.
2. **No machine-readable contract**: `fail()` guards are unstructured template logic, not part of
   the chart's declared `values.schema.json` contract — external tooling (IDE schema validation,
   `helm-docs`, values-file linters) has no way to know these fields are conditionally mandatory.

A follow-up triage while scoping this hardening (Issue #1984) additionally discovered an
**architectural gap**: `global.fleet.oauth2.enabled` defaults to `false` for 7 of the 8
fleet-capable services (Gateway, RemediationOrchestrator, APIFrontend, EffectivenessMonitor,
SignalProcessing, WorkflowExecution, KubernautAgent), permitting those services to call the MCP
Gateway with **zero authentication**. This contradicts
[ADR-068 (Fleet Federation Architecture)](../architecture/decisions/ADR-068-fleet-federation-architecture.md),
which mandates OAuth2 for every service-to-MCP-Gateway connection with no unauthenticated
fallback. FleetMetadataCache (FMC) already implements the correct, unconditional pattern
(`pkg/fleet/fmc/config/config.go:100-109,161-186`); the other 7 services do not. Filed and tracked
as [Issue #1991](https://github.com/jordigilh/kubernaut/issues/1991), fixed in this same PR per
scope decision.

**Impact**:

- Schema-level gaps: invalid `values.yaml` configurations (missing mandatory policy content,
  missing Fleet MCP Gateway endpoints, inconsistent cross-component toggles) are only caught deep
  into `helm template`/`helm install`, increasing time-to-detect for operators and CI pipelines
  that lint/validate values without a full render.
- OAuth2 gap: a cluster operator who enables Fleet + the MCP Gateway for 7 of the 8 services,
  without also explicitly setting `global.fleet.oauth2.enabled=true`, silently deploys an
  **unauthenticated** MCP Gateway client — a direct violation of the "OAuth2 mandatory, no
  fallback" boundary documented in ADR-068 (Goal, line 22; Boundary 1 auth table, line 614+; line
  792).

---

## Business Objective

Move mandatory-ness and cross-field enforcement from template-level `fail()` guards to
`values.schema.json` wherever a JSON Schema (draft-07) construct (`anyOf`, `if`/`then`) can express
the same constraint, so that violations are caught before template rendering begins. Close the
ADR-068 OAuth2 gap by making `global.fleet.oauth2.enabled=true` (+ `tokenURL` +
`credentialsSecretRef`) schema- and Go-`Validate()`-mandatory whenever a service's own MCP Gateway
endpoint field is set, mirroring FMC's existing correct pattern.

### Success Criteria

1. Every `fail()` guard in `charts/kubernaut/templates/**/*.yaml` and `_helpers.tpl` that
   validates **raw input** (not a post-`default`/post-lookup derived value) is replaced by an
   equivalent `values.schema.json` constraint, and removed from the template.
2. `global.fleet.oauth2.enabled` becomes mandatory-`true` (with `tokenURL` and
   `credentialsSecretRef` non-empty) whenever the consuming service's MCP-Gateway-endpoint field is
   non-empty, enforced at both the schema level (`values.schema.json`) and the Go level
   (`Validate()` in `pkg/fleet/config.go`, `pkg/signalprocessing/config/config.go`,
   `pkg/workflowexecution/config/config.go`, `internal/kubernautagent/config/config.go`), mirroring
   `pkg/fleet/fmc/config/config.go`.
3. Zero behavior change for any legitimate existing configuration: every real usage site in the
   repository (E2E harness, example values files, CI schema-consistency job) already satisfies the
   new constraints (verified during preflight; the one live-cluster E2E harness that installs the
   chart with Fleet + a real MCP Gateway already sets `oauth2.enabled=true` unconditionally).
4. `helm lint --strict` and `helm template` (zero-override defaults) both remain clean; all
   existing and new `helm-unittest` cases pass.

---

## Functional Requirements

- **FR-1 (Policy content)**: `aianalysis.policies.{content,existingConfigMap}` and
  `signalprocessing.policies.{content,existingConfigMap}` become schema-enforced via `anyOf`
  (exactly one of the two must be non-empty), replacing the 2 `fail()` guards in
  `aianalysis.yaml:3` and `signalprocessing.yaml:10,13,18` (policies portion).
- **FR-2 (Fleet MCP Gateway endpoint)**: `global.fleet.mcpGatewayEndpoint` becomes schema-required
  via `if`/`then` whenever Gateway/RemediationOrchestrator are fleet-enabled or
  FleetMetadataCache is effectively enabled, replacing the 3 associated `fail()` guards
  (`gateway.yaml`, `remediationorchestrator.yaml`, `fleetmetadatacache.yaml`).
- **FR-3 (Cross-component dependencies + console leaf fields)**: schema `if`/`then` blocks enforce:
  `console.enabled` requires `apifrontend.enabled`; `console.auth.secretName`, the OIDC issuer, and
  `console.ingress.host` become schema-required leaf fields when console is enabled;
  `kubernautAgent.interactive.enabled` requires `apifrontend.enabled`; and
  `fleetmetadatacache.enabled=false` conflicting with a configured `global.fleet.backend` is
  schema-rejected — replacing the 6 associated `fail()` guards across `console.yaml`,
  `kubernaut-agent.yaml:24`, and `_helpers.tpl:472`.
- **FR-4 (OAuth2 mandatory-when-MCP-Gateway-used, schema)**: `global.fleet.oauth2.{enabled,
  tokenURL, credentialsSecretRef}` become schema-required (via `if`/`then`, `const: true` on
  `enabled`) whenever `global.fleet.mcpGatewayEndpoint` is non-empty (shared block covering
  GW/RO/AF/EM/SP/KA/FMC), plus a dedicated no-fallback `if`/`then` for WorkflowExecution's own
  `credentialsSecretRef` field — replacing all 8 services' oauth2 `fail()` guard pairs.
- **FR-5 (OAuth2 mandatory-when-MCP-Gateway-used, Go defense-in-depth)**: `Validate()` in
  `pkg/fleet/config.go` (covers GW/RO/AF/EM via the shared `FleetConfig`),
  `pkg/signalprocessing/config/config.go`, `pkg/workflowexecution/config/config.go`, and
  `internal/kubernautagent/config/config.go` reject `OAuth2.Enabled == false` once the respective
  service's own MCP-Gateway-endpoint field is non-empty, mirroring
  `pkg/fleet/fmc/config/config.go`'s existing unconditional requirement. Gate condition is the
  service's own endpoint field, **not** `global.fleet.enabled` alone — a GW/RO deployment may
  enable Fleet for scope-checking only (Backend + Endpoint against ACM/FMC) without ever calling
  the MCP Gateway, in which case OAuth2 is not applicable.
- **FR-6 (Test suite updates)**: existing `helm-unittest` cases that assert on the exact `fail()`
  `errorMessage:` string for a guard removed by FR-1–FR-4 are rewritten to assert
  `failedTemplate: {}` (Helm's generic schema-validation failure) instead. `helm-unittest` happy-path
  cases whose `set:` blocks would newly fail schema validation under FR-4 (7 cases in
  `fleet_config_consolidation_test.yaml`) gain the now-mandatory `oauth2` fields to remain passing.

---

## Non-Goals

- Does not move guards that validate **post-`default`/post-lookup derived values** to schema — JSON
  Schema validates raw `values.yaml` input before Helm's template `default`/sprig-merge/lookup
  logic runs, so these are inherently unrepresentable in schema and remain as `fail()` guards:
  TLS `certManager.issuerRef.name` auto-discovery (`_helpers.tpl:62,66,69`), monitoring URL
  auto-discovery (`_helpers.tpl:803,806`), `networkPolicies.apiServerCIDR` auto-discovery
  (`_helpers.tpl:1036`), `llmProfileRef` referential integrity (cross-key map lookup,
  `_helpers.tpl:1372`), KubernautAgent phase-name enum against a computed valid-phases list
  (`kubernaut-agent.yaml:50,277`), per-`llmProfile` OAuth2 (`kubernaut-agent.yaml:516,519` — same
  shape as FR-4 but out of scope: no ADR-068 gap exists for LLM profiles), and any live-cluster
  lookup guard (`infrastructure/secrets.yaml`, `infrastructure/singleinstallguard.yaml`).
- Does not attempt to fully replicate the Go-side `credentialsSecretRef` global-fallback resolution
  in schema (`kubernaut.fleet.oauth2` helper). The schema instead requires
  `global.fleet.oauth2.credentialsSecretRef` unconditionally whenever the MCP Gateway endpoint is
  set, treating per-service overrides as additive refinement — stricter than, and a superset of,
  the status quo. This is a deliberate, explicitly-flagged design simplification, not a functional
  gap (WorkflowExecution keeps its existing dedicated no-fallback requirement unchanged).
- Does not change the Kubernaut Operator's equivalent validation (out of scope; tracked separately
  if a parity gap is later identified there).

---

## FedRAMP / NIST 800-53 Control Mapping

| Control | Requirement Satisfied |
|---|---|
| **SI-10** (Information Input Validation) | FR-1–FR-4: chart input (`values.yaml`) is validated against a declared schema before any template logic executes, rejecting malformed/incomplete configuration at the earliest possible point. |
| **CM-6** (Configuration Settings) | FR-1–FR-4: mandatory and conditionally-mandatory configuration settings are declared in a single machine-readable artifact (`values.schema.json`) rather than scattered imperative guards, making the chart's configuration contract auditable. |
| **IA-2** (Identification and Authentication) | FR-4/FR-5: closes the ADR-068 gap — every service-to-MCP-Gateway connection is now authenticated (OAuth2) by construction, with no configuration path that silently disables authentication while the connection remains active. |
| **AC-3** (Access Enforcement) | FR-4/FR-5: the MCP Gateway cannot be reached by a fleet-capable service without a valid OAuth2 credential reference being configured — access enforcement is a precondition of the connection existing at all, not a runtime-optional feature. |
| **AC-17** (Remote Access) | FR-4/FR-5: service-to-MCP-Gateway is a remote (cross-namespace/cross-cluster) access path; mandatory OAuth2 ensures this remote access path is always authenticated, consistent with ADR-068's Boundary 1 threat model. |

---

## Related Decisions

- **Tracked in**: [Issue #1984](https://github.com/jordigilh/kubernaut/issues/1984) (Helm chart
  schema-level input validation), [Issue #1991](https://github.com/jordigilh/kubernaut/issues/1991)
  (ADR-068 `oauth2.enabled=false` gap, folded into the same PR).
- **Authoritative source for FR-4/FR-5**:
  [ADR-068 (Fleet Federation Architecture)](../architecture/decisions/ADR-068-fleet-federation-architecture.md)
  — Goal (line 22), Boundary 1 auth table (line 614+), line 792 ("FMC → MCP Gateway: OAuth2
  (mandatory) — there is no unauthenticated fallback").
- **Reference pattern**: `pkg/fleet/fmc/config/config.go`,
  `charts/kubernaut/templates/fleetmetadatacache/fleetmetadatacache.yaml` — FMC already implements
  FR-4/FR-5's target end state unconditionally.

---

**Document Status**: ✅ Approved
**Priority**: P1 — hardens input validation (SI-10/CM-6) and closes an authentication-bypass gap
(IA-2/AC-3/AC-17) relative to the chart's own architectural mandate (ADR-068)
