# Dynamic Toolset E2E Test Implementation Plan V1.1

**Service**: Dynamic Toolset Service
**Test Tier**: End-to-End (E2E)
**Version**: V1.1
**Status**: 📋 PLANNING
**Created**: November 9, 2025
**Business Requirement**: BR-TOOLSET-041 (E2E Testing)

---

## 🎯 Overview

This plan defines the E2E test implementation for the Dynamic Toolset service, leveraging the existing Kind cluster infrastructure used by Gateway and Context API services. The E2E tests will deploy mock service pods to a Kind cluster and verify that the Dynamic Toolset service correctly discovers them and generates ConfigMaps with appropriate HolmesGPT toolset configurations.

### Goals

1. **Reuse Existing Infrastructure**: Leverage `test/infrastructure/` for Kind cluster management
2. **Test Real Discovery**: Deploy actual Kubernetes resources and verify discovery
3. **Validate ConfigMap Generation**: Ensure generated ConfigMaps match expected structure
4. **Test Update Scenarios**: Verify ConfigMap updates when services change
5. **Production-Like Environment**: Use Kind cluster similar to Gateway/Context API E2E tests

---

## 📋 Business Requirement

### BR-TOOLSET-041: End-to-End Testing

**Priority**: P1 (Production Readiness)
**Status**: 🔴 Missing
**Category**: Testing & Quality Assurance

**Description**: End-to-end tests that deploy mock service pods to a Kind cluster and verify Dynamic Toolset discovery and ConfigMap generation in a production-like environment.

**Acceptance Criteria**:
1. Deploy mock service pods with various annotations to Kind cluster
2. Verify Dynamic Toolset discovers services correctly
3. Validate generated ConfigMaps contain correct toolset definitions
4. Test ConfigMap updates when services are added/removed/modified
5. Verify namespace filtering works correctly
6. Test error handling for invalid service configurations

**Test Coverage**:
- E2E: `test/e2e/toolset/01_discovery_lifecycle_test.go` (6 scenarios)
- E2E: `test/e2e/toolset/02_configmap_updates_test.go` (4 scenarios)
- E2E: `test/e2e/toolset/03_namespace_filtering_test.go` (3 scenarios)

---

## 🏗️ Infrastructure Design

### Shared Infrastructure Approach

**Rationale**: Gateway and Context API already have robust Kind cluster infrastructure in `test/infrastructure/`. We will create a new `toolset.go` file following the same pattern, enabling infrastructure reuse while maintaining service-specific logic.

### Infrastructure Files

```
test/infrastructure/
├── gateway.go          # Gateway E2E infrastructure (existing)
├── contextapi.go       # Context API E2E infrastructure (existing)
├── datastorage.go      # Data Storage infrastructure (existing)
└── toolset.go          # NEW: Dynamic Toolset E2E infrastructure
```

### Key Functions (toolset.go)

```go
// CreateToolsetCluster creates a Kind cluster for Dynamic Toolset E2E testing
// Reuses Kind cluster creation logic from gateway.go
func CreateToolsetCluster(clusterName, kubeconfigPath string, writer io.Writer) error

// DeployToolsetTestServices deploys Dynamic Toolset and mock services in a namespace
// Steps:
// 1. Create namespace
// 2. Build and load Dynamic Toolset image
// 3. Deploy Dynamic Toolset service
// 4. Deploy mock service pods with annotations
// 5. Wait for all services ready
func DeployToolsetTestServices(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error

// DeployMockServices deploys mock service pods for discovery testing
// Creates pods with various annotations:
// - holmesgpt.io/enabled: "true"
// - holmesgpt.io/tools: "logs,metrics,describe"
// - holmesgpt.io/namespace: "test-namespace"
func DeployMockServices(namespace, kubeconfigPath string, mockServices []MockServiceConfig, writer io.Writer) error

// CleanupToolsetNamespace deletes a test namespace and all resources
func CleanupToolsetNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error

// DeleteToolsetCluster deletes the Kind cluster
func DeleteToolsetCluster(clusterName, kubeconfigPath string, writer io.Writer) error
```

---

## 🧪 Test Structure

### Test Suite Organization

```
test/e2e/toolset/
├── toolset_e2e_suite_test.go           # Suite setup (cluster creation ONCE)
├── 01_discovery_lifecycle_test.go       # Service discovery scenarios
├── 02_configmap_updates_test.go         # ConfigMap update scenarios
├── 03_namespace_filtering_test.go       # Namespace filtering scenarios
└── mock-service-template.yaml           # Template for mock services
```

