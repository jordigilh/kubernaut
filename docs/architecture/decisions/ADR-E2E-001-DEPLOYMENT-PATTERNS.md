# ADR-E2E-001: E2E Test Service Deployment Patterns

**Status**: ✅ APPROVED - Authoritative
**Date**: December 22, 2025
**Priority**: MANDATORY - All E2E tests MUST follow these patterns

---

## Decision

E2E tests MUST deploy services programmatically using Go code within the `test/infrastructure/` package, NOT through manual `kubectl apply` commands or external shell scripts.

---

## Approved Deployment Patterns

### Pattern 1: kubectl apply -f YAML File (Preferred for Static Resources)

**Use Case**: Simple, static Kubernetes resources (RBAC, ConfigMaps, Services, Deployments)

**Advantages**:
- ✅ Simple and readable
- ✅ Easy to maintain YAML files
- ✅ Version-controlled configuration
- ✅ Matches production deployment model

**Implementation**:
```go
func deployNotificationConfigMap(namespace, kubeconfigPath string, writer io.Writer) error {
    workspaceRoot, err := findWorkspaceRoot()
    if err != nil {
        return fmt.Errorf("failed to find workspace root: %w", err)
    }

    // Path to YAML manifest
    configMapPath := filepath.Join(workspaceRoot, "test", "e2e", "notification", "manifests", "notification-configmap.yaml")
    if _, err := os.Stat(configMapPath); os.IsNotExist(err) {
        return fmt.Errorf("ConfigMap manifest not found at %s", configMapPath)
    }

    // Apply using kubectl
    applyCmd := exec.Command("kubectl", "apply", "-f", configMapPath, "-n", namespace)
    applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
    applyCmd.Stdout = writer
    applyCmd.Stderr = writer

    if err := applyCmd.Run(); err != nil {
        return fmt.Errorf("failed to apply ConfigMap: %w", err)
    }

    fmt.Fprintf(writer, "   ConfigMap deployed in namespace: %s\n", namespace)
    return nil
}
```

**Location**: `test/infrastructure/{service}.go`
**Example**: `test/infrastructure/notification.go` lines 459-484, 512-534

---

### Pattern 2: Programmatic Kubernetes Client (Required for Dynamic Resources)

**Use Case**: Dynamic resources requiring runtime configuration (database URLs, ports, complex specs)

**Advantages**:
- ✅ Full programmatic control
- ✅ Type-safe resource creation
- ✅ Dynamic configuration injection
- ✅ No template string manipulation

**Implementation**:
```go
func deployDataStorageServiceForNotification(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
    clientset, err := getKubernetesClient(kubeconfigPath)
    if err != nil {
        return err
    }

    // 1. Create ConfigMap programmatically
    configYAML := fmt.Sprintf(`database:
  host: postgresql.%s.svc.cluster.local
  port: 5432
redis:
  addr: redis.%s.svc.cluster.local:6379`, namespace, namespace)

    configMap := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "datastorage-config",
            Namespace: namespace,
        },
        Data: map[string]string{
            "config.yaml": configYAML,
        },
    }

    _, err = clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
    if err != nil {
        return fmt.Errorf("failed to create ConfigMap: %w", err)
    }

    // 2. Create Deployment programmatically
    replicas := int32(1)
    deployment := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "datastorage",
            Namespace: namespace,
        },
        Spec: appsv1.DeploymentSpec{
            Replicas: &replicas,
            // ... full deployment spec
        },
    }

    _, err = clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
    if err != nil {
        return fmt.Errorf("failed to create Deployment: %w", err)
    }

    return nil
}
```

**Location**: `test/infrastructure/{service}.go`
**Example**: `test/infrastructure/notification.go` lines 576-813

---

### Pattern 3: Template + kubectl apply -f - (Acceptable for Parameterized Resources)

**Use Case**: Resources requiring placeholder substitution (service names, namespaces)

