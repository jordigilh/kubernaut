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