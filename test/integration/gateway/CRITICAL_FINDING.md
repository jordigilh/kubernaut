# 🚨 CRITICAL FINDING: K8s Dropping stormAggregation Field

## ✅ **Confirmed: JSON Payload is Correct**

The JSON being sent to K8s **DOES** include `stormAggregation`:

```json
{
  "kind": "RemediationRequest",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",
  "spec": {
    "stormAggregation": {
      "pattern": "HighMemoryUsage in prod-payments",
      "alertCount": 1,
      "affectedResources": [
        {
          "kind": "Pod",
          "name": "payment-api-1",
          "namespace": "prod-payments"
        }
      ],
      "aggregationWindow": "5m",
      "firstSeen": "2025-10-27T13:52:00Z",
      "lastSeen": "2025-10-27T13:52:00Z"
    }
  }
}
```

## ❌ **Problem: K8s Drops the Field**

When reading the CRD back from K8s, `StormAggregation` is **nil**, causing a panic:

```
panic: runtime error: nil pointer dereference
-> github.com/jordigilh/kubernaut/pkg/gateway/server.(*Server).respondAggregatedAlert
->   /Users/jgil/go/src/github.com/jordigilh/kubernaut/pkg/gateway/server/handlers.go:433
```

Line 433: `stormCRD.Spec.StormAggregation.AlertCount` - panics because `StormAggregation` is nil

## 🔍 **What We've Ruled Out**

1. ✅ JSON marshaling - Field is present in JSON
2. ✅ Go struct tags - `json:"stormAggregation,omitempty"` is correct
3. ✅ CRD schema - Field exists with correct structure
4. ✅ APIVersion/Kind - Now set correctly
5. ✅ Scheme registration - `remediationv1alpha1.AddToScheme(scheme)` called
6. ✅ Fresh cluster - Deleted and recreated Kind cluster
7. ✅ Required fields - All required fields are present in JSON

## 🤔 **Hypothesis: CRD Schema Issue**

The K8s API server warning persists:
```
unknown field "spec.stormAggregation"
```

This suggests the API server doesn't recognize the field, even though:
- The CRD definition includes it
- The JSON payload includes it
- All validation passes

## 🎯 **Next Steps**

### Option 1: Check CRD Installation
Maybe the CRD wasn't actually updated in K8s?

```bash
KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o yaml > /tmp/installed-crd.yaml
diff /tmp/installed-crd.yaml config/crd/remediation.kubernaut.io_remediationrequests.yaml
```

### Option 2: Check controller-runtime Client
Maybe there's a bug in how controller-runtime handles optional pointer fields?

### Option 3: Regenerate CRD from Go Types
Maybe the CRD file is out of sync with the Go types?

```bash
# Fix the controller-gen error first
make manifests
```

### Option 4: Try Without Pointer
Change the Go struct to use a value instead of pointer:

```go
// Instead of:
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`