### Suite Setup Pattern (Following Gateway)

```go
// toolset_e2e_suite_test.go
package toolset

import (
	"context"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

func TestToolsetE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dynamic Toolset E2E Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger *zap.Logger

	// Cluster configuration (shared across all tests)
	clusterName    string
	kubeconfigPath string

	// Track if any test failed (for cluster cleanup decision)
	anyTestFailed bool
)

var _ = BeforeSuite(func() {
	// Initialize context
	ctx, cancel = context.WithCancel(context.Background())

	// Initialize logger
	var err error
	logger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred())

	// Initialize failure tracking
	anyTestFailed = false

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Dynamic Toolset E2E Test Suite - Cluster Setup (ONCE)")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Creating Kind cluster for all E2E tests...")
	logger.Info("  • Kind cluster (2 nodes: control-plane + worker)")
	logger.Info("  • Dynamic Toolset Docker image (build + load)")
	logger.Info("  • Kubeconfig: ~/.kube/kind-config-toolset")
	logger.Info("")
	logger.Info("Note: Each test will deploy services in a unique namespace")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Set cluster configuration
	clusterName = "toolset-e2e"
	homeDir, err := os.UserHomeDir()
	Expect(err).ToNot(HaveOccurred())
	kubeconfigPath = fmt.Sprintf("%s/.kube/kind-config-toolset", homeDir)

	// Create Kind cluster (ONCE for all tests)
	err = infrastructure.CreateToolsetCluster(clusterName, kubeconfigPath, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	// Set KUBECONFIG environment variable
	err = os.Setenv("KUBECONFIG", kubeconfigPath)
	Expect(err).ToNot(HaveOccurred())

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Cluster Setup Complete - Tests can now deploy services per-namespace")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info(fmt.Sprintf("  • Cluster: %s", clusterName))
	logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

// Track test failures for cluster cleanup decision
var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
	}
})

var _ = AfterSuite(func() {
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Dynamic Toolset E2E Test Suite - Cluster Teardown")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Check if any test failed - preserve cluster for debugging
	if anyTestFailed || os.Getenv("SKIP_CLEANUP") == "true" {
		logger.Warn("⚠️  Test FAILED - Keeping cluster alive for debugging")
		logger.Info("To debug:")
		logger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
		logger.Info("  kubectl get namespaces | grep toolset-test")
		logger.Info("  kubectl get pods -n <namespace>")
		logger.Info("  kubectl logs -n <namespace> deployment/dynamic-toolset")
		logger.Info("  kubectl get configmaps -n <namespace>")
		logger.Info("To cleanup manually:")
		logger.Info(fmt.Sprintf("  kind delete cluster --name %s", clusterName))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		return
	}

	// All tests passed - cleanup cluster
	logger.Info("✅ All tests passed - cleaning up cluster...")
	err := infrastructure.DeleteToolsetCluster(clusterName, kubeconfigPath, GinkgoWriter)
	if err != nil {
		logger.Warn("Failed to delete cluster", zap.Error(err))
	}

	// Cancel context
	if cancel != nil {
		cancel()
	}

	// Sync logger
	if logger != nil {
		_ = logger.Sync()
	}

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Cluster Teardown Complete")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})
```

---

## 📝 Test Scenarios

### Test File 1: Discovery Lifecycle (01_discovery_lifecycle_test.go)

**Purpose**: Verify Dynamic Toolset discovers services and generates ConfigMaps correctly

#### Scenario 1: Discover Single Service with Basic Annotations
- **Setup**: Deploy 1 mock service with `holmesgpt.io/enabled: "true"`
- **Action**: Wait for Dynamic Toolset to discover service
- **Verify**: ConfigMap created with correct toolset definition
- **Priority**: P0

#### Scenario 2: Discover Multiple Services in Same Namespace
- **Setup**: Deploy 3 mock services with different tool annotations
- **Action**: Wait for discovery
- **Verify**: ConfigMap contains all 3 services with correct tools
- **Priority**: P0

#### Scenario 3: Ignore Services Without Annotations
- **Setup**: Deploy 2 services (1 with annotations, 1 without)
- **Action**: Wait for discovery
- **Verify**: ConfigMap only contains annotated service
- **Priority**: P1

#### Scenario 4: Handle Service with Custom Tools
- **Setup**: Deploy service with `holmesgpt.io/tools: "logs,metrics,describe,events"`
- **Action**: Wait for discovery
- **Verify**: ConfigMap contains all 4 tools
- **Priority**: P1

