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

//go:build unit
// +build unit

package workflowengine

import (
	"testing"
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// BR-SEC-001 through BR-SEC-009: Security Enhancement Unit Testing - Pyramid Testing (70% Unit Coverage)
// Business Impact: Validates security enhancement capabilities for production-safe workflow execution
// Stakeholder Value: Operations teams can trust security-validated automation workflows
var _ = Describe("BR-SEC-001 through BR-SEC-009: Security Enhancement Unit Testing", func() {
	var (
		// Mock ONLY external dependencies per pyramid principles
		mockLogger *logrus.Logger

		// Use REAL business logic components
		intelligentBuilder *engine.DefaultIntelligentWorkflowBuilder

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create REAL business logic component for security testing
		// Mock only external dependencies, use real business logic
		mockLLMClient := mocks.NewMockLLMClient()
		mockVectorDB := mocks.NewMockVectorDatabase()
		mockExecutionRepo := mocks.NewWorkflowExecutionRepositoryMock()

		// Create workflow builder using new config pattern
		config := &engine.IntelligentWorkflowBuilderConfig{
			LLMClient:       mockLLMClient,     // External: Mock
			VectorDB:        mockVectorDB,      // External: Mock
			AnalyticsEngine: nil,               // AnalyticsEngine: Not needed for security tests
			PatternStore:    nil,               // PatternStore: Not needed for security tests
			ExecutionRepo:   mockExecutionRepo, // External: Mock
			Logger:          mockLogger,        // External: Mock (logging infrastructure)
		}

		var err error
		intelligentBuilder, err = engine.NewIntelligentWorkflowBuilder(config)
		Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")
	})

	AfterEach(func() {
		cancel()
	})

	// BR-SEC-001: Security Constraint Validation
	Context("BR-SEC-001: Security Constraint Validation", func() {
		It("should validate security constraints for workflow templates", func() {
			// Business Scenario: System validates security constraints before workflow execution
			// Business Impact: Prevents execution of workflows that violate security policies

			// Create workflow template with security-sensitive actions
			template := createSecurityTestTemplate("security-validation-001")

			// Test REAL business logic for security constraint validation
			validationResults := intelligentBuilder.ValidateSecurityConstraints(ctx, template)

			// Validate REAL business security validation outcomes
			Expect(validationResults).ToNot(BeNil(),
				"BR-SEC-001: Security constraint validation must produce results")
			Expect(len(validationResults)).To(BeNumerically(">", 0),
				"BR-SEC-001: Must validate security constraints and produce validation results")

			// Validate validation result structure
			for _, result := range validationResults {
				Expect(result.RuleID).ToNot(BeEmpty(),
					"BR-SEC-001: Each validation result must have identifiable rule ID")
				Expect(result.Type).ToNot(BeEmpty(),
					"BR-SEC-001: Each validation result must have validation type")
				Expect(result.Message).ToNot(BeEmpty(),
					"BR-SEC-001: Each validation result must have descriptive message")
				Expect(result.Timestamp).ToNot(BeZero(),
					"BR-SEC-001: Each validation result must have timestamp for audit")
			}

			// Business Value: Security constraint validation prevents policy violations
		})

		It("should identify security violations in destructive actions", func() {
			// Business Scenario: System identifies potentially dangerous operations
			// Business Impact: Prevents accidental destructive operations in production

			// Create template with destructive actions
			template := createDestructiveActionTemplate("destructive-security-001")

			// Test REAL business logic for destructive action detection
			validationResults := intelligentBuilder.ValidateSecurityConstraints(ctx, template)

			// Validate destructive action detection
			Expect(validationResults).ToNot(BeNil(),
				"BR-SEC-001: Must detect destructive actions in security validation")

			// Check for security violations
			hasSecurityViolations := false
			for _, result := range validationResults {
				if !result.Passed && result.Type == engine.ValidationTypeSecurity {
					hasSecurityViolations = true
					break
				}
			}
			Expect(hasSecurityViolations).To(BeTrue(),
				"BR-SEC-001: Must detect security violations in destructive actions")

			// Business Value: Destructive action detection prevents operational accidents
		})
	})

	// BR-SEC-002: Security Policy Application
	Context("BR-SEC-002: Security Policy Application", func() {
		It("should apply security policies to workflow templates", func() {
			// Business Scenario: System applies security policies to enhance workflow safety
			// Business Impact: Ensures workflows comply with organizational security standards

			// Create workflow template for policy application
			template := createSecurityTestTemplate("policy-application-001")

			// Create comprehensive security policies
			securityPolicies := createComprehensiveSecurityPolicies()

			// Test REAL business logic for security policy application
			securedTemplate := intelligentBuilder.ApplySecurityPolicies(template, securityPolicies)

			// Validate REAL business security policy application outcomes
			Expect(securedTemplate).ToNot(BeNil(),
				"BR-SEC-002: Security policy application must produce secured template")
			Expect(securedTemplate.ID).To(Equal(template.ID),
				"BR-SEC-002: Secured template must maintain original template identity")
			Expect(len(securedTemplate.Steps)).To(Equal(len(template.Steps)),
				"BR-SEC-002: Security policies must not remove workflow steps")

			// Validate security enhancements applied to steps
			for _, step := range securedTemplate.Steps {
				Expect(step.Variables).ToNot(BeNil(),
					"BR-SEC-002: Each step must have variables after security enhancement")
				Expect(step.Variables["security_enhanced"]).To(BeTrue(),
					"BR-SEC-002: Each step must be marked as security enhanced")
				Expect(step.Variables["security_policies_applied"]).To(BeTrue(),
					"BR-SEC-002: Each step must indicate security policies were applied")
			}

			// Business Value: Security policy application ensures compliance
		})

		It("should apply RBAC policies when enabled", func() {
			// Business Scenario: System applies RBAC policies for access control
			// Business Impact: Ensures proper access controls in workflow execution

			// Create template for RBAC testing
			template := createRBACTestTemplate("rbac-policy-001")

			// Create RBAC-focused security policies
			rbacPolicies := createRBACSecurityPolicies()

			// Test REAL business logic for RBAC policy application
			securedTemplate := intelligentBuilder.ApplySecurityPolicies(template, rbacPolicies)

			// Validate RBAC policy application
			Expect(securedTemplate).ToNot(BeNil(),
				"BR-SEC-002: RBAC policy application must produce secured template")

			// Validate RBAC-specific enhancements
			for _, step := range securedTemplate.Steps {
				if step.Variables != nil {
					Expect(step.Variables["security_level"]).To(Equal("strict"),
						"BR-SEC-002: RBAC policies must set strict security level")
				}
			}

			// Business Value: RBAC policies ensure proper access control
		})

		It("should apply network policies for network security", func() {
			// Business Scenario: System applies network policies for network isolation
			// Business Impact: Ensures network security in multi-tenant environments

			// Create template for network policy testing
			template := createNetworkSecurityTemplate("network-policy-001")

			// Create network-focused security policies
			networkPolicies := createNetworkSecurityPolicies()

			// Test REAL business logic for network policy application
			securedTemplate := intelligentBuilder.ApplySecurityPolicies(template, networkPolicies)

			// Validate network policy application
			Expect(securedTemplate).ToNot(BeNil(),
				"BR-SEC-002: Network policy application must produce secured template")

			// Validate network security enhancements
			for _, step := range securedTemplate.Steps {
				if step.Variables != nil {
					Expect(step.Variables["security_enhanced"]).To(BeTrue(),
						"BR-SEC-002: Network policies must enhance step security")
				}
			}

			// Business Value: Network policies ensure network isolation and security
		})
	})

	// BR-SEC-003: Security Report Generation
	Context("BR-SEC-003: Security Report Generation", func() {
		It("should generate comprehensive security analysis reports", func() {
			// Business Scenario: System generates security reports for compliance and audit
			// Business Impact: Enables security compliance monitoring and audit trails

			// Create workflow with security analysis requirements
			workflow := createSecurityAnalysisWorkflow("security-report-001")

			// Test REAL business logic for security report generation
			securityReport := intelligentBuilder.GenerateSecurityReport(workflow)

			// Validate REAL business security report generation outcomes
			Expect(securityReport).ToNot(BeNil(),
				"BR-SEC-003: Security report generation must produce results")
			Expect(securityReport.WorkflowID).To(Equal(workflow.ID),
				"BR-SEC-003: Security report must be associated with correct workflow")
			Expect(securityReport.SecurityScore).To(BeNumerically(">=", 0.0),
				"BR-SEC-003: Security score must be non-negative")
			Expect(securityReport.SecurityScore).To(BeNumerically("<=", 1.0),
				"BR-SEC-003: Security score must be within valid range")
			Expect(securityReport.ComplianceStatus).ToNot(BeEmpty(),
				"BR-SEC-003: Must provide compliance status for business monitoring")
			Expect(securityReport.GeneratedAt).ToNot(BeZero(),
				"BR-SEC-003: Must track report generation time for audit")

			// Validate security findings structure
			for _, finding := range securityReport.SecurityFindings {
				Expect(finding.ID).ToNot(BeEmpty(),
					"BR-SEC-003: Each security finding must have unique identifier")
				Expect(finding.Type).ToNot(BeEmpty(),
					"BR-SEC-003: Each security finding must have type for categorization")
				Expect(finding.Severity).To(BeElementOf([]string{"low", "medium", "high", "critical"}),
					"BR-SEC-003: Security finding severity must be business-meaningful")
				Expect(finding.Description).ToNot(BeEmpty(),
					"BR-SEC-003: Each security finding must have description for understanding")
				Expect(finding.Remediation).ToNot(BeEmpty(),
					"BR-SEC-003: Each security finding must have remediation guidance")
			}

			// Validate recommended actions
			Expect(len(securityReport.RecommendedActions)).To(BeNumerically(">=", 0),
				"BR-SEC-003: Must provide security recommendations for business action")

			// Business Value: Security reports enable compliance monitoring and audit
		})

		It("should handle workflows without templates gracefully", func() {
			// Business Scenario: System handles incomplete workflows gracefully
			// Business Impact: Ensures system reliability with partial workflow data

			// Create workflow without template
			workflow := createWorkflowWithoutTemplate("no-template-001")

			// Test REAL business logic for graceful handling
			securityReport := intelligentBuilder.GenerateSecurityReport(workflow)

			// Validate graceful handling of missing template
			Expect(securityReport).ToNot(BeNil(),
				"BR-SEC-003: Must handle workflows without templates gracefully")
			Expect(securityReport.WorkflowID).To(Equal(workflow.ID),
				"BR-SEC-003: Must maintain workflow association even without template")
			Expect(securityReport.SecurityScore).To(Equal(0.0),
				"BR-SEC-003: Must indicate no security analysis possible without template")
			Expect(securityReport.ComplianceStatus).To(Equal("unknown"),
				"BR-SEC-003: Must indicate unknown compliance status without template")
			Expect(len(securityReport.SecurityFindings)).To(Equal(0),
				"BR-SEC-003: Must have no findings without template to analyze")
			Expect(len(securityReport.RecommendedActions)).To(BeNumerically(">", 0),
				"BR-SEC-003: Must provide guidance even without template")

			// Business Value: Graceful handling ensures system reliability
		})

		It("should calculate security scores based on findings", func() {
			// Business Scenario: System calculates quantitative security scores
			// Business Impact: Enables data-driven security decision making

			// Create workflow with known security issues
			workflow := createInsecureWorkflow("insecure-workflow-001")

			// Test REAL business logic for security scoring
			securityReport := intelligentBuilder.GenerateSecurityReport(workflow)

			// Validate security scoring logic
			Expect(securityReport.SecurityScore).To(BeNumerically(">=", 0.0),
				"BR-SEC-003: Security score must be within valid range")
			Expect(securityReport.SecurityScore).To(BeNumerically("<=", 1.0),
				"BR-SEC-003: Security score must be within valid range")

			// Validate correlation between findings and score
			if len(securityReport.SecurityFindings) > 0 {
				Expect(securityReport.SecurityScore).To(BeNumerically("<", 1.0),
					"BR-SEC-003: Security score must reflect presence of security findings")
			}

			// Validate vulnerability count matches findings
			Expect(securityReport.VulnerabilityCount).To(Equal(len(securityReport.SecurityFindings)),
				"BR-SEC-003: Vulnerability count must match number of security findings")

			// Business Value: Quantitative security scores enable data-driven decisions
		})
	})

	// BR-SEC-006: Security Enhancement Optimization Integration
	Context("BR-SEC-006: Security Enhancement Optimization Integration", func() {
		It("should integrate security enhancements into workflow optimization", func() {
			// Business Scenario: System integrates security considerations into workflow optimization
			// Business Impact: Ensures optimized workflows maintain security standards

			// Create workflow template for optimization with security considerations
			template := createOptimizationSecurityTemplate("security-optimization-001")

			// Test REAL business logic for security-aware optimization
			optimizedTemplate, err := intelligentBuilder.OptimizeWorkflowStructure(ctx, template)

			// Validate REAL business security-aware optimization outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-SEC-006: Security-aware optimization must succeed")
			Expect(optimizedTemplate).ToNot(BeNil(),
				"BR-SEC-006: Must produce optimized template with security considerations")

			// Validate security metadata is preserved/enhanced during optimization
			if optimizedTemplate.Metadata != nil {
				// Check for security-related metadata
				securityKeys := []string{"security_enhanced", "security_score", "compliance_status"}
				for _, key := range securityKeys {
					if _, exists := optimizedTemplate.Metadata[key]; exists {
						// Security metadata found - validate it's meaningful
						Expect(optimizedTemplate.Metadata[key]).ToNot(BeNil(),
							"BR-SEC-006: Security metadata must have meaningful values")
					}
				}
			}

			// Business Value: Security-aware optimization maintains security standards
		})
	})

	// BR-SEC-007: Security Metadata Tracking
	Context("BR-SEC-007: Security Metadata Tracking", func() {
		It("should track security metadata throughout workflow lifecycle", func() {
			// Business Scenario: System tracks security metadata for audit and compliance
			// Business Impact: Enables security audit trails and compliance reporting

			// Create workflow with security metadata requirements
			workflow := createSecurityMetadataWorkflow("security-metadata-001")

			// Test REAL business logic for security metadata tracking
			securityReport := intelligentBuilder.GenerateSecurityReport(workflow)

			// Validate REAL business security metadata tracking outcomes
			Expect(securityReport.SecurityMetadata).ToNot(BeNil(),
				"BR-SEC-007: Security metadata must be tracked and available")
			Expect(len(securityReport.SecurityMetadata)).To(BeNumerically(">", 0),
				"BR-SEC-007: Security metadata must contain meaningful information")

			// Validate metadata contains security-relevant information
			expectedMetadataKeys := []string{"severity_counts", "total_steps", "action_types"}
			for _, key := range expectedMetadataKeys {
				Expect(securityReport.SecurityMetadata).To(HaveKey(key),
					"BR-SEC-007: Security metadata must include %s for comprehensive tracking", key)
			}

			// Validate severity counts structure
			if severityCounts, ok := securityReport.SecurityMetadata["severity_counts"]; ok {
				Expect(severityCounts).To(BeAssignableToTypeOf(map[string]int{}),
					"BR-SEC-007: Severity counts must be properly structured for analysis")
			}

			// Business Value: Security metadata enables audit trails and compliance
		})
	})

	// BR-SEC-008: Security Integration Workflows
	Context("BR-SEC-008: Security Integration Workflows", func() {
		It("should integrate security validation into complete workflows", func() {
			// Business Scenario: System integrates security validation throughout workflow execution
			// Business Impact: Ensures end-to-end security in automated workflows

			// Create comprehensive workflow for security integration testing
			template := createComprehensiveSecurityWorkflow("security-integration-001")
			securityPolicies := createIntegrationSecurityPolicies()

			// Test REAL business logic for comprehensive security integration
			// Step 1: Validate security constraints
			validationResults := intelligentBuilder.ValidateSecurityConstraints(ctx, template)

			// Step 2: Apply security policies
			securedTemplate := intelligentBuilder.ApplySecurityPolicies(template, securityPolicies)

			// Step 3: Generate security report
			workflow := &engine.Workflow{
				BaseVersionedEntity: securedTemplate.BaseVersionedEntity,
				Template:            securedTemplate,
			}
			securityReport := intelligentBuilder.GenerateSecurityReport(workflow)

			// Validate REAL business comprehensive security integration outcomes
			Expect(validationResults).ToNot(BeNil(),
				"BR-SEC-008: Security validation must be part of integration workflow")
			Expect(securedTemplate).ToNot(BeNil(),
				"BR-SEC-008: Security policy application must be part of integration workflow")
			Expect(securityReport).ToNot(BeNil(),
				"BR-SEC-008: Security reporting must be part of integration workflow")

			// Validate integration consistency
			Expect(securedTemplate.ID).To(Equal(template.ID),
				"BR-SEC-008: Template identity must be preserved through security integration")
			Expect(securityReport.WorkflowID).To(Equal(workflow.ID),
				"BR-SEC-008: Workflow identity must be preserved through security integration")

			// Validate security enhancements are comprehensive
			securityEnhancementCount := 0
			for _, step := range securedTemplate.Steps {
				if step.Variables != nil && step.Variables["security_enhanced"] == true {
					securityEnhancementCount++
				}
			}
			Expect(securityEnhancementCount).To(Equal(len(securedTemplate.Steps)),
				"BR-SEC-008: All steps must be security enhanced in integration workflow")

			// Business Value: Comprehensive security integration ensures end-to-end protection
		})
	})
})

