> **Historical Note (v1.2):** This document contains references to storm detection / aggregation
> which was removed in v1.2 per DD-GATEWAY-015. Storm-related content is retained for historical
> context only and does not reflect current architecture.

# Storm CRD StormAggregation Field Mystery

## 🔍 **Problem**

K8s API server warning: `unknown field "spec.stormAggregation"`

All storm CRDs show `hasStormAggregation=false` even though:
- ✅ CRD schema includes `stormAggregation` field
- ✅ Go struct has `StormAggregation *StormAggregation json:"stormAggregation,omitempty"`
- ✅ CRD object has `StormAggregation != nil` before sending to K8s
- ✅ `APIVersion` and `Kind` are now set correctly
- ✅ Scheme is registered with `remediationv1alpha1.AddToScheme(scheme)`

## ✅ **What We've Verified**

### 1. CRD Schema is Correct
```bash
$ KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | grep -o "stormAggregation"
stormAggregation  # ✅ Field exists in schema
```

### 2. CRD File Has Correct Structure
```yaml
spec:
  properties:
    stormAggregation:
      description: |-
        Storm Aggregation (BR-GATEWAY-016)
        Populated only for storm-aggregated CRDs
        nil for individual alert CRDs
      properties:
        affectedResources:
          items:
            properties:
              kind:
                type: string
              name:
                type: string
              namespace:
                type: string
            required:
            - kind
            - name
            type: object
          type: array
        # ... more fields
      type: object  # ✅ Structural schema
```

### 3. Go Struct Has Correct JSON Tag
```go
// api/remediation/v1alpha1/remediationrequest_types.go:89
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`
```

### 4. CRD Object Has Field Set Before K8s Create
```json
{
  "level": "info",
  "msg": "Attempting to create storm CRD",
  "name": "storm-highmemoryusage-in-prod-payments-87dd33ff1973",
  "namespace": "prod-payments",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",  // ✅ Set
  "kind": "RemediationRequest",  // ✅ Set
  "has_storm_aggregation": true,  // ✅ Not nil
  "alert_count": 1  // ✅ Has data
}
```

### 5. Scheme is Registered
```go
// test/integration/gateway/helpers.go:153
scheme := k8sruntime.NewScheme()
_ = remediationv1alpha1.AddToScheme(scheme)  // ✅ Registered
```

### 6. Fresh Kind Cluster
- Deleted old cluster
- Created new cluster
- CRD installed fresh
- No cached schema

## ❌ **What's Still Failing**

1. **K8s API Warning**: `unknown field "spec.stormAggregation"`
2. **Field Dropped**: All CRDs in K8s show `hasStormAggregation=false`
3. **Test Fails**: Cannot find storm CRD with `StormAggregation != nil`

## 🤔 **Hypotheses**

### Hypothesis 1: Controller-Runtime Client Issue
Maybe `controller-runtime` v0.19.2 has a bug with optional pointer fields?

### Hypothesis 2: CRD Schema Validation Issue
Maybe the CRD schema has a subtle structural issue that's not obvious?

### Hypothesis 3: Go Struct Definition Issue
Maybe there's something wrong with how `StormAggregation` type is defined?

### Hypothesis 4: Omitempty Behavior
Maybe `omitempty` is causing the field to be dropped even when not nil?

## 🔬 **Next Steps to Try**

### Option A: Remove `omitempty` from JSON Tag
```go
// Try without omitempty
StormAggregation *StormAggregation `json:"stormAggregation"`
```

### Option B: Check StormAggregation Type Definition
```bash
# Verify the StormAggregation type is properly defined
grep -A30 "type StormAggregation struct" api/remediation/v1alpha1/remediationrequest_types.go
```

### Option C: Test with Simple Field
```go
// Add a simple non-pointer field to test
StormTest string `json:"stormTest"`
```

### Option D: Check controller-runtime Version
```bash
# Maybe upgrade controller-runtime?
go get sigs.k8s.io/controller-runtime@latest
```

### Option E: Marshal to JSON and Inspect
```go
// Before sending to K8s, marshal to JSON and log it
jsonBytes, _ := json.Marshal(crd)
c.logger.Info("CRD JSON", zap.String("json", string(jsonBytes)))
```

## 📊 **Test Results**

- **Test**: Single failing test with clean Redis
- **Result**: Fails even in isolation
- **Conclusion**: NOT a test infrastructure problem
- **Root Cause**: K8s is dropping the `stormAggregation` field

## 🎯 **Recommendation**

Try **Option E** first to see the actual JSON being sent to K8s, then **Option B** to verify the type definition.



## 🔍 **Problem**

K8s API server warning: `unknown field "spec.stormAggregation"`

All storm CRDs show `hasStormAggregation=false` even though:
- ✅ CRD schema includes `stormAggregation` field
- ✅ Go struct has `StormAggregation *StormAggregation json:"stormAggregation,omitempty"`
- ✅ CRD object has `StormAggregation != nil` before sending to K8s
- ✅ `APIVersion` and `Kind` are now set correctly
- ✅ Scheme is registered with `remediationv1alpha1.AddToScheme(scheme)`

## ✅ **What We've Verified**

### 1. CRD Schema is Correct
```bash
$ KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | grep -o "stormAggregation"
stormAggregation  # ✅ Field exists in schema
```

### 2. CRD File Has Correct Structure
```yaml
spec:
  properties:
    stormAggregation:
      description: |-
        Storm Aggregation (BR-GATEWAY-016)
        Populated only for storm-aggregated CRDs
        nil for individual alert CRDs
      properties:
        affectedResources:
          items:
            properties:
              kind:
                type: string
              name:
                type: string
              namespace:
                type: string
            required:
            - kind
            - name
            type: object
          type: array
        # ... more fields
      type: object  # ✅ Structural schema
