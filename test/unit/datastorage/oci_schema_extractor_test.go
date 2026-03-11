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

package datastorage

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// ========================================
// OCI SCHEMA EXTRACTOR TESTS (DD-WORKFLOW-017 + ADR-043)
// ========================================
// Authority: DD-WORKFLOW-017 (OCI-based Workflow Registration)
// Authority: BR-WORKFLOW-004 (Workflow Schema Format Specification)
// Authority: ADR-043 v1.3 (detectedLabels schema field)
// Authority: DD-WORKFLOW-001 v2.0 (DetectedLabels architecture)
// Business Requirement: BR-WORKFLOW-001 (Workflow Registry Management)
// Test Plan: docs/testing/ADR-043/TEST_PLAN.md
// ========================================
//
// Tests cover:
// - C2: ActionType at top-level + ExtractLabels camelCase keys
// - C3: OCI schema extraction (pull image, extract /workflow-schema.yaml)
// - C4: detectedLabels parsing, validation, extraction (ADR-043 v1.3)
//
// ========================================

// baseOCITestCRD returns the baseline CRD used by OCI extractor tests.
// Uses lowercase priority "p1" to verify normalization in UT-DS-017-010.
func baseOCITestCRD() *models.WorkflowSchemaCRD {
	crd := testutil.NewTestWorkflowCRD("oomkill-scale-down", "RestartPod", "tekton")
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:          "Restarts a pod that was OOMKilled to restore service",
		WhenToUse:     "OOMKilled events where the pod is managed by a controller",
		WhenNotToUse:  "When the crash is caused by a persistent code bug",
		Preconditions: "Pod is managed by a Deployment or StatefulSet",
	}
	crd.Spec.Labels.Priority = "p1"
	crd.Spec.Execution.Bundle = "quay.io/kubernaut/oomkill-workflow:v1.0.0@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
		{Name: "POD_NAME", Type: "string", Required: true, Description: "Target pod name"},
	}
	return crd
}