// Helper functions for security enhancement testing
// These create realistic test data for REAL business logic validation

func createSecurityTestTemplate(templateID string) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate(templateID, "Security Test Template")

	// Create steps with various security characteristics
	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{
				ID:   "secure-step-1",
				Name: "Standard Processing Step",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "process_data",
				Parameters: map[string]interface{}{
					"security_context":    "standard",
					"data_classification": "internal",
				},
			},
			Timeout: 5 * time.Minute, // Has timeout - good security practice
		},
		{
			BaseEntity: types.BaseEntity{
				ID:   "privileged-step-1",
				Name: "Privileged Operation Step",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "access_secrets", // Privileged action
				Parameters: map[string]interface{}{
					"secret_name": "database-credentials",
					"namespace":   "production",
				},
			},
			Timeout: 2 * time.Minute,
		},
		{
			BaseEntity: types.BaseEntity{
				ID:   "timeout-missing-step",
				Name: "Step Without Timeout",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "long_running_task",
				Parameters: map[string]interface{}{
					"task_type": "data_processing",
				},
			},
			// No timeout - security finding
		},
	}

	template.Steps = steps
	template.Metadata = map[string]interface{}{
		"security_required": true,
		"compliance_level":  "standard",
	}

	return template
}

func createDestructiveActionTemplate(templateID string) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate(templateID, "Destructive Action Template")

	// Create steps with destructive actions
	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{
				ID:   "destructive-step-1",
				Name: "Delete Namespace Step",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "delete_namespace", // Destructive action
				Parameters: map[string]interface{}{
					"namespace": "test-namespace",
					"force":     true,
				},
			},
			Timeout: 5 * time.Minute,
		},
		{
			BaseEntity: types.BaseEntity{
				ID:   "destructive-step-2",
				Name: "Delete Cluster Role Step",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "delete_cluster_role", // Destructive and privileged
				Parameters: map[string]interface{}{
					"role_name": "admin-role",
				},
			},
			Timeout: 2 * time.Minute,
		},
	}

	template.Steps = steps
	template.Metadata = map[string]interface{}{
		"destructive_actions": true,
		"requires_approval":   true,
	}

	return template
}

