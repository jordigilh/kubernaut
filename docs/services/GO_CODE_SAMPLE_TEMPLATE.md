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
| v1.0 | 2025-10-09 | Initial template creation with standardized patterns |

---

**Maintained by**: Kubernaut Development Team
**Questions**: File issue in [github.com/jordigilh/kubernaut](https://github.com/jordigilh/kubernaut)