**Advantages**:
- ✅ Template reusability
- ✅ Simple placeholder replacement
- ✅ Still uses kubectl (consistent tooling)

**Implementation**:
```go
func DeployMockService(ctx context.Context, namespace, serviceName string, kubeconfigPath string, writer io.Writer) error {
    // Read template
    templatePath := filepath.Join(workspaceRoot, "test", "e2e", "toolset", "mock-service-template.yaml")
    templateContent, err := os.ReadFile(templatePath)
    if err != nil {
        return fmt.Errorf("failed to read template: %w", err)
    }

    // Replace placeholders
    manifestContent := string(templateContent)
    manifestContent = strings.ReplaceAll(manifestContent, "{{NAMESPACE}}", namespace)
    manifestContent = strings.ReplaceAll(manifestContent, "{{SERVICE_NAME}}", serviceName)

    // Apply via stdin
    applyCmd := exec.Command("kubectl", "apply", "-f", "-")
    applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
    applyCmd.Stdin = strings.NewReader(manifestContent)
    applyCmd.Stdout = writer
    applyCmd.Stderr = writer

    if err := applyCmd.Run(); err != nil {
        return fmt.Errorf("failed to apply manifest: %w", err)
    }

    return nil
}
```

**Location**: `test/infrastructure/toolset.go`
**Example**: `test/infrastructure/toolset.go` lines 118-169

---

## Pattern Selection Decision Matrix

| Resource Type | Pattern | Justification |
|--------------|---------|---------------|
| **ConfigMap (static YAML)** | Pattern 1 (kubectl -f) | Simple, readable, matches production |
| **ConfigMap (dynamic, namespace-aware)** | Pattern 2 (K8s client) | Requires runtime configuration injection |
| **Deployment (static)** | Pattern 1 (kubectl -f) | Production-like deployment |
| **Deployment (complex, multi-namespace)** | Pattern 2 (K8s client) | Full programmatic control needed |
| **RBAC (ServiceAccount, Role, RoleBinding)** | Pattern 1 (kubectl -f) | Static, security-sensitive, version-controlled |
| **Service (NodePort, fixed ports)** | Pattern 1 (kubectl -f) | Simple, declarative |
| **Service (dynamic ports, complex routing)** | Pattern 2 (K8s client) | Runtime port calculation |
| **Mock Services (test-only)** | Pattern 3 (template) | Reusable across tests |

---

## Standard E2E Deployment Function Signature

### Required Signature
```go
// Deploy{Service}Controller deploys the {Service} Controller in a test namespace
// This is called in BeforeEach for each test file (or shared setup)
//
// Steps:
// 1. Create namespace
// 2. Deploy RBAC (ServiceAccount, Role, RoleBinding)
// 3. Deploy ConfigMap (if needed for configuration)
// 4. Deploy Service (for metrics/health)
// 5. Deploy Controller Deployment
// 6. Wait for controller ready
//
// Time: ~10-15 seconds
func Deploy{Service}Controller(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
    // Implementation follows one of the 3 approved patterns
}
```

### Required Parameters
- `ctx context.Context` - For cancellation and timeouts
- `namespace string` - Target Kubernetes namespace
- `kubeconfigPath string` - Path to isolated kubeconfig file
- `writer io.Writer` - Progress output (typically GinkgoWriter)

### Required Return
- `error` - Descriptive error with context, or `nil` on success

---

## Anti-Patterns - FORBIDDEN

### ❌ Manual kubectl Commands in Documentation
```bash
# BAD: Documentation suggesting manual deployment
kubectl apply -f test/e2e/notification/manifests/notification-configmap.yaml
kubectl apply -f test/e2e/notification/manifests/notification-deployment.yaml
```

**Why Forbidden**: E2E tests must be self-contained and reproducible. Manual commands introduce human error and are not programmatically validated.