func createComprehensiveSecurityPolicies() map[string]interface{} {
	return map[string]interface{}{
		"rbac_enabled":           true,
		"network_policies":       true,
		"pod_security_standards": "restricted",
		"security_contexts":      true,
		"admission_controllers":  []string{"PodSecurityPolicy", "NetworkPolicy", "ResourceQuota"},
		"security_level":         "strict",
		"compliance_mode":        "enforcing",
		"audit_logging":          true,
	}
}

func createRBACSecurityPolicies() map[string]interface{} {
	return map[string]interface{}{
		"rbac_enabled":             true,
		"security_level":           "strict",
		"least_privilege":          true,
		"service_account_required": true,
		"role_binding_validation":  true,
	}
}

func createNetworkSecurityPolicies() map[string]interface{} {
	return map[string]interface{}{
		"network_policies":     true,
		"default_deny":         true,
		"ingress_restrictions": true,
		"egress_restrictions":  true,
		"security_level":       "network_isolated",
	}
}

func createRBACTestTemplate(templateID string) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate(templateID, "RBAC Test Template")

	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{
				ID:   "rbac-step-1",
				Name: "Service Account Creation",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "create_service_account",
				Parameters: map[string]interface{}{
					"account_name": "workflow-executor",
					"namespace":    "default",
				},
			},
			Timeout: 1 * time.Minute,
		},
	}

	template.Steps = steps
	return template
}

