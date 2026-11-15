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

package datastorage_test

import (
	"maps"

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
	crd.Spec.Labels.Severity = []string{"warning"}
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
			minVal, maxVal := 0.0, 1.0
			crd.Spec.Parameters = []models.WorkflowParameter{
				{Name: "THRESHOLD", Type: "float", Description: "Memory threshold percentage", Required: true, Minimum: &minVal, Maximum: &maxVal},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Parameters[0].Type).To(Equal("float"))
		})

		It("UT-WF-005-002: should parse float minimum/maximum bounds", func() {
			crd := floatParamBaseCRD()
			minVal, maxVal := 0.25, 8.0
			crd.Spec.Parameters = []models.WorkflowParameter{
				{Name: "CPU_LIMIT", Type: "float", Description: "CPU limit in cores", Required: true, Minimum: &minVal, Maximum: &maxVal},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(*parsedSchema.Parameters[0].Minimum).To(BeNumerically("~", 0.25, 0.001))
			Expect(*parsedSchema.Parameters[0].Maximum).To(BeNumerically("~", 8.0, 0.001))
		})

		It("UT-WF-005-004: should maintain integer min/max backward compatibility", func() {
			crd := floatParamBaseCRD()
			minVal, maxVal := 1.0, 10.0
			crd.Spec.Parameters = []models.WorkflowParameter{
				{Name: "REPLICAS", Type: "integer", Description: "Number of replicas", Required: true, Minimum: &minVal, Maximum: &maxVal},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(*parsedSchema.Parameters[0].Minimum).To(BeNumerically("==", 1))
			Expect(*parsedSchema.Parameters[0].Maximum).To(BeNumerically("==", 10))
		})
	})

	Context("parameter value validation", func() {
		DescribeTable("BR-KA-191 validates complete effective parameter maps without rewriting", func(parameter models.WorkflowParameter, values map[string]string, valid bool) {
			original := maps.Clone(values)
			err := models.ValidateParameterSet([]models.WorkflowParameter{parameter}, values)
			Expect(values).To(Equal(original))
			if valid {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
			}
		},
			Entry("UT-VAL-2570-080: exact string", models.WorkflowParameter{Name: "P", Type: "string", Pattern: "^[a-z ]+$"}, map[string]string{"P": "keep text"}, true),
			Entry("integer exact", models.WorkflowParameter{Name: "P", Type: "integer"}, map[string]string{"P": "9007199254740993"}, true),
			Entry("integer decimal", models.WorkflowParameter{Name: "P", Type: "integer"}, map[string]string{"P": "1.0"}, false),
			Entry("UT-VAL-2570-082: integer over bound", models.WorkflowParameter{Name: "P", Type: "integer", Maximum: ptrFloat64(9007199254740992)}, map[string]string{"P": "9007199254740993"}, false),
			Entry("float exact text", models.WorkflowParameter{Name: "P", Type: "float"}, map[string]string{"P": "1.0"}, true),
			Entry("UT-VAL-2570-090: float non-finite", models.WorkflowParameter{Name: "P", Type: "float"}, map[string]string{"P": "NaN"}, false),
			Entry("UT-VAL-2570-091: float overflow", models.WorkflowParameter{Name: "P", Type: "float"}, map[string]string{"P": "1e400"}, false),
			Entry("UT-VAL-2570-092: float underflow", models.WorkflowParameter{Name: "P", Type: "float"}, map[string]string{"P": "1e-4000"}, false),
			Entry("UT-VAL-2570-093: integer above int64", models.WorkflowParameter{Name: "P", Type: "integer"}, map[string]string{"P": "9223372036854775808"}, false),
			Entry("UT-VAL-2570-093: integer below int64", models.WorkflowParameter{Name: "P", Type: "integer"}, map[string]string{"P": "-9223372036854775809"}, false),
			Entry("UT-VAL-2570-081: array", models.WorkflowParameter{Name: "P", Type: "array"}, map[string]string{"P": `["a"]`}, true),
			Entry("UT-VAL-2570-081: array null", models.WorkflowParameter{Name: "P", Type: "array"}, map[string]string{"P": "null"}, false),
			Entry("missing required", models.WorkflowParameter{Name: "P", Type: "string", Required: true}, map[string]string{}, false),
			Entry("UT-VAL-2570-088: default remains omitted", models.WorkflowParameter{Name: "P", Type: "string", Default: "auto"}, map[string]string{}, true),
			Entry("UT-VAL-2570-085: missing dependency", models.WorkflowParameter{Name: "P", Type: "string", DependsOn: []string{"Q"}}, map[string]string{"P": "x"}, false),
			Entry("malformed pattern absent value", models.WorkflowParameter{Name: "P", Type: "string", Pattern: "["}, map[string]string{}, false),
			Entry("UT-VAL-2570-084: pattern mismatch", models.WorkflowParameter{Name: "P", Type: "string", Pattern: "^[a-z]+$"}, map[string]string{"P": "FAST"}, false),
			Entry("UT-VAL-2570-083: enum mismatch", models.WorkflowParameter{Name: "P", Type: "string", Enum: []string{"safe"}}, map[string]string{"P": "fast"}, false),
			Entry("unknown parameter", models.WorkflowParameter{Name: "P", Type: "string"}, map[string]string{"NEW": "x"}, false),
		)
		It("UT-DS-2578-001: BR-KA-191 rejects malformed typed parameter values", func() {
			integer := models.WorkflowParameter{Name: "REPLICAS", Type: "integer"}
			for _, value := range []string{"1.0", "1e3", "9223372036854775808"} {
				Expect(models.ValidateParameterValue(integer, value)).To(HaveOccurred(), "integer %q", value)
			}
			Expect(models.ValidateParameterValue(integer, "9007199254740993")).To(Succeed())
			integer.Maximum = ptrFloat64(9007199254740992)
			Expect(models.ValidateParameterValue(integer, "9007199254740993")).To(HaveOccurred())

			floating := models.WorkflowParameter{Name: "THRESHOLD", Type: "float"}
			for _, value := range []string{"NaN", "Inf", "1e400", "1e-4000"} {
				Expect(models.ValidateParameterValue(floating, value)).To(HaveOccurred(), "float %q", value)
			}
			Expect(models.ValidateParameterValue(floating, "1.0")).To(Succeed())
		})

		It("UT-DS-2578-002: BR-KA-191 validates boolean and array representations", func() {
			boolean := models.WorkflowParameter{Name: "ENABLED", Type: "boolean"}
			Expect(models.ValidateParameterValue(boolean, "true")).To(Succeed())
			Expect(models.ValidateParameterValue(boolean, "false")).To(Succeed())
			Expect(models.ValidateParameterValue(boolean, "notbool")).To(HaveOccurred())

			array := models.WorkflowParameter{Name: "ITEMS", Type: "array"}
			Expect(models.ValidateParameterValue(array, `["a","b"]`)).To(Succeed())
			Expect(models.ValidateParameterValue(array, "garbage")).To(HaveOccurred())
			Expect(models.ValidateParameterValue(array, "null")).To(HaveOccurred())
			Expect(models.ValidateParameterValue(models.WorkflowParameter{Name: "UNKNOWN", Type: "unknown"}, "value")).To(HaveOccurred())
		})

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
