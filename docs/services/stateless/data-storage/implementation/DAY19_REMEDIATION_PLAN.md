# Day 19: Triage Remediation Plan (1h) - V5.3 CRITICAL GAPS

**Status**: ❌ **NOT STARTED** (P0 fixes required before V1.0 deployment)
**Version**: V5.3
**Date**: 2025-11-05
**Purpose**: Address critical gaps identified in V5.3 triage to achieve production readiness

**Triage Reference**: [BR-STORAGE-031-05-IMPLEMENTATION-TRIAGE.md](../../../../BR-STORAGE-031-05-IMPLEMENTATION-TRIAGE.md)

---

## 📊 **REMEDIATION OVERVIEW**

**Total Effort**: **57 minutes** (7 min P0 + 35 min P1 + 15 min P2)

**Confidence Progression**:
- **Before Remediation**: 85% (2 critical gaps, 3 improvements)
- **After P0 (7 min)**: 95% (critical gaps resolved, V1.0 ready)
- **After P0+P1 (42 min)**: 98% (all high-priority gaps resolved)
- **After P0+P1+P2 (57 min)**: 99% (comprehensive, production-ready)

**Production Readiness**:
- **After P0**: ✅ **V1.0 DEPLOYMENT APPROVED**
- **After P0+P1**: ✅ **PRODUCTION-READY** (recommended)
- **After P0+P1+P2**: ✅ **GOLD STANDARD** (best practice)

---

## 🚨 **Phase 1: Critical Fixes (P0)** - **7 minutes**

**Goal**: Unblock V1.0 deployment by fixing documentation/traceability gaps

### **Task 19.1: Add OpenAPI Tag for Success Rate Analytics** (5 min)

**File**: `docs/services/stateless/data-storage/openapi/v2.yaml`

**Problem**: No "Success Rate Analytics" tag in OpenAPI spec

**Fix**:
```yaml
# Add at line ~5918 (after existing tags)
tags:
  # ... existing tags ...
  - name: Success Rate Analytics
    description: Multi-dimensional success tracking for AI-driven remediation effectiveness (ADR-033, BR-STORAGE-031-01, BR-STORAGE-031-02, BR-STORAGE-031-05)
```

**Update Endpoints**:
```yaml
# Update all 3 success-rate endpoints
/api/v1/success-rate/incident-type:
  get:
    tags: [Success Rate Analytics]  # ← Add this

/api/v1/success-rate/playbook:
  get:
    tags: [Success Rate Analytics]  # ← Add this

/api/v1/success-rate/multi-dimensional:
  get:
    tags: [Success Rate Analytics]  # ← Add this
```

**Validation**:
```bash
# Verify tag is present
grep -A 2 "Success Rate Analytics" docs/services/stateless/data-storage/openapi/v2.yaml
```

---

### **Task 19.2: Add BR Comment to Repository Method** (2 min)

**File**: `pkg/datastorage/repository/action_trace_repository.go` (line 418)

**Problem**: `GetSuccessRateMultiDimensional()` lacks BR-STORAGE-031-05 comment

**Fix**:
```go
// GetSuccessRateMultiDimensional calculates success rate across multiple dimensions
// BR-STORAGE-031-05: Multi-Dimensional Success Rate API
// ADR-033: Remediation Playbook Catalog - Multi-dimensional tracking
// Supports any combination of: incident_type, playbook_id + playbook_version, action_type
func (r *ActionTraceRepository) GetSuccessRateMultiDimensional(
```

**Validation**:
```bash
# Verify BR comment is present
grep -B 2 "GetSuccessRateMultiDimensional" pkg/datastorage/repository/action_trace_repository.go | grep "BR-STORAGE-031-05"
```

---

## ⚠️ **Phase 2: High-Priority Improvements (P1)** - **35 minutes**

**Goal**: Improve code traceability and add missing validation

### **Task 19.3: Add BR Comment to Handler** (2 min)

**File**: `pkg/datastorage/server/aggregation_handlers.go` (line 355)

**Problem**: Handler has generic comment, no explicit BR/ADR reference

**Fix**:
```go
// HandleGetSuccessRateMultiDimensional handles GET /api/v1/success-rate/multi-dimensional
// BR-STORAGE-031-05: Multi-Dimensional Success Rate API
// ADR-033: Remediation Playbook Catalog - Cross-dimensional aggregation
// Supports any combination of: incident_type, playbook_id + playbook_version, action_type
func (h *Handler) HandleGetSuccessRateMultiDimensional(w http.ResponseWriter, r *http.Request) {
```

