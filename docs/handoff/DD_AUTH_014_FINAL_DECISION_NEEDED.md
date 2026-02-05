# DD-AUTH-014: Final Decision Required - envtest IPv6 Cannot Be Overridden

**Date**: 2026-01-27  
**Status**: 🚫 **BLOCKED - Architecture Decision Required**  
**Time Invested**: ~6 hours of troubleshooting

---

## 🚨 **Bottom Line**

**envtest IPv6 binding cannot be overridden on macOS** - tried 7 different approaches, all failed.

**Tests**: 46/59 passing (78%), blocked by auth networking

**Decision Required**: Accept limitation or fundamentally change approach?

---

## 📊 **What We Tried (All Failed)**

### 1. Environment Variable (`TEST_ASSET_KUBE_APISERVER_BIND_ADDRESS`) ❌
```go
os.Setenv("TEST_ASSET_KUBE_APISERVER_BIND_ADDRESS", "0.0.0.0")
```
**Result**: envtest ignored it, still bound to `[::1]`

### 2. APIServer Args Override ❌
```go
APIServer: &envtest.APIServer{
    Args: []string{"--bind-address=0.0.0.0"},
}
```
**Result**: kube-apiserver failed to start (timeout after 179s)

### 3. Kubeconfig URL Rewrite ❌
```go
containerAPIServer := strings.Replace(cfg.Host, "[::1]", "host.containers.internal", 1)
```
**Result**: `host.containers.internal:PORT` not reachable (`connection refused`)

### 4. Podman `--network=host` ❌
```go
args = append(args, "--network", "host")
```
**Result**: On macOS, `--network=host` means "Podman VM's network", not macOS host  
**Impact**: macOS can't reach container, container can't reach macOS services

### 5. Disable System IPv6 ❌
```bash
sudo networksetup -setv6off Wi-Fi
```
**Result**: Go's `net.Resolve` TCP still resolves "localhost" to `[::1]`  
**Evidence**: `go run test: "localhost" → ::1` (even with IPv6 OFF)

### 6. Pre-Set SecureServing.Address ❌
```go
APIServer: &envtest.APIServer{
    SecureServing: envtest.SecureServing{
        ListenAddr: envtest.ListenAddr{
            Address: "127.0.0.1",
        },
    },
}
```
**Result**: envtest **STILL** bound to `[::1]`, ignored our setting  
**Evidence**: `ps aux` shows `--bind-address=::1` in latest test run

### 7. PORT Environment Variable (Option D) ❌
```go
// cmd/datastorage/main.go
if portEnv := os.Getenv("PORT"); portEnv != "" {
    cfg.Server.Port = port
}
```
**Result**: Port override works, but `--network=host` blocked by Podman VM isolation on macOS

---

## 🔍 **Root Cause Analysis**

### Why envtest Binds to IPv6

**Source Code**: `sigs.k8s.io/controller-runtime/pkg/internal/testing/addr/manager.go`

```go
func suggest(listenHost string) (*net.TCPListener, int, string, error) {
    if listenHost == "" {
        listenHost = "localhost"  // ← Defaults to "localhost"
    }
    addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(listenHost, "0"))
    // On macOS: Go ALWAYS resolves "localhost" to [::1], even with IPv6 disabled
    l, err := net.ListenTCP("tcp", addr)
    return l, l.Addr().(*net.TCPAddr).Port, addr.IP.String(), nil
}
```

**The Chain**:
1. envtest calls `addr.Suggest("")`
2. Defaults to `"localhost"`
3. Go's DNS resolver prefers IPv6 on macOS **regardless of system IPv6 state**
4. Returns `"::1"` as the address
5. Becomes `--bind-address=::1` in kube-apiserver

**Attempted Overrides**: All ignored by envtest's default configuration logic

---

## ⚖️ **Decision Framework**

### Option X: Accept Limitation (Recommended)

**Approach**: Use mock auth in integration tests, real auth in E2E tests

**Rationale**:
- Integration tests focus on **business logic**, not auth
- E2E tests (Kind cluster) validate auth properly
- No infrastructure complexity
- Works universally (macOS, Linux, CI/CD)

**Implementation**:
```go
// Phase 2: Use MockUserTransport (no envtest needed)
// E2E tests will validate real auth in Kind cluster

dsClients := integration.NewAuthenticatedDataStorageClients(
    dataStorageBaseURL,
    "mock-user", // Mock token (no K8s validation)
    5*time.Second,
)
```

**Pros**:
- ✅ Works immediately (no networking issues)
- ✅ Faster test execution (no envtest startup)
- ✅ Simpler infrastructure (no kubeconfig management)
- ✅ Aligns with testing best practices (test one thing at a time)

**Cons**:
- ⚠️ Integration tests don't validate real K8s auth (but E2E does)
- ⚠️ Less "real" environment

---

### Option Y: Wait for envtest IPv4 Support

**Approach**: Monitor controller-runtime for IPv4 binding option

**Rationale**: This is an envtest limitation, not our code

**Timeline**: Uncertain (weeks to months?)

**Pros**:
- ✅ Eventually solves it "properly"

**Cons**:
- ❌ Blocks all services indefinitely
- ❌ May never be implemented
- ❌ Out of our control