// Try:
StormAggregation StormAggregation `json:"stormAggregation,omitempty"`
```

## 📊 **Impact**

- **Test Status**: Failing (1 test)
- **Root Cause**: K8s dropping `stormAggregation` field
- **Workaround**: None identified yet
- **Severity**: **CRITICAL** - Blocks storm aggregation feature

## 🚀 **Recommendation**

Try **Option 3** (regenerate CRD) first, as this is most likely to reveal any schema mismatch.



## ✅ **Confirmed: JSON Payload is Correct**

The JSON being sent to K8s **DOES** include `stormAggregation`:

```json
{
  "kind": "RemediationRequest",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",
  "spec": {
    "stormAggregation": {
      "pattern": "HighMemoryUsage in prod-payments",
      "alertCount": 1,
      "affectedResources": [
        {
          "kind": "Pod",
          "name": "payment-api-1",
          "namespace": "prod-payments"
        }
      ],
      "aggregationWindow": "5m",
      "firstSeen": "2025-10-27T13:52:00Z",
      "lastSeen": "2025-10-27T13:52:00Z"
    }
  }
}
```

## ❌ **Problem: K8s Drops the Field**

When reading the CRD back from K8s, `StormAggregation` is **nil**, causing a panic:

```
panic: runtime error: nil pointer dereference
-> github.com/jordigilh/kubernaut/pkg/gateway/server.(*Server).respondAggregatedAlert
->   /Users/jgil/go/src/github.com/jordigilh/kubernaut/pkg/gateway/server/handlers.go:433
```

Line 433: `stormCRD.Spec.StormAggregation.AlertCount` - panics because `StormAggregation` is nil

## 🔍 **What We've Ruled Out**

1. ✅ JSON marshaling - Field is present in JSON
2. ✅ Go struct tags - `json:"stormAggregation,omitempty"` is correct
3. ✅ CRD schema - Field exists with correct structure
4. ✅ APIVersion/Kind - Now set correctly
5. ✅ Scheme registration - `remediationv1alpha1.AddToScheme(scheme)` called
6. ✅ Fresh cluster - Deleted and recreated Kind cluster
7. ✅ Required fields - All required fields are present in JSON

## 🤔 **Hypothesis: CRD Schema Issue**

The K8s API server warning persists:
```
unknown field "spec.stormAggregation"
```

This suggests the API server doesn't recognize the field, even though:
- The CRD definition includes it
- The JSON payload includes it
- All validation passes

## 🎯 **Next Steps**

### Option 1: Check CRD Installation
Maybe the CRD wasn't actually updated in K8s?

```bash
KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o yaml > /tmp/installed-crd.yaml
diff /tmp/installed-crd.yaml config/crd/remediation.kubernaut.io_remediationrequests.yaml
```

### Option 2: Check controller-runtime Client
Maybe there's a bug in how controller-runtime handles optional pointer fields?

### Option 3: Regenerate CRD from Go Types
Maybe the CRD file is out of sync with the Go types?

```bash
# Fix the controller-gen error first
make manifests
```

### Option 4: Try Without Pointer
Change the Go struct to use a value instead of pointer:

```go
// Instead of:
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`

// Try:
StormAggregation StormAggregation `json:"stormAggregation,omitempty"`
```

## 📊 **Impact**

- **Test Status**: Failing (1 test)
- **Root Cause**: K8s dropping `stormAggregation` field
- **Workaround**: None identified yet
- **Severity**: **CRITICAL** - Blocks storm aggregation feature

## 🚀 **Recommendation**

Try **Option 3** (regenerate CRD) first, as this is most likely to reveal any schema mismatch.

# 🚨 CRITICAL FINDING: K8s Dropping stormAggregation Field

## ✅ **Confirmed: JSON Payload is Correct**

The JSON being sent to K8s **DOES** include `stormAggregation`:

```json
{
  "kind": "RemediationRequest",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",
  "spec": {
    "stormAggregation": {
      "pattern": "HighMemoryUsage in prod-payments",
      "alertCount": 1,
      "affectedResources": [
        {
          "kind": "Pod",
          "name": "payment-api-1",
          "namespace": "prod-payments"
        }
      ],
      "aggregationWindow": "5m",
      "firstSeen": "2025-10-27T13:52:00Z",
      "lastSeen": "2025-10-27T13:52:00Z"
    }
  }
}
```

## ❌ **Problem: K8s Drops the Field**

When reading the CRD back from K8s, `StormAggregation` is **nil**, causing a panic:

```
panic: runtime error: nil pointer dereference
-> github.com/jordigilh/kubernaut/pkg/gateway/server.(*Server).respondAggregatedAlert
->   /Users/jgil/go/src/github.com/jordigilh/kubernaut/pkg/gateway/server/handlers.go:433
```

Line 433: `stormCRD.Spec.StormAggregation.AlertCount` - panics because `StormAggregation` is nil

## 🔍 **What We've Ruled Out**

1. ✅ JSON marshaling - Field is present in JSON
2. ✅ Go struct tags - `json:"stormAggregation,omitempty"` is correct
3. ✅ CRD schema - Field exists with correct structure
4. ✅ APIVersion/Kind - Now set correctly
5. ✅ Scheme registration - `remediationv1alpha1.AddToScheme(scheme)` called
6. ✅ Fresh cluster - Deleted and recreated Kind cluster
7. ✅ Required fields - All required fields are present in JSON

