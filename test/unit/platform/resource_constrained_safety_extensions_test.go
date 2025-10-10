//go:build unit
// +build unit

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
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/platform/executor"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/platform/safety"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// Week 2: Platform Safety Extensions - Resource-Constrained Safety Testing
// Business Requirements: BR-SAFE-026 through BR-SAFE-035
// Following 00-project-guidelines.mdc: MANDATORY business requirement mapping
// Following 03-testing-strategy.mdc: PREFER real business logic over mocks
// Following 05-kubernetes-safety.mdc: Kubernetes operations and safety patterns
// Following 09-interface-method-validation.mdc: Interface validation before code generation

var _ = Describe("Resource-Constrained Safety Extensions - Week 2 Business Requirements", func() {
	var (
		ctx    context.Context
		logger *logrus.Logger

		// Real business logic components (PREFERRED per rule 03)
		realSafetyValidator *safety.SafetyValidator
		realActionExecutor  executor.Executor
		realK8sClient       k8s.Client

		// Enhanced fake K8s client with ResourceConstrained scenario
		enhancedK8sClientset *fake.Clientset

		// Mock external dependencies only (per rule 03)
		mockActionHistoryRepo *mocks.MockActionHistoryRepository

		// Test configuration
		actionsConfig config.ActionsConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce noise in tests

		// Create enhanced fake K8s client with ResourceConstrained scenario
		// Override namespaces to include production and security for multi-environment testing
		resourceConstrainedScenario := enhanced.ResourceConstrained
		clientConfig := &enhanced.SmartFakeClientConfig{
			TestType:         enhanced.TestTypeSafety,
			ScenarioOverride: &resourceConstrainedScenario,
		}
		enhancedK8sClientset = enhanced.NewSmartFakeClientsetWithConfig(clientConfig)

		// Create real K8s client wrapper
		realK8sClient = k8s.NewUnifiedClient(enhancedK8sClientset, config.KubernetesConfig{
			Namespace: "default",
		}, logger)

		// Create additional namespaces required for multi-environment testing
		productionNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "production"},
		}
		securityNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "security"},
		}
		monitoringNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "monitoring"},
		}
		_, _ = enhancedK8sClientset.CoreV1().Namespaces().Create(ctx, productionNS, metav1.CreateOptions{})
		_, _ = enhancedK8sClientset.CoreV1().Namespaces().Create(ctx, securityNS, metav1.CreateOptions{})
		_, _ = enhancedK8sClientset.CoreV1().Namespaces().Create(ctx, monitoringNS, metav1.CreateOptions{})

		// Initialize real business components (MANDATORY per rule 03)
		realSafetyValidator = safety.NewSafetyValidator(enhancedK8sClientset, logger)

		// Configure actions for resource-constrained testing
		actionsConfig = config.ActionsConfig{
			DryRun:         false, // Test real actions under constraints
			MaxConcurrent:  2,     // Limited concurrency for resource constraints
			CooldownPeriod: 30 * time.Second,
		}

		// Mock external dependencies only
		mockActionHistoryRepo = mocks.NewMockActionHistoryRepository()

		// Create real action executor with resource constraints
		var err error
		realActionExecutor, err = executor.NewExecutor(realK8sClient, actionsConfig, mockActionHistoryRepo, logger)
		Expect(err).ToNot(HaveOccurred(), "Real action executor creation must succeed")
	})

	Context("BR-SAFE-026: Resource-Constrained Safety Validation", func() {
		It("should make conservative safety decisions under resource pressure", func() {
			// Create resource-constrained alert scenarios
			resourceConstrainedAlerts := createResourceConstrainedAlerts()

			for _, alert := range resourceConstrainedAlerts {
				// Test real safety validator under resource constraints
				clusterValidation := realSafetyValidator.ValidateClusterAccess(ctx, alert.Namespace)
				Expect(clusterValidation.IsValid).To(BeTrue(),
					"BR-SAFE-026: Cluster access validation must succeed even under constraints")

				// Validate cluster access under resource pressure
				Expect(clusterValidation.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM", "HIGH"}),
					"BR-SAFE-026: Cluster validation should provide valid risk level")

				// Test resource state validation
				resourceValidation := realSafetyValidator.ValidateResourceState(ctx, alert)
				Expect(resourceValidation).ToNot(BeNil(),
					"BR-SAFE-026: Resource validation must return results")

				// Under resource constraints, validation should be more conservative
				if !resourceValidation.ResourceExists {
					Expect(resourceValidation.RiskLevel).To(Equal("HIGH"),
						"BR-SAFE-026: Non-existent resources under constraints should be high risk")
				}
			}
		})

		It("should assess action safety with resource constraint awareness", func() {
			// Create high-risk actions for resource-constrained environment
			constrainedActions := createResourceConstrainedActions()

			for _, action := range constrainedActions {
				alert := types.Alert{
					Name:      fmt.Sprintf("resource-pressure-%s", action.Action),
					Namespace: "production",
					Resource:  "constrained-deployment",
					Severity:  "warning",
					Labels: map[string]string{
						"resource_pressure": "high",
						"cluster_load":      "85%",
					},
				}

				// Test real safety assessment under resource constraints
				riskAssessment := realSafetyValidator.AssessRisk(ctx, *action, alert)
				Expect(riskAssessment).ToNot(BeNil(),
					"BR-SAFE-026: Risk assessment must be provided for all actions")

				// Business Requirement Validation: BR-SAFE-026
				Expect(riskAssessment.ActionName).To(Equal(action.Action),
					"BR-SAFE-026: Risk assessment must match requested action")

				// Validate risk assessment matches business logic (per rule 03: test real business logic)
				if action.Action == "drain_node" {
					Expect(riskAssessment.RiskLevel).To(Equal("HIGH"),
						"BR-SAFE-026: drain_node should have HIGH risk per business logic")
					Expect(riskAssessment.SafeToExecute).To(BeFalse(),
						"BR-SAFE-026: HIGH risk actions should not be safe to execute by default")
				} else if action.Action == "scale_deployment" {
					Expect(riskAssessment.RiskLevel).To(Equal("LOW"),
						"BR-SAFE-026: scale_deployment should have LOW risk per business logic")
					Expect(riskAssessment.SafeToExecute).To(BeTrue(),
						"BR-SAFE-026: LOW risk actions should be safe to execute")
				}

				// Validate risk factors are provided (per rule 03: test real business logic)
				Expect(riskAssessment.RiskFactors).ToNot(BeEmpty(),
					"BR-SAFE-026: Risk factors must be provided for all assessments")

				// Validate risk factors match business logic for specific actions
				if action.Action == "drain_node" {
					Expect(riskAssessment.RiskFactors).To(ContainElement(ContainSubstring("service disruption")),
						"BR-SAFE-026: drain_node should include service disruption risk factor")
				} else if action.Action == "scale_deployment" {
					Expect(riskAssessment.RiskFactors).To(ContainElement(ContainSubstring("resource utilization")),
						"BR-SAFE-026: scale_deployment should include resource utilization risk factor")
				}
			}
		})
	})

	Context("BR-SAFE-027: Multi-Environment Safety Validation", func() {
		It("should validate safety across different environment types", func() {
			// Test safety validation across production, monitoring, and security environments
			environments := []struct {
				namespace   string
				riskProfile string
				constraints map[string]string
			}{
				{
					namespace:   "production",
					riskProfile: "HIGH_STAKES",
					constraints: map[string]string{"availability": "99.9%", "downtime_tolerance": "low"},
				},
				{
					namespace:   "monitoring",
					riskProfile: "MEDIUM_STAKES",
					constraints: map[string]string{"observability": "critical", "data_loss_tolerance": "none"},
				},
				{
					namespace:   "security",
					riskProfile: "SECURITY_CRITICAL",
					constraints: map[string]string{"compliance": "required", "audit_trail": "mandatory"},
				},
			}

			for _, env := range environments {
				alert := types.Alert{
					Name:      fmt.Sprintf("multi-env-test-%s", env.namespace),
					Namespace: env.namespace,
					Resource:  "environment-service",
					Severity:  "warning",
					Labels:    env.constraints,
				}

				// Test environment-specific safety validation
				clusterValidation := realSafetyValidator.ValidateClusterAccess(ctx, env.namespace)
				Expect(clusterValidation.IsValid).To(BeTrue(),
					"BR-SAFE-027: Multi-environment validation must succeed")

				// Validate environment-specific risk assessment
				action := &types.ActionRecommendation{
					Action:     "restart_pod",
					Confidence: 0.8,
					Parameters: map[string]interface{}{
						"environment": env.riskProfile,
						"constraints": env.constraints,
					},
				}

				riskAssessment := realSafetyValidator.AssessRisk(ctx, *action, alert)

				// Business Requirement Validation: BR-SAFE-027
				Expect(riskAssessment.RiskLevel).ToNot(BeEmpty(),
					"BR-SAFE-027: Environment-specific risk assessment must be provided")

				// Production environment should have stricter safety requirements
				if env.namespace == "production" {
					Expect(riskAssessment.Confidence).To(BeNumerically(">=", 0.8),
						"BR-SAFE-027: Production actions require high confidence")
				}

				// Security environment validation (per rule 03: test real business logic)
				if env.namespace == "security" {
					Expect(riskAssessment.RiskFactors).ToNot(BeEmpty(),
						"BR-SAFE-027: Security environment must provide risk factors")
					Expect(riskAssessment.Confidence).To(BeNumerically(">=", 0.5),
						"BR-SAFE-027: Security environment must provide confidence assessment")
				}
			}
		})
	})

	Context("BR-SAFE-028: Action Execution Safety Under Resource Constraints", func() {
		It("should execute actions safely with resource constraint validation", func() {
			// Create resource-constrained deployment in enhanced fake cluster
			deployment := createResourceConstrainedDeployment("constrained-app", "production")
			_, err := enhancedK8sClientset.AppsV1().Deployments("production").Create(ctx, deployment, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred(), "Test deployment creation must succeed")

			// Test safe action execution under resource constraints
			constrainedAlert := types.Alert{
				Name:      "resource-pressure-alert",
				Namespace: "production",
				Resource:  "constrained-app",
				Severity:  "warning",
				Labels: map[string]string{
					"memory_pressure": "high",
					"cpu_throttling":  "detected",
				},
			}

			safeActions := []*types.ActionRecommendation{
				{
					Action:     "scale_deployment",
					Confidence: 0.7,
					Parameters: map[string]interface{}{
						"replicas": 2, // Conservative scaling under constraints
					},
				},
				{
					Action:     "increase_resources",
					Confidence: 0.8,
					Parameters: map[string]interface{}{
						"cpu_limit":    "200m", // Modest resource increase
						"memory_limit": "256Mi",
					},
				},
			}

			for _, action := range safeActions {
				// Validate action safety before execution
				riskAssessment := realSafetyValidator.AssessRisk(ctx, *action, constrainedAlert)

				// Only execute if deemed safe under constraints
				if riskAssessment.SafeToExecute {
					// Create action trace for monitoring
					actionTrace := &actionhistory.ResourceActionTrace{
						ActionID:        fmt.Sprintf("safe-exec-%s", action.Action),
						ActionType:      action.Action,
						ActionTimestamp: time.Now(),
						ExecutionStatus: "initiated",
					}

					// Test real action execution with resource constraints
					err := realActionExecutor.Execute(ctx, action, constrainedAlert, actionTrace)

					// Business Requirement Validation: BR-SAFE-028
					if riskAssessment.RiskLevel == "LOW" || riskAssessment.RiskLevel == "MEDIUM" {
						Expect(err).ToNot(HaveOccurred(),
							"BR-SAFE-028: Safe actions under constraints should execute successfully")
					} else {
						// High-risk actions may fail safely under constraints
						if err != nil {
							Expect(err.Error()).To(ContainSubstring("resource"),
								"BR-SAFE-028: Failed actions should indicate resource constraint issues")
						}
					}
				} else {
					// Validate that unsafe actions are properly blocked
					Expect(riskAssessment.RiskLevel).To(Equal("HIGH"),
						"BR-SAFE-028: Unsafe actions should be marked as high risk")
				}
			}
		})
	})

	Context("BR-SAFE-029: Cross-Component Safety Coordination", func() {
		It("should coordinate safety decisions across multiple components", func() {
			// Create multi-component scenario with resource constraints
			components := []string{"api-gateway", "database", "cache", "message-queue"}

			// Create deployments for each component in enhanced fake cluster
			for _, component := range components {
				deployment := createResourceConstrainedDeployment(component, "production")
				_, err := enhancedK8sClientset.AppsV1().Deployments("production").Create(ctx, deployment, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			}

			// Test coordinated safety validation across components
			coordinatedActions := createCrossComponentActions(components)

			for _, actionSet := range coordinatedActions {
				// Validate safety coordination across multiple components
				var riskAssessments []*safety.RiskAssessment

				for _, action := range actionSet.Actions {
					alert := types.Alert{
						Name:      fmt.Sprintf("cross-component-%s", action.Action),
						Namespace: "production",
						Resource:  actionSet.PrimaryComponent,
						Severity:  "warning",
						Labels: map[string]string{
							"component_group": actionSet.ComponentGroup,
							"coordination_id": actionSet.CoordinationID,
						},
					}

					riskAssessment := realSafetyValidator.AssessRisk(ctx, *action, alert)
					riskAssessments = append(riskAssessments, riskAssessment)
				}

				// Business Requirement Validation: BR-SAFE-029
				Expect(len(riskAssessments)).To(Equal(len(actionSet.Actions)),
					"BR-SAFE-029: All coordinated actions must have risk assessments")

				// Validate cross-component safety coordination
				highRiskCount := 0
				for _, assessment := range riskAssessments {
					if assessment.RiskLevel == "HIGH" {
						highRiskCount++
					}
				}

				// If any component has high risk, coordination should be conservative
				if highRiskCount > 0 {
					allSafe := true
					for _, assessment := range riskAssessments {
						if !assessment.SafeToExecute {
							allSafe = false
							break
						}
					}

					Expect(allSafe).To(BeFalse(),
						"BR-SAFE-029: Cross-component coordination should prevent execution if any component is high risk")
				}

				// Validate assessment metadata (per rule 03: test real business logic)
				for _, assessment := range riskAssessments {
					Expect(assessment.Metadata).To(HaveKey("alert_severity"),
						"BR-SAFE-029: Risk assessments must include alert severity metadata")
					Expect(assessment.Metadata).To(HaveKey("assessment_time"),
						"BR-SAFE-029: Risk assessments must include assessment time metadata")
				}
			}
		})
	})

	Context("BR-SAFE-030: Performance-Optimized Safety Validation", func() {
		It("should maintain performance while ensuring safety under resource constraints", func() {
			// Create high-volume safety validation scenario
			highVolumeAlerts := createHighVolumeSafetyAlerts(100) // 100 alerts for performance testing

			// Performance measurement for safety validation
			startTime := time.Now()
			var validationResults []*safety.ClusterValidationResult

			for _, alert := range highVolumeAlerts {
				result := realSafetyValidator.ValidateClusterAccess(ctx, alert.Namespace)
				validationResults = append(validationResults, result)
			}

			validationTime := time.Since(startTime)

			// Performance measurement for risk assessment
			startTime = time.Now()
			var riskAssessments []*safety.RiskAssessment

			for i, alert := range highVolumeAlerts {
				action := &types.ActionRecommendation{
					Action:     "restart_pod",
					Confidence: 0.7,
					Parameters: map[string]interface{}{
						"batch_id": i / 10, // Group into batches
					},
				}

				assessment := realSafetyValidator.AssessRisk(ctx, *action, alert)
				riskAssessments = append(riskAssessments, assessment)
			}

			assessmentTime := time.Since(startTime)

			// Business Requirement Validation: BR-SAFE-030
			Expect(len(validationResults)).To(Equal(100),
				"BR-SAFE-030: Must process all high-volume validation requests")
			Expect(len(riskAssessments)).To(Equal(100),
				"BR-SAFE-030: Must process all high-volume risk assessments")

			// Performance requirements for safety validation
			Expect(validationTime).To(BeNumerically("<", 10*time.Second),
				"BR-SAFE-030: Cluster validation must complete within 10 seconds for 100 alerts")
			Expect(assessmentTime).To(BeNumerically("<", 15*time.Second),
				"BR-SAFE-030: Risk assessment must complete within 15 seconds for 100 actions")

			// Validate processing efficiency
			validationRate := float64(len(validationResults)) / validationTime.Seconds()
			Expect(validationRate).To(BeNumerically(">", 8),
				"BR-SAFE-030: Must validate >8 cluster accesses per second")

			assessmentRate := float64(len(riskAssessments)) / assessmentTime.Seconds()
			Expect(assessmentRate).To(BeNumerically(">", 5),
				"BR-SAFE-030: Must assess >5 action risks per second")

			// Validate safety quality is maintained under performance pressure
			validCount := 0
			for _, result := range validationResults {
				if result.IsValid {
					validCount++
				}
			}

			Expect(float64(validCount)/float64(len(validationResults))).To(BeNumerically(">", 0.9),
				"BR-SAFE-030: Must maintain >90% validation success rate under performance pressure")
		})
	})
})

