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

package datastorage

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
)

// ========================================
// FLOAT PARAMETER TYPE TESTS (BR-WORKFLOW-005)
// ========================================
// Authority: BR-WORKFLOW-005 (Float Parameter Type)
// Test Plan: docs/testing/45/TEST_PLAN.md
// ========================================

const floatParamBaseYAML = `schemaVersion: "1.0"
metadata:
  workflowId: float-param-test
  version: "1.0.0"
  description:
    what: Tests float parameter type
    whenToUse: When validating float parameter support
    whenNotToUse: N/A
    preconditions: None
actionType: ScaleMemory
labels:
  signalType: MemoryPressure
  severity: [medium]
  component: pod
  environment: [production]
  priority: P1
`

const floatParamValidSchemaYAML = floatParamBaseYAML + `parameters:
  - name: THRESHOLD
    type: float
    description: Memory threshold percentage
    required: true
    minimum: 0.0
    maximum: 1.0
execution:
  engine: tekton
  bundle: quay.io/kubernaut/workflows/scale@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
`

const intParamBackwardCompatYAML = floatParamBaseYAML + `parameters:
  - name: REPLICAS
    type: integer
    description: Number of replicas
    required: true
    minimum: 1
    maximum: 10
execution:
  engine: tekton
  bundle: quay.io/kubernaut/workflows/scale@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
`

const floatParamWithDecimalBoundsYAML = floatParamBaseYAML + `parameters:
  - name: CPU_LIMIT
    type: float
    description: CPU limit in cores
    required: true
    minimum: 0.25
    maximum: 8.0
execution:
  engine: tekton
  bundle: quay.io/kubernaut/workflows/scale@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
`

var _ = Describe("Float Parameter Type [BR-WORKFLOW-005]", func() {
	var parser *schema.Parser

	BeforeEach(func() {
		parser = schema.NewParser()
	})

	Context("schema validation", func() {
		It("UT-WF-005-001: should accept float parameter type in schema", func() {
			parsedSchema, err := parser.ParseAndValidate(floatParamValidSchemaYAML)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Parameters[0].Type).To(Equal("float"))
		})

		It("UT-WF-005-002: should parse float minimum/maximum bounds", func() {
			parsedSchema, err := parser.ParseAndValidate(floatParamWithDecimalBoundsYAML)
			Expect(err).ToNot(HaveOccurred())
			Expect(*parsedSchema.Parameters[0].Minimum).To(BeNumerically("~", 0.25, 0.001))
			Expect(*parsedSchema.Parameters[0].Maximum).To(BeNumerically("~", 8.0, 0.001))
		})

		It("UT-WF-005-004: should maintain integer min/max backward compatibility", func() {
			parsedSchema, err := parser.ParseAndValidate(intParamBackwardCompatYAML)
			Expect(err).ToNot(HaveOccurred())
			Expect(*parsedSchema.Parameters[0].Minimum).To(BeNumerically("==", 1))
			Expect(*parsedSchema.Parameters[0].Maximum).To(BeNumerically("==", 10))
		})
	})

	Context("parameter value validation", func() {
		It("UT-WF-005-003: should reject float value outside min/max bounds", func() {
			param := models.WorkflowParameter{
				Name:    "THRESHOLD",
				Type:    "float",
				Minimum: ptrFloat64(0.0),
				Maximum: ptrFloat64(1.0),
			}

			err := models.ValidateParameterValue(param, "1.5")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maximum"))

			err = models.ValidateParameterValue(param, "-0.1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("minimum"))

			err = models.ValidateParameterValue(param, "0.5")
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-WF-005-003b: should reject integer value outside bounds", func() {
			param := models.WorkflowParameter{
				Name:    "REPLICAS",
				Type:    "integer",
				Minimum: ptrFloat64(1),
				Maximum: ptrFloat64(10),
			}

			err := models.ValidateParameterValue(param, "15")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maximum"))

			err = models.ValidateParameterValue(param, "0")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("minimum"))

			err = models.ValidateParameterValue(param, "5")
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-WF-005-003c: should skip bounds check for non-numeric types", func() {
			param := models.WorkflowParameter{
				Name: "NAME",
				Type: "string",
			}

			err := models.ValidateParameterValue(param, "anything")
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-WF-005-003d: should reject non-numeric value for numeric type", func() {
			param := models.WorkflowParameter{
				Name: "REPLICAS",
				Type: "integer",
			}

			err := models.ValidateParameterValue(param, "not-a-number")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid"))
		})
	})
})

func ptrFloat64(f float64) *float64 {
	return &f
}
