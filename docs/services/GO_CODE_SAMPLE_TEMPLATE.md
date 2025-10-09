# Go Code Sample Template

**Version**: v1.0
**Last Updated**: October 9, 2025
**Purpose**: Standardized import patterns for documentation code samples

---

## Overview

This document defines standardized import patterns for Go code samples in Kubernaut service documentation. Following these conventions ensures:

- ✅ Code samples are copy-paste ready
- ✅ Import aliases are consistent across documentation
- ✅ Developers can easily identify package origins
- ✅ Documentation examples compile without modification

---

## Standard Import Aliases

### Kubernetes Core Types

```go
import (
    corev1 "k8s.io/api/core/v1"
    appsv1 "k8s.io/api/apps/v1"
    batchv1 "k8s.io/api/batch/v1"
)
```

### Kubernetes API Machinery

```go
import (
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    apimeta "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
)
```

### Controller Runtime

```go
import (
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/log"
    "sigs.k8s.io/controller-runtime/pkg/predicate"
)
```

### Client-Go

```go
import (
    "k8s.io/client-go/tools/record"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
)
```

### Kubernaut Project APIs

```go
import (
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"
    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1"
)
```

---

## Complete Import Templates

### Template 1: Kubebuilder Controller Implementation

Use this template for controller reconciliation logic documentation.

```go
package controller

import (
    "context"
    "fmt"
    "time"

    corev1 "k8s.io/api/core/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    apimeta "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/log"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
)

const (
    finalizerName = "kubernaut.io/finalizer"
)

type ExampleReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder
}

func (r *ExampleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    // Implementation...
    return ctrl.Result{}, nil
}

func (r *ExampleReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1.RemediationRequest{}).
        Complete(r)
}
```

**Key Patterns**:
- Package: `package controller`
- Standard library first: `context`, `fmt`, `time`
- K8s imports second (with aliases)
- Controller-runtime imports third
- Project-specific imports last
- Blank line separating groups

---

### Template 2: CRD API Type Definition

Use this template for CRD schema type definitions.

```go
package v1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExampleSpec defines the desired state
type ExampleSpec struct {
    // RemediationRequestRef references the parent CRD
    RemediationRequestRef corev1.LocalObjectReference `json:"remediationRequestRef"`

    // ConfigData contains configuration
    ConfigData map[string]string `json:"configData,omitempty"`
}

// ExampleStatus defines the observed state
type ExampleStatus struct {
    // Phase tracks current processing stage
    Phase string `json:"phase"`

    // Conditions for status tracking
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Example is the Schema for the examples API
type Example struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   ExampleSpec   `json:"spec,omitempty"`
    Status ExampleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ExampleList contains a list of Example
type ExampleList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []Example `json:"items"`
}

func init() {
    SchemeBuilder.Register(&Example{}, &ExampleList{})
}
```

**Key Patterns**:
- Package: `package v1` (or `v1alpha1`, `v1beta1`)
- Only `corev1` and `metav1` typically needed
- Kubebuilder markers use `+kubebuilder:` prefix
- Keep imports minimal for CRD types

---

### Template 3: Business Logic Package

Use this template for business logic implementations in `pkg/`.

```go
package processing

import (
    "context"
    "fmt"
    "time"

    "github.com/go-logr/logr"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/controller-runtime/pkg/client"

    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Processor handles business logic
type Processor interface {
    Process(ctx context.Context, input *Input) (*Output, error)
}

type ProcessorImpl struct {
    client client.Client
    logger logr.Logger
}

func NewProcessor(client client.Client, logger logr.Logger) *ProcessorImpl {
    return &ProcessorImpl{
        client: client,
        logger: logger,
    }
}

func (p *ProcessorImpl) Process(ctx context.Context, input *Input) (*Output, error) {
    // Implementation...
    return &Output{}, nil
}
```

**Key Patterns**:
- Package: Descriptive business domain name
- Third-party imports after standard library
- K8s imports before project imports
- Interface definitions before implementations

---

