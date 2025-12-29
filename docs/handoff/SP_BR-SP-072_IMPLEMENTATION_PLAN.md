# SignalProcessing BR-SP-072 Full Implementation Plan

**Date**: 2025-12-13
**Decision**: Option 1 - Complete Full BR-SP-072 Implementation
**Estimated Duration**: 8-12 hours
**Status**: 🔄 IN PROGRESS

---

## 📊 IMPLEMENTATION OVERVIEW

### Scope
Implement complete ConfigMap hot-reload for all SignalProcessing Rego policies:
1. ✅ Priority Engine (`priority.rego`)
2. ✅ Environment Classifier (`environment.rego`)
3. ✅ CustomLabels Engine (`labels.rego`)
4. ✅ Policy validation and graceful degradation
5. ✅ Component API exposure for testing

### Success Criteria
- All 69 integration tests passing
- ConfigMap updates reload policies within ~60s
- Invalid policies gracefully degraded (keep previous)
- Component APIs accessible for testing

---

## 🎯 PHASE 1: FileWatcher Integration (4 hours)

### Phase 1.1: Priority Engine Hot-Reload (2h)
**Status**: 🔄 IN PROGRESS
**Files Modified**:
- `pkg/signalprocessing/classifier/priority_engine.go`
- `cmd/signalprocessing/main.go`

**Tasks**:
1. [ ] Add `ReloadPolicy(content string) error` method to `PriorityEngine`
2. [ ] Add `sync.RWMutex` for thread-safe policy access
3. [ ] Update `AssignPriority` to use read lock
4. [ ] Wire up `FileWatcher` in `main.go` for `/etc/kubernaut/policies/priority.rego`
5. [ ] Start FileWatcher in background goroutine
6. [ ] Test hot-reload manually with ConfigMap update

**Expected Outcome**:
- Priority policy reloads on ConfigMap change
- Old policy retained if new policy invalid
- No crashes during reload

---

### Phase 1.2: Environment Classifier Hot-Reload (1h)
**Status**: ⏸️ PENDING
**Files Modified**:
- `pkg/signalprocessing/classifier/environment.go`
- `cmd/signalprocessing/main.go`

**Tasks**:
1. [ ] Add `ReloadPolicy(content string) error` method to `EnvironmentClassifier`
2. [ ] Add `sync.RWMutex` for thread-safe policy access
3. [ ] Update `Classify` to use read lock
4. [ ] Wire up `FileWatcher` in `main.go` for `/etc/kubernaut/policies/environment.rego`
5. [ ] Start FileWatcher in background goroutine
6. [ ] Test hot-reload manually

**Expected Outcome**:
- Environment policy reloads on ConfigMap change
- ConfigMap fallback still works

---

### Phase 1.3: CustomLabels Engine Hot-Reload (1h)
**Status**: ⏸️ PENDING
**Files Modified**:
- `pkg/signalprocessing/rego/engine.go`
- `cmd/signalprocessing/main.go`

**Tasks**:
1. [ ] `LoadPolicy` already exists in `rego.Engine` - just needs validation
2. [ ] Add `ReloadPolicy(content string) error` wrapper if needed
3. [ ] Wire up `FileWatcher` in `main.go` for `/etc/kubernaut/policies/labels.rego`
4. [ ] Start FileWatcher in background goroutine
5. [ ] Test hot-reload manually

**Expected Outcome**:
- CustomLabels policy reloads on ConfigMap change
- Security filtering preserved after reload

---

## 🔧 PHASE 2: Policy Validation & Graceful Degradation (3 hours)

### Phase 2.1: Rego Policy Validation (1.5h)
**Status**: ⏸️ PENDING
**Files Modified**:
- `pkg/signalprocessing/classifier/priority_engine.go`
- `pkg/signalprocessing/classifier/environment.go`
- `pkg/signalprocessing/rego/engine.go`

**Tasks**:
1. [ ] Add `ValidatePolicy(content string) error` to each engine
2. [ ] Implement Rego compilation test without executing
3. [ ] Return descriptive errors for parse failures
4. [ ] Log validation errors with policy hash
5. [ ] Test with intentionally broken Rego policies

