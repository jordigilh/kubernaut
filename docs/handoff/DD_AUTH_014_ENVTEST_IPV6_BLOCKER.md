# DD-AUTH-014: envtest IPv6 Connectivity Blocker - SME Assistance Needed

**Date**: 2026-01-27  
**Status**: 🚫 BLOCKED - Need SME guidance  
**Importance**: HIGH - Blocks real K8s auth in integration tests for all services

---

## 📋 Executive Summary

**Problem**: DataStorage container (Podman) cannot reach envtest API server on host due to IPv6 binding issue.

**Impact**: Integration tests cannot use real Kubernetes authentication (TokenReview + SAR). Must either:
- Use mock auth (less realistic)
- Run DataStorage as native binary (diverges from E2E/production)
- Solve this networking issue (preferred - reusable across all 7+ services)

**Ask**: How to enable Podman container (bridge network) to reach host's envtest API server bound to IPv6 `[::1]`?

---

## 🎯 What We Need

### Goal
Enable DataStorage Podman container to call envtest Kubernetes API server running on host for TokenReview/SAR validation.

### Requirements
1. **Containerized DataStorage** (not native binary)
2. **Real Kubernetes auth** (not mocks)
3. **envtest** (not full Kind cluster - too heavy for integration tests)
4. **Podman bridge network** (normal test isolation)
5. **Reusable solution** (same approach for all services)

### Success Criteria
```bash
# From inside DataStorage container:
curl -k https://HOST_ADDRESS:PORT/apis/authentication.k8s.io/v1/tokenreviews
# Should succeed (not "connection refused")
```

---

## 🔍 Root Cause Analysis

### Current Architecture
```
┌─────────────────────────────────────────────────┐
│ Host (macOS darwin 24.6.0)                      │
│                                                  │
│  ┌──────────────────┐                           │
│  │ envtest          │ ← Binds to [::1]:RANDOM   │
│  │ kube-apiserver   │   (IPv6 localhost)        │
│  └──────────────────┘                           │
│         ↑                                        │
│         │ ❌ connection refused                  │
│         │                                        │
│  ┌──────┴───────────────────────────────────┐   │
│  │ Podman Bridge Network                    │   │
│  │                                          │   │
│  │  ┌──────────────────────────────────┐   │   │
│  │  │ DataStorage Container            │   │   │
│  │  │ Tries: https://[::1]:PORT        │   │   │
│  │  │ Result: dial tcp connection      │   │   │
│  │  │         refused                  │   │   │
│  │  └──────────────────────────────────┘   │   │
│  └──────────────────────────────────────────┘   │
└─────────────────────────────────────────────────┘
```

### The IPv6 Issue
- **envtest binds to IPv6** `[::1]:PORT` by default
- **Podman bridge network** cannot route to host's IPv6 localhost
- **`host.containers.internal`** resolves to IPv4 `192.168.127.254`
- **Mismatch**: Container expects IPv4, envtest provides IPv6

### Evidence
```bash
# envtest API server (from logs):
"api_server": "https://[::1]:58539/"

# DataStorage container error:
ERROR: "dial tcp [::1]:58539: connect: connection refused"
```

---

## 🛠️ What We've Tried

### Attempt 1: Rewrite API Server URL in Kubeconfig ❌
**Approach**: Replace `[::1]` with `host.containers.internal` in kubeconfig  
**Code**: `test/infrastructure/serviceaccount.go:276-280`
```go
containerAPIServer := strings.Replace(cfg.Host, "[::1]", "host.containers.internal", 1)
containerAPIServer = strings.Replace(containerAPIServer, "127.0.0.1", "host.containers.internal", 1)
```
**Result**: Failed - `host.containers.internal` resolves to IPv4, but envtest still bound to IPv6

---

### Attempt 2: Podman `--network=host` ❌
**Approach**: Use host networking so container shares host's network stack  
**Code**: `test/infrastructure/datastorage_bootstrap.go:462-476`
```go
if useHostNetwork {
    args = append(args, "--network", "host")
}
```
**Result**: Failed - DataStorage listened on port 8080 (internal), test expected 18140 (external mapped port)  
**Issue**: `--network=host` disables port mapping; would need DataStorage to read `PORT` env var from config  
**Log**: `HTTP server listening on 0.0.0.0:8080` but test expects `localhost:18140`