---

### **Task 19.4: Add "At Least One Dimension" Validation** (30 min)

**Problem**: Handler accepts queries with NO dimensions (contradicts api-specification.md)

**Files to Update**:
1. `pkg/datastorage/server/aggregation_handlers.go`
2. `test/unit/datastorage/aggregation_handlers_test.go`
3. `test/integration/datastorage/aggregation_api_adr033_test.go`

**Implementation**:

**Step 1**: Add validation in `parseMultiDimensionalParams()`:
```go
// parseMultiDimensionalParams extracts and validates query parameters for multi-dimensional queries
func (h *Handler) parseMultiDimensionalParams(r *http.Request) (*models.MultiDimensionalQuery, error) {
	query := r.URL.Query()

	// Extract dimension filters
	incidentType := query.Get("incident_type")
	playbookID := query.Get("playbook_id")
	playbookVersion := query.Get("playbook_version")
	actionType := query.Get("action_type")

	// Validate at least one dimension is provided
	if incidentType == "" && playbookID == "" && actionType == "" {
		return nil, fmt.Errorf("at least one dimension filter (incident_type, playbook_id, or action_type) must be specified")
	}

	// ... rest of validation ...
}
```

**Step 2**: Add unit test:
```go
It("should return 400 Bad Request when no dimensions are specified", func() {
	// ARRANGE: Request with no dimension filters
	req = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/success-rate/multi-dimensional?time_range=7d",
		nil,
	)

	// ACT: Call handler
	handler.HandleGetSuccessRateMultiDimensional(rec, req)

	// ASSERT: HTTP 400 Bad Request
	Expect(rec.Code).To(Equal(http.StatusBadRequest),
		"Handler should return 400 when no dimensions are specified")

	// CORRECTNESS: Error message explains the issue
	var problem validation.RFC7807Problem
	json.NewDecoder(rec.Body).Decode(&problem)
	Expect(problem.Detail).To(ContainSubstring("at least one dimension filter"))
})
```

**Step 3**: Add integration test:
```go
It("should return 400 Bad Request when no dimensions are specified", func() {
	// ARRANGE: Query with no dimension filters
	resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/multi-dimensional?time_range=7d", datastorageURL))
	Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// ASSERT: HTTP 400 Bad Request
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

	// CORRECTNESS: RFC 7807 error response
	var problem map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&problem)
	Expect(problem["detail"]).To(ContainSubstring("at least one dimension filter"))
})
```

**Validation**:
```bash
# Run unit tests
go test ./test/unit/datastorage/... -v | grep "no dimensions"

# Run integration tests
go test ./test/integration/datastorage/... -v | grep "no dimensions"
```

---

## 📝 **Phase 3: Documentation Polish (P2)** - **15 minutes**

**Goal**: Improve API documentation quality

### **Task 19.5: Add OpenAPI Example Responses** (15 min)

**File**: `docs/services/stateless/data-storage/openapi/v2.yaml`

**Problem**: No example responses for 200, 400, 500 status codes

**Fix**:
```yaml
/api/v1/success-rate/multi-dimensional:
  get:
    # ... existing spec ...
    responses:
      '200':
        description: Multi-dimensional success rate data
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/MultiDimensionalSuccessRateResponse'
            examples:
              all_dimensions:
                summary: All three dimensions specified
                value:
                  dimensions:
                    incident_type: "pod-oom-killer"
                    playbook_id: "pod-oom-recovery"
                    playbook_version: "v1.2"
                    action_type: "increase_memory"
                  time_range: "7d"
                  total_executions: 50
                  successful_executions: 45
                  failed_executions: 5
                  success_rate: 90.0
                  confidence: "medium"
                  min_samples_met: true
              
              partial_dimensions:
                summary: Two dimensions (incident + playbook)
                value:
                  dimensions:
                    incident_type: "pod-oom-killer"
                    playbook_id: "pod-oom-recovery"
                    playbook_version: "v1.2"
                    action_type: ""
                  time_range: "7d"
                  total_executions: 120
                  successful_executions: 108
                  failed_executions: 12
                  success_rate: 90.0
                  confidence: "high"
                  min_samples_met: true

      '400':
        description: Invalid parameters
        content:
          application/problem+json:
            schema:
              $ref: '#/components/schemas/RFC7807Error'
            examples:
              missing_dimensions:
                summary: No dimensions specified
                value:
                  type: "https://api.kubernaut.io/problems/validation-error"
                  title: "Invalid Query Parameter"
                  status: 400
                  detail: "at least one dimension filter (incident_type, playbook_id, or action_type) must be specified"
                  instance: "/api/v1/success-rate/multi-dimensional"
              
              playbook_version_without_id:
                summary: playbook_version without playbook_id
                value:
                  type: "https://api.kubernaut.io/problems/validation-error"
                  title: "Invalid Query Parameter"
                  status: 400
                  detail: "playbook_version requires playbook_id to be specified"
                  instance: "/api/v1/success-rate/multi-dimensional"

      '500':
        description: Internal server error
        content:
          application/problem+json:
            schema:
              $ref: '#/components/schemas/RFC7807Error'
            example:
              type: "https://api.kubernaut.io/problems/internal-error"
              title: "Internal Server Error"
              status: 500
              detail: "Failed to retrieve multi-dimensional success rate data"
              instance: "/api/v1/success-rate/multi-dimensional"
```