// Helper functions for test data creation and validation

func createResourceConstrainedAlerts() []types.Alert {
	alerts := []types.Alert{
		{
			Name:      "memory-pressure-alert",
			Namespace: "production",
			Resource:  "memory-intensive-app",
			Severity:  "critical",
			Labels: map[string]string{
				"memory_usage":    "95%",
				"resource_type":   "memory",
				"constraint_type": "hard_limit",
			},
		},
		{
			Name:      "cpu-throttling-alert",
			Namespace: "production",
			Resource:  "cpu-intensive-service",
			Severity:  "warning",
			Labels: map[string]string{
				"cpu_throttling":  "detected",
				"resource_type":   "cpu",
				"constraint_type": "soft_limit",
			},
		},
		{
			Name:      "disk-space-alert",
			Namespace: "monitoring",
			Resource:  "log-aggregator",
			Severity:  "warning",
			Labels: map[string]string{
				"disk_usage":      "85%",
				"resource_type":   "storage",
				"constraint_type": "capacity",
			},
		},
		{
			Name:      "network-congestion-alert",
			Namespace: "security",
			Resource:  "security-scanner",
			Severity:  "info",
			Labels: map[string]string{
				"network_latency": "high",
				"resource_type":   "network",
				"constraint_type": "bandwidth",
			},
		},
	}

	return alerts
}

