# RR Reconstruction - Next Steps After Gap #8 - January 12, 2026

## 🎯 **Current Status**

**Just Completed**: ✅ **Gap #8 - TimeoutConfig Audit Capture**
- ✅ `orchestrator.lifecycle.created` event emits TimeoutConfig
- ✅ Webhook captures operator mutations
- ✅ Integration tests passing
- ✅ Production-ready

**Overall Progress**: **65% Complete** (Infrastructure)

---

## 📊 **Gap Status Overview**

### **✅ Completed Gaps (Infrastructure)**

| Gap | Field | Service | Status | Completed |
|---|---|---|---|---|
| **#1** | `OriginalPayload` | Gateway | ✅ **COMPLETE** | SOC2 Week 1 |
| **#2** | `ProviderData` | AI Analysis | ✅ **COMPLETE** | SOC2 Week 1 |
| **#3** | `SignalLabels` | Gateway | ✅ **COMPLETE** | SOC2 Week 1 |
| **#8** | `TimeoutConfig` | Orchestrator | ✅ **COMPLETE** | Jan 12, 2026 |

**65% of audit capture complete**

---

### **⚠️ Remaining Gaps (Verification + Implementation)**

| Gap | Field | Service | Status | Estimated Effort |
|---|---|---|---|---|
| **#4** | `SignalAnnotations` | Gateway | ⚠️ **NEEDS VERIFICATION** | 30 min - 1 hour |
| **#5** | `SelectedWorkflowRef` | Workflow | ⚠️ **NEEDS VERIFICATION** | 1-2 hours |
| **#6** | `ExecutionRef` | Orchestrator | ⚠️ **NEEDS VERIFICATION** | 30 min - 1 hour |
| **#7** | `Error` (detailed) | All Services | ✅ **COMPLETE** | (Already done) |

**35% of audit capture remains**

---

### **❌ Not Started (Core Feature)**

| Component | Description | Estimated Effort |
|---|---|---|
| **Reconstruction Logic** | Parse audit events → build RR YAML | **17 hours** (~2 days) |
| **REST API Endpoint** | `/api/v1/audit/remediation-requests/{id}/reconstruct` | **6 hours** (~1 day) |
| **Integration Tests** | E2E reconstruction validation | **4 hours** |
| **Documentation** | API docs + user guide | **4 hours** |

**Total Remaining**: **31 hours** (~4 days)

---

## 🗺️ **Roadmap: Next 4 Days**

### **Day 1: Gap Verification & Completion** (8 hours)

#### **Morning: Verification (3 hours)**

**Task 1.1: Verify Gap #4 - SignalAnnotations** (1 hour)
```bash
# Check if Gateway already captures annotations
grep -r "SignalAnnotations\|signal_annotations" pkg/gateway/ --include="*.go"

# Check audit emission
grep -A 30 "emitSignalReceivedAudit" pkg/gateway/server.go
```

**Expected Outcomes**:
- ✅ Best case: Already captured (just needs verification)
- ⚠️ Likely case: Schema exists, population missing (1 hour to add)
- ❌ Worst case: Schema + population missing (2 hours)

---

**Task 1.2: Verify Gap #5 - SelectedWorkflowRef** (1 hour)
```bash
# Check RR status structure
grep -A 30 "type RemediationRequestStatus" api/remediation/v1alpha1/remediationrequest_types.go

# Check if workflow selection is audited
grep -r "workflowexecution.selection.completed" pkg/workflowexecution/audit/
```

**Expected Outcomes**:
- ✅ Best case: Field exists + audit event exists (just connect them)
- ⚠️ Likely case: Field exists, audit population missing (1-2 hours)
- ❌ Worst case: Field missing (requires CRD change - defer to Phase 2)

---

**Task 1.3: Verify Gap #6 - ExecutionRef** (1 hour)
```bash
# Check if orchestrator captures execution ref
grep -r "ExecutionRef\|execution_ref" pkg/remediationorchestrator/ --include="*.go"

# Check workflow execution audit events
grep -r "workflowexecution.execution.started" pkg/workflowexecution/audit/
```

**Expected Outcomes**:
- ✅ Best case: Already captured in `orchestrator.lifecycle.*` events
- ⚠️ Likely case: Needs minor audit payload update
- ❌ Worst case: Requires new audit event (2 hours)

---

#### **Afternoon: Implementation (5 hours)**

Based on verification results, implement missing audit captures.

**Contingency**: If all gaps are already captured, move to Day 2 work.

---

### **Day 2: Reconstruction Logic - Core Algorithm** (8 hours)

#### **Morning: Query & Parse (4 hours)**

**Task 2.1: Audit Query Function** (1 hour)
```go
// pkg/datastorage/reconstruction/query.go
func QueryAuditEventsForReconstruction(ctx context.Context, correlationID string) ([]AuditEvent, error)
```

**Task 2.2: Event Parser** (1 hour)
```go
// pkg/datastorage/reconstruction/parser.go
func ParseAuditEvent(event AuditEvent) (FieldMapping, error)
```

