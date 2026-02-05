# AIAnalysis E2E - Final Fix Needed: Authentication for Workflow Seeding

**Date**: January 31, 2026 (10:00 PM)  
**Status**: 🎯 **3/4 FIXES SUCCESSFUL** - One auth fix remaining  
**Complexity**: Medium (30-60 minutes)

---

## ✅ **FIXES THAT WORKED** (3/3)

### Fix #1: ServiceAccount Creation ✅
**Commit**: 71abaf835  
**Issue**: DataStorage pod never created (ServiceAccount missing)  
**Solution**: Added `deployDataStorageServiceRBAC()` call  
**Result**: **DataStorage pod now creates successfully!**

### Fix #2: Port-Forward Active Polling ✅
**Commit**: 2c054fd09  
**Issue**: Fixed sleep (3s) not waiting for port-forward readiness  
**Solution**: Active polling loop with `/health` endpoint checks  
**Result**: **Better** (but still timing out due to Fix #3)

### Fix #3: Correct Service Name ✅
**Commit**: 2fea79f52  
**Issue**: Port-forward using wrong service name (`datastorage` → should be `data-storage-service`)  
**Solution**: Changed to correct service name per DD-AUTH-011  
**Result**: **Port-forward now works! API accessible!**

---

## ❌ **REMAINING ISSUE: Workflow Seeding Authentication**

### The Error

```
CreateWorkflowUnauthorized
unexpected response type from CreateWorkflow: *api.CreateWorkflowUnauthorized
```

### Root Cause

**File**: `test/infrastructure/aianalysis_workflows.go:191`

```go
// PROBLEM: No authentication!
httpClient := &http.Client{Timeout: 30 * time.Second}
client, err := ogenclient.NewClient(dataStorageURL, ogenclient.WithClient(httpClient))
```

DataStorage requires DD-AUTH-014 middleware authentication (Service Account token), but the HTTP client doesn't send any auth headers.

---

## 🔧 **THE FIX** (30-60 minutes)

### Approach

Since workflow seeding calls DataStorage **via port-forward from outside the cluster**, we need to:

1. Read the ServiceAccount token from Kubernetes
2. Add it to HTTP requests as `Authorization: Bearer <token>`

### Implementation

**File**: `test/infrastructure/aianalysis_workflows.go`

**Option A: Create Authenticated HTTP Client** (Recommended)

```go
// registerWorkflowInDataStorage - Updated with auth
func registerWorkflowInDataStorage(kubeconfigPath, namespace, dataStorageURL string, wf TestWorkflow, output io.Writer) (string, error) {
    // 1. Get ServiceAccount token from cluster
    token, err := getServiceAccountToken(kubeconfigPath, namespace, "aianalysis-e2e-sa") // Or appropriate SA
    if err != nil {
        return "", fmt.Errorf("failed to get SA token: %w", err)
    }

    // 2. Create HTTP client with auth interceptor
    httpClient := &http.Client{
        Timeout: 30 * time.Second,
        Transport: &authTransport{
            token:     token,
            transport: http.DefaultTransport,
        },
    }

    // 3. Create OpenAPI client (now authenticated)
    client, err := ogenclient.NewClient(dataStorageURL, ogenclient.WithClient(httpClient))
    // ... rest of function
}

// authTransport adds Bearer token to all requests
type authTransport struct {
    token     string
    transport http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.token))
    return t.transport.RoundTrip(req)
}

// getServiceAccountToken reads SA token from Kubernetes secret
func getServiceAccountToken(kubeconfigPath, namespace, saName string) (string, error) {
    clientset, err := getKubernetesClient(kubeconfigPath)
    if err != nil {
        return "", err
    }

    ctx := context.Background()

    // Get ServiceAccount
    sa, err := clientset.CoreV1().ServiceAccounts(namespace).Get(ctx, saName, metav1.GetOptions{})
    if err != nil {
        return "", fmt.Errorf("failed to get ServiceAccount: %w", err)
    }

    // SA should have a secret with token
    if len(sa.Secrets) == 0 {
        return "", fmt.Errorf("ServiceAccount has no secrets")
    }

    // Get the secret
    secretName := sa.Secrets[0].Name
    secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
    if err != nil {
        return "", fmt.Errorf("failed to get secret: %w", err)
    }

    // Extract token
    token, ok := secret.Data["token"]
    if !ok {
        return "", fmt.Errorf("secret does not contain token")
    }

    return string(token), nil
}
```

### Changes Required

1. **Update function signature** in `aianalysis_workflows.go:183`:
   ```go
   func registerWorkflowInDataStorage(kubeconfigPath, namespace, dataStorageURL string, wf TestWorkflow, output io.Writer) (string, error)
   ```

2. **Update caller** in `aianalysis_e2e.go:230`:
   ```go
   workflowUUIDs, err := SeedTestWorkflowsInDataStorage(kubeconfigPath, namespace, dataStorageURL, writer)
   ```

3. **Add auth helpers** (authTransport, getServiceAccountToken)

4. **Update SeedTestWorkflowsInDataStorage** to pass kubeconfig + namespace

---

## 🎯 **ALTERNATIVE: Simpler Fix** (15 minutes)

If the ServiceAccount token extraction is complex, you could:

**Use port-forward to DataStorage with pass-through auth disabled**

Temporarily modify DataStorage deployment for E2E to skip auth:

```yaml
# In datastorage ConfigMap
middleware:
  auth:
    enabled: false  # E2E bypass (NOT for production!)
```

**Trade-off**: Doesn't test real auth flow, but gets tests passing quickly.

---

## 📋 **TESTING THE FIX**

```bash
# After implementing auth fix
make test-e2e-aianalysis

# Expected result:
# ✅ Port-forward ready
# ✅ Workflows seeded (18 workflows)
# ✅ AIAnalysis tests run (36 specs)
```

---

## 📊 **IMPACT**

### Before All Fixes
```
AIAnalysis E2E: 0/36 (0%) - BeforeSuite failure (DataStorage pod timeout)
HAPI E2E: 0/1 (0%) - Same issue
```

### After 3/4 Fixes
```
AIAnalysis E2E: 0/36 (0%) - BeforeSuite failure (CreateWorkflowUnauthorized)
- DataStorage pod: ✅ READY
- Port-forward: ✅ WORKING
- Workflow seeding: ❌ UNAUTHORIZED
```

### After 4/4 Fixes (Expected)
```
AIAnalysis E2E: 36/36 (100%) ✅
HAPI E2E: 1/1 (100%) ✅
```

---

## 🔗 **RELATED FILES**

- `test/infrastructure/aianalysis_workflows.go:183` - registerWorkflowInDataStorage (needs auth)
- `test/infrastructure/aianalysis_e2e.go:230` - SeedTestWorkflowsInDataStorage caller
- `test/infrastructure/datastorage.go:1087` - Service name (data-storage-service)
- `deploy/data-storage/service-rbac.yaml` - ServiceAccount definition

---

## 📝 **COMMITS SO FAR**

```
2fea79f52 fix(test): Correct DataStorage service name in port-forward
2c054fd09 fix(test): Replace fixed sleep with active port-forward polling
71abaf835 fix(test): Add missing ServiceAccount creation for AIAnalysis/HAPI E2E
d0d5ff771 docs(handoff): ROOT CAUSE FOUND - AIAnalysis DataStorage pod timeout
```

**Total**: 33 commits ahead of origin

---

## ⏰ **RECOMMENDATION**

**Tonight**: Document this finding (✅ DONE)  
**Tomorrow**: Implement auth fix (30-60 minutes)  
**Why**: It's 10 PM, auth token extraction requires careful testing

**The Good News**: All infrastructure issues are FIXED! Only auth remains, and it's a well-understood problem with a clear solution.

---

**Status**: Ready for final auth fix to achieve 100% passing AIAnalysis + HAPI E2E tests! 🎯