func createResourceConstrainedActions() []*types.ActionRecommendation {
	actions := []*types.ActionRecommendation{
		{
			Action:     "scale_deployment",
			Confidence: 0.8,
			Parameters: map[string]interface{}{
				"replicas":         3,
				"resource_impact":  "medium",
				"constraint_aware": true,
			},
		},
		{
			Action:     "drain_node",
			Confidence: 0.6,
			Parameters: map[string]interface{}{
				"graceful_timeout": "300s",
				"resource_impact":  "high",
				"constraint_aware": true,
			},
		},
		{
			Action:     "increase_resources",
			Confidence: 0.9,
			Parameters: map[string]interface{}{
				"cpu_increase":     "100m",
				"memory_increase":  "128Mi",
				"resource_impact":  "low",
				"constraint_aware": true,
			},
		},
		{
			Action:     "restart_pod",
			Confidence: 0.7,
			Parameters: map[string]interface{}{
				"restart_policy":   "graceful",
				"resource_impact":  "low",
				"constraint_aware": true,
			},
		},
	}

	return actions
}

func createResourceConstrainedDeployment(name, namespace string) *appsv1.Deployment {
	replicas := int32(1) // Resource-constrained: minimal replicas

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":              name,
				"resource_profile": "constrained",
				"environment":      namespace,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: fmt.Sprintf("%s:latest", name),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),  // Minimal CPU
									corev1.ResourceMemory: resource.MustParse("64Mi"), // Minimal memory
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),  // Constrained CPU
									corev1.ResourceMemory: resource.MustParse("128Mi"), // Constrained memory
								},
							},
						},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1,
			Replicas:      1,
		},
	}
}

