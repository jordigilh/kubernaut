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

	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
)

// ========================================
// OCI SCHEMA EXTRACTOR TESTS (DD-WORKFLOW-017)
// ========================================
// Authority: DD-WORKFLOW-017 (OCI-based Workflow Registration)
// Authority: BR-WORKFLOW-004 (Workflow Schema Format Specification)
// Business Requirement: BR-WORKFLOW-001 (Workflow Registry Management)
// ========================================
//
// Tests cover:
// - C2: ActionType at top-level + ExtractLabels camelCase keys
// - C3: OCI schema extraction (pull image, extract /workflow-schema.yaml)
//
// BR-WORKFLOW-004 format: plain config file, camelCase fields, no apiVersion/kind,
// structured description, no riskTolerance
//
// ========================================

// validWorkflowSchemaYAML is a minimal valid workflow-schema.yaml per BR-WORKFLOW-004
const validWorkflowSchemaYAML = `metadata:
  workflowId: oomkill-scale-down
  version: "1.0.0"
  description:
    what: Restarts a pod that was OOMKilled to restore service
    whenToUse: OOMKilled events where the pod is managed by a controller
    whenNotToUse: When the crash is caused by a persistent code bug
    preconditions: Pod is managed by a Deployment or StatefulSet
actionType: RestartPod
labels:
  signalType: OOMKilled
  severity: critical
  environment: [production]
  component: pod
  priority: p1
execution:
  engine: tekton
  bundle: quay.io/kubernaut/oomkill-workflow:v1.0.0
parameters:
  - name: NAMESPACE
    type: string
    required: true
    description: Target namespace
  - name: POD_NAME
    type: string
    required: true
    description: Target pod name
`