## 🤔 **Hypothesis: CRD Schema Issue**

The K8s API server warning persists:
```
unknown field "spec.stormAggregation"
```

This suggests the API server doesn't recognize the field, even though:
- The CRD definition includes it
- The JSON payload includes it
- All validation passes

## 🎯 **Next Steps**

### Option 1: Check CRD Installation
Maybe the CRD wasn't actually updated in K8s?

```bash
KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o yaml > /tmp/installed-crd.yaml
diff /tmp/installed-crd.yaml config/crd/remediation.kubernaut.io_remediationrequests.yaml
```

### Option 2: Check controller-runtime Client
Maybe there's a bug in how controller-runtime handles optional pointer fields?

### Option 3: Regenerate CRD from Go Types
Maybe the CRD file is out of sync with the Go types?

```bash
# Fix the controller-gen error first
make manifests
```

### Option 4: Try Without Pointer
Change the Go struct to use a value instead of pointer:

```go
// Instead of:
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`

// Try:
StormAggregation StormAggregation `json:"stormAggregation,omitempty"`
```

## 📊 **Impact**

- **Test Status**: Failing (1 test)
- **Root Cause**: K8s dropping `stormAggregation` field
- **Workaround**: None identified yet
- **Severity**: **CRITICAL** - Blocks storm aggregation feature

## 🚀 **Recommendation**

Try **Option 3** (regenerate CRD) first, as this is most likely to reveal any schema mismatch.

# 🚨 CRITICAL FINDING: K8s Dropping stormAggregation Field

## ✅ **Confirmed: JSON Payload is Correct**

The JSON being sent to K8s **DOES** include `stormAggregation`:

```json
{
  "kind": "RemediationRequest",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",
  "spec": {
    "stormAggregation": {
      "pattern": "HighMemoryUsage in prod-payments",
      "alertCount": 1,
      "affectedResources": [
        {
          "kind": "Pod",
          "name": "payment-api-1",
          "namespace": "prod-payments"
        }
      ],
      "aggregationWindow": "5m",
      "firstSeen": "2025-10-27T13:52:00Z",
      "lastSeen": "2025-10-27T13:52:00Z"
    }
  }
}
```

## ❌ **Problem: K8s Drops the Field**

When reading the CRD back from K8s, `StormAggregation` is **nil**, causing a panic:

```
panic: runtime error: nil pointer dereference
-> github.com/jordigilh/kubernaut/pkg/gateway/server.(*Server).respondAggregatedAlert
->   /Users/jgil/go/src/github.com/jordigilh/kubernaut/pkg/gateway/server/handlers.go:433
```

Line 433: `stormCRD.Spec.StormAggregation.AlertCount` - panics because `StormAggregation` is nil

## 🔍 **What We've Ruled Out**

1. ✅ JSON marshaling - Field is present in JSON
2. ✅ Go struct tags - `json:"stormAggregation,omitempty"` is correct
3. ✅ CRD schema - Field exists with correct structure
4. ✅ APIVersion/Kind - Now set correctly
5. ✅ Scheme registration - `remediationv1alpha1.AddToScheme(scheme)` called
6. ✅ Fresh cluster - Deleted and recreated Kind cluster
7. ✅ Required fields - All required fields are present in JSON

## 🤔 **Hypothesis: CRD Schema Issue**

The K8s API server warning persists:
```
unknown field "spec.stormAggregation"
```

This suggests the API server doesn't recognize the field, even though:
- The CRD definition includes it
- The JSON payload includes it
- All validation passes

## 🎯 **Next Steps**

### Option 1: Check CRD Installation
Maybe the CRD wasn't actually updated in K8s?

```bash
KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o yaml > /tmp/installed-crd.yaml
diff /tmp/installed-crd.yaml config/crd/remediation.kubernaut.io_remediationrequests.yaml
```

### Option 2: Check controller-runtime Client
Maybe there's a bug in how controller-runtime handles optional pointer fields?

### Option 3: Regenerate CRD from Go Types
Maybe the CRD file is out of sync with the Go types?

```bash
# Fix the controller-gen error first
make manifests
```

### Option 4: Try Without Pointer
Change the Go struct to use a value instead of pointer:

```go
// Instead of:
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`

// Try:
StormAggregation StormAggregation `json:"stormAggregation,omitempty"`
```

