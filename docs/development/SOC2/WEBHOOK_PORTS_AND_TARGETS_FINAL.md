# Webhook Ports and Make Targets - FINAL CONFIGURATION
**Date**: January 6, 2026  
**Status**: ✅ **APPROVED & COMMITTED**  
**Version**: Final (DD-TEST-001 v2.1)

---

## ✅ **PORT ALLOCATIONS - NO CONFLICTS**

### **Integration Tests** (`test/integration/authwebhook/`)

| Component | Port | Purpose | Conflict Check |
|-----------|------|---------|----------------|
| **PostgreSQL** | 15442 | Audit event storage | ✅ Last available in 15433-15442 range |
| **Redis** | 16386 | Data Storage DLQ | ✅ Available between 16385 (Notification) and 16387 (HAPI) |
| **Data Storage** | 18099 | Audit API | ✅ Last available in 18090-18099 range |

### **E2E Tests** (`test/e2e/authwebhook/`)

| Component | Port | Purpose | Conflict Check |
|-----------|------|---------|----------------|
| **PostgreSQL** | 25442 | Audit event storage | ✅ Corresponding E2E port (+10000 offset) |
| **Redis** | 26386 | Data Storage DLQ | ✅ Corresponding E2E port (+10000 offset) |
| **Data Storage** | 28099 | Audit API | ✅ Corresponding E2E port (+10000 offset) |

---

## 🔧 **PORT COLLISION RESOLUTION**

### **Issue Identified**

Initial allocation in `WEBHOOK_MAKEFILE_TRIAGE.md`:
- ❌ PostgreSQL: 15435 (CONFLICT with RemediationOrchestrator)
- ❌ Redis: 16381 (CONFLICT with RemediationOrchestrator)
- ✅ Data Storage: 18099 (OK)

### **Resolution Applied**

Updated to use available ports:
- ✅ PostgreSQL: 15442 (last available in range)
- ✅ Redis: 16386 (available between 16385 and 16387)
- ✅ Data Storage: 18099 (already correct)

### **Verification**

Checked against DD-TEST-001 v2.1 collision matrix:
- ✅ No conflicts with any of the 9 services
- ✅ All services can run integration tests in parallel
- ✅ Webhook added to both integration and E2E collision matrices

---

## 📋 **SIMPLIFIED MAKE TARGETS**

### **Before** (Too Many - 10 targets)
```bash
test-unit-authwebhook
test-coverage-authwebhook                  # ❌ Redundant
test-integration-authwebhook
test-coverage-integration-authwebhook      # ❌ Redundant
test-e2e-authwebhook
test-coverage-e2e-authwebhook              # ❌ Redundant
test-all-authwebhook
test-coverage-all-authwebhook              # ❌ Redundant
clean-authwebhook-integration
```

### **After** (Simplified - 5 targets)
```bash
test-unit-authwebhook              # Coverage enabled by default
test-integration-authwebhook       # Coverage enabled by default
test-e2e-authwebhook               # Coverage enabled by default
test-all-authwebhook               # Runs all 3 tiers
clean-authwebhook-integration      # Cleanup
```

### **Key Changes**

1. **Coverage is Always Enabled**: Using `--cover --covermode=atomic` flags by default
2. **No Separate Coverage Targets**: Removed 4 redundant `-coverage` targets
3. **Matches Other Services**: Gateway, DataStorage, SignalProcessing use same pattern
4. **Simpler Usage**: No need to remember separate coverage commands

---

## 🎯 **FINAL MAKEFILE TARGETS**