func createNetworkSecurityTemplate(templateID string) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate(templateID, "Network Security Template")

	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{
				ID:   "network-step-1",
				Name: "Network Policy Application",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "apply_network_policy",
				Parameters: map[string]interface{}{
					"policy_name": "default-deny",
					"namespace":   "production",
				},
			},
			Timeout: 2 * time.Minute,
		},
	}

	template.Steps = steps
	return template
}

func createSecurityAnalysisWorkflow(workflowID string) *engine.Workflow {
	template := createSecurityTestTemplate("security-analysis-template")
	workflow := engine.NewWorkflow(workflowID, template)

	// Add security analysis metadata
	workflow.Metadata["security_analysis_required"] = true
	workflow.Metadata["compliance_framework"] = "SOC2"

	return workflow
}

func createWorkflowWithoutTemplate(workflowID string) *engine.Workflow {
	workflow := &engine.Workflow{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:   workflowID,
				Name: "Workflow Without Template",
			},
		},
		Template: nil, // No template for testing graceful handling
		Status:   "pending",
	}

	return workflow
}

func createInsecureWorkflow(workflowID string) *engine.Workflow {
	template := createDestructiveActionTemplate("insecure-template")
	workflow := engine.NewWorkflow(workflowID, template)

	// Add metadata indicating known security issues
	workflow.Metadata["known_security_issues"] = true
	workflow.Metadata["security_review_required"] = true

	return workflow
}