```

### 3. Go Struct Has Correct JSON Tag
```go
// api/remediation/v1alpha1/remediationrequest_types.go:89
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`
```

### 4. CRD Object Has Field Set Before K8s Create
```json
{
  "level": "info",
  "msg": "Attempting to create storm CRD",
  "name": "storm-highmemoryusage-in-prod-payments-87dd33ff1973",
  "namespace": "prod-payments",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",  // ✅ Set
  "kind": "RemediationRequest",  // ✅ Set
  "has_storm_aggregation": true,  // ✅ Not nil
  "alert_count": 1  // ✅ Has data
}
```

### 5. Scheme is Registered
```go
// test/integration/gateway/helpers.go:153
scheme := k8sruntime.NewScheme()
_ = remediationv1alpha1.AddToScheme(scheme)  // ✅ Registered
```

### 6. Fresh Kind Cluster
- Deleted old cluster
- Created new cluster
- CRD installed fresh
- No cached schema

## ❌ **What's Still Failing**

1. **K8s API Warning**: `unknown field "spec.stormAggregation"`
2. **Field Dropped**: All CRDs in K8s show `hasStormAggregation=false`
3. **Test Fails**: Cannot find storm CRD with `StormAggregation != nil`

## 🤔 **Hypotheses**

### Hypothesis 1: Controller-Runtime Client Issue
Maybe `controller-runtime` v0.19.2 has a bug with optional pointer fields?

### Hypothesis 2: CRD Schema Validation Issue
Maybe the CRD schema has a subtle structural issue that's not obvious?

### Hypothesis 3: Go Struct Definition Issue
Maybe there's something wrong with how `StormAggregation` type is defined?

### Hypothesis 4: Omitempty Behavior
Maybe `omitempty` is causing the field to be dropped even when not nil?

## 🔬 **Next Steps to Try**

### Option A: Remove `omitempty` from JSON Tag
```go
// Try without omitempty
StormAggregation *StormAggregation `json:"stormAggregation"`
```

### Option B: Check StormAggregation Type Definition
```bash
# Verify the StormAggregation type is properly defined
grep -A30 "type StormAggregation struct" api/remediation/v1alpha1/remediationrequest_types.go
```

### Option C: Test with Simple Field
```go
// Add a simple non-pointer field to test
StormTest string `json:"stormTest"`
```

### Option D: Check controller-runtime Version
```bash
# Maybe upgrade controller-runtime?
go get sigs.k8s.io/controller-runtime@latest
```

### Option E: Marshal to JSON and Inspect
```go
// Before sending to K8s, marshal to JSON and log it
jsonBytes, _ := json.Marshal(crd)
c.logger.Info("CRD JSON", zap.String("json", string(jsonBytes)))
```

## 📊 **Test Results**

- **Test**: Single failing test with clean Redis
- **Result**: Fails even in isolation
- **Conclusion**: NOT a test infrastructure problem
- **Root Cause**: K8s is dropping the `stormAggregation` field

## 🎯 **Recommendation**

Try **Option E** first to see the actual JSON being sent to K8s, then **Option B** to verify the type definition.

# Storm CRD StormAggregation Field Mystery

## 🔍 **Problem**

K8s API server warning: `unknown field "spec.stormAggregation"`

All storm CRDs show `hasStormAggregation=false` even though:
- ✅ CRD schema includes `stormAggregation` field
- ✅ Go struct has `StormAggregation *StormAggregation json:"stormAggregation,omitempty"`
- ✅ CRD object has `StormAggregation != nil` before sending to K8s
- ✅ `APIVersion` and `Kind` are now set correctly
- ✅ Scheme is registered with `remediationv1alpha1.AddToScheme(scheme)`

## ✅ **What We've Verified**

### 1. CRD Schema is Correct
```bash
$ KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | grep -o "stormAggregation"
stormAggregation  # ✅ Field exists in schema
```