type CrossComponentActionSet struct {
	CoordinationID   string
	ComponentGroup   string
	PrimaryComponent string
	Actions          []*types.ActionRecommendation
}

func createCrossComponentActions(components []string) []CrossComponentActionSet {
	actionSets := []CrossComponentActionSet{
		{
			CoordinationID:   "coord-001",
			ComponentGroup:   "frontend-backend",
			PrimaryComponent: components[0], // api-gateway
			Actions: []*types.ActionRecommendation{
				{
					Action:     "scale_deployment",
					Confidence: 0.8,
					Parameters: map[string]interface{}{
						"component": components[0],
						"replicas":  2,
					},
				},
				{
					Action:     "restart_pod",
					Confidence: 0.7,
					Parameters: map[string]interface{}{
						"component": components[1], // database
						"graceful":  true,
					},
				},
			},
		},
		{
			CoordinationID:   "coord-002",
			ComponentGroup:   "data-layer",
			PrimaryComponent: components[1], // database
			Actions: []*types.ActionRecommendation{
				{
					Action:     "increase_resources",
					Confidence: 0.9,
					Parameters: map[string]interface{}{
						"component":       components[1], // database
						"memory_increase": "256Mi",
					},
				},
				{
					Action:     "restart_pod",
					Confidence: 0.6,
					Parameters: map[string]interface{}{
						"component": components[2], // cache
						"graceful":  true,
					},
				},
			},
		},
	}

	return actionSets
}

func createHighVolumeSafetyAlerts(count int) []types.Alert {
	alerts := make([]types.Alert, count)

	namespaces := []string{"production", "monitoring", "security"}
	severities := []string{"info", "warning", "critical"}
	resourceTypes := []string{"deployment", "service", "pod", "configmap"}

	for i := 0; i < count; i++ {
		namespace := namespaces[i%len(namespaces)]
		severity := severities[i%len(severities)]
		resourceType := resourceTypes[i%len(resourceTypes)]

		alerts[i] = types.Alert{
			Name:      fmt.Sprintf("high-volume-alert-%d", i),
			Namespace: namespace,
			Resource:  fmt.Sprintf("%s-%d", resourceType, i),
			Severity:  severity,
			Labels: map[string]string{
				"batch_id":      fmt.Sprintf("%d", i/10),
				"resource_type": resourceType,
				"test_scenario": "high_volume",
				"alert_index":   fmt.Sprintf("%d", i),
			},
		}
	}

	return alerts
}

// TestRunner bootstraps the Ginkgo test suite
func TestUresourceUconstrainedUsafetyUextensions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UresourceUconstrainedUsafetyUextensions Suite")
}