### Template 4: HTTP Service/Handler

Use this template for stateless HTTP service implementations.

```go
package gateway

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-logr/logr"
    "github.com/prometheus/client_golang/prometheus"

    "sigs.k8s.io/controller-runtime/pkg/client"

    "github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// GatewayService handles webhook ingestion
type GatewayService interface {
    HandleWebhook(w http.ResponseWriter, r *http.Request)
}

type GatewayServiceImpl struct {
    client  client.Client
    logger  logr.Logger
    metrics *prometheus.CounterVec
}

func NewGatewayService(client client.Client, logger logr.Logger) *GatewayServiceImpl {
    return &GatewayServiceImpl{
        client: client,
        logger: logger,
    }
}

func (s *GatewayServiceImpl) HandleWebhook(w http.ResponseWriter, r *http.Request) {
    // Implementation...
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "received"})
}

func (s *GatewayServiceImpl) SetupRoutes(r chi.Router) {
    r.Post("/api/v1/webhook", s.HandleWebhook)
}
```

**Key Patterns**:
- Package: Service domain name
- HTTP/encoding imports after standard library
- Third-party HTTP frameworks (chi, gin, etc.)
- Observability imports (prometheus, tracing)

---

### Template 5: Test File (Ginkgo/Gomega)

Use this template for BDD-style test files.

```go
package controller_test

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    "github.com/jordigilh/kubernaut/internal/controller"
    "github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("RemediationRequest Controller", func() {
    var (
        ctx        context.Context
        reconciler *controller.RemediationRequestReconciler
        testClient client.Client
    )

    BeforeEach(func() {
        ctx = context.Background()
        testClient = testutil.NewFakeClient()
        reconciler = &controller.RemediationRequestReconciler{
            Client: testClient,
            Scheme: testutil.GetScheme(),
        }
    })

    Context("when reconciling a RemediationRequest", func() {
        It("should successfully create processing CRD", func() {
            // Test implementation...
            result, err := reconciler.Reconcile(ctx, ctrl.Request{})
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Requeue).To(BeFalse())
        })
    })
})
```

**Key Patterns**:
- Package: `<package>_test` for black-box testing
- Ginkgo/Gomega imported with dot imports (`.`)
- Test utilities in `pkg/testutil`
- BDD structure: Describe/Context/It

---

## Test-Specific Import Patterns

### Test Import Guidelines

**Purpose**: Tests should have complete, copy-paste ready imports to ensure pristine documentation.

**Core Principles**:
1. **Complete Imports**: All potential dependencies included upfront
2. **Progressive Disclosure**: TDD phases may not use all imports initially
3. **HTTP Testing**: Include `net/http` and `net/http/httptest` for service tests
4. **Time Dependencies**: Include `time` for any temporal operations
5. **Context Usage**: Always include `context` for concurrent operations

---

### Template 6: Unit Test (Pure Logic)

For testing pure business logic without infrastructure dependencies.

```go
package processor_test

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/processor"
)

var _ = Describe("Business Logic Calculator", func() {
    var calculator *processor.Calculator

    BeforeEach(func() {
        calculator = processor.NewCalculator()
    })

    Context("Score Calculation", func() {
        It("should calculate effectiveness score correctly", func() {
            result := calculator.Calculate(0.85)
            Expect(result).To(BeNumerically(">", 0.8))
        })
    })
})
```

**Key Patterns**:
- Minimal imports for pure logic tests
- `context` and `time` included for common patterns
- No infrastructure mocking required
- Focus on algorithm correctness

---

### Template 7: Integration Test (HTTP Service)

For testing HTTP services with cross-service integration.

