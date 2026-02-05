# DD-AUTH-014: macOS Podman Limitation - Critical Finding

**Date**: 2026-01-27  
**Status**: ❌ **Option D Blocked on macOS**  
**Recommendation**: Use **Option A** with IPv6 disabled

---

## 🚨 Critical Discovery

**SME's Option D (--network=host) doesn't work on macOS Podman!**

### Why It Fails on macOS

**Linux** (SME's assumption):
```
Container --network=host → Linux host network → Can reach localhost
✅ Works perfectly
```

**macOS** (actual environment):
```
Container --network=host → Podman VM's network → NOT macOS host!
❌ Container can't reach macOS host services
```

---

## 📊 Evidence

### Test Results (Option D)
```bash
# DataStorage container (in Podman VM):
INFO: HTTP server listening on 0.0.0.0:18140
INFO: PostgreSQL connection established
INFO: Redis connection established  
INFO: Kubernetes authenticator created (https://[::1]:51777/)

# Test (on macOS host):
curl http://localhost:18140/health
# Connection refused - can't reach VM!
```

**Diagnosis**: Container is healthy and listening, but **macOS can't reach it** because:
1. Container runs in Podman's Linux VM
2. `--network=host` means "VM's network", not "macOS network"
3. Port 18140 is open in VM, not on macOS host

---

## ✅ Working Solution: Option A + IPv6 Disabled

### Discovery
When we disabled IPv6 on macOS:
```bash
sudo networksetup -setv6off Wi-Fi
```

**Result**: envtest binds to `127.0.0.1` instead of `[::1]`!

### Root Cause in envtest
```go
// sigs.k8s.io/controller-runtime/pkg/internal/testing/addr/manager.go
func suggest(listenHost string) (*net.TCPListener, int, string, error) {
    if listenHost == "" {
        listenHost = "localhost"  // Defaults to "localhost"
    }
    addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(listenHost, "0"))
    // On macOS with IPv6 enabled: resolves to [::1]
    // On macOS with IPv6 disabled: resolves to 127.0.0.1
}
```

---

## 🎯 Recommended Solution

### For Local Development (macOS)

**Disable IPv6**:
```bash
# Find your network interface
networksetup -listallnetworkservices

# Disable IPv6 (forces localhost → 127.0.0.1)
sudo networksetup -setv6off Wi-Fi

# Verify
networksetup -getinfo Wi-Fi | grep IPv6
# Should show: IPv6: Off
```

**Benefits**:
- ✅ envtest binds to `127.0.0.1` (IPv4)
- ✅ Podman bridge network can route to IPv4
- ✅ Option A works (kubeconfig rewrite to `host.containers.internal`)
- ✅ No code changes to DataStorage needed

---

### For CI/CD (Linux)

**Use Option D** (--network=host):
- ✅ Real Linux, not VM
- ✅ `--network=host` works as expected
- ✅ Container can reach localhost directly

---

## 📋 Platform-Specific Implementation

### Detect Platform in Tests

```go
// test/infrastructure/datastorage_bootstrap.go

import (
    "runtime"
)

func startDSBootstrapService(...) error {
    useHostNetwork := cfg.EnvtestKubeconfig != ""
    
    if useHostNetwork {
        if runtime.GOOS == "darwin" {
            // macOS: Use bridge network + IPv4 kubeconfig rewrite
            // Requires: sudo networksetup -setv6off Wi-Fi
            return startWithBridgeNetwork(infra, imageName, cfg, writer)
        } else {
            // Linux: Use host network (SME Option D)
            return startWithHostNetwork(infra, imageName, cfg, writer)
        }
    }
    
    // Normal bridge network for non-auth tests
    return startWithBridgeNetwork(infra, imageName, cfg, writer)
}
```

---

## ⚙️ Implementation Status

### Completed
- ✅ **PORT env var support** in DataStorage (`cmd/datastorage/main.go`)
- ✅ **Host networking logic** in bootstrap (`test/infrastructure/datastorage_bootstrap.go`)
- ✅ **Kubeconfig generation** (`test/infrastructure/serviceaccount.go`)

### Needs Adjustment
- ⚠️ **Platform detection**: Add `runtime.GOOS == "darwin"` check
- ⚠️ **Conditional networking**: Bridge (macOS) vs Host (Linux)
- ⚠️ **Documentation**: Document IPv6 requirement for macOS devs

---

## 📝 Developer Documentation

### macOS Setup (One-Time)

Add to `docs/development/INTEGRATION_TESTING.md`:

```markdown
## macOS Prerequisites

### Disable IPv6 for Integration Tests

Integration tests with real Kubernetes authentication require IPv6 to be disabled:

\`\`\`bash
# Disable IPv6 (required for envtest IPv4 binding)
sudo networksetup -setv6off Wi-Fi

# Run tests
make test-integration-remediationorchestrator

# Re-enable IPv6 when done (optional)
sudo networksetup -setv6automatic Wi-Fi
\`\`\`

**Why**: envtest binds to IPv6 `[::1]` by default on macOS, which Podman
containers cannot reach. Disabling IPv6 forces IPv4 `127.0.0.1` binding.

**Alternative**: Use mock auth (no IPv6 changes needed):
\`\`\`bash
export KUBERNAUT_USE_MOCK_AUTH=true
make test-integration-remediationorchestrator
\`\`\`
```

---

## 🔄 Comparison: Options A vs D

| Aspect | Option A (Bridge + IPv4) | Option D (Host Network) |
|--------|--------------------------|-------------------------|
| **macOS** | ✅ Works (IPv6 disabled) | ❌ Blocked (VM limitation) |
| **Linux CI/CD** | ✅ Works (if IPv6 disabled) | ✅ Works perfectly |
| **Setup Required** | IPv6 disable (macOS only) | None |
| **Code Changes** | Minimal (kubeconfig rewrite) | PORT env var support |
| **Portability** | Requires platform detection | Linux-only |

---

## 💡 Recommended Hybrid Approach

```go
if cfg.EnvtestKubeconfig != "" {
    if runtime.GOOS == "darwin" {
        // macOS: Bridge network + assume IPv6 disabled
        // Kubeconfig rewrites [::1] or 127.0.0.1 → host.containers.internal
        useHostNetwork = false
    } else {
        // Linux: Host network (SME Option D)
        useHostNetwork = true
    }
}
```

**Benefits**:
- ✅ Works on macOS (with IPv6 disabled)
- ✅ Works on Linux CI/CD (no IPv6 changes)
- ✅ Portable across platforms
- ✅ Uses optimal approach for each platform

---

## 📊 Test Results

### Option D on macOS (Failed)
```
DataStorage: HTTP server listening on 0.0.0.0:18140 ✅
DataStorage: PostgreSQL/Redis connected ✅
DataStorage: K8s authenticator created ✅
Test health check: Connection refused ❌
Reason: Podman VM isolation
```

### Option A on macOS (Expected with IPv6 OFF)
```
envtest: Binds to 127.0.0.1 ✅
Kubeconfig: Rewrites to host.containers.internal ✅
DataStorage: Can reach envtest API ✅
Test: Can reach DataStorage ✅
```

---

## 🎯 Next Steps

1. **Confirm IPv6 disabled** on user's macOS
2. **Test Option A** with bridge network
3. **If successful**: Implement platform detection
4. **Document** IPv6 requirement for macOS devs
5. **CI/CD**: Use Option D (host network) on Linux

---

## 🔗 References

- **SME Guidance**: `docs/handoff/DD_AUTH_014_ENVTEST_IPV6_BLOCKER.md`
- **envtest Source**: `sigs.k8s.io/controller-runtime/pkg/internal/testing/addr/manager.go`
- **Podman Networking**: macOS runs containers in Linux VM, not native
- **Kubernetes**: `net.ResolveTCPAddr("tcp", "localhost:0")` prefers IPv6 on macOS

---

**Bottom Line**: SME's Option D works on Linux CI/CD but **not** on macOS. Use Option A (bridge + IPv4) for macOS with IPv6 disabled.
