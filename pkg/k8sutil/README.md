# Kubernetes Client Utilities

**Package**: `github.com/jordigilh/kubernaut/pkg/k8sutil`
**Design Decision**: [DD-013: Kubernetes Client Initialization Standard](../../docs/architecture/decisions/DD-013-kubernetes-client-initialization-standard.md)

---

## Purpose

This package provides standard utilities for Kubernetes client initialization across Kubernaut services.

**Key Features**:
- ✅ Automatic configuration resolution (KUBECONFIG → in-cluster)
- ✅ Proper error handling (returns errors, doesn't panic)
- ✅ Well-documented with usage examples
- ✅ Consistent pattern across all HTTP services

---

## When to Use

### ✅ USE `pkg/k8sutil` for:
- **HTTP Services** that need Kubernetes API access (e.g., Dynamic Toolset)
- **Background Workers** that query Kubernetes resources
- **CLI Tools** that interact with Kubernetes
- **Services** that need direct clientset access

### ❌ DO NOT USE for:
- **CRD Controllers** - Use `ctrl.GetConfigOrDie()` from controller-runtime instead
- **Services using controller-runtime manager** - Use `mgr.GetClient()` instead
- **Test Infrastructure** - Inline pattern is acceptable for tests

---

## API

### `GetConfig() (*rest.Config, error)`

Returns a Kubernetes REST config, trying KUBECONFIG first, then in-cluster.

**Configuration Resolution Order**:
1. `KUBECONFIG` environment variable (development mode)
2. In-cluster config from service account (production mode)

**Example**:
```go
import "github.com/jordigilh/kubernaut/pkg/k8sutil"

config, err := k8sutil.GetConfig()
if err != nil {
    return fmt.Errorf("failed to get Kubernetes config: %w", err)
}
clientset, err := kubernetes.NewForConfig(config)
```

---

### `NewClientset() (kubernetes.Interface, error)`

Creates a Kubernetes clientset using the standard config resolution.

This is a convenience wrapper around `GetConfig()` + `kubernetes.NewForConfig()`.

**Example**:
```go
import "github.com/jordigilh/kubernaut/pkg/k8sutil"

clientset, err := k8sutil.NewClientset()
if err != nil {
    return fmt.Errorf("failed to create Kubernetes client: %w", err)
}

// Use clientset to interact with Kubernetes API
serverVersion, err := clientset.Discovery().ServerVersion()
if err != nil {
    return fmt.Errorf("failed to get server version: %w", err)
}
```

---

## Usage Examples

### HTTP Service (Dynamic Toolset)

```go
package main

import (
    "github.com/jordigilh/kubernaut/pkg/k8sutil"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewProduction()

    // Create Kubernetes client using standard helper (DD-013)
    clientset, err := k8sutil.NewClientset()
    if err != nil {
        logger.Fatal("Failed to create Kubernetes client", zap.Error(err))
    }

    // Verify client is working
    serverVersion, err := clientset.Discovery().ServerVersion()
    if err != nil {
        logger.Fatal("Failed to connect to Kubernetes API server", zap.Error(err))
    }

    logger.Info("Kubernetes client initialized",
        zap.String("server_version", serverVersion.String()))

    // ... rest of service ...
}
```

### CRD Controller (Notification Controller) - DO NOT USE k8sutil

```go
package main

import (
    ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
    // CRD controllers should use controller-runtime's GetConfigOrDie()
    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme: scheme,
        // ... other options ...
    })
    if err != nil {
        setupLog.Error(err, "unable to start manager")
        os.Exit(1)
    }

    // Use manager's client
    client := mgr.GetClient()

    // ... rest of controller ...
}
```

---

## Configuration

### Development Mode

Set `KUBECONFIG` environment variable to point to your kubeconfig file:

```bash
export KUBECONFIG=$HOME/.kube/config
./dynamictoolset
```

### Production Mode (In-Cluster)

No configuration needed. The service will automatically use the service account mounted at `/var/run/secrets/kubernetes.io/serviceaccount/`.

```bash
# In Kubernetes pod
./dynamictoolset
```

---

## Pattern Matrix

| Service Type | Kubernetes Access | Standard Pattern | Example |
|--------------|-------------------|------------------|---------|
| **HTTP Service** | Direct clientset | `k8sutil.NewClientset()` | Dynamic Toolset |
| **CRD Controller** | Manager client | `ctrl.GetConfigOrDie()` + `mgr.GetClient()` | Notification Controller |
| **Test Infrastructure** | Test-specific | Inline (acceptable) | `test/infrastructure/*.go` |

---

## Testing

Run tests with:

```bash
# Unit tests (requires KUBECONFIG or in-cluster config)
go test ./pkg/k8sutil/... -v

# Skip tests if no Kubernetes access
go test ./pkg/k8sutil/... -v -short
```

---

## References

- **Design Decision**: [DD-013](../../docs/architecture/decisions/DD-013-kubernetes-client-initialization-standard.md)
- **Triage Analysis**: [K8S_CLIENT_HELPER_TRIAGE.md](../../K8S_CLIENT_HELPER_TRIAGE.md)
- **client-go docs**: https://pkg.go.dev/k8s.io/client-go/kubernetes
- **controller-runtime docs**: https://pkg.go.dev/sigs.k8s.io/controller-runtime

---

**Last Updated**: 2025-11-08
**Maintained By**: Kubernaut Architecture Team

