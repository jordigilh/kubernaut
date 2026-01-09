# Notification Service - Audit Struct Migration Triage
## January 8, 2026

**Context**: OpenAPI client migrated from oapi-codegen to ogen for proper typed structs
**Issue**: E2E tests failing because AuthWebhook still compiles with old client that has compilation errors

---

## 🔍 **ROOT CAUSE ANALYSIS**

### What Changed
1. ✅ Removed `x-go-type: interface{}` from OpenAPI spec (OPENAPI_UNSTRUCTURED_DATA_FIX_JAN08.md)
2. ✅ Generated new ogen client with proper discriminated unions (`pkg/datastorage/ogen-client/`)
3. ✅ Migrated all business code to ogen (OGEN_MIGRATION_STATUS_JAN08.md Phase 2 Complete)
4. ❌ **48 test files still use old oapi-codegen client** (`pkg/datastorage/client/`)

### Why E2E Tests Fail
```
Error: building at STEP "RUN ... go build ./cmd/webhooks/main.go"
pkg/datastorage/client/generated.go:2375:4: v.EventType undefined (type AIAnalysisPhaseTransitionPayload has no field or method EventType)
```

**Explanation**:
- AuthWebhook binary (used in E2E tests) imports test helpers
- Test helpers import old `pkg/datastorage/client` (oapi-codegen)
- Old client now has compilation errors because payload structs don't have `EventType` field
- oapi-codegen doesn't properly support discriminated unions like ogen does

---

## 📊 **IMPACT ANALYSIS**

### Business Code: ✅ **ALREADY MIGRATED**

**Notification Service** (6 files):
```bash
$ grep -r "ogenclient" pkg/notification/
pkg/notification/audit/manager.go:  import ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
pkg/notification/delivery/orchestrator.go: (uses ogenclient via audit helpers)
```

**Status**: ✅ **NO CHANGES NEEDED** - Already using ogen client

---

### Test Code: ❌ **48 FILES NEED MIGRATION**

**Notification Test Files** (4 files):
1. ✅ `test/unit/notification/audit_test.go`
2. ✅ `test/unit/notification/audit_adr032_compliance_test.go`
3. ✅ `test/integration/notification/controller_audit_emission_test.go`
4. ✅ `test/e2e/notification/01_notification_lifecycle_audit_test.go`
5. ✅ `test/e2e/notification/02_audit_correlation_test.go`
6. ✅ `test/e2e/notification/04_failed_delivery_audit_test.go`

**Other Test Files** (42 files):
- Gateway: 3 files (integration + E2E)
- AIAnalysis: 7 files (unit + integration + E2E)
- WorkflowExecution: 6 files (unit + integration + E2E)
- DataStorage: 14 files (unit + E2E)
- SignalProcessing: 4 files (unit + integration + E2E)
- RemediationOrchestrator: 3 files (unit + E2E)
- AuthWebhook: 2 files (integration + E2E)
- Shared: 3 files (pkg/testutil, pkg/audit tests)

---

## 🔧 **MIGRATION PATTERN**

### Import Change
```go
// OLD (oapi-codegen)
import dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"

// NEW (ogen)
import ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
```

### Optional Field Changes
```go
// OLD
event.ActorId = &actorID
if event.CorrelationId != nil {
    value := *event.CorrelationId
}

// NEW
event.ActorID.SetTo(actorID)
if event.CorrelationID.IsSet() {
    value := event.CorrelationID.Value
}
```

### Event Data Union Changes
```go
// OLD (unstructured)
eventData := map[string]interface{}{
    "notification_id": notification.Name,
    "channel": channel,
}

// NEW (typed)
payload := ogenclient.NotificationMessageSentPayload{
    NotificationID: notification.Name,
    Channel: channel,
}
event.EventData = ogenclient.NewNotificationMessageSentPayloadAuditEventRequestEventData(payload)
```

---

## ✅ **CONFIDENCE ASSESSMENT**

### Can I Fix This Without Help?

**YES** - **Confidence: 85%**

**Rationale**:
1. ✅ **Business code already migrated** - No NT service logic changes needed
2. ✅ **Clear migration pattern** - Documented in OGEN_MIGRATION_STATUS_JAN08.md
3. ✅ **Mechanical transformation** - Import changes + optional field API changes
4. ✅ **Type-safe payloads** - Compiler will catch all issues
5. ⚠️ **Large scope** - 48 files, but most are small test helpers
6. ⚠️ **Test validation** - Each service needs re-testing after migration

**Risk Assessment**:
- **Low Risk**: Business logic unchanged (ogen client is API-compatible)
- **Medium Risk**: Test breakage if I miss edge cases in assertions
- **Low Risk**: Easy to verify (compile + run tests per service)

---

## 📋 **RECOMMENDED APPROACH**

### Phase 1: Fix NT E2E Tests (PRIORITY)
**Goal**: Unblock NT E2E test run