---

### Attempt 3: Environment Variable to Force IPv4 ❌
**Approach**: Set `TEST_ASSET_KUBE_APISERVER_BIND_ADDRESS=0.0.0.0`  
**Code**: `test/integration/remediationorchestrator/suite_test.go:150`
```go
os.Setenv("TEST_ASSET_KUBE_APISERVER_BIND_ADDRESS", "0.0.0.0")
```
**Result**: Failed - envtest ignored environment variable, still bound to IPv6

---

### Attempt 4: Configure envtest ControlPlane.APIServer.Args ❌
**Approach**: Explicitly set `--bind-address=0.0.0.0` in envtest API server args  
**Code**: `test/integration/remediationorchestrator/suite_test.go:156-164`
```go
sharedTestEnv := &envtest.Environment{
    ControlPlane: envtest.ControlPlane{
        APIServer: &envtest.APIServer{
            Args: []string{
                "--bind-address=0.0.0.0",
            },
        },
    },
}
```
**Result**: Failed - envtest control plane didn't start (timeout after 179 seconds)  
**Log**: `unable to start the controlplane: timeout waiting for process kube-apiserver to start`  
**Hypothesis**: `--bind-address` might not be the correct flag for kube-apiserver in envtest context

---

## 📊 Relevant Code

### envtest Setup (Phase 1 - Shared)
**File**: `test/integration/remediationorchestrator/suite_test.go:147-173`
```go
sharedTestEnv := &envtest.Environment{
    CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
    ErrorIfCRDPathMissing: true,
}
sharedCfg, err := sharedTestEnv.Start()
// Returns: cfg.Host = "https://[::1]:RANDOM_PORT/"
```

### Kubeconfig Generation
**File**: `test/infrastructure/serviceaccount.go:262-290`
```go
kubeconfig := clientcmdapi.Config{
    Clusters: map[string]*clientcmdapi.Cluster{
        "envtest": {
            Server:                   cfg.Host, // ← "https://[::1]:PORT"
            CertificateAuthorityData: cfg.CAData,
        },
    },
    // ... contexts, auth info with ServiceAccount token
}
kubeconfigPath := filepath.Join(homeDir, "tmp", "kubernaut-envtest", fmt.Sprintf("kubeconfig-%s.yaml", saName))
clientcmd.WriteToFile(kubeconfig, kubeconfigPath)
```

### DataStorage Container Startup
**File**: `test/infrastructure/datastorage_bootstrap.go:460-505`
```go
args := []string{"run", "-d", "--name", infra.DataStorageContainer}
args = append(args, "--network", infra.Network) // Bridge network

// Mount kubeconfig
if cfg.EnvtestKubeconfig != "" {
    args = append(args,
        "-v", fmt.Sprintf("%s:/tmp/kubeconfig:ro", cfg.EnvtestKubeconfig),
        "-e", "KUBECONFIG=/tmp/kubeconfig",
        "-e", "POD_NAMESPACE=default",
    )
}

cmd := exec.Command("podman", args...)
```

### DataStorage K8s Client Initialization
**File**: `cmd/datastorage/main.go:123-145`
```go
var k8sConfig *rest.Config
kubeconfigPath := os.Getenv("KUBECONFIG")
if kubeconfigPath != "" {
    k8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
} else {
    k8sConfig, err = rest.InClusterConfig() // Production fallback
}

k8sClient, err := kubernetes.NewForConfig(k8sConfig)
authenticator := auth.NewK8sAuthenticator(k8sClient)
authorizer := auth.NewK8sAuthorizer(k8sClient)
```

---

## 🤔 Questions for SME

### Primary Question
**How can a Podman container (bridge network) reach an envtest kube-apiserver bound to IPv6 `[::1]` on the host?**

### Specific Technical Questions

1. **envtest IPv4 Binding**:
   - Is there a way to force envtest to bind to IPv4 `127.0.0.1` instead of IPv6 `[::1]`?
   - What are the correct kube-apiserver flags for envtest?
   - Does envtest respect `--bind-address` or `--advertise-address`?

