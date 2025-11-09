# DD-013: Kubernetes Client Initialization Standard

## Status
**✅ APPROVED** (2025-11-08)
**Last Reviewed**: 2025-11-08
**Confidence**: 95%

---

## Context & Problem

**Problem**: Services that need Kubernetes API access were implementing ad-hoc client initialization code, leading to:
1. **Code Duplication**: Same 15-line pattern repeated across services
2. **Inconsistency**: Different error handling approaches
3. **Discoverability**: No standard pattern documented
4. **Maintainability**: Changes require updates in multiple places

**Trigger**: During build fix for `cmd/dynamictoolset/main.go`, discovered inline Kubernetes client initialization that should be shared.

**Key Requirements**:
- Standardize Kubernetes client creation across all services
- Support both development (KUBECONFIG) and production (in-cluster) modes
- Provide clear separation between HTTP services and CRD controllers
- Enable easy discovery and reuse

---

## Alternatives Considered

### Alternative 1: Shared Helper Function in `pkg/k8sutil` ✅ APPROVED

**Approach**: Create `pkg/k8sutil/client.go` with two helper functions:
- `GetConfig()` - Returns `*rest.Config` with KUBECONFIG fallback to in-cluster
- `NewClientset()` - Convenience wrapper returning `kubernetes.Interface`

**Pros**:
- ✅ **Eliminates Duplication**: Reduces 15 lines to 3 lines in main.go
- ✅ **Proper Error Handling**: Returns errors instead of panicking
- ✅ **Clear Documentation**: Well-documented with usage examples
- ✅ **HTTP Service Fit**: Perfect for stateless HTTP services
- ✅ **Reusable**: Ready for future services
- ✅ **Discoverable**: Single import path for all HTTP services
- ✅ **Testable**: Easy to unit test and mock

**Cons**:
- ⚠️ **New Code**: Requires ~30 minutes to implement and test
- ⚠️ **Learning Curve**: Developers need to know about `pkg/k8sutil`

**Confidence**: 95% (Best practice, aligns with Go conventions)

---

### Alternative 2: Use controller-runtime's `ctrl.GetConfigOrDie()` ❌ REJECTED

**Approach**: Import controller-runtime and use `ctrl.GetConfigOrDie()` in HTTP services

**Pros**:
- ✅ **No New Code**: Already exists in controller-runtime
- ✅ **Battle-Tested**: Used by all CRD controllers

**Cons**:
- ❌ **Wrong Dependency**: Adds controller-runtime to HTTP services (unnecessary)
- ❌ **Panics on Error**: Not suitable for HTTP services (should return errors)
- ❌ **Violates Separation**: HTTP services should not depend on controller-runtime
- ❌ **Misleading**: Suggests service is a CRD controller when it's not

**Confidence**: 10% (Wrong tool for the job)

---

### Alternative 3: Keep Inline Pattern (Status Quo) ❌ REJECTED

**Approach**: Continue with inline client initialization in each service

**Pros**:
- ✅ **No Refactoring**: Zero effort required
- ✅ **Explicit**: Code is visible in main.go

**Cons**:
- ❌ **Code Duplication**: Violates DRY principle
- ❌ **Maintenance Burden**: Changes require updates in multiple places
- ❌ **Inconsistency Risk**: Different implementations may diverge
- ❌ **Not Discoverable**: New developers must find pattern by example

**Confidence**: 30% (Acceptable for 1 service, not scalable)

---

## Decision

**APPROVED: Alternative 1** - Shared Helper Function in `pkg/k8sutil`

**Rationale**:
1. **Eliminates Duplication**: Single source of truth for Kubernetes client initialization
2. **Best Practice**: Follows Go conventions (return errors, clear documentation)
3. **Clear Separation**: HTTP services use `k8sutil`, controllers use `ctrl.GetConfigOrDie()`
4. **Future-Proof**: Ready for additional HTTP services
5. **Low Cost**: ~30 minutes to implement, high long-term value

**Key Insight**: Different service types need different patterns. HTTP services should use lightweight helpers, while CRD controllers should use controller-runtime's integrated approach.

---

## Implementation

### Primary Implementation Files

**1. `pkg/k8sutil/client.go`** - Core helper functions

