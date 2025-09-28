package enhanced

import (
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/fake"
)

// SmartFakeClientConfig defines the configuration for smart fake client creation
type SmartFakeClientConfig struct {
	// TestType is automatically detected from call stack
	TestType TestType
	// Scenario override - if specified, uses this instead of auto-detection
	ScenarioOverride *ClusterScenario
	// ResourceProfile override - if specified, uses this instead of auto-detection
	ResourceProfileOverride *ResourceProfile
	// Logger for debugging scenario selection
	Logger *logrus.Logger
}

// TestType represents different types of tests for scenario selection
type TestType string

const (
	TestTypeUnit        TestType = "unit"
	TestTypeIntegration TestType = "integration"
	TestTypeE2E         TestType = "e2e"
	TestTypeSafety      TestType = "safety"
	TestTypePlatform    TestType = "platform"
	TestTypeWorkflow    TestType = "workflow"
	TestTypeAI          TestType = "ai"
	TestTypeUnknown     TestType = "unknown"
)

// NewSmartFakeClientset creates an enhanced fake clientset with automatic scenario selection
// This is a drop-in replacement for fake.NewSimpleClientset() with production fidelity
func NewSmartFakeClientset() *fake.Clientset {
	return NewSmartFakeClientsetWithConfig(nil)
}

// NewSmartFakeClientsetWithConfig creates an enhanced fake clientset with custom configuration
func NewSmartFakeClientsetWithConfig(config *SmartFakeClientConfig) *fake.Clientset {
	if config == nil {
		config = &SmartFakeClientConfig{}
	}

	// Auto-detect test type from call stack if not specified
	if config.TestType == "" {
		config.TestType = detectTestType()
	}

	// Create logger if not provided
	if config.Logger == nil {
		config.Logger = logrus.New()
		config.Logger.SetLevel(logrus.WarnLevel) // Reduce noise in tests
	}

	// Select appropriate scenario based on test type
	scenario := selectScenarioForTestType(config.TestType, config.ScenarioOverride)
	resourceProfile := selectResourceProfileForTestType(config.TestType, config.ResourceProfileOverride)

	// Log scenario selection for debugging
	config.Logger.WithFields(logrus.Fields{
		"test_type":        config.TestType,
		"scenario":         scenario,
		"resource_profile": resourceProfile,
		"auto_detected":    config.ScenarioOverride == nil,
	}).Debug("Smart fake client scenario selected")

	// Create enhanced cluster configuration
	clusterConfig := &ClusterConfig{
		Scenario:        scenario,
		NodeCount:       getNodeCountForTestType(config.TestType),
		Namespaces:      getNamespacesForTestType(config.TestType),
		WorkloadProfile: getWorkloadProfileForTestType(config.TestType),
		ResourceProfile: resourceProfile,
	}

	// Create production-like cluster with selected scenario
	return NewProductionLikeCluster(clusterConfig)
}

// detectTestType analyzes the call stack to determine the test type
func detectTestType() TestType {
	// Get call stack to analyze test file paths and function names
	for i := 2; i < 10; i++ { // Skip current function and immediate caller
		_, file, _, ok := runtime.Caller(i)
		if !ok {
			break
		}

		// Analyze file path patterns
		if strings.Contains(file, "/test/unit/") {
			return detectUnitTestSubtype(file)
		}
		if strings.Contains(file, "/test/integration/") {
			return TestTypeIntegration
		}
		if strings.Contains(file, "/test/e2e/") {
			return TestTypeE2E
		}
	}

	return TestTypeUnknown
}

// detectUnitTestSubtype analyzes unit test file paths for more specific test types
func detectUnitTestSubtype(filePath string) TestType {
	switch {
	case strings.Contains(filePath, "/platform/safety"):
		return TestTypeSafety
	case strings.Contains(filePath, "/platform/"):
		return TestTypePlatform
	case strings.Contains(filePath, "/workflow"):
		return TestTypeWorkflow
	case strings.Contains(filePath, "/ai/"):
		return TestTypeAI
	default:
		return TestTypeUnit
	}
}

