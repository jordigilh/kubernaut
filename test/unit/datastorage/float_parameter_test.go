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
	"github.com/jordigilh/kubernaut/test/testutil"
)

// ========================================
// FLOAT PARAMETER TYPE TESTS (BR-WORKFLOW-005)
// ========================================
// Authority: BR-WORKFLOW-005 (Float Parameter Type)
// Test Plan: docs/testing/45/TEST_PLAN.md
// ========================================

// floatParamBaseCRD returns a CRD tailored for float parameter tests.
func floatParamBaseCRD() *models.WorkflowSchemaCRD {
	crd := testutil.NewTestWorkflowCRD("float-param-test", "ScaleMemory", "tekton")
	crd.Spec.Labels.Severity = []string{"medium"}
	return crd
}

var _ = Describe("Float Parameter Type [BR-WORKFLOW-005]", func() {
	var parser *schema.Parser

	BeforeEach(func() {
		parser = schema.NewParser()
	})

	Context("schema validation", func() {
		It("UT-WF-005-001: should accept float parameter type in schema", func() {
			crd := floatParamBaseCRD()
			min, max := 0.0, 1.0
			crd.Spec.Parameters = []models.WorkflowParameter{
				{Name: "THRESHOLD", Type: "float", Description: "Memory threshold percentage", Required: true, Minimum: &min, Maximum: &max},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Parameters[0].Type).To(Equal("float"))
		})

		It("UT-WF-005-002: should parse float minimum/maximum bounds", func() {
			crd := floatParamBaseCRD()
			min, max := 0.25, 8.0
			crd.Spec.Parameters = []models.WorkflowParameter{
				{Name: "CPU_LIMIT", Type: "float", Description: "CPU limit in cores", Required: true, Minimum: &min, Maximum: &max},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(*parsedSchema.Parameters[0].Minimum).To(BeNumerically("~", 0.25, 0.001))
			Expect(*parsedSchema.Parameters[0].Maximum).To(BeNumerically("~", 8.0, 0.001))
		})

		It("UT-WF-005-004: should maintain integer min/max backward compatibility", func() {
			crd := floatParamBaseCRD()
			min, max := 1.0, 10.0
			crd.Spec.Parameters = []models.WorkflowParameter{
				{Name: "REPLICAS", Type: "integer", Description: "Number of replicas", Required: true, Minimum: &min, Maximum: &max},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
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
