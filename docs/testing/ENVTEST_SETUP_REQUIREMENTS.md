# envtest Setup Requirements

**Last Updated**: 2025-10-12  
**Status**: ✅ Complete

---

## ⚠️ Critical: envtest Requires setup-envtest

**YES, envtest absolutely requires `setup-envtest`!**

Unlike Fake Client (which needs zero setup), **envtest requires downloading real Kubernetes API server binaries**.

---

## What is setup-envtest?

`setup-envtest` is a tool from `sigs.k8s.io/controller-runtime` that:

1. **Downloads kube-apiserver binary** (~50MB)
2. **Downloads etcd binary** (~20MB)  
3. **Manages multiple K8s versions** (1.29, 1.30, 1.31, etc.)
4. **Provides binary paths** via `KUBEBUILDER_ASSETS` environment variable

**Total Download**: ~70MB per Kubernetes version

---

## Why envtest Needs Binaries

envtest creates a **real Kubernetes API server** in your test process:

```go
// When you call testEnv.Start()...
testEnv := &envtest.Environment{}
cfg, err := testEnv.Start() // ← This starts kube-apiserver + etcd!

// It starts TWO processes:
// 1. etcd (Kubernetes database)
// 2. kube-apiserver (API server that validates requests)
```

**Without these binaries**, envtest **cannot start** and tests will fail with:

```
Error: unable to start control plane: failed to start etcd
```

---

## Setup Instructions

### For Developers

```bash
# Install setup-envtest tool
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# Download Kubernetes 1.31 binaries
setup-envtest use 1.31.0 -p path

# Or use Makefile
make setup-envtest
```

### For CI/CD

```yaml
# .github/workflows/test.yml
- name: Setup envtest
  run: |
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
    setup-envtest use 1.31.0 --bin-dir ./testbin -p path

- name: Run tests
  run: make test
  env:
    KUBEBUILDER_ASSETS: ${{ github.workspace }}/testbin/k8s/1.31.0-linux-amd64
```

### Cache Binaries (Recommended for CI)

```yaml
# Cache the downloaded binaries
- name: Cache envtest binaries
  uses: actions/cache@v3
  with:
    path: ~/testbin
    key: envtest-${{ runner.os }}-1.31.0
```

---

## Comparison: Fake Client vs envtest

| Aspect | Fake Client | envtest |
|--------|-------------|---------|
| **Prerequisites** | ❌ None | ⚠️ **setup-envtest required** |
| **Binary Download** | ❌ None | ✅ ~70MB (kube-apiserver + etcd) |
| **Setup Time** | ✅ Instant | ⚠️ ~10-30 seconds (first time) |
| **Test Startup** | ✅ < 1 second | ⚠️ ~2 seconds (API server start) |
| **CI/CD Impact** | ✅ No extra steps | ⚠️ Download + cache step needed |
| **Disk Space** | ✅ Minimal | ⚠️ ~70MB per K8s version |
| **API Server** | ❌ No | ✅ Yes (real kube-apiserver) |
| **Field Selectors** | ❌ No | ✅ Yes |
| **Schema Validation** | ❌ No | ✅ Yes |

---

## When to Use Each

### Use **Fake Client** (Zero Setup) ✅

```go
import "k8s.io/client-go/kubernetes/fake"

// No prerequisites needed!
fakeClient := fake.NewSimpleClientset(objects...)
```

**Best For**:
- Simple K8s READ operations (list, get)
- No field selectors needed
- Speed is critical
- Zero setup desired

---

### Use **envtest** (Requires setup-envtest) ⚠️

```go
import "sigs.k8s.io/controller-runtime/pkg/envtest"

// Requires setup-envtest binaries!
testEnv := &envtest.Environment{}
cfg, _ := testEnv.Start() // Starts kube-apiserver + etcd
```

**Best For**:
- **Testing with CRDs** (register definitions + use controller-runtime client)
- Need field selectors (`.spec.nodeName=worker`)
- Need API server validation
- Need realistic watch behavior
- Controller development with custom resources

**Accept**:
- ~70MB binary download
- ~10-30 second CI/CD setup step
- ~2 second test startup time

---

## CRD Support in envtest

**Important**: envtest **fully supports Custom Resource Definitions (CRDs)**!

You just need to:
1. **Register CRD YAML files** via `CRDDirectoryPaths`
2. **Register scheme** with your CRD types
3. **Use controller-runtime client** (not just standard K8s client)

**Example**:
```go
import (
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
)

// Register CRD scheme
err := remediationv1.AddToScheme(scheme.Scheme)

// Point to CRD YAML files
testEnv := &envtest.Environment{
    CRDDirectoryPaths: []string{"./config/crd/bases"},
}

// Use controller-runtime client (supports CRDs!)
k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})

// Create CRD instances
remediation := &remediationv1.RemediationRequest{...}
err = k8sClient.Create(ctx, remediation)
```

**This is why Kubernaut CRD controllers use envtest** - they need full CRD support with schema validation.

---

## Project Usage

Kubernaut currently uses `setup-envtest` for:

### CRD Controllers (envtest required)
- ✅ RemediationRequest Controller
- ✅ AIAnalysis Controller  
- ✅ WorkflowExecution Controller
- ✅ KubernetesExecution Controller (DEPRECATED - ADR-025)

**See**: `Makefile` targets:
```makefile
test: setup-envtest
test-integration: setup-envtest
```

### Stateless Services (Fake Client recommended)
- ✅ Gateway Service (reads K8s, no field selectors needed)
- ✅ HolmesGPT API (reads K8s logs/events, simple queries)
- ✅ Data Storage (no K8s operations)
- ✅ Context API (no K8s operations)

---

## Summary

**Question**: Does envtest need setup-envtest?

**Answer**: **YES, absolutely!**

- **Fake Client**: ✅ Zero setup, instant tests
- **envtest**: ⚠️ Requires `setup-envtest` to download ~70MB of Kubernetes binaries
- **KIND**: ⚠️ Requires Docker/Podman + cluster creation (30-60 seconds)

**Recommendation**: Start with **Fake Client** for stateless services. Only use **envtest** if you specifically need field selectors, CRDs, or API server validation.

---

## References

- [Fake Client vs envtest Decision Guide](./INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md#-decision-guide-which-one-should-i-use)
- [controller-runtime envtest docs](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest)
- [setup-envtest tool](https://github.com/kubernetes-sigs/controller-runtime/tree/main/tools/setup-envtest)
- [Kubernaut Makefile](../../Makefile) - `make setup-envtest` target


