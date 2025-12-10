# ⚠️ NOTICE: Podman Stale Port Binding Fix - AUTHORITATIVE

**From**: Data Storage Team
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

## ✅ Solution: Pre-Flight Port Cleanup

### Makefile Target (AUTHORITATIVE)

Add this target to your service's Makefile **before** any Podman-based integration test targets:

```makefile
.PHONY: clean-podman-ports
clean-podman-ports: ## Clean stale Podman port bindings (fixes "proxy already running" errors)
	@echo "🧹 Cleaning stale Podman port bindings..."
	@# Kill any processes holding test ports (customize ports for your service)
	@lsof -ti:15433 2>/dev/null | xargs kill -9 2>/dev/null || true
	@lsof -ti:16379 2>/dev/null | xargs kill -9 2>/dev/null || true
	@lsof -ti:5432 2>/dev/null | xargs kill -9 2>/dev/null || true
	@lsof -ti:6379 2>/dev/null | xargs kill -9 2>/dev/null || true
	@# Remove any stale containers (customize names for your service)
	@podman rm -f datastorage-postgres datastorage-redis ai-redis 2>/dev/null || true
	@echo "✅ Port cleanup complete"
```

### Make Integration Test Depend on Cleanup

```makefile
.PHONY: test-integration-myservice
test-integration-myservice: clean-podman-ports ## Run MyService integration tests
	# ... your existing target ...
```

---

## 📦 Services That Need This Fix

| Service | Test Target | Ports Used | Status |
|---------|-------------|------------|--------|
| **Data Storage** | `test-integration-datastorage` | 5432, 15433 | ✅ Fixed |
| **AI Service** | `test-integration-ai` | 6379, 16379 | ⚠️ Needs fix |
| **Gateway** | `test-integration-gateway` | Various | ⚠️ Needs fix |
| **HolmesGPT API** | `test-integration-holmesgpt` | 5432, 6379 | ⚠️ Needs fix |
| **WorkflowExecution** | `test-integration-workflowexecution` | Various | ⚠️ Needs fix |

---

## 🛠️ Manual Recovery (When Tests Fail)

If tests fail with "proxy already running" and you need immediate recovery:

### Option 1: Kill Specific Port (Fast)

```bash
# Find and kill process holding port
lsof -ti:15433 | xargs kill -9
lsof -ti:16379 | xargs kill -9
```

### Option 2: Restart Podman Machine (Comprehensive)

```bash
podman machine stop && podman machine start
```

This takes ~30 seconds but guarantees a clean state.

### Option 3: Use Cleanup Target

```bash
make clean-podman-ports
```

---

## 🔍 Verification

After implementing the fix, verify with:

```bash
# Run integration tests twice in a row
make test-integration-datastorage
make test-integration-datastorage

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
| [ADR-016](../architecture/decisions/ADR-016-integration-test-infrastructure.md) | Podman-based test infrastructure design |
| [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc) | Testing strategy and infrastructure patterns |

---

## ✅ Acceptance Criteria

- [ ] All services add `clean-podman-ports` target to Makefile
- [ ] All `test-integration-*` targets depend on `clean-podman-ports`
- [ ] No more "proxy already running" failures in CI/CD

---

**Document Version**: 1.0
**Created**: December 10, 2025
**Maintained By**: Data Storage Team

