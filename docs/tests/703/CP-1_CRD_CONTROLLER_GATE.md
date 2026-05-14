# CP-1: CRD & Controller Gate — Test Case Specifications

**Checkpoint**: CP-1
**Gate Type**: Unit Tests (Ginkgo/Gomega)
**Total Checks**: 13
**Merge Criteria**: All 13 unit tests pass, existing AA controller tests unmodified and green
**PR**: PR1 (CRD Scaffolding)

---

## Overview

CP-1 validates that CRD changes (`InteractiveSessionInfo` on `AIAnalysisStatus`) are backward-compatible, the poll handler correctly routes the new `user_driving` status, and timeout extension logic is bounded by the global maximum.

---

## Test Environment

- **Package**: `test/unit/kubernautagent/mcp/crd/`
- **Framework**: Ginkgo/Gomega BDD
- **Key Imports**:
  ```go
  "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
  "github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
  metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
  ```
- **Mocks**: None required (pure type/logic tests)
- **Setup**: Standard Ginkgo suite with `RegisterFailHandler(Fail)` and `RunSpecs`

---

## Test Cases

### UT-KA-INT-001: v1.5 CRD applies cleanly over v1.4 AIAnalysis data

**BR**: BR-INTERACTIVE-007
**Type**: Unit
**Category**: Backward Compatibility
**Check ID**: ADV-CRD-01

**Description**: A v1.4 AIAnalysis resource (without `interactiveSession` field) can be deserialized by v1.5 types without error, and the `InteractiveSession` field is nil.

**Preconditions**:
- v1.4 AIAnalysis JSON fixture exists (no `interactiveSession` key in status)
- v1.5 `AIAnalysisStatus` type imported

**Steps**:
1. Create a raw JSON payload representing a v1.4 `AIAnalysisStatus` (no `interactiveSession` field)
2. Unmarshal into `v1alpha1.AIAnalysisStatus`
3. Assert no error
4. Assert `status.InteractiveSession` is `nil`
5. Assert all other v1.4 fields are preserved correctly

**Acceptance Criteria**:
- Unmarshal succeeds with zero errors
- `InteractiveSession` is nil (not empty struct)
- All existing v1.4 status fields (Phase, SessionID, StartedAt, etc.) are intact

---

### UT-KA-INT-002: v1.4 controller reads v1.5 AIAnalysis without error

**BR**: BR-INTERACTIVE-007
**Type**: Unit
**Category**: Backward Compatibility
**Check ID**: ADV-CRD-02

**Description**: A v1.5 AIAnalysis resource (with `interactiveSession` populated) can be read by code that only accesses v1.4 fields — unknown fields are ignored per K8s strategic merge patch semantics.

**Preconditions**:
- v1.5 AIAnalysis JSON fixture exists (with `interactiveSession` populated)

**Steps**:
1. Create a full v1.5 `AIAnalysisStatus` with `InteractiveSession` populated (SessionID, ActingUser, StartedAt)
2. Marshal to JSON
3. Unmarshal into a v1.4-like struct (same type but only accessing existing fields)
4. Assert no error
5. Assert v1.4 fields are correctly populated
6. Assert accessing `InteractiveSession` on the full v1.5 type returns populated data

**Acceptance Criteria**:
- Round-trip marshal/unmarshal succeeds
- No data loss on v1.4 fields
- `InteractiveSession` accessible on v1.5 type

---

### UT-KA-INT-003: InteractiveSessionInfo fields are mutable via status update

**BR**: BR-INTERACTIVE-007
**Type**: Unit
**Category**: Happy Path
**Check ID**: ADV-CRD-03

**Description**: The `InteractiveSessionInfo` struct supports all field mutations needed for lifecycle transitions (populate on takeover, set CompletedAt on disconnect).

**Preconditions**:
- `v1alpha1.InteractiveSessionInfo` type available

**Steps**:
1. Create `AIAnalysisStatus` with `InteractiveSession = nil`
2. Set `InteractiveSession` to a new `&InteractiveSessionInfo{SessionID: "sess-01", ActingUser: "user-a@corp", StartedAt: &now}`
3. Assert all fields populated
4. Set `InteractiveSession.CompletedAt = &later`
5. Assert CompletedAt is set while other fields preserved
6. Set `InteractiveSession = nil` (session cleared on autonomous resume)
7. Assert `InteractiveSession` is nil

**Acceptance Criteria**:
- All lifecycle transitions (nil → populated → completed → nil) succeed
- No field corruption between transitions
- `omitempty` JSON tags work correctly (nil fields not serialized)

---

### UT-KA-INT-004: Unknown poll status treated as pending (requeue)

**BR**: BR-INTERACTIVE-007
**Type**: Unit
**Category**: Error Handling
**Check ID**: ADV-CRD-04

**Description**: When `handleSessionPoll` receives an unknown status string from KA, it treats it as "pending" and requeues (defensive coding against future status values).