**Correct Approach**: Call `infrastructure.DeployNotificationController(ctx, namespace, kubeconfigPath, GinkgoWriter)`

---

### ❌ Shell Scripts for Deployment
```bash
# BAD: External shell script
./scripts/deploy-notification-e2e.sh
```

**Why Forbidden**: Shell scripts are not integrated with Go test framework, make debugging harder, and add maintenance burden.

**Correct Approach**: Implement deployment logic in `test/infrastructure/{service}.go`

---

### ❌ Direct Go Kubernetes Client in Test Files
```go
// BAD: E2E test file creates Deployment directly
var _ = Describe("Notification E2E", func() {
    It("should deploy controller", func() {
        deployment := &appsv1.Deployment{
            // ... deployment spec ...
        }
        k8sClient.Create(ctx, deployment)
    })
})
```

**Why Forbidden**: Duplicates infrastructure code, makes tests brittle, violates separation of concerns.

**Correct Approach**: Move deployment logic to `test/infrastructure/notification.go`, call from test

---

## Implementation Requirements

### File Organization
```
test/
├── infrastructure/
│   ├── notification.go         # Notification deployment functions
│   ├── gateway.go             # Gateway deployment functions
│   ├── datastorage.go         # DataStorage deployment functions (shared)
│   ├── postgresql.go          # PostgreSQL deployment (shared)
│   └── redis.go               # Redis deployment (shared)
└── e2e/
    ├── notification/
    │   ├── manifests/         # YAML manifests for Pattern 1
    │   │   ├── notification-configmap.yaml
    │   │   ├── notification-deployment.yaml
    │   │   ├── notification-rbac.yaml
    │   │   └── notification-service.yaml
    │   └── notification_e2e_suite_test.go
    └── gateway/
        ├── manifests/
        │   └── ...
        └── gateway_e2e_suite_test.go
```

### Manifest Naming Convention
- **Format**: `{service}-{resource-type}.yaml`
- **Examples**:
  - `notification-configmap.yaml` (ConfigMap)
  - `notification-deployment.yaml` (Deployment)
  - `notification-rbac.yaml` (ServiceAccount + ClusterRole + ClusterRoleBinding)
  - `notification-service.yaml` (Service)

---

## Compliance Checklist

Before submitting E2E test infrastructure code:

### Infrastructure Function Requirements
- [ ] Deployment function in `test/infrastructure/{service}.go`
- [ ] Follows standard function signature (ctx, namespace, kubeconfigPath, writer)
- [ ] Uses one of the 3 approved patterns (justify choice in comments)
- [ ] Returns descriptive errors with context
- [ ] Writes progress messages to `writer` (for GinkgoWriter visibility)

### Manifest Requirements (Pattern 1 only)
- [ ] Manifests in `test/e2e/{service}/manifests/` directory
- [ ] Follows naming convention: `{service}-{resource-type}.yaml`
- [ ] No hardcoded namespaces (use `-n namespace` in kubectl apply)
- [ ] No hardcoded image pull policies (use `Never` for E2E, `IfNotPresent` for production)
- [ ] ConfigMap follows ADR-030 (YAML configuration standard)

### Test Suite Integration
- [ ] Test suite calls `infrastructure.Deploy{Service}Controller()`, NOT kubectl
- [ ] Test suite uses `infrastructure.Create{Service}Cluster()` for cluster setup
- [ ] Test suite uses `infrastructure.Delete{Service}Cluster()` for cleanup
- [ ] No manual `kubectl apply` commands in test code or documentation

### Documentation Requirements
- [ ] Deployment pattern documented in function comments
- [ ] Pattern choice justified (Pattern 1 vs 2 vs 3)
- [ ] Expected deployment time documented
- [ ] Dependencies and prerequisites listed

---

## Examples from Codebase