### 2. CRD File Has Correct Structure
```yaml
spec:
  properties:
    stormAggregation:
      description: |-
        Storm Aggregation (BR-GATEWAY-016)
        Populated only for storm-aggregated CRDs
        nil for individual alert CRDs
      properties:
        affectedResources:
          items:
            properties:
              kind:
                type: string
              name:
                type: string
              namespace:
                type: string
            required:
            - kind
            - name
            type: object
          type: array
        # ... more fields
      type: object  # ✅ Structural schema
```

### 3. Go Struct Has Correct JSON Tag
```go
// api/remediation/v1alpha1/remediationrequest_types.go:89
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`
```

### 4. CRD Object Has Field Set Before K8s Create
```json
{
  "level": "info",
  "msg": "Attempting to create storm CRD",
  "name": "storm-highmemoryusage-in-prod-payments-87dd33ff1973",
  "namespace": "prod-payments",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",  // ✅ Set
  "kind": "RemediationRequest",  // ✅ Set
  "has_storm_aggregation": true,  // ✅ Not nil
  "alert_count": 1  // ✅ Has data
}
```

### 5. Scheme is Registered
```go
// test/integration/gateway/helpers.go:153
scheme := k8sruntime.NewScheme()
_ = remediationv1alpha1.AddToScheme(scheme)  // ✅ Registered
```

### 6. Fresh Kind Cluster
- Deleted old cluster
- Created new cluster
- CRD installed fresh
- No cached schema

## ❌ **What's Still Failing**

1. **K8s API Warning**: `unknown field "spec.stormAggregation"`
2. **Field Dropped**: All CRDs in K8s show `hasStormAggregation=false`
3. **Test Fails**: Cannot find storm CRD with `StormAggregation != nil`

## 🤔 **Hypotheses**

### Hypothesis 1: Controller-Runtime Client Issue
Maybe `controller-runtime` v0.19.2 has a bug with optional pointer fields?

### Hypothesis 2: CRD Schema Validation Issue
Maybe the CRD schema has a subtle structural issue that's not obvious?

### Hypothesis 3: Go Struct Definition Issue
Maybe there's something wrong with how `StormAggregation` type is defined?

### Hypothesis 4: Omitempty Behavior
Maybe `omitempty` is causing the field to be dropped even when not nil?

## 🔬 **Next Steps to Try**

### Option A: Remove `omitempty` from JSON Tag
```go
// Try without omitempty
StormAggregation *StormAggregation `json:"stormAggregation"`
```

### Option B: Check StormAggregation Type Definition
```bash
# Verify the StormAggregation type is properly defined
grep -A30 "type StormAggregation struct" api/remediation/v1alpha1/remediationrequest_types.go
```

### Option C: Test with Simple Field
```go
// Add a simple non-pointer field to test
StormTest string `json:"stormTest"`
```

### Option D: Check controller-runtime Version
```bash
# Maybe upgrade controller-runtime?
go get sigs.k8s.io/controller-runtime@latest
```

### Option E: Marshal to JSON and Inspect
```go
// Before sending to K8s, marshal to JSON and log it
jsonBytes, _ := json.Marshal(crd)
c.logger.Info("CRD JSON", zap.String("json", string(jsonBytes)))
```

## 📊 **Test Results**

- **Test**: Single failing test with clean Redis
- **Result**: Fails even in isolation
- **Conclusion**: NOT a test infrastructure problem
- **Root Cause**: K8s is dropping the `stormAggregation` field

## 🎯 **Recommendation**

Try **Option E** first to see the actual JSON being sent to K8s, then **Option B** to verify the type definition.

# Storm CRD StormAggregation Field Mystery

## 🔍 **Problem**

K8s API server warning: `unknown field "spec.stormAggregation"`

All storm CRDs show `hasStormAggregation=false` even though:
- ✅ CRD schema includes `stormAggregation` field
- ✅ Go struct has `StormAggregation *StormAggregation json:"stormAggregation,omitempty"`
- ✅ CRD object has `StormAggregation != nil` before sending to K8s
- ✅ `APIVersion` and `Kind` are now set correctly
- ✅ Scheme is registered with `remediationv1alpha1.AddToScheme(scheme)`

## ✅ **What We've Verified**

### 1. CRD Schema is Correct
```bash
$ KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | grep -o "stormAggregation"
stormAggregation  # ✅ Field exists in schema
```

### 2. CRD File Has Correct Structure
```yaml
spec:
  properties:
    stormAggregation:
      description: |-
        Storm Aggregation (BR-GATEWAY-016)
        Populated only for storm-aggregated CRDs
        nil for individual alert CRDs
      properties:
        affectedResources:
          items:
            properties:
              kind:
                type: string
              name:
                type: string
              namespace:
                type: string
            required:
            - kind
            - name
            type: object
          type: array
        # ... more fields
      type: object  # ✅ Structural schema
```

### 3. Go Struct Has Correct JSON Tag
```go
// api/remediation/v1alpha1/remediationrequest_types.go:89
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`
```