#### Scenario 5: Discover Service with Namespace Annotation
- **Setup**: Deploy service with `holmesgpt.io/namespace: "custom-ns"`
- **Action**: Wait for discovery
- **Verify**: ConfigMap toolset scoped to custom-ns
- **Priority**: P1

#### Scenario 6: Handle Service Deletion
- **Setup**: Deploy service, wait for ConfigMap creation, delete service
- **Action**: Wait for ConfigMap update
- **Verify**: ConfigMap no longer contains deleted service
- **Priority**: P0

---

### Test File 2: ConfigMap Updates (02_configmap_updates_test.go)

**Purpose**: Verify Dynamic Toolset updates ConfigMaps when services change

#### Scenario 1: Add New Service to Existing ConfigMap
- **Setup**: Deploy service A, wait for ConfigMap, deploy service B
- **Action**: Wait for ConfigMap update
- **Verify**: ConfigMap contains both services
- **Priority**: P0

#### Scenario 2: Update Service Annotations
- **Setup**: Deploy service with tools "logs,metrics"
- **Action**: Update service annotations to "logs,metrics,describe"
- **Verify**: ConfigMap reflects new tools
- **Priority**: P1

#### Scenario 3: Remove Service from ConfigMap
- **Setup**: Deploy 2 services, wait for ConfigMap
- **Action**: Delete 1 service
- **Verify**: ConfigMap only contains remaining service
- **Priority**: P0

#### Scenario 4: ConfigMap Regeneration After Manual Deletion
- **Setup**: Deploy service, wait for ConfigMap
- **Action**: Manually delete ConfigMap
- **Verify**: Dynamic Toolset recreates ConfigMap
- **Priority**: P1

---

### Test File 3: Namespace Filtering (03_namespace_filtering_test.go)

**Purpose**: Verify Dynamic Toolset respects namespace filtering configuration

#### Scenario 1: Discover Services in Watched Namespace Only
- **Setup**: Configure Dynamic Toolset to watch namespace "test-ns"
- **Action**: Deploy services in "test-ns" and "other-ns"
- **Verify**: ConfigMap only contains services from "test-ns"
- **Priority**: P1

#### Scenario 2: Discover Services Across All Namespaces
- **Setup**: Configure Dynamic Toolset to watch all namespaces
- **Action**: Deploy services in multiple namespaces
- **Verify**: ConfigMap contains services from all namespaces
- **Priority**: P2

#### Scenario 3: Ignore System Namespaces
- **Setup**: Deploy services in "kube-system" and "test-ns"
- **Action**: Wait for discovery
- **Verify**: ConfigMap only contains services from "test-ns"
- **Priority**: P1

---

## 🔧 Mock Service Configuration

### Mock Service Template (mock-service-template.yaml)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: {{SERVICE_NAME}}
  namespace: {{NAMESPACE}}
  annotations:
    holmesgpt.io/enabled: "{{ENABLED}}"
    holmesgpt.io/tools: "{{TOOLS}}"
    holmesgpt.io/namespace: "{{TARGET_NAMESPACE}}"
spec:
  selector:
    app: {{SERVICE_NAME}}
  ports:
    - protocol: TCP
      port: 80
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{SERVICE_NAME}}
  namespace: {{NAMESPACE}}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {{SERVICE_NAME}}
  template:
    metadata:
      labels:
        app: {{SERVICE_NAME}}
    spec:
      containers:
      - name: mock-service
        image: nginx:alpine
        ports:
        - containerPort: 8080
```

### Mock Service Configurations

```go
type MockServiceConfig struct {
	Name            string
	Namespace       string
	Enabled         bool
	Tools           []string
	TargetNamespace string
}