**Task 2.3: Field Mapper** (2 hours)
```go
// pkg/datastorage/reconstruction/mapper.go
func MapAuditToRRFields(events []AuditEvent) (RemediationRequestSpec, error)
```

---

#### **Afternoon: CRD Builder (4 hours)**

**Task 2.4: YAML Generator** (2 hours)
```go
// pkg/datastorage/reconstruction/builder.go
func BuildRemediationRequest(spec RemediationRequestSpec, status RemediationRequestStatus) (*remediationv1.RemediationRequest, error)
```

**Task 2.5: Validation** (2 hours)
```go
// pkg/datastorage/reconstruction/validator.go
func ValidateReconstructedRR(rr *remediationv1.RemediationRequest) error
```

---

### **Day 3: REST API + Integration** (8 hours)

#### **Morning: API Implementation (4 hours)**

**Task 3.1: REST Handler** (2 hours)
```go
// pkg/datastorage/api/reconstruct_handler.go
// GET /api/v1/audit/remediation-requests/{correlation_id}/reconstruct
```

**Task 3.2: OpenAPI Schema** (1 hour)
- Update `api/openapi/data-storage-v1.yaml`
- Add reconstruction endpoint
- Regenerate ogen client

**Task 3.3: RBAC** (1 hour)
- Add permissions for reconstruction endpoint
- Update ClusterRole manifests

---

#### **Afternoon: Integration Tests (4 hours)**

**Task 3.4: E2E Reconstruction Test** (3 hours)
```go
// test/integration/datastorage/reconstruction_integration_test.go
// Scenario 1: Reconstruct RR with all fields
// Scenario 2: Reconstruct RR with partial audit data
// Scenario 3: Validate reconstruction accuracy
```

**Task 3.5: Error Scenarios** (1 hour)
- Missing audit data
- Invalid correlation ID
- Malformed audit events

---

### **Day 4: Polish & Documentation** (7 hours)

#### **Morning: Testing & Validation (3 hours)**

**Task 4.1: Full E2E Test** (2 hours)
- Deploy all services
- Create RR through Gateway
- Reconstruct from audit trail
- Compare original vs reconstructed

**Task 4.2: Edge Cases** (1 hour)
- Failed remediations
- Partial workflows
- Multiple recovery attempts

---

#### **Afternoon: Documentation (4 hours)**

**Task 4.3: API Documentation** (2 hours)
- Endpoint specification
- Request/response examples
- Error codes and handling

**Task 4.4: User Guide** (2 hours)
- When to use reconstruction
- CLI examples with `curl`
- Troubleshooting guide

---

## 🎯 **Success Criteria**

### **Phase 1: Gap Completion** ✅
- [ ] All Gaps #4-6 verified
- [ ] Missing audit captures implemented
- [ ] Integration tests passing

### **Phase 2: Reconstruction Logic** ✅
- [ ] Query function retrieves all audit events
- [ ] Parser extracts RR fields correctly
- [ ] YAML generator produces valid CRD
- [ ] Unit tests passing (>70% coverage)

### **Phase 3: REST API** ✅
- [ ] Endpoint deployed and accessible
- [ ] OpenAPI schema updated
- [ ] RBAC configured correctly
- [ ] Integration tests passing

### **Phase 4: Production Ready** ✅
- [ ] Full E2E test passing
- [ ] Documentation complete
- [ ] Performance validated (<2s reconstruction)
- [ ] User guide published

---

## 📋 **Immediate Next Steps (This Week)**

### **Priority 1: Gap Verification** (Today/Tomorrow)

**Action Items**:
1. ✅ **Verify Gap #4** (SignalAnnotations)
   - Check Gateway audit code
   - Add if missing (1 hour)

2. ✅ **Verify Gap #5** (SelectedWorkflowRef)
   - Check RR status structure
   - Check workflow audit events
   - Connect if separated (1-2 hours)

3. ✅ **Verify Gap #6** (ExecutionRef)
   - Check orchestrator audit events
   - Verify execution ref captured (30 min)

**Time Required**: **3-4 hours**

---

### **Priority 2: Start Reconstruction Logic** (This Week)

**Action Items**:
1. **Create Reconstruction Package** (30 min)
   ```bash
   mkdir -p pkg/datastorage/reconstruction
   touch pkg/datastorage/reconstruction/{query,parser,mapper,builder,validator}.go
   ```

2. **Implement Query Function** (1 hour)
   - Use existing audit query infrastructure
   - Filter by correlation ID
   - Order by timestamp

3. **TDD: Write Failing Tests** (2 hours)
   - Test with mock audit data
   - Validate field mapping
   - Verify YAML structure

**Time Required**: **4 hours**

---

## 🚀 **Quick Start Commands**

### **1. Verify Current Gap Status**