### 4. CRD Object Has Field Set Before K8s Create
```json
{
  "level": "info",
  "msg": "Attempting to create storm CRD",
  "name": "storm-highmemoryusage-in-prod-payments-87dd33ff1973",
  "namespace": "prod-payments",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",  // ✅ Set
  "kind": "RemediationRequest",  // ✅ Set
  "has_storm_aggregation": true,  // ✅ Not nil
  "alert_count": 1  // ✅ Has data
}
```

### 5. Scheme is Registered
```go
// test/integration/gateway/helpers.go:153
scheme := k8sruntime.NewScheme()
_ = remediationv1alpha1.AddToScheme(scheme)  // ✅ Registered
```

### 6. Fresh Kind Cluster
- Deleted old cluster
- Created new cluster
- CRD installed fresh
- No cached schema

## ❌ **What's Still Failing**

1. **K8s API Warning**: `unknown field "spec.stormAggregation"`
2. **Field Dropped**: All CRDs in K8s show `hasStormAggregation=false`
3. **Test Fails**: Cannot find storm CRD with `StormAggregation != nil`

## 🤔 **Hypotheses**

### Hypothesis 1: Controller-Runtime Client Issue
Maybe `controller-runtime` v0.19.2 has a bug with optional pointer fields?

### Hypothesis 2: CRD Schema Validation Issue
Maybe the CRD schema has a subtle structural issue that's not obvious?

### Hypothesis 3: Go Struct Definition Issue
Maybe there's something wrong with how `StormAggregation` type is defined?

### Hypothesis 4: Omitempty Behavior
Maybe `omitempty` is causing the field to be dropped even when not nil?

## 🔬 **Next Steps to Try**

### Option A: Remove `omitempty` from JSON Tag
```go
// Try without omitempty
StormAggregation *StormAggregation `json:"stormAggregation"`
```

### Option B: Check StormAggregation Type Definition
```bash
# Verify the StormAggregation type is properly defined
grep -A30 "type StormAggregation struct" api/remediation/v1alpha1/remediationrequest_types.go
```

### Option C: Test with Simple Field
```go
// Add a simple non-pointer field to test
StormTest string `json:"stormTest"`
```

### Option D: Check controller-runtime Version
```bash
# Maybe upgrade controller-runtime?
go get sigs.k8s.io/controller-runtime@latest
```

### Option E: Marshal to JSON and Inspect
```go
// Before sending to K8s, marshal to JSON and log it
jsonBytes, _ := json.Marshal(crd)
c.logger.Info("CRD JSON", zap.String("json", string(jsonBytes)))
```

## 📊 **Test Results**

- **Test**: Single failing test with clean Redis
- **Result**: Fails even in isolation
- **Conclusion**: NOT a test infrastructure problem
- **Root Cause**: K8s is dropping the `stormAggregation` field

## 🎯 **Recommendation**

Try **Option E** first to see the actual JSON being sent to K8s, then **Option B** to verify the type definition.



## 🔍 **Problem**

K8s API server warning: `unknown field "spec.stormAggregation"`

All storm CRDs show `hasStormAggregation=false` even though:
- ✅ CRD schema includes `stormAggregation` field
- ✅ Go struct has `StormAggregation *StormAggregation json:"stormAggregation,omitempty"`
- ✅ CRD object has `StormAggregation != nil` before sending to K8s
- ✅ `APIVersion` and `Kind` are now set correctly
- ✅ Scheme is registered with `remediationv1alpha1.AddToScheme(scheme)`

## ✅ **What We've Verified**

### 1. CRD Schema is Correct
```bash
$ KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | grep -o "stormAggregation"
stormAggregation  # ✅ Field exists in schema
```

### 2. CRD File Has Correct Structure
```yaml
spec:
  properties:
    stormAggregation:
      description: |-
        Storm Aggregation (BR-GATEWAY-016)
        Populated only for storm-aggregated CRDs
        nil for individual alert CRDs
      properties:
        affectedResources:
          items:
            properties:
              kind:
                type: string
              name:
                type: string
              namespace:
                type: string
            required:
            - kind
            - name
            type: object
          type: array
        # ... more fields
      type: object  # ✅ Structural schema
```