---

## ✅ **VALIDATION COMMANDS**

### **After P0 Fixes**:
```bash
# Run all tests
go test ./test/unit/datastorage/... -v
go test ./test/integration/datastorage/... -v

# Verify OpenAPI tag
grep -A 2 "Success Rate Analytics" docs/services/stateless/data-storage/openapi/v2.yaml

# Verify BR comments
grep "BR-STORAGE-031-05" pkg/datastorage/repository/action_trace_repository.go
```

### **After P1 Fixes** (should have 2 new tests):
```bash
# Run tests for new validation
go test ./test/unit/datastorage/... -v | grep "no dimensions"
go test ./test/integration/datastorage/... -v | grep "no dimensions"

# Verify handler BR comment
grep "BR-STORAGE-031-05" pkg/datastorage/server/aggregation_handlers.go

# Verify all tests still pass
go test ./test/unit/datastorage/... -v | tail -5
go test ./test/integration/datastorage/... -v | tail -5
```

### **After P2 Fixes**:
```bash
# Verify OpenAPI examples
grep -A 30 "examples:" docs/services/stateless/data-storage/openapi/v2.yaml | grep "all_dimensions"
```

---

## 📊 **FILES TO UPDATE**

### **Critical (P0)** - 2 files:
1. `docs/services/stateless/data-storage/openapi/v2.yaml` - Add tag
2. `pkg/datastorage/repository/action_trace_repository.go` - Add BR comment

### **High Priority (P1)** - 3 files:
3. `pkg/datastorage/server/aggregation_handlers.go` - Add validation + BR comment
4. `test/unit/datastorage/aggregation_handlers_test.go` - Add test
5. `test/integration/datastorage/aggregation_api_adr033_test.go` - Add test

### **Medium Priority (P2)** - 1 file:
6. `docs/services/stateless/data-storage/openapi/v2.yaml` - Add examples

**Total**: 6 files (1 file updated twice: openapi/v2.yaml in P0 and P2)

---

## 🎯 **SUCCESS CRITERIA**

### **After P0** (7 minutes):
- ✅ OpenAPI tag "Success Rate Analytics" exists
- ✅ All 3 success-rate endpoints use the new tag
- ✅ Repository method has BR-STORAGE-031-05 comment
- ✅ All 473 unit tests pass
- ✅ All 23 integration tests pass
- ✅ **V1.0 DEPLOYMENT APPROVED** (95% confidence)

### **After P0+P1** (42 minutes):
- ✅ Handler has BR-STORAGE-031-05 + ADR-033 comment
- ✅ Validation rejects queries with no dimensions
- ✅ 475 unit tests pass (2 new tests)
- ✅ 24 integration tests pass (1 new test)
- ✅ **PRODUCTION-READY** (98% confidence)

### **After P0+P1+P2** (57 minutes):
- ✅ OpenAPI has comprehensive example responses
- ✅ All validation commands pass
- ✅ **GOLD STANDARD** (99% confidence)

---

## 📝 **NEXT STEPS**

1. **Immediate**: Apply P0 fixes (7 minutes) to unblock V1.0
2. **Recommended**: Apply P1 fixes (35 minutes) for production readiness
3. **Optional**: Apply P2 fixes (15 minutes) for best practice documentation

**Total Time to Production-Ready**: **42 minutes** (P0 + P1)

---

**Document Created**: 2025-11-05
**Status**: Ready for execution
**Approval**: Awaiting user confirmation to proceed with remediation