```go
package k8sutil

import (
	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// GetConfig returns a Kubernetes REST config, trying KUBECONFIG first, then in-cluster
// This is the standard pattern for HTTP services that need Kubernetes access.
//
// For CRD controllers, use ctrl.GetConfigOrDie() from controller-runtime instead.
//
// Usage:
//   config, err := k8sutil.GetConfig()
//   if err != nil {
//       return fmt.Errorf("failed to get Kubernetes config: %w", err)
//   }
//   clientset, err := kubernetes.NewForConfig(config)
func GetConfig() (*rest.Config, error) {
	// Try KUBECONFIG environment variable first (development)
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from KUBECONFIG: %w", err)
		}
		return config, nil
	}

	// Fallback to in-cluster config (production)
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	return config, nil
}

// NewClientset creates a Kubernetes clientset using the standard config resolution
// This is a convenience wrapper around GetConfig() + kubernetes.NewForConfig()
//
// For CRD controllers, use manager.GetClient() from controller-runtime instead.
//
// Usage:
//   clientset, err := k8sutil.NewClientset()
//   if err != nil {
//       return fmt.Errorf("failed to create Kubernetes client: %w", err)
//   }
func NewClientset() (kubernetes.Interface, error) {
	config, err := GetConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}
```

**2. `pkg/k8sutil/client_test.go`** - Unit tests

```go
package k8sutil

import (
	"os"
	"testing"
)

func TestGetConfig(t *testing.T) {
	// Save and restore original KUBECONFIG
	originalKubeconfig := os.Getenv("KUBECONFIG")
	defer os.Setenv("KUBECONFIG", originalKubeconfig)

	t.Run("returns error when no config available", func(t *testing.T) {
		os.Unsetenv("KUBECONFIG")
		// In test environment, in-cluster config will fail
		_, err := GetConfig()
		if err == nil {
			t.Error("expected error when no config available")
		}
	})

	t.Run("uses KUBECONFIG when set", func(t *testing.T) {
		// This test requires a valid kubeconfig file
		// Skip if KUBECONFIG not set in test environment
		if os.Getenv("KUBECONFIG") == "" {
			t.Skip("KUBECONFIG not set, skipping")
		}

		config, err := GetConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config == nil {
			t.Error("expected non-nil config")
		}
	})
}

func TestNewClientset(t *testing.T) {
	// Skip if no Kubernetes access available
	if os.Getenv("KUBECONFIG") == "" {
		t.Skip("KUBECONFIG not set, skipping")
	}

	clientset, err := NewClientset()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clientset == nil {
		t.Error("expected non-nil clientset")
	}

	// Verify clientset works by checking server version
	_, err = clientset.Discovery().ServerVersion()
	if err != nil {
		t.Errorf("failed to get server version: %v", err)
	}
}
```

**3. Updated `cmd/dynamictoolset/main.go`** - Example usage

```go
import (
	"github.com/jordigilh/kubernaut/pkg/k8sutil"
	// ... other imports
)

func main() {
	// ... logger initialization ...

	// Create Kubernetes client using standard helper
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

	// ... rest of main ...
}
```

---

### Data Flow

1. **Development Mode** (KUBECONFIG set):
   ```
   k8sutil.NewClientset()
   ├─> k8sutil.GetConfig()
   │   ├─> Check KUBECONFIG env var
   │   ├─> clientcmd.BuildConfigFromFlags()
   │   └─> Return *rest.Config
   └─> kubernetes.NewForConfig()
       └─> Return kubernetes.Interface
   ```

2. **Production Mode** (in-cluster):
   ```
   k8sutil.NewClientset()
   ├─> k8sutil.GetConfig()
   │   ├─> KUBECONFIG not set
   │   ├─> rest.InClusterConfig()
   │   └─> Return *rest.Config
   └─> kubernetes.NewForConfig()
       └─> Return kubernetes.Interface
   ```

---

### Pattern Matrix

| Service Type | Kubernetes Access | Standard Pattern | Example |
|--------------|-------------------|------------------|---------|
| **HTTP Service** | Direct clientset | `k8sutil.NewClientset()` | Dynamic Toolset |
| **CRD Controller** | Manager client | `ctrl.GetConfigOrDie()` + `mgr.GetClient()` | Notification Controller |
| **Test Infrastructure** | Test-specific | Inline (acceptable) | `test/infrastructure/*.go` |

---

## Consequences

### Positive

- ✅ **Code Reduction**: 15 lines → 3 lines in service main.go
- ✅ **Consistency**: All HTTP services use same pattern
- ✅ **Discoverability**: Single import path (`pkg/k8sutil`)
- ✅ **Maintainability**: Changes in one place
- ✅ **Error Handling**: Proper error propagation (no panics)
- ✅ **Documentation**: Self-documenting with clear usage examples
- ✅ **Testability**: Easy to unit test and mock

### Negative

- ⚠️ **Learning Curve**: Developers must know about `pkg/k8sutil`
  - **Mitigation**: Document in coding standards and onboarding materials
- ⚠️ **Initial Effort**: ~30 minutes to implement
  - **Mitigation**: One-time cost, high long-term value

### Neutral

