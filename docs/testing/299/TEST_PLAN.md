# Test Plan: CRD + Admission Webhook Workflow Registration (#299)

**Feature**: Replace OCI-based workflow schema registration with Kubernetes-native CRD + Admission Webhook mechanism
**Version**: 1.0
**Created**: 2026-03-08
**Author**: AI Assistant
**Status**: Complete
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- BR-WORKFLOW-006: Kubernetes-native Workflow Registration
- ADR-058: ValidatingWebhookConfiguration for RemediationWorkflow CRD
- DD-WEBHOOK-003: Complete Audit Events for Admission Webhooks

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Phase 1 Test Plan (#292)](../292/TEST_PLAN.md)
- [GitHub Issue #299](https://github.com/jordigilh/kubernaut/issues/299)

---

## 1. Scope

### In Scope

- **DS Inline Schema Endpoint**: Modified `HandleCreateWorkflow` accepts `{ content, source, registeredBy }` inline request, rejects legacy `schemaImage`, re-enables disabled workflows
- **AW RemediationWorkflow Handler**: Intercepts CREATE/DELETE via `ValidatingWebhookConfiguration`, bridges CRD lifecycle with DS catalog
- **CRD Status Update**: Async `k8sClient.Status().Update()` populates `.status.workflowId`, `.catalogStatus`, `.registeredBy`, `.registeredAt`, `.previouslyExisted`
- **Audit Events**: `remediationworkflow.admitted.create`, `.delete`, `.denied` with actor attribution
- **Helm Charts**: `ValidatingWebhookConfiguration` entry, RBAC for `remediationworkflows` and `remediationworkflows/status`
- **E2E Infrastructure**: Inline registration in test seeding and bundle setup

### Out of Scope

- Phase 1 (#292) format migration (covered by [Phase 1 Test Plan](../292/TEST_PLAN.md))
- Seed-workflows Helm Job migration to CRD manifests (deferred, Step 8)
- AW integration tests with real Kind cluster (deferred to E2E)
- OCI bundle validation (`ValidateBundleExists`) -- unchanged, still uses OCI puller

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| ValidatingWebhookConfiguration (not Mutating) | `+kubebuilder:subresource:status` prevents mutating webhook from patching `.status` |
| Async status update via goroutine | Avoids blocking API server admission response; 10s timeout guards against hangs |
| `json.Unmarshal` instead of `decoder` | Simpler; no decoder injection needed; CRD is in `req.Object.Raw` |
| `authenticator.ExtractUser()` for user identity | Consistent with all other AW handlers; provides UID/Groups for audit |
| `callCreateWorkflowInline` shared helper | Deduplicates registration logic between `workflow_bundles.go` and `workflow_seeding.go` |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (handler logic, audit emission, status update, DS adapter)
- **Integration**: Deferred to E2E suites that exercise the full AW -> DS pipeline in Kind
- **E2E**: Existing E2E suites will validate via inline registration infrastructure

### 2-Tier Minimum

- **Unit tests** verify handler logic, audit correctness, and status update with fake k8s client
- **E2E tests** (existing infrastructure) validate full registration pipeline via inline DS API

### Business Outcome Quality Bar

Tests validate that:
1. CRD CREATE triggers DS workflow registration and CRD status population
2. CRD DELETE triggers DS workflow disable (best-effort)
3. Audit events are emitted with correct actor attribution for SOC2 compliance
4. Legacy OCI registration path is rejected with actionable error message

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/authwebhook/remediationworkflow_handler.go` | `Handle`, `handleCreate`, `handleDelete`, `updateCRDStatus` | ~150 |
| `pkg/authwebhook/remediationworkflow_audit.go` | `emitAdmitAudit`, `emitDeniedAudit` | ~55 |
| `pkg/authwebhook/ds_client.go` | `CreateWorkflowInline`, `DisableWorkflow`, `mapOgenWorkflowToResult` | ~60 |
| `pkg/datastorage/server/workflow_handlers.go` | `HandleCreateWorkflow` (inline path), `buildWorkflowFromInlineSchema`, `buildWorkflowCommon`, `tryReEnableWorkflow` | ~120 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `cmd/authwebhook/main.go` | Handler registration, DS client wiring, scheme setup | ~30 |
| `charts/kubernaut/templates/authwebhook/webhooks.yaml` | ValidatingWebhookConfiguration | ~20 |
| `charts/kubernaut/templates/authwebhook/authwebhook.yaml` | ClusterRole RBAC | ~5 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WORKFLOW-006 | Inline schema accepted and registered in catalog | P0 | Unit | UT-DS-299-001 | Pass |
| BR-WORKFLOW-006 | Legacy OCI schemaImage format rejected | P0 | Unit | UT-DS-299-002 | Pass |
| BR-WORKFLOW-006 | Disabled workflow re-enabled on re-CREATE | P0 | Unit | UT-DS-299-003 | Pass |
| BR-WORKFLOW-006 | Full validation pipeline (actionType, bundle, deps) | P0 | Unit | UT-DS-299-004 | Pass |
| BR-WORKFLOW-006 | Invalid inline schema rejected with 400 | P0 | Unit | UT-DS-299-005 | Pass |
| BR-WORKFLOW-006 | Content hash computed from inline YAML | P1 | Unit | UT-DS-299-006 | Pass |
| BR-WORKFLOW-006 | SchemaImage nil for inline registration | P1 | Unit | UT-DS-299-007 | Pass |
| ADR-058 | CREATE forwards CRD spec to DS, returns Allowed | P0 | Unit | UT-AW-299-001 | Pass |
| ADR-058 | DELETE disables workflow in DS, returns Allowed | P0 | Unit | UT-AW-299-002 | Pass |
| ADR-058 | CREATE denied when DS unreachable | P0 | Unit | UT-AW-299-003 | Pass |
| ADR-058 | Re-CREATE re-enables previously deleted workflow | P0 | Unit | UT-AW-299-004 | Pass |
| DD-WEBHOOK-003 | CREATE audit event with actor attribution | P0 | Unit | UT-AW-299-005 | Pass |
| DD-WEBHOOK-003 | DELETE audit event with actor attribution | P0 | Unit | UT-AW-299-006 | Pass |
| DD-WEBHOOK-003 | Denied audit event on CREATE failure | P0 | Unit | UT-AW-299-007 | Pass |
| SOC2 CC8.1 | User extracted from AdmissionReview.userInfo | P0 | Unit | UT-AW-299-008 | Pass |
| ADR-058 | UPDATE operations pass through without DS call | P1 | Unit | UT-AW-299-009 | Pass |
| ADR-058 | DELETE always allowed even if DS disable fails | P0 | Unit | UT-AW-299-010 | Pass |
| ADR-058 | Source and registeredBy passed to DS | P1 | Unit | UT-AW-299-011 | Pass |
| ADR-058 | DELETE with empty status skips DS disable | P0 | Unit | UT-AW-299-012 | Pass |
| ADR-058 | CREATE populates CRD .status asynchronously | P0 | Unit | UT-AW-299-013 | Pass |

### Status Legend

- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: `DS` (DataStorage), `AW` (AuthWebhook)
- **BR_NUMBER**: Issue number (299) for this feature
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests -- Data Storage Inline Schema

**File**: `test/unit/datastorage/workflow_create_handler_test.go`

| ID | Business Outcome Under Test | Given | When | Then |
|----|----------------------------|-------|------|------|
| UT-DS-299-001 | Valid inline schema is accepted and stored | Valid CRD YAML in `content` field | POST to `/api/v1/workflows` with inline request | 201 Created with populated workflow (name, version, actionType, labels) |
| UT-DS-299-002 | Legacy OCI format rejected with guidance | Request body contains `schemaImage` field | POST to `/api/v1/workflows` | 400 Bad Request with message explaining inline migration |
| UT-DS-299-003 | Disabled workflow re-enabled on duplicate | Workflow exists in DB with status=disabled | POST with same workflow content | 200 OK with re-activated workflow, status=active |
| UT-DS-299-004 | Full validation pipeline executes | Valid inline YAML with actionType, bundle, dependencies | POST to `/api/v1/workflows` | 201 Created after passing actionType taxonomy, bundle validation, and dependency checks |
| UT-DS-299-005 | Invalid schema produces clear error | YAML missing `apiVersion` or wrong `kind` | POST with malformed content | 400 Bad Request with validation error naming specific field |
| UT-DS-299-006 | Content hash deterministic | Valid inline YAML | POST to `/api/v1/workflows` | `contentHash` is SHA-256 of the raw YAML bytes |
| UT-DS-299-007 | OCI fields nil for inline | Valid inline registration | POST to `/api/v1/workflows` | `schemaImage` and `schemaDigest` are nil in stored workflow |

### Tier 1: Unit Tests -- AuthWebhook Handler

**File**: `test/unit/authwebhook/remediationworkflow_handler_test.go`

| ID | Business Outcome Under Test | Given | When | Then |
|----|----------------------------|-------|------|------|
| UT-AW-299-001 | CRD CREATE triggers DS registration | Valid `RemediationWorkflow` CRD in admission request | CREATE admission review | `Allowed` response; DS `CreateWorkflowInline` called with CRD JSON as content, `source="crd"`, `registeredBy=username` |
| UT-AW-299-002 | CRD DELETE triggers DS disable | CRD with `status.workflowId` populated | DELETE admission review | `Allowed` response; DS `DisableWorkflow` called with correct workflowId and `reason="CRD deleted"` |
| UT-AW-299-003 | DS failure denies CREATE | DS returns error on CreateWorkflowInline | CREATE admission review | `Denied` response with "data storage registration failed" message |
| UT-AW-299-004 | Re-CREATE re-enables | DS returns `PreviouslyExisted=true` | CREATE admission review for previously deleted workflow | `Allowed` response; result contains `PreviouslyExisted=true` |
| UT-AW-299-005 | CREATE emits audit event | Successful CREATE flow | CREATE admission review | `remediationworkflow.admitted.create` audit event with correct actor, resource, correlationID |
| UT-AW-299-006 | DELETE emits audit event | Successful DELETE flow | DELETE admission review | `remediationworkflow.admitted.delete` audit event emitted |
| UT-AW-299-007 | Denied CREATE emits denied audit | DS call fails | CREATE admission review | `remediationworkflow.admitted.denied` audit event with failure outcome |
| UT-AW-299-008 | User identity extracted from AdmissionReview | `req.UserInfo` populated with username, UID, groups | CREATE admission review | `registeredBy` in DS call matches `userInfo.username`; audit actor matches |
| UT-AW-299-009 | UPDATE passes through | Any UPDATE operation | UPDATE admission review | `Allowed("operation not intercepted")`; no DS call made |
| UT-AW-299-010 | DELETE resilient to DS failure | DS `DisableWorkflow` returns error | DELETE admission review | `Allowed` response (best-effort); error logged but not surfaced |
| UT-AW-299-011 | Source and registeredBy forwarded | CREATE with `userInfo.username="admin@example.com"` | CREATE admission review | DS called with `source="crd"`, `registeredBy="admin@example.com"` |
| UT-AW-299-012 | Empty status skips disable | CRD with empty `.status.workflowId` | DELETE admission review | `Allowed`; DS `DisableWorkflow` NOT called; audit event still emitted |
| UT-AW-299-013 | Status update via fake k8s client | Successful CREATE with fake k8s client | CREATE admission review | CRD `.status.workflowId`, `.catalogStatus`, `.registeredBy`, `.registeredAt`, `.previouslyExisted` populated within 5s |

### Tier Skip Rationale

| Tier | Rationale |
|------|-----------|
| Integration (AW) | Requires real Kind cluster with CRD installed, DS service running, and webhook certificate. Covered by existing E2E suites that use inline registration infrastructure. |
| E2E | Existing E2E suites (`test/e2e/datastorage/`, `test/e2e/workflowexecution/`) already validate the inline registration pipeline through updated infrastructure helpers. |

---

## 6. Files Changed

### Production Code

| File | Change Type | Description |
|------|-------------|-------------|
| `api/openapi/data-storage-v1.yaml` | Modified | Replaced `CreateWorkflowFromOCIRequest` with `CreateWorkflowInlineRequest`; removed 422/502 responses; added 200 for re-enable |
| `pkg/datastorage/ogen-client/oas_*.go` | Regenerated | `make generate-datastorage-client` after OpenAPI changes |
| `pkg/datastorage/server/workflow_handlers.go` | Modified | Inline schema flow; removed `classifyExtractionError`, `buildWorkflowFromSchema`, `oci` import; added `tryReEnableWorkflow`, `buildWorkflowFromInlineSchema`, `buildWorkflowCommon` |
| `pkg/authwebhook/remediationworkflow_handler.go` | Modified | Full handler implementation: `handleCreate`, `handleDelete`, `updateCRDStatus` with authenticator |
| `pkg/authwebhook/remediationworkflow_audit.go` | New | Extracted audit helpers: `emitAdmitAudit`, `emitDeniedAudit` |
| `pkg/authwebhook/ds_client.go` | New | `DSClientAdapter` wrapping ogen client; `CreateWorkflowInline`, `DisableWorkflow` |
| `pkg/authwebhook/types.go` | Modified | Added `EventTypeRWAdmittedCreate`, `EventTypeRWAdmittedDelete`, `EventTypeRWAdmittedDenied` |
| `cmd/authwebhook/main.go` | Modified | Scheme registration, DS client creation, handler registration at `/validate-remediationworkflow` |
| `charts/kubernaut/templates/authwebhook/webhooks.yaml` | Modified | New ValidatingWebhookConfiguration entry for `remediationworkflows` |
| `charts/kubernaut/templates/authwebhook/authwebhook.yaml` | Modified | RBAC: `remediationworkflows` (get/list/watch), `remediationworkflows/status` (update/patch) |

### Test Code

| File | Change Type | Description |
|------|-------------|-------------|
| `test/unit/authwebhook/remediationworkflow_handler_test.go` | Modified | 13 unit tests (UT-AW-299-001 through UT-AW-299-013) |
| `test/unit/datastorage/workflow_create_handler_test.go` | Renamed + Modified | From `workflow_create_oci_handler_test.go`; 8 inline schema tests (UT-DS-299-001 through UT-DS-299-007) |
| `test/unit/datastorage/execution_bundle_validation_test.go` | Modified | Converted to inline schema requests |
| `test/infrastructure/workflow_bundles.go` | Modified | Inline registration; extracted shared `callCreateWorkflowInline` |
| `test/infrastructure/workflow_seeding.go` | Modified | Uses shared `callCreateWorkflowInline`; stale OCI comments updated |
| `test/e2e/datastorage/*.go` | Modified | Updated to `CreateWorkflowInlineRequest` and new response types |
| `kubernaut-agent/tests/fixtures/workflow_fixtures.py` | Modified | `bootstrap_workflows` uses `to_inline_request()` |

---

## 7. Validation Results

### Build

```
go build ./...  # PASS (zero errors)
```

### Unit Tests

```
go test ./test/unit/... -count=1  # 49 suites, all PASS
```

| Suite | Tests | Status |
|-------|-------|--------|
| `test/unit/authwebhook` | 74 (13 new for #299) | Pass |
| `test/unit/datastorage` | All (8 new for #299) | Pass |
| All other suites | No regressions | Pass |

---

## 8. Known Limitations

| Limitation | Impact | Mitigation |
|------------|--------|------------|
| Async status update is fire-and-forget | If API server latency exceeds 10s, status may not be populated | Logged at error level; retry logic can be added in future |
| DELETE with empty status skips DS disable | If status update hasn't completed before DELETE, workflow is not disabled in DS | Logged; workflow will remain active in DS until TTL or manual cleanup |
| Audit adapter and workflow adapter create separate HTTP clients | Minor resource overhead (2 connection pools to same URL) | `NewDSClientAdapterFromClient` enables sharing when audit package API is updated |
| Seed-workflows migration not yet complete | Helm Job still uses old curl-based registration | Deferred to Step 8; tracked separately |
