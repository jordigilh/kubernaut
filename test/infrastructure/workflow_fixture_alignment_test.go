/*
Copyright 2026 Jordi Gil.

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

package infrastructure

import (
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

var _ = Describe("AIAnalysis workflow fixture context", func() {
	It("UT-WORKFLOW-004-001: test signal workflow matches supported signal contexts", func() {
		content, err := readWorkflowFixtureContent("test-signal-handler")
		Expect(err).NotTo(HaveOccurred())

		workflow := &rwv1alpha1.RemediationWorkflow{}
		err = yaml.Unmarshal([]byte(content), workflow)
		Expect(err).NotTo(HaveOccurred())
		Expect(workflow.Spec.ActionType).To(Equal("DeletePod"))
		Expect(workflow.Spec.Labels.Severity).To(ContainElements("critical", "high", "warning", "info"))
		Expect(workflow.Spec.Labels.Priority).To(Equal("*"))
	})

	It("UT-WORKFLOW-004-003: generic restart workflow matches arbitrary Pod priorities", func() {
		content, err := readWorkflowFixtureContent("generic-restart")
		Expect(err).NotTo(HaveOccurred())

		workflow := &rwv1alpha1.RemediationWorkflow{}
		Expect(yaml.Unmarshal([]byte(content), workflow)).To(Succeed())
		Expect(workflow.Spec.Labels.Component).To(Equal([]string{"v1/Pod"}))
		Expect(workflow.Spec.Labels.Environment).To(Equal([]string{"production", "staging", "test", "development"}))
		Expect(workflow.Spec.Labels.Priority).To(Equal("*"))
	})

	It("UT-WORKFLOW-004-004: Fleet and local E2E lanes seed their own workflow fixtures", func() {
		// Fleet E2E-014/012 and E2E-015 consume only these job-backed fixtures;
		// Fleet-exec has its own fixture seeded by SetupFleetE2EInfrastructure.
		fleetSeeds := fullPipelineWorkflowSeeds(true)
		Expect(fleetSeeds).To(ConsistOf(
			WorkflowSeedSpec{FixtureDir: "crashloop-config-fix-job", Environment: "production"},
			WorkflowSeedSpec{FixtureDir: "oomkill-increase-memory-job", Environment: "production"},
		))

		localSeeds := fullPipelineWorkflowSeeds(false)
		Expect(localSeeds).To(ConsistOf(
			WorkflowSeedSpec{FixtureDir: "crashloop-config-fix-job", Environment: "production"},
			WorkflowSeedSpec{FixtureDir: "oomkill-increase-memory-job", Environment: "production"},
			WorkflowSeedSpec{FixtureDir: "gitops-drift-2390", Environment: "production"},
			WorkflowSeedSpec{FixtureDir: "standalone-exec-cluster-id", Environment: "production"},
			WorkflowSeedSpec{FixtureDir: "fix-certificate", Environment: "production"},
			WorkflowSeedSpec{FixtureDir: "generic-restart", Environment: "production"},
			WorkflowSeedSpec{FixtureDir: "crashloop-config-fix-job", Environment: "staging"},
			WorkflowSeedSpec{FixtureDir: "oomkill-increase-memory-job", Environment: "staging"},
			WorkflowSeedSpec{FixtureDir: "gitops-drift-2390", Environment: "staging"},
			WorkflowSeedSpec{FixtureDir: "standalone-exec-cluster-id", Environment: "staging"},
			WorkflowSeedSpec{FixtureDir: "generic-restart", Environment: "staging"},
			WorkflowSeedSpec{FixtureDir: "fullpipeline-consent-job", Environment: "staging"},
		))
	})

	It("UT-WORKFLOW-004-006: consent discovery has an exact Pod-matching Job workflow", func() {
		content, err := readWorkflowFixtureContent("fullpipeline-consent-job")
		Expect(err).NotTo(HaveOccurred())

		workflow := &rwv1alpha1.RemediationWorkflow{}
		Expect(yaml.Unmarshal([]byte(content), workflow)).To(Succeed())
		Expect(workflow.Spec.Labels.Severity).To(Equal([]string{"warning"}))
		Expect(workflow.Spec.Labels.Environment).To(Equal([]string{"staging"}))
		Expect(workflow.Spec.Labels.Component).To(Equal([]string{"v1/Pod"}))
		Expect(workflow.Spec.Execution.Engine).To(Equal("job"))
	})

	It("UT-WORKFLOW-004-005: A2A selectors resolve the workflow UUID for the target environment", func() {
		workflowUUIDs := map[string]string{
			"oomkill-increase-memory-v1:production": "oom-production-uuid",
			"oomkill-increase-memory-v1:staging":    "oom-staging-uuid",
			"generic-restart-v1:production":         "restart-production-uuid",
			"generic-restart-v1:staging":            "restart-staging-uuid",
		}

		Expect(resolveWorkflowUUIDForEnvironment(workflowUUIDs, "oomkill-increase-memory-v1", "staging")).To(Equal("oom-staging-uuid"))
		Expect(resolveWorkflowUUIDForEnvironment(workflowUUIDs, "oomkill-increase-memory-v1", "production")).To(Equal("oom-production-uuid"))
		Expect(resolveWorkflowUUIDForEnvironment(workflowUUIDs, "generic-restart-v1", "staging")).To(Equal("restart-staging-uuid"))
	})

	It("UT-WORKFLOW-004-007: pipeline default discovery selects the executable staging Job", func() {
		pipelineWorkflowUUIDs := map[string]string{
			"generic-restart-v1:staging":          "restart-staging-uuid",
			"fullpipeline-consent-job-v1:staging": "consent-staging-uuid",
		}
		legacyWorkflowUUIDs := map[string]string{
			"generic-restart-v1:staging": "restart-staging-uuid",
		}

		Expect(resolveDefaultWorkflowUUID(pipelineWorkflowUUIDs)).To(Equal("consent-staging-uuid"))
		Expect(resolveDefaultWorkflowUUID(legacyWorkflowUUIDs)).To(Equal("restart-staging-uuid"))
	})

	It("UT-WORKFLOW-004-009 (BR-INTERACTIVE-010, AC-6): consent selectors use a seeded workflow in each E2E lane", func() {
		fleetWorkflowUUIDs := map[string]string{
			"oomkill-increase-memory-v1:production": "oom-production-uuid",
		}
		fullPipelineWorkflowUUIDs := map[string]string{
			"oomkill-increase-memory-v1:staging":  "oom-staging-uuid",
			"fullpipeline-consent-job-v1:staging": "consent-staging-uuid",
		}

		fleetFallback := resolveWorkflowUUIDForEnvironment(fleetWorkflowUUIDs, "oomkill-increase-memory-v1", "staging")
		fullPipelineFallback := resolveWorkflowUUIDForEnvironment(fullPipelineWorkflowUUIDs, "oomkill-increase-memory-v1", "staging")

		Expect(fleetFallback).To(Equal("oom-production-uuid"))
		Expect(resolveConsentWorkflowUUID(fullPipelineWorkflowUUIDs, fullPipelineFallback)).To(Equal("consent-staging-uuid"),
			"FullPipeline should retain its exact-Pod consent fixture")
		Expect(resolveConsentWorkflowUUID(fleetWorkflowUUIDs, fleetFallback)).To(Equal("oom-production-uuid"),
			"Fleet should fall back to the catalog UUID it actually seeded")
	})

	It("UT-WORKFLOW-004-008: GitOps FullPipeline fixture admits the SP-derived warning severity and RCA Pod target", func() {
		content, err := readWorkflowFixtureContent("gitops-drift-2390")
		Expect(err).NotTo(HaveOccurred())

		workflow := &rwv1alpha1.RemediationWorkflow{}
		Expect(yaml.Unmarshal([]byte(content), workflow)).To(Succeed())
		Expect(workflow.Spec.Labels.Severity).To(Equal([]string{"warning"}))
		Expect(workflow.Spec.Labels.Environment).To(ContainElement("staging"))
		Expect(workflow.Spec.Labels.Component).To(ContainElements("apps/v1/Deployment", "v1/Pod"))
	})

	DescribeTable("UT-WORKFLOW-004-002: isolated AIAnalysis fixtures keep exact label contracts",
		func(fixture, actionType string, severity []string, environment string, component []string, priority string) {
			content, err := readWorkflowFixtureContent(fixture)
			Expect(err).NotTo(HaveOccurred())

			workflow := &rwv1alpha1.RemediationWorkflow{}
			Expect(yaml.Unmarshal([]byte(content), workflow)).To(Succeed())
			Expect(workflow.Spec.ActionType).To(Equal(actionType))
			Expect(workflow.Spec.Labels.Severity).To(Equal(severity))
			Expect(workflow.Spec.Labels.Environment).To(Equal([]string{environment}))
			Expect(workflow.Spec.Labels.Component).To(Equal(component))
			Expect(workflow.Spec.Labels.Priority).To(Equal(priority))
		},
		Entry("staging OOM", "oomkill-increase-memory-aa-staging", "IncreaseMemoryLimits", []string{"warning"}, "staging", []string{"*"}, "P2"),
		Entry("production approval CrashLoop", "crashloop-config-fix-aa-approval", "RestartDeployment", []string{"high"}, "production", []string{"apps/v1/Deployment"}, "P1"),
		Entry("production audit CrashLoop", "crashloop-config-fix-aa-audit", "RestartDeployment", []string{"warning"}, "production", []string{"apps/v1/Deployment"}, "P1"),
		Entry("staging Rego CrashLoop", "crashloop-config-fix-aa-rego", "RestartDeployment", []string{"warning"}, "staging", []string{"apps/v1/Deployment"}, "P1"),
		Entry("staging session CrashLoop", "crashloop-config-fix-aa-session", "RestartDeployment", []string{"warning"}, "staging", []string{"*"}, "P2"),
		Entry("production detected-labels CrashLoop", "crashloop-config-fix-aa-detected-labels", "RestartDeployment", []string{"critical"}, "production", []string{"apps/v1/Deployment"}, "P0"),
		Entry("production data-quality CrashLoop", "crashloop-config-fix-aa-data-quality", "RestartDeployment", []string{"warning"}, "production", []string{"*"}, "P2"),
	)
})