**Files** (6 NT test files):
1. `test/unit/notification/audit_test.go`
2. `test/unit/notification/audit_adr032_compliance_test.go`
3. `test/integration/notification/controller_audit_emission_test.go`
4. `test/e2e/notification/01_notification_lifecycle_audit_test.go`
5. `test/e2e/notification/02_audit_correlation_test.go`
6. `test/e2e/notification/04_failed_delivery_audit_test.go`

**Time**: ~30 minutes (5 min per file)
**Confidence**: 90%

**Validation**:
```bash
# Recompile test binaries
go test -c test/unit/notification/audit_test.go
go test -c test/integration/notification/controller_audit_emission_test.go
go test -c test/e2e/notification/

# Run tests
make test-unit-notification
make test-integration-notification
make test-e2e-notification
```

---

### Phase 2: Fix AuthWebhook Dependencies (CRITICAL FOR E2E)
**Goal**: Fix AuthWebhook image build

AuthWebhook imports test helpers which import old client. Need to check:
1. `test/integration/authwebhook/helpers.go`
2. `test/integration/authwebhook/suite_test.go`
3. `test/e2e/authwebhook/authwebhook_e2e_suite_test.go`

**Time**: ~15 minutes
**Confidence**: 85%

---

### Phase 3: Remaining 39 Files (OPTIONAL - OUT OF SCOPE)
**Goal**: Complete ogen migration across all services

**Time**: ~2-3 hours (systematic file-by-file migration)
**Confidence**: 80%
**Decision**: **DEFER** - Not needed for NT service completion

---

## 🎯 **IMMEDIATE ACTION PLAN**

### Step 1: Migrate NT Test Files (6 files)
```bash
# 1. Update imports
sed -i 's|dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"|ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"|g' test/unit/notification/audit*.go
sed -i 's|dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"|ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"|g' test/integration/notification/controller_audit_emission_test.go
sed -i 's|dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"|ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"|g' test/e2e/notification/*audit*.go

# 2. Update type references
sed -i 's|dsgen\.|ogenclient.|g' test/unit/notification/audit*.go
sed -i 's|dsgen\.|ogenclient.|g' test/integration/notification/controller_audit_emission_test.go
sed -i 's|dsgen\.|ogenclient.|g' test/e2e/notification/*audit*.go
```

### Step 2: Fix Optional Fields (Manual)
For each file, replace:
- `*string` → `OptString` with `.SetTo()` / `.IsSet()` + `.Value`
- `ActorId` → `ActorID`
- `ResourceId` → `ResourceID`
- `CorrelationId` → `CorrelationID`

### Step 3: Fix AuthWebhook Test Helpers
```bash
sed -i 's|dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"|ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"|g' test/integration/authwebhook/*.go
sed -i 's|dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"|ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"|g' test/e2e/authwebhook/*.go
```

### Step 4: Compile & Validate
```bash
# Test NT unit tests compile
go test -c ./test/unit/notification/

# Test NT integration tests compile
go test -c ./test/integration/notification/

# Test AuthWebhook image builds
make test-e2e-notification  # Will trigger AuthWebhook build
```

---

## 🚨 **RISKS & MITIGATIONS**

### Risk 1: Test Assertions Fail
**Issue**: Ogen types might have different struct shapes than oapi-codegen
**Mitigation**: Compiler will catch all issues; tests validate behavior
**Likelihood**: Medium (20%)

### Risk 2: Missed Optional Field Conversions
**Issue**: `*string` → `OptString` conversions might be incomplete
**Mitigation**: Run linter + tests to catch issues
**Likelihood**: Low (10%)

### Risk 3: AuthWebhook Transitive Dependencies
**Issue**: AuthWebhook might import other services that use old client
**Mitigation**: Fix test helpers first (shared across all services)
**Likelihood**: Medium (30%)

---

## 📊 **FINAL CONFIDENCE ASSESSMENT**

| Aspect | Confidence | Rationale |
|--------|------------|-----------|
| **Business Code** | 100% | Already migrated ✅ |
| **NT Test Files (6)** | 90% | Clear pattern, small scope |
| **AuthWebhook Helpers (3)** | 85% | May have transitive deps |
| **E2E Test Success** | 80% | Depends on AuthWebhook fix |
| **Overall** | **85%** | High confidence, clear path |

---

## ✅ **RECOMMENDATION**

**Proceed with migration immediately**:
1. Start with NT test files (6 files, ~30 min)
2. Fix AuthWebhook helpers (3 files, ~15 min)
3. Run E2E tests (validate success)

**Total Time**: ~45-60 minutes
**Success Probability**: 85%

**If Blocked**: Request help on:
- Transitive dependencies in AuthWebhook
- Complex optional field conversions
- Edge cases in test assertions

---

**Status**: ✅ **READY TO PROCEED**
**Decision**: **FIX NOW** - Clear path to unblock NT E2E tests

