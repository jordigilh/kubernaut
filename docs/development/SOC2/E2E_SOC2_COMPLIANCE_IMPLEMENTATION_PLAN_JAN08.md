# E2E SOC2 Compliance Implementation Plan - January 8, 2026

## 🎯 **OBJECTIVE**

Deploy **real OAuth proxy** and **AuthWebhook** to all E2E test suites requiring SOC2 user attribution for CRD operations.

**User Decision**: Option 3 (Shared Infrastructure) + Option 2 (All 3 E2E Suites)

---

## 📋 **EXECUTIVE SUMMARY**

### **Current State**
- ✅ **AuthWebhook E2E**: Has AuthWebhook deployed, but NO oauth-proxy with DataStorage
- ❌ **RemediationOrchestrator E2E**: Missing AuthWebhook, has DataStorage but NO oauth-proxy
- ❌ **WorkflowExecution E2E**: Missing AuthWebhook, has DataStorage but NO oauth-proxy
- ❌ **Notification E2E**: Missing AuthWebhook, has DataStorage but NO oauth-proxy
- ⚠️ **DataStorage E2E**: N/A (DataStorage is storage service, doesn't create CRDs)

### **Target State**
- ✅ **AuthWebhook E2E**: AuthWebhook + DataStorage with oauth-proxy ✅
- ✅ **RemediationOrchestrator E2E**: AuthWebhook + DataStorage with oauth-proxy ✅
- ✅ **WorkflowExecution E2E**: AuthWebhook + DataStorage with oauth-proxy ✅
- ✅ **Notification E2E**: AuthWebhook + DataStorage with oauth-proxy ✅

### **Timeline**
- **Week 1 (Jan 8-10)**: Phase 1-2 (Shared infrastructure + AuthWebhook E2E reference)
- **Week 1 (Jan 11-12)**: Phase 3 (Rollout to RO, WE, NT E2E suites)
- **Total Effort**: 8-10 hours across 5 days

---

## 🔍 **TECHNICAL BACKGROUND**

### **Why OAuth Proxy for DataStorage?**

**SOC2 Requirement (CC8.1)**: "Capture authenticated user identity for all data modifications"

**Problem**: DataStorage API doesn't have built-in authentication. Controllers use service accounts.

**Solution**: `ose-oauth-proxy` sidecar injects `X-Forwarded-User` header from K8s authentication context.

**Pattern**:
```
kubectl request → K8s API auth → oauth-proxy → X-Forwarded-User header → DataStorage
```

**Result**: Audit events have real user identity, not service account.

---

### **Why AuthWebhook for Controllers?**

**SOC2 Requirement (CC8.1)**: "Capture WHO made manual operational changes"

**Problem**: Controllers create CRDs automatically. Operators manually modify CRD status fields.

**Solution**: Admission webhooks intercept manual status updates and inject authenticated user.

**Pattern**:
```
kubectl patch → K8s admission → AuthWebhook → Adds actor annotations → Audit event with user
```

**Result**: Manual operator actions (approvals, block clearances, cancellations) have user attribution.

---

## 🏗️ **IMPLEMENTATION PHASES**

### **Phase 1: Shared OAuth-Proxy Infrastructure** ⏱️ 3-4 hours

**Objective**: Create reusable `DeployDataStorageWithOAuthProxy()` function

**File**: `test/infrastructure/shared_e2e_utils.go` (NEW)

**Function Signature**:
```go
// DeployDataStorageWithOAuthProxy deploys DataStorage with oauth-proxy sidecar for SOC2 user attribution
// This is the AUTHORITATIVE E2E pattern for all test suites creating CRDs
//
// OAuth Proxy Configuration (E2E-specific):
//   - Image: quay.io/openshift/origin-oauth-proxy:latest (multi-platform)
//   - Static User: "test-operator@kubernaut.ai" (no real OAuth provider needed)
//   - Header Injection: X-Forwarded-User (validated by DataStorage)
//   - Client ID: "e2e-test-client" (placeholder for E2E)
//
// Parameters:
//   - ctx: Context for deployment operations
//   - namespace: K8s namespace for deployment
//   - dsImageName: DataStorage image (from BuildImageForKind)
//   - kubeconfigPath: Isolated kubeconfig for E2E cluster
//   - dsAPIPort: DataStorage API container port (default: 8080)
//   - dsNodePort: NodePort for external access (per DD-TEST-001)
//   - writer: Output stream for deployment logs
//
// Returns:
//   - error: Deployment failure or nil on success
//
// SOC2 Compliance:
//   - BR-AUTH-001: User attribution via oauth-proxy header injection
//   - CC8.1: All DataStorage API calls include authenticated user identity
//
// Authority:
//   - DD-AUTH-003: Externalized Authorization Sidecar pattern
//   - DD-TEST-001: E2E test infrastructure standards
func DeployDataStorageWithOAuthProxy(
    ctx context.Context,
    namespace string,
    dsImageName string,
    kubeconfigPath string,
    dsAPIPort string,
    dsNodePort string,
    writer io.Writer,
) error
```

**Implementation Details**:

1. **OAuth Proxy ConfigMap** (static E2E configuration):
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: oauth-proxy-config
  namespace: {{ .Namespace }}
data:
  oauth-proxy.cfg: |
    # E2E OAuth Proxy Configuration (Static User Injection)
    # NOT a real OAuth provider - just validates proxy pattern
    provider = "static"
    email_domains = ["*"]
    upstreams = ["http://localhost:8080/"]
    http_address = "0.0.0.0:4180"
    # E2E: Inject static user for all requests
    static_user = "test-operator@kubernaut.ai"
```

2. **DataStorage Deployment with OAuth Proxy Sidecar**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: {{ .Namespace }}
spec:
  replicas: 1
  template:
    spec:
      containers:
      # Primary container: DataStorage service
      - name: datastorage
        image: {{ .DataStorageImage }}
        imagePullPolicy: Never
        ports:
        - name: http
          containerPort: 8080  # Internal port (not exposed externally)
        env:
        - name: CONFIG_PATH
          value: /etc/datastorage/config.yaml
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
        - name: secrets
          mountPath: /etc/datastorage/secrets

      # Sidecar container: OAuth Proxy
      - name: oauth-proxy
        image: quay.io/openshift/origin-oauth-proxy:latest
        imagePullPolicy: IfNotPresent
        ports:
        - name: proxy
          containerPort: 4180  # Proxy port (exposed via NodePort)
        args:
        - --config=/etc/oauth-proxy/oauth-proxy.cfg
        - --upstream=http://localhost:8080
        - --http-address=0.0.0.0:4180
        - --pass-user-bearer-token=false
        - --pass-access-token=false
        - --set-xauthrequest=true
        volumeMounts:
        - name: oauth-config
          mountPath: /etc/oauth-proxy

      volumes:
      - name: oauth-config
        configMap:
          name: oauth-proxy-config
      - name: config
        configMap:
          name: datastorage-config
      - name: secrets
        secret:
          secretName: datastorage-secret
---
apiVersion: v1
kind: Service
metadata:
  name: datastorage
  namespace: {{ .Namespace }}
spec:
  type: NodePort
  ports:
  - name: api
    port: 8080
    targetPort: 4180  # Route to oauth-proxy, not DataStorage directly
    nodePort: {{ .NodePort }}
    protocol: TCP
  selector:
    app: datastorage
```

**Key Design Points**:
- OAuth proxy listens on `:4180`, DataStorage on `:8080`
- Service routes external traffic → oauth-proxy (`:4180`) → DataStorage (`:8080`)
- Static user `test-operator@kubernaut.ai` injected for ALL requests
- No real OAuth provider needed for E2E validation

**Validation**:
```go
// After deployment, validate oauth-proxy is working:
resp, err := http.Get("http://localhost:" + dsNodePort + "/health/ready")
// Should succeed with X-Forwarded-User header present in DataStorage logs
```

---

### **Phase 1b: Shared AuthWebhook Infrastructure** ⏱️ 2-3 hours

**Objective**: Create reusable `DeploySharedAuthWebhook()` function

**File**: `test/infrastructure/shared_e2e_utils.go` (ADD to same file)

**Function Signature**:
```go
// DeploySharedAuthWebhook deploys AuthWebhook service for SOC2-compliant CRD operations
// This is the AUTHORITATIVE E2E pattern for all test suites with manual operator actions
//
// Webhook Configuration:
//   - Service: Consolidated webhook for WorkflowExecution, RemediationApprovalRequest, NotificationRequest
//   - Operations: STATUS updates (manual operator changes) and DELETE (cancellations)
//   - Authority: DD-WEBHOOK-001 (CRD Webhook Requirements Matrix)
//
// Parameters:
//   - ctx: Context for deployment operations
//   - clusterName: Kind cluster name (for image loading)
//   - namespace: K8s namespace for webhook deployment
//   - kubeconfigPath: Isolated kubeconfig for E2E cluster
//   - writer: Output stream for deployment logs
//
// Returns:
//   - error: Deployment failure or nil on success
//
// Deployment Steps:
//   1. Build AuthWebhook image (if not already built)
//   2. Load image to Kind cluster
//   3. Generate webhook TLS certificates
//   4. Apply CRDs (all kubernaut.ai CRDs)
//   5. Deploy AuthWebhook service
//   6. Patch webhook configurations with CA bundle
//   7. Wait for webhook pod readiness
//
// SOC2 Compliance:
//   - BR-AUTH-001: User attribution for manual CRD operations
//   - CC8.1: All manual status updates include authenticated user
//
// Authority:
//   - DD-WEBHOOK-001: Webhook requirements matrix
//   - DD-WEBHOOK-003: Webhook-complete audit pattern
func DeploySharedAuthWebhook(
    ctx context.Context,
    clusterName string,
    namespace string,
    kubeconfigPath string,
    writer io.Writer,
) error
```

**Implementation**: Refactor existing `deployAuthWebhookToKind()` from `test/infrastructure/authwebhook_e2e.go` into shared function.

---

### **Phase 2: AuthWebhook E2E Reference Implementation** ⏱️ 1-2 hours

**Objective**: Validate shared infrastructure in AuthWebhook E2E (already has webhook, add oauth-proxy)

**File**: `test/infrastructure/authwebhook_e2e.go`

**Changes**:

**BEFORE** (line 240):
```go
// Deploy DataStorage service (E2E ports per DD-TEST-001)
_, _ = fmt.Fprintln(writer, "  📦 Deploying DataStorage service...")
if err := deployDataStorageToKind(kubeconfigPath, namespace, dsImageName, "28099", "30099", writer); err != nil {
    return "", "", fmt.Errorf("failed to deploy DataStorage: %w", err)
}
```

**AFTER**:
```go
// Deploy DataStorage service WITH oauth-proxy (SOC2 user attribution)
_, _ = fmt.Fprintln(writer, "  📦 Deploying DataStorage service with oauth-proxy...")
if err := DeployDataStorageWithOAuthProxy(ctx, namespace, dsImageName, kubeconfigPath, "8080", "30099", writer); err != nil {
    return "", "", fmt.Errorf("failed to deploy DataStorage with oauth-proxy: %w", err)
}
```

**Validation**:
```bash
# Run AuthWebhook E2E tests
make test-e2e-authwebhook

# Expected: All audit events have actor_id: "test-operator@kubernaut.ai"
# Not: "system:serviceaccount:authwebhook-e2e:datastorage"
```

---

### **Phase 3: Rollout to RO, WE, NT E2E Suites** ⏱️ 3-4 hours

**Objective**: Deploy AuthWebhook + oauth-proxy to 3 E2E suites simultaneously

---

#### **3a. RemediationOrchestrator E2E** ⏱️ 1 hour

**File**: `test/infrastructure/remediationorchestrator_e2e_hybrid.go`

**Change 1: Add AuthWebhook Deployment** (after line 310):
```go
// PHASE 4.5: Deploy AuthWebhook for SOC2-compliant CRD operations
_, _ = fmt.Fprintln(writer, "\n🔐 PHASE 4.5: Deploying AuthWebhook for operator attribution...")
if err := DeploySharedAuthWebhook(ctx, clusterName, namespace, kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy AuthWebhook: %w", err)
}
_, _ = fmt.Fprintln(writer, "  ✅ AuthWebhook deployed (SOC2 CC8.1 user attribution)")
```

**Change 2: Replace DataStorage Deployment** (line 302):

**BEFORE**:
```go
go func() {
    dsImage := builtImages["DataStorage"]
    err := deployDataStorageServiceInNamespace(ctx, WorkflowExecutionNamespace, kubeconfigPath, dsImage, writer)
    deployResults <- deployResult{"DataStorage", err}
}()
```

**AFTER**:
```go
go func() {
    dsImage := builtImages["DataStorage"]
    // Deploy DataStorage WITH oauth-proxy for SOC2 user attribution
    err := DeployDataStorageWithOAuthProxy(ctx, namespace, dsImage, kubeconfigPath, "8080", "30090", writer)
    deployResults <- deployResult{"DataStorage + OAuth Proxy", err}
}()
```

---

#### **3b. WorkflowExecution E2E** ⏱️ 1 hour

**File**: `test/infrastructure/workflowexecution_e2e_hybrid.go`

**Change 1: Add AuthWebhook Deployment** (after line 310):
```go
// PHASE 4.5: Deploy AuthWebhook for SOC2-compliant CRD operations
_, _ = fmt.Fprintln(writer, "\n🔐 PHASE 4.5: Deploying AuthWebhook for block clearance attribution...")
if err := DeploySharedAuthWebhook(ctx, clusterName, WorkflowExecutionNamespace, kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy AuthWebhook: %w", err)
}
_, _ = fmt.Fprintln(writer, "  ✅ AuthWebhook deployed (SOC2 CC8.1 block clearance)")
```

**Change 2: Replace DataStorage Deployment** (line 302):

**BEFORE**:
```go
go func() {
    dsImage := builtImages["DataStorage"]
    err := deployDataStorageServiceInNamespace(ctx, WorkflowExecutionNamespace, kubeconfigPath, dsImage, writer)
    deployResults <- deployResult{"DataStorage", err}
}()
```

**AFTER**:
```go
go func() {
    dsImage := builtImages["DataStorage"]
    // Deploy DataStorage WITH oauth-proxy for SOC2 user attribution
    err := DeployDataStorageWithOAuthProxy(ctx, WorkflowExecutionNamespace, dsImage, kubeconfigPath, "8080", "30081", writer)
    deployResults <- deployResult{"DataStorage + OAuth Proxy", err}
}()
```

---

#### **3c. Notification E2E** ⏱️ 1 hour

**File**: `test/infrastructure/notification_e2e.go`

**Change 1: Add AuthWebhook Deployment** (after DataStorage deployment):

**BEFORE** (line 219-237):
```go
// Deploy DataStorage
err = infrastructure.DeployNotificationAuditInfrastructure(ctx, controllerNamespace, kubeconfigPath, GinkgoWriter)
Expect(err).ToNot(HaveOccurred(), "Audit infrastructure deployment should succeed")
logger.Info("✅ Audit infrastructure ready")
```

**AFTER**:
```go
// Deploy DataStorage WITH oauth-proxy for SOC2 user attribution
err = infrastructure.DeployNotificationAuditInfrastructure(ctx, controllerNamespace, kubeconfigPath, GinkgoWriter)
Expect(err).ToNot(HaveOccurred(), "Audit infrastructure deployment should succeed")
logger.Info("✅ Audit infrastructure ready")

// Deploy AuthWebhook for SOC2-compliant notification cancellations
logger.Info("🔐 Deploying AuthWebhook for cancellation attribution...")
err = infrastructure.DeploySharedAuthWebhook(ctx, clusterName, controllerNamespace, kubeconfigPath, GinkgoWriter)
Expect(err).ToNot(HaveOccurred(), "AuthWebhook deployment should succeed")
logger.Info("✅ AuthWebhook deployed (SOC2 CC8.1 cancellation attribution)")
```

**Change 2: Update `DeployNotificationAuditInfrastructure()`** (line 218-303):

Replace existing DataStorage deployment with oauth-proxy version:
```go
// In DeployNotificationAuditInfrastructure():
// BEFORE:
err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, dsImageName, writer)

// AFTER:
err := DeployDataStorageWithOAuthProxy(ctx, namespace, dsImageName, kubeconfigPath, "8080", "30090", writer)
```

---

### **Phase 4: E2E Validation** ⏱️ 1-2 hours

**Objective**: Validate SOC2 compliance across all 4 E2E suites

**Test Suite**: Run all E2E tests
```bash
# 1. AuthWebhook E2E (reference implementation)
make test-e2e-authwebhook

# 2. RemediationOrchestrator E2E
make test-e2e-ro

# 3. WorkflowExecution E2E
make test-e2e-workflowexecution

# 4. Notification E2E
make test-e2e-notification
```

**Validation Checklist**:
- [ ] All E2E tests pass ✅
- [ ] OAuth proxy pods running in all 4 E2E clusters
- [ ] AuthWebhook pods running in 4 E2E clusters (RO, WE, NT, AuthWebhook)
- [ ] Audit events have `actor_id: "test-operator@kubernaut.ai"` (not service accounts)
- [ ] DataStorage logs show `X-Forwarded-User: test-operator@kubernaut.ai`
- [ ] Manual CRD operations (approvals, block clearances, cancellations) have user attribution

---

## 📊 **SUCCESS CRITERIA**

### **SOC2 CC8.1 Compliance**

All E2E test suites MUST demonstrate:

1. **User Attribution for DataStorage**:
   - ✅ All audit events have `actor_type: "user"`
   - ✅ All audit events have `actor_id: "test-operator@kubernaut.ai"`
   - ❌ No audit events with `actor_id: "system:serviceaccount:..."`

2. **User Attribution for Manual CRD Operations**:
   - ✅ WorkflowExecution block clearances have authenticated user
   - ✅ RemediationApprovalRequest approvals have authenticated user
   - ✅ NotificationRequest cancellations have authenticated user

3. **Infrastructure Health**:
   - ✅ OAuth proxy pods running and healthy
   - ✅ AuthWebhook pods running and healthy
   - ✅ TLS certificates valid and webhook registered
   - ✅ No admission errors in K8s API server logs

---

## 🔗 **AUTHORITY DOCUMENTS**

### **Design Decisions**
- **DD-WEBHOOK-001**: CRD Webhook Requirements Matrix
- **DD-WEBHOOK-003**: Webhook-Complete Audit Pattern
- **DD-AUTH-001**: Shared Authentication Webhook
- **DD-AUTH-003**: Externalized Authorization Sidecar
- **DD-TEST-001**: E2E Test Infrastructure Standards

### **Business Requirements**
- **BR-AUTH-001**: SOC2 CC8.1 User Attribution
- **BR-AUDIT-005**: Enterprise-Grade Audit Integrity

### **SOC2 Controls**
- **CC8.1**: Change Management - Attribution Requirements
- **CC7.3**: Audit Trail Integrity
- **CC7.4**: Audit Trail Completeness

---

## 📋 **RISK ASSESSMENT**

### **Technical Risks**

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| OAuth proxy image platform mismatch | LOW | HIGH | Use multi-platform `quay.io/openshift/origin-oauth-proxy:latest` |
| TLS certificate expiration in E2E | MEDIUM | MEDIUM | Generate fresh certs per test run (existing pattern) |
| Port conflicts across E2E suites | LOW | MEDIUM | Use DD-TEST-001 port allocation standard |
| Webhook admission failures | MEDIUM | HIGH | Implement webhook readiness checks before tests |
| DataStorage connectivity through proxy | MEDIUM | HIGH | Validate X-Forwarded-User header in E2E tests |

### **Process Risks**

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Code duplication if not using shared functions | HIGH | LOW | Enforce shared infrastructure pattern (Option 3) |
| E2E test flakiness during rollout | MEDIUM | MEDIUM | Validate AuthWebhook E2E first (reference impl) |
| Regression in existing E2E tests | LOW | HIGH | Run full E2E suite after each phase |

---

## 🎯 **NEXT STEPS**

1. ✅ **User Approval**: Option 3 (Shared Infra) + Option 2 (All E2E Suites) **APPROVED**
2. 🚀 **Start Implementation**: Phase 1 (Shared oauth-proxy infrastructure)
3. 📝 **Track Progress**: Update TODO list as phases complete
4. ✅ **Validation**: Run E2E tests after each phase
5. 📊 **Documentation**: Update E2E test documentation with SOC2 patterns

---

## 📚 **DELIVERABLES**

### **Code Artifacts**
- [ ] `test/infrastructure/shared_e2e_utils.go` (NEW) - Shared oauth-proxy + webhook functions
- [ ] `test/infrastructure/authwebhook_e2e.go` (MODIFIED) - Use shared oauth-proxy
- [ ] `test/infrastructure/remediationorchestrator_e2e_hybrid.go` (MODIFIED) - Add webhook + oauth-proxy
- [ ] `test/infrastructure/workflowexecution_e2e_hybrid.go` (MODIFIED) - Add webhook + oauth-proxy
- [ ] `test/infrastructure/notification_e2e.go` (MODIFIED) - Add webhook + oauth-proxy

### **Documentation**
- [ ] This implementation plan (COMPLETE)
- [ ] Updated E2E test documentation with SOC2 patterns
- [ ] SOC2 compliance validation report

### **Validation**
- [ ] All 4 E2E test suites passing with SOC2 compliance
- [ ] Audit events demonstrate user attribution
- [ ] OAuth proxy operational in all E2E clusters

---

**AUTHOR**: AI Assistant
**DATE**: January 8, 2026
**STATUS**: 🚀 READY TO IMPLEMENT
**ESTIMATED COMPLETION**: January 12, 2026 (5 days)