var _ = Describe("OCI Schema Extractor (DD-WORKFLOW-017)", func() {

	Context("C2: ActionType and Labels (BR-WORKFLOW-004)", func() {
		var parser *schema.Parser

		BeforeEach(func() {
			parser = schema.NewParser()
		})

		It("UT-DS-017-001: should parse actionType from top-level field", func() {
			yamlContent := testutil.MarshalWorkflowCRD(baseOCITestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.ActionType).To(Equal("RestartPod"),
				"actionType should be parsed from top-level field")
		})

		It("UT-DS-017-002: should extract labels with camelCase keys", func() {
			yamlContent := testutil.MarshalWorkflowCRD(baseOCITestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			labelsJSON, err := parser.ExtractLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			var labels map[string]interface{}
			Expect(json.Unmarshal(labelsJSON, &labels)).To(Succeed())
			Expect(labels["severity"]).To(Equal([]interface{}{"critical"}))
			Expect(labels["environment"]).To(Equal([]interface{}{"production"}))
			Expect(labels).To(HaveKeyWithValue("component", "pod"))
			Expect(labels).To(HaveKeyWithValue("priority", "P1"))
		})

		It("UT-DS-017-010: should normalize lowercase priority to uppercase (OpenAPI enum compliance)", func() {
			yamlContent := testutil.MarshalWorkflowCRD(baseOCITestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Labels.Priority).To(Equal("p1"), "raw parsed value should be lowercase")

			labelsJSON, err := parser.ExtractLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			var labels map[string]interface{}
			Expect(json.Unmarshal(labelsJSON, &labels)).To(Succeed())
			Expect(labels).To(HaveKeyWithValue("priority", "P1"),
				"ExtractLabels must normalize priority to uppercase per OpenAPI enum [P0, P1, P2, P3]")
		})

		It("UT-DS-017-002b: should parse environment as []string (array format)", func() {
			yamlContent := testutil.MarshalWorkflowCRD(baseOCITestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Labels.Environment).To(Equal([]string{"production"}))

			crd := testutil.NewTestWorkflowCRD("multi-env-test", "RestartPod", "tekton")
			crd.Spec.Labels.Environment = []string{"staging", "production"}
			multiEnvYAML := testutil.MarshalWorkflowCRD(crd)

			parsedArray, err := parser.ParseAndValidate(multiEnvYAML)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedArray.Labels.Environment).To(Equal([]string{"staging", "production"}))
		})

		It("UT-DS-017-008: should parse structured description", func() {
			yamlContent := testutil.MarshalWorkflowCRD(baseOCITestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			desc := parsedSchema.Description
			Expect(desc.What).To(ContainSubstring("Restarts a pod"))
			Expect(desc.WhenToUse).To(ContainSubstring("OOMKilled events"))
			Expect(desc.WhenNotToUse).To(ContainSubstring("persistent code bug"))
			Expect(desc.Preconditions).To(ContainSubstring("Deployment or StatefulSet"))
		})

		It("UT-DS-017-019: should accept schema without signalName (DD-WORKFLOW-016)", func() {
			crd := testutil.NewTestWorkflowCRD("no-signal-name", "RestartPod", "tekton")
			crd.Spec.Description = sharedtypes.StructuredDescription{
				What:      "Workflow without signalName",
				WhenToUse: "When signalName is optional",
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred(), "schema without signalName should be accepted")

			labelsJSON, err := parser.ExtractLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())
			var labels map[string]interface{}
			Expect(json.Unmarshal(labelsJSON, &labels)).To(Succeed())
			Expect(labels).ToNot(HaveKey("signalName"),
				"signalName should be omitted from labels JSONB when empty")
			Expect(labels["severity"]).To(Equal([]interface{}{"critical"}))
		})

		It("UT-DS-017-009: should extract structured description as JSON", func() {
			yamlContent := testutil.MarshalWorkflowCRD(baseOCITestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			descJSON, err := parser.ExtractDescription(parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			var desc map[string]string
			Expect(json.Unmarshal(descJSON, &desc)).To(Succeed())
			Expect(desc).To(HaveKey("what"))
			Expect(desc).To(HaveKey("whenToUse"))
		})

		It("UT-DS-212-001: ExtractLabels must NOT include custom labels (#212)", func() {
			crd := baseOCITestCRD()
			crd.Metadata.Name = "test-custom"
			crd.Spec.Labels.Priority = "P1"
			crd.Spec.CustomLabels = map[string]string{
				"constraint": "cost-constrained",
				"team":       "payments",
			}
			crd.Spec.Execution.Engine = "job"
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			labelsJSON, err := parser.ExtractLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			var labels map[string]interface{}
			Expect(json.Unmarshal(labelsJSON, &labels)).To(Succeed())
			Expect(labels).ToNot(HaveKey("constraint"),
				"custom labels must NOT be merged into mandatory labels map")
			Expect(labels).ToNot(HaveKey("team"),
				"custom labels must NOT be merged into mandatory labels map")
		})

		It("UT-DS-212-002: ExtractCustomLabels must return custom labels as map[string][]string (#212)", func() {
			crd := baseOCITestCRD()
			crd.Metadata.Name = "test-custom"
			crd.Spec.Labels.Priority = "P1"
			crd.Spec.CustomLabels = map[string]string{
				"constraint": "cost-constrained",
				"team":       "payments",
			}
			crd.Spec.Execution.Engine = "job"
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			customLabels := parser.ExtractCustomLabels(parsedSchema)
			Expect(customLabels).To(HaveKeyWithValue("constraint", []string{"cost-constrained"}))
			Expect(customLabels).To(HaveKeyWithValue("team", []string{"payments"}))
		})
	})

	Context("C3: OCI Schema Extraction — Happy Path", func() {
		It("UT-DS-017-003: should extract workflow-schema.yaml from OCI image", func() {
			yamlContent := testutil.MarshalWorkflowCRD(baseOCITestCRD())
			mockPuller := oci.NewMockImagePuller(yamlContent)

			extractor := oci.NewSchemaExtractor(mockPuller, schema.NewParser())

			result, err := extractor.ExtractFromImage(context.Background(), "quay.io/test/workflow:v1.0.0")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Schema.WorkflowName).To(Equal("oomkill-scale-down"))
			Expect(result.Schema.ActionType).To(Equal("RestartPod"))
			Expect(result.Digest).To(HavePrefix("sha256:"),
				"digest must be a valid sha256 content-addressable identifier")
			Expect(result.RawContent).To(ContainSubstring("actionType"))
		})
	})

	// ========================================
	// C4: detectedLabels (ADR-043 v1.3)
	// ========================================
	Context("C4: detectedLabels Parsing and Validation (ADR-043 v1.3)", func() {
		var parser *schema.Parser

		BeforeEach(func() {
			parser = schema.NewParser()
		})

		It("UT-DS-043-001: workflow requiring HPA-enabled targets is correctly represented after parsing", func() {
			crd := baseOCITestCRD()
			crd.Spec.DetectedLabels = &models.DetectedLabelsSchema{
				HPAEnabled:      "true",
				PDBProtected:    "true",
				PopulatedFields: []string{"hpaEnabled", "pdbProtected"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.DetectedLabels.HPAEnabled).To(Equal("true"))
			Expect(parsedSchema.DetectedLabels.PDBProtected).To(Equal("true"))
		})

		It("UT-DS-043-002: workflow requiring any GitOps tool is correctly represented after parsing", func() {
			crd := baseOCITestCRD()
			crd.Spec.DetectedLabels = &models.DetectedLabelsSchema{
				GitOpsTool:      "*",
				PopulatedFields: []string{"gitOpsTool"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.DetectedLabels.GitOpsTool).To(Equal("*"))
		})

		It("UT-DS-043-003: workflow with no infrastructure requirements has nil detectedLabels", func() {
			yamlContent := testutil.MarshalWorkflowCRD(baseOCITestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.DetectedLabels).To(BeNil(),
				"absence of detectedLabels in YAML means no infrastructure constraints")
		})

		It("UT-DS-043-004: workflow author gets actionable error for invalid boolean value", func() {
			crd := baseOCITestCRD()
			crd.Spec.DetectedLabels = &models.DetectedLabelsSchema{
				HPAEnabled:      "false",
				PopulatedFields: []string{"hpaEnabled"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("hpaEnabled"))
			Expect(err.Error()).To(ContainSubstring("true"),
				"error should tell the author what values are accepted")
		})

		It("UT-DS-043-005: workflow author gets actionable error for invalid gitOpsTool", func() {
			crd := baseOCITestCRD()
			crd.Spec.DetectedLabels = &models.DetectedLabelsSchema{
				GitOpsTool:      "terraform",
				PopulatedFields: []string{"gitOpsTool"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("gitOpsTool"))
			Expect(err.Error()).To(ContainSubstring("argocd"))
		})

		It("UT-DS-043-006: workflow author gets actionable error for invalid serviceMesh", func() {
			crd := baseOCITestCRD()
			crd.Spec.DetectedLabels = &models.DetectedLabelsSchema{
				ServiceMesh:     "consul",
				PopulatedFields: []string{"serviceMesh"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("serviceMesh"))
			Expect(err.Error()).To(ContainSubstring("istio"))
		})

		It("UT-DS-043-007: all 8 detectedLabels fields survive YAML-to-model conversion with exact values", func() {
			crd := baseOCITestCRD()
			crd.Spec.DetectedLabels = &models.DetectedLabelsSchema{
				GitOpsManaged:   "true",
				GitOpsTool:      "argocd",
				PDBProtected:    "true",
				HPAEnabled:      "true",
				Stateful:        "true",
				HelmManaged:     "true",
				NetworkIsolated: "true",
				ServiceMesh:     "istio",
				PopulatedFields: []string{
					"gitOpsManaged", "gitOpsTool", "pdbProtected", "hpaEnabled",
					"stateful", "helmManaged", "networkIsolated", "serviceMesh",
				},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			Expect(parsedSchema.DetectedLabels.PopulatedFields).To(ConsistOf(
				"gitOpsManaged", "gitOpsTool", "pdbProtected", "hpaEnabled",
				"stateful", "helmManaged", "networkIsolated", "serviceMesh",
			), "all 8 fields should be tracked as populated")

			extracted, err := parser.ExtractDetectedLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			Expect(extracted.GitOpsManaged).To(BeTrue())
			Expect(extracted.GitOpsTool).To(Equal("argocd"))
			Expect(extracted.PDBProtected).To(BeTrue())
			Expect(extracted.HPAEnabled).To(BeTrue())
			Expect(extracted.Stateful).To(BeTrue())
			Expect(extracted.HelmManaged).To(BeTrue())
			Expect(extracted.NetworkIsolated).To(BeTrue())
			Expect(extracted.ServiceMesh).To(Equal("istio"))
			Expect(extracted.FailedDetections).To(BeEmpty())
		})

		It("UT-DS-043-008: multi-field combination mirrors real demo scenario schemas", func() {
			crd := baseOCITestCRD()
			crd.Spec.DetectedLabels = &models.DetectedLabelsSchema{
				PDBProtected:    "true",
				HPAEnabled:      "true",
				GitOpsTool:      "*",
				PopulatedFields: []string{"pdbProtected", "hpaEnabled", "gitOpsTool"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			Expect(parsedSchema.DetectedLabels.PopulatedFields).To(ConsistOf(
				"pdbProtected", "hpaEnabled", "gitOpsTool",
			), "only declared fields should be tracked")

			extracted, err := parser.ExtractDetectedLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			Expect(extracted.PDBProtected).To(BeTrue())
			Expect(extracted.HPAEnabled).To(BeTrue())
			Expect(extracted.GitOpsTool).To(Equal("*"))
			Expect(extracted.GitOpsManaged).To(BeFalse(),
				"unset boolean fields should be false (no requirement)")
		})

		It("UT-DS-043-009: OCI extraction pipeline preserves detectedLabels end-to-end", func() {
			crd := baseOCITestCRD()
			crd.Spec.DetectedLabels = &models.DetectedLabelsSchema{
				HPAEnabled:      "true",
				GitOpsTool:      "argocd",
				PopulatedFields: []string{"hpaEnabled", "gitOpsTool"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			mockPuller := oci.NewMockImagePuller(yamlContent)
			extractor := oci.NewSchemaExtractor(mockPuller, schema.NewParser())

			result, err := extractor.ExtractFromImage(context.Background(), "quay.io/test/detected:v1.0.0")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Schema.DetectedLabels.HPAEnabled).To(Equal("true"))
			Expect(result.Schema.DetectedLabels.GitOpsTool).To(Equal("argocd"))
		})

		It("UT-DS-043-010: unknown field in detectedLabels is rejected with error naming the field", func() {
			// Tier 3: Raw YAML — tests parser error for unknown YAML key that cannot
			// be represented by the DetectedLabelsSchema struct.
			yamlContent := `apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: unknown-field-test
spec:
  version: "1.0.0"
  description:
    what: Test
    whenToUse: Test
  actionType: RestartPod
  labels:
    severity: [critical]
    environment: [production]
    component: pod
    priority: P1
  parameters:
    - name: PARAM
      type: string
      required: true
      description: A param
  detectedLabels:
    hpaEnabled: "true"
    customField: "true"
`
			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("customField"),
				"error should name the invalid field so the author can fix their typo")
		})

		It("UT-DS-043-011: empty detectedLabels section produces empty struct, not nil", func() {
			// Tier 3: Raw YAML — tests explicit empty `detectedLabels: {}` which
			// cannot be expressed as a non-nil *DetectedLabelsSchema with zero
			// PopulatedFields (MarshalYAML would omit it entirely).
			yamlContent := `apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: empty-detected-labels-test
spec:
  version: "1.0.0"
  description:
    what: Test
    whenToUse: Test
  actionType: RestartPod
  labels:
    severity: [critical]
    environment: [production]
    component: pod
    priority: P1
  execution:
    engine: tekton
    bundle: quay.io/test/wf:v1@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
  parameters:
    - name: PARAM
      type: string
      required: true
      description: A param
  detectedLabels: {}
`
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.DetectedLabels).ToNot(BeNil(),
				"empty detectedLabels section should produce empty struct (distinct from absent)")

			extracted, err := parser.ExtractDetectedLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())
			Expect(extracted.IsEmpty()).To(BeTrue())
		})
	})

	// Table-driven error cases: extractor returns descriptive errors for failure modes
	Context("C3: OCI Schema Extraction — Error Cases", func() {
		DescribeTable("should return descriptive error",
			func(puller oci.ImagePuller, imageRef string, expectedErrSubstring string) {
				extractor := oci.NewSchemaExtractor(puller, schema.NewParser())

				_, err := extractor.ExtractFromImage(context.Background(), imageRef)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedErrSubstring))
			},

			Entry("UT-DS-017-004: image pull fails",
				oci.NewFailingMockImagePuller("connection refused"),
				"quay.io/test/unreachable:v1.0.0",
				"pull",
			),

			Entry("UT-DS-017-005: /workflow-schema.yaml missing from image",
				oci.NewMockImagePuller(""),
				"quay.io/test/no-schema:v1.0.0",
				"workflow-schema.yaml",
			),

			// Tier 3: Raw YAML — deliberately malformed syntax for parser error path
			Entry("UT-DS-017-006: schema YAML is malformed",
				oci.NewMockImagePuller(`this is: [not valid: yaml`),
				"quay.io/test/bad-yaml:v1.0.0",
				"YAML",
			),

			Entry("UT-DS-017-007: schema missing required fields (actionType)",
				func() oci.ImagePuller {
					crd := testutil.NewTestWorkflowCRD("incomplete", "RestartPod", "tekton")
					crd.Spec.ActionType = ""
					crd.Spec.Execution = nil
					return oci.NewMockImagePuller(testutil.MarshalWorkflowCRD(crd))
				}(),
				"quay.io/test/incomplete:v1.0.0",
				"actionType",
			),
		)
	})
})
