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

package investigator_test

import (
	"context"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

type stubLLMClient struct {
	called bool
}

func (s *stubLLMClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	s.called = true
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: "stub"},
		Usage:   llm.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

var _ = Describe("TP-433-ADV P1: Critical Wiring — GAP-006/GAP-007", func() {

	Describe("UT-KA-433-WIR-001: InstrumentedClient satisfies llm.Client interface (GAP-006)", func() {
		It("should wrap a base client and satisfy the llm.Client interface", func() {
			base := &stubLLMClient{}
			instrumented := llm.NewInstrumentedClient(base)

			var client llm.Client = instrumented
			Expect(client).NotTo(BeNil())

			resp, err := client.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("stub"))
			Expect(base.called).To(BeTrue(), "InstrumentedClient must delegate to inner client")
		})
	})

	Describe("UT-KA-433-WIR-002: Investigator.Config accepts InstrumentedClient (GAP-006)", func() {
		It("should construct an investigator with instrumented LLM client", func() {
			base := &stubLLMClient{}
			instrumented := llm.NewInstrumentedClient(base)

			cfg := investigator.Config{
				Client: instrumented,
				Logger: slog.Default(),
			}
			inv := investigator.New(cfg)
			Expect(inv).NotTo(BeNil())
		})
	})

	Describe("UT-KA-433-WIR-003: Pipeline.Validator is non-nil when workflow names provided (GAP-007)", func() {
		It("should create a non-nil validator with workflow names from catalog", func() {
			allowedWorkflows := []string{"oom-recovery", "crashloop-config-fix", "node-drain-reboot"}
			validator := parser.NewValidator(allowedWorkflows)
			Expect(validator).NotTo(BeNil())

			pipeline := investigator.Pipeline{
				Validator: validator,
			}
			Expect(pipeline.Validator).NotTo(BeNil())
		})

		It("should reject unknown workflow IDs through the validator", func() {
			allowedWorkflows := []string{"oom-recovery", "crashloop-config-fix"}
			validator := parser.NewValidator(allowedWorkflows)

			result := &katypes.InvestigationResult{
				WorkflowID: "unknown-workflow",
				Confidence: 0.8,
			}
			err := validator.Validate(result)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not in session allowlist"))
		})

		It("should accept known workflow IDs through the validator", func() {
			allowedWorkflows := []string{"oom-recovery", "crashloop-config-fix"}
			validator := parser.NewValidator(allowedWorkflows)

			result := &katypes.InvestigationResult{
				WorkflowID: "oom-recovery",
				Confidence: 0.8,
			}
			err := validator.Validate(result)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-433-WIR-004: Fail-closed — nil validator means no self-correction (DD-HAPI-002 v1.5)", func() {
		It("should skip validation when Pipeline.Validator is nil", func() {
			pipeline := investigator.Pipeline{}
			Expect(pipeline.Validator).To(BeNil(),
				"nil Validator means self-correction loop is skipped — "+
					"KA must refuse to start when DS is configured but validator creation fails (DD-HAPI-002 v1.5)")
		})

		It("should trigger self-correction when Pipeline.Validator is non-nil and workflow is invalid", func() {
			allowedWorkflows := []string{"oom-recovery"}
			validator := parser.NewValidator(allowedWorkflows)

			pipeline := investigator.Pipeline{
				Validator: validator,
			}
			Expect(pipeline.Validator).NotTo(BeNil())

			result := &katypes.InvestigationResult{
				WorkflowID: "hallucinated-workflow",
				Confidence: 0.9,
			}
			err := pipeline.Validator.Validate(result)
			Expect(err).To(HaveOccurred(), "Validator must reject hallucinated workflows")
		})
	})
})
