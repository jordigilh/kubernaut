# Test Plan — #1378: Extend DetectedLabels with CNV/KubeVirt Fields

**IEEE 829 Compliant** | **Issue**: [#1378](https://github.com/jordigilh/kubernaut/issues/1378) | **Milestone**: v1.5

## 1. Test Plan Identifier

TP-1378-CNV-DETECTED-LABELS

## 2. Introduction

Extend the `DetectedLabels` struct with 4 new fields (`virtualMachine`, `liveMigratable`, `cdiManaged`, `storageBackend`) for OpenShift Virtualization (CNV/KubeVirt) workload detection. These fields are populated deterministically during enrichment — before the LLM investigates — giving it a head start on CNV-specific resource chains and raising investigation confidence by 2-8% across 5 planned scenarios.

The feature spans 6 layers: shared types, KA enrichment detection, KA prompt/result propagation, DS OpenAPI/SQL filtering/scoring, DS model/schema validation, and ogen client regeneration.

**Business Requirements**:
- BR-WORKFLOW-018: CNV workload detection for v1.5 scenarios (demo-scenarios #375-#380)
- BR-WORKFLOW-004: DS catalog labels use GVK format; discovery filters must match
- BR-SP-101: DetectedLabels Auto-Detection
- BR-SP-103: FailedDetections Tracking

**FedRAMP Control Mapping**:

| Control | Objective | Behavioral Assurance |
|---------|-----------|---------------------|
| SI-10 | Information input validation | RESTMapper pre-check validates CNV CRD existence before API calls; `storageBackend` enum validation prevents injection of arbitrary provisioner strings; `FailedDetections` tracks query failures without silently returning false |
| AU-2 | Auditable events defined | `detectedLabels` in structured logs and investigation prompt reflects actual cluster CNV state. Workflow discovery logs carry CNV label values for audit traceability |
| AU-12 | Audit generation | Every enrichment invocation logs the resolved `DetectedLabels` including CNV fields, enabling post-incident audit of which infrastructure context was used for workflow selection |
| CM-6 | Configuration settings | `storageBackend` values (`odf-ceph`, `lvms`, `local`) are derived from StorageClass provisioner, reflecting actual cluster storage configuration |

## 3. Test Items

| Item | File | Description |
|------|------|-------------|
| `DetectedLabels` struct | `pkg/shared/types/enrichment.go` | 4 new fields + updated `FailedDetections` enum |
| `detectVirtualMachine` | `internal/kubernautagent/enrichment/label_detector.go` | Owner chain walk for CNV kinds |
| `detectLiveMigratable` | `internal/kubernautagent/enrichment/label_detector.go` | Conditional GET VM spec for evictionStrategy |
| `detectCDIManaged` | `internal/kubernautagent/enrichment/label_detector.go` | Conditional LIST PVCs for CDI annotations |
| `detectStorageBackend` | `internal/kubernautagent/enrichment/label_detector.go` | PVC -> StorageClass -> provisioner mapping |
| `detectedLabelsToPromptMap` | `internal/kubernautagent/investigator/investigator_phases.go` | 4 new prompt map entries |
| `detectedLabelsToResult` | `internal/kubernautagent/investigator/investigator_phases.go` | 4 new result map entries |
| `buildContextFilterSQL` | `pkg/datastorage/repository/workflow/discovery.go` | 3 new boolFields + 1 stringField |
| `buildDetectedLabelsBoostSQL` | `pkg/datastorage/repository/workflow/scoring.go` | 4 new scoring weights |
| `SerializeLabels` | `pkg/datastorage/models/workflow_labels.go` | 4 new sparse JSON entries |
| `DetectedLabelsSchema` | `pkg/datastorage/models/workflow_schema.go` | 4 new field specs + validation |
| `ExtractDetectedLabels` | `pkg/datastorage/schema/parser.go` | 4 new field conversions |
| DS OpenAPI spec | `api/openapi/data-storage-v1.yaml` | 4 new properties + enum updates |

## 4. Features to Be Tested

### 4.1 CNV Label Detection (UT tier — KA Enrichment)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-KA-1378-001 | BR-WORKFLOW-018 | SI-10 | VirtualMachine in owner chain | `labels.VirtualMachine == true` |
| UT-KA-1378-002 | BR-WORKFLOW-018 | — | VirtualMachineInstance as target kind | `labels.VirtualMachine == true` |
| UT-KA-1378-003 | BR-WORKFLOW-018 | — | DataVolume in owner chain | `labels.VirtualMachine == true` |
| UT-KA-1378-004 | BR-WORKFLOW-018 | — | Non-VM workload (Deployment) | `labels.VirtualMachine == false`, no FailedDetections |
| UT-KA-1378-005 | BR-WORKFLOW-018 | — | VM with evictionStrategy=LiveMigrate | `labels.LiveMigratable == true` |
| UT-KA-1378-006 | BR-WORKFLOW-018 | — | VM without evictionStrategy | `labels.LiveMigratable == false` |
| UT-KA-1378-007 | BR-WORKFLOW-018 | — | Non-VM workload skips liveMigratable detection | `labels.LiveMigratable == false`, no extra API call |
| UT-KA-1378-008 | BR-WORKFLOW-018 | — | PVC with `cdi.kubevirt.io/storage.import.*` annotation | `labels.CDIManaged == true` |
| UT-KA-1378-009 | BR-WORKFLOW-018 | — | PVC without CDI annotations | `labels.CDIManaged == false` |
| UT-KA-1378-010 | BR-WORKFLOW-018 | CM-6 | PVC -> StorageClass with `rbd.csi.ceph.com` provisioner | `labels.StorageBackend == "odf-ceph"` |
| UT-KA-1378-011 | BR-WORKFLOW-018 | CM-6 | PVC -> StorageClass with `topolvm.io` provisioner | `labels.StorageBackend == "lvms"` |
| UT-KA-1378-012 | BR-WORKFLOW-018 | CM-6 | PVC -> StorageClass with `kubernetes.io/no-provisioner` | `labels.StorageBackend == "local"` |
| UT-KA-1378-013 | BR-WORKFLOW-018 | CM-6 | PVC -> StorageClass with unknown provisioner | `labels.StorageBackend == ""` |
| UT-KA-1378-014 | BR-WORKFLOW-018 | — | No PVCs in namespace | `labels.StorageBackend == ""`, `labels.CDIManaged == false` |
| UT-KA-1378-015 | BR-WORKFLOW-018 | SI-10 | CNV CRDs not installed (RESTMapper pre-check) | All 4 CNV fields == zero values, NOT in FailedDetections |
| UT-KA-1378-016 | BR-SP-103 | SI-10 | VM GET fails (RBAC denied) | `liveMigratable` in FailedDetections |
| UT-KA-1378-017 | BR-SP-103 | SI-10 | PVC LIST fails (timeout) | `cdiManaged` and `storageBackend` in FailedDetections |
| UT-KA-1378-018 | BR-WORKFLOW-018 | — | VirtualMachineInstanceMigration as target kind | `labels.VirtualMachine == true` |

### 4.2 Prompt and Result Propagation (UT tier — KA Investigator)

| ID | BR | Scenario | Asserts |
|----|-----|----------|---------|
| UT-KA-1378-020 | BR-WORKFLOW-018 | detectedLabelsToPromptMap with VM+migration+CDI+odf-ceph | Map contains `virtualMachine=true`, `liveMigratable=true`, `cdiManaged=true`, `storageBackend=odf-ceph` |
| UT-KA-1378-021 | BR-WORKFLOW-018 | detectedLabelsToPromptMap with VM but no storage | Map contains `virtualMachine=true`, no `storageBackend` key |
| UT-KA-1378-022 | BR-WORKFLOW-018 | detectedLabelsToPromptMap with non-VM workload | No CNV keys in map |
| UT-KA-1378-023 | BR-WORKFLOW-018 | detectedLabelsToResult with all CNV fields | Result map contains all 4 CNV fields (including false values) |

### 4.3 DS SQL Filter (UT tier — DataStorage)

| ID | BR | FedRAM | Scenario | Asserts |
|----|-----|--------|----------|---------|
| UT-DS-1378-001 | BR-WORKFLOW-004 | SI-10 | buildContextFilterSQL with virtualMachine=true | SQL contains `virtualMachine` condition |
| UT-DS-1378-002 | BR-WORKFLOW-004 | SI-10 | buildContextFilterSQL with liveMigratable=true | SQL contains `liveMigratable` condition |
| UT-DS-1378-003 | BR-WORKFLOW-004 | SI-10 | buildContextFilterSQL with cdiManaged=true | SQL contains `cdiManaged` condition |
| UT-DS-1378-004 | BR-WORKFLOW-004 | SI-10 | buildContextFilterSQL with storageBackend=odf-ceph | SQL contains `storageBackend` condition with wildcard support |
| UT-DS-1378-005 | BR-WORKFLOW-004 | — | buildContextFilterSQL with all 4 CNV fields set | SQL contains all 4 conditions joined by AND |
| UT-DS-1378-006 | BR-WORKFLOW-004 | — | buildContextFilterSQL with no CNV fields (all false/empty) | No CNV conditions in SQL |

### 4.4 DS Scoring (UT tier — DataStorage)

| ID | BR | Scenario | Asserts |
|----|-----|----------|---------|
| UT-DS-1378-010 | BR-WORKFLOW-004 | buildDetectedLabelsBoostSQL with virtualMachine=true | SQL contains 0.08 weight |
| UT-DS-1378-011 | BR-WORKFLOW-004 | buildDetectedLabelsBoostSQL with storageBackend=odf-ceph | SQL contains exact match + wildcard + 0.05 weight |
| UT-DS-1378-012 | BR-WORKFLOW-004 | buildDetectedLabelsBoostSQL with all 4 CNV fields | SQL contains all 4 boost cases |

### 4.5 DS Schema Validation (UT tier — DataStorage)

| ID | BR | Scenario | Asserts |
|----|-----|----------|---------|
| UT-DS-1378-020 | BR-WORKFLOW-004 | DetectedLabelsSchema validates virtualMachine="true" | No error |
| UT-DS-1378-021 | BR-WORKFLOW-004 | DetectedLabelsSchema validates storageBackend="odf-ceph" | No error |
| UT-DS-1378-022 | BR-WORKFLOW-004 | DetectedLabelsSchema rejects storageBackend="invalid" | SchemaValidationError |
| UT-DS-1378-023 | BR-WORKFLOW-004 | DetectedLabelsSchema rejects virtualMachine="false" | SchemaValidationError (booleans only accept "true") |
| UT-DS-1378-024 | BR-WORKFLOW-004 | SerializeLabels includes CNV fields when set | JSON contains `virtualMachine`, `storageBackend` |
| UT-DS-1378-025 | BR-WORKFLOW-004 | SerializeLabels omits CNV fields when false/empty | JSON does not contain CNV keys |
| UT-DS-1378-026 | BR-WORKFLOW-004 | ExtractDetectedLabels converts "true" to bool for 3 CNV booleans | `dl.VirtualMachine == true`, etc. |
| UT-DS-1378-027 | BR-WORKFLOW-004 | ExtractDetectedLabels preserves storageBackend string | `dl.StorageBackend == "odf-ceph"` |

### 4.6 Enrichment Wiring (IT tier — KA Enrichment)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| IT-KA-1378-001 | BR-WORKFLOW-018 | AU-12 | Full DetectLabels with fake VirtualMachine + DataVolume + PVC + StorageClass | All 4 CNV labels correctly detected in single call |
| IT-KA-1378-002 | BR-WORKFLOW-018 | SI-10 | DetectLabels on non-CNV cluster (no kubevirt.io CRDs in mapper) | All 4 CNV fields are zero values; no FailedDetections for CNV |
| IT-KA-1378-003 | BR-WORKFLOW-018 | SI-10 | DetectLabels with VM but RBAC-denied PVC LIST | `virtualMachine=true`, `cdiManaged` and `storageBackend` in FailedDetections |

### 4.7 Investigator Prompt Propagation Wiring (IT tier — KA Investigator)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| IT-KA-1378-004 | BR-WORKFLOW-018 | AU-2 | MCP `discover_workflows` (or direct Investigator dispatch) with CNV-enriched DetectedLabels: virtualMachine=true, liveMigratable=true, cdiManaged=true, storageBackend=odf-ceph | Prompt map passed to LLM contains all 4 CNV entries; result map returned contains all 4 CNV fields |

### 4.8 DS Discovery Filter Wiring (IT tier — DataStorage)

| ID | BR | Scenario | Asserts |
|----|-----|----------|---------|
| IT-DS-1378-001 | BR-WORKFLOW-004 | ListWorkflowsByActionType with virtualMachine=true filters correctly | Only VM-compatible workflows returned; scoring boost applied |
| IT-DS-1378-002 | BR-WORKFLOW-004 | ListWorkflowsByActionType with storageBackend=odf-ceph matches exact + wildcard | Correct workflow ranking |

### 4.9 DS RW CRD Roundtrip Wiring (IT tier — DataStorage)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| IT-DS-1378-003 | BR-WORKFLOW-004 | SI-10 | Register RW CRD with detectedLabels containing virtualMachine=true, storageBackend=odf-ceph -> parse via DetectedLabelsSchema -> serialize via SerializeLabels -> extract via ExtractDetectedLabels -> query via ListWorkflowsByActionType with matching filters | Workflow returned with correct CNV label match; schema validation passes; roundtrip preserves all 4 CNV field values |

### 4.10 Enrichment Pipeline E2E (negative-path — CI-runnable)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| E2E-KA-1378-001 | BR-WORKFLOW-018 | SI-10 | Non-CNV cluster (Kind, no KubeVirt CRDs): trigger enrichment for a standard Deployment alert; verify all 4 CNV fields are zero-valued and NOT listed in FailedDetections | `virtualMachine=false`, `liveMigratable=false`, `cdiManaged=false`, `storageBackend=""`, no CNV entries in `FailedDetections` |

**Rationale**: On a non-CNV cluster, the RESTMapper pre-check must cleanly skip all CNV detection without errors or FailedDetections entries. This E2E proves the graceful degradation path runs through the full production stack (alert -> enrichment -> investigation -> workflow discovery) without CNV infra interference. It runs in the existing Kind-based E2E suite with no additional infrastructure.

### 4.11 Full CNV Journey (E2E tier — deferred to demo-scenarios #375)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| E2E-KA-1378-002 | BR-WORKFLOW-018 | SI-10, AU-12, CM-6 | Deploy a VM with DataVolume on Ceph StorageClass; trigger enrichment; verify all 4 labels detected; verify CNV-specific workflow matched | All 4 CNV labels present in enrichment result; DS returns CNV-compatible workflow |

**Note**: E2E-KA-1378-002 requires CNV operator + ODF in the E2E cluster, which exceeds GitHub Actions CI resource limits. Execution is deferred to `kubernaut-demo-scenarios#376` (vm-boot-failure), which is the first scenario exercising all 4 CNV labels. UT + IT tiers + E2E-KA-1378-001 (negative path) provide behavioral assurance for GA readiness.

**Deferred validation requirements** (tracked via comment on `kubernaut-demo-scenarios#376`):
- `validate.sh` must assert `enrichment.detectedLabels` contains `virtualMachine=true`, `liveMigratable`, `cdiManaged`, `storageBackend`
- RW CRD fixture for the scenario must include `detectedLabels` schema with CNV fields to exercise DS filter/scoring
- Workflow selection must be verified as causally dependent on CNV DetectedLabels (not just GVK match)

## 5. Features Not to Be Tested

- Autonomous `Investigate()` path enrichment pipeline (unchanged, already tested)
- Signal processing DetectedLabels path (SP uses its own Python detection, not KA Go code)
- Existing 10 DetectedLabels fields (gitOpsManaged, stateful, etc.) — unchanged, regression covered by existing tests
- ogen-generated client code (auto-generated, tested via its own framework)
- DataVolume CRD controller behavior (CDI operator, external)
- CNV scenario workflow logic (demo-scenarios #376-#380, separate issues)

## 6. Approach

### 6.1 TDD Methodology

Each implementation phase follows strict TDD RED-GREEN-REFACTOR:

1. **RED**: Write failing Ginkgo/Gomega tests defining the business contract
2. **GREEN**: Minimal implementation to pass all tests
3. **REFACTOR**: Improve code quality; validate against [100 Go Mistakes](https://github.com/teivah/100-go-mistakes)

### 6.2 Test Pyramid (Pyramid Invariant)

> UT proves logic. IT proves wiring. E2E proves the journey.

| Tier | Count | Target Coverage | Strategy |
|------|-------|-----------------|----------|
| Unit (UT) | 39 scenarios | >= 80% of new KA detection + DS filter/scoring/schema code | Table-driven where appropriate, Ginkgo BDD for KA, standard Go testing for DS |
| Integration (IT) | 7 scenarios | >= 80% of wiring points; every Wiring Manifest row has an IT | Fake K8s clients for KA, envtest/real DB for DS |
| E2E | 1 CI-runnable (negative path) + 1 deferred (positive path) | Graceful degradation on non-CNV cluster (CI); full CNV journey (deferred to #375) | Kind cluster without KubeVirt CRDs |

### 6.3 Go Anti-Pattern Avoidance (100 Go Mistakes)

The following mistakes are specifically relevant to this feature and will be validated during the REFACTOR phase:

| # | Mistake | Relevance to #1378 |
|---|---------|-------------------|
| #1 | Unintended variable shadowing | `detectCDIManaged`/`detectStorageBackend` share PVC list; `err` shadow risk in nested loops |
| #5 | Interface pollution | `SignalContextResolver` already slimmed; no new interfaces unless needed |
| #8 | `any` says nothing | All new fields use concrete types (`bool`, `string` with enum) |
| #21 | Inefficient slice initialization | PVC list results should pre-allocate if count is known |
| #27 | Inefficient map initialization | `detectedLabelsToPromptMap` should pre-size with `make(map, N)` |
| #29 | Comparing values incorrectly | `storageBackend` comparison must be case-sensitive (provisioner strings are exact) |
| #48 | Forgetting the return statement after replying to an HTTP request | Not directly relevant but watch for early returns after FailedDetections |
| #53 | Not handling an error | Every K8s API call must have error handling with FailedDetections tracking |
| #54 | Not handling defer errors | `defer` in test helpers must not swallow errors |
| #77 | Common JSON-handling mistakes | `SerializeLabels` must handle omitempty correctly for `storageBackend` |
| #89 | Writing inaccurate benchmarks | No benchmarks in this PR, but test timeouts must be realistic |

### 6.4 Checkpoints

#### CP-1: After TDD Red (All tiers)

| Dimension | Gate |
|-----------|------|
| All new tests fail with expected errors | Functions undefined / wrong behavior |
| Build succeeds | `go build ./...` green (production code only has struct changes) |
| Existing tests unbroken | UT/IT for existing labels still pass |

#### CP-2: After TDD Green (Detection Logic)

| Dimension | Gate | Evidence |
|-----------|------|----------|
| Build | `go build ./...` green | CI output |
| UT KA | UT-KA-1378-001..018 pass | Test output |
| UT KA Prompt | UT-KA-1378-020..023 pass | Test output |
| IT KA | IT-KA-1378-001..003 pass | Test output |
| Coverage KA | >= 80% of `detectVirtualMachine`, `detectLiveMigratable`, `detectCDIManaged`, `detectStorageBackend` | `go test -cover` |
| FedRAMP SI-10 | RESTMapper pre-check validates CRD existence | UT-KA-1378-015 |
| FedRAMP AU-12 | CNV labels appear in enrichment result | IT-KA-1378-001 |
| Pyramid Invariant | UT proves detection logic, IT proves wiring through DetectLabels | All tiers green |
| Wiring | All 4 detect functions called from `DetectLabels` | grep evidence |
| No regressions | All existing UT-KA-776-* and UT-KA-433-* tests pass | CI output |

#### CP-3: After TDD Green (DS Layer)

| Dimension | Gate | Evidence |
|-----------|------|----------|
| Build | `go build ./...` green | CI output |
| UT DS Filter | UT-DS-1378-001..006 pass | Test output |
| UT DS Scoring | UT-DS-1378-010..012 pass | Test output |
| UT DS Schema | UT-DS-1378-020..027 pass | Test output |
| IT DS | IT-DS-1378-001..002 pass | Test output |
| Coverage DS | >= 80% of new `buildContextFilterSQL` and `buildDetectedLabelsBoostSQL` code | `go test -cover` |
| FedRAMP SI-10 | Schema validation rejects invalid `storageBackend` values | UT-DS-1378-022 |
| Ogen regen | `make generate-datastorage-client` succeeds, no manual drift | Generated files match spec |
| No regressions | All existing DS filter/scoring tests pass | CI output |

#### CP-4: After TDD Refactor (Full Pipeline)

| Dimension | Gate | Evidence |
|-----------|------|----------|
| Build + Lint | `go build ./...` + `golangci-lint` clean | CI output |
| All tiers green | UT + IT pass, no flakes | Full CI pipeline green |
| 100 Go Mistakes | No violations in changed code (checklist in Section 6.3) | Code review |
| No orphaned code | All new functions have production callers | grep evidence |
| Wiring Manifest | All rows verified | CHECKPOINT W results |
| Coverage per tier | >= 80% of testable code per tier | Coverage report |
| Confidence >= 95% | Grounded in test results | Assessment report |

## 7. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| `detectVirtualMachine` | `LabelDetector.DetectLabels()` | `label_detector.go` | IT-KA-1378-001 |
| `detectLiveMigratable` | `LabelDetector.DetectLabels()` | `label_detector.go` | IT-KA-1378-001 |
| `detectCDIManaged` | `LabelDetector.DetectLabels()` | `label_detector.go` | IT-KA-1378-001 |
| `detectStorageBackend` | `LabelDetector.DetectLabels()` | `label_detector.go` | IT-KA-1378-001 |
| `buildContextFilterSQL` (3 bool + 1 string) | `ListActions` / `ListWorkflowsByActionType` | `discovery.go` | IT-DS-1378-001 |
| `buildDetectedLabelsBoostSQL` (4 weights) | `ListWorkflowsByActionType` | `scoring.go` | IT-DS-1378-001 |
| `detectedLabelsToPromptMap` (4 entries) | `Investigator.Investigate()` | `investigator_phases.go` | IT-KA-1378-004 |
| `detectedLabelsToResult` (4 entries) | `Investigator.Investigate()` | `investigator_phases.go` | IT-KA-1378-004 |
| `SerializeLabels` (4 entries) | RW registration via DS handlers | `workflow_labels.go` | IT-DS-1378-003 |
| `ExtractDetectedLabels` (4 fields) | Schema parser | `parser.go` | IT-DS-1378-003 |
| `DetectedLabelsSchema` validation | RW schema parse | `workflow_schema.go` | IT-DS-1378-003 |

## 8. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/tests/1378/TEST_PLAN.md` |
| UT: CNV detection | `internal/kubernautagent/enrichment/detected_labels_1378_test.go` |
| UT: Prompt/Result propagation | `internal/kubernautagent/investigator/signal_label_override_test.go` (extend existing) |
| UT: DS SQL filter | `pkg/datastorage/repository/workflow/discovery_filter_test.go` (extend existing) |
| UT: DS scoring | `pkg/datastorage/repository/workflow/scoring_test.go` (extend existing) |
| UT: DS schema validation | `pkg/datastorage/models/workflow_schema_test.go` (extend existing) |
| UT: DS serialization | `pkg/datastorage/models/workflow_labels_test.go` (extend existing) |
| IT: KA enrichment wiring | `internal/kubernautagent/enrichment/detected_labels_1378_it_test.go` |
| IT: KA investigator prompt wiring | `test/integration/kubernautagent/mcp/wiring_proof_test.go` (extend) or `investigator_cnv_it_test.go` |
| IT: DS discovery wiring | `test/integration/datastorage/workflow_discovery_cnv_test.go` |
| IT: DS RW roundtrip wiring | `test/integration/datastorage/workflow_roundtrip_cnv_test.go` |
| Test fixture: CNV workflow | `test/fixtures/workflows/cnv-vm-boot-failure/workflow-schema.yaml` |
| E2E: Negative-path (CI) | `test/e2e/kubernautagent/cnv_graceful_skip_e2e_test.go` |
| E2E: Full CNV pipeline (deferred) | `test/e2e/kubernautagent/cnv_enrichment_e2e_test.go` (deferred to demo-scenarios #375) |

## 9. Test Environment

| Component | Tool |
|-----------|------|
| Go version | 1.26+ |
| Test framework | Ginkgo v2 / Gomega (KA enrichment, IT); standard Go testing (DS unit) |
| K8s testing | `dynamicfake.NewSimpleDynamicClient` + `meta.NewDefaultRESTMapper` for UT; envtest for IT |
| CI | GitHub Actions (`ci-pipeline.yml`) |
| CNV CRDs | Simulated via fake dynamic client (UT/IT); real CNV operator (E2E, deferred) |

## 10. Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation | Test Coverage |
|------|--------|------------|------------|---------------|
| CNV CRDs not in RESTMapper on non-CNV clusters | Medium | High (most clusters) | RESTMapper pre-check skips all CNV detection cleanly | UT-KA-1378-015 |
| PVC LIST returns large result sets on busy namespaces | Low | Medium | PVC LIST is already O(namespace) for existing HPA/PDB patterns; CNV adds one LIST | UT-KA-1378-014 |
| StorageClass provisioner strings vary across providers | Medium | Medium | Explicit mapping with empty-string fallback for unknown provisioners | UT-KA-1378-013 |
| Ogen regeneration produces large diff | Low | High (expected) | Committed as separate commit for review clarity | CI build |
| E2E requires CNV infrastructure not yet available | Medium | High | UT + IT provide >= 80% coverage; E2E deferred to demo-scenarios #375 | IT-KA-1378-001..003 |
| `storageBackend` wildcard `"*"` in RW CRD matches any backend | Low | Low | Follows existing `serviceMesh` / `gitOpsTool` wildcard pattern | UT-DS-1378-004 |

## 11. Schedule

| Phase | Tests | Checkpoint |
|-------|-------|------------|
| Phase 1: Shared Types + TDD Red (all tiers) | Write all UT + IT test skeletons with expected behavior | **CP-1** |
| Phase 2: TDD Green — KA Detection | UT-KA-1378-001..018, UT-KA-1378-020..023, IT-KA-1378-001..004 | **CP-2** |
| Phase 3: TDD Green — DS Layer | UT-DS-1378-001..027, IT-DS-1378-001..003 | **CP-3** |
| Phase 4: TDD Refactor + E2E | 100 Go Mistakes audit, lint, E2E-KA-1378-001 (negative path), wiring manifest verification | **CP-4** |

## 12. Approvals

| Role | Name | Date |
|------|------|------|
| Author | AI Agent | 2026-06-08 |
| Reviewer | — | — |
