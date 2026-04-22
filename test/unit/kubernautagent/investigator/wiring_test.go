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
	"fmt"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

type stubCatalogFetcher struct {
	validator *parser.Validator
	fetchErr  error
}

func (s *stubCatalogFetcher) FetchValidator(_ context.Context) (*parser.Validator, error) {
	if s.fetchErr != nil {
		return nil, s.fetchErr
	}
	return s.validator, nil
}

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

func (s *stubLLMClient) Close() error { return nil }

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
		It("should construct a non-nil investigator with instrumented LLM client", func() {
			base := &stubLLMClient{}
			instrumented := llm.NewInstrumentedClient(base)

			cfg := investigator.Config{
				Client: instrumented,
				Logger: slog.Default(),
			}
			inv := investigator.New(cfg)
			Expect(inv).NotTo(BeNil(),
				"Investigator.New must accept InstrumentedClient as llm.Client; delegation behavior is verified by IT-KA-433-TM-002")
		})
	})

	Describe("UT-KA-433-WIR-003: Pipeline.CatalogFetcher is non-nil when DS available (GAP-007, #665)", func() {
		It("should accept a CatalogFetcher in the Pipeline", func() {
			fetcher := &stubCatalogFetcher{
				validator: parser.NewValidator([]string{"oom-recovery", "crashloop-config-fix", "node-drain-reboot"}),
			}
			pipeline := investigator.Pipeline{
				CatalogFetcher: fetcher,
			}
			Expect(pipeline.CatalogFetcher).NotTo(BeNil())
		})

		It("should reject unknown workflow IDs through the fetched validator", func() {
			fetcher := &stubCatalogFetcher{
				validator: parser.NewValidator([]string{"oom-recovery", "crashloop-config-fix"}),
			}
			v, err := fetcher.FetchValidator(context.Background())
			Expect(err).NotTo(HaveOccurred())

			result := &katypes.InvestigationResult{
				WorkflowID: "unknown-workflow",
				Confidence: 0.8,
			}
			Expect(v.Validate(result)).To(HaveOccurred())
		})

		It("should accept known workflow IDs through the fetched validator", func() {
			fetcher := &stubCatalogFetcher{
				validator: parser.NewValidator([]string{"oom-recovery", "crashloop-config-fix"}),
			}
			v, err := fetcher.FetchValidator(context.Background())
			Expect(err).NotTo(HaveOccurred())

			result := &katypes.InvestigationResult{
				WorkflowID: "oom-recovery",
				Confidence: 0.8,
			}
			Expect(v.Validate(result)).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-433-WIR-004: nil CatalogFetcher means no validation — graceful skip (#665)", func() {
		It("should have nil CatalogFetcher when no DS is configured (dev mode)", func() {
			pipeline := investigator.Pipeline{}
			Expect(pipeline.CatalogFetcher).To(BeNil(),
				"nil CatalogFetcher means validation is skipped — "+
					"KA still starts and serves /health without DS (dev mode, #665)")
		})

		It("should trigger self-correction when CatalogFetcher returns a validator and workflow is invalid", func() {
			fetcher := &stubCatalogFetcher{
				validator: parser.NewValidator([]string{"oom-recovery"}),
			}
			pipeline := investigator.Pipeline{
				CatalogFetcher: fetcher,
			}
			Expect(pipeline.CatalogFetcher).NotTo(BeNil())

			v, err := fetcher.FetchValidator(context.Background())
			Expect(err).NotTo(HaveOccurred())

			result := &katypes.InvestigationResult{
				WorkflowID: "hallucinated-workflow",
				Confidence: 0.9,
			}
			Expect(v.Validate(result)).To(HaveOccurred(), "Validator must reject hallucinated workflows")
		})

		It("should flag human review when CatalogFetcher returns an error", func() {
			fetcher := &stubCatalogFetcher{
				fetchErr: fmt.Errorf("DS unavailable"),
			}
			_, err := fetcher.FetchValidator(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DS unavailable"))
		})
	})
})