**Expected Outcome**:
- Invalid policies rejected during reload
- Detailed error messages logged
- No crashes on bad Rego syntax

---

### Phase 2.2: Graceful Degradation (1.5h)
**Status**: ⏸️ PENDING
**Files Modified**:
- `cmd/signalprocessing/main.go` (FileWatcher callbacks)

**Tasks**:
1. [ ] FileWatcher callbacks call `ValidatePolicy` first
2. [ ] If validation fails, return error (keeps previous policy)
3. [ ] Log validation failure with hash
4. [ ] If validation succeeds, call `ReloadPolicy`
5. [ ] Test with valid→invalid→valid policy sequence

**Expected Outcome**:
- Invalid policies don't crash controller
- Previous policy retained on validation failure
- "policy reloaded" vs "policy rejected" logged clearly

---

## 🧩 PHASE 3: Component API Exposure (2 hours)

**Status**: ⏸️ PENDING
**Files Modified**:
- `pkg/signalprocessing/classifier/priority_engine.go`
- `pkg/signalprocessing/classifier/environment.go`
- `pkg/signalprocessing/classifier/business.go`
- `pkg/signalprocessing/ownerchain/builder.go`

**Tasks**:
1. [ ] Export `PriorityEngine.AssignPriority()` if not already public
2. [ ] Export `EnvironmentClassifier.Classify()` if not already public
3. [ ] Export `BusinessClassifier.Classify()` if not already public
4. [ ] Export `Builder.BuildOwnerChain()` from ownerchain package
5. [ ] Update integration tests to use exported APIs
6. [ ] Run component integration tests

**Expected Outcome**:
- Component APIs accessible for direct testing
- 3 component integration test failures resolved

---

## 🧪 PHASE 4: Test Fixes & Validation (2 hours)

### Phase 4.1: Hot-Reload Test Fixes (1h)
**Status**: ⏸️ PENDING
**Files Modified**:
- `test/integration/signalprocessing/hot_reloader_test.go`

**Tasks**:
1. [ ] Update tests to use actual ConfigMap volume mounts
2. [ ] Fix test expectations for ~60s reload latency
3. [ ] Add test cleanup for FileWatcher goroutines
4. [ ] Run hot-reload tests: `ginkgo --focus="Hot-Reload" ./test/integration/signalprocessing/`
5. [ ] Fix any remaining failures

**Expected Outcome**:
- 4/4 hot-reload tests passing
- Tests clean up resources properly

---

### Phase 4.2: Rego Integration Test Fixes (1h)
**Status**: ⏸️ PENDING
**Files Modified**:
- `test/integration/signalprocessing/rego_integration_test.go`

**Tasks**:
1. [ ] Update ConfigMap policy loading tests for hot-reload
2. [ ] Fix CustomLabels extraction tests
3. [ ] Fix system prefix protection tests
4. [ ] Fix policy fallback tests
5. [ ] Fix key/value truncation tests

**Expected Outcome**:
- 5/5 rego integration test failures resolved

---

### Phase 4.3: Full Test Suite Validation (30min)
**Status**: ⏸️ PENDING

**Tasks**:
1. [ ] Run full integration test suite: `ginkgo ./test/integration/signalprocessing/...`
2. [ ] Verify 67/69 passing (2 pre-existing audit failures expected)
3. [ ] Run unit tests: `ginkgo ./test/unit/signalprocessing/...`
4. [ ] Verify 194/194 passing (1 pre-existing priority engine failure expected)
5. [ ] Document any remaining failures

**Expected Outcome**:
- Integration: 67/69 passing (94% → 97%)
- Unit: 194/194 passing (99.5%)
- E2E: Run separately after Podman machine fixed

---

## 📋 IMPLEMENTATION CHECKLIST

### Prerequisites
- [x] `pkg/shared/hotreload/FileWatcher` exists and tested
- [x] DD-INFRA-001 documentation exists
- [x] ConfigMap deployment guide created
- [x] Tests enabled (pending-v2 removed)

