# Test Plan: apifrontend Helm chart interactive-timeout defaults must match Go code defaults

**Issue**: [#1803](https://github.com/jordigilh/kubernaut/issues/1803)
**Authority**: DD-PLATFORM-006 Decision Area 14 (schema-materialized Helm defaults, `kubernaut.mergedValues`)
**Business Requirements**: None directly (operational-correctness bug fix; see Section 2)
**Branch**: `fix/1803-apifrontend-interactive-timeout-defaults`
**Created**: 2026-07-31
**Status**: Active

---

## 1. Purpose

`charts/kubernaut/values.schema.json` declares schema defaults of `"10s"` /
`"15s"` for `apifrontend.config.interactive.awaitSessionTimeout` /
`bridgeInactivityTimeout`. These are meant to merely *document* the Go
package defaults they mirror
(`pkg/apifrontend/tools/crd_tools_session.go:AwaitSessionTimeout` = `3 *
time.Minute`, `pkg/apifrontend/tools/ka_investigate_bridge.go:BridgeInactivityTimeout`
= `180 * time.Second`), but were populated with short, test-suite-style
values instead.

Because `values.yaml` never sets these two fields (they only appear as a
**commented-out** documentation example), `kubernaut.mergedValues`
(`_helpers.tpl`) falls through to the schema defaults on every real
`helm install`/`upgrade` — silently shipping the 10s/15s values to 100% of
chart-based deployments instead of the safe 3m/180s Go defaults.

Impact (confirmed in the issue): AF's interactive SSE bridge
(`bridgeEventsCollectSummary`) abandons an in-flight investigation/
workflow-selection turn after only 15s of inter-event silence from KA, well
short of real investigation round-trip times. Caught by the new
console-equivalent streaming E2E test tracked in #1799
(`E2E-FP-1189-005`), which stalled at the 15s mark.

Preflight confirmed (2026-07-31, against `origin/main`@`5828161c5`):

- `cmd/apifrontend/main.go:378-383` only overrides the Go package vars when
  `cfg.Interactive.{AwaitSessionTimeout,BridgeInactivityTimeout} > 0` — any
  non-zero chart value (including the wrong 10s/15s) always wins over the
  safe Go defaults.
- The issue's own "Fix" description says the template's `default` filter
  fallback needs correcting; the actual template
  (`charts/kubernaut/templates/apifrontend/apifrontend.yaml:93-96`) does
  **not** use a `| default` filter at all — it reads
  `$v.config.interactive.awaitSessionTimeout` directly, where `$v` already
  comes pre-merged from `kubernaut.mergedValues`. The real, single source of
  truth is `values.schema.json`'s two `"default"` fields; both
  `charts/kubernaut/templates/_generated_defaults.tpl` and
  `docs/generated/helm-values-reference.md` are auto-generated *from* that
  schema (`make generate-helm-defaults` / `make generate-helm-config-docs`)
  and must not be hand-edited — the CI drift checks
  (`.github/workflows/ci-pipeline.yml:306-324`) will fail if they are out of
  sync with the schema.
- `charts/kubernaut/README.md` contains no literal `"10s"`/`"15s"` values for
  these fields (only a cross-reference to the generated reference doc), so
  no direct edit is needed there, contrary to the issue text's literal
  wording.
- Confirmed empirically (`git show origin/release/v1.5:charts/kubernaut/templates/apifrontend/apifrontend.yaml`)
  that `release/v1.5` has zero references to `interactive` — the issue's "no
  backport needed" claim is correct.

This is a **configuration-default correction** (AGENTS.md "Configuration
changes" — no new components, no Go code changes, no architecture impact).
Per AGENTS.md's complexity table this qualifies for Standard TDD Only
(skip the full Preflight → Spikes → Confidence Score → Readiness Audit
workflow), but the preflight above was still performed per explicit request
and because it corrected a misconception in the issue's own fix description.

## 2. FedRAMP/SOC2 Control Mapping

None of the actively-enforced controls in AGENTS.md (AU-2/AU-3/AU-9/AU-11,
AC-4/AC-6, SC-8, SI-10) directly apply — this change touches neither audit
event emission, access control, transmission confidentiality, nor input
validation. It is an operational-reliability default-value correction for a
session-timeout config. No control mapping is force-fitted here per the
"never invent placeholder/compliance claims" convention.

## 3. Test Scenario Inventory (Helm chart tier — `make test-helm`)

No Go UT/IT is added or needed: `Config.DefaultConfig()`,
`cmd/apifrontend/main.go`'s override guard, and the two `tools` package
default vars are all already correct and unchanged by this fix (confirmed in
Section 1). The bug is entirely in the Helm chart's schema-declared default
value, so the Pyramid Invariant is satisfied by the chart's own dedicated
unit-test tier (`charts/kubernaut/tests/*.yaml`, run via `make test-helm`),
which is the established coverage mechanism for this class of bug in this
codebase (see `apifrontend_default_override_bugs_test.yaml`,
`kubernaut_mergedvalues_test.yaml`).

| ID | Tier | Business-Level Behavior Description | Test File |
|---|---|---|---|
| CHART-1803-001 | Helm unit | Default install (no `interactive.*` override) renders `awaitSessionTimeout: "3m"` and `bridgeInactivityTimeout: "180s"` in the `apifrontend-config` ConfigMap — matching the Go package defaults exactly | `apifrontend_interactive_timeouts_test.yaml` (new) |
| CHART-1803-002 | Helm unit (regression guard) | An explicit user override of either field (e.g. `awaitSessionTimeout: "45s"`) still renders the override, not the default — proves the fix doesn't regress `kubernaut.mergedValues`' override precedence | `apifrontend_interactive_timeouts_test.yaml` (new) |
| CHART-1803-003 | Helm unit (regression guard) | `interactive.enabled` true/false override behavior (pre-existing BUG 2 coverage) is unaffected by this fix | `apifrontend_default_override_bugs_test.yaml` (pre-existing, re-run) |
| CHART-1803-004 | Generator drift check | `make generate-helm-defaults` and `make generate-helm-config-docs` produce zero diff after the schema fix + regeneration (proves the two derived artifacts are back in sync) | CI-equivalent of `.github/workflows/ci-pipeline.yml:306-324`, run locally |
| CHART-1803-005 | Parity check | `make verify-helm-defaults-parity` passes (trimmed `values.yaml` still renders identically to the full-defaults tree) | `make verify-helm-defaults-parity` |

## 4. Why the Existing Suite Missed This

`apifrontend_default_override_bugs_test.yaml` (DD-PLATFORM-006 DA14, PR9)
already tests `interactive.enabled`'s true-defaulting boolean behavior in
detail (BUG 2), but never asserted on the two duration fields' rendered
*values* — only the boolean gets a regression guard. No test in the suite
ever pins `awaitSessionTimeout`/`bridgeInactivityTimeout` to a specific
string, which is how the wrong 10s/15s schema defaults shipped unnoticed
until a real E2E run (#1799) hit the 15s SSE-bridge timeout in practice.

## 5. Wiring Manifest (Helm-chart-specific analogue of CHECKPOINT W)

| Component | Production Entry Point | Wiring Code Location | Proving Test |
|---|---|---|---|
| Schema default value | `values.schema.json` (hand-maintained, single source of truth) | `apifrontend.config.interactive.{awaitSessionTimeout,bridgeInactivityTimeout}.default` | CHART-1803-001 |
| Materialized default tree | `make generate-helm-defaults` (`hack/gen-helm-defaults`) | `charts/kubernaut/templates/_generated_defaults.tpl` (generated, not hand-edited) | CHART-1803-004 |
| Merge with user overrides | `kubernaut.mergedValues` (`_helpers.tpl:1468`) | `templates/apifrontend/apifrontend.yaml:93-96` (`$v.config.interactive.*`) | CHART-1803-001/002 |
| Rendered ConfigMap → Go override guard | `cmd/apifrontend/main.go:378-383` | `if cfg.Interactive.AwaitSessionTimeout > 0 { tools.AwaitSessionTimeout = ... }` | Unchanged, pre-existing — not modified by this fix |
| Generated docs | `make generate-helm-config-docs` (`hack/gen-helm-config-docs`) | `docs/generated/helm-values-reference.md` (generated, not hand-edited) | CHART-1803-004 |

No new production components are introduced. This manifest documents the
pre-existing wiring whose *value* was wrong, not its *structure*.

## 6. Test Data / Fixtures

Reuses the exact `set:` fixture block already established in sibling
`apifrontend/*` helm-unittest files (`postgresql.auth.existingSecret`,
`valkey.existingSecret`, `global.llmProfiles.primary`,
`kubernautAgent.llmProfileRef`, `apifrontend.config.auth.issuerURL`) — no new
fixtures needed.

## 7. Out of Scope (tracked separately)

- The console-equivalent streaming E2E test itself (`E2E-FP-1189-005`,
  issue #1799) — this plan fixes the chart default it caught; it does not
  modify that test.
- `release/v1.5` backport — confirmed not applicable (Section 1); no action.

## 8. Coverage Target

Helm chart unit-test tier only (`make test-helm`), per AGENTS.md's Quick
Reference (`make test-helm` is the canonical tier for
`charts/kubernaut/tests/`). No Go UT/IT/E2E tier applies — no Go production
logic changed.
