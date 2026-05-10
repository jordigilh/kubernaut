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
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// pinDecoratorSpy records whether the PinDecorator was called.
type pinDecoratorSpy struct {
	called   atomic.Bool
	received llm.Client
	mu       sync.Mutex
}

func (d *pinDecoratorSpy) fn(c llm.Client) llm.Client {
	d.called.Store(true)
	d.mu.Lock()
	d.received = c
	d.mu.Unlock()
	return llm.NewInstrumentedClient(c)
}

// pinSubmitClient returns a submit_result tool call on the first Chat,
// then a workflow selection on the second.
type pinSubmitClient struct {
	mu    sync.Mutex
	calls int
}

func (s *pinSubmitClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	s.mu.Lock()
	call := s.calls
	s.calls++
	s.mu.Unlock()

	if call == 0 {
		return llm.ChatResponse{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tc-1",
					Name: "submit_result",
					Arguments: `{"rca_summary":"test rca","confidence":0.9,` +
						`"remediation_target":{"kind":"Pod","name":"test","namespace":"default"}}`,
				},
			},
			Usage: llm.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}, nil
	}
	return llm.ChatResponse{
		ToolCalls: []llm.ToolCall{
			{ID: "tc-wf", Name: "submit_result_with_workflow", Arguments: `{"workflow_id":"fix","confidence":0.9}`},
		},
		Usage: llm.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

func (s *pinSubmitClient) Close() error { return nil }

var _ = Describe("PinDecorator — C-1 LLMProxy Bypass Fix", func() {
	var (
		signal   katypes.SignalContext
		store    *gateRecordingAuditStore
		logger   = logr.Discard()
	)

	BeforeEach(func() {
		signal = katypes.SignalContext{
			Name:          "test-pod",
			Namespace:     "default",
			Severity:      "high",
			Message:       "OOMKilled",
			RemediationID: "rem-pin-001",
		}
		store = &gateRecordingAuditStore{}
	})

	Describe("UT-SA-C1-001: PinDecorator is called when set and Swappable is non-nil", func() {
		It("should invoke PinDecorator with the snapshotted client", func() {
			innerClient := &pinSubmitClient{}
			spy := &pinDecoratorSpy{}

			swappable, err := llm.NewSwappableClient(innerClient, "test-model")
			Expect(err).NotTo(HaveOccurred())

			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			enricher := enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, store, logger)

			inv := investigator.New(investigator.Config{
				Client:       innerClient,
				Builder:      builder,
				ResultParser: parser.NewResultParser(),
				Enricher:     enricher,
				AuditStore:   store,
				Logger:       logger,
				MaxTurns:     5,
				PhaseTools:   investigator.DefaultPhaseToolMap(),
				Swappable:    swappable,
				PinDecorator: spy.fn,
			})

			_, _ = inv.Investigate(context.Background(), signal)

			Expect(spy.called.Load()).To(BeTrue(),
				"PinDecorator must be called when Swappable is non-nil")
		})
	})

	Describe("UT-SA-C1-002: Without PinDecorator, falls back to NewInstrumentedClient", func() {
		It("should not panic and should use default instrumented client", func() {
			innerClient := &pinSubmitClient{}

			swappable, err := llm.NewSwappableClient(innerClient, "test-model")
			Expect(err).NotTo(HaveOccurred())

			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			enricher := enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, store, logger)

			inv := investigator.New(investigator.Config{
				Client:       innerClient,
				Builder:      builder,
				ResultParser: parser.NewResultParser(),
				Enricher:     enricher,
				AuditStore:   store,
				Logger:       logger,
				MaxTurns:     5,
				PhaseTools:   investigator.DefaultPhaseToolMap(),
				Swappable:    swappable,
			})

			Expect(func() {
				_, _ = inv.Investigate(context.Background(), signal)
			}).NotTo(Panic(), "nil PinDecorator must fall back to NewInstrumentedClient")
		})
	})
})