## 📊 **Impact**

- **Test Status**: Failing (1 test)
- **Root Cause**: K8s dropping `stormAggregation` field
- **Workaround**: None identified yet
- **Severity**: **CRITICAL** - Blocks storm aggregation feature

## 🚀 **Recommendation**

Try **Option 3** (regenerate CRD) first, as this is most likely to reveal any schema mismatch.



## ✅ **Confirmed: JSON Payload is Correct**

The JSON being sent to K8s **DOES** include `stormAggregation`:

```json
{
  "kind": "RemediationRequest",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",
  "spec": {
    "stormAggregation": {
      "pattern": "HighMemoryUsage in prod-payments",
      "alertCount": 1,
      "affectedResources": [
        {
          "kind": "Pod",
          "name": "payment-api-1",
          "namespace": "prod-payments"
        }
      ],
      "aggregationWindow": "5m",
      "firstSeen": "2025-10-27T13:52:00Z",
      "lastSeen": "2025-10-27T13:52:00Z"
    }
  }
}
```

## ❌ **Problem: K8s Drops the Field**

When reading the CRD back from K8s, `StormAggregation` is **nil**, causing a panic:

```
panic: runtime error: nil pointer dereference
-> github.com/jordigilh/kubernaut/pkg/gateway/server.(*Server).respondAggregatedAlert
->   /Users/jgil/go/src/github.com/jordigilh/kubernaut/pkg/gateway/server/handlers.go:433
```

Line 433: `stormCRD.Spec.StormAggregation.AlertCount` - panics because `StormAggregation` is nil

## 🔍 **What We've Ruled Out**

1. ✅ JSON marshaling - Field is present in JSON
2. ✅ Go struct tags - `json:"stormAggregation,omitempty"` is correct
3. ✅ CRD schema - Field exists with correct structure
4. ✅ APIVersion/Kind - Now set correctly
5. ✅ Scheme registration - `remediationv1alpha1.AddToScheme(scheme)` called
6. ✅ Fresh cluster - Deleted and recreated Kind cluster
7. ✅ Required fields - All required fields are present in JSON

## 🤔 **Hypothesis: CRD Schema Issue**

The K8s API server warning persists:
```
unknown field "spec.stormAggregation"
```

This suggests the API server doesn't recognize the field, even though:
- The CRD definition includes it
- The JSON payload includes it
- All validation passes

## 🎯 **Next Steps**

### Option 1: Check CRD Installation
Maybe the CRD wasn't actually updated in K8s?

```bash
KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o yaml > /tmp/installed-crd.yaml
diff /tmp/installed-crd.yaml config/crd/remediation.kubernaut.io_remediationrequests.yaml
```

### Option 2: Check controller-runtime Client
Maybe there's a bug in how controller-runtime handles optional pointer fields?

### Option 3: Regenerate CRD from Go Types
Maybe the CRD file is out of sync with the Go types?

```bash
# Fix the controller-gen error first
make manifests
```

### Option 4: Try Without Pointer
Change the Go struct to use a value instead of pointer:

```go
// Instead of:
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`