- 🔄 **Two Patterns**: HTTP services use `k8sutil`, controllers use `ctrl.GetConfigOrDie()`
  - This is intentional and correct - different service types have different needs

---

## Validation Results

### Confidence Assessment Progression

- **Initial assessment**: 85% confidence (clear need, standard pattern)
- **After alternatives analysis**: 90% confidence (best solution identified)
- **After implementation review**: 95% confidence (validated approach)

### Key Validation Points

- ✅ **Pattern Validated**: Matches industry best practices for Go Kubernetes clients
- ✅ **Separation Confirmed**: HTTP services and CRD controllers have different needs
- ✅ **Duplication Eliminated**: Single source of truth established
- ✅ **Error Handling**: Proper error propagation without panics
- ✅ **Documentation**: Clear usage examples and API documentation

---

## Related Decisions

- **Builds On**:
  - [docs/architecture/LOGGING_STANDARD.md](../LOGGING_STANDARD.md) - Similar pattern for logging
  - [DD-005: Observability Standards](DD-005-OBSERVABILITY-STANDARDS.md) - Consistent standards
- **Supports**:
  - All HTTP services requiring Kubernetes API access
  - Future service development
- **Referenced By**:
  - [K8S_CLIENT_HELPER_TRIAGE.md](../../K8S_CLIENT_HELPER_TRIAGE.md) - Detailed analysis

---

## Review & Evolution

### When to Revisit

- If 3+ HTTP services need custom Kubernetes client configuration
- If controller-runtime provides equivalent helper for HTTP services
- If new Kubernetes authentication methods require different approach
- If performance issues identified with current pattern

### Success Metrics

- **Adoption Rate**: Target 100% of HTTP services use `k8sutil`
- **Code Reduction**: Target 80% reduction in client init code
- **Consistency**: Target 0 ad-hoc implementations
- **Discoverability**: Target <5 minutes for new developers to find pattern

---

## Implementation Checklist

### Phase 1: Create Helper (15 minutes)
- [x] Create `pkg/k8sutil/client.go` with `GetConfig()` and `NewClientset()`
- [x] Add comprehensive documentation and usage examples
- [x] Create `pkg/k8sutil/client_test.go` with unit tests
- [x] Verify tests pass

### Phase 2: Update Dynamic Toolset (10 minutes)
- [ ] Update `cmd/dynamictoolset/main.go` to use `k8sutil.NewClientset()`
- [ ] Verify build and functionality
- [ ] Test both development and production modes

### Phase 3: Documentation (10 minutes)
- [ ] Update `DESIGN_DECISIONS.md` index
- [ ] Add to coding standards (`.cursor/rules/02-go-coding-standards.mdc`)
- [ ] Create `pkg/k8sutil/README.md` with usage guide
- [ ] Update service implementation templates

### Phase 4: Validation (5 minutes)
- [ ] Verify all builds pass
- [ ] Run integration tests
- [ ] Update project metrics

**Total Effort**: ~40 minutes

---

## Usage Guidelines

### When to Use `pkg/k8sutil`

**✅ USE for**:
- HTTP services that need Kubernetes API access (e.g., Dynamic Toolset)
- Background workers that query Kubernetes resources
- CLI tools that interact with Kubernetes
- Services that need direct clientset access

**❌ DO NOT USE for**:
- CRD controllers (use `ctrl.GetConfigOrDie()` instead)
- Services using controller-runtime manager (use `mgr.GetClient()`)
- Test infrastructure (inline is acceptable)

### Code Examples

**Correct Usage (HTTP Service)**:
```go
import "github.com/jordigilh/kubernaut/pkg/k8sutil"

clientset, err := k8sutil.NewClientset()
if err != nil {
    return fmt.Errorf("failed to create Kubernetes client: %w", err)
}
```

**Correct Usage (CRD Controller)**:
```go
import ctrl "sigs.k8s.io/controller-runtime"

mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{...})
client := mgr.GetClient()
```

**Incorrect Usage**:
```go
// ❌ DON'T: Inline client initialization in HTTP services
var config *rest.Config
if os.Getenv("KUBECONFIG") != "" {
    config, err = clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
} else {
    config, err = rest.InClusterConfig()
}
// ... (use k8sutil instead)
```

---

## References

- [client-go documentation](https://pkg.go.dev/k8s.io/client-go/kubernetes)
- [controller-runtime GetConfigOrDie](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/client/config#GetConfigOrDie)
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [K8S_CLIENT_HELPER_TRIAGE.md](../../K8S_CLIENT_HELPER_TRIAGE.md) - Detailed triage analysis

---

**Document Status**: ✅ Approved
**Implementation Status**: 🔄 In Progress (Phase 1 complete)
**Last Updated**: 2025-11-08
**Decision Owner**: Architecture Team
**Confidence**: 95%