---

### Option Z: Fork/Patch envtest (Not Recommended)

**Approach**: Maintain custom envtest fork with IPv4 binding

**Pros**:
- ✅ Full control

**Cons**:
- ❌ Maintenance burden
- ❌ Complex setup
- ❌ Breaks on updates
- ❌ Not worth the effort for integration tests

---

## 📈 **Impact Assessment**

### What Works Today
- ✅ **46/59 tests passing** (78% success rate)
- ✅ **Shared infrastructure** ready (ServiceAccount creation, token injection)
- ✅ **DataStorage auth middleware** working correctly
- ✅ **E2E tests** will work perfectly (Kind cluster has normal networking)

### What's Blocked
- ❌ **13 audit tests** failing with `401 Unauthorized`
- ❌ Real K8s auth in integration tests (macOS only)
- ❌ Full auth validation before E2E

---

## 🎯 **Recommendation: Option X (Accept Limitation)**

**Use mock auth for integration tests**, real auth for E2E:

### Why This Makes Sense

1. **Testing Philosophy**:
   - **Integration**: Test business logic with controlled inputs
   - **E2E**: Test full system including auth (Kind cluster)

2. **Practical Benefits**:
   - ✅ Works on all platforms immediately
   - ✅ Faster integration test execution
   - ✅ Simpler infrastructure

3. **Coverage**:
   - **Unit tests**: Auth middleware logic (pure Go)
   - **Integration tests**: Business logic (mock auth)
   - **E2E tests**: Full system (real K8s auth in Kind)

### Implementation Path

**Keep all the infrastructure we built** (not wasted!):
- ✅ `test/infrastructure/serviceaccount.go` → Use in E2E tests
- ✅ `test/shared/integration/datastorage_auth.go` → Update to support mock mode
- ✅ `cmd/datastorage/main.go` PORT override → Useful for other scenarios

**Add mock mode**:
```go
// test/shared/integration/datastorage_auth.go

func NewAuthenticatedDataStorageClients(baseURL, token string, timeout time.Duration, useMockAuth bool) *AuthenticatedDataStorageClients {
    var transport http.RoundTripper
    
    if useMockAuth {
        transport = testauth.NewMockUserTransport("mock-user")
    } else {
        transport = testauth.NewServiceAccountTransport(token)
    }
    
    // ... rest of implementation
}
```

---

## 📝 **Code Investment Summary**

### What We Built (Reusable for E2E)
- **480 lines** of shared infrastructure
- **ServiceAccount + RBAC** creation (`test/infrastructure/serviceaccount.go`)
- **Authenticated clients** helper (`test/shared/integration/datastorage_auth.go`)
- **Container bootstrap** with auth support (`test/infrastructure/datastorage_bootstrap.go`)
- **PORT env var** support (`cmd/datastorage/main.go`)

### What We Learned
- envtest IPv6 binding cannot be overridden (tried 7 approaches)
- Podman `--network=host` doesn't work on macOS (VM isolation)
- Go's DNS resolver prefers IPv6 regardless of system configuration
- Integration vs E2E testing boundaries

---

## ⏭️ **Next Steps Based on Decision**

### If Option X (Accept Limitation) - **5 minutes**
1. Update `NewAuthenticatedDataStorageClients` to support mock mode
2. Update RemediationOrchestrator suite to use mock auth
3. Validate 59/59 tests pass
4. Document approach for other services

### If Option Y (Wait for envtest) - **Indefinite**
1. Open issue with controller-runtime project
2. Monitor for IPv4 binding support
3. Use mocks in the meantime

### If Option Z (Fork envtest) - **Weeks**
1. Fork controller-runtime
2. Patch addr.Suggest to use "127.0.0.1"
3. Maintain fork forever
4. Not recommended

---

## 🎯 **My Strong Recommendation**

**Choose Option X** because:

1. **Testing best practices**: Integration tests shouldn't test auth (that's what E2E is for)
2. **Pragmatic**: 6 hours invested, fundamental platform limitation discovered
3. **Not wasted**: All infrastructure reusable for E2E tests (Kind cluster)
4. **Fast resolution**: 5 minutes to implement vs weeks/months for alternatives
5. **Universal**: Works on macOS, Linux, CI/CD without platform detection

**The 480 lines of infrastructure we built are NOT wasted** - they're **perfect for E2E tests** where Kind cluster has normal networking!

---

## 📞 **Decision Required From User**

**Question**: Should we:
- **A**: Use mock auth for integration tests, real auth for E2E (5 min to implement)
- **B**: Wait for envtest IPv4 support (indefinite timeline)
- **C**: Continue investigating (uncertain if solvable)

**My recommendation**: **Option A** - pragmatic, follows testing best practices, unblocks all services immediately.

---

## 📚 **Related Documentation**

- `DD_AUTH_014_ENVTEST_IPV6_BLOCKER.md` - SME guidance + original analysis
- `DD_AUTH_014_MACOS_PODMAN_LIMITATION.md` - Podman VM limitation discovery
- `DD_AUTH_014_SESSION_STATUS_IPV6_BLOCKER.md` - Session summary

---

**Time to make a call**: Continue fighting envtest, or accept limitation and move forward? 🤔
