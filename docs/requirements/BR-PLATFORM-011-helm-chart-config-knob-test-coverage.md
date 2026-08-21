# BR-PLATFORM-011: Helm Chart Config-Knob Test-Coverage Completeness

**Business Requirement ID**: BR-PLATFORM-011
**Category**: Platform
**Priority**: P2
**Target Version**: V1.5
**Status**: Approved
**Date**: 2026-08-21

---

## Business Need

### Problem Statement

`charts/kubernaut/values.schema.json` declares 202 schema-defaulted/map leaf fields across the
chart's 11 config-bearing services. A cross-service consistency audit performed while fixing
Issues #2216-#2221 (see [PR #2225](https://github.com/jordigilh/kubernaut/pull/2225)) found that
**63 of those 202 fields have no `helm-unittest` assertion proving they are actually wired** —
i.e. that setting the field in `values.yaml` observably changes the rendered manifest. Of those
63, 22 were confirmed by direct template inspection to be genuinely correctly wired already (no
bug — see #2225's Issue #2219 for a counter-example where the same kind of audit *did* find a
real bug), simply untested. The remaining ~41 are pre-existing boilerplate
(`replicas`, `image.{repository,pullPolicy,tag}`, secret refs, autoscaling knobs, etc.) that is a
lower-priority, separately-tracked concern.