```bash
# Gap #4: SignalAnnotations
grep -r "SignalAnnotations" pkg/gateway/server.go

# Gap #5: SelectedWorkflowRef
grep -A 30 "RemediationRequestStatus" api/remediation/v1alpha1/remediationrequest_types.go

# Gap #6: ExecutionRef
grep -r "lifecycle.transitioned" pkg/remediationorchestrator/audit/

# Gap #7: Error (already complete via Gap #7 work)
grep -r "error_details" pkg/remediationorchestrator/audit/

# Gap #8: TimeoutConfig (just completed)
grep -r "lifecycle.created" pkg/remediationorchestrator/audit/
```

---

### **2. Start Reconstruction Logic**

```bash
# Create package structure
mkdir -p pkg/datastorage/reconstruction
cd pkg/datastorage/reconstruction

# Create files (TDD: tests first)
cat > query_test.go <<EOF
package reconstruction_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Audit Query for RR Reconstruction", func() {
    Context("when querying by correlation ID", func() {
        It("should retrieve all audit events for a remediation", func() {
            // TDD RED: Write failing test first
            Skip("Not implemented yet")
        })
    })
})
EOF

# Run tests (will fail - RED phase)
ginkgo run -v .
```

---

## 📊 **Effort Breakdown**

| Phase | Description | Estimated | Actual (Planned) |
|---|---|---|---|
| **Gap Verification** | Check Gaps #4-6 | 3 hours | Day 1 Morning |
| **Gap Completion** | Implement missing captures | 5 hours | Day 1 Afternoon |
| **Reconstruction Logic** | Core algorithm | 8 hours | Day 2 |
| **REST API** | Endpoint + RBAC | 4 hours | Day 3 Morning |
| **Integration Tests** | E2E validation | 4 hours | Day 3 Afternoon |
| **Polish** | Testing + edge cases | 3 hours | Day 4 Morning |
| **Documentation** | API docs + guide | 4 hours | Day 4 Afternoon |

**Total**: **31 hours** (~4 days)

---

## 🎯 **Confidence Assessment**

### **Overall Confidence**: 90%

**Why High Confidence**:
- ✅ 65% of infrastructure already complete (SOC2 + Gap #8)
- ✅ Audit query infrastructure proven working
- ✅ OpenAPI schema patterns established
- ✅ Integration test patterns validated
- ✅ TDD methodology in place

**Risks**:
- ⚠️ Gaps #4-6 may need more work than expected (add 1 day buffer)
- ⚠️ YAML generation complexity (add validation layer)
- ⚠️ E2E testing may reveal edge cases (add debugging time)

**Mitigation**:
- Start with verification ASAP (today/tomorrow)
- Use TDD for reconstruction logic
- Incremental E2E testing (don't wait until end)

---

## 📚 **References**

### **Authoritative Plans**

1. ✅ **[RR_RECONSTRUCTION_V1_1_IMPLEMENTATION_PLAN_JAN10.md](../development/SOC2/RR_RECONSTRUCTION_V1_1_IMPLEMENTATION_PLAN_JAN10.md)**
   - 3-day roadmap (now 4 days with Gap #8 complete)
   - Gap analysis
   - Implementation strategy

2. ✅ **[SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md](../development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md)**
   - Test scenarios
   - Validation criteria
   - Coverage requirements

3. ✅ **[RR_RECONSTRUCTION_API_DESIGN_DEC_18_2025.md](../handoff/RR_RECONSTRUCTION_API_DESIGN_DEC_18_2025.md)**
   - REST API specification
   - Request/response formats
   - Error handling

### **Completed Work**

1. ✅ **Gap #8 Implementation**
   - `docs/handoff/GAP8_COMPLETE_IMPLEMENTATION_SUMMARY_JAN12.md`
   - `docs/handoff/GAP8_IMPLEMENTATION_TRIAGE_JAN12.md`
   - `docs/handoff/GAP8_PRIORITY1_FIXES_COMPLETE_JAN12.md`

2. ✅ **SOC2 Gaps #1-3**
   - `docs/development/SOC2/SOC2_AUDIT_IMPLEMENTATION_PLAN.md`
   - `docs/handoff/SOC2_IMPLEMENTATION_TRIAGE_JAN12_2026.md`

---

## ✅ **Recommended Next Action**

### **Option 1: Start Verification Now** (3-4 hours)

Verify Gaps #4-6 to understand exact remaining work.

### **Option 2: Commit Gap #8 First** (30 min)

Commit current work, then start verification in clean state.

### **Option 3: Start Reconstruction Logic** (Parallel Track)

Begin TDD for reconstruction while gaps are verified.

**My Recommendation**: **Option 2** - Commit Gap #8 first, then start fresh with verification tomorrow.

**Rationale**:
- ✅ Clean slate for new feature work
- ✅ Gap #8 is production-ready
- ✅ Avoid mixing concerns
- ✅ Fresh start for RR reconstruction

---

**Document Status**: ✅ **COMPLETE**
**Current Phase**: **Gap #8 Complete, Ready for Gaps #4-6**
**Recommendation**: **COMMIT GAP #8, THEN START VERIFICATION**