```go
package gateway_test

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/gateway"
    "github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("Gateway Service Integration", func() {
    var (
        ctx     context.Context
        service *gateway.Service
        server  *httptest.Server
    )

    BeforeEach(func() {
        ctx = context.Background()
        service = gateway.NewService()
        server = httptest.NewServer(http.HandlerFunc(service.HandleWebhook))
    })

    AfterEach(func() {
        server.Close()
    })

    Context("Webhook Ingestion", func() {
        It("should accept Prometheus alert webhook", func() {
            payload := testutil.NewPrometheusAlert()
            body, _ := json.Marshal(payload)

            resp, err := http.Post(server.URL+"/webhook", "application/json", bytes.NewBuffer(body))

            Expect(err).ToNot(HaveOccurred())
            Expect(resp.StatusCode).To(Equal(http.StatusOK))
        })
    })
})
```

**Key Patterns**:
- `bytes` for request body construction
- `encoding/json` for payload marshaling
- `net/http` and `net/http/httptest` for HTTP testing
- `time` for timeout and retry testing
- `context` for request lifecycle management

---

### Template 8: Integration Test (Database/Storage)

For testing database interactions and storage operations.

```go
package storage_test

import (
    "context"
    "database/sql"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/storage"
    "github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("PostgreSQL Integration", func() {
    var (
        ctx     context.Context
        db      *sql.DB
        service *storage.Service
    )

    BeforeEach(func() {
        ctx = context.Background()
        db = testutil.NewTestPostgresDB()
        service = storage.NewService(db)
    })

    AfterEach(func() {
        testutil.CleanupTestDatabase(db)
    })

    Context("Action History Storage", func() {
        It("should persist remediation action to database", func() {
            action := &storage.Action{
                ID:        "act-123",
                Type:      "restart-pod",
                Status:    "success",
                Timestamp: time.Now(),
            }

            err := service.SaveAction(ctx, action)

            Expect(err).ToNot(HaveOccurred())
        })
    })
})
```

**Key Patterns**:
- `database/sql` for database operations
- `time` for timestamp operations
- `context` for query lifecycle
- Test utilities for database setup/cleanup

---

### Template 9: Controller Integration Test

For testing Kubernetes controllers with fake clients.

```go
package controller_test

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    corev1 "k8s.io/api/core/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    "github.com/jordigilh/kubernaut/internal/controller"
    "github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("RemediationRequest Controller Integration", func() {
    var (
        ctx        context.Context
        fakeClient client.Client
        reconciler *controller.RemediationRequestReconciler
        scheme     *runtime.Scheme
    )

    BeforeEach(func() {
        ctx = context.Background()
        scheme = runtime.NewScheme()
        _ = remediationv1.AddToScheme(scheme)
        _ = processingv1.AddToScheme(scheme)

        fakeClient = fake.NewClientBuilder().
            WithScheme(scheme).
            Build()

        reconciler = &controller.RemediationRequestReconciler{
            Client: fakeClient,
            Scheme: scheme,
        }
    })

    Context("CRD Creation", func() {
        It("should create RemediationProcessing CRD when RemediationRequest is created", func() {
            rr := testutil.NewRemediationRequest("test-rr", "default")
            Expect(fakeClient.Create(ctx, rr)).To(Succeed())

            req := ctrl.Request{
                NamespacedName: types.NamespacedName{
                    Name:      rr.Name,
                    Namespace: rr.Namespace,
                },
            }

            result, err := reconciler.Reconcile(ctx, req)

            Expect(err).ToNot(HaveOccurred())
            Expect(result.Requeue).To(BeFalse())

            // Verify RemediationProcessing was created
            var ap processingv1.RemediationProcessing
            key := types.NamespacedName{Name: "test-rr-processing", Namespace: "default"}
            Expect(fakeClient.Get(ctx, key, &ap)).To(Succeed())
        })
    })
})
```

**Key Patterns**:
- Full Kubernetes client and runtime imports
- `fake.NewClientBuilder()` for test client creation
- `time` for requeue delays
- `context` for reconciliation lifecycle
- `types.NamespacedName` for resource lookups
- `apierrors` for error checking

---

### TDD Progressive Import Disclosure

**Principle**: In TDD methodology, tests are written before implementation exists. Imports should reflect the *complete* interface contract, even if implementation hasn't been written yet.

#### Phase Progression Example

