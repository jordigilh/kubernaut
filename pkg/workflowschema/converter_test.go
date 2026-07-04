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

package workflowschema_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/workflowschema"
)

// ========================================
// Cluster Label Round-Trip Tests (BR-FLEET-003, Issue #1511)
// ========================================
// Authority: BR-FLEET-003 R7 (Cluster field remains optional at schema level)
// Authority: DD-FLEET-002 (Cluster-Scoped Workflow Targeting)
// ========================================

func baseWorkflowSpec() rwv1alpha1.RemediationWorkflowSpec {
	return rwv1alpha1.RemediationWorkflowSpec{
		Version: "1.0.0",
		Description: rwv1alpha1.RemediationWorkflowDescription{
			What:      "Restarts a pod",
			WhenToUse: "OOMKilled events",
		},
		ActionType: "RestartPod",
		Labels: rwv1alpha1.RemediationWorkflowLabels{
			Severity:    []string{"critical"},
			Environment: []string{"production"},
			Component:   []string{"v1/Pod"},
			Priority:    "P1",
		},
		Parameters: []rwv1alpha1.RemediationWorkflowParameter{
			{Name: "NAMESPACE", Type: "string", Required: true},
		},
	}
}

var _ = Describe("Converter Cluster Label Round-Trip (BR-FLEET-003, #1511)", func() {

	It("UT-DS-1511-003: SpecToSchema preserves Cluster labels", func() {
		spec := baseWorkflowSpec()
		spec.Labels.Cluster = []string{"production", "staging-eu"}

		schema, err := workflowschema.SpecToSchema("test-workflow", spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(schema.Labels.Cluster).To(Equal([]string{"production", "staging-eu"}),
			"Cluster labels must survive CRD -> DS schema conversion")
	})

	It("UT-DS-1511-003b: SpecToSchema tolerates absent Cluster labels (non-fleet)", func() {
		spec := baseWorkflowSpec()

		schema, err := workflowschema.SpecToSchema("test-workflow", spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(schema.Labels.Cluster).To(BeEmpty(),
			"non-fleet workflows never set Cluster; conversion must not fail or fabricate a value")
	})

	It("UT-DS-1511-003c: SchemaToSpec preserves Cluster labels", func() {
		spec := baseWorkflowSpec()
		spec.Labels.Cluster = []string{"production"}
		schema, err := workflowschema.SpecToSchema("test-workflow", spec)
		Expect(err).ToNot(HaveOccurred())

		roundTripped, err := workflowschema.SchemaToSpec(schema)
		Expect(err).ToNot(HaveOccurred())
		Expect(roundTripped.Labels.Cluster).To(Equal([]string{"production"}),
			"Cluster labels must survive DS schema -> CRD conversion")
	})
})

// ========================================
// SchemaToSpec Characterization Tests (Wave E complexity remediation)
// ========================================
// These tests pin down current behavior of SchemaToSpec's optional-field
// branches (DetectedLabels, Execution, Dependencies, Parameters) before
// refactoring to reduce cognitive complexity.
// ========================================

var _ = Describe("SchemaToSpec (characterization)", func() {

	baseSchema := func() *models.WorkflowSchema {
		return &models.WorkflowSchema{
			WorkflowName: "test-workflow",
			Version:      "1.0.0",
			Description: models.WorkflowDescription{
				What:      "Restarts a pod",
				WhenToUse: "OOMKilled events",
			},
			ActionType: "RestartPod",
			Labels: models.WorkflowSchemaLabels{
				Severity:  []string{"critical"},
				Priority:  "P1",
				Component: []string{"v1/Pod"},
			},
		}
	}

	It("returns a minimal spec when all optional sections are absent", func() {
		spec, err := workflowschema.SchemaToSpec(baseSchema())
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.Version).To(Equal("1.0.0"))
		Expect(spec.DetectedLabels).To(BeNil())
		Expect(spec.Execution).To(Equal(rwv1alpha1.RemediationWorkflowExecution{}))
		Expect(spec.Dependencies).To(BeNil())
		Expect(spec.Parameters).To(BeEmpty())
		Expect(spec.RollbackParameters).To(BeEmpty())
	})

	It("marshals DetectedLabels into raw JSON when present", func() {
		schema := baseSchema()
		schema.DetectedLabels = &models.DetectedLabelsSchema{GitOpsManaged: "true"}

		spec, err := workflowschema.SchemaToSpec(schema)
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.DetectedLabels).ToNot(BeNil())
		Expect(string(spec.DetectedLabels.Raw)).To(ContainSubstring(`"gitOpsManaged":"true"`))
	})

	It("converts Execution and marshals EngineConfig when present", func() {
		schema := baseSchema()
		schema.Execution = &models.WorkflowExecution{
			Engine:       "tekton",
			Bundle:       "quay.io/example/bundle:v1",
			BundleDigest: "sha256:abc",
			EngineConfig: map[string]interface{}{"timeoutSeconds": 30},
		}

		spec, err := workflowschema.SchemaToSpec(schema)
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.Execution.Engine).To(Equal("tekton"))
		Expect(spec.Execution.Bundle).To(Equal("quay.io/example/bundle:v1"))
		Expect(spec.Execution.EngineConfig).ToNot(BeNil())
		Expect(string(spec.Execution.EngineConfig.Raw)).To(ContainSubstring("timeoutSeconds"))
	})

	It("propagates a wrapped error when EngineConfig cannot be marshaled", func() {
		schema := baseSchema()
		schema.Execution = &models.WorkflowExecution{
			Engine:       "tekton",
			EngineConfig: func() {},
		}

		_, err := workflowschema.SchemaToSpec(schema)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("marshal engineConfig"))
	})

	It("converts Execution without EngineConfig when EngineConfig is nil", func() {
		schema := baseSchema()
		schema.Execution = &models.WorkflowExecution{Engine: "job"}

		spec, err := workflowschema.SchemaToSpec(schema)
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.Execution.Engine).To(Equal("job"))
		Expect(spec.Execution.EngineConfig).To(BeNil())
	})

	It("converts Dependencies (Secrets and ConfigMaps) when present", func() {
		schema := baseSchema()
		schema.Dependencies = &models.WorkflowDependencies{
			Secrets:    []models.ResourceDependency{{Name: "db-creds"}},
			ConfigMaps: []models.ResourceDependency{{Name: "app-config"}},
		}

		spec, err := workflowschema.SchemaToSpec(schema)
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.Dependencies).ToNot(BeNil())
		Expect(spec.Dependencies.Secrets).To(HaveLen(1))
		Expect(spec.Dependencies.Secrets[0].Name).To(Equal("db-creds"))
		Expect(spec.Dependencies.ConfigMaps).To(HaveLen(1))
		Expect(spec.Dependencies.ConfigMaps[0].Name).To(Equal("app-config"))
	})

	It("converts Parameters and RollbackParameters including default values", func() {
		schema := baseSchema()
		schema.Parameters = []models.WorkflowParameter{
			{Name: "NAMESPACE", Type: "string", Required: true, Description: "target namespace", Default: "default"},
		}
		schema.RollbackParameters = []models.WorkflowParameter{
			{Name: "REVISION", Type: "integer", Required: false, Description: "rollback revision", Default: 1},
		}

		spec, err := workflowschema.SchemaToSpec(schema)
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.Parameters).To(HaveLen(1))
		Expect(spec.Parameters[0].Name).To(Equal("NAMESPACE"))
		Expect(spec.Parameters[0].Default).ToNot(BeNil())
		Expect(string(spec.Parameters[0].Default.Raw)).To(Equal(`"default"`))

		Expect(spec.RollbackParameters).To(HaveLen(1))
		Expect(spec.RollbackParameters[0].Name).To(Equal("REVISION"))
		Expect(spec.RollbackParameters[0].Default).ToNot(BeNil())
		Expect(string(spec.RollbackParameters[0].Default.Raw)).To(Equal("1"))
	})

	It("propagates a wrapped error when a parameter default cannot be marshaled", func() {
		schema := baseSchema()
		schema.Parameters = []models.WorkflowParameter{
			{Name: "BAD", Type: "string", Description: "unmarshalable default", Default: func() {}},
		}

		_, err := workflowschema.SchemaToSpec(schema)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`convert parameter "BAD"`))
	})

	It("propagates a wrapped error when a rollback parameter default cannot be marshaled", func() {
		schema := baseSchema()
		schema.RollbackParameters = []models.WorkflowParameter{
			{Name: "BAD_RB", Type: "string", Description: "unmarshalable default", Default: func() {}},
		}

		_, err := workflowschema.SchemaToSpec(schema)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`convert rollback parameter "BAD_RB"`))
	})
})