### 3. Go Struct Has Correct JSON Tag
```go
// api/remediation/v1alpha1/remediationrequest_types.go:89
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`
```

### 4. CRD Object Has Field Set Before K8s Create
```json
{
  "level": "info",
  "msg": "Attempting to create storm CRD",
  "name": "storm-highmemoryusage-in-prod-payments-87dd33ff1973",
  "namespace": "prod-payments",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",  // ✅ Set
  "kind": "RemediationRequest",  // ✅ Set
  "has_storm_aggregation": true,  // ✅ Not nil
  "alert_count": 1  // ✅ Has data
}
```

### 5. Scheme is Registered
```go
// test/integration/gateway/helpers.go:153
scheme := k8sruntime.NewScheme()
_ = remediationv1alpha1.AddToScheme(scheme)  // ✅ Registered
```

### 6. Fresh Kind Cluster
- Deleted old cluster
- Created new cluster
- CRD installed fresh
- No cached schema

## ❌ **What's Still Failing**

1. **K8s API Warning**: `unknown field "spec.stormAggregation"`
2. **Field Dropped**: All CRDs in K8s show `hasStormAggregation=false`
3. **Test Fails**: Cannot find storm CRD with `StormAggregation != nil`

## 🤔 **Hypotheses**

### Hypothesis 1: Controller-Runtime Client Issue
Maybe `controller-runtime` v0.19.2 has a bug with optional pointer fields?

### Hypothesis 2: CRD Schema Validation Issue
Maybe the CRD schema has a subtle structural issue that's not obvious?

### Hypothesis 3: Go Struct Definition Issue
Maybe there's something wrong with how `StormAggregation` type is defined?

### Hypothesis 4: Omitempty Behavior
Maybe `omitempty` is causing the field to be dropped even when not nil?

## 🔬 **Next Steps to Try**

### Option A: Remove `omitempty` from JSON Tag
```go
// Try without omitempty
StormAggregation *StormAggregation `json:"stormAggregation"`
```

### Option B: Check StormAggregation Type Definition
```bash
# Verify the StormAggregation type is properly defined
grep -A30 "type StormAggregation struct" api/remediation/v1alpha1/remediationrequest_types.go
```

### Option C: Test with Simple Field
```go
// Add a simple non-pointer field to test
StormTest string `json:"stormTest"`
```

### Option D: Check controller-runtime Version
```bash
# Maybe upgrade controller-runtime?
go get sigs.k8s.io/controller-runtime@latest
```

### Option E: Marshal to JSON and Inspect
```go
// Before sending to K8s, marshal to JSON and log it
jsonBytes, _ := json.Marshal(crd)
c.logger.Info("CRD JSON", zap.String("json", string(jsonBytes)))
```

## 📊 **Test Results**

- **Test**: Single failing test with clean Redis
- **Result**: Fails even in isolation
- **Conclusion**: NOT a test infrastructure problem
- **Root Cause**: K8s is dropping the `stormAggregation` field

## 🎯 **Recommendation**

Try **Option E** first to see the actual JSON being sent to K8s, then **Option B** to verify the type definition.

# Storm CRD StormAggregation Field Mystery

## 🔍 **Problem**

K8s API server warning: `unknown field "spec.stormAggregation"`

All storm CRDs show `hasStormAggregation=false` even though:
- ✅ CRD schema includes `stormAggregation` field
- ✅ Go struct has `StormAggregation *StormAggregation json:"stormAggregation,omitempty"`
- ✅ CRD object has `StormAggregation != nil` before sending to K8s
- ✅ `APIVersion` and `Kind` are now set correctly
- ✅ Scheme is registered with `remediationv1alpha1.AddToScheme(scheme)`

## ✅ **What We've Verified**

### 1. CRD Schema is Correct
```bash
$ KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | grep -o "stormAggregation"
stormAggregation  # ✅ Field exists in schema
```

### 2. CRD File Has Correct Structure
```yaml
spec:
  properties:
    stormAggregation:
      description: |-
        Storm Aggregation (BR-GATEWAY-016)
        Populated only for storm-aggregated CRDs
        nil for individual alert CRDs
      properties:
        affectedResources:
          items:
            properties:
              kind:
                type: string
              name:
                type: string
              namespace:
                type: string
            required:
            - kind
            - name
            type: object
          type: array
        # ... more fields
      type: object  # ✅ Structural schema
```

### 3. Go Struct Has Correct JSON Tag
```go
// api/remediation/v1alpha1/remediationrequest_types.go:89
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`
```

### 4. CRD Object Has Field Set Before K8s Create
```json
{
  "level": "info",
  "msg": "Attempting to create storm CRD",
  "name": "storm-highmemoryusage-in-prod-payments-87dd33ff1973",
  "namespace": "prod-payments",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",  // ✅ Set
  "kind": "RemediationRequest",  // ✅ Set
  "has_storm_aggregation": true,  // ✅ Not nil
  "alert_count": 1  // ✅ Has data
}
```

### 5. Scheme is Registered
```go
// test/integration/gateway/helpers.go:153
scheme := k8sruntime.NewScheme()
_ = remediationv1alpha1.AddToScheme(scheme)  // ✅ Registered
```

### 6. Fresh Kind Cluster
- Deleted old cluster
- Created new cluster
- CRD installed fresh
- No cached schema

## ❌ **What's Still Failing**

1. **K8s API Warning**: `unknown field "spec.stormAggregation"`
2. **Field Dropped**: All CRDs in K8s show `hasStormAggregation=false`
3. **Test Fails**: Cannot find storm CRD with `StormAggregation != nil`

## 🤔 **Hypotheses**

### Hypothesis 1: Controller-Runtime Client Issue
Maybe `controller-runtime` v0.19.2 has a bug with optional pointer fields?

### Hypothesis 2: CRD Schema Validation Issue
Maybe the CRD schema has a subtle structural issue that's not obvious?