var mockServiceConfigs = []MockServiceConfig{
	{
		Name:            "mock-service-basic",
		Namespace:       "test-ns",
		Enabled:         true,
		Tools:           []string{"logs", "metrics"},
		TargetNamespace: "",
	},
	{
		Name:            "mock-service-full",
		Namespace:       "test-ns",
		Enabled:         true,
		Tools:           []string{"logs", "metrics", "describe", "events"},
		TargetNamespace: "custom-ns",
	},
	{
		Name:            "mock-service-disabled",
		Namespace:       "test-ns",
		Enabled:         false,
		Tools:           []string{},
		TargetNamespace: "",
	},
}
```

---

## 📊 Expected ConfigMap Structure

### Generated ConfigMap Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: holmesgpt-toolset
  namespace: test-ns
  labels:
    app: dynamic-toolset
    generated-by: kubernaut
data:
  toolset.yaml: |
    tools:
      - name: kubectl_logs
        description: "Get logs from mock-service-basic pods"
        command: "kubectl logs -n test-ns -l app=mock-service-basic --tail=100"

      - name: kubectl_metrics
        description: "Get metrics from mock-service-basic"
        command: "kubectl top pods -n test-ns -l app=mock-service-basic"

      - name: kubectl_logs_custom
        description: "Get logs from mock-service-full pods in custom-ns"
        command: "kubectl logs -n custom-ns -l app=mock-service-full --tail=100"

      - name: kubectl_metrics_custom
        description: "Get metrics from mock-service-full in custom-ns"
        command: "kubectl top pods -n custom-ns -l app=mock-service-full"

      - name: kubectl_describe_custom
        description: "Describe mock-service-full pods in custom-ns"
        command: "kubectl describe pods -n custom-ns -l app=mock-service-full"

      - name: kubectl_events_custom
        description: "Get events for mock-service-full in custom-ns"
        command: "kubectl get events -n custom-ns --field-selector involvedObject.name=mock-service-full"
```

---

## 🎯 Validation Criteria

### Test Behavior and Correctness

**Critical Principle**: Tests must validate **behavior and correctness**, not just existence or structure.

**Reference**: As identified during RFC 7807/Graceful Shutdown implementation:
> "When I said test for behavior and correctness I meant the tests, not the implementation logic. You should triage the existing test code."

**What This Means for E2E Tests**:
- ✅ **Validate Business Outcomes**: Does the generated ConfigMap enable HolmesGPT to troubleshoot the service?
- ✅ **Verify Functional Correctness**: Do the kubectl commands actually work when executed?
- ✅ **Test Real Scenarios**: Deploy actual services and verify discovery works end-to-end
- ❌ **Don't Just Check Existence**: Finding a ConfigMap is not enough
- ❌ **Don't Just Validate Structure**: YAML format alone doesn't prove correctness
- ❌ **Don't Mock Discovery**: E2E tests must use real Kubernetes resources

### ConfigMap Validation

For each test scenario, verify **behavior and correctness**:

#### 1. **Business Outcome Validation** (Primary)
- **Can HolmesGPT use this ConfigMap?** Execute generated commands to verify they work
- **Do commands return expected data?** Verify kubectl commands produce valid output
- **Are service selectors correct?** Commands target the right pods/services
- **Is namespace scoping accurate?** Commands execute in correct namespaces

#### 2. **Functional Correctness** (Secondary)
- **ConfigMap Existence**: ConfigMap created in correct namespace
- **ConfigMap Labels**: Contains `app: dynamic-toolset` and `generated-by: kubernaut`
- **Tool Count**: Number of tools matches expected count based on service annotations
- **Tool Names**: Tool names follow convention `kubectl_{tool}_{service}`
- **Tool Commands**: Commands reference correct namespace and service labels
- **Tool Descriptions**: Descriptions are human-readable and accurate
- **YAML Structure**: Valid YAML format with proper indentation

#### 3. **Behavioral Validation Examples**

**Example 1: Verify kubectl logs command works**
```go
// ❌ WRONG: Just check the command exists
Expect(toolset.Tools[0].Command).To(ContainSubstring("kubectl logs"))

// ✅ CORRECT: Execute the command and verify it returns logs
cmd := exec.Command("sh", "-c", toolset.Tools[0].Command)
cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
output, err := cmd.CombinedOutput()
Expect(err).ToNot(HaveOccurred())
Expect(string(output)).To(ContainSubstring("nginx"))  // Verify actual log content
```

**Example 2: Verify service selector targets correct pods**
```go
// ❌ WRONG: Just check the selector exists
Expect(toolset.Tools[0].Command).To(ContainSubstring("-l app=mock-service"))

// ✅ CORRECT: Verify the selector actually matches deployed pods
cmd := exec.Command("kubectl", "get", "pods", "-n", namespace, "-l", "app=mock-service", "-o", "json")
cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
output, err := cmd.CombinedOutput()
Expect(err).ToNot(HaveOccurred())

var podList struct {
    Items []interface{} `json:"items"`
}
json.Unmarshal(output, &podList)
Expect(len(podList.Items)).To(BeNumerically(">", 0))  // Selector matches actual pods
```

