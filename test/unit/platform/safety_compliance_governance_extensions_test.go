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

package platform

import (
	"testing"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"

	"github.com/jordigilh/kubernaut/pkg/platform/safety"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// Phase 1: Platform Safety Framework Compliance & Governance Extensions
// Business Requirements: BR-SAFE-016 through BR-SAFE-020
// Following 00-project-guidelines.mdc: MANDATORY business requirement mapping
// Following 03-testing-strategy.mdc: PREFER real business logic over mocks
// Following 05-kubernetes-safety.mdc: Kubernetes operations and safety patterns

var _ = Describe("Safety Compliance & Governance Extensions - Phase 1 Business Requirements", func() {
	var (
		ctx             context.Context
		safetyValidator *safety.SafetyValidator
		fakeK8sClient   *fake.Clientset
		mockLogger      *mocks.MockLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockLogger = mocks.NewMockLogger()

		// Following 09-interface-method-validation.mdc: Use existing real implementations
		// Following 03-testing-strategy.mdc: PREFER real business logic over mocks
		fakeK8sClient = enhanced.NewSmartFakeClientset()
		safetyValidator = safety.NewSafetyValidator(fakeK8sClient, mockLogger.Logger)
	})

	// BR-SAFE-016: Policy-based action filtering
	Context("BR-SAFE-016: MUST implement policy-based action filtering", func() {
		It("should filter actions based on organizational policies for business compliance", func() {
			// Business Scenario: Enterprise needs to enforce organizational policies to prevent unauthorized actions
			// Business Impact: Policy-based filtering ensures compliance with corporate governance and reduces security risks

			// Setup organizational policies for business compliance
			restrictivePolicy := types.Alert{
				Name:      "HighRiskPolicyViolation",
				Namespace: "production",
				Resource:  "critical-database",
				Severity:  "critical",
				Labels: map[string]string{
					"environment": "production",
					"criticality": "high",
					"policy":      "restrictive",
				},
			}

			permissivePolicy := types.Alert{
				Name:      "LowRiskOperationalAlert",
				Namespace: "development",
				Resource:  "test-service",
				Severity:  "warning",
				Labels: map[string]string{
					"environment": "development",
					"criticality": "low",
					"policy":      "permissive",
				},
			}

			// Test high-risk action in production environment
			highRiskAction := types.ActionRecommendation{
				Action:     "delete_pod",
				Confidence: 0.85,
				Parameters: map[string]interface{}{
					"force":     true,
					"immediate": true,
				},
			}

			// Execute policy-based validation using real business logic
			productionRisk := safetyValidator.AssessRisk(ctx, highRiskAction, restrictivePolicy)
			Expect(productionRisk).ToNot(BeNil(), "BR-SAFE-016: Policy assessment must succeed for business compliance")

			// Business Requirement Validation: High-risk actions in production must be restricted
			Expect(productionRisk.RiskLevel).To(Equal("MEDIUM"),
				"BR-SAFE-016: Delete actions must be assessed as medium risk for policy compliance")
			Expect(productionRisk.SafeToExecute).To(BeFalse(),
				"BR-SAFE-016: High-risk actions in production must be blocked by policy for business safety")

			// Test low-risk action in development environment
			lowRiskAction := types.ActionRecommendation{
				Action:     "restart_pod",
				Confidence: 0.90,
				Parameters: map[string]interface{}{
					"graceful": true,
				},
			}

			// Execute policy validation for permissive environment
			developmentRisk := safetyValidator.AssessRisk(ctx, lowRiskAction, permissivePolicy)
			Expect(developmentRisk).ToNot(BeNil(), "BR-SAFE-016: Policy assessment must succeed for all environments")

			// Business Requirement Validation: Low-risk actions in development should be permitted
			Expect(developmentRisk.RiskLevel).To(Equal("LOW"),
				"BR-SAFE-016: Restart actions must be assessed as low risk for business operational flexibility")
			Expect(developmentRisk.SafeToExecute).To(BeTrue(),
				"BR-SAFE-016: Low-risk actions in development must be permitted by policy for business agility")

			// Validate policy-based risk factors
			Expect(productionRisk.RiskFactors).To(ContainElement("service interruption"),
				"BR-SAFE-016: Must identify business impact factors for policy decision making")
			Expect(productionRisk.Mitigation).ToNot(BeEmpty(),
				"BR-SAFE-016: Must provide mitigation strategies for business risk management")
		})

		It("should support multi-environment policy differentiation for business flexibility", func() {
			// Business Scenario: Different environments require different policy enforcement levels
			// Business Impact: Flexible policies enable development velocity while maintaining production safety

			environments := []struct {
				name        string
				namespace   string
				policyLevel string
				expectation string
			}{
				{"production", "prod-ns", "strict", "HIGH_SECURITY"},
				{"staging", "staging-ns", "moderate", "BALANCED"},
				{"development", "dev-ns", "permissive", "FLEXIBLE"},
				{"sandbox", "sandbox-ns", "experimental", "MINIMAL"},
			}

			// Test policy enforcement across different environments
			testAction := types.ActionRecommendation{
				Action:     "scale_deployment",
				Confidence: 0.80,
				Parameters: map[string]interface{}{
					"replicas": 10,
				},
			}

			for _, env := range environments {
				// Create environment-specific alert
				envAlert := types.Alert{
					Name:      "ResourceScalingAlert",
					Namespace: env.namespace,
					Resource:  "web-application",
					Severity:  "warning",
					Labels: map[string]string{
						"environment":   env.name,
						"policy_level":  env.policyLevel,
						"business_unit": "engineering",
					},
				}

				// Execute environment-specific policy validation
				envRisk := safetyValidator.AssessRisk(ctx, testAction, envAlert)
				Expect(envRisk).ToNot(BeNil(),
					"BR-SAFE-016: Policy assessment must succeed for %s environment", env.name)

				// Business Requirement Validation: Different environments should have appropriate risk levels
				switch env.name {
				case "production":
					Expect(envRisk.RiskLevel).To(BeElementOf([]string{"MEDIUM", "HIGH"}),
						"BR-SAFE-016: Production environment must enforce strict policies for business protection")
				case "development", "sandbox":
					Expect(envRisk.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM"}),
						"BR-SAFE-016: Development environments must allow flexibility for business innovation")
				}

				// Validate business-appropriate confidence levels
				Expect(envRisk.Confidence).To(BeNumerically(">=", 0.5),
					"BR-SAFE-016: Policy decisions must have adequate confidence for business trust")
			}
		})
	})

	// BR-SAFE-017: Compliance rule validation
	Context("BR-SAFE-017: MUST support compliance rule validation", func() {
		It("should validate actions against regulatory compliance requirements for business governance", func() {
			// Business Scenario: Financial services company needs SOX compliance for production changes
			// Business Impact: Compliance validation prevents regulatory violations and ensures audit readiness

			// Setup compliance-sensitive alert scenario
			complianceAlert := types.Alert{
				Name:      "DatabasePerformanceDegradation",
				Namespace: "financial-prod",
				Resource:  "customer-database",
				Severity:  "critical",
				Labels: map[string]string{
					"compliance":  "sox-required",
					"data_class":  "customer_pii",
					"environment": "production",
					"audit_level": "high",
				},
			}

			// Test action requiring compliance validation
			complianceAction := types.ActionRecommendation{
				Action:     "restart_pods",
				Confidence: 0.85,
				Parameters: map[string]interface{}{
					"target":      "database-cluster",
					"force":       false,
					"audit_trail": true,
				},
			}

			// Execute compliance rule validation using real business logic
			complianceRisk := safetyValidator.AssessRisk(ctx, complianceAction, complianceAlert)
			Expect(complianceRisk).ToNot(BeNil(), "BR-SAFE-017: Compliance assessment must succeed for regulatory validation")

			// Business Requirement Validation: Compliance rules must influence risk assessment
			Expect(complianceRisk.RiskLevel).To(BeElementOf([]string{"MEDIUM", "HIGH"}),
				"BR-SAFE-017: Compliance-sensitive actions must receive appropriate risk classification")
			Expect(complianceRisk.Metadata).To(HaveKey("compliance_assessment"),
				"BR-SAFE-017: Must track compliance validation for audit requirements")

			// Validate compliance-specific risk factors
			Expect(complianceRisk.RiskFactors).ToNot(BeEmpty(),
				"BR-SAFE-017: Must identify compliance-related risk factors for business governance")

			// Validate mitigation includes compliance considerations
			Expect(complianceRisk.Mitigation).To(ContainSubstring("audit"),
				"BR-SAFE-017: Mitigation must include audit considerations for compliance readiness")
		})

		It("should implement different compliance frameworks for business regulatory alignment", func() {
			// Business Scenario: Multi-national company operates under different regulatory frameworks
			// Business Impact: Framework-specific compliance ensures global business operations remain compliant

			complianceFrameworks := []struct {
				framework    string
				region       string
				requirements []string
			}{
				{
					framework:    "SOX",
					region:       "us-east",
					requirements: []string{"audit_trail", "change_approval", "segregation_of_duties"},
				},
				{
					framework:    "GDPR",
					region:       "eu-west",
					requirements: []string{"data_protection", "privacy_impact", "consent_tracking"},
				},
				{
					framework:    "HIPAA",
					region:       "us-central",
					requirements: []string{"phi_protection", "access_logging", "encryption_required"},
				},
			}

			// Test compliance validation across different frameworks
			testAction := types.ActionRecommendation{
				Action:     "update_configmap",
				Confidence: 0.75,
				Parameters: map[string]interface{}{
					"data_sensitive": true,
				},
			}

			for _, framework := range complianceFrameworks {
				// Create framework-specific alert
				frameworkAlert := types.Alert{
					Name:      "ConfigurationChangeRequired",
					Namespace: framework.region + "-prod",
					Resource:  "application-config",
					Severity:  "high",
					Labels: map[string]string{
						"compliance_framework": framework.framework,
						"region":               framework.region,
						"data_classification":  "sensitive",
					},
				}

				// Execute framework-specific compliance validation
				frameworkRisk := safetyValidator.AssessRisk(ctx, testAction, frameworkAlert)
				Expect(frameworkRisk).ToNot(BeNil(),
					"BR-SAFE-017: Compliance assessment must succeed for %s framework", framework.framework)

				// Business Requirement Validation: Framework-specific compliance considerations
				Expect(frameworkRisk.RiskLevel).To(BeElementOf([]string{"MEDIUM", "HIGH"}),
					"BR-SAFE-017: Sensitive data actions must be appropriately risk-classified for %s", framework.framework)

				// Validate framework-appropriate confidence and metadata
				Expect(frameworkRisk.Confidence).To(BeNumerically(">=", 0.6),
					"BR-SAFE-017: Compliance decisions must have adequate confidence for business governance")
				Expect(frameworkRisk.Metadata).ToNot(BeEmpty(),
					"BR-SAFE-017: Must maintain compliance metadata for audit and regulatory reporting")
			}
		})
	})

	// BR-SAFE-018: Audit trails for all safety decisions
	Context("BR-SAFE-018: MUST maintain audit trails for all safety decisions", func() {
		It("should create comprehensive audit trails for business accountability and compliance", func() {
			// Business Scenario: Auditors need complete visibility into safety decisions for compliance validation
			// Business Impact: Comprehensive audit trails enable regulatory compliance and incident investigation

			// Setup auditable action scenario
			auditableAlert := types.Alert{
				Name:      "SecurityIncidentResponse",
				Namespace: "security-prod",
				Resource:  "compromised-service",
				Severity:  "critical",
				Labels: map[string]string{
					"incident_id":    "SEC-2025-001",
					"audit_required": "true",
					"business_unit":  "security",
				},
			}

			auditableAction := types.ActionRecommendation{
				Action:     "quarantine_pod",
				Confidence: 0.90,
				Parameters: map[string]interface{}{
					"isolation_level": "network",
					"preserve_logs":   true,
				},
			}

			// Execute action with audit trail generation
			auditRisk := safetyValidator.AssessRisk(ctx, auditableAction, auditableAlert)
			Expect(auditRisk).ToNot(BeNil(), "BR-SAFE-018: Audit trail generation must succeed for business accountability")

			// Business Requirement Validation: Audit information must be comprehensive
			Expect(auditRisk.ActionName).To(Equal("quarantine_pod"),
				"BR-SAFE-018: Audit trail must record exact action for business traceability")
			Expect(auditRisk.RiskLevel).ToNot(BeEmpty(),
				"BR-SAFE-018: Audit trail must record risk assessment for business compliance")
			Expect(auditRisk.Confidence).To(BeNumerically(">", 0),
				"BR-SAFE-018: Audit trail must record decision confidence for business validation")

			// Validate audit metadata for business accountability
			Expect(auditRisk.Metadata).To(HaveKey("timestamp"),
				"BR-SAFE-018: Audit trail must include timestamps for business chronological tracking")
			Expect(auditRisk.Metadata).To(HaveKey("decision_factors"),
				"BR-SAFE-018: Audit trail must record decision reasoning for business transparency")

			// Validate business context preservation
			Expect(auditRisk.RiskFactors).ToNot(BeEmpty(),
				"BR-SAFE-018: Audit trail must preserve risk factors for business incident analysis")
			Expect(auditRisk.Mitigation).ToNot(BeEmpty(),
				"BR-SAFE-018: Audit trail must record mitigation strategies for business learning")
		})

		It("should support audit trail aggregation and reporting for business intelligence", func() {
			// Business Scenario: Management needs periodic safety decision reports for business oversight
			// Business Impact: Aggregated audit data enables trend analysis and process improvement

			// Simulate multiple safety decisions for audit aggregation
			auditScenarios := []struct {
				actionType   string
				riskLevel    string
				namespace    string
				businessUnit string
			}{
				{"restart_pod", "LOW", "app-prod", "engineering"},
				{"scale_deployment", "MEDIUM", "api-prod", "platform"},
				{"drain_node", "HIGH", "infra-prod", "infrastructure"},
				{"rollback_deployment", "MEDIUM", "web-prod", "frontend"},
			}

			auditResults := make([]*safety.RiskAssessment, 0, len(auditScenarios))

			for i, scenario := range auditScenarios {
				// Create scenario-specific alert and action
				alert := types.Alert{
					Name:      "OperationalAlert" + string(rune(i)),
					Namespace: scenario.namespace,
					Resource:  "application-service",
					Severity:  "warning",
					Labels: map[string]string{
						"business_unit": scenario.businessUnit,
						"audit_batch":   "daily_report",
					},
				}

				action := types.ActionRecommendation{
					Action:     scenario.actionType,
					Confidence: 0.80,
					Parameters: map[string]interface{}{
						"automated": true,
					},
				}

				// Execute safety decision with audit capture
				risk := safetyValidator.AssessRisk(ctx, action, alert)
				Expect(risk).ToNot(BeNil(),
					"BR-SAFE-018: Audit capture must succeed for scenario %s", scenario.actionType)

				auditResults = append(auditResults, risk)
			}

			// Business Requirement Validation: Audit aggregation capabilities
			Expect(len(auditResults)).To(Equal(4),
				"BR-SAFE-018: Must capture all safety decisions for comprehensive business reporting")

			// Validate audit data consistency for business intelligence
			for i, result := range auditResults {
				expectedAction := auditScenarios[i].actionType
				Expect(result.ActionName).To(Equal(expectedAction),
					"BR-SAFE-018: Audit trail must maintain action type accuracy for business analytics")
				Expect(result.Confidence).To(BeNumerically(">=", 0.5),
					"BR-SAFE-018: Audit trail must record meaningful confidence for business decision analysis")
			}

			// Validate business reporting capabilities
			actionTypes := make(map[string]int)
			riskLevels := make(map[string]int)

			for _, result := range auditResults {
				actionTypes[result.ActionName]++
				riskLevels[result.RiskLevel]++
			}

			Expect(len(actionTypes)).To(BeNumerically(">=", 3),
				"BR-SAFE-018: Must capture diverse action types for comprehensive business analysis")
			Expect(len(riskLevels)).To(BeNumerically(">=", 2),
				"BR-SAFE-018: Must capture varied risk levels for business risk management insights")
		})
	})

	// BR-SAFE-019: Governance reporting and compliance metrics
	Context("BR-SAFE-019: MUST provide governance reporting and compliance metrics", func() {
		It("should generate governance reports for business executive oversight", func() {
			// Business Scenario: Executive team needs governance dashboards for board reporting
			// Business Impact: Governance metrics enable executive decision making and stakeholder confidence

			// Setup governance-focused safety scenarios
			governanceAlert := types.Alert{
				Name:      "GovernanceComplianceCheck",
				Namespace: "governance-prod",
				Resource:  "critical-system",
				Severity:  "high",
				Labels: map[string]string{
					"governance_scope": "enterprise",
					"report_period":    "quarterly",
					"stakeholder":      "board",
				},
			}

			governanceAction := types.ActionRecommendation{
				Action:     "update_security_policy",
				Confidence: 0.85,
				Parameters: map[string]interface{}{
					"policy_scope": "enterprise_wide",
					"compliance":   "mandatory",
				},
			}

			// Execute governance-level safety assessment
			governanceRisk := safetyValidator.AssessRisk(ctx, governanceAction, governanceAlert)
			Expect(governanceRisk).ToNot(BeNil(), "BR-SAFE-019: Governance assessment must succeed for executive reporting")

			// Business Requirement Validation: Governance-appropriate risk assessment
			Expect(governanceRisk.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM", "HIGH"}),
				"BR-SAFE-019: Governance actions must receive appropriate risk classification for executive decision making")
			Expect(governanceRisk.Confidence).To(BeNumerically(">=", 0.7),
				"BR-SAFE-019: Governance decisions must have high confidence for stakeholder trust")

			// Validate governance metadata for business reporting
			Expect(governanceRisk.Metadata).To(HaveKey("governance_impact"),
				"BR-SAFE-019: Must track governance impact for executive dashboards")
			Expect(governanceRisk.ActionName).To(Equal("update_security_policy"),
				"BR-SAFE-019: Must accurately record governance actions for compliance reporting")

			// Validate business-appropriate risk factors
			Expect(governanceRisk.RiskFactors).ToNot(BeEmpty(),
				"BR-SAFE-019: Must identify governance risk factors for executive risk management")
			Expect(governanceRisk.Mitigation).ToNot(BeEmpty(),
				"BR-SAFE-019: Must provide governance mitigation strategies for board confidence")
		})

		It("should track compliance metrics for business regulatory reporting", func() {
			// Business Scenario: Compliance team needs metrics for regulatory submissions
			// Business Impact: Compliance metrics ensure regulatory requirements are met and violations are prevented

			// Setup compliance metrics tracking scenario
			complianceScenarios := []struct {
				complianceType string
				severity       string
				expectedRisk   string
			}{
				{"data_protection", "critical", "HIGH"},
				{"access_control", "high", "MEDIUM"},
				{"audit_logging", "medium", "LOW"},
				{"change_management", "high", "MEDIUM"},
			}

			complianceMetrics := make(map[string]*safety.RiskAssessment)

			for _, scenario := range complianceScenarios {
				// Create compliance-specific alert
				complianceAlert := types.Alert{
					Name:      "ComplianceValidation",
					Namespace: "compliance-prod",
					Resource:  "regulated-service",
					Severity:  scenario.severity,
					Labels: map[string]string{
						"compliance_type":  scenario.complianceType,
						"regulatory_scope": "enterprise",
						"audit_frequency":  "monthly",
					},
				}

				complianceAction := types.ActionRecommendation{
					Action:     "validate_compliance",
					Confidence: 0.88,
					Parameters: map[string]interface{}{
						"validation_scope": scenario.complianceType,
						"audit_trail":      true,
					},
				}

				// Execute compliance metrics collection
				complianceRisk := safetyValidator.AssessRisk(ctx, complianceAction, complianceAlert)
				Expect(complianceRisk).ToNot(BeNil(),
					"BR-SAFE-019: Compliance metrics collection must succeed for %s", scenario.complianceType)

				complianceMetrics[scenario.complianceType] = complianceRisk
			}

			// Business Requirement Validation: Comprehensive compliance coverage
			Expect(len(complianceMetrics)).To(Equal(4),
				"BR-SAFE-019: Must track all compliance areas for comprehensive regulatory reporting")

			// Validate compliance metrics quality for business reporting
			for complianceType, metrics := range complianceMetrics {
				Expect(metrics.ActionName).To(Equal("validate_compliance"),
					"BR-SAFE-019: Compliance metrics must accurately record validation actions for %s", complianceType)
				Expect(metrics.Confidence).To(BeNumerically(">=", 0.8),
					"BR-SAFE-019: Compliance assessments must have high confidence for regulatory credibility")
				Expect(metrics.RiskLevel).ToNot(BeEmpty(),
					"BR-SAFE-019: Must assign risk levels for compliance risk management")
			}

			// Validate business regulatory reporting capabilities
			highRiskCompliance := 0
			for _, metrics := range complianceMetrics {
				if metrics.RiskLevel == "HIGH" {
					highRiskCompliance++
				}
			}

			// Business intelligence: Track high-risk compliance areas for prioritization
			Expect(highRiskCompliance).To(BeNumerically("<=", 2),
				"BR-SAFE-019: High-risk compliance areas must be manageable for business remediation")
		})
	})

	// BR-SAFE-020: External policy integration (OPA, Gatekeeper)
	Context("BR-SAFE-020: MUST support external policy integration", func() {
		It("should integrate with Open Policy Agent for enterprise policy enforcement", func() {
			// Business Scenario: Enterprise uses OPA for centralized policy management
			// Business Impact: OPA integration ensures consistent policy enforcement across all business operations

			// Setup OPA-integrated policy scenario
			opaAlert := types.Alert{
				Name:      "OPAPolicyViolation",
				Namespace: "policy-managed",
				Resource:  "regulated-deployment",
				Severity:  "high",
				Labels: map[string]string{
					"policy_engine":   "opa",
					"policy_version":  "v2.1",
					"enforcement":     "strict",
					"business_impact": "high",
				},
			}

			opaAction := types.ActionRecommendation{
				Action:     "enforce_policy",
				Confidence: 0.92,
				Parameters: map[string]interface{}{
					"policy_engine": "opa",
					"policy_set":    "enterprise_security",
					"enforcement":   "immediate",
				},
			}

			// Execute OPA policy integration assessment
			opaRisk := safetyValidator.AssessRisk(ctx, opaAction, opaAlert)
			Expect(opaRisk).ToNot(BeNil(), "BR-SAFE-020: OPA policy integration must succeed for enterprise policy enforcement")

			// Business Requirement Validation: OPA integration quality
			Expect(opaRisk.RiskLevel).To(BeElementOf([]string{"MEDIUM", "HIGH"}),
				"BR-SAFE-020: Policy enforcement actions must receive appropriate risk assessment for business governance")
			Expect(opaRisk.Confidence).To(BeNumerically(">=", 0.85),
				"BR-SAFE-020: OPA-integrated decisions must have high confidence for enterprise trust")

			// Validate OPA-specific business metadata
			Expect(opaRisk.ActionName).To(Equal("enforce_policy"),
				"BR-SAFE-020: Must accurately record OPA policy actions for business audit trails")
			Expect(opaRisk.Metadata).To(HaveKey("policy_integration"),
				"BR-SAFE-020: Must track external policy integration for business compliance reporting")

			// Validate business policy enforcement considerations
			Expect(opaRisk.RiskFactors).To(ContainElement("policy enforcement"),
				"BR-SAFE-020: Must identify policy-related risk factors for business decision making")
			Expect(opaRisk.Mitigation).To(ContainSubstring("policy"),
				"BR-SAFE-020: Mitigation must include policy considerations for business compliance")
		})

		It("should support Gatekeeper integration for Kubernetes-native policy enforcement", func() {
			// Business Scenario: Kubernetes platform team uses Gatekeeper for admission control
			// Business Impact: Gatekeeper integration ensures consistent policy enforcement in Kubernetes environments

			// Setup Gatekeeper-integrated policy scenario
			gatekeeperAlert := types.Alert{
				Name:      "GatekeeperPolicyViolation",
				Namespace: "gatekeeper-system",
				Resource:  "admission-webhook",
				Severity:  "critical",
				Labels: map[string]string{
					"policy_engine":       "gatekeeper",
					"constraint_template": "security_baseline",
					"violation_type":      "admission_control",
					"business_scope":      "platform_wide",
				},
			}

			gatekeeperAction := types.ActionRecommendation{
				Action:     "update_gatekeeper_policy",
				Confidence: 0.87,
				Parameters: map[string]interface{}{
					"constraint_scope":  "cluster_wide",
					"policy_strictness": "enforced",
					"business_impact":   "platform_security",
				},
			}

			// Execute Gatekeeper policy integration assessment
			gatekeeperRisk := safetyValidator.AssessRisk(ctx, gatekeeperAction, gatekeeperAlert)
			Expect(gatekeeperRisk).ToNot(BeNil(), "BR-SAFE-020: Gatekeeper policy integration must succeed for Kubernetes governance")

			// Business Requirement Validation: Gatekeeper integration effectiveness
			Expect(gatekeeperRisk.RiskLevel).To(BeElementOf([]string{"MEDIUM", "HIGH"}),
				"BR-SAFE-020: Gatekeeper policy actions must receive appropriate risk classification for platform security")
			Expect(gatekeeperRisk.Confidence).To(BeNumerically(">=", 0.8),
				"BR-SAFE-020: Gatekeeper-integrated decisions must have adequate confidence for platform trust")

			// Validate Gatekeeper-specific business considerations
			Expect(gatekeeperRisk.ActionName).To(Equal("update_gatekeeper_policy"),
				"BR-SAFE-020: Must accurately record Gatekeeper actions for business platform management")
			Expect(gatekeeperRisk.Metadata).To(HaveKey("kubernetes_native"),
				"BR-SAFE-020: Must track Kubernetes-native policy integration for business platform governance")

			// Validate business platform security factors
			Expect(gatekeeperRisk.RiskFactors).ToNot(BeEmpty(),
				"BR-SAFE-020: Must identify platform security risk factors for business Kubernetes governance")
			Expect(gatekeeperRisk.SafeToExecute).To(BeFalse(),
				"BR-SAFE-020: High-impact policy changes must require approval for business platform stability")
		})

		It("should support multiple external policy engines for enterprise flexibility", func() {
			// Business Scenario: Large enterprise uses multiple policy engines across different business units
			// Business Impact: Multi-engine support enables business unit autonomy while maintaining governance

			policyEngines := []struct {
				engine       string
				businessUnit string
				policyScope  string
				riskProfile  string
			}{
				{"opa", "security", "enterprise_wide", "high_control"},
				{"gatekeeper", "platform", "kubernetes_native", "medium_control"},
				{"custom_rbac", "development", "team_specific", "low_control"},
				{"compliance_engine", "legal", "regulatory", "strict_control"},
			}

			engineAssessments := make(map[string]*safety.RiskAssessment)

			for _, engine := range policyEngines {
				// Create engine-specific policy scenario
				engineAlert := types.Alert{
					Name:      "MultiEnginePolicyCheck",
					Namespace: engine.businessUnit + "-prod",
					Resource:  "business-application",
					Severity:  "high",
					Labels: map[string]string{
						"policy_engine": engine.engine,
						"business_unit": engine.businessUnit,
						"policy_scope":  engine.policyScope,
						"risk_profile":  engine.riskProfile,
					},
				}

				engineAction := types.ActionRecommendation{
					Action:     "validate_multi_engine_policy",
					Confidence: 0.85,
					Parameters: map[string]interface{}{
						"engine":            engine.engine,
						"integration_scope": engine.policyScope,
						"business_context":  engine.businessUnit,
					},
				}

				// Execute multi-engine policy assessment
				engineRisk := safetyValidator.AssessRisk(ctx, engineAction, engineAlert)
				Expect(engineRisk).ToNot(BeNil(),
					"BR-SAFE-020: Multi-engine policy integration must succeed for %s engine", engine.engine)

				engineAssessments[engine.engine] = engineRisk
			}

			// Business Requirement Validation: Multi-engine support quality
			Expect(len(engineAssessments)).To(Equal(4),
				"BR-SAFE-020: Must support all enterprise policy engines for comprehensive business governance")

			// Validate engine-specific business considerations
			for engineName, assessment := range engineAssessments {
				Expect(assessment.ActionName).To(Equal("validate_multi_engine_policy"),
					"BR-SAFE-020: Must accurately record policy engine actions for %s", engineName)
				Expect(assessment.Confidence).To(BeNumerically(">=", 0.7),
					"BR-SAFE-020: Multi-engine assessments must have adequate confidence for business trust")
				Expect(assessment.RiskLevel).ToNot(BeEmpty(),
					"BR-SAFE-020: Must assign appropriate risk levels for business policy management")
			}

			// Validate business governance consistency across engines
			confidenceSum := 0.0
			for _, assessment := range engineAssessments {
				confidenceSum += assessment.Confidence
			}
			averageConfidence := confidenceSum / float64(len(engineAssessments))

			Expect(averageConfidence).To(BeNumerically(">=", 0.8),
				"BR-SAFE-020: Multi-engine policy integration must maintain high confidence for enterprise governance")
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUsafetyUcomplianceUgovernanceUextensions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UsafetyUcomplianceUgovernanceUextensions Suite")
}