### Pattern 1 Example: Notification RBAC
**File**: `test/infrastructure/notification.go` lines 432-457
**Resource**: RBAC (ServiceAccount, ClusterRole, ClusterRoleBinding)
**Why Pattern 1**: Static, security-sensitive, version-controlled

### Pattern 2 Example: DataStorage for Notification
**File**: `test/infrastructure/notification.go` lines 576-813
**Resource**: DataStorage Service with namespace-specific configuration
**Why Pattern 2**: Dynamic database URLs, namespace-aware ConfigMap

### Pattern 3 Example: Mock Services
**File**: `test/infrastructure/toolset.go` lines 118-169
**Resource**: Mock services for Toolset E2E tests
**Why Pattern 3**: Reusable template with placeholder substitution

---

## Migration Guide for Non-Compliant Tests

### Step 1: Identify Manual kubectl Commands
```bash
# Find documentation with manual kubectl commands
grep -r "kubectl apply" docs/services/ --include="*.md"
```

### Step 2: Create Infrastructure Function
```go
// In test/infrastructure/{service}.go
func Deploy{Service}Controller(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
    // 1. Create namespace
    // 2. Deploy RBAC
    // 3. Deploy ConfigMap
    // 4. Deploy Service
    // 5. Deploy Controller
    // 6. Wait for ready
}
```

### Step 3: Update Test Suite
```go
// In test/e2e/{service}/suite_test.go
var _ = BeforeSuite(func() {
    // OLD: Manual kubectl commands
    // kubectl apply -f manifests/...

    // NEW: Programmatic deployment
    err := infrastructure.Deploy{Service}Controller(ctx, namespace, kubeconfigPath, GinkgoWriter)
    Expect(err).ToNot(HaveOccurred())
})
```

### Step 4: Update Documentation
Remove all manual `kubectl apply` commands from:
- `docs/services/crd-controllers/*/`
- `docs/handoff/`
- `README.md` files

---

## Rationale

### Why This Decision Was Made
1. **Reproducibility**: Programmatic deployment ensures consistent E2E test environment
2. **Maintainability**: Centralized deployment logic in `test/infrastructure/` reduces duplication
3. **Testability**: Go-based deployment is tested by the E2E framework itself
4. **Debugging**: Progress messages via `writer` provide visibility during test execution
5. **Integration**: Seamless integration with Ginkgo/Gomega test lifecycle

### Consequences
- **Positive**:
  - ✅ All E2E tests follow consistent deployment pattern
  - ✅ Easy to add new services following proven templates
  - ✅ No manual steps required to run E2E tests
  - ✅ Test infrastructure is version-controlled and code-reviewed

- **Negative**:
  - ⚠️ Requires learning Kubernetes Go client for Pattern 2
  - ⚠️ More upfront work than simple shell scripts
  - ⚠️ Pattern selection requires architectural judgment

---

## Validation

### How to Verify Compliance
```bash
# 1. Check for manual kubectl commands in docs
grep -r "kubectl apply" docs/services/ && echo "❌ FAIL: Found manual kubectl" || echo "✅ PASS"

# 2. Check infrastructure functions exist
for service in notification gateway datastorage; do
    grep -q "Deploy${service^}Controller" test/infrastructure/${service}.go && echo "✅ $service" || echo "❌ $service"
done

# 3. Run E2E tests (should not require manual setup)
make test-e2e-notification
```

---

## Related Documents
- [ADR-030: Configuration Management](ADR-030-CONFIGURATION-MANAGEMENT.md) - ConfigMap YAML format
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) - E2E test strategy
- [.cursor/rules/03-testing-strategy.mdc](.cursor/rules/03-testing-strategy.mdc) - Defense-in-depth testing

---

**Status**: ✅ **APPROVED - AUTHORITATIVE**
**Enforcement**: MANDATORY for all E2E tests
**Priority**: FOUNDATIONAL - All services MUST comply
**Next Review**: After 5+ services migrated to Pattern 2 (assess if Pattern 1 should be deprecated)

