# ⚠️ NOTICE: Podman Stale Port Binding Fix - AUTHORITATIVE

**From**: Data Storage Team (updated by WorkflowExecution Team)
**To**: ALL Teams Using Podman Integration Tests
**Date**: December 10, 2025
**Priority**: 🟢 LOW (preventive fix)
**Status**: ✅ **IMPLEMENTED**

---

## 📋 Problem Summary

When running Podman-based integration tests, tests can fail with:

```
❌ Failed to start PostgreSQL: Error: "proxy already running"
```

Or:

```
⚠️  Ports 15433 or 16379 may be in use:
gvproxy 30754 jgil   16u  IPv6  TCP *:16379 (LISTEN)
gvproxy 30754 jgil   20u  IPv6  TCP *:15433 (LISTEN)
```

---

## 🔍 Root Cause

**`gvproxy`** is Podman's network proxy that forwards ports from containers to the host (on macOS). When:

1. A previous test run started containers (e.g., PostgreSQL on 15433, Redis on 16379)
2. The test was interrupted (Ctrl+C, crash, timeout, IDE restart, etc.)
3. Containers were removed but `gvproxy` kept the port bindings
4. Next test run tries to bind the same ports → **"proxy already running"**

This is a known issue with Podman machine on macOS where the proxy state gets out of sync with container state.

---

## ✅ Solution: Service-Specific Port Cleanup

### ⚠️ CRITICAL: Use Service-Specific Targets

**DO NOT** create a global `clean-podman-ports` target that kills all ports. Services share the `podman-compose.test.yml` infrastructure, and killing all ports will break other services.

**Pattern**: `clean-podman-ports-<servicename>`

Each service MUST have its **own** cleanup target with **service-specific ports only**.

---

### Service Port Mapping (Reference)

| Service | Ports Used | Container Names |
|---------|------------|-----------------|
| **WorkflowExecution** | 18090, 19090, 15433, 16379 | `kubernaut_datastorage_1`, `kubernaut_postgres_1`, `kubernaut_redis_1` |
| **Data Storage** | 5432, 15433 | `datastorage-postgres` |
| **AI Service** | 6379, 16379 | `ai-redis` |
| **Gateway** | 16379 | `kubernaut_redis_1` |
| **HolmesGPT API** | 8081, 15433, 16379 | `kubernaut_holmesgpt-api_1`, `kubernaut_postgres_1`, `kubernaut_redis_1` |

---

### Makefile Target Examples

#### WorkflowExecution Service ✅ IMPLEMENTED

```makefile
.PHONY: clean-podman-ports-workflowexecution
clean-podman-ports-workflowexecution: ## Clean stale Podman ports for WE tests only
	@echo "🧹 Cleaning stale Podman ports for WorkflowExecution tests..."
	@# WE uses: 18090 (DS HTTP), 19090 (DS Metrics), 15433 (PostgreSQL), 16379 (Redis)
	@lsof -ti:18090 2>/dev/null | xargs kill -9 2>/dev/null || true
	@lsof -ti:19090 2>/dev/null | xargs kill -9 2>/dev/null || true
	@lsof -ti:15433 2>/dev/null | xargs kill -9 2>/dev/null || true
	@lsof -ti:16379 2>/dev/null | xargs kill -9 2>/dev/null || true
	@podman rm -f kubernaut_datastorage_1 kubernaut_postgres_1 kubernaut_redis_1 2>/dev/null || true
	@echo "✅ WE port cleanup complete"
```

#### Data Storage Service

```makefile
.PHONY: clean-podman-ports-datastorage
clean-podman-ports-datastorage: ## Clean stale Podman ports for DS tests only
	@echo "🧹 Cleaning stale Podman ports for Data Storage tests..."
	@lsof -ti:5432 2>/dev/null | xargs kill -9 2>/dev/null || true
	@lsof -ti:15433 2>/dev/null | xargs kill -9 2>/dev/null || true
	@podman rm -f datastorage-postgres 2>/dev/null || true
	@echo "✅ DS port cleanup complete"
```

---

## 📦 Services Implementation Status

| Service | Cleanup Target | Status |
|---------|----------------|--------|
| **WorkflowExecution** | `clean-podman-ports-workflowexecution` | ✅ Implemented |
| **Data Storage** | `clean-podman-ports-datastorage` | ⚠️ Needs implementation |
| **AI Service** | `clean-podman-ports-ai` | ⚠️ Needs implementation |
| **Gateway** | `clean-podman-ports-gateway` | ⚠️ Needs implementation |
| **HolmesGPT API** | `clean-podman-ports-holmesgpt` | ⚠️ Needs implementation |

---

## 🛠️ Manual Recovery (When Tests Fail)

If tests fail with "proxy already running" and you need immediate recovery:

### Option 1: Use Service-Specific Cleanup Target (Recommended)

```bash
make clean-podman-ports-workflowexecution  # For WE tests
make clean-podman-ports-datastorage        # For DS tests
```

### Option 2: Kill Specific Port (Fast)

```bash
# Find and kill process holding specific port
lsof -ti:18090 | xargs kill -9  # Data Storage HTTP
lsof -ti:15433 | xargs kill -9  # PostgreSQL
```

### Option 3: Restart Podman Machine (Nuclear Option)

```bash
podman machine stop && podman machine start
```

This takes ~30 seconds but guarantees a clean state. Use only when service-specific cleanup doesn't work.

---

## 🔍 Verification

After implementing the fix, verify with:

```bash
# Run integration tests twice in a row
make test-integration-workflowexecution
make test-integration-workflowexecution

# Both should pass without "proxy already running" errors
```

---

## 📊 Technical Details

### Why This Happens on macOS

Podman on macOS runs containers in a Linux VM (`podman machine`). The `gvproxy` process handles port forwarding between:

```
Host (macOS) ←→ gvproxy ←→ Podman VM ←→ Container
```

When a container is removed without graceful shutdown, `gvproxy` may not release the port binding, causing conflicts on the next run.

### Why `lsof | xargs kill` Works

`lsof -ti:PORT` returns PIDs of processes using that port. On macOS, this is typically `gvproxy`. Killing it releases the port binding. Podman automatically restarts `gvproxy` when needed.

---

## 🔗 Related Documents

| Document | Purpose |
|----------|---------|
| [ADR-016](../architecture/decisions/ADR-016-SERVICE-SPECIFIC-INTEGRATION-TEST-INFRASTRUCTURE.md) | Podman-based test infrastructure design |
| [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc) | Testing strategy and infrastructure patterns |

---

## ✅ Acceptance Criteria

- [x] WorkflowExecution: `clean-podman-ports-workflowexecution` implemented
- [ ] Data Storage: `clean-podman-ports-datastorage` implemented
- [ ] AI Service: `clean-podman-ports-ai` implemented
- [ ] Gateway: `clean-podman-ports-gateway` implemented
- [ ] HolmesGPT API: `clean-podman-ports-holmesgpt` implemented
- [ ] No more "proxy already running" failures in CI/CD

---

**Document Version**: 1.1
**Created**: December 10, 2025
**Updated**: December 10, 2025 (Added service-specific guidance, WE implementation)
**Maintained By**: Data Storage Team + WorkflowExecution Team