2. **Podman IPv6 Routing**:
   - Can Podman bridge network route to host's IPv6 localhost (`[::1]`)?
   - Is there a Podman configuration to enable IPv6 host access?
   - Does `--network=host` work differently on macOS vs Linux?

3. **Alternative Approaches**:
   - Should we use `--network=slirp4netns` instead of bridge?
   - Is there a better host alias than `host.containers.internal`?
   - Can we configure a custom DNS entry for envtest API server?

4. **Port Configuration**:
   - With `--network=host`, how should DataStorage know which port to listen on?
   - Should we modify DataStorage to read `PORT` from environment (currently reads from config file)?
   - Is there a way to use port mapping with host networking?

---

## 🎯 Proposed Solutions (Need Validation)

### Option A: Fix envtest IPv4 Binding
**Goal**: Make envtest bind to `127.0.0.1` instead of `[::1]`

**Steps**:
1. Find correct kube-apiserver flags for envtest
2. Configure envtest to use IPv4
3. Update kubeconfig to use `host.containers.internal:PORT`

**Pros**: Clean solution, works with bridge network  
**Cons**: envtest might not support this  
**Confidence**: Low (tried, failed so far)

---

### Option B: Enable Podman IPv6 Host Routing
**Goal**: Configure Podman to route to host's IPv6 localhost

**Steps**:
1. Enable IPv6 in Podman network configuration
2. Configure routing from container to `[::1]`
3. Keep kubeconfig with `[::1]:PORT`

**Pros**: No envtest changes needed  
**Cons**: May not work on macOS, adds complexity  
**Confidence**: Low (unknown feasibility)

---

### Option C: Use `--network=host` + Fix Port Configuration
**Goal**: Use host networking, resolve port mismatch

**Steps**:
1. Modify DataStorage to read `PORT` from environment variable
2. Use `--network=host` for DataStorage container
3. Set `PORT=18140` in environment

**Pros**: Container can reach `[::1]` directly  
**Cons**: Changes production code, diverges from E2E approach  
**Confidence**: Medium (would work, but requires code changes)

---

### Option D: Dual-Mode Port Binding in DataStorage
**Goal**: DataStorage binds to external port when `PORT` env var is set

**Steps**:
1. Add logic in `cmd/datastorage/main.go` to check `PORT` env var
2. Override config port if `PORT` is set
3. Use `--network=host` with `PORT=18140`

**Pros**: No config file changes, works with host networking  
**Cons**: Adds conditional logic to production binary  
**Confidence**: High (would definitely work)

**Code Sketch**:
```go
// cmd/datastorage/main.go (after loading config)
if portEnv := os.Getenv("PORT"); portEnv != "" {
    if port, err := strconv.Atoi(portEnv); err == nil {
        cfg.Server.Port = port
        logger.Info("Port overridden by PORT environment variable", "port", port)
    }
}
```

---

## 📈 Success Metrics

If solution works, we should see:
```bash
# DataStorage logs:
INFO: Token validated successfully
INFO: Authorization successful

# Test output:
✅ Authenticated DataStorage clients created
✅ 59/59 tests passing (not 46/59 with auth failures)
```

---

## 🔄 Next Steps

### If SME Recommends Option A (envtest IPv4):
1. Implement correct envtest API server configuration
2. Validate envtest starts with IPv4 binding
3. Test container connectivity
4. Document for other services

### If SME Recommends Option B (Podman IPv6):
1. Configure Podman network for IPv6 host routing
2. Validate container can reach `[::1]`
3. Test with existing kubeconfig
4. Document for other services

### If SME Recommends Option C/D (Port Configuration):
1. Modify DataStorage `main.go` to support `PORT` env var
2. Update `datastorage_bootstrap.go` to use host networking
3. Test with `PORT=18140`
4. Document for other services

---

## 📚 References