// Try:
StormAggregation StormAggregation `json:"stormAggregation,omitempty"`
```

## 📊 **Impact**

- **Test Status**: Failing (1 test)
- **Root Cause**: K8s dropping `stormAggregation` field
- **Workaround**: None identified yet
- **Severity**: **CRITICAL** - Blocks storm aggregation feature

## 🚀 **Recommendation**

Try **Option 3** (regenerate CRD) first, as this is most likely to reveal any schema mismatch.

# 🚨 CRITICAL FINDING: K8s Dropping stormAggregation Field

## ✅ **Confirmed: JSON Payload is Correct**

The JSON being sent to K8s **DOES** include `stormAggregation`:

```json
{
  "kind": "RemediationRequest",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",
  "spec": {
    "stormAggregation": {
      "pattern": "HighMemoryUsage in prod-payments",
      "alertCount": 1,
      "affectedResources": [
        {
          "kind": "Pod",
          "name": "payment-api-1",
          "namespace": "prod-payments"
        }
      ],
      "aggregationWindow": "5m",
      "firstSeen": "2025-10-27T13:52:00Z",
      "lastSeen": "2025-10-27T13:52:00Z"
    }
  }
}
```

## ❌ **Problem: K8s Drops the Field**

When reading the CRD back from K8s, `StormAggregation` is **nil**, causing a panic:

```
panic: runtime error: nil pointer dereference
-> github.com/jordigilh/kubernaut/pkg/gateway/server.(*Server).respondAggregatedAlert
->   /Users/jgil/go/src/github.com/jordigilh/kubernaut/pkg/gateway/server/handlers.go:433
```

Line 433: `stormCRD.Spec.StormAggregation.AlertCount` - panics because `StormAggregation` is nil

## 🔍 **What We've Ruled Out**

1. ✅ JSON marshaling - Field is present in JSON
2. ✅ Go struct tags - `json:"stormAggregation,omitempty"` is correct
3. ✅ CRD schema - Field exists with correct structure
4. ✅ APIVersion/Kind - Now set correctly
5. ✅ Scheme registration - `remediationv1alpha1.AddToScheme(scheme)` called
6. ✅ Fresh cluster - Deleted and recreated Kind cluster
7. ✅ Required fields - All required fields are present in JSON

## 🤔 **Hypothesis: CRD Schema Issue**

The K8s API server warning persists:
```
unknown field "spec.stormAggregation"
```

This suggests the API server doesn't recognize the field, even though:
- The CRD definition includes it
- The JSON payload includes it
- All validation passes

## 🎯 **Next Steps**

### Option 1: Check CRD Installation
Maybe the CRD wasn't actually updated in K8s?

```bash
KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o yaml > /tmp/installed-crd.yaml
diff /tmp/installed-crd.yaml config/crd/remediation.kubernaut.io_remediationrequests.yaml
```

### Option 2: Check controller-runtime Client
Maybe there's a bug in how controller-runtime handles optional pointer fields?

### Option 3: Regenerate CRD from Go Types
Maybe the CRD file is out of sync with the Go types?

```bash
# Fix the controller-gen error first
make manifests
```

### Option 4: Try Without Pointer
Change the Go struct to use a value instead of pointer:

```go
// Instead of:
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`

// Try:
StormAggregation StormAggregation `json:"stormAggregation,omitempty"`
```

## 📊 **Impact**

- **Test Status**: Failing (1 test)
- **Root Cause**: K8s dropping `stormAggregation` field
- **Workaround**: None identified yet
- **Severity**: **CRITICAL** - Blocks storm aggregation feature

## 🚀 **Recommendation**

Try **Option 3** (regenerate CRD) first, as this is most likely to reveal any schema mismatch.

# 🚨 CRITICAL FINDING: K8s Dropping stormAggregation Field

## ✅ **Confirmed: JSON Payload is Correct**

The JSON being sent to K8s **DOES** include `stormAggregation`:

```json
{
  "kind": "RemediationRequest",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",
  "spec": {
    "stormAggregation": {
      "pattern": "HighMemoryUsage in prod-payments",
      "alertCount": 1,
      "affectedResources": [
        {
          "kind": "Pod",
          "name": "payment-api-1",
          "namespace": "prod-payments"
        }
      ],
      "aggregationWindow": "5m",
      "firstSeen": "2025-10-27T13:52:00Z",
      "lastSeen": "2025-10-27T13:52:00Z"
    }
  }
}
```

## ❌ **Problem: K8s Drops the Field**

When reading the CRD back from K8s, `StormAggregation` is **nil**, causing a panic:

```
panic: runtime error: nil pointer dereference
-> github.com/jordigilh/kubernaut/pkg/gateway/server.(*Server).respondAggregatedAlert
->   /Users/jgil/go/src/github.com/jordigilh/kubernaut/pkg/gateway/server/handlers.go:433
```

Line 433: `stormCRD.Spec.StormAggregation.AlertCount` - panics because `StormAggregation` is nil

## 🔍 **What We've Ruled Out**

