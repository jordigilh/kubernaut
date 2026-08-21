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

**Correction (2026-08-21, tool implementation)**: the 202/63/41/22 figures above came from a
narrower, ad-hoc preflight spike (Python, grep-based, scoped to the 11 services' `config:`
subtrees only). The finished tool, built exactly per this BR's own FR-1 spec ("same walk shape as
`gen-helm-defaults`'s `walkDefaults`" — i.e. every top-level schema property's full tree, not just
`config:` subtrees), found **356 total leaves / 265 real gaps** once run against the actual
schema. A full manual spot-check of every one of the 66 gaps a moderate `$v.<path>`/`.Values.<path>`
pattern search couldn't confirm (not just a naive grep — actual template/helper reads) found 265
of the 266 originally-counted gaps are genuinely wired (via `_helpers.tpl` macro indirection:
`kubernaut.scheduling`, `kubernaut.additionalClusterRoleBindings`, merged-`$v` per-pseudo-service
patterns for `tls`/`postgresql`/`valkey`/`networkPolicies`/etc.) — confirming the "boilerplate,
just untested" characterization holds at the larger scale too. One field,
`datastorage.config.redis.tls.insecureSkipVerify`, was found to be genuinely dead (no template
wiring, no Go struct field, and contradictory to `redis_tls_test.go`'s existing SC-8 guarantee that
TLS verification is never skippable) and was removed from the schema rather than allowlisted. The
seeded allowlist (`charts/kubernaut/.helm-coverage-allowlist.yaml`) therefore has 243 entries
(265 gaps − 22 backfilled in this PR), not the originally-estimated ~41.