### Hypothesis 3: Go Struct Definition Issue
Maybe there's something wrong with how `StormAggregation` type is defined?

### Hypothesis 4: Omitempty Behavior
Maybe `omitempty` is causing the field to be dropped even when not nil?

## 🔬 **Next Steps to Try**

### Option A: Remove `omitempty` from JSON Tag
```go
// Try without omitempty
StormAggregation *StormAggregation `json:"stormAggregation"`
```

### Option B: Check StormAggregation Type Definition
```bash
# Verify the StormAggregation type is properly defined
grep -A30 "type StormAggregation struct" api/remediation/v1alpha1/remediationrequest_types.go
```

### Option C: Test with Simple Field
```go
// Add a simple non-pointer field to test
StormTest string `json:"stormTest"`
```

### Option D: Check controller-runtime Version
```bash
# Maybe upgrade controller-runtime?
go get sigs.k8s.io/controller-runtime@latest
```

### Option E: Marshal to JSON and Inspect
```go
// Before sending to K8s, marshal to JSON and log it
jsonBytes, _ := json.Marshal(crd)
c.logger.Info("CRD JSON", zap.String("json", string(jsonBytes)))
```

## 📊 **Test Results**

- **Test**: Single failing test with clean Redis
- **Result**: Fails even in isolation
- **Conclusion**: NOT a test infrastructure problem
- **Root Cause**: K8s is dropping the `stormAggregation` field

## 🎯 **Recommendation**

Try **Option E** first to see the actual JSON being sent to K8s, then **Option B** to verify the type definition.

# Storm CRD StormAggregation Field Mystery

## 🔍 **Problem**

K8s API server warning: `unknown field "spec.stormAggregation"`

All storm CRDs show `hasStormAggregation=false` even though:
- ✅ CRD schema includes `stormAggregation` field
- ✅ Go struct has `StormAggregation *StormAggregation json:"stormAggregation,omitempty"`
- ✅ CRD object has `StormAggregation != nil` before sending to K8s
- ✅ `APIVersion` and `Kind` are now set correctly
- ✅ Scheme is registered with `remediationv1alpha1.AddToScheme(scheme)`

## ✅ **What We've Verified**

### 1. CRD Schema is Correct
```bash
$ KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | grep -o "stormAggregation"
stormAggregation  # ✅ Field exists in schema
```

### 2. CRD File Has Correct Structure
```yaml
spec:
  properties:
    stormAggregation:
      description: |-
        Storm Aggregation (BR-GATEWAY-016)
        Populated only for storm-aggregated CRDs
        nil for individual alert CRDs
      properties:
        affectedResources:
          items:
            properties:
              kind:
                type: string
              name:
                type: string
              namespace:
                type: string
            required:
            - kind
            - name
            type: object
          type: array
        # ... more fields
      type: object  # ✅ Structural schema
```

