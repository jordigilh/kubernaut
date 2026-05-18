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

package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

var _ = Describe("BR-HAPI-191: Parameter Validation Constraints (#1170)", func() {

	var (
		v *parser.Validator
	)

	BeforeEach(func() {
		v = parser.NewValidator([]string{"test-workflow"})
	})

	// ========================================
	// GROUP A: Individual Constraint Validation
	// ========================================

	Context("UT-KA-1170-001: Required — missing required param produces error", func() {
		It("should return an error when a required parameter is absent", func() {
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas"},
			}
			params := map[string]interface{}{} // missing REPLICA_COUNT

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeFalse())
			Expect(result.Errors).To(ContainElement(ContainSubstring("REPLICA_COUNT")))
			Expect(result.Errors).To(ContainElement(ContainSubstring("required")))
		})
	})

	Context("UT-KA-1170-002: Required — optional param absent is valid", func() {
		It("should not produce an error when an optional parameter is absent", func() {
			schema := []models.WorkflowParameter{
				{Name: "TIMEOUT", Type: "integer", Required: false, Description: "Timeout in seconds"},
			}
			params := map[string]interface{}{}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
			Expect(result.Errors).To(BeEmpty())
		})
	})

	Context("UT-KA-1170-003: Type/string — string value passes string type check", func() {
		It("should accept a string value for string type parameter", func() {
			schema := []models.WorkflowParameter{
				{Name: "POD_NAME", Type: "string", Required: true, Description: "Pod name"},
			}
			params := map[string]interface{}{"POD_NAME": "my-pod-123"}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
		})
	})

	Context("UT-KA-1170-004: Type/integer — integer value (float64 without fraction) passes", func() {
		It("should accept float64 with no fractional part for integer type", func() {
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas"},
			}
			params := map[string]interface{}{"REPLICA_COUNT": float64(3)}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
		})
	})

	Context("UT-KA-1170-005: Type/integer — non-numeric value fails integer check", func() {
		It("should reject a string value for integer type", func() {
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas"},
			}
			params := map[string]interface{}{"REPLICA_COUNT": "three"}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeFalse())
			Expect(result.Errors).To(ContainElement(ContainSubstring("REPLICA_COUNT")))
			Expect(result.Errors).To(ContainElement(ContainSubstring("integer")))
		})
	})

	Context("UT-KA-1170-006: Type/integer — bool value rejected for integer type", func() {
		It("should reject a boolean value for integer type", func() {
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas"},
			}
			params := map[string]interface{}{"REPLICA_COUNT": true}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeFalse())
			Expect(result.Errors).To(ContainElement(ContainSubstring("REPLICA_COUNT")))
		})
	})

	Context("UT-KA-1170-007: Type/boolean — bool value passes boolean check", func() {
		It("should accept a boolean value for boolean type", func() {
			schema := []models.WorkflowParameter{
				{Name: "DRY_RUN", Type: "boolean", Required: true, Description: "Dry run mode"},
			}
			params := map[string]interface{}{"DRY_RUN": true}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
		})
	})

	Context("UT-KA-1170-008: Type/float — numeric value passes float check", func() {
		It("should accept a float64 value for float type", func() {
			schema := []models.WorkflowParameter{
				{Name: "THRESHOLD", Type: "float", Required: true, Description: "Alert threshold"},
			}
			params := map[string]interface{}{"THRESHOLD": float64(0.85)}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
		})
	})

	Context("UT-KA-1170-009: Type/array — slice value passes array check", func() {
		It("should accept a slice value for array type", func() {
			schema := []models.WorkflowParameter{
				{Name: "NAMESPACES", Type: "array", Required: true, Description: "Namespaces list"},
			}
			params := map[string]interface{}{"NAMESPACES": []interface{}{"ns1", "ns2"}}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
		})
	})

	Context("UT-KA-1170-010: Type/array — non-slice value fails array check", func() {
		It("should reject a string value for array type", func() {
			schema := []models.WorkflowParameter{
				{Name: "NAMESPACES", Type: "array", Required: true, Description: "Namespaces list"},
			}
			params := map[string]interface{}{"NAMESPACES": "ns1,ns2"}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeFalse())
			Expect(result.Errors).To(ContainElement(ContainSubstring("NAMESPACES")))
			Expect(result.Errors).To(ContainElement(ContainSubstring("array")))
		})
	})

	Context("UT-KA-1170-011: Minimum — value below minimum produces error", func() {
		It("should reject value below minimum", func() {
			min := float64(1)
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas", Minimum: &min},
			}
			params := map[string]interface{}{"REPLICA_COUNT": float64(0)}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeFalse())
			Expect(result.Errors).To(ContainElement(ContainSubstring("REPLICA_COUNT")))
			Expect(result.Errors).To(ContainElement(ContainSubstring("minimum")))
		})
	})

	Context("UT-KA-1170-012: Minimum — value at minimum is valid", func() {
		It("should accept value at minimum boundary", func() {
			min := float64(1)
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas", Minimum: &min},
			}
			params := map[string]interface{}{"REPLICA_COUNT": float64(1)}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
		})
	})

	Context("UT-KA-1170-013: Maximum — value above maximum produces error", func() {
		It("should reject value above maximum", func() {
			max := float64(10)
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas", Maximum: &max},
			}
			params := map[string]interface{}{"REPLICA_COUNT": float64(15)}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeFalse())
			Expect(result.Errors).To(ContainElement(ContainSubstring("REPLICA_COUNT")))
			Expect(result.Errors).To(ContainElement(ContainSubstring("maximum")))
		})
	})

	Context("UT-KA-1170-014: Maximum — value at maximum is valid", func() {
		It("should accept value at maximum boundary", func() {
			max := float64(10)
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas", Maximum: &max},
			}
			params := map[string]interface{}{"REPLICA_COUNT": float64(10)}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
		})
	})

	Context("UT-KA-1170-015: Enum — value in enum set is valid", func() {
		It("should accept a value present in the enum list", func() {
			schema := []models.WorkflowParameter{
				{Name: "STRATEGY", Type: "string", Required: true, Description: "Rollout strategy", Enum: []string{"rolling", "recreate", "blue-green"}},
			}
			params := map[string]interface{}{"STRATEGY": "rolling"}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
		})
	})

	Context("UT-KA-1170-016: Enum — value not in enum set produces error", func() {
		It("should reject a value not in the enum list", func() {
			schema := []models.WorkflowParameter{
				{Name: "STRATEGY", Type: "string", Required: true, Description: "Rollout strategy", Enum: []string{"rolling", "recreate", "blue-green"}},
			}
			params := map[string]interface{}{"STRATEGY": "canary"}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeFalse())
			Expect(result.Errors).To(ContainElement(ContainSubstring("STRATEGY")))
			Expect(result.Errors).To(ContainElement(ContainSubstring("enum")))
		})
	})

	Context("UT-KA-1170-017: Pattern — value matching regex is valid", func() {
		It("should accept a value that matches the pattern", func() {
			schema := []models.WorkflowParameter{
				{Name: "POD_NAME", Type: "string", Required: true, Description: "Pod name", Pattern: `^[a-z][a-z0-9-]*$`},
			}
			params := map[string]interface{}{"POD_NAME": "my-pod-123"}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
		})
	})

	Context("UT-KA-1170-018: Pattern — value not matching regex produces error", func() {
		It("should reject a value that does not match the pattern", func() {
			schema := []models.WorkflowParameter{
				{Name: "POD_NAME", Type: "string", Required: true, Description: "Pod name", Pattern: `^[a-z][a-z0-9-]*$`},
			}
			params := map[string]interface{}{"POD_NAME": "My_Pod!"}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeFalse())
			Expect(result.Errors).To(ContainElement(ContainSubstring("POD_NAME")))
			Expect(result.Errors).To(ContainElement(ContainSubstring("pattern")))
		})
	})

	Context("UT-KA-1170-019: Pattern — invalid regex pattern is skipped with warning", func() {
		It("should not error on an invalid regex pattern (graceful degradation)", func() {
			schema := []models.WorkflowParameter{
				{Name: "POD_NAME", Type: "string", Required: true, Description: "Pod name", Pattern: `[invalid`},
			}
			params := map[string]interface{}{"POD_NAME": "anything"}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
			Expect(result.Warnings).To(ContainElement(ContainSubstring("POD_NAME")))
		})
	})

	Context("UT-KA-1170-020: DependsOn — param present when dependency is present is valid", func() {
		It("should accept when both the param and its dependency are present", func() {
			schema := []models.WorkflowParameter{
				{Name: "DEPLOYMENT_NAME", Type: "string", Required: true, Description: "Deployment name"},
				{Name: "CONTAINER_NAME", Type: "string", Required: false, Description: "Container name", DependsOn: []string{"DEPLOYMENT_NAME"}},
			}
			params := map[string]interface{}{
				"DEPLOYMENT_NAME": "my-app",
				"CONTAINER_NAME":  "main",
			}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
		})
	})

	Context("UT-KA-1170-021: DependsOn — param present when dependency is absent produces error", func() {
		It("should reject when a parameter is present but its dependency is absent", func() {
			schema := []models.WorkflowParameter{
				{Name: "DEPLOYMENT_NAME", Type: "string", Required: false, Description: "Deployment name"},
				{Name: "CONTAINER_NAME", Type: "string", Required: false, Description: "Container name", DependsOn: []string{"DEPLOYMENT_NAME"}},
			}
			params := map[string]interface{}{
				"CONTAINER_NAME": "main",
			}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeFalse())
			Expect(result.Errors).To(ContainElement(ContainSubstring("CONTAINER_NAME")))
			Expect(result.Errors).To(ContainElement(ContainSubstring("depends")))
		})
	})

	// ========================================
	// GROUP B: Undeclared Parameter Stripping
	// ========================================

	Context("UT-KA-1170-030: Undeclared params are removed in-place", func() {
		It("should strip parameters not in the schema", func() {
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas"},
			}
			params := map[string]interface{}{
				"REPLICA_COUNT":     float64(3),
				"HALLUCINATED_PARAM": "evil-value",
			}

			result := v.ValidateParameters(params, schema)
			Expect(result.StrippedParams).To(ContainElement("HALLUCINATED_PARAM"))
			Expect(params).NotTo(HaveKey("HALLUCINATED_PARAM"))
		})
	})

	Context("UT-KA-1170-031: Declared params are preserved", func() {
		It("should not strip declared parameters", func() {
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas"},
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Namespace"},
			}
			params := map[string]interface{}{
				"REPLICA_COUNT": float64(3),
				"NAMESPACE":     "default",
			}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
			Expect(params).To(HaveKey("REPLICA_COUNT"))
			Expect(params).To(HaveKey("NAMESPACE"))
			Expect(result.StrippedParams).To(BeEmpty())
		})
	})

	Context("UT-KA-1170-032: KA-managed params (TARGET_RESOURCE_*) are never stripped", func() {
		It("should preserve TARGET_RESOURCE_* params even though they are not in schema", func() {
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas"},
			}
			params := map[string]interface{}{
				"REPLICA_COUNT":                float64(3),
				"TARGET_RESOURCE_NAME":         "my-pod",
				"TARGET_RESOURCE_KIND":         "Pod",
				"TARGET_RESOURCE_NAMESPACE":    "default",
				"TARGET_RESOURCE_API_VERSION":  "v1",
			}

			result := v.ValidateParameters(params, schema)
			Expect(params).To(HaveKey("TARGET_RESOURCE_NAME"))
			Expect(params).To(HaveKey("TARGET_RESOURCE_KIND"))
			Expect(params).To(HaveKey("TARGET_RESOURCE_NAMESPACE"))
			Expect(params).To(HaveKey("TARGET_RESOURCE_API_VERSION"))
			Expect(result.StrippedParams).NotTo(ContainElement("TARGET_RESOURCE_NAME"))
			Expect(result.StrippedParams).NotTo(ContainElement("TARGET_RESOURCE_KIND"))
			Expect(result.StrippedParams).NotTo(ContainElement("TARGET_RESOURCE_NAMESPACE"))
			Expect(result.StrippedParams).NotTo(ContainElement("TARGET_RESOURCE_API_VERSION"))
		})
	})

	Context("UT-KA-1170-033: No schema (nil Parameters) strips ALL LLM params", func() {
		It("should strip all non-KA-managed params when schema is nil", func() {
			params := map[string]interface{}{
				"REPLICA_COUNT":            float64(3),
				"TARGET_RESOURCE_NAME":     "my-pod",
				"TARGET_RESOURCE_KIND":     "Pod",
				"TARGET_RESOURCE_NAMESPACE": "default",
				"TARGET_RESOURCE_API_VERSION": "v1",
			}

			result := v.ValidateParameters(params, nil)
			Expect(params).To(HaveKey("TARGET_RESOURCE_NAME"))
			Expect(params).To(HaveKey("TARGET_RESOURCE_KIND"))
			Expect(params).To(HaveKey("TARGET_RESOURCE_NAMESPACE"))
			Expect(params).To(HaveKey("TARGET_RESOURCE_API_VERSION"))
			Expect(params).NotTo(HaveKey("REPLICA_COUNT"))
			Expect(result.StrippedParams).To(ContainElement("REPLICA_COUNT"))
		})
	})

	Context("UT-KA-1170-034: Empty schema (0 params declared) strips ALL LLM params", func() {
		It("should strip all non-KA-managed params when schema is empty slice", func() {
			schema := []models.WorkflowParameter{}
			params := map[string]interface{}{
				"SOME_PARAM":               "value",
				"TARGET_RESOURCE_NAME":     "my-pod",
			}

			result := v.ValidateParameters(params, schema)
			Expect(params).NotTo(HaveKey("SOME_PARAM"))
			Expect(params).To(HaveKey("TARGET_RESOURCE_NAME"))
			Expect(result.StrippedParams).To(ContainElement("SOME_PARAM"))
		})
	})

	Context("UT-KA-1170-035: KA-managed params skipped during validation (no required check)", func() {
		It("should not validate TARGET_RESOURCE_* params even if they appear in schema", func() {
			schema := []models.WorkflowParameter{
				{Name: "TARGET_RESOURCE_NAME", Type: "string", Required: true, Description: "Target resource name"},
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Replicas"},
			}
			params := map[string]interface{}{
				"REPLICA_COUNT": float64(3),
			}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
			for _, e := range result.Errors {
				Expect(e).NotTo(ContainSubstring("TARGET_RESOURCE"),
					"Should not produce errors for KA-managed params")
			}
		})
	})

	// ========================================
	// GROUP D: Multi-Error ValidationResult
	// ========================================

	Context("UT-KA-1170-050: Multiple constraint violations produce multiple errors", func() {
		It("should collect all validation errors, not stop at first", func() {
			min := float64(1)
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas", Minimum: &min},
				{Name: "STRATEGY", Type: "string", Required: true, Description: "Strategy", Enum: []string{"rolling", "recreate"}},
			}
			params := map[string]interface{}{
				"REPLICA_COUNT": "not-a-number",
				"STRATEGY":      "canary",
			}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeFalse())
			Expect(len(result.Errors)).To(BeNumerically(">=", 2))
		})
	})

	Context("UT-KA-1170-051: ValidationResult.IsValid=true when no errors", func() {
		It("should return valid result when all params pass validation", func() {
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas"},
			}
			params := map[string]interface{}{"REPLICA_COUNT": float64(3)}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeTrue())
			Expect(result.Errors).To(BeEmpty())
		})
	})

	Context("UT-KA-1170-052: ValidationResult.SchemaHint populated on failure", func() {
		It("should include schema hint when validation fails", func() {
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas"},
			}
			params := map[string]interface{}{"REPLICA_COUNT": "bad"}

			result := v.ValidateParameters(params, schema)
			Expect(result.IsValid).To(BeFalse())
			Expect(result.SchemaHint).NotTo(BeEmpty())
			Expect(result.SchemaHint).To(ContainSubstring("REPLICA_COUNT"))
		})
	})
})