**Example 3: Verify namespace scoping is correct**
```go
// ❌ WRONG: Just check namespace is in the command
Expect(toolset.Tools[0].Command).To(ContainSubstring("-n custom-ns"))

// ✅ CORRECT: Verify command executes in correct namespace and returns expected data
cmd := exec.Command("sh", "-c", toolset.Tools[0].Command)
cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
output, err := cmd.CombinedOutput()
Expect(err).ToNot(HaveOccurred())

// Verify output is from correct namespace (not default or wrong namespace)
Expect(string(output)).To(ContainSubstring("custom-ns"))
Expect(string(output)).ToNot(ContainSubstring("default"))
```

### Helper Functions

```go
// ValidateConfigMap verifies ConfigMap structure and content
func ValidateConfigMap(cm *corev1.ConfigMap, expectedServices []MockServiceConfig) error {
	// Verify labels
	if cm.Labels["app"] != "dynamic-toolset" {
		return fmt.Errorf("missing app label")
	}

	// Parse toolset.yaml
	toolsetYAML := cm.Data["toolset.yaml"]
	var toolset struct {
		Tools []struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
			Command     string `yaml:"command"`
		} `yaml:"tools"`
	}
	if err := yaml.Unmarshal([]byte(toolsetYAML), &toolset); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	// Verify tool count
	expectedToolCount := calculateExpectedToolCount(expectedServices)
	if len(toolset.Tools) != expectedToolCount {
		return fmt.Errorf("expected %d tools, got %d", expectedToolCount, len(toolset.Tools))
	}

	// Verify each tool
	for _, service := range expectedServices {
		if !service.Enabled {
			continue
		}
		for _, tool := range service.Tools {
			toolName := fmt.Sprintf("kubectl_%s_%s", tool, service.Name)
			if !containsTool(toolset.Tools, toolName) {
				return fmt.Errorf("missing tool: %s", toolName)
			}
		}
	}

	return nil
}
```

---

## ⏱️ Implementation Timeline

### Phase 1: Infrastructure Setup (2-3 hours)
1. Create `test/infrastructure/toolset.go` (1 hour)
2. Create `test/e2e/toolset/toolset_e2e_suite_test.go` (30 min)
3. Create mock service template YAML (30 min)
4. Test cluster creation and teardown (1 hour)

### Phase 2: Discovery Lifecycle Tests (3-4 hours)
1. Implement Scenario 1-3 (1.5 hours)
2. Implement Scenario 4-6 (1.5 hours)
3. Add helper functions for validation (1 hour)

### Phase 3: ConfigMap Update Tests (2-3 hours)
1. Implement Scenario 1-2 (1 hour)
2. Implement Scenario 3-4 (1 hour)
3. Test and debug (1 hour)

### Phase 4: Namespace Filtering Tests (2 hours)
1. Implement Scenario 1-2 (1 hour)
2. Implement Scenario 3 (30 min)
3. Test and debug (30 min)

### Phase 5: Documentation and BR Update (1 hour)
1. Update BR-TOOLSET-041 in BUSINESS_REQUIREMENTS.md
2. Document E2E test coverage
3. Update README.md test statistics

**Total Estimated Effort**: 10-13 hours

---

## 🔗 Related Documentation

- **Gateway E2E Tests**: `test/e2e/gateway/` - Reference implementation
- **Context API E2E Tests**: `test/e2e/contextapi/` - Reference implementation
- **Infrastructure Code**: `test/infrastructure/gateway.go` - Reusable patterns
- **BR-TOOLSET-041**: Business requirement for E2E testing
- **Kind Documentation**: https://kind.sigs.k8s.io/

---

## ✅ Success Criteria

1. **All 13 E2E scenarios passing** (6 discovery + 4 updates + 3 filtering)
2. **ConfigMap validation** for all scenarios
3. **Infrastructure reuse** from Gateway/Context API
4. **Cluster cleanup** working correctly
5. **Documentation updated** with E2E test coverage
6. **CI/CD ready** - tests can run in automated pipelines

---

## 🚀 Next Steps

1. **Review this plan** with stakeholders
2. **Approve implementation approach**
3. **Begin Phase 1** (Infrastructure Setup)
4. **Implement tests following TDD** (RED-GREEN-REFACTOR)
5. **Update BR documentation** after completion

---

**Document Version**: 1.0
**Last Updated**: November 9, 2025
**Status**: 📋 Awaiting Approval
**Maintained By**: Kubernaut Architecture Team