### 3. Go Struct Has Correct JSON Tag
```go
// api/remediation/v1alpha1/remediationrequest_types.go:89
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`
```

### 4. CRD Object Has Field Set Before K8s Create
```json
{
  "level": "info",
  "msg": "Attempting to create storm CRD",
  "name": "storm-highmemoryusage-in-prod-payments-87dd33ff1973",
  "namespace": "prod-payments",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",  // ✅ Set
  "kind": "RemediationRequest",  // ✅ Set
  "has_storm_aggregation": true,  // ✅ Not nil
  "alert_count": 1  // ✅ Has data
}
```

### 5. Scheme is Registered
```go
// test/integration/gateway/helpers.go:153
scheme := k8sruntime.NewScheme()
_ = remediationv1alpha1.AddToScheme(scheme)  // ✅ Registered
```

### 6. Fresh Kind Cluster
- Deleted old cluster
- Created new cluster
- CRD installed fresh
- No cached schema

## ❌ **What's Still Failing**

1. **K8s API Warning**: `unknown field "spec.stormAggregation"`
2. **Field Dropped**: All CRDs in K8s show `hasStormAggregation=false`
3. **Test Fails**: Cannot find storm CRD with `StormAggregation != nil`

## 🤔 **Hypotheses**

### Hypothesis 1: Controller-Runtime Client Issue
Maybe `controller-runtime` v0.19.2 has a bug with optional pointer fields?

### Hypothesis 2: CRD Schema Validation Issue
Maybe the CRD schema has a subtle structural issue that's not obvious?

### Hypothesis 3: Go Struct Definition Issue
Maybe there's something wrong with how `StormAggregation` type is defined?

### Hypothesis 4: Omitempty Behavior
Maybe `omitempty` is causing the field to be dropped even when not nil?

## 🔬 **Next Steps to Try**

### Option A: Remove `omitempty` from JSON Tag
```go
// Try without omitempty
StormAggregation *StormAggregation `json:"stormAggregation"`
```

### Option B: Check StormAggregation Type Definition
```bash
# Verify the StormAggregation type is properly defined
grep -A30 "type StormAggregation struct" api/remediation/v1alpha1/remediationrequest_types.go
```

### Option C: Test with Simple Field
```go
// Add a simple non-pointer field to test
StormTest string `json:"stormTest"`
```

### Option D: Check controller-runtime Version
```bash
# Maybe upgrade controller-runtime?
go get sigs.k8s.io/controller-runtime@latest
```

### Option E: Marshal to JSON and Inspect
```go
// Before sending to K8s, marshal to JSON and log it
jsonBytes, _ := json.Marshal(crd)
c.logger.Info("CRD JSON", zap.String("json", string(jsonBytes)))
```

## 📊 **Test Results**

- **Test**: Single failing test with clean Redis
- **Result**: Fails even in isolation
- **Conclusion**: NOT a test infrastructure problem
- **Root Cause**: K8s is dropping the `stormAggregation` field

## 🎯 **Recommendation**

Try **Option E** first to see the actual JSON being sent to K8s, then **Option B** to verify the type definition.



## 🔍 **Problem**

K8s API server warning: `unknown field "spec.stormAggregation"`

All storm CRDs show `hasStormAggregation=false` even though:
- ✅ CRD schema includes `stormAggregation` field
- ✅ Go struct has `StormAggregation *StormAggregation json:"stormAggregation,omitempty"`
- ✅ CRD object has `StormAggregation != nil` before sending to K8s
- ✅ `APIVersion` and `Kind` are now set correctly
- ✅ Scheme is registered with `remediationv1alpha1.AddToScheme(scheme)`

## ✅ **What We've Verified**

### 1. CRD Schema is Correct
```bash
$ KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | grep -o "stormAggregation"
stormAggregation  # ✅ Field exists in schema
```

### 2. CRD File Has Correct Structure
```yaml
spec:
  properties:
    stormAggregation:
      description: |-
        Storm Aggregation (BR-GATEWAY-016)
        Populated only for storm-aggregated CRDs
        nil for individual alert CRDs
      properties:
        affectedResources:
          items:
            properties:
              kind:
                type: string
              name:
                type: string
              namespace:
                type: string
            required:
            - kind
            - name
            type: object
          type: array
        # ... more fields
      type: object  # ✅ Structural schema
```

### 3. Go Struct Has Correct JSON Tag
```go
// api/remediation/v1alpha1/remediationrequest_types.go:89
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`
```

### 4. CRD Object Has Field Set Before K8s Create
```json
{
  "level": "info",
  "msg": "Attempting to create storm CRD",
  "name": "storm-highmemoryusage-in-prod-payments-87dd33ff1973",
  "namespace": "prod-payments",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",  // ✅ Set
  "kind": "RemediationRequest",  // ✅ Set
  "has_storm_aggregation": true,  // ✅ Not nil
  "alert_count": 1  // ✅ Has data
}
```

### 5. Scheme is Registered
```go
// test/integration/gateway/helpers.go:153
scheme := k8sruntime.NewScheme()
_ = remediationv1alpha1.AddToScheme(scheme)  // ✅ Registered
```

### 6. Fresh Kind Cluster
- Deleted old cluster
- Created new cluster
- CRD installed fresh
- No cached schema

## ❌ **What's Still Failing**

1. **K8s API Warning**: `unknown field "spec.stormAggregation"`
2. **Field Dropped**: All CRDs in K8s show `hasStormAggregation=false`
3. **Test Fails**: Cannot find storm CRD with `StormAggregation != nil`

## 🤔 **Hypotheses**

### Hypothesis 1: Controller-Runtime Client Issue
Maybe `controller-runtime` v0.19.2 has a bug with optional pointer fields?

### Hypothesis 2: CRD Schema Validation Issue
Maybe the CRD schema has a subtle structural issue that's not obvious?

### Hypothesis 3: Go Struct Definition Issue
Maybe there's something wrong with how `StormAggregation` type is defined?

### Hypothesis 4: Omitempty Behavior
Maybe `omitempty` is causing the field to be dropped even when not nil?

## 🔬 **Next Steps to Try**

### Option A: Remove `omitempty` from JSON Tag
```go
// Try without omitempty
StormAggregation *StormAggregation `json:"stormAggregation"`
```

### Option B: Check StormAggregation Type Definition
```bash
# Verify the StormAggregation type is properly defined
grep -A30 "type StormAggregation struct" api/remediation/v1alpha1/remediationrequest_types.go
```

### Option C: Test with Simple Field
```go
// Add a simple non-pointer field to test
StormTest string `json:"stormTest"`
```

### Option D: Check controller-runtime Version
```bash
# Maybe upgrade controller-runtime?
go get sigs.k8s.io/controller-runtime@latest
```

### Option E: Marshal to JSON and Inspect
```go
// Before sending to K8s, marshal to JSON and log it
jsonBytes, _ := json.Marshal(crd)
c.logger.Info("CRD JSON", zap.String("json", string(jsonBytes)))
```

## 📊 **Test Results**