**Addendum (2026-08-21, [PR #2225 review](https://github.com/jordigilh/kubernaut/pull/2225#issuecomment-5371062661))**:
a related but distinct failure mode was flagged during that PR's review. For a config knob that
carries a real Go-side default in addition to its schema-declared one
(`apifrontend.config.mcp.sessionIdleTimeout`/`toolTimeout`/`toolTimeouts`), the same conceptual
value now exists in two independently-authored places — `pkg/apifrontend/config/config.go`'s
`DefaultConfig()` (Go literal) and `values.schema.json`'s `"default"` (JSON literal) — kept in
sync only by `make generate-helm-defaults` plus human code review, with no automated cross-check.
This is structurally the same drift class that caused
[kubernaut-operator#374](https://github.com/jordigilh/kubernaut-operator/pull/374): a
hand-maintained copy of this exact `ToolTimeouts` map silently fell 2-of-4 entries out of date
when AF's own binary default gained two new tools. Unlike #374, deleting the schema-side copy is
not the right fix here — issue #2221 deliberately made these fields user-overridable via
`values.yaml`, so the schema's own default must exist for `kubernaut.mergedValues` to materialize
something when the user doesn't override it. The right fix is detection, not deletion: FR-6 below.

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
3. `charts/kubernaut/.helm-coverage-allowlist.yaml` is seeded with the 243 pre-existing,
   verified-genuinely-wired-but-untested gaps identified during triage (see Problem Statement
   correction above), so the gate is green on merge (once the 22-field backfill lands) and only
   prevents *new*, unreviewed coverage regressions going forward.
4. The 22 fields confirmed genuinely wired-but-untested are backfilled with real `helm-unittest`
   assertions (default value + at least one explicit-override-renders-through case each), across
   `gateway` (2 fields), `datastorage` (3 fields), and `remediationorchestrator` (17 fields — its
   first-ever chart test file).
5. `helm unittest charts/kubernaut/`, `make check-helm-coverage`, `go build ./...`, and
   `golangci-lint run` all pass cleanly after the change.
6. For the config-knob fields that carry a real Go-side default in addition to a schema-declared
   one (`apifrontend.config.mcp.{sessionIdleTimeout,toolTimeout,toolTimeouts.*}`), a generic,
   table-driven test proves `config.DefaultConfig()`'s Go value and `values.schema.json`'s declared
   `"default"` are identical, so a future edit to only one side fails CI instead of silently
   drifting (see Problem Statement addendum below).

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
- **FR-5a (Dead-field removal, unplanned corollary finding)**:
  `datastorage.config.redis.tls.insecureSkipVerify` was removed from `values.schema.json` (and its
  materialized default/docs regenerated) after verification found it had no template wiring, no
  corresponding Go struct field, and directly contradicted `pkg/datastorage/redis_tls_test.go`'s
  existing `UT-DS-1048-P5-064` assertion that TLS verification is never skippable (SC-8). This is
  the tool's own coverage-gate premise validating itself: a schema field that looks
  user-configurable but silently does nothing is exactly the failure class this BR exists to make
  impossible to introduce (or, as here, to leave undetected) going forward.
- **FR-6 (Go-vs-schema default drift detection)**: a table-driven Ginkgo test
  (`pkg/apifrontend/config/schema_default_drift_test.go`, `TC-P2C-04`) reads the real
  `charts/kubernaut/values.schema.json`, resolves each dual-sourced MCP duration field's declared
  `"default"`, and asserts it is `time.Duration`-equal to `config.DefaultConfig()`'s corresponding
  Go value. This is deliberately a small, self-contained JSON-tree walk in the test file itself,
  not a reuse of `hack/internal/helmschema` — that package is Go-`internal`-scoped to `hack/` (its
  parent directory), so `pkg/` cannot import it, and blurring that boundary to serve one caller
  isn't warranted. Adding a future dual-sourced default only requires one new `Entry()` row, not a
  bespoke parsing/comparison test (the gap the single-field `TC-P2C-03d` had).

---

## Non-Goals

- Does not attempt to reach 100% coverage of all 356 fields in this PR — the 243 pre-existing
  boilerplate gaps (`replicas`, `image.*`, secret refs, autoscaling knobs, `nodeSelector`/`pdb`/
  `tolerations`, `postgresql.*`/`valkey.*`/`tls.*`/`networkPolicies.*`, CORS/rate-limit/retention
  knobs not yet in scope, etc.) are deliberately seeded into the allowlist as a known,
  lower-priority backlog rather than blocking this gate's introduction. Each was individually
  spot-checked against its actual template/helper wiring (not just grepped) before being
  allowlisted — see the Problem Statement correction above.
- Does not change any chart template or schema wiring — this BR is exclusively about
  *detecting and closing test-coverage gaps* for already-correct wiring, complementing
  BR-PLATFORM-010 (which hardens the schema's *input-validation* contract) rather than overlapping
  it.
- Does not replicate or replace `helm lint --strict` or the existing `generate-helm-defaults`/
  `generate-helm-config-docs` freshness checks; it is a new, independent gate in the same CI job.
- Does not extend coverage checking to the Kubernaut Operator's Helm chart (out of scope; tracked
  separately if a parity gap is later identified there).
- FR-6's drift detection is deliberately scoped to the three `apifrontend.config.mcp.*` fields
  concretely identified as dual-sourced during PR #2225's review, not a fully generic
  reflection-based comparison across every service's `DefaultConfig()` struct against the schema.
  Several services' config structs don't follow a uniform pattern amenable to that today (e.g.
  `pkg/datastorage/config` has no `DefaultConfig()` at all — its timeout fields are plain strings
  with defaults, where they exist, scattered across `validate*()` methods) — generalizing further
  is a separate, larger effort, not a byproduct of this fast-follow.

---

## FedRAMP / NIST 800-53 Control Mapping

| Control | Requirement Satisfied |
|---|---|
| **SI-10** (Information Input Validation) | FR-1–FR-5: proves — via an enforced, structural CI gate rather than incidental test presence — that every schema-declared, defaulted config input is not just accepted (BR-PLATFORM-010's concern) but actually observably takes effect through the chart's rendering path, and that this property cannot silently regress. |
| **CM-6** (Configuration Settings) | FR-3/FR-4: makes the chart's config-knob test-coverage state an explicit, machine-readable, version-controlled artifact (`.helm-coverage-allowlist.yaml`) rather than an unknown or manually-tracked property. FR-6: proves the two independently-authored copies of a baseline configuration setting (Go default, schema default) remain a single logical value, not two that can silently diverge. |

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
- **FR-6 precedent**: [kubernaut-operator#374](https://github.com/jordigilh/kubernaut-operator/pull/374)
  (the same `mcp.toolTimeouts` drift class, previously caused by an operator-side hand-maintained
  copy — fixed there by deleting the redundant copy, which isn't viable for the chart's
  deliberately user-overridable schema default, hence FR-6's detection-based approach instead).

---

**Document Status**: ✅ Approved
**Priority**: P2 — closes a regression-detection gap (SI-10/CM-6) in the chart's config-knob
wiring, complementing BR-PLATFORM-010's input-validation hardening
