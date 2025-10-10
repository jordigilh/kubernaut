//go:build unit
// +build unit

<<<<<<< HEAD
package platform

import (
	"testing"
	"context"
=======
/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package platform

import (
	"context"
	"testing"
>>>>>>> crd_implementation
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/platform/safety"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/fake"
)

// BR-SAFE-001 through BR-SAFE-010: Kubernetes Safety Unit Testing - Pyramid Testing (70% Unit Coverage)
// Business Impact: Validates Kubernetes safety capabilities for production-safe cluster operations
// Stakeholder Value: Operations teams can trust safety-validated Kubernetes automation
var _ = Describe("BR-SAFE-001 through BR-SAFE-010: Kubernetes Safety Unit Testing", func() {
	var (
		// Use REAL business logic components per pyramid principles
		safetyValidator *safety.SafetyValidator

		// Mock ONLY external dependencies (Kubernetes API)
		fakeK8sClient *fake.Clientset
		mockLogger    *logrus.Logger

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only (Kubernetes API)
		fakeK8sClient = enhanced.NewSmartFakeClientset()
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create REAL business logic component for safety testing
		// Mock only external dependencies, use real business logic
		safetyValidator = safety.NewSafetyValidator(fakeK8sClient, mockLogger)
	})

	AfterEach(func() {
		cancel()
	})

	// BR-SAFE-001: Cluster Connectivity Validation
	Context("BR-SAFE-001: Cluster Connectivity Validation", func() {
		It("should validate cluster access and permissions successfully", func() {
			// Business Scenario: System validates cluster connectivity before executing actions
			// Business Impact: Prevents action failures due to connectivity or permission issues

			// Setup fake cluster with accessible namespace
			namespace := "production"

			// Test REAL business logic for cluster access validation
			result := safetyValidator.ValidateClusterAccess(ctx, namespace)

			// Validate REAL business cluster validation outcomes
			Expect(result).ToNot(BeNil(),
				"BR-SAFE-001: Cluster access validation must produce results")
			Expect(result.IsValid).To(BeTrue(),
				"BR-SAFE-001: Cluster access validation must succeed with proper setup")
			Expect(result.ConnectivityCheck).To(BeTrue(),
				"BR-SAFE-001: Connectivity check must pass with accessible cluster")
			Expect(result.PermissionLevel).ToNot(BeEmpty(),
				"BR-SAFE-001: Must determine permission level for business authorization")
			Expect(result.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}),
				"BR-SAFE-001: Risk level must be business-meaningful")

			// Business Value: Cluster validation prevents execution failures
		})

		It("should handle cluster connectivity failures gracefully", func() {
			// Business Scenario: System handles cluster connectivity issues gracefully
			// Business Impact: Provides clear error messages for troubleshooting

			// Test with non-existent namespace to simulate connectivity issues

			// Test REAL business logic for connectivity failure handling
			result := safetyValidator.ValidateClusterAccess(ctx, "test-namespace")

			// Validate graceful failure handling
			Expect(result).ToNot(BeNil(),
				"BR-SAFE-001: Must handle connectivity failures gracefully")
			Expect(result.IsValid).To(BeFalse(),
				"BR-SAFE-001: Must indicate validation failure on connectivity issues")
			Expect(result.ConnectivityCheck).To(BeFalse(),
				"BR-SAFE-001: Connectivity check must fail when cluster is unreachable")
			Expect(result.ErrorMessage).ToNot(BeEmpty(),
				"BR-SAFE-001: Must provide error message for troubleshooting")
			Expect(result.RiskLevel).To(Equal("CRITICAL"),
				"BR-SAFE-001: Connectivity failure must be marked as critical risk")

			// Business Value: Clear error handling enables quick troubleshooting
		})

		It("should validate namespace access permissions correctly", func() {
			// Business Scenario: System validates namespace-specific permissions
			// Business Impact: Ensures actions are only attempted with sufficient permissions

			// Setup fake cluster with restricted namespace access
			restrictedNamespace := "restricted-namespace"

			// Test REAL business logic for namespace permission validation
			result := safetyValidator.ValidateClusterAccess(ctx, restrictedNamespace)

			// Validate namespace permission assessment
			Expect(result).ToNot(BeNil(),
				"BR-SAFE-001: Namespace permission validation must produce results")
			Expect(result.IsValid).To(BeTrue(),
				"BR-SAFE-001: Namespace access validation must succeed when namespace exists")
			Expect(result.PermissionLevel).ToNot(BeEmpty(),
				"BR-SAFE-001: Must determine specific permission level for namespace")
			Expect(result.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM", "HIGH"}),
				"BR-SAFE-001: Risk level must reflect permission restrictions")

			// Business Value: Permission validation prevents unauthorized operations
		})
	})

	// BR-SAFE-002: Resource State Validation
	Context("BR-SAFE-002: Resource State Validation", func() {
		It("should validate deployment resource state correctly", func() {
			// Business Scenario: System validates deployment state before taking actions
			// Business Impact: Ensures actions are appropriate for current resource state

			// Setup fake deployment with known state
			namespace := "production"
			deploymentName := "web-app"

			// Create alert for deployment validation
			alert := types.Alert{
				Namespace: namespace,
				Resource:  deploymentName,
				Name:      "deployment-alert",
			}

			// Test REAL business logic for resource state validation
			result := safetyValidator.ValidateResourceState(ctx, alert)

			// Validate REAL business resource state validation outcomes
			Expect(result).ToNot(BeNil(),
				"BR-SAFE-002: Resource state validation must produce results")
			Expect(result.IsValid).To(BeTrue(),
				"BR-SAFE-002: Resource state validation must succeed for existing resources")
			Expect(result.ResourceExists).To(BeTrue(),
				"BR-SAFE-002: Must correctly identify existing resources")
			Expect(result.CurrentState).ToNot(BeEmpty(),
				"BR-SAFE-002: Must provide current resource state for business decisions")
			Expect(result.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM", "HIGH"}),
				"BR-SAFE-002: Risk level must be business-meaningful")

			// Business Value: Resource state validation ensures appropriate actions
		})

		It("should validate pod resource state correctly", func() {
			// Business Scenario: System validates pod state for pod-specific actions
			// Business Impact: Ensures pod actions are appropriate for current pod state

			// Setup fake pod with running state
			namespace := "production"
			podName := "web-app-pod"

			// Create alert for pod validation
			alert := types.Alert{
				Namespace: namespace,
				Resource:  podName,
				Name:      "pod-alert",
			}

			// Test REAL business logic for pod state validation
			result := safetyValidator.ValidateResourceState(ctx, alert)

			// Validate pod state validation outcomes
			Expect(result).ToNot(BeNil(),
				"BR-SAFE-002: Pod state validation must produce results")
			Expect(result.IsValid).To(BeTrue(),
				"BR-SAFE-002: Pod state validation must succeed for existing pods")
			Expect(result.ResourceExists).To(BeTrue(),
				"BR-SAFE-002: Must correctly identify existing pods")
			Expect(result.CurrentState).To(Equal("Running"),
				"BR-SAFE-002: Must accurately report pod phase")
			Expect(result.RiskLevel).To(Equal("LOW"),
				"BR-SAFE-002: Running pods should have low risk level")

			// Business Value: Pod state validation enables precise pod management
		})

		It("should handle non-existent resources gracefully", func() {
			// Business Scenario: System handles requests for non-existent resources
			// Business Impact: Prevents actions on missing resources and provides clear feedback

			// Setup namespace without the requested resource
			namespace := "production"

			// Create alert for non-existent resource
			alert := types.Alert{
				Namespace: namespace,
				Resource:  "non-existent-resource",
				Name:      "missing-resource-alert",
			}

			// Test REAL business logic for missing resource handling
			result := safetyValidator.ValidateResourceState(ctx, alert)

			// Validate missing resource handling
			Expect(result).ToNot(BeNil(),
				"BR-SAFE-002: Must handle missing resources gracefully")
			Expect(result.IsValid).To(BeFalse(),
				"BR-SAFE-002: Must indicate validation failure for missing resources")
			Expect(result.ResourceExists).To(BeFalse(),
				"BR-SAFE-002: Must correctly identify missing resources")
			Expect(result.ErrorMessage).ToNot(BeEmpty(),
				"BR-SAFE-002: Must provide error message for missing resources")
			Expect(result.RiskLevel).To(Equal("HIGH"),
				"BR-SAFE-002: Missing resources should have high risk level")

			// Business Value: Clear handling of missing resources prevents action failures
		})
	})

	// BR-SAFE-003: Risk Assessment and Mitigation
	Context("BR-SAFE-003: Risk Assessment and Mitigation", func() {
		It("should assess risk for destructive actions correctly", func() {
			// Business Scenario: System assesses risk for potentially destructive operations
			// Business Impact: Prevents accidental destructive operations through risk analysis

			// Create destructive action recommendation
			destructiveAction := types.ActionRecommendation{
				Action:     "delete_deployment",
				Confidence: 0.8,
				Parameters: map[string]interface{}{
					"force": true,
				},
			}

			// Create alert for production resource
			alert := types.Alert{
				Namespace: "production",
				Resource:  "critical-app",
				Name:      "critical-app-alert",
				Severity:  "critical",
			}

			// Test REAL business logic for destructive action risk assessment
			assessment := safetyValidator.AssessRisk(ctx, destructiveAction, alert)

			// Validate REAL business risk assessment outcomes
			Expect(assessment).ToNot(BeNil(),
				"BR-SAFE-003: Risk assessment must produce results")
			Expect(assessment.ActionName).To(Equal("delete_deployment"),
				"BR-SAFE-003: Risk assessment must be associated with correct action")
			Expect(assessment.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}),
				"BR-SAFE-003: Risk level must be business-meaningful")
			Expect(len(assessment.RiskFactors)).To(BeNumerically(">", 0),
				"BR-SAFE-003: Must identify specific risk factors for business evaluation")
			Expect(assessment.Confidence).To(BeNumerically(">=", 0.0),
				"BR-SAFE-003: Risk assessment confidence must be measurable")
			Expect(assessment.Confidence).To(BeNumerically("<=", 1.0),
				"BR-SAFE-003: Risk assessment confidence must be within valid range")

			// Validate risk factors for destructive actions
			riskFactorsStr := ""
			for _, factor := range assessment.RiskFactors {
				riskFactorsStr += factor + " "
			}
			Expect(riskFactorsStr).To(ContainSubstring("destructive"),
				"BR-SAFE-003: Must identify destructive nature as risk factor")

			// Business Value: Risk assessment prevents accidental destructive operations
		})

		It("should assess risk for safe actions with low risk", func() {
			// Business Scenario: System correctly identifies low-risk operations
			// Business Impact: Enables automated execution of safe operations

			// Create safe action recommendation
			safeAction := types.ActionRecommendation{
				Action:     "get_pod_status",
				Confidence: 0.9,
				Parameters: map[string]interface{}{},
			}

			// Create alert for development resource
			alert := types.Alert{
				Namespace: "development",
				Resource:  "test-app",
				Name:      "test-app-alert",
				Severity:  "warning",
			}

			// Test REAL business logic for safe action risk assessment
			assessment := safetyValidator.AssessRisk(ctx, safeAction, alert)

			// Validate safe action risk assessment
			Expect(assessment).ToNot(BeNil(),
				"BR-SAFE-003: Safe action risk assessment must produce results")
			Expect(assessment.ActionName).To(Equal("get_pod_status"),
				"BR-SAFE-003: Risk assessment must be associated with correct action")
			Expect(assessment.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM"}),
				"BR-SAFE-003: Safe actions should have low to medium risk")
			Expect(assessment.SafeToExecute).To(BeTrue(),
				"BR-SAFE-003: Safe actions should be marked as safe to execute")
			Expect(assessment.Confidence).To(BeNumerically(">=", 0.7),
				"BR-SAFE-003: Safe action assessment should have high confidence")

			// Business Value: Accurate risk assessment enables automated safe operations
		})

		It("should provide mitigation strategies for high-risk actions", func() {
			// Business Scenario: System provides mitigation strategies for risky operations
			// Business Impact: Enables safer execution of necessary but risky operations

			// Create high-risk action recommendation
			highRiskAction := types.ActionRecommendation{
				Action:     "restart_deployment",
				Confidence: 0.7,
				Parameters: map[string]interface{}{
					"replicas": 10,
				},
			}

			// Create alert for production resource
			alert := types.Alert{
				Namespace: "production",
				Resource:  "payment-service",
				Name:      "payment-service-alert",
				Severity:  "critical",
			}

			// Test REAL business logic for high-risk action assessment
			assessment := safetyValidator.AssessRisk(ctx, highRiskAction, alert)

			// Validate mitigation strategy provision
			Expect(assessment).ToNot(BeNil(),
				"BR-SAFE-003: High-risk action assessment must produce results")
			Expect(assessment.Mitigation).ToNot(BeEmpty(),
				"BR-SAFE-003: High-risk actions must have mitigation strategies")
			Expect(len(assessment.RiskFactors)).To(BeNumerically(">", 0),
				"BR-SAFE-003: High-risk actions must identify specific risk factors")
			Expect(assessment.Metadata).ToNot(BeNil(),
				"BR-SAFE-003: Risk assessment must provide metadata for business analysis")

			// Business Value: Mitigation strategies enable safer execution of necessary operations
		})
	})

	// BR-SAFE-004: Health Check Validation
	Context("BR-SAFE-004: Health Check Validation", func() {
		It("should validate safety validator health status", func() {
			// Business Scenario: System validates safety validator is healthy before operations
			// Business Impact: Ensures safety validation is operational before critical operations

			// Test REAL business logic for health check validation
			healthErr := safetyValidator.IsHealthy(ctx)

			// Validate REAL business health check outcomes
			Expect(healthErr).ToNot(HaveOccurred(),
				"BR-SAFE-004: Safety validator health check must pass with proper setup")

			// Business Value: Health validation ensures safety systems are operational
		})

		It("should handle health check failures gracefully", func() {
			// Business Scenario: System handles safety validator health issues
			// Business Impact: Prevents operations when safety systems are compromised

			// Create a safety validator with nil client to simulate failure
			unhealthyValidator := safety.NewSafetyValidator(nil, mockLogger)

			// Test REAL business logic for health check failure handling
			healthErr := unhealthyValidator.IsHealthy(ctx)

			// Validate health check failure handling
			Expect(healthErr).To(HaveOccurred(),
				"BR-SAFE-004: Health check must fail with compromised safety validator")

			// Business Value: Health check failures prevent unsafe operations
		})
	})

	// BR-SAFE-005: Comprehensive Safety Integration
	Context("BR-SAFE-005: Comprehensive Safety Integration", func() {
		It("should integrate all safety validations for comprehensive protection", func() {
			// Business Scenario: System integrates all safety validations for comprehensive protection
			// Business Impact: Ensures complete safety coverage for production operations

			// Setup comprehensive test environment
			namespace := "production"
			deploymentName := "critical-service"

			// Create action for comprehensive validation
			action := types.ActionRecommendation{
				Action:     "scale_deployment",
				Confidence: 0.8,
				Parameters: map[string]interface{}{
					"replicas": 5,
				},
			}

			// Create production alert
			alert := types.Alert{
				Namespace: namespace,
				Resource:  deploymentName,
				Name:      "scaling-alert",
				Severity:  "warning",
			}

			// Test REAL business logic for comprehensive safety integration
			// Step 1: Validate cluster access
			clusterResult := safetyValidator.ValidateClusterAccess(ctx, namespace)

			// Step 2: Validate resource state
			resourceResult := safetyValidator.ValidateResourceState(ctx, alert)

			// Step 3: Assess risk
			riskAssessment := safetyValidator.AssessRisk(ctx, action, alert)

			// Step 4: Check health
			healthErr := safetyValidator.IsHealthy(ctx)

			// Validate REAL business comprehensive safety integration outcomes
			Expect(clusterResult).ToNot(BeNil(),
				"BR-SAFE-005: Cluster validation must be part of comprehensive safety")
			Expect(resourceResult).ToNot(BeNil(),
				"BR-SAFE-005: Resource validation must be part of comprehensive safety")
			Expect(riskAssessment).ToNot(BeNil(),
				"BR-SAFE-005: Risk assessment must be part of comprehensive safety")
			Expect(healthErr).ToNot(HaveOccurred(),
				"BR-SAFE-005: Health check must be part of comprehensive safety")

			// Validate integration consistency
			Expect(clusterResult.IsValid).To(BeTrue(),
				"BR-SAFE-005: Cluster access must be valid for comprehensive safety")
			Expect(riskAssessment.ActionName).To(Equal(action.Action),
				"BR-SAFE-005: Risk assessment must be associated with correct action")

			// Business Value: Comprehensive safety integration ensures complete protection
		})
	})
})

// Helper functions for Kubernetes safety testing
// These create realistic test data for REAL business logic validation

// TestRunner bootstraps the Ginkgo test suite
func TestUkubernetesUsafetyUunit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UkubernetesUsafetyUunit Suite")
}
