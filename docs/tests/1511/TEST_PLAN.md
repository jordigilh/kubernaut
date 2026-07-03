# Test Plan: Issue #1511 — Fleet: Cluster-Scoped Workflow Targeting via SP Rego Classification

## 1. Test Plan Identifier

TP-FLEET-1511

## 2. References

- **Issue**: [#1511](https://github.com/jordigilh/kubernaut/issues/1511)
- **Business Requirement**: [BR-FLEET-003](../../requirements/BR-FLEET-003-cluster-scoped-workflow-targeting.md)
- **Design Decision**: [DD-FLEET-002](../../architecture/decisions/DD-FLEET-002-cluster-scoped-workflow-targeting.md) (Alternative 3, approved)
- **Related**: DD-FLEET-001, ADR-068, BR-FLEET-002, BR-FLEET-054
- **FedRAMP Controls**: AC-4, SC-7, SI-10, AU-3

## 3. Introduction

This test plan validates the introduction of an optional `cluster` classification
dimension, derived by SignalProcessing's (SP) Rego policy from MCP Gateway
cluster-registration labels, threaded end-to-end (SP → RemediationOrchestrator (RO) →
AIAnalysis (AA) → KubernautAgent (KA) → DataStorage (DS)) so that workflow discovery can
be restricted per cluster classification without breaking non-fleet deployments.

### Root Cause / Gap

`RemediationWorkflowLabels` supports `severity`/`environment`/`component`/`priority`
filtering but has no cluster dimension. Once fleet mode is enabled, all cataloged
workflows are eligible for selection regardless of originating cluster, including
workflows an operator intended to be single-cluster-only.

### Fix

Add a `cluster` classification computed by SP's existing Rego policy engine from
`ClusterInfo.Labels` (sourced from the MCP Gateway's `Backend`/`MCPServerRegistration`
CRD), and thread it through RO/AA/KA to DS as a new, optional, backward-compatible filter
dimension reusing the existing `labels JSONB` column.

## 4. Test Items

- `pkg/signalprocessing/evaluator/{types,evaluator}.go` — `PolicyInput.Cluster`, `EvaluateCluster()`
- `internal/controller/signalprocessing/signalprocessing_classifying.go` — `evaluateClusterOrSkip`, `Status.ClusterClassification` persistence
- `pkg/signalprocessing/enricher/k8s_enricher.go` — `ClusterRegistry` lookup, `KubernetesContext.Cluster` population
- `cmd/signalprocessing/main.go` — `ClusterRegistry` construction
- `pkg/remediationorchestrator/creator/aianalysis.go` — `buildSignalContext()` cluster propagation
- `internal/kubernautagent/tools/custom/tools.go` — discovery tool-call `cluster` param
- `pkg/datastorage/repository/workflow/discovery.go` — `appendMandatoryLabelConditions` cluster SQL branch
- `pkg/datastorage/server/workflow_discovery_handlers.go` — `ParseDiscoveryFilters` cluster param
- `pkg/workflowschema/converter.go`, `pkg/datastorage/schema/parser.go` — `Cluster` label round-trip

## 5. Features to Be Tested

| Feature | FedRAMP Control | Description |
|---|---|---|
| CRD `Cluster` label round-trip | AU-3 | `cluster` labels survive CRD → Authwebhook → DS unmodified |
| SP cluster label resolution | SI-10 | `ClusterRegistry.Get()` degrades gracefully on unregistered/missing cluster |
| SP Rego cluster classification | AC-4 | `EvaluateCluster()` produces classification from `input.cluster.labels`; non-fatal on error |
| SP status persistence | AU-3 | `Status.ClusterClassification` set via real reconcile loop |
| RO→AA→KA propagation | AU-3 | `ClusterClassification` traceable end-to-end through `SignalContextInput` and `katypes.SignalContext` |
| DS cluster filter dimension | AC-4 | Query includes `cluster` dimension with exact + `*` wildcard match; omitted when filter empty |
| Matching semantics | AC-4, SC-7 | Workflow with no `cluster` entries excluded once concrete filter active; `["*"]` matches any value |
| Backward compatibility | SC-7 | Non-fleet deployments produce zero behavioral change (no `cluster` param sent, no filter applied) |

## 6. Features Not to Be Tested

- Rego policy authoring UX / policy language features: covered by existing SP Rego test suites
- `ClusterRegistry` internals (EAIGW/Kuadrant CRD parsing): already covered by `pkg/fleet/registry` and FMC test suites; this plan only tests SP's *consumption* of `ClusterRegistry.Get()`
- FMC sync cadence / broker readiness: out of scope, covered by BR-FLEET-002 test suites
- Cluster taxonomy/label conventions operators choose: business policy, not implementation

## 7. Approach

### 7.1 Coverage Policy

Per `.cursor/rules/03-testing-strategy.mdc`: Unit ≥80% of unit-testable code (Rego
evaluator logic, SQL builder logic, filter parsing); Integration ≥80% of
integration-testable code (controller reconcile wiring, HTTP handler wiring, real
PostgreSQL queries); E2E covers the critical full-chain journey.

### 7.2 Pyramid Invariant Allocation

| Tier | What it proves | Tests |
|---|---|---|
| UT | CRD label extraction logic; `PolicyInput.Cluster` population; `EvaluateCluster()` classification logic; KA `SignalContext` param logic; DS SQL fragment generation and filter parsing | UT-DS-1511-004..007, UT-SP-1511-001, UT-SP-1511-002, UT-KA-1511-001 |
| IT | CRD round-trip through Authwebhook; SP reconcile loop produces `Status.ClusterClassification`; RO builds `SignalContext.Cluster`; KA passes `cluster` param through production dispatch; DS query executes against real PostgreSQL | IT-AW-1511-001, IT-SP-1511-001, IT-SP-1511-002, IT-RO-1511-001, IT-KA-1511-001, IT-DS-1511-001..003 |
| E2E | Full SP→RO→AA→KA→DS journey: excluded on mismatch, included on match, included when fleet disabled | E2E-FLEET-1511-001 |

### 7.3 Two-Tier Minimum

Every BR requirement below has both a UT (or IT, where the logic is inseparable from I/O,
e.g. CRD round-trip) and an IT covering it — see BR Coverage Matrix (§8).

### 7.4 Pass/Fail Criteria

**PASS**: All UT/IT/E2E tests below pass; per-tier coverage ≥80% on touched
files; zero regressions in existing SP/RO/KA/DS test suites; DS query latency for the
`cluster`-filtered path is within the same order of magnitude as `severity` (no new
index required, same query shape).

**FAIL**: Any test above fails; coverage drops below 80% on a touched tier; an
existing severity/environment/priority test regresses; a non-fleet deployment observes
any behavioral change (must be verified via a dedicated regression test, IT-DS-1511-003).

### 7.5 Suspension & Resumption

**Suspend** if: CRD manifest regeneration (`make manifests generate`) fails to produce a
mechanical diff (would indicate an unexpected `controller-gen` interaction); build breaks
after any phase's GREEN step.

**Resume** when: root cause identified and fixed; `go build ./...` green again.

### 7.6 Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT/E2E Test ID |
|---|---|---|---|
| `ClusterRegistry` construction | SP manager startup | `cmd/signalprocessing/main.go` | IT-SP-1511-001 |
| `K8sEnricher` cluster label lookup | `performK8sEnrichment` | `internal/controller/signalprocessing/signalprocessing_enriching.go` → `pkg/signalprocessing/enricher/k8s_enricher.go` | IT-SP-1511-001 |
| `PolicyEvaluator.EvaluateCluster()` | `evaluateClusterOrSkip` | `internal/controller/signalprocessing/signalprocessing_classifying.go` → `pkg/signalprocessing/evaluator/evaluator.go` | IT-SP-1511-002 |
| `Status.ClusterClassification` persistence | `finalizeClassification`'s `AtomicStatusUpdate` | `signalprocessing_classifying.go:148-170` | IT-SP-1511-002 |
| `SignalContextInput.Cluster` population | `buildSignalContext()` | `pkg/remediationorchestrator/creator/aianalysis.go` | IT-RO-1511-001 |
| `SignalContext.ClusterClassification` consumption | discovery tool-call builder | `internal/kubernautagent/tools/custom/tools.go` | IT-KA-1511-001 |
| DS `cluster` filter dimension | `buildContextFilterSQL` → `appendMandatoryLabelConditions` | `pkg/datastorage/repository/workflow/discovery.go:331-376` | IT-DS-1511-001..003 |
| Shared `pkg/fleet.MCPGatewayConfig` | FMC + SP config loaders | `pkg/fleet/config.go`, `pkg/fleet/fmc/config/config.go`, `pkg/signalprocessing/config/config.go` | Existing FMC IT suite + IT-SP-1511-001 |
| CRD `Cluster` round-trip | Authwebhook admission (unmodified, generic pass-through) | `pkg/datastorage/schema/parser.go: ExtractLabels` | IT-AW-1511-001 |
| Full chain | SP → RO → AA → KA → DS | n/a (cross-service) | E2E-FLEET-1511-001 |

## 8. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|---|---|---|---|---|---|
| BR-FLEET-003 R1 | SP resolves cluster labels via `ClusterRegistry`, degrades gracefully | P1 | Unit | UT-SP-1511-002 | Pending |
| BR-FLEET-003 R1 | SP resolves cluster labels via `ClusterRegistry`, degrades gracefully | P1 | Integration | IT-SP-1511-001 | Pending |
| BR-FLEET-003 R2 | Rego produces `cluster` classification; evaluation non-fatal on error | P1 | Unit | UT-SP-1511-001 | Pending |
| BR-FLEET-003 R2 | Rego produces `cluster` classification; evaluation non-fatal on error | P1 | Integration | IT-SP-1511-002 | Pending |
| BR-FLEET-003 R3 | `Status.ClusterClassification` persisted atomically | P1 | Integration | IT-SP-1511-002 | Pending |
| BR-FLEET-003 R4 | End-to-end propagation SP→RO→AA→KA | P1 | Integration | IT-RO-1511-001 | Pending |
| BR-FLEET-003 R4 | End-to-end propagation SP→RO→AA→KA | P1 | Unit | UT-KA-1511-001 | Pending |
| BR-FLEET-003 R4 | End-to-end propagation SP→RO→AA→KA | P1 | Integration | IT-KA-1511-001 | Pending |
| BR-FLEET-003 R7 | `WorkflowSchemaLabels`/`MandatoryLabels`/`RemediationWorkflowLabels` gain optional `Cluster []string` | P1 | Unit | UT-DS-1511-001 | Pending |
| BR-FLEET-003 R5 | `ExtractLabels` emits `cluster` key only when non-empty | P1 | Unit | UT-DS-1511-002 | Pending |
| BR-FLEET-003 R7 | `converter.go` `SpecToSchema`/`SchemaToSpec` preserve `Cluster` round-trip | P1 | Unit | UT-DS-1511-003 | Pending |
| BR-FLEET-003 R5 | DS `cluster` filter dimension, JSONB pattern | P1 | Unit | UT-DS-1511-004 | Pending |
| BR-FLEET-003 R5 | DS `cluster` filter dimension, JSONB pattern | P1 | Integration | IT-DS-1511-001 | Pending |
| BR-FLEET-003 R6 | Exclusion semantics: no `cluster` entry excluded once filter active | P1 | Unit | UT-DS-1511-005 | Pending |
| BR-FLEET-003 R6 | Exclusion semantics: no `cluster` entry excluded once filter active | P1 | Integration | IT-DS-1511-002 | Pending |
| BR-FLEET-003 R6 | Wildcard `cluster: ["*"]` matches any concrete filter value | P1 | Unit | UT-DS-1511-006 | Pending |
| BR-FLEET-003 R6 | No filter supplied → no cluster condition applied (backward compat) | P0 | Unit | UT-DS-1511-007 | Pending |
| BR-FLEET-003 R6 | No filter supplied → no cluster condition applied (backward compat) | P0 | Integration | IT-DS-1511-003 | Pending |
| BR-FLEET-003 R7 | `Cluster` field remains optional at schema level | P1 | Integration | IT-AW-1511-001 | Pending |
| BR-FLEET-003 (full chain) | Full SP→RO→AA→KA→DS journey | P1 | E2E | E2E-FLEET-1511-001 | Pending |

### Status Legend

Pending / RED / GREEN / REFACTORED / Pass

## 9. Test Cases

### UT-DS-1511-001: `Cluster []string` is a valid optional field on label structs (BR-FLEET-003 R7)

- **File**: `pkg/datastorage/models/workflow_labels_cnv_test.go` (or new file)
- **Given**: `WorkflowSchemaLabels{}` / `MandatoryLabels{}` / `RemediationWorkflowLabels{}` zero values
- **When**: `Cluster` is left unset
- **Then**: Struct is valid (no required-field validation error) — confirms `+optional` /
  no `min=1` validation tag

### UT-DS-1511-002: `ExtractLabels` emits `cluster` key only when non-empty (BR-FLEET-003 R5)

- **File**: `pkg/datastorage/oci_schema_extractor_test.go`
- **Given**: A parsed `WorkflowSchema` with `Labels.Cluster = ["production", "staging"]`
- **When**: `ExtractLabels()` is called
- **Then**: Resulting JSON contains `"cluster": ["production", "staging"]`
- **Also covers**: `Labels.Cluster` empty/nil → `cluster` key omitted entirely (mirrors
  `environment`/`component` omission behavior, not `severity`'s always-present behavior)

### UT-DS-1511-003: `converter.go` round-trips `Cluster` between CRD and DS schema (BR-FLEET-003 R7)

- **File**: `pkg/workflowschema/converter_test.go`
- **Given**: A `RemediationWorkflowSpec` with `Labels.Cluster = ["production"]`
- **When**: `SpecToSchema()` then `SchemaToSpec()` are applied
- **Then**: `Cluster` survives both conversions unchanged

### UT-SP-1511-001: `EvaluateCluster()` produces classification from cluster labels (AC-4)

- **File**: `pkg/signalprocessing/evaluator/evaluator_test.go`
- **Given**: A `PolicyInput` with `Cluster.Labels{"environment": "production"}` and a Rego
  policy rule `cluster := input.cluster.labels.environment`
- **When**: `EvaluateCluster(input)` is called
- **Then**: Returns `ClusterResult{Classification: "production"}`, no error
- **Also covers**: empty/nil `Cluster.Labels` → empty classification, no error (non-fatal);
  malformed Rego cluster rule → error surfaced to caller but does not panic

### UT-SP-1511-002: `K8sEnricher` populates `KubernetesContext.Cluster` via `ClusterRegistry` (SI-10)

- **File**: `pkg/signalprocessing/enricher/k8s_enricher_test.go`
- **Given**: A fake `ClusterRegistry` with a registered cluster ID mapped to `ClusterInfo{Labels: {...}}`
- **When**: `Enrich()` is called with a signal whose `ClusterID` matches
- **Then**: `KubernetesContext.Cluster.Labels` populated with the registry's labels
- **Also covers**: unregistered `ClusterID` (comma-ok `false`) → `KubernetesContext.Cluster`
  is nil, no error, enrichment proceeds normally (graceful degradation)

### IT-SP-1511-001: `ClusterRegistry` wired at SP startup and reachable through enrichment (SI-10)

- **File**: `test/integration/signalprocessing/sp_cluster_enrichment_test.go`
- **Given**: SP manager started with `cmd/signalprocessing/main.go` wiring, fleet enabled,
  a `ClusterRegistry` backed by a real (envtest) Backend CRD with labels
- **When**: A `SignalProcessing` CR is reconciled for a signal with a matching `ClusterID`
- **Then**: The reconcile loop's enrichment phase populates `KubernetesContext.Cluster`
  from the real registered CRD — proves `ClusterRegistry` is constructed and reachable
  from production wiring, not just unit-testable in isolation

### IT-SP-1511-002: `Status.ClusterClassification` set via real reconcile loop (AC-4, AU-3)

- **File**: `test/integration/signalprocessing/sp_cluster_classification_test.go`
- **Given**: A real Rego policy fixture with a `cluster` rule, SP reconciling a signal from
  a registered, labeled cluster
- **When**: The `SignalProcessing` CR completes classification
- **Then**: `Status.ClusterClassification` is set to the expected value
- **Also covers**: a Rego cluster-rule evaluation error does not set
  `Status.Phase = Failed` (non-fatal per R2) — severity errors, by contrast, do

### IT-RO-1511-001: `buildSignalContext()` propagates `ClusterClassification` into AIAnalysis (AU-3)

- **File**: `test/integration/remediationorchestrator/ro_cluster_propagation_test.go`
- **Given**: A `SignalProcessing` with `Status.ClusterClassification = "production"`
- **When**: RO creates the `AIAnalysis` CR via `buildSignalContext()`
- **Then**: `AIAnalysis.Spec.SignalContext.Cluster == "production"`

### UT-KA-1511-001: KA passes `cluster` param when `ClusterClassification` non-empty

- **File**: `internal/kubernautagent/tools/custom/tools_test.go`
- **Given**: `katypes.SignalContext.ClusterClassification = "production"`
- **When**: The discovery tool-call params are built
- **Then**: `cluster` param present with value `"production"`
- **Also covers**: empty `ClusterClassification` → `cluster` param omitted entirely (not
  sent as empty string — preserves backward compatibility per R6.1)

### IT-KA-1511-001: `cluster` param reaches DS discovery call through production dispatch (AU-3)

- **File**: `test/integration/kubernautagent/fleet/ka_cluster_discovery_test.go`
- **Given**: A KA agent processing a signal with `SignalContext.ClusterClassification` set
- **When**: The agent's workflow discovery tool executes against a real DS instance
- **Then**: The DS request includes the `cluster` query parameter with the expected value

### UT-DS-1511-004: SQL builder emits `cluster` JSONB condition when filter present (AC-4)

- **File**: `pkg/datastorage/repository/workflow/discovery_test.go`
- **Given**: `WorkflowDiscoveryFilters{Cluster: "production"}`
- **When**: `appendMandatoryLabelConditions` builds the SQL fragment
- **Then**: Fragment matches `severity`/`environment` shape:
  `EXISTS(...jsonb_array_elements_text(labels->'cluster')... WHERE LOWER(elem)=LOWER($N)) OR labels->'cluster' ? '*'`

### UT-DS-1511-005: Exclusion semantics — workflow with no `cluster` labels excluded once filter active (AC-4, SC-7)

- **File**: `pkg/datastorage/repository/workflow/discovery_test.go`
- **Given**: `Filters.Cluster = "production"`, a workflow row with no `cluster` key in `labels`
- **When**: The generated SQL condition is evaluated against the row
- **Then**: The workflow is excluded (condition evaluates false — no wildcard present)

### UT-DS-1511-006: Wildcard `cluster: ["*"]` matches any concrete filter value (AC-4)

- **File**: `pkg/datastorage/repository/workflow/discovery_test.go`
- **Given**: `Filters.Cluster = "staging-eu"`, a workflow row with `labels->'cluster' = ["*"]`
- **When**: The generated SQL condition is evaluated
- **Then**: The workflow matches (wildcard branch)

### UT-DS-1511-007: No `cluster` filter supplied → condition omitted entirely (SC-7, backward compat)

- **File**: `pkg/datastorage/repository/workflow/discovery_test.go`
- **Given**: `Filters.Cluster = ""` (empty, not sent by KA)
- **When**: `appendMandatoryLabelConditions` builds the SQL fragment
- **Then**: No `cluster` clause is added to the query at all — identical query shape to
  pre-#1511 behavior

### IT-DS-1511-001: `cluster` filter executes against real PostgreSQL, exact match (AC-4)

- **File**: `test/integration/datastorage/workflow_discovery_cluster_test.go`
- **Given**: Real PostgreSQL with catalog rows labeled `cluster: ["production"]` and
  `cluster: ["staging"]`
- **When**: Discovery API called with `cluster=production`
- **Then**: Only the `production`-labeled workflow is returned

### IT-DS-1511-002: `cluster` filter excludes unlabeled workflows once active (SC-7)

- **File**: `test/integration/datastorage/workflow_discovery_cluster_test.go`
- **Given**: Real PostgreSQL with a workflow row that has no `cluster` label at all
- **When**: Discovery API called with `cluster=production`
- **Then**: The unlabeled workflow is NOT returned

### IT-DS-1511-003: No `cluster` param → identical result set to pre-#1511 behavior (SC-7, regression)

- **File**: `test/integration/datastorage/workflow_discovery_cluster_test.go`
- **Given**: Real PostgreSQL with a mix of labeled and unlabeled workflows
- **When**: Discovery API called with no `cluster` param (simulating non-fleet deployment)
- **Then**: All workflows matching other filters are returned, regardless of `cluster`
  label presence — zero behavioral change from pre-#1511

### IT-AW-1511-001: CRD `cluster` labels round-trip through unmodified Authwebhook (AU-3)

- **File**: `test/integration/authwebhook/aw_workflow_labels_test.go`
- **Given**: A `RemediationWorkflow` CRD with `labels.cluster: ["production", "staging"]`
- **When**: The CRD is submitted through Authwebhook's existing generic label-forwarding
  admission path (no AW code changes) to DS
- **Then**: DS's `ExtractLabels` captures `cluster` in the stored `labels JSONB` identically
  to how it captures `severity`/`environment` today

### E2E-FLEET-1511-001: Full chain — exclude/include by cluster classification

- **File**: `test/e2e/fleet/cluster_scoped_workflow_targeting_test.go`
- **Given**: A fleet-enabled deployment with a registered, labeled cluster; two workflows,
  one labeled `cluster: ["production"]`, one with no `cluster` label
- **When**: A signal originates from a cluster whose Rego classification is `"production"`
- **Then**: Only the `production`-labeled workflow is discoverable
- **And when**: Fleet is disabled (no cluster registry)
- **Then**: Both workflows are discoverable (no `cluster` filter applied at all)

## 10. Environmental Needs

- **Unit**: Ginkgo/Gomega BDD, no I/O, `pkg/signalprocessing/evaluator`,
  `pkg/signalprocessing/enricher`, `pkg/datastorage/repository/workflow`,
  `internal/kubernautagent/tools/custom`
- **Integration**: Ginkgo/Gomega BDD, ZERO mocks, real PostgreSQL (DS), envtest (SP/RO
  controllers), real KA agent dispatch against a real DS instance
- **E2E**: Kind cluster, full service set (SP, RO, AA, KA, DS), Mock LLM

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — this issue is self-contained and does not depend on other open issues.

### 11.2 Execution Order

1. **Phase 1 (RED→GREEN→REFACTOR)**: CRD schema + label round-trip (IT-AW-1511-001)
2. **Phase 2 (RED→GREEN→REFACTOR)**: Shared fleet config + SP cluster enrichment (IT-SP-1511-001)
3. **Phase 3 (RED→GREEN→REFACTOR)**: SP Rego classification (UT-SP-1511-001/002, IT-SP-1511-002)
4. **Phase 4 (RED→GREEN→REFACTOR)**: RO→AA→KA propagation (IT-RO-1511-001, UT/IT-KA-1511-001)
5. **Phase 5 (RED→GREEN→REFACTOR)**: DS filter dimension (UT-DS-1511-004..007, IT-DS-1511-001..003)
6. **Phase 6**: E2E-FLEET-1511-001 + formal Wiring Audit

## 12. Test Deliverables

| Deliverable | Location |
|---|---|
| This test plan | `docs/tests/1511/TEST_PLAN.md` |
| BR requirement doc | `docs/requirements/BR-FLEET-003-cluster-scoped-workflow-targeting.md` |
| Unit test suites | `pkg/signalprocessing/evaluator/`, `pkg/signalprocessing/enricher/`, `pkg/datastorage/repository/workflow/`, `internal/kubernautagent/tools/custom/` |
| Integration test suites | `test/integration/signalprocessing/`, `test/integration/remediationorchestrator/`, `test/integration/kubernautagent/fleet/`, `test/integration/datastorage/`, `test/integration/authwebhook/` |
| E2E test suite | `test/e2e/fleet/` |

## 13. Execution

```bash
# Unit tests
go test ./pkg/signalprocessing/... ./pkg/datastorage/repository/workflow/... ./internal/kubernautagent/tools/custom/... -ginkgo.v

# Integration tests
make test-integration-signalprocessing
make test-integration-remediationorchestrator
make test-integration-kubernautagent
make test-integration-datastorage
make test-integration-authwebhook

# E2E
make test-e2e-fleet

# Specific test by ID
go test ./pkg/signalprocessing/evaluator/... -ginkgo.focus="UT-SP-1511-001"
```

## 14. Wiring Verification (TDD Phase 4)

See §7.6 Wiring Manifest above. Formal Wiring Audit Protocol execution occurs in Phase 6
of the implementation plan, after all Wiring Manifest rows have a passing IT/E2E.

## 15. Existing Tests Requiring Updates

None expected — `cluster` is purely additive to `RemediationWorkflowLabels`,
`PolicyInput`, `SignalContextInput`, `katypes.SignalContext`, and `WorkflowDiscoveryFilters`.
IT-DS-1511-003 exists specifically to guard against unintended behavioral change to
existing severity/environment/priority filtering.

## 16. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-07-03 | Initial test plan |