**Preconditions**:
- `handlers.InvestigatingHandler` instantiated with mock dependencies
- Mock KA client returns a status response with `Status: "unknown_future_value"`

**Steps**:
1. Create a mock KA session status response: `{Status: "unknown_future_value"}`
2. Call `handleSessionPoll(ctx, analysis, status)`
3. Assert the handler does NOT return an error
4. Assert the handler requeues (returns `ctrl.Result{RequeueAfter: ...}`)
5. Assert a log entry is emitted with level Info containing "Unknown session status"

**Acceptance Criteria**:
- No panic or error on unknown status
- Requeue behavior (same as "pending")
- Info-level log emitted (not error level)

---

### UT-KA-INT-005: Timeout extension bounded by global max (1h)

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: Security
**Check ID**: ADV-CRD-05

**Description**: When interactive session requests timeout extension, the extended deadline cannot exceed the global maximum (1h from RR creation). Late takeovers get only the remaining global time.

**Preconditions**:
- RR creation timestamp available
- Global timeout configured at 1h
- Function `calculateExtendedTimeout(rrCreatedAt, globalMax, requestedExtension)` exists

**Steps**:
1. Set RR creation time to `now - 50m` (50 minutes elapsed)
2. Request extension of 30m (would exceed 1h global)
3. Call `calculateExtendedTimeout(createdAt, 1h, 30m)`
4. Assert returned timeout is 10m (remaining global time), NOT 30m
5. Repeat with RR creation at `now - 10m`, request 30m extension
6. Assert returned timeout is 30m (within global bounds)
7. Repeat with RR creation at `now - 61m` (already past global)
8. Assert returned timeout is 0 or error indicating session must end

**Acceptance Criteria**:
- Extension never exceeds `globalMax - elapsed` time
- Late takeover gets only remaining time (not a fresh window)
- Past-global returns zero/error (immediate session end)
- Function is pure (no side effects, easily testable)

---

### UT-KA-INT-006: Nil InteractiveSession returns default poll interval (10m)

**BR**: BR-INTERACTIVE-007
**Type**: Unit
**Category**: Happy Path
**Check ID**: ADV-CRD-06

**Description**: When `InteractiveSession` is nil (autonomous mode), the poll interval calculation returns the default 10m requeue, not a panic.

**Preconditions**:
- `AIAnalysisStatus` with `InteractiveSession = nil`
- Function `getPollInterval(status)` exists

**Steps**:
1. Create `AIAnalysisStatus{InteractiveSession: nil}`
2. Call `getPollInterval(status)`
3. Assert returned interval is `10 * time.Minute` (default)
4. Create `AIAnalysisStatus{InteractiveSession: &InteractiveSessionInfo{...}}`
5. Call `getPollInterval(status)` for active session
6. Assert returned interval is shorter (e.g., `30 * time.Second`) for active interactive polling

**Acceptance Criteria**:
- Nil `InteractiveSession` → default 10m interval (no panic)
- Populated `InteractiveSession` → shorter interval for responsive interactive polling
- Pure function, no nil pointer dereference

---

### UT-KA-INT-007: No spec field changes (status-only)

**BR**: BR-INTERACTIVE-007
**Type**: Unit
**Category**: Security
**Check ID**: SEC-CRD-01

**Description**: Interactive mode adds NO fields to `AIAnalysisSpec`. All interactive state lives in `AIAnalysisStatus`. This prevents users from controlling interactive behavior via spec (which they can set at creation).

**Preconditions**:
- `v1alpha1.AIAnalysisSpec` type definition available
- Reference v1.4 field list documented

**Steps**:
1. List all fields in `v1alpha1.AIAnalysisSpec` struct using reflection
2. Compare against expected v1.4 field list
3. Assert no new fields related to interactive mode (no `InteractiveMode`, `InteractiveTimeout`, etc.)
4. Assert `InteractiveSessionInfo` lives only in `AIAnalysisStatus`

**Acceptance Criteria**:
- Zero new fields in `AIAnalysisSpec`
- `InteractiveSession` exists ONLY in `AIAnalysisStatus`
- Spec remains immutable for interactive concerns

---

### UT-KA-INT-008: ActingUser populated from authenticated source only

**BR**: BR-INTERACTIVE-003, BR-INTERACTIVE-007
**Type**: Unit
**Category**: Security
**Check ID**: SEC-CRD-02

**Description**: The `ActingUser` field in `InteractiveSessionInfo` must be set from the authenticated user identity (TokenReview/SAR result), never from client-supplied data.

**Preconditions**:
- Function that populates `InteractiveSessionInfo` from session context
- Authenticated user context available

**Steps**:
1. Create a session context with authenticated user `"admin@corp"` (from TokenReview)
2. Call function that builds `InteractiveSessionInfo` from this context
3. Assert `ActingUser == "admin@corp"`
4. Attempt to build with a client-supplied user hint `"attacker@evil"` 
5. Assert `ActingUser` is still `"admin@corp"` (authenticated source wins)
6. Verify there is no code path that sets ActingUser from request body or query params

