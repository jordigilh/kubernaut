# Go Code Standards for Implementation Plans

**Status**: MANDATORY for all expansion plans
**Purpose**: Ensure all Go code examples are production-ready with complete imports

---

## Import Statement Requirements

### Every Go Code Block MUST Include

1. **Complete package declaration**
   ```go
   package packagename  // NEVER use packagename_test for test files
   ```
   **⚠️ CRITICAL**: Test files MUST use the same package name as the source code (white-box testing).
   - ✅ CORRECT: `package remediationprocessing` in both `remediationprocessing.go` and `remediationprocessing_test.go`
   - ❌ WRONG: `package remediationprocessing_test` in test files

2. **Full import statements** (not just partial)
   ```go
   import (
       "context"
       "fmt"
       "time"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

       notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
   )
   ```

3. **All referenced types imported** - No undefined symbols

---

## Standard Import Sets by File Type

### Test Files (Ginkgo/Gomega)
```go
package packagename  // Same package as source code (white-box testing)

import (
    "context"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"

    // CRD imports
    remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1alpha1"
)

func TestSuiteName(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Suite Name")
}
```

### Controller Files
```go
package controllername

import (
    "context"
    "fmt"
    "time"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    // CRD imports
    somev1alpha1 "github.com/jordigilh/kubernaut/api/some/v1alpha1"
)
```

### Business Logic Files
```go
package packagename

import (
    "context"
    "fmt"
    "time"

    "github.com/sirupsen/logrus"

    // Internal imports
    "github.com/jordigilh/kubernaut/pkg/some/dependency"
)
```

### Integration Test Files (Envtest)
```go
package packagename

import (
    "context"
    "path/filepath"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/kubernetes/scheme"
    "k8s.io/client-go/rest"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    // CRD imports
    somev1alpha1 "github.com/jordigilh/kubernaut/api/some/v1alpha1"
)

var (
    cfg       *rest.Config
    k8sClient client.Client
    testEnv   *envtest.Environment
    ctx       context.Context
    cancel    context.CancelFunc
)

var _ = BeforeSuite(func() {
    ctx, cancel = context.WithCancel(context.Background())

    By("Bootstrapping test environment")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
        ErrorIfCRDPathMissing: true,
    }

    var err error
    cfg, err = testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    Expect(cfg).NotTo(BeNil())

    err = somev1alpha1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())

    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())
    Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
    cancel()
    By("Tearing down the test environment")
    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())
})
```

### Main Application Files
```go
package main

import (
    "flag"
    "os"
    "time"

    "k8s.io/apimachinery/pkg/runtime"
    utilruntime "k8s.io/apimachinery/pkg/util/runtime"
    clientgoscheme "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/healthz"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
    metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

    // CRD imports
    somev1alpha1 "github.com/jordigilh/kubernaut/api/some/v1alpha1"

    // Internal imports
    "github.com/jordigilh/kubernaut/internal/controller/some"
    "github.com/jordigilh/kubernaut/pkg/some/component"
)

var (
    scheme   = runtime.NewScheme()
    setupLog = ctrl.Log.WithName("setup")
)

func init() {
    utilruntime.Must(clientgoscheme.AddToScheme(scheme))
    utilruntime.Must(somev1alpha1.AddToScheme(scheme))
}
```

---

## Common Import Groups

### Standard Library
```go
import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "time"
)
```

### Kubernetes Core
```go
import (
    corev1 "k8s.io/api/core/v1"
    appsv1 "k8s.io/api/apps/v1"
    batchv1 "k8s.io/api/batch/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
    "k8s.io/client-go/kubernetes"
)
```

### Controller-Runtime
```go
import (
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller"
    "sigs.k8s.io/controller-runtime/pkg/log"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
)
```

### Testing (Ginkgo/Gomega)
```go
import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)
```

### Database
```go
import (
    "database/sql"

    "github.com/lib/pq"
    "github.com/pgvector/pgvector-go"
)
```

### Metrics
```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
    "sigs.k8s.io/controller-runtime/pkg/metrics"
)
```

### Rego Policy
```go
import (
    "github.com/open-policy-agent/opa/rego"
)
```

---

## Helper Function: ptrInt32

**Every plan that uses `ptrInt32` MUST include this helper**:

```go
// Helper function for pointer conversion
func ptrInt32(i int32) *int32 {
    return &i
}
```

**Or import from a common package**:
```go
import (
    "k8s.io/utils/ptr"
)

// Then use:
ptr.To[int32](5)
```

---

## Validation Checklist

Before considering any code example "complete":

- [ ] Package declaration present
- [ ] Import block with ALL dependencies
- [ ] No undefined symbols
- [ ] Standard library imports separated from third-party
- [ ] Third-party imports separated from internal
- [ ] Alias imports for clarity (e.g., `metav1`, `ctrl`)
- [ ] Helper functions defined or imported
- [ ] No `// import statements` comments without actual imports

---

## Examples of INCOMPLETE vs COMPLETE Code

### ❌ INCOMPLETE (Current Plans)
```go
var _ = Describe("BR-REMEDIATION-002: Context Enrichment", func() {
    var enricher *ContextEnricher
    // ... test code
})
```

### ✅ COMPLETE (Required)
```go
package remediationprocessing_test

import (
    "context"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/remediationprocessing/enricher"
)

func TestRemediationProcessing(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Remediation Processing Suite")
}

var _ = Describe("BR-REMEDIATION-002: Context Enrichment", func() {
    var (
        ctx      context.Context
        enricher *enricher.ContextEnricher
    )

    BeforeEach(func() {
        ctx = context.Background()
        enricher = enricher.NewContextEnricher()
    })

    // ... test code
})
```

---

## Application to Expansion Plans

### All Three Plans Must Include

1. **Test Files**: Complete imports at the start of every test code block
2. **Implementation Files**: Complete imports at the start of every implementation block
3. **Integration Tests**: Full Envtest setup with imports
4. **Main Files**: Complete main.go with all dependencies

### Update Priority

1. **High Priority**: Test files (most visible in plans)
2. **High Priority**: Integration test suite templates
3. **Medium Priority**: Implementation files
4. **Medium Priority**: Main application files

---

**Status**: This standard applies to ALL expansion plans immediately. Plans will be updated before implementation begins.