1. ✅ JSON marshaling - Field is present in JSON
2. ✅ Go struct tags - `json:"stormAggregation,omitempty"` is correct
3. ✅ CRD schema - Field exists with correct structure
4. ✅ APIVersion/Kind - Now set correctly
5. ✅ Scheme registration - `remediationv1alpha1.AddToScheme(scheme)` called
6. ✅ Fresh cluster - Deleted and recreated Kind cluster
7. ✅ Required fields - All required fields are present in JSON

## 🤔 **Hypothesis: CRD Schema Issue**

The K8s API server warning persists:
```
unknown field "spec.stormAggregation"
```

This suggests the API server doesn't recognize the field, even though:
- The CRD definition includes it
- The JSON payload includes it
- All validation passes

## 🎯 **Next Steps**

### Option 1: Check CRD Installation
Maybe the CRD wasn't actually updated in K8s?

```bash
KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o yaml > /tmp/installed-crd.yaml
diff /tmp/installed-crd.yaml config/crd/remediation.kubernaut.io_remediationrequests.yaml
```

### Option 2: Check controller-runtime Client
Maybe there's a bug in how controller-runtime handles optional pointer fields?

### Option 3: Regenerate CRD from Go Types
Maybe the CRD file is out of sync with the Go types?

```bash
# Fix the controller-gen error first
make manifests
```

### Option 4: Try Without Pointer
Change the Go struct to use a value instead of pointer:

```go
// Instead of:
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`

// Try:
StormAggregation StormAggregation `json:"stormAggregation,omitempty"`
```

## 📊 **Impact**

- **Test Status**: Failing (1 test)
- **Root Cause**: K8s dropping `stormAggregation` field
- **Workaround**: None identified yet
- **Severity**: **CRITICAL** - Blocks storm aggregation feature

## 🚀 **Recommendation**

Try **Option 3** (regenerate CRD) first, as this is most likely to reveal any schema mismatch.



## ✅ **Confirmed: JSON Payload is Correct**

The JSON being sent to K8s **DOES** include `stormAggregation`:

```json
{
  "kind": "RemediationRequest",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",
  "spec": {
    "stormAggregation": {
      "pattern": "HighMemoryUsage in prod-payments",
      "alertCount": 1,
      "affectedResources": [
        {
          "kind": "Pod",
          "name": "payment-api-1",
          "namespace": "prod-payments"
        }
      ],
      "aggregationWindow": "5m",
      "firstSeen": "2025-10-27T13:52:00Z",
      "lastSeen": "2025-10-27T13:52:00Z"
    }
  }
}
```

## ❌ **Problem: K8s Drops the Field**

When reading the CRD back from K8s, `StormAggregation` is **nil**, causing a panic:

```
panic: runtime error: nil pointer dereference
-> github.com/jordigilh/kubernaut/pkg/gateway/server.(*Server).respondAggregatedAlert
->   /Users/jgil/go/src/github.com/jordigilh/kubernaut/pkg/gateway/server/handlers.go:433
```

Line 433: `stormCRD.Spec.StormAggregation.AlertCount` - panics because `StormAggregation` is nil

## 🔍 **What We've Ruled Out**

1. ✅ JSON marshaling - Field is present in JSON
2. ✅ Go struct tags - `json:"stormAggregation,omitempty"` is correct
3. ✅ CRD schema - Field exists with correct structure
4. ✅ APIVersion/Kind - Now set correctly
5. ✅ Scheme registration - `remediationv1alpha1.AddToScheme(scheme)` called
6. ✅ Fresh cluster - Deleted and recreated Kind cluster
7. ✅ Required fields - All required fields are present in JSON

## 🤔 **Hypothesis: CRD Schema Issue**

The K8s API server warning persists:
```
unknown field "spec.stormAggregation"
```

This suggests the API server doesn't recognize the field, even though:
- The CRD definition includes it
- The JSON payload includes it
- All validation passes

## 🎯 **Next Steps**

### Option 1: Check CRD Installation
Maybe the CRD wasn't actually updated in K8s?

```bash
KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o yaml > /tmp/installed-crd.yaml
diff /tmp/installed-crd.yaml config/crd/remediation.kubernaut.io_remediationrequests.yaml
```

### Option 2: Check controller-runtime Client
Maybe there's a bug in how controller-runtime handles optional pointer fields?

### Option 3: Regenerate CRD from Go Types
Maybe the CRD file is out of sync with the Go types?

```bash
# Fix the controller-gen error first
make manifests
```

### Option 4: Try Without Pointer
Change the Go struct to use a value instead of pointer:

```go
// Instead of:
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`

// Try:
StormAggregation StormAggregation `json:"stormAggregation,omitempty"`
```