**Acceptance Criteria**:
- `ActingUser` always from authenticated context (never request body)
- No setter that accepts arbitrary string for ActingUser
- Authenticated source is the ONLY input

---

### UT-KA-INT-009: Regression — existing Investigating handler tests pass

**BR**: BR-INTERACTIVE-007
**Type**: Unit
**Category**: Regression
**Check ID**: REG-01

**Description**: All pre-existing unit tests in the investigating handler package pass without modification after PR1 changes.

**Preconditions**:
- Pre-existing test files in `pkg/aianalysis/handlers/` unchanged
- PR1 changes applied (new `user_driving` case added)

**Steps**:
1. Run `go test ./pkg/aianalysis/handlers/... -v`
2. Assert all pre-existing tests pass
3. Assert no test was modified (git diff shows no changes to existing test files)

**Acceptance Criteria**:
- 100% pass rate on existing handler tests
- Zero modifications to existing test files
- New `user_driving` case does not affect existing status routing

---

### UT-KA-INT-010: Regression — existing CRD deepcopy works

**BR**: BR-INTERACTIVE-007
**Type**: Unit
**Category**: Regression
**Check ID**: REG-02

**Description**: `DeepCopyObject()` on AIAnalysis correctly deep-copies the new `InteractiveSession` pointer field.

**Preconditions**:
- `controller-gen` has been re-run to generate deepcopy for new field

**Steps**:
1. Create `AIAnalysis` with `Status.InteractiveSession` populated
2. Call `DeepCopy()`
3. Assert copy's `InteractiveSession` is not nil
4. Assert copy's `InteractiveSession` is a different pointer (not same memory)
5. Mutate copy's `InteractiveSession.ActingUser`
6. Assert original is unchanged

**Acceptance Criteria**:
- Deep copy creates independent copy of `InteractiveSession`
- Mutation of copy does not affect original
- Nil `InteractiveSession` deep copies to nil (not empty struct)

---

### UT-KA-INT-011: Regression — AIAnalysis serialization round-trip

**BR**: BR-INTERACTIVE-007
**Type**: Unit
**Category**: Regression
**Check ID**: REG-03

**Description**: Full AIAnalysis resource survives JSON round-trip with the new field present.

**Preconditions**:
- Complete AIAnalysis fixture with all v1.4 + v1.5 fields

**Steps**:
1. Create fully-populated `AIAnalysis` (all spec fields, all status fields including `InteractiveSession`)
2. Marshal to JSON
3. Unmarshal back to `AIAnalysis`
4. Assert deep equality between original and round-tripped copy

**Acceptance Criteria**:
- Zero data loss in round-trip
- `metav1.Time` fields serialize correctly (RFC3339)
- `omitempty` fields absent when nil

---

### UT-KA-INT-012: Regression — CRD validation webhook (if exists) accepts new status

**BR**: BR-INTERACTIVE-007
**Type**: Unit
**Category**: Regression
**Check ID**: REG-04

**Description**: If AIAnalysis has a validating webhook, it accepts status updates that include `InteractiveSession`.

**Preconditions**:
- Check if validating webhook exists for AIAnalysis (if not, this test is N/A)

**Steps**:
1. Check if `api/aianalysis/v1alpha1/` has a webhook file
2. If yes: create AIAnalysis, update status with `InteractiveSession` populated
3. Assert webhook validation passes
4. If no webhook: mark test as N/A (document in test output)

**Acceptance Criteria**:
- Webhook (if exists) does not reject `InteractiveSession` in status
- If no webhook: documented as N/A with reason

---

### UT-KA-INT-013: Regression — controller-gen markers produce valid CRD YAML

**BR**: BR-INTERACTIVE-007
**Type**: Unit
**Category**: Regression
**Check ID**: REG-05

**Description**: Running `controller-gen` produces valid CRD YAML that includes `interactiveSession` in the OpenAPI schema under `.status`.

**Preconditions**:
- `controller-gen` available in PATH
- Makefile target `make manifests` exists

**Steps**:
1. Run `make manifests` (regenerates CRD YAML)
2. Open generated CRD YAML for AIAnalysis
3. Assert `.spec.versions[0].schema.openAPIV3Schema.properties.status.properties` contains `interactiveSession`
4. Assert `interactiveSession` schema has properties: `sessionId`, `mcpSessionId`, `actingUser`, `startedAt`, `completedAt`
5. Assert all properties have correct types (string, string, string, date-time, date-time)
6. Assert `interactiveSession` is NOT in `.spec` schema

**Acceptance Criteria**:
- CRD YAML generated without errors
- `interactiveSession` appears under status (not spec)
- All 5 sub-fields present with correct types
- No validation errors when applying CRD YAML to cluster
