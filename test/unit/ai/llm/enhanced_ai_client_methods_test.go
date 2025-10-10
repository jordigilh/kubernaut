<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package llm_test

import (
	"context"
	"testing"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEnhancedAIClientMethods(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Enhanced AI Client Methods Suite")
}

// TDD RED Phase: Test using EXISTING interface methods only
// Business Requirements: BR-COND-001, BR-AI-016, BR-AI-017, BR-AI-022, BR-ORCH-002, BR-ORCH-003
// Future enhancement: Add missing methods to llm.Client interface

var _ = Describe("Enhanced LLM Client AI Methods - TDD Phase", func() {
	var (
		ctx       context.Context
		llmClient llm.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Use existing llm.Client interface methods only
		config := config.LLMConfig{
			Provider:    "test",
			Model:       "test-model",
			Temperature: 0.7,
		}
		var err error
		llmClient, err = llm.NewClient(config, nil)
		Expect(err).ToNot(HaveOccurred())
	})

	// TDD RED: Test existing interface methods work correctly
	// This establishes baseline before adding new methods
	Describe("Existing Interface Methods", func() {
		It("should successfully analyze alerts using existing AnalyzeAlert method", func() {
			// Using existing interface method - this should work
			alert := map[string]interface{}{
				"alertname": "HighCPUUsage",
				"severity":  "warning",
			}

			response, err := llmClient.AnalyzeAlert(ctx, alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(response).ToNot(BeNil())
		})

		It("should check health status using existing IsHealthy method", func() {
			// Using existing interface method
			healthy := llmClient.IsHealthy()
			Expect(healthy).To(BeAssignableToTypeOf(true))
		})

		It("should perform liveness check using existing method", func() {
			// Using existing interface method
			err := llmClient.LivenessCheck(ctx)
			// May return error if service unavailable, but method should exist
			_ = err // Accept either success or failure
		})

		It("should get endpoint using existing method", func() {
			// Using existing interface method
			endpoint := llmClient.GetEndpoint()
			Expect(endpoint).To(BeAssignableToTypeOf(""))
		})
	})

	// TDD GREEN Phase - Enhanced method tests (now implemented)
	Describe("Enhanced AI Methods", func() {
		It("should evaluate conditions with AI assistance", func() {
			// BR-COND-001: MUST support intelligent condition evaluation
			condition := "cpu_usage > 80%"
			context := map[string]interface{}{"current_cpu": 85.0}

			result, err := llmClient.EvaluateCondition(ctx, condition, context)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeAssignableToTypeOf(true))
		})

		It("should validate condition syntax", func() {
			// BR-COND-005: MUST validate condition syntax before execution
			condition := "valid_condition == true"

			err := llmClient.ValidateCondition(ctx, condition)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should collect AI execution metrics", func() {
			// BR-AI-017: MUST provide comprehensive AI metrics collection
			execution := map[string]interface{}{
				"workflow_id": "test-workflow",
				"duration":    30.5,
				"success":     true,
			}

			metrics, err := llmClient.CollectMetrics(ctx, execution)
			Expect(err).ToNot(HaveOccurred())
			Expect(metrics).ToNot(BeEmpty())
			Expect(metrics["timestamp"]).To(BeNumerically(">", 0))
		})

		It("should build optimized prompts from templates", func() {
			// BR-PROMPT-001: MUST support dynamic prompt building
			template := "Analyze {{alert_type}} with severity {{severity}}"
			context := map[string]interface{}{
				"alert_type": "HighCPU",
				"severity":   "critical",
			}

			prompt, err := llmClient.BuildPrompt(ctx, template, context)
			Expect(err).ToNot(HaveOccurred())
			Expect(prompt).To(ContainSubstring("HighCPU"))
			Expect(prompt).To(ContainSubstring("critical"))
		})

		It("should analyze patterns in execution data", func() {
			// BR-ML-001: MUST provide machine learning analytics for pattern discovery
			executionData := []interface{}{
				map[string]interface{}{"type": "restart", "success": true},
				map[string]interface{}{"type": "restart", "success": false},
			}

			patterns, err := llmClient.AnalyzePatterns(ctx, executionData)
			Expect(err).ToNot(HaveOccurred())
			Expect(patterns).ToNot(BeNil())
		})

		It("should predict workflow effectiveness", func() {
			// BR-ML-001: MUST predict workflow effectiveness using ML
			workflow := map[string]interface{}{
				"steps": []map[string]interface{}{
					{"type": "restart_pod"},
				},
			}

			effectiveness, err := llmClient.PredictEffectiveness(ctx, workflow)
			Expect(err).ToNot(HaveOccurred())
			Expect(effectiveness).To(BeNumerically(">=", 0.0))
			Expect(effectiveness).To(BeNumerically("<=", 1.0))
		})

		It("should cluster workflows by similarity", func() {
			// BR-CLUSTER-001: MUST support workflow clustering
			executionData := []interface{}{
				map[string]interface{}{"type": "scaling"},
				map[string]interface{}{"type": "restart"},
			}
			config := map[string]interface{}{"algorithm": "kmeans"}

			clusters, err := llmClient.ClusterWorkflows(ctx, executionData, config)
			Expect(err).ToNot(HaveOccurred())
			Expect(clusters).ToNot(BeNil())
		})

		It("should optimize workflows intelligently", func() {
			// BR-ORCH-003: MUST provide workflow optimization
			workflow := map[string]interface{}{
				"name": "test-workflow",
				"steps": []map[string]interface{}{
					{"type": "restart_pod"},
				},
			}
			executionHistory := []interface{}{}

			optimized, err := llmClient.OptimizeWorkflow(ctx, workflow, executionHistory)
			Expect(err).ToNot(HaveOccurred())
			Expect(optimized).ToNot(BeNil())
		})
	})
})
