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
// Business Requirement: BR-WORKFLOW-001 (Workflow Registry Management)
// ========================================
//
// Tests cover:
// - C2: ActionType in WorkflowSchemaLabels + ExtractLabels
// - C3: OCI schema extraction (pull image, extract /workflow-schema.yaml)
//
// ========================================

// validWorkflowSchemaYAML is a minimal valid workflow-schema.yaml for testing
const validWorkflowSchemaYAML = `apiVersion: kubernaut.io/v1alpha1
kind: WorkflowSchema
metadata:
  workflow_id: oomkill-scale-down
  version: "1.0.0"
  description: OOMKill recovery workflow
labels:
  signal_type: OOMKilled
  severity: critical
  risk_tolerance: low
  action_type: restart_pod
  environment: production
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

	Context("C2: ActionType in WorkflowSchemaLabels", func() {
		var parser *schema.Parser

		BeforeEach(func() {
			parser = schema.NewParser()
		})

		It("UT-DS-017-001: should parse action_type from workflow-schema.yaml", func() {
			// BR-WORKFLOW-001: action_type is a mandatory field for workflow indexing
			parsedSchema, err := parser.ParseAndValidate(validWorkflowSchemaYAML)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Labels.ActionType).To(Equal("restart_pod"),
				"action_type should be parsed from labels section")
		})

		It("UT-DS-017-002: should include action_type in ExtractLabels output", func() {
			// DD-WORKFLOW-016: action_type must be extractable for DB indexing
			parsedSchema, err := parser.ParseAndValidate(validWorkflowSchemaYAML)
			Expect(err).ToNot(HaveOccurred())

			labelsJSON, err := parser.ExtractLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			var labels map[string]string
			Expect(json.Unmarshal(labelsJSON, &labels)).To(Succeed())
			Expect(labels).To(HaveKeyWithValue("action_type", "restart_pod"))
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
			Expect(result.Schema.Labels.ActionType).To(Equal("restart_pod"))
			Expect(result.Digest).ToNot(BeEmpty())
			Expect(result.RawContent).To(ContainSubstring("apiVersion"))
		})
	})

	// ⭐ TABLE-DRIVEN: Error cases share the same assert structure
	// BEHAVIOR: Extractor returns descriptive errors for various failure modes
	// CORRECTNESS: Error messages identify the root cause (pull, missing file, parse, validate)
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

			Entry("UT-DS-017-007: schema missing mandatory labels (severity, risk_tolerance)",
				oci.NewMockImagePuller(`apiVersion: kubernaut.io/v1alpha1
kind: WorkflowSchema
metadata:
  workflow_id: incomplete
  version: "1.0.0"
  description: Missing labels
labels:
  signal_type: OOMKilled
parameters:
  - name: PARAM
    type: string
    required: true
    description: A param
`),
				"quay.io/test/incomplete:v1.0.0",
				"severity",
			),
		)
	})
})