## 📊 **Impact**

- **Test Status**: Failing (1 test)
- **Root Cause**: K8s dropping `stormAggregation` field
- **Workaround**: None identified yet
- **Severity**: **CRITICAL** - Blocks storm aggregation feature

## 🚀 **Recommendation**

Try **Option 3** (regenerate CRD) first, as this is most likely to reveal any schema mismatch.

# 🚨 CRITICAL FINDING: K8s Dropping stormAggregation Field

## ✅ **Confirmed: JSON Payload is Correct**

The JSON being sent to K8s **DOES** include `stormAggregation`:

```json
{
  "kind": "RemediationRequest",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",
  "spec": {
    "stormAggregation": {
      "pattern": "HighMemoryUsage in prod-payments",
      "alertCount": 1,
      "affectedResources": [
        {
          "kind": "Pod",
          "name": "payment-api-1",
          "namespace": "prod-payments"
        }
      ],
      "aggregationWindow": "5m",
      "firstSeen": "2025-10-27T13:52:00Z",
      "lastSeen": "2025-10-27T13:52:00Z"
    }
  }
}
```

## ❌ **Problem: K8s Drops the Field**

When reading the CRD back from K8s, `StormAggregation` is **nil**, causing a panic:

```
panic: runtime error: nil pointer dereference
-> github.com/jordigilh/kubernaut/pkg/gateway/server.(*Server).respondAggregatedAlert
->   /Users/jgil/go/src/github.com/jordigilh/kubernaut/pkg/gateway/server/handlers.go:433
```

Line 433: `stormCRD.Spec.StormAggregation.AlertCount` - panics because `StormAggregation` is nil

## 🔍 **What We've Ruled Out**

1. ✅ JSON marshaling - Field is present in JSON
2. ✅ Go struct tags - `json:"stormAggregation,omitempty"` is correct
3. ✅ CRD schema - Field exists with correct structure
4. ✅ APIVersion/Kind - Now set correctly
5. ✅ Scheme registration - `remediationv1alpha1.AddToScheme(scheme)` called
6. ✅ Fresh cluster - Deleted and recreated Kind cluster
7. ✅ Required fields - All required fields are present in JSON

## 🤔 **Hypothesis: CRD Schema Issue**

The K8s API server warning persists:
```
unknown field "spec.stormAggregation"
```

This suggests the API server doesn't recognize the field, even though:
- The CRD definition includes it
- The JSON payload includes it
- All validation passes

## 🎯 **Next Steps**

### Option 1: Check CRD Installation
Maybe the CRD wasn't actually updated in K8s?

```bash
KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o yaml > /tmp/installed-crd.yaml
diff /tmp/installed-crd.yaml config/crd/remediation.kubernaut.io_remediationrequests.yaml
```

### Option 2: Check controller-runtime Client
Maybe there's a bug in how controller-runtime handles optional pointer fields?

### Option 3: Regenerate CRD from Go Types
Maybe the CRD file is out of sync with the Go types?

```bash
# Fix the controller-gen error first
make manifests
```

### Option 4: Try Without Pointer
Change the Go struct to use a value instead of pointer:

```go
// Instead of:
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`

// Try:
StormAggregation StormAggregation `json:"stormAggregation,omitempty"`
```

## 📊 **Impact**

- **Test Status**: Failing (1 test)
- **Root Cause**: K8s dropping `stormAggregation` field
- **Workaround**: None identified yet
- **Severity**: **CRITICAL** - Blocks storm aggregation feature

## 🚀 **Recommendation**

Try **Option 3** (regenerate CRD) first, as this is most likely to reveal any schema mismatch.