var _ = Describe("BR-HAPI-191: Schema Hint Formatting (#1170)", func() {

	// ========================================
	// GROUP C: Schema Hint Formatting
	// ========================================

	Context("UT-KA-1170-040: Schema hint includes param names and types", func() {
		It("should include parameter names and types in the hint", func() {
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas"},
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
			}

			hint := parser.FormatSchemaHint(schema)
			Expect(hint).To(ContainSubstring("REPLICA_COUNT"))
			Expect(hint).To(ContainSubstring("integer"))
			Expect(hint).To(ContainSubstring("NAMESPACE"))
			Expect(hint).To(ContainSubstring("string"))
		})
	})

	Context("UT-KA-1170-041: Required params marked as (required)", func() {
		It("should mark required parameters in the hint", func() {
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas"},
				{Name: "TIMEOUT", Type: "integer", Required: false, Description: "Timeout"},
			}

			hint := parser.FormatSchemaHint(schema)
			Expect(hint).To(MatchRegexp(`REPLICA_COUNT.*required`))
		})
	})

	Context("UT-KA-1170-042: Constraints (min, max, enum) included in hint", func() {
		It("should include min, max, and enum constraints in the hint", func() {
			min := float64(1)
			max := float64(10)
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas", Minimum: &min, Maximum: &max},
				{Name: "STRATEGY", Type: "string", Required: true, Description: "Strategy", Enum: []string{"rolling", "recreate"}},
			}

			hint := parser.FormatSchemaHint(schema)
			Expect(hint).To(ContainSubstring("1"))
			Expect(hint).To(ContainSubstring("10"))
			Expect(hint).To(ContainSubstring("rolling"))
			Expect(hint).To(ContainSubstring("recreate"))
		})
	})

	Context("UT-KA-1170-043: KA-managed params excluded from hint", func() {
		It("should not include TARGET_RESOURCE_* params in the hint", func() {
			schema := []models.WorkflowParameter{
				{Name: "TARGET_RESOURCE_NAME", Type: "string", Required: true, Description: "Target resource name"},
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas"},
			}

			hint := parser.FormatSchemaHint(schema)
			Expect(hint).NotTo(ContainSubstring("TARGET_RESOURCE_NAME"))
			Expect(hint).To(ContainSubstring("REPLICA_COUNT"))
		})
	})

	Context("UT-KA-1170-044: Empty schema returns appropriate message", func() {
		It("should return a message indicating no schema is available", func() {
			hint := parser.FormatSchemaHint(nil)
			Expect(hint).To(ContainSubstring("No parameter schema available"))
		})

		It("should return the same message for an empty slice", func() {
			hint := parser.FormatSchemaHint([]models.WorkflowParameter{})
			Expect(hint).To(ContainSubstring("No parameter schema available"))
		})
	})
})