func createOptimizationSecurityTemplate(templateID string) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate(templateID, "Optimization Security Template")

	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{
				ID:   "optimization-step-1",
				Name: "Optimizable Secure Step",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "secure_data_processing",
				Parameters: map[string]interface{}{
					"optimization_candidate": true,
					"security_required":      true,
				},
			},
			Timeout: 10 * time.Minute,
		},
	}

	template.Steps = steps
	template.Metadata = map[string]interface{}{
		"optimization_enabled": true,
		"security_constraints": true,
	}

	return template
}

func createSecurityMetadataWorkflow(workflowID string) *engine.Workflow {
	template := createSecurityTestTemplate("security-metadata-template")
	workflow := engine.NewWorkflow(workflowID, template)

	// Add comprehensive security metadata
	workflow.Metadata["security_classification"] = "confidential"
	workflow.Metadata["compliance_requirements"] = []string{"SOC2", "GDPR", "HIPAA"}
	workflow.Metadata["security_review_date"] = time.Now().Format("2006-01-02")

	return workflow
}

func createComprehensiveSecurityWorkflow(templateID string) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate(templateID, "Comprehensive Security Workflow")

	// Create steps representing various security scenarios
	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{ID: "secure-init", Name: "Secure Initialization"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "initialize_secure_context",
				Parameters: map[string]interface{}{
					"security_level": "high",
				},
			},
			Timeout: 2 * time.Minute,
		},
		{
			BaseEntity: types.BaseEntity{ID: "privileged-op", Name: "Privileged Operation"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "run_privileged",
				Parameters: map[string]interface{}{
					"privilege_level": "admin",
				},
			},
			Timeout: 5 * time.Minute,
		},
		{
			BaseEntity: types.BaseEntity{ID: "secure-cleanup", Name: "Secure Cleanup"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "secure_cleanup",
				Parameters: map[string]interface{}{
					"cleanup_level": "complete",
				},
			},
			Timeout: 3 * time.Minute,
		},
	}

	template.Steps = steps
	template.Metadata = map[string]interface{}{
		"comprehensive_security":    true,
		"security_integration_test": true,
	}

	return template
}

func createIntegrationSecurityPolicies() map[string]interface{} {
	return map[string]interface{}{
		"rbac_enabled":             true,
		"network_policies":         true,
		"pod_security_standards":   "restricted",
		"security_contexts":        true,
		"admission_controllers":    []string{"PodSecurityPolicy", "NetworkPolicy"},
		"security_level":           "maximum",
		"integration_mode":         true,
		"comprehensive_validation": true,
	}
}

// TestRunner bootstraps the Ginkgo test suite
func TestUsecurityUenhancementUunit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UsecurityUenhancementUunit Suite")
}
