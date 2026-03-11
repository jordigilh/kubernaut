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

package testutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/test/testutil"
)

func TestTestutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Testutil Suite")
}

var _ = Describe("Workflow Fixture Helpers", func() {

	Describe("NewTestWorkflowCRD + MarshalWorkflowCRD", func() {
		It("produces YAML that round-trips through ParseAndValidate", func() {
			crd := testutil.NewTestWorkflowCRD("round-trip-test", "RestartPod", "tekton")
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parser := schema.NewParser()
			result, err := parser.ParseAndValidate(yamlContent)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.WorkflowName).To(Equal("round-trip-test"))
			Expect(result.ActionType).To(Equal("RestartPod"))
			Expect(result.Version).To(Equal("1.0.0"))
			Expect(result.Execution.Engine).To(Equal("tekton"))
			Expect(result.Labels.Severity).To(Equal([]string{"critical"}))
			Expect(result.Parameters).To(HaveLen(1))
			Expect(result.Parameters[0].Name).To(Equal("NAMESPACE"))
		})

		It("supports field mutation before marshaling", func() {
			crd := testutil.NewTestWorkflowCRD("mutated-test", "ScaleMemory", "job")
			crd.Spec.Labels.Severity = []string{"high", "critical"}
			crd.Spec.Labels.Environment = []string{"staging", "production"}
			crd.Spec.Dependencies = &models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{{Name: "db-creds"}},
			}

			yamlContent := testutil.MarshalWorkflowCRD(crd)
			parser := schema.NewParser()
			result, err := parser.ParseAndValidate(yamlContent)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Labels.Severity).To(Equal([]string{"high", "critical"}))
			Expect(result.Labels.Environment).To(Equal([]string{"staging", "production"}))
			Expect(result.Dependencies.Secrets).To(HaveLen(1))
			Expect(result.Dependencies.Secrets[0].Name).To(Equal("db-creds"))
		})

		It("supports detectedLabels round-trip", func() {
			crd := testutil.NewTestWorkflowCRD("detected-labels-test", "RollbackDeployment", "job")
			crd.Spec.DetectedLabels = &models.DetectedLabelsSchema{
				GitOpsManaged:   "true",
				GitOpsTool:      "argocd",
				PopulatedFields: []string{"gitOpsManaged", "gitOpsTool"},
			}

			yamlContent := testutil.MarshalWorkflowCRD(crd)
			parser := schema.NewParser()
			result, err := parser.ParseAndValidate(yamlContent)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.DetectedLabels).NotTo(BeNil())
			Expect(result.DetectedLabels.GitOpsManaged).To(Equal("true"))
			Expect(result.DetectedLabels.GitOpsTool).To(Equal("argocd"))
			Expect(result.DetectedLabels.PopulatedFields).To(ConsistOf("gitOpsManaged", "gitOpsTool"))
		})
	})

	Describe("LoadWorkflowFixture", func() {
		It("loads an existing fixture file", func() {
			content := testutil.LoadWorkflowFixture("hello-world")
			Expect(content).To(ContainSubstring("apiVersion: kubernaut.ai/v1alpha1"))
			Expect(content).To(ContainSubstring("kind: RemediationWorkflow"))
		})

		It("panics on non-existent fixture", func() {
			Expect(func() {
				testutil.LoadWorkflowFixture("this-does-not-exist")
			}).To(Panic())
		})
	})
})