```makefile
##@ Special Cases - Authentication Webhook

.PHONY: test-unit-authwebhook
test-unit-authwebhook: ginkgo ## Run authentication webhook unit tests
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Authentication Webhook - Unit Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_UNIT) --procs=$(TEST_PROCS) --cover --covermode=atomic ./test/unit/authwebhook/...

.PHONY: test-integration-authwebhook
test-integration-authwebhook: ginkgo ## Run webhook integration tests (envtest + real CRDs)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Authentication Webhook - Integration Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Pattern: DD-INTEGRATION-001 v2.0 (envtest + programmatic infrastructure)"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_INTEGRATION) --procs=$(TEST_PROCS) --cover --covermode=atomic --fail-fast ./test/integration/authwebhook/...

.PHONY: test-e2e-authwebhook
test-e2e-authwebhook: ginkgo ensure-coverdata ## Run webhook E2E tests (Kind cluster)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Authentication Webhook - E2E Tests (Kind cluster, $(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_E2E) --procs=$(TEST_PROCS) --cover --covermode=atomic ./test/e2e/authwebhook/...

.PHONY: test-all-authwebhook
test-all-authwebhook: ## Run all webhook test tiers (Unit + Integration + E2E)
	@echo "═══════════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Running ALL Authentication Webhook Tests (3 tiers)"
	@echo "═══════════════════════════════════════════════════════════════════════════════"
	@FAILED=0; \
	$(MAKE) test-unit-authwebhook || FAILED=$$((FAILED + 1)); \
	$(MAKE) test-integration-authwebhook || FAILED=$$((FAILED + 1)); \
	$(MAKE) test-e2e-authwebhook || FAILED=$$((FAILED + 1)); \
	if [ $$FAILED -gt 0 ]; then \
		echo "❌ $$FAILED test tier(s) failed"; \
		exit 1; \
	fi
	@echo "✅ All webhook test tiers completed successfully!"

.PHONY: clean-authwebhook-integration
clean-authwebhook-integration: ## Clean webhook integration test infrastructure
	@echo "🧹 Cleaning webhook integration infrastructure..."
	@podman stop authwebhook_postgres_1 authwebhook_redis_1 authwebhook_datastorage_1 2>/dev/null || true
	@podman rm authwebhook_postgres_1 authwebhook_redis_1 authwebhook_datastorage_1 2>/dev/null || true
	@podman network rm authwebhook_test-network 2>/dev/null || true
	@echo "✅ Cleanup complete"
```

---

## 📊 **USAGE EXAMPLES**

```bash
# Day 1: Unit tests (coverage automatic)
make test-unit-authwebhook

# Day 2-4: Integration tests (coverage automatic)
make test-integration-authwebhook

# Day 5-6: E2E tests (coverage automatic)
make test-e2e-authwebhook

# Run all test tiers
make test-all-authwebhook

# Clean up integration infrastructure
make clean-authwebhook-integration

# Coverage reports are automatically generated in:
# - test/unit/authwebhook/coverprofile.txt
# - test/integration/authwebhook/coverprofile.txt
# - test/e2e/authwebhook/coverprofile.txt
```

---

## ✅ **COMPLIANCE VERIFICATION**

| Requirement | Status | Evidence |
|-------------|--------|----------|
| **DD-TEST-001 v2.1** | ✅ | Webhook ports added to authoritative document |
| **No Port Conflicts** | ✅ | Verified against collision matrix (9 services) |
| **DD-TEST-002** (Parallel) | ✅ | `--procs=$(TEST_PROCS)` in all targets |
| **Coverage by Default** | ✅ | `--cover --covermode=atomic` in all targets |
| **Pattern Consistency** | ✅ | Matches Gateway/DataStorage/SignalProcessing |
| **Simplified Targets** | ✅ | 5 targets (down from 10) |

---

## 📝 **AUTHORITATIVE REFERENCES**

- **Port Allocations**: `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` v2.1
- **Makefile Targets**: `docs/development/SOC2/WEBHOOK_MAKEFILE_IMPLEMENTATION_APPROVED.md`
- **Testing Guidelines**: `docs/development/business-requirements/TESTING_GUIDELINES.md`
- **Parallel Execution**: `docs/architecture/decisions/DD-TEST-002-parallel-test-execution-standard.md`

---

## 🎯 **READY FOR IMPLEMENTATION**

✅ **All documents updated and committed**  
✅ **Port allocations verified conflict-free**  
✅ **Make targets simplified to match existing patterns**  
✅ **Coverage enabled by default for all tiers**  
✅ **DD-TEST-001 v2.1 is now authoritative for webhook ports**

**Next Step**: Add make targets to `Makefile` and begin TDD Day 1 (unit tests)

---

**Status**: ✅ **COMPLETE**  
**Committed**: Git commit `229cd9ffe`  
**Date**: 2026-01-06