This is a **regression-detection gap**, not a bug in the current chart: nothing today prevents a
future change from silently breaking the wiring between a schema-declared config knob and its
template consumption — the schema would still validate `values.yaml` input successfully (BR-
PLATFORM-010's concern), but the value could be silently dropped on the floor before it ever
reaches the rendered manifest, with no test failing to catch it.

A naive coverage heuristic ("does the leaf name appear anywhere in `tests/*.yaml` text") was
tried and rejected during triage as unreliable in both directions:

- **False positive (doc comment)**: a test file's descriptive comment mentioning a field name
  (e.g. "...readTimeout/writeTimeout are schema-driven...") gets counted as coverage even though
  no assertion in that file ever exercises the field.
- **False positive (common word)**: `remediationorchestrator.config.timeouts.global` matches the
  unrelated word "global" appearing in every test file's `global.fleet...` override block.
- **False negative risk**: identically-named leaves across services (`readTimeout` exists under
  both `datastorage.config.server` and `gateway.config.server`) cannot be disambiguated by a
  service-unaware text search, risking either under- or over-counting depending on match order.

**Impact**: without a structural, per-service coverage gate, wiring regressions in schema-driven
config knobs can land and merge undetected — the same class of gap that allowed #2219's real bug
(a config field silently ignored in favor of a hardcoded literal) to go unnoticed until a manual
audit was performed.

---

## Business Objective

Build a Go coverage-gap checker that structurally parses `values.schema.json` and
`charts/kubernaut/tests/*.yaml`, computes per-service test coverage for every schema-defaulted
leaf field, and wire it into CI as a merge gate — so that any future schema-driven config field
added without a corresponding wiring test fails CI, and any existing untested-but-correctly-wired
field is either backfilled with a real test or explicitly, visibly allowlisted as a known,
lower-priority gap.

### Success Criteria

1. A new tool (`hack/check-helm-coverage`) computes coverage by parsing test files as structured
   YAML (not raw text), extracting only real assertion content (excluding `it:`/`suite:`
   descriptions, `set:` override keys, and comments), and scoping each assertion to its own
   suite's rendered service — eliminating all three false-positive/negative classes identified
   above.
2. The tool runs as a new step in the existing `helm-unittest` CI job
   (`.github/workflows/ci-pipeline.yml`) and fails the build when a schema-defaulted field is
   uncovered and not present in a seeded allowlist.
3. `charts/kubernaut/.helm-coverage-allowlist.yaml` is seeded with the ~41 pre-existing boilerplate
   gaps identified during triage, so the gate is green on merge and only prevents *new*,
   unreviewed coverage regressions going forward.
4. The 22 fields confirmed genuinely wired-but-untested are backfilled with real `helm-unittest`
   assertions (default value + at least one explicit-override-renders-through case each), across
   `gateway` (2 fields), `datastorage` (3 fields), and `remediationorchestrator` (17 fields — its
   first-ever chart test file).
5. `helm unittest charts/kubernaut/`, `make check-helm-coverage`, `go build ./...`, and
   `golangci-lint run` all pass cleanly after the change.

---

## Functional Requirements

- **FR-1 (Structural coverage extraction)**: `hack/check-helm-coverage/main.go` reuses
  `hack/internal/helmschema` to walk `values.schema.json` into per-service defaultable leaf paths
  (including map-typed nodes, e.g. `mcp.toolTimeouts`, as their own checkable unit distinct from
  their named sub-properties), and parses every `charts/kubernaut/tests/*.yaml` suite with
  `yaml.v3`, resolving each suite's `templates:` entries to an owning service and collecting only
  the string content found under `tests[].asserts[]` into that service's assertion corpus.
- **FR-2 (Whole-word, service-scoped matching)**: a schema leaf is marked covered only if its bare
  key name appears as a whole-word match within its *own service's* assertion corpus — not the
  whole test suite's raw text, and not comments or suite/it descriptions.
- **FR-3 (Allowlist mechanism)**: an uncovered field that is present in
  `charts/kubernaut/.helm-coverage-allowlist.yaml` does not fail the gate, allowing lower-priority,
  already-known gaps to be tracked explicitly rather than silently ignored or force-fixed all at
  once. A `-write-baseline` flag regenerates the allowlist from the current gap set for future
  re-baselining.
- **FR-4 (CI enforcement)**: `make check-helm-coverage` runs as a new step in the `helm-unittest`
  CI job, immediately after the existing `generate-helm-config-docs`/`generate-helm-defaults`
  freshness-check steps, following the same "run tool, fail with an actionable `::error::` message"
  pattern.
- **FR-5 (Backfill, no wiring changes)**: the 22 fields confirmed already correctly wired
  (`gateway.config.server.{readTimeout,writeTimeout}`;
  `datastorage.config.database.{connMaxLifetime,maxIdleConns,maxOpenConns}`;
  `remediationorchestrator.config.{timeouts,routing,effectivenessAssessment,asyncPropagation,
  notifications,retention}` sub-fields) receive new `helm-unittest` assertions only — no template
  or schema changes, since these are test-coverage gaps, not bugs.

---

## Non-Goals

- Does not attempt to reach 100% coverage of all 202 fields in this PR — the ~41 pre-existing
  boilerplate gaps (`replicas`, `image.*`, secret refs, autoscaling knobs, CORS/rate-limit/
  retention knobs not yet in scope, etc.) are deliberately seeded into the allowlist as a known,
  lower-priority backlog rather than blocking this gate's introduction.
- Does not change any chart template or schema wiring — this BR is exclusively about
  *detecting and closing test-coverage gaps* for already-correct wiring, complementing
  BR-PLATFORM-010 (which hardens the schema's *input-validation* contract) rather than overlapping
  it.
- Does not replicate or replace `helm lint --strict` or the existing `generate-helm-defaults`/
  `generate-helm-config-docs` freshness checks; it is a new, independent gate in the same CI job.
- Does not extend coverage checking to the Kubernaut Operator's Helm chart (out of scope; tracked
  separately if a parity gap is later identified there).

---

## FedRAMP / NIST 800-53 Control Mapping

| Control | Requirement Satisfied |
|---|---|
| **SI-10** (Information Input Validation) | FR-1–FR-5: proves — via an enforced, structural CI gate rather than incidental test presence — that every schema-declared, defaulted config input is not just accepted (BR-PLATFORM-010's concern) but actually observably takes effect through the chart's rendering path, and that this property cannot silently regress. |
| **CM-6** (Configuration Settings) | FR-3/FR-4: makes the chart's config-knob test-coverage state an explicit, machine-readable, version-controlled artifact (`.helm-coverage-allowlist.yaml`) rather than an unknown or manually-tracked property. |

---

## Related Decisions

- **Tracked in**: [Issue #2226](https://github.com/jordigilh/kubernaut/issues/2226) (Helm
  config-knob test-coverage gate + 22-field backfill).
- **Follows from**: Issues #2216-#2221 /
  [PR #2225](https://github.com/jordigilh/kubernaut/pull/2225), whose cross-service audit
  surfaced this coverage gap (and, separately, a real wiring bug in #2219 — the motivating example
  for why this class of gap matters).
- **Complements**: [BR-PLATFORM-010](BR-PLATFORM-010-helm-chart-schema-level-input-validation.md)
  — BR-PLATFORM-010 hardens what the schema *accepts*; this BR proves what the chart *does* with
  what it accepts, and keeps that proof from silently rotting.
- **Reusable component**: `hack/internal/helmschema` (`ParseSchema`, `ResolveNode`, `IsMap`,
  `IsObjectWithProperties`, `HasDefault`, `SortedKeys`), already shared by `hack/gen-helm-defaults`
  and `hack/gen-helm-config-docs`; this tool is a third consumer.

---

**Document Status**: ✅ Approved
**Priority**: P2 — closes a regression-detection gap (SI-10/CM-6) in the chart's config-knob
wiring, complementing BR-PLATFORM-010's input-validation hardening