### Phase 1: FileWatcher Integration
- [ ] Priority Engine hot-reload
- [ ] Environment Classifier hot-reload
- [ ] CustomLabels Engine hot-reload

### Phase 2: Validation & Graceful Degradation
- [ ] Policy validation methods
- [ ] FileWatcher callbacks with validation
- [ ] Error logging and metrics

### Phase 3: Component API Exposure
- [ ] Export PriorityEngine API
- [ ] Export EnvironmentClassifier API
- [ ] Export BusinessClassifier API
- [ ] Export OwnerChainBuilder API

### Phase 4: Test Fixes
- [ ] Hot-reload tests passing
- [ ] Rego integration tests passing
- [ ] Full test suite validated

---

## 🔍 VERIFICATION STEPS

After each phase, verify:

### Phase 1 Verification
```bash
# Start controller
go run ./cmd/signalprocessing/main.go

# In another terminal, update ConfigMap
kubectl edit configmap kubernaut-rego-policies -n kubernaut-system

# Check logs for "policy reloaded"
kubectl logs -f signalprocessing-controller | grep "policy reloaded"
```

### Phase 2 Verification
```bash
# Apply invalid policy
kubectl apply -f invalid-policy-configmap.yaml

# Check logs for "policy rejected"
kubectl logs -f signalprocessing-controller | grep "policy rejected"

# Verify controller still works
kubectl get signalprocessings
```

### Phase 3 Verification
```bash
# Run component tests
ginkgo --focus="Component Integration" ./test/integration/signalprocessing/

# Verify APIs are accessible
go test -v ./pkg/signalprocessing/classifier/... -run TestPriorityEngineAPI
```

### Phase 4 Verification
```bash
# Run full integration suite
ginkgo ./test/integration/signalprocessing/...

# Expected: 67/69 passing (2 pre-existing audit failures)
```

---

## 📊 PROGRESS TRACKING

| Phase | Task | Status | Duration | Completion |
|-------|------|--------|----------|------------|
| 1.1 | Priority Engine hot-reload | 🔄 IN PROGRESS | 0/2h | 0% |
| 1.2 | Environment hot-reload | ⏸️ PENDING | 0/1h | 0% |
| 1.3 | CustomLabels hot-reload | ⏸️ PENDING | 0/1h | 0% |
| 2.1 | Policy validation | ⏸️ PENDING | 0/1.5h | 0% |
| 2.2 | Graceful degradation | ⏸️ PENDING | 0/1.5h | 0% |
| 3 | Component APIs | ⏸️ PENDING | 0/2h | 0% |
| 4.1 | Hot-reload tests | ⏸️ PENDING | 0/1h | 0% |
| 4.2 | Rego integration tests | ⏸️ PENDING | 0/1h | 0% |
| 4.3 | Full test validation | ⏸️ PENDING | 0/0.5h | 0% |
| **TOTAL** | | | **0/12h** | **0%** |

---

## 🚨 RISKS & MITIGATION

### Risk 1: Context Window Limit
**Mitigation**: This plan document tracks progress. Resume from last completed phase.

### Risk 2: Unexpected Test Failures
**Mitigation**: Each phase has verification steps. Fix issues before moving to next phase.

### Risk 3: Performance Impact
**Mitigation**: Use `sync.RWMutex` for minimal read-path overhead. Profile if needed.

### Risk 4: Breaking Changes
**Mitigation**: Graceful degradation ensures controller keeps running even with bad policies.

---

## 📝 NOTES

### Design Decisions
- Using `pkg/shared/hotreload/FileWatcher` (DD-INFRA-001)
- File-based ConfigMap watch (~60s latency acceptable)
- Graceful degradation on invalid policies
- `sync.RWMutex` for thread-safe policy access

### Test Strategy
- Manual verification after each phase
- Automated integration tests in Phase 4
- Accept 2 pre-existing audit failures (already triaged)

### Deployment Notes
- Requires ConfigMap volume mounts in deployment
- Policy files mounted at `/etc/kubernaut/policies/`
- FileWatcher started in background goroutines

---

**Last Updated**: 2025-12-13 14:30 PST
**Next Action**: Start Phase 1.1 - Priority Engine Hot-Reload