var _ = Describe("OCI Schema Extractor (DD-WORKFLOW-017)", func() {

	Context("C2: ActionType and Labels (BR-WORKFLOW-004)", func() {
		var parser *schema.Parser

		BeforeEach(func() {
			parser = schema.NewParser()
		})

		It("UT-DS-017-001: should parse actionType from top-level field", func() {
			// BR-WORKFLOW-004: actionType is a required top-level field
			parsedSchema, err := parser.ParseAndValidate(validWorkflowSchemaYAML)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.ActionType).To(Equal("RestartPod"),
				"actionType should be parsed from top-level field")
		})

		It("UT-DS-017-002: should extract labels with camelCase keys", func() {
			// BR-WORKFLOW-004: JSONB labels use camelCase keys
			// DD-WORKFLOW-016: environment is []string (JSONB array)
			parsedSchema, err := parser.ParseAndValidate(validWorkflowSchemaYAML)
			Expect(err).ToNot(HaveOccurred())

			labelsJSON, err := parser.ExtractLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			var labels map[string]interface{}
			Expect(json.Unmarshal(labelsJSON, &labels)).To(Succeed())
			Expect(labels).To(HaveKeyWithValue("signalType", "OOMKilled"))
			Expect(labels).To(HaveKeyWithValue("severity", "critical"))
			Expect(labels["environment"]).To(Equal([]interface{}{"production"}))
			Expect(labels).To(HaveKeyWithValue("component", "pod"))
			Expect(labels).To(HaveKeyWithValue("priority", "P1"))
		})

		It("UT-DS-017-010: should normalize lowercase priority to uppercase (OpenAPI enum compliance)", func() {
			// BR-WORKFLOW-004 + OpenAPI spec: MandatoryLabels.priority enum is [P0, P1, P2, P3, "*"]
			// OCI images may contain lowercase priority in workflow-schema.yaml
			// ExtractLabels MUST normalize to uppercase to pass ogen response validation
			parsedSchema, err := parser.ParseAndValidate(validWorkflowSchemaYAML)
			Expect(err).ToNot(HaveOccurred())
			// validWorkflowSchemaYAML contains "priority: p1" (lowercase)
			Expect(parsedSchema.Labels.Priority).To(Equal("p1"), "raw parsed value should be lowercase")

			labelsJSON, err := parser.ExtractLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			var labels map[string]interface{}
			Expect(json.Unmarshal(labelsJSON, &labels)).To(Succeed())
			Expect(labels).To(HaveKeyWithValue("priority", "P1"),
				"ExtractLabels must normalize priority to uppercase per OpenAPI enum [P0, P1, P2, P3]")
		})

		It("UT-DS-017-002b: should parse environment as []string (array format)", func() {
			// DD-WORKFLOW-016: Environment is strictly []string
			parsedSchema, err := parser.ParseAndValidate(validWorkflowSchemaYAML)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Labels.Environment).To(Equal([]string{"production"}))

			// Multi-value array format
			arrayYAML := `metadata:
  workflowId: multi-env-test
  version: "1.0.0"
  description:
    what: Test
    whenToUse: Test
actionType: RestartPod
labels:
  signalType: OOMKilled
  severity: critical
  environment: [staging, production]
  component: pod
  priority: p1
parameters:
  - name: PARAM
    type: string
    required: true
    description: A param
`
			parsedArray, err := parser.ParseAndValidate(arrayYAML)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedArray.Labels.Environment).To(Equal([]string{"staging", "production"}))
		})

		It("UT-DS-017-008: should parse structured description", func() {
			// BR-WORKFLOW-004: description is structured (what, whenToUse, whenNotToUse, preconditions)
			parsedSchema, err := parser.ParseAndValidate(validWorkflowSchemaYAML)
			Expect(err).ToNot(HaveOccurred())

			desc := parsedSchema.Metadata.Description
			Expect(desc.What).To(ContainSubstring("Restarts a pod"))
			Expect(desc.WhenToUse).To(ContainSubstring("OOMKilled events"))
			Expect(desc.WhenNotToUse).To(ContainSubstring("persistent code bug"))
			Expect(desc.Preconditions).To(ContainSubstring("Deployment or StatefulSet"))
		})

		It("UT-DS-017-008: should accept schema without signalType (DD-WORKFLOW-016)", func() {
			// DD-WORKFLOW-016: signalType is optional metadata, not required for registration
			noSignalTypeYAML := `metadata:
  workflowId: no-signal-type
  version: "1.0.0"
  description:
    what: Workflow without signalType
    whenToUse: When signalType is optional
actionType: RestartPod
labels:
  severity: critical
  environment: [production]
  component: pod
  priority: p1
parameters:
  - name: PARAM
    type: string
    required: true
    description: A param
execution:
  engine: tekton
  bundle: quay.io/test/no-signal:v1.0.0
`
			parsedSchema, err := parser.ParseAndValidate(noSignalTypeYAML)
			Expect(err).ToNot(HaveOccurred(), "schema without signalType should be accepted")
			Expect(parsedSchema.Labels.SignalType).To(BeEmpty())

			// Labels JSONB should NOT contain signalType key when empty
			labelsJSON, err := parser.ExtractLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())
			var labels map[string]interface{}
			Expect(json.Unmarshal(labelsJSON, &labels)).To(Succeed())
			Expect(labels).ToNot(HaveKey("signalType"),
				"signalType should be omitted from labels JSONB when empty")
			Expect(labels).To(HaveKeyWithValue("severity", "critical"))
		})

		It("UT-DS-017-009: should extract structured description as JSON", func() {
			// BR-WORKFLOW-004: description stored as JSONB in DB
			parsedSchema, err := parser.ParseAndValidate(validWorkflowSchemaYAML)
			Expect(err).ToNot(HaveOccurred())

			descJSON, err := parser.ExtractDescription(parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			var desc map[string]string
			Expect(json.Unmarshal(descJSON, &desc)).To(Succeed())
			Expect(desc).To(HaveKey("what"))
			Expect(desc).To(HaveKey("whenToUse"))
		})
	})

	Context("C3: OCI Schema Extraction — Happy Path", func() {
		It("UT-DS-017-003: should extract workflow-schema.yaml from OCI image", func() {
			// DD-WORKFLOW-017: Pull image, find /workflow-schema.yaml, parse it
			mockPuller := oci.NewMockImagePuller(validWorkflowSchemaYAML)

			extractor := oci.NewSchemaExtractor(mockPuller, schema.NewParser())

			result, err := extractor.ExtractFromImage(context.Background(), "quay.io/test/workflow:v1.0.0")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Schema).ToNot(BeNil())
			Expect(result.Schema.Metadata.WorkflowID).To(Equal("oomkill-scale-down"))
			Expect(result.Schema.ActionType).To(Equal("RestartPod"))
			Expect(result.Digest).ToNot(BeEmpty())
			Expect(result.RawContent).To(ContainSubstring("actionType"))
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
				oci.NewMockImagePuller(""), // empty = no schema file
				"quay.io/test/no-schema:v1.0.0",
				"workflow-schema.yaml",
			),

			Entry("UT-DS-017-006: schema YAML is malformed",
				oci.NewMockImagePuller(`this is: [not valid: yaml`),
				"quay.io/test/bad-yaml:v1.0.0",
				"YAML",
			),

			Entry("UT-DS-017-007: schema missing required fields (actionType, labels)",
				oci.NewMockImagePuller(`metadata:
  workflowId: incomplete
  version: "1.0.0"
  description:
    what: Incomplete workflow
    whenToUse: Testing validation
labels:
  signalType: OOMKilled
  severity: critical
  environment: [production]
  component: pod
  priority: p1
parameters:
  - name: PARAM
    type: string
    required: true
    description: A param
`),
				"quay.io/test/incomplete:v1.0.0",
				"actionType",
			),
		)
	})
})