// selectScenarioForTestType returns the optimal scenario for each test type
func selectScenarioForTestType(testType TestType, override *ClusterScenario) ClusterScenario {
	if override != nil {
		return *override
	}

	switch testType {
	case TestTypeUnit:
		return BasicDevelopment // Fast, minimal resources for unit tests
	case TestTypeSafety:
		return ResourceConstrained // Realistic constraints for safety validation
	case TestTypePlatform:
		return MonitoringStack // Platform tests benefit from monitoring components
	case TestTypeWorkflow:
		return HighLoadProduction // Workflow tests need realistic workloads
	case TestTypeAI:
		return HighLoadProduction // AI tests benefit from production-like resources
	case TestTypeIntegration:
		return HighLoadProduction // Integration tests need production-like scenarios
	case TestTypeE2E:
		return MultiTenantDevelopment // E2E tests benefit from complex multi-tenant scenarios
	default:
		return BasicDevelopment // Safe default for unknown test types
	}
}

// selectResourceProfileForTestType returns the optimal resource profile for each test type
func selectResourceProfileForTestType(testType TestType, override *ResourceProfile) ResourceProfile {
	if override != nil {
		return *override
	}

	switch testType {
	case TestTypeUnit:
		return DevelopmentResources // Minimal resources for fast unit tests
	case TestTypeSafety:
		return ProductionResourceLimits // Realistic limits for safety testing
	case TestTypePlatform, TestTypeWorkflow:
		return ProductionResourceLimits // Production-like resources
	case TestTypeAI:
		return GPUAcceleratedNodes // AI tests may benefit from GPU resources
	case TestTypeIntegration, TestTypeE2E:
		return ProductionResourceLimits // Production fidelity for integration tests
	default:
		return DevelopmentResources // Safe default
	}
}

// getNodeCountForTestType returns the optimal node count for each test type
func getNodeCountForTestType(testType TestType) int {
	switch testType {
	case TestTypeUnit:
		return 1 // Minimal for unit tests
	case TestTypeSafety:
		return 2 // Enough for safety constraint testing
	case TestTypePlatform, TestTypeWorkflow:
		return 3 // Good balance for platform/workflow tests
	case TestTypeAI:
		return 2 // AI tests don't need many nodes
	case TestTypeIntegration:
		return 3 // Production-like for integration
	case TestTypeE2E:
		return 5 // Complex scenarios for E2E
	default:
		return 1 // Safe minimal default
	}
}

// getNamespacesForTestType returns the optimal namespaces for each test type
func getNamespacesForTestType(testType TestType) []string {
	switch testType {
	case TestTypeUnit:
		return []string{"default"} // Simple for unit tests
	case TestTypeSafety:
		return []string{"default", "kube-system"} // System namespaces for safety testing
	case TestTypePlatform:
		return []string{"default", "kubernaut", "monitoring"} // Platform-specific
	case TestTypeWorkflow:
		return []string{"default", "kubernaut", "workflows"} // Workflow-specific
	case TestTypeAI:
		return []string{"default", "ai-ml", "kubernaut"} // AI-specific
	case TestTypeIntegration:
		return []string{"default", "kubernaut", "monitoring", "apps"} // Multi-namespace
	case TestTypeE2E:
		return []string{"default", "kubernaut", "monitoring", "apps", "infrastructure"} // Complex
	default:
		return []string{"default"} // Safe default
	}
}

// getWorkloadProfileForTestType returns the optimal workload profile for each test type
func getWorkloadProfileForTestType(testType TestType) WorkloadProfile {
	switch testType {
	case TestTypeUnit:
		return WebApplicationStack // Simple workloads for unit tests
	case TestTypeSafety:
		return KubernautOperator // Kubernaut-specific for safety testing
	case TestTypePlatform:
		return MonitoringWorkload // Platform tests benefit from monitoring workloads
	case TestTypeWorkflow:
		return HighThroughputServices // Workflow tests need realistic services
	case TestTypeAI:
		return AIMLWorkload // AI-specific workloads
	case TestTypeIntegration:
		return HighThroughputServices // Production-like services
	case TestTypeE2E:
		return WebApplicationStack // Complex application stack
	default:
		return WebApplicationStack // Safe default
	}
}

// GetScenarioInfo returns information about the scenario selection for debugging
func GetScenarioInfo(testType TestType) map[string]interface{} {
	return map[string]interface{}{
		"test_type":        testType,
		"scenario":         selectScenarioForTestType(testType, nil),
		"resource_profile": selectResourceProfileForTestType(testType, nil),
		"node_count":       getNodeCountForTestType(testType),
		"namespaces":       getNamespacesForTestType(testType),
		"workload_profile": getWorkloadProfileForTestType(testType),
	}
}