### Related Files
- `test/integration/remediationorchestrator/suite_test.go` - envtest setup
- `test/infrastructure/serviceaccount.go` - Kubeconfig generation
- `test/infrastructure/datastorage_bootstrap.go` - Container startup
- `cmd/datastorage/main.go` - K8s client initialization
- `pkg/shared/auth/k8s_auth.go` - TokenReview/SAR implementation

### External Documentation
- [envtest Package Docs](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest)
- [Podman Networking](https://docs.podman.io/en/latest/markdown/podman-network.1.html)
- [kube-apiserver Flags](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-apiserver/)

### Test Logs
- Latest: `/tmp/kubernaut-must-gather/remediationorchestrator-integration-20260127-102911/`
- DataStorage: `remediationorchestrator_remediationorchestrator_datastorage_test.log`

---

## 💬 Contact

**Question Author**: AI Assistant (via @jgil)  
**Context**: DD-AUTH-014 - Kubernetes Subject Access Review integration  
**Urgency**: High - Blocks migration of 6 remaining services to real auth

---

**Thank you for your assistance!** Once we solve this, the solution will be reusable across:
- RemediationOrchestrator ✅ (current POC)
- SignalProcessing
- WorkflowExecution
- Notification
- Gateway
- AIAnalysis
- AuthWebhook


Answer:
You are running into a fundamental limitation on macOS: your container cannot reach the host’s IPv6 loopback 
[
:
:
1
]
[::1] at all, so the only practical path that satisfies your “real K8s auth + envtest + containerized DataStorage” requirements is to make the envtest API server reachable via IPv4 and point the container at that.

Below is a concrete path I’d recommend, plus notes on why the other options are either brittle or macOS‑infeasible.

Recommended direction
Use envtest with an IPv4‑reachable API server and keep Podman on a normal bridge network:

Make envtest bind on an IPv4 address (or at least advertise an IPv4 host/port) instead of 
[
:
:
1
]
[::1].

Rewrite the kubeconfig clusters[].server entry to use https://host.containers.internal:PORT (IPv4) and ensure the port is reachable from the container.

Keep DataStorage unchanged: it still reads KUBECONFIG and talks to the API server using the rewritten URL and CA.

The key change is: instead of trying to reach [::1] from inside the container, make the kubeconfig talk to an IPv4 endpoint that is actually routable from the Podman bridge network.
​

Why Podman cannot reach [::1] on macOS
On macOS, Podman is not running “on the host” in the Linux sense; it runs inside a Linux VM, and the “bridge” network is between containers and that VM, not your macOS kernel.

That implies:

The container’s ::1 is the container’s own loopback.

The VM’s ::1 is the VM’s loopback.

Your macOS ::1 is another loopback again.

There is no standard way to route from the container to the macOS host’s IPv6 loopback; host.containers.internal is wired to an IPv4 address that forwards into the host/VM boundary, not to ::1.
​

So “make Podman bridge reach host [::1]” is effectively a non‑starter on macOS without deep custom VM/network hacking.

Option A (envtest IPv4 binding) – how to make it workable
Your first attempts failed because envtest did not pick up the env var, and using --bind-address=0.0.0.0 as a raw arg led kube‑apiserver to fail to start. The important nuance is that envtest manages the control plane and will already set a bunch of flags; you usually need to append to them in a way it accepts.
​

A practical variant that avoids fighting directly with IPv6 binding is:

Keep whatever envtest binds to, but override the advertised URL and kubeconfig:

When sharedTestEnv.Start() returns cfg, capture:

The actual port from cfg.Host (regex or url.Parse).

The CA data.

Create your own kubeconfig pointing at IPv4:

go
u, err := url.Parse(cfg.Host)
if err != nil { /* handle */ }

port := u.Port()
if port == "" {
    port = "443" // fallback
}

ipv4Server := fmt.Sprintf("https://host.containers.internal:%s", port)

kubeconfig := clientcmdapi.Config{
    Clusters: map[string]*clientcmdapi.Cluster{
        "envtest": {
            Server:                   ipv4Server,
            CertificateAuthorityData: cfg.CAData,
        },
    },
    AuthInfos:  /* as you already do with SA token */,
    Contexts:   /* as you already do */,
    CurrentContext: "envtest",
}
This way, the container always talks to host.containers.internal:PORT over IPv4, regardless of what cfg.Host looks like.
​

Ensure the port is reachable on IPv4:

This is the only subtle bit: if kube‑apiserver is genuinely bound only to ::1, it may not accept IPv4 on that port. On a dual‑stack kernel, ::1 binding sometimes implies a v4‑mapped listener, but that is OS/flags dependent. On macOS with the envtest binary inside the VM, it may or may not happen automatically.

If curl https://127.0.0.1:PORT from inside the envtest process’s network namespace fails, you will need to persuade envtest/kube‑apiserver to either:

Bind to 0.0.0.0 or 127.0.0.1 in addition to whatever IPv6 it uses, or

Bind to an IPv4 address only.

In envtest, this typically means adding to ControlPlane.APIServer.Args only those flags kube‑apiserver actually accepts. If --bind-address=0.0.0.0 caused a startup timeout, inspect the generated kube-apiserver logs in your must‑gather directory to see the exact argument error.

Valid modern flags include:

--advertise-address=<IPv4> – influences what appears in discovery and may impact how cfg.Host is built.
​

--bind-address=<IP> – listening address for the secure port.

--secure-port=<PORT> – to pin down a port and avoid “random”.

The safest pattern is to:

Fix secure-port to a known value.

Set bind-address=0.0.0.0 (so IPv4 listens).

Still rewrite cfg.Host in your kubeconfig to host.containers.internal:<PORT>.

Then test from the host/VM:

bash
curl -k https://127.0.0.1:PORT/healthz
Once that works, test from inside the container:

bash
podman exec datastorage curl -k https://host.containers.internal:PORT/healthz
Option B (Podman IPv6 routing) – why it’s not attractive
On Linux, you could create an IPv6‑enabled Podman network and possibly route to a host IPv6 address.

But on macOS:

Podman uses a Linux VM; IPv6 support is focused on the VM and its bridge networks, not your macOS host’s ::1.

Even if you enabled IPv6 inside a custom Podman network (with podman network create --ipv6 and a v6 subnet), you would still be in the VM’s IPv6 domain, not the macOS loopback’s.
​

You would end up building a fairly custom relay (e.g., a sidecar proxy on IPv4 that forwards to [::1]), which increases complexity and reduces the “reusable” aspect you want.

Option C/D (host networking + PORT env var) – when to choose them
On macOS, “host networking” is also mediated by the VM, and port mapping behaves differently than on bare Linux. However, from a correctness point of view, Options C/D are reliable because they avoid the IPv4/IPv6 mismatch entirely:

DataStorage and envtest share the same network namespace.

Both can use https://127.0.0.1:PORT or https://[::1]:PORT.

No need for host.containers.internal at all.

If you are willing to add the PORT env var override logic to cmd/datastorage/main.go, Option D is very clean:

Add the PORT override snippet you sketched.

In your bootstrap:

Run podman run --network host -e PORT=18140 ....

Expose no mapped ports; the tests should hit http://localhost:18140 on the host, which is forwarded into the VM’s “host” namespace where the container listens.

For envtest:

Leave it binding on [::1]:RANDOM_PORT or similar; from the container’s point of view on host networking, ::1 and 127.0.0.1 are reachable in that namespace.

This does touch production code, but the change (honor PORT if set) is common practice in many Go services and is unlikely to be controversial. It also gives you a single pattern to reuse across all 7 services.

Concrete recommendation
Given your constraints and the macOS/VM reality, I would:

Try the IPv4 kubeconfig rewrite path first:

Derive port from cfg.Host.

Force kubeconfig.clusters["envtest"].Server to https://host.containers.internal:<port>.

Verify from inside the container.

If you see “connection refused”, confirm whether the API server is listening on IPv4 at all.

If envtest simply refuses to listen on IPv4 in your setup, adopt Option D:

Implement the PORT env var override.

Use --network host and drive test ports purely via env vars and host networking.

Both patterns are reusable for your other services: either “rewrite kubeconfig to IPv4 + Podman bridge” or “host network + PORT override.” The only thing I would not invest in is trying to make the container talk directly to the host’s [::1] on macOS; that is fighting the platform rather than using it