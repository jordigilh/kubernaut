//go:build unit
// +build unit

package platform

import (
	"testing"
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/platform/safety"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"
)

// BR-SAFE-REAL-001: Real Safety Validator Component Testing
// Business Impact: Ensures actual safety validation logic works correctly in production scenarios
// Stakeholder Value: Operations teams get confidence in real safety mechanisms

var _ = Describe("Real Safety Validator - Phase 1 Implementation", func() {
	var (
		ctx               context.Context
		realValidator     *safety.SafetyValidator
		enhancedK8sClient *fake.Clientset
		logger            *logrus.Logger
		performanceStart  time.Time
		performanceTarget time.Duration = 100 * time.Millisecond // Phase 1 requirement: <100ms
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise

		// Create enhanced fake Kubernetes client with ResourceConstrained scenario
		// Auto-detects safety test type and provides realistic resource constraints
		enhancedK8sClient = enhanced.NewSmartFakeClientset()

		// Create REAL SafetyValidator instance - Phase 1 core requirement
		realValidator = safety.NewSafetyValidator(enhancedK8sClient, logger)

		// Setup additional test cluster resources if needed
		setupTestClusterResources(ctx, enhancedK8sClient)

		// Start performance monitoring for Phase 1 compliance
		performanceStart = time.Now()
	})

	AfterEach(func() {
		// Verify Phase 1 performance requirement: <100ms execution time
		elapsed := time.Since(performanceStart)
		if elapsed > performanceTarget {
			logger.WithFields(logrus.Fields{
				"elapsed":          elapsed,
				"target":           performanceTarget,
				"phase1_violation": true,
			}).Warn("Phase 1 performance target exceeded")
		}
	})

	Context("BR-SAFE-001: Real Cluster Connectivity Validation", func() {
		It("should validate cluster access using real business logic", func() {
			// Business Scenario: Operations team needs to validate cluster access before executing actions
			// Phase 1 Requirement: Use REAL SafetyValidator instead of mocks

			startTime := time.Now()

			// Execute REAL business logic - not mocked behavior
			result := realValidator.ValidateClusterAccess(ctx, "production")

			// Performance validation for Phase 1: <100ms
			executionTime := time.Since(startTime)
			Expect(executionTime).To(BeNumerically("<", performanceTarget),
				"BR-SAFE-001: Real safety validation must meet <100ms performance requirement")

			// Business validation using REAL component
			Expect(result).ToNot(BeNil(),
				"BR-SAFE-001: Real validator should return cluster validation result")
			Expect(result.IsValid).To(BeTrue(),
				"BR-SAFE-001: Real validator should validate cluster connectivity successfully")
			Expect(result.ConnectivityCheck).To(BeTrue(),
				"BR-SAFE-001: Real validator should confirm cluster connectivity")
			Expect(result.PermissionLevel).To(BeElementOf([]string{"admin", "read-only", "cluster-admin"}),
				"BR-SAFE-001: Real validator should determine actual permission level")
			Expect(result.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}),
				"BR-SAFE-001: Real validator should assess actual risk level")
		})

		It("should handle real connectivity failures gracefully", func() {
			// Business Scenario: Test real error handling without mocked behavior
			// Phase 1 Requirement: Real component error handling

			// Use canceled context to simulate real connectivity issues
			canceledCtx, cancel := context.WithCancel(ctx)
			cancel() // Immediately cancel to simulate connection failure

			startTime := time.Now()
			result := realValidator.ValidateClusterAccess(canceledCtx, "unreachable-namespace")
			executionTime := time.Since(startTime)

			// Performance validation even for error cases
			Expect(executionTime).To(BeNumerically("<", performanceTarget),
				"BR-SAFE-001: Real validator error handling must be performant")

			// Real error handling validation
			Expect(result).ToNot(BeNil(),
				"BR-SAFE-001: Real validator should handle errors gracefully")
			Expect(result.IsValid).To(BeFalse(),
				"BR-SAFE-001: Real validator should correctly identify connectivity failures")
			Expect(result.RiskLevel).To(BeElementOf([]string{"HIGH", "CRITICAL"}),
				"BR-SAFE-001: Real validator should escalate risk for connectivity failures")
		})
	})

	Context("BR-SAFE-002: Real Resource State Validation", func() {
		It("should validate actual Kubernetes resource states", func() {
			// Business Scenario: Validate real resource state before taking actions
			// Phase 1 Requirement: Test actual business logic for resource validation

			alert := types.Alert{
				Name:      "HighCPUUsage",
				Severity:  "warning",
				Namespace: "production",
				Resource:  "web-app",
			}

			startTime := time.Now()

			// Execute REAL resource validation business logic
			result := realValidator.ValidateResourceState(ctx, alert)

			// Performance validation
			executionTime := time.Since(startTime)
			Expect(executionTime).To(BeNumerically("<", performanceTarget),
				"BR-SAFE-002: Real resource validation must meet performance requirements")

			// Real business logic validation
			Expect(result).ToNot(BeNil(),
				"BR-SAFE-002: Real validator should return resource validation result")
			Expect(result.IsValid).To(BeTrue(),
				"BR-SAFE-002: Real validator should validate existing resource state")
			Expect(result.ResourceExists).To(BeTrue(),
				"BR-SAFE-002: Real validator should detect existing deployment resource")
			Expect(result.CurrentState).To(BeElementOf([]string{"Available", "Ready", "NotReady"}),
				"BR-SAFE-002: Real validator should determine actual resource state")
			Expect(result.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM", "HIGH"}),
				"BR-SAFE-002: Real validator should assess resource-specific risk")
		})

		It("should handle non-existent resources with real validation logic", func() {
			// Business Scenario: Real handling of missing resources
			// Phase 1 Requirement: Actual business logic for resource not found scenarios

			alert := types.Alert{
				Name:      "MissingResource",
				Severity:  "critical",
				Namespace: "production",
				Resource:  "non-existent-service",
			}

			startTime := time.Now()
			result := realValidator.ValidateResourceState(ctx, alert)
			executionTime := time.Since(startTime)

			// Performance validation for error cases
			Expect(executionTime).To(BeNumerically("<", performanceTarget),
				"BR-SAFE-002: Real validator must handle missing resources efficiently")

			// Real error case handling
			Expect(result).ToNot(BeNil(),
				"BR-SAFE-002: Real validator should handle missing resources gracefully")
			Expect(result.ResourceExists).To(BeFalse(),
				"BR-SAFE-002: Real validator should correctly identify missing resources")
			Expect(result.ErrorMessage).To(ContainSubstring("not found"),
				"BR-SAFE-002: Real validator should provide specific error information")
		})
	})

	Context("BR-SAFE-003: Real Risk Assessment Logic", func() {
		It("should perform actual risk assessment for different action types", func() {
			// Business Scenario: Real risk assessment for operational decisions
			// Phase 1 Requirement: Actual business algorithms for risk calculation

			testCases := []struct {
				action       string
				severity     string
				expectedRisk string
				shouldAllow  bool
			}{
				{"restart_pod", "warning", "LOW", true},
				{"scale_deployment", "info", "LOW", true},
				{"drain_node", "warning", "HIGH", false},
				{"delete_pod", "critical", "MEDIUM", true}, // Critical allows higher risk
			}

			for _, tc := range testCases {
				action := types.ActionRecommendation{
					Action:     tc.action,
					Confidence: 0.8,
				}

				alert := types.Alert{
					Name:      "TestAlert",
					Severity:  tc.severity,
					Namespace: "production",
					Resource:  "test-resource",
				}

				startTime := time.Now()

				// Execute REAL risk assessment business logic
				assessment := realValidator.AssessRisk(ctx, action, alert)

				// Performance validation per action
				executionTime := time.Since(startTime)
				Expect(executionTime).To(BeNumerically("<", performanceTarget),
					"BR-SAFE-003: Real risk assessment must be performant for action %s", tc.action)

				// Real business logic validation
				Expect(assessment).ToNot(BeNil(),
					"BR-SAFE-003: Real validator should return risk assessment for %s", tc.action)
				Expect(assessment.ActionName).To(Equal(tc.action),
					"BR-SAFE-003: Real assessment should track correct action name")
				Expect(assessment.RiskLevel).To(Equal(tc.expectedRisk),
					"BR-SAFE-003: Real risk assessment should calculate expected risk for %s", tc.action)
				Expect(assessment.SafeToExecute).To(Equal(tc.shouldAllow),
					"BR-SAFE-003: Real assessment should make correct safety decisions for %s", tc.action)
				Expect(assessment.Confidence).To(BeNumerically(">", 0.5),
					"BR-SAFE-003: Real assessment should provide meaningful confidence levels")
				Expect(assessment.RiskFactors).ToNot(BeEmpty(),
					"BR-SAFE-003: Real assessment should identify specific risk factors")
				Expect(assessment.Mitigation).ToNot(BeEmpty(),
					"BR-SAFE-003: Real assessment should provide mitigation strategies")
			}
		})
	})

	Context("BR-SAFE-PERF-001: Phase 1 Performance Requirements", func() {
		It("should meet <100ms performance target for all validation operations", func() {
			// Business Scenario: Performance requirements for production safety operations
			// Phase 1 Requirement: All operations must complete within 100ms

			alert := types.Alert{
				Name:      "PerformanceTest",
				Severity:  "warning",
				Namespace: "production",
				Resource:  "web-app",
			}

			action := types.ActionRecommendation{
				Action:     "restart_pod",
				Confidence: 0.9,
			}

			// Test cluster access validation performance
			startTime := time.Now()
			clusterResult := realValidator.ValidateClusterAccess(ctx, alert.Namespace)
			clusterTime := time.Since(startTime)

			// Test resource state validation performance
			startTime = time.Now()
			resourceResult := realValidator.ValidateResourceState(ctx, alert)
			resourceTime := time.Since(startTime)

			// Test risk assessment performance
			startTime = time.Now()
			riskResult := realValidator.AssessRisk(ctx, action, alert)
			riskTime := time.Since(startTime)

			// Phase 1 Performance Assertions
			Expect(clusterTime).To(BeNumerically("<", performanceTarget),
				"BR-SAFE-PERF-001: Cluster validation must complete within 100ms")
			Expect(resourceTime).To(BeNumerically("<", performanceTarget),
				"BR-SAFE-PERF-001: Resource validation must complete within 100ms")
			Expect(riskTime).To(BeNumerically("<", performanceTarget),
				"BR-SAFE-PERF-001: Risk assessment must complete within 100ms")

			// Verify results are still valid despite performance constraints
			Expect(clusterResult.IsValid).To(BeTrue(),
				"BR-SAFE-PERF-001: Performance optimization should not compromise validation accuracy")
			Expect(resourceResult.IsValid).To(BeTrue(),
				"BR-SAFE-PERF-001: Performance optimization should not compromise resource detection")
			Expect(riskResult.SafeToExecute).To(BeTrue(),
				"BR-SAFE-PERF-001: Performance optimization should not compromise risk assessment")

			logger.WithFields(logrus.Fields{
				"cluster_validation_time":  clusterTime,
				"resource_validation_time": resourceTime,
				"risk_assessment_time":     riskTime,
				"performance_target":       performanceTarget,
				"phase1_compliance":        true,
			}).Info("Phase 1 performance requirements validated")
		})
	})

	Context("BR-SAFE-HEALTH-001: Real Component Health Monitoring", func() {
		It("should provide accurate health status using real validator", func() {
			// Business Scenario: Health monitoring for operational reliability
			// Phase 1 Requirement: Real health checks instead of mocked responses

			startTime := time.Now()

			// Execute REAL health check business logic
			err := realValidator.IsHealthy(ctx)

			// Performance validation for health checks
			executionTime := time.Since(startTime)
			Expect(executionTime).To(BeNumerically("<", performanceTarget),
				"BR-SAFE-HEALTH-001: Health checks must be performant")

			// Real health validation
			Expect(err).ToNot(HaveOccurred(),
				"BR-SAFE-HEALTH-001: Real validator should report healthy status with functioning cluster")

			logger.WithFields(logrus.Fields{
				"health_check_time":  executionTime,
				"performance_target": performanceTarget,
				"real_component":     true,
			}).Info("Real safety validator health check completed")
		})
	})
})

// setupTestClusterResources creates test Kubernetes resources for realistic testing
func setupTestClusterResources(ctx context.Context, client *fake.Clientset) {
	// Create test namespace
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "production",
		},
	}
	_, _ = client.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})

	// Create test deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-app",
			Namespace: "production",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: testInt32Ptr(3),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "web-app",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "web-app",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "web",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 3,
			Replicas:      3,
		},
	}
	_, _ = client.AppsV1().Deployments("production").Create(ctx, deployment, metav1.CreateOptions{})

	// Create test pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "production",
			Labels: map[string]string{
				"app": "web-app",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "web",
					Image: "nginx:latest",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	_, _ = client.CoreV1().Pods("production").Create(ctx, pod, metav1.CreateOptions{})

	// Create test service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-service",
			Namespace: "production",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "web-app",
			},
			Ports: []corev1.ServicePort{
				{
					Port:     80,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}
	_, _ = client.CoreV1().Services("production").Create(ctx, service, metav1.CreateOptions{})
}

// Helper function for int32 pointer
func testInt32Ptr(i int32) *int32 {
	return &i
}

// TestRunner bootstraps the Ginkgo test suite
func TestUsafetyUvalidatorUreal(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UsafetyUvalidatorUreal Suite")
}