**RED Phase** (Test written, implementation doesn't exist):
```go
// test/unit/effectiveness/calculator_test.go
package effectiveness_test

import (
    "context"
    "time"  // ← Included even if not used yet

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/effectiveness"
)

var _ = Describe("Effectiveness Calculator", func() {
    It("should calculate score", func() {
        calculator := effectiveness.NewCalculator()  // DOESN'T EXIST YET
        score := calculator.Calculate(ctx, data)     // DOESN'T EXIST YET
        Expect(score).To(BeNumerically(">", 0.0))
    })
})
```

**GREEN Phase** (Minimal implementation):
```go
// pkg/effectiveness/calculator.go
package effectiveness

type Calculator struct{}

func NewCalculator() *Calculator {
    return &Calculator{}
}

func (c *Calculator) Calculate(ctx context.Context, data *Data) float64 {
    return 0.85  // Minimal implementation
}
```

**REFACTOR Phase** (Enhanced implementation uses time):
```go
// pkg/effectiveness/calculator.go
package effectiveness

import (
    "context"
    "time"  // ← Now actually used
)

type Calculator struct {
    cache map[string]cachedResult
}

type cachedResult struct {
    score     float64
    timestamp time.Time  // ← time now used
}

func (c *Calculator) Calculate(ctx context.Context, data *Data) float64 {
    // Check cache with TTL
    if cached, ok := c.cache[data.ID]; ok {
        if time.Since(cached.timestamp) < 5*time.Minute {
            return cached.score
        }
    }
    // ... sophisticated calculation
}
```

**Key Point**: Test imports included `time` in RED phase even though it wasn't used until REFACTOR phase. This ensures pristine documentation throughout TDD workflow.

---

### HTTP Testing Import Patterns

For services that handle HTTP requests (Gateway, Context API, Notification, Dynamic Toolset):

**Required Imports**:
- `bytes` - Request body construction
- `net/http` - HTTP client operations
- `net/http/httptest` - Test server creation
- `encoding/json` - Payload marshaling
- `time` - Timeout and retry testing

**Example Pattern**:
```go
import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/gateway"
)
```

---

### Mock and Fake Patterns

**Controller Runtime Fake Client**:
```go
import (
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

fakeClient := fake.NewClientBuilder().
    WithScheme(scheme).
    WithObjects(existingObjects...).
    Build()
```

**HTTP Test Server**:
```go
import (
    "net/http"
    "net/http/httptest"
)

server := httptest.NewServer(http.HandlerFunc(handler))
defer server.Close()
```

**External Service Mocking**:
```go
import (
    "github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

mockAIService := mocks.NewMockAIService()
mockAIService.SetResponse("analysis-result", &aiResult)
```

---

### Test Import Checklist

When writing test documentation, ensure:

- [ ] `context` imported for lifecycle management
- [ ] `time` imported for temporal operations
- [ ] Ginkgo/Gomega with dot imports (`. "github.com/onsi/ginkgo/v2"`)
- [ ] HTTP imports (`net/http`, `net/http/httptest`) for service tests
- [ ] `bytes` and `encoding/json` for payload construction
- [ ] Fake client imports for controller tests
- [ ] Test utilities (`pkg/testutil`) imported
- [ ] All business logic package imports present
- [ ] No missing imports that would prevent copy-paste execution

---

## Import Ordering Rules

### Order of Import Groups

1. **Standard Library**: `context`, `fmt`, `time`, `strings`, etc.
2. **Third-Party Libraries**: `github.com/go-logr`, `github.com/prometheus`, etc.
3. **Kubernetes Core**: `k8s.io/api/*`, `k8s.io/apimachinery/*`
4. **Kubernetes Extensions**: `k8s.io/client-go/*`
5. **Controller Runtime**: `sigs.k8s.io/controller-runtime/*`
6. **Project APIs**: `github.com/jordigilh/kubernaut/api/*`
7. **Project Packages**: `github.com/jordigilh/kubernaut/pkg/*`, `github.com/jordigilh/kubernaut/internal/*`

### Within Each Group

- Alphabetical order by import path
- Use blank lines to separate groups
- Keep related imports together

**Example**:
```go
import (
    "context"
    "fmt"
    "time"

    "github.com/go-logr/logr"
    "github.com/prometheus/client_golang/prometheus"

    corev1 "k8s.io/api/core/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    "k8s.io/client-go/tools/record"

    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"

    "github.com/jordigilh/kubernaut/pkg/processing"
    "github.com/jordigilh/kubernaut/pkg/shared/types"
)
```

---

## Common Patterns by Use Case

### Pattern: Error Handling with apierrors

```go
import (
    apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// Check if resource not found
if err := r.Get(ctx, req.NamespacedName, &resource); err != nil {
    if apierrors.IsNotFound(err) {
        return ctrl.Result{}, nil
    }
    return ctrl.Result{}, err
}

// Check if already exists
if err := r.Create(ctx, resource); err != nil {
    if !apierrors.IsAlreadyExists(err) {
        return ctrl.Result{}, err
    }
}
```

### Pattern: Status Conditions with apimeta

```go
import (
    apimeta "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Set status condition
apimeta.SetStatusCondition(&resource.Status.Conditions, metav1.Condition{
    Type:               "Ready",
    Status:             metav1.ConditionTrue,
    Reason:             "ReconciliationSucceeded",
    Message:            "Resource reconciled successfully",
    LastTransitionTime: metav1.Now(),
})
```

### Pattern: Event Recording

```go
import (
    corev1 "k8s.io/api/core/v1"
    "k8s.io/client-go/tools/record"
)

type Reconciler struct {
    Recorder record.EventRecorder
}

// Emit events
r.Recorder.Event(resource, corev1.EventTypeNormal, "Created", "Successfully created resource")
r.Recorder.Event(resource, corev1.EventTypeWarning, "Failed", fmt.Sprintf("Failed: %v", err))
```

### Pattern: Finalizer Management

```go
import (
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const finalizerName = "kubernaut.io/finalizer"

// Add finalizer
if !controllerutil.ContainsFinalizer(resource, finalizerName) {
    controllerutil.AddFinalizer(resource, finalizerName)
    if err := r.Update(ctx, resource); err != nil {
        return ctrl.Result{}, err
    }
}

// Remove finalizer
controllerutil.RemoveFinalizer(resource, finalizerName)
if err := r.Update(ctx, resource); err != nil {
    return ctrl.Result{}, err
}
```

---

## Documentation Code Sample Checklist

When adding Go code samples to documentation, ensure:

- [ ] Package declaration present
- [ ] Complete import block with correct aliases
- [ ] Import groups properly ordered
- [ ] All referenced types have corresponding imports
- [ ] Standard aliases used (corev1, metav1, ctrl, apierrors, apimeta)
- [ ] Project-specific imports use full path with alias
- [ ] No unused imports (add `// ... other imports` comment if abbreviated)
- [ ] Code sample compiles without modification

---

## Tools and Validation

### Goimports

Use `goimports` to automatically format and organize imports:

```bash
goimports -w <file>.go
```

### Manual Validation

Extract code sample and verify compilation:

```bash
# Extract code block from markdown
cat documentation.md | sed -n '/```go/,/```/p' | sed '1d;$d' > sample.go

# Add module and dependencies
go mod init sample
go get k8s.io/client-go@latest
go get sigs.k8s.io/controller-runtime@latest

# Verify compilation
go build sample.go
```

---

## References

- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [Controller Runtime Documentation](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Effective Go](https://go.dev/doc/effective_go)

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| v1.1 | 2025-10-09 | Added test-specific import patterns (Templates 6-9), TDD progressive disclosure, HTTP testing patterns |
| v1.0 | 2025-10-09 | Initial template creation with standardized patterns |

---

**Maintained by**: Kubernaut Development Team
**Questions**: File issue in [github.com/jordigilh/kubernaut](https://github.com/jordigilh/kubernaut)