- **Test**: Single failing test with clean Redis
- **Result**: Fails even in isolation
- **Conclusion**: NOT a test infrastructure problem
- **Root Cause**: K8s is dropping the `stormAggregation` field

## 🎯 **Recommendation**

Try **Option E** first to see the actual JSON being sent to K8s, then **Option B** to verify the type definition.

# Storm CRD StormAggregation Field Mystery

## 🔍 **Problem**

K8s API server warning: `unknown field "spec.stormAggregation"`

All storm CRDs show `hasStormAggregation=false` even though:
- ✅ CRD schema includes `stormAggregation` field
- ✅ Go struct has `StormAggregation *StormAggregation json:"stormAggregation,omitempty"`
- ✅ CRD object has `StormAggregation != nil` before sending to K8s
- ✅ `APIVersion` and `Kind` are now set correctly
- ✅ Scheme is registered with `remediationv1alpha1.AddToScheme(scheme)`

## ✅ **What We've Verified**

### 1. CRD Schema is Correct
```bash
$ KUBECONFIG="${HOME}/.kube/kind-config" kubectl get crd remediationrequests.remediation.kubernaut.io -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | grep -o "stormAggregation"
stormAggregation  # ✅ Field exists in schema
```

### 2. CRD File Has Correct Structure
```yaml
spec:
  properties:
    stormAggregation:
      description: |-
        Storm Aggregation (BR-GATEWAY-016)
        Populated only for storm-aggregated CRDs
        nil for individual alert CRDs
      properties:
        affectedResources:
          items:
            properties:
              kind:
                type: string
              name:
                type: string
              namespace:
                type: string
            required:
            - kind
            - name
            type: object
          type: array
        # ... more fields
      type: object  # ✅ Structural schema
```

### 3. Go Struct Has Correct JSON Tag
```go
// api/remediation/v1alpha1/remediationrequest_types.go:89
StormAggregation *StormAggregation `json:"stormAggregation,omitempty"`
```

### 4. CRD Object Has Field Set Before K8s Create
```json
{
  "level": "info",
  "msg": "Attempting to create storm CRD",
  "name": "storm-highmemoryusage-in-prod-payments-87dd33ff1973",
  "namespace": "prod-payments",
  "apiVersion": "remediation.kubernaut.io/v1alpha1",  // ✅ Set
  "kind": "RemediationRequest",  // ✅ Set
  "has_storm_aggregation": true,  // ✅ Not nil
  "alert_count": 1  // ✅ Has data
}
```

### 5. Scheme is Registered
```go
// test/integration/gateway/helpers.go:153
scheme := k8sruntime.NewScheme()
_ = remediationv1alpha1.AddToScheme(scheme)  // ✅ Registered
```

### 6. Fresh Kind Cluster
- Deleted old cluster
- Created new cluster
- CRD installed fresh
- No cached schema

## ❌ **What's Still Failing**

1. **K8s API Warning**: `unknown field "spec.stormAggregation"`
2. **Field Dropped**: All CRDs in K8s show `hasStormAggregation=false`
3. **Test Fails**: Cannot find storm CRD with `StormAggregation != nil`

## 🤔 **Hypotheses**

### Hypothesis 1: Controller-Runtime Client Issue
Maybe `controller-runtime` v0.19.2 has a bug with optional pointer fields?

### Hypothesis 2: CRD Schema Validation Issue
Maybe the CRD schema has a subtle structural issue that's not obvious?

### Hypothesis 3: Go Struct Definition Issue
Maybe there's something wrong with how `StormAggregation` type is defined?

### Hypothesis 4: Omitempty Behavior
Maybe `omitempty` is causing the field to be dropped even when not nil?

## 🔬 **Next Steps to Try**

### Option A: Remove `omitempty` from JSON Tag
```go
// Try without omitempty
StormAggregation *StormAggregation `json:"stormAggregation"`
```

### Option B: Check StormAggregation Type Definition
```bash
# Verify the StormAggregation type is properly defined
grep -A30 "type StormAggregation struct" api/remediation/v1alpha1/remediationrequest_types.go
```

### Option C: Test with Simple Field
```go
// Add a simple non-pointer field to test
StormTest string `json:"stormTest"`
```

### Option D: Check controller-runtime Version
```bash
# Maybe upgrade controller-runtime?
go get sigs.k8s.io/controller-runtime@latest
```

### Option E: Marshal to JSON and Inspect
```go
// Before sending to K8s, marshal to JSON and log it
jsonBytes, _ := json.Marshal(crd)
c.logger.Info("CRD JSON", zap.String("json", string(jsonBytes)))
```

## 📊 **Test Results**

- **Test**: Single failing test with clean Redis
- **Result**: Fails even in isolation
- **Conclusion**: NOT a test infrastructure problem
- **Root Cause**: K8s is dropping the `stormAggregation` field

## 🎯 **Recommendation**

Try **Option E** first to see the actual JSON being sent to K8s, then **Option B** to verify the type definition.




