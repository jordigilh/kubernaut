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

package alignment_test

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func cleanResponseWithUsage(prompt, completion, total int) llm.ChatResponse {
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: `{"suspicious":false,"explanation":"clean"}`},
		Usage:   llm.TokenUsage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total},
	}
}

func suspiciousResponseWithUsage(prompt, completion, total int) llm.ChatResponse {
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: `{"suspicious":true,"explanation":"injection detected"}`},
		Usage:   llm.TokenUsage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total},
	}
}

var _ = Describe("Shadow Agent LLM Token Audit — #1059", func() {

	Describe("UT-SA-1059-001: Evaluator with WithAuditStore emits shadow.llm.request before Chat", func() {
		It("should emit a shadow.llm.request event with correct fields", func() {
			store := &mockAuditStore{}
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponseWithUsage(10, 20, 30)}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "", alignment.WithAuditStore(store))

			step := alignment.Step{
				Index:         0,
				Kind:          alignment.StepKindToolResult,
				Tool:          "get_pods",
				Content:       "pod list output",
				CorrelationID: "rr-test-001",
			}
			evaluator.EvaluateStep(context.Background(), step)

			store.mu.Lock()
			defer store.mu.Unlock()
			reqEvents := filterEvents(store.events, audit.EventTypeShadowLLMRequest)
			Expect(reqEvents).To(HaveLen(1), "exactly one shadow.llm.request event expected")
			Expect(reqEvents[0].CorrelationID).To(Equal("rr-test-001"))
			Expect(reqEvents[0].Data["step_index"]).To(Equal(0))
			Expect(reqEvents[0].Data["step_kind"]).To(Equal("tool_result"))
			Expect(reqEvents[0].Data["prompt_length"]).To(BeNumerically(">", 0))
		})
	})

	Describe("UT-SA-1059-002: Evaluator emits shadow.llm.response on successful Chat", func() {
		It("should emit a shadow.llm.response event with token usage matching resp.Usage", func() {
			store := &mockAuditStore{}
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponseWithUsage(10, 20, 30)}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "", alignment.WithAuditStore(store))

			step := alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Content: "data",
				CorrelationID: "rr-test-002",
			}
			evaluator.EvaluateStep(context.Background(), step)

			store.mu.Lock()
			defer store.mu.Unlock()
			respEvents := filterEvents(store.events, audit.EventTypeShadowLLMResponse)
			Expect(respEvents).To(HaveLen(1), "exactly one shadow.llm.response event expected")
			Expect(respEvents[0].Data["prompt_tokens"]).To(Equal(10))
			Expect(respEvents[0].Data["completion_tokens"]).To(Equal(20))
			Expect(respEvents[0].Data["total_tokens"]).To(Equal(30))
			Expect(respEvents[0].Data["attempt"]).To(Equal(1))
			Expect(respEvents[0].Data["evaluation_result"]).To(Equal("success"))
		})
	})

	Describe("UT-SA-1059-003: Evaluator does NOT emit when CorrelationID is empty (canary path)", func() {
		It("should emit zero audit events when step.CorrelationID is empty", func() {
			store := &mockAuditStore{}
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponseWithUsage(10, 20, 30)}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "", alignment.WithAuditStore(store))

			step := alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Content: "canary test",
			}
			evaluator.EvaluateStep(context.Background(), step)

			store.mu.Lock()
			defer store.mu.Unlock()
			Expect(store.events).To(BeEmpty(), "canary path (empty CorrelationID) must not emit audit events")
		})
	})

	Describe("UT-SA-1059-004: Evaluator does NOT emit shadow.llm.response on failed Chat", func() {
		It("should emit only 1 request event and 0 response events when all retries fail", func() {
			store := &mockAuditStore{}
			client := &mockLLMClient{
				errs: []error{errors.New("fail1"), errors.New("fail2"), errors.New("fail3")},
			}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 2 * time.Second, MaxStepTokens: 4000, MaxRetries: 3,
			}, "", alignment.WithAuditStore(store))

			step := alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Content: "content",
				CorrelationID: "rr-fail-001",
			}
			obs := evaluator.EvaluateStep(context.Background(), step)
			Expect(obs.Suspicious).To(BeTrue(), "all retries failed => fail-closed")

			store.mu.Lock()
			defer store.mu.Unlock()
			reqEvents := filterEvents(store.events, audit.EventTypeShadowLLMRequest)
			respEvents := filterEvents(store.events, audit.EventTypeShadowLLMResponse)
			Expect(reqEvents).To(HaveLen(1), "exactly one request event before retry loop")
			Expect(respEvents).To(BeEmpty(), "no response event when Chat never succeeds")
		})
	})

	Describe("UT-SA-1059-005: Observation carries Usage from successful response", func() {
		It("should populate obs.Usage.TotalTokens from the LLM response", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponseWithUsage(10, 20, 30)}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")

			step := alignment.Step{Index: 0, Kind: alignment.StepKindToolResult, Content: "data"}
			obs := evaluator.EvaluateStep(context.Background(), step)
			Expect(obs.Usage.PromptTokens).To(Equal(10))
			Expect(obs.Usage.CompletionTokens).To(Equal(20))
			Expect(obs.Usage.TotalTokens).To(Equal(30))
		})
	})

	Describe("UT-SA-1059-006: Zero-usage path emits response event with tokens_used=0", func() {
		It("should emit shadow.llm.response even when token usage is zero", func() {
			store := &mockAuditStore{}
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponseWithUsage(0, 0, 0)}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "", alignment.WithAuditStore(store))

			step := alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Content: "data",
				CorrelationID: "rr-zero-001",
			}
			evaluator.EvaluateStep(context.Background(), step)

			store.mu.Lock()
			defer store.mu.Unlock()
			respEvents := filterEvents(store.events, audit.EventTypeShadowLLMResponse)
			Expect(respEvents).To(HaveLen(1), "response event emitted even with zero tokens")
			Expect(respEvents[0].Data["total_tokens"]).To(Equal(0))
		})
	})

	Describe("UT-SA-1059-007: Evaluator without WithAuditStore emits no events", func() {
		It("should produce correct observation without any audit side effects", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponseWithUsage(10, 20, 30)}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")

			step := alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Content: "data",
				CorrelationID: "rr-noaudit-001",
			}
			obs := evaluator.EvaluateStep(context.Background(), step)
			Expect(obs.Suspicious).To(BeFalse())
			Expect(obs.Usage.TotalTokens).To(Equal(30))
		})
	})

	Describe("UT-SA-1059-008: Observer stamps CorrelationID on steps in SubmitAsync", func() {
		It("should set step.CorrelationID to the observer's correlationID", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")

			observer, err := alignment.NewObserver(evaluator, alignment.WithCorrelationID("rr-stamp-001"))
			Expect(err).NotTo(HaveOccurred())

			observer.SubmitAsync(context.Background(), alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Content: "data",
			})

			wr := observer.WaitForCompletion(5 * time.Second)
			Expect(wr.Complete).To(BeTrue())
			Expect(wr.Observations).To(HaveLen(1))
			Expect(wr.Observations[0].Step.CorrelationID).To(Equal("rr-stamp-001"),
				"Observer must stamp its correlationID on every submitted step")
		})
	})

	Describe("UT-SA-1059-009: Observer with empty correlationID stamps empty string", func() {
		It("should leave step.CorrelationID as empty when observer has no correlationID", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")

			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())

			observer.SubmitAsync(context.Background(), alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Content: "data",
			})

			wr := observer.WaitForCompletion(5 * time.Second)
			Expect(wr.Observations).To(HaveLen(1))
			Expect(wr.Observations[0].Step.CorrelationID).To(BeEmpty())
		})
	})

	Describe("UT-SA-1059-014: Verdict event includes summed shadow tokens", func() {
		It("should include exact shadow_prompt_tokens, shadow_completion_tokens, shadow_total_tokens from observer-submitted steps", func() {
			store := &mockAuditStore{}
			innerRes := &katypes.InvestigationResult{
				RCASummary: "test", Confidence: 0.9,
			}
			inner := &mockInvestigationRunnerWithObserver{
				result: innerRes,
				onInvestigate: func(ctx context.Context) {
					obs := alignment.ObserverFromContext(ctx)
					if obs != nil {
						obs.SubmitAsync(ctx, alignment.Step{
							Index: obs.NextStepIndex(), Kind: alignment.StepKindToolResult,
							Tool: "tool1", Content: "output1",
						})
						obs.SubmitAsync(ctx, alignment.Step{
							Index: obs.NextStepIndex(), Kind: alignment.StepKindToolResult,
							Tool: "tool2", Content: "output2",
						})
					}
				},
			}

			// Mock responses consumed in order:
			// 1. Canary (no CorrelationID, bypasses Observer): suspiciousResponse(5,10,15)
			// 2. Signal input step (via Observer): clean(10,20,30)
			// 3. Tool1 step (via Observer): clean(15,25,40)
			// 4. Tool2 step (via Observer): clean(20,30,50)
			// Expected verdict sum = steps 2+3+4: prompt=45, completion=75, total=120
			shadowClient := &mockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponseWithUsage(5, 10, 15),
				cleanResponseWithUsage(10, 20, 30),
				cleanResponseWithUsage(15, 25, 40),
				cleanResponseWithUsage(20, 30, 50),
			}}
			evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "", alignment.WithAuditStore(store))

			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				AuditStore:     store,
				Logger:         logr.Discard(),
				Mode:           config.AlignmentModeMonitor,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{
				Name: "test-signal", Namespace: "ns", Message: "alert",
				RemediationID: "rr-token-sum-001",
			}
			_, investigateErr := wrapper.Investigate(context.Background(), sig)
			Expect(investigateErr).NotTo(HaveOccurred())

			store.mu.Lock()
			defer store.mu.Unlock()
			verdictEvents := filterEvents(store.events, audit.EventTypeAlignmentVerdict)
			Expect(verdictEvents).To(HaveLen(1), "exactly one verdict event")

			v := verdictEvents[0]
			Expect(v.Data["shadow_prompt_tokens"]).To(Equal(45),
				"shadow_prompt_tokens should be sum of all observer-submitted steps: 10+15+20=45")
			Expect(v.Data["shadow_completion_tokens"]).To(Equal(75),
				"shadow_completion_tokens should be sum of all observer-submitted steps: 20+25+30=75")
			Expect(v.Data["shadow_total_tokens"]).To(Equal(120),
				"shadow_total_tokens should be sum of all observer-submitted steps: 30+40+50=120")
		})
	})

	Describe("UT-SA-1059-015: correlationID passed to NewObserver matches signal identity", func() {
		It("should use signal.RemediationID as correlation when available", func() {
			store := &mockAuditStore{}
			innerRes := &katypes.InvestigationResult{RCASummary: "test", Confidence: 0.9}
			inner := &mockInvestigationRunner{result: innerRes}
			shadowClient := &mockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponseWithUsage(5, 10, 15),
				cleanResponseWithUsage(10, 20, 30),
			}}
			evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "", alignment.WithAuditStore(store))

			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				AuditStore:     store,
				Logger:         logr.Discard(),
				Mode:           config.AlignmentModeMonitor,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{
				Name: "test-signal", Namespace: "ns", Message: "alert",
				RemediationID: "rr-corr-001",
			}
			_, investigateErr := wrapper.Investigate(context.Background(), sig)
			Expect(investigateErr).NotTo(HaveOccurred())

			store.mu.Lock()
			defer store.mu.Unlock()
			shadowReqEvents := filterEvents(store.events, audit.EventTypeShadowLLMRequest)
			for _, e := range shadowReqEvents {
				Expect(e.CorrelationID).To(Equal("rr-corr-001"),
					"shadow LLM events must carry the signal's RemediationID as correlation")
			}
		})
	})

	Describe("QE-HIGH-1: JSON parse failure path does NOT emit shadow.llm.response", func() {
		It("should emit only request event when json.Unmarshal fails on all retries", func() {
			store := &mockAuditStore{}
			client := &mockLLMClient{responses: []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: "not valid json"}, Usage: llm.TokenUsage{TotalTokens: 50}},
				{Message: llm.Message{Role: "assistant", Content: "still invalid"}, Usage: llm.TokenUsage{TotalTokens: 60}},
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 2,
			}, "", alignment.WithAuditStore(store))

			step := alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Content: "data",
				CorrelationID: "rr-json-fail-001",
			}
			obs := evaluator.EvaluateStep(context.Background(), step)
			Expect(obs.Suspicious).To(BeTrue(), "fail-closed on JSON parse failure")

			store.mu.Lock()
			defer store.mu.Unlock()
			reqEvents := filterEvents(store.events, audit.EventTypeShadowLLMRequest)
			respEvents := filterEvents(store.events, audit.EventTypeShadowLLMResponse)
			Expect(reqEvents).To(HaveLen(1), "one request event before retry loop")
			Expect(respEvents).To(BeEmpty(), "no response event when all attempts produce unparseable JSON")
		})
	})

	Describe("QE-HIGH-1b: duplicate-key attack path emits response with evaluation_result=malformed_response", func() {
		It("should emit response event with evaluation_result=malformed_response for duplicate key attack", func() {
			store := &mockAuditStore{}
			client := &mockLLMClient{responses: []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant",
					Content: `{"suspicious":true,"suspicious":false}`},
					Usage: llm.TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}},
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "", alignment.WithAuditStore(store))

			step := alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Content: "data",
				CorrelationID: "rr-dupkey-001",
			}
			obs := evaluator.EvaluateStep(context.Background(), step)
			Expect(obs.Suspicious).To(BeTrue())

			store.mu.Lock()
			defer store.mu.Unlock()
			respEvents := filterEvents(store.events, audit.EventTypeShadowLLMResponse)
			Expect(respEvents).To(HaveLen(1))
			Expect(respEvents[0].Data["evaluation_result"]).To(Equal("malformed_response"))
			Expect(respEvents[0].Data["total_tokens"]).To(Equal(30))
		})
	})

	Describe("QE-HIGH-1c: missing suspicious field emits response with evaluation_result=missing_field", func() {
		It("should emit response event with evaluation_result=missing_field when suspicious key absent", func() {
			store := &mockAuditStore{}
			client := &mockLLMClient{responses: []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant",
					Content: `{"explanation":"no suspicious key"}`},
					Usage: llm.TokenUsage{PromptTokens: 5, CompletionTokens: 15, TotalTokens: 20}},
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "", alignment.WithAuditStore(store))

			step := alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Content: "data",
				CorrelationID: "rr-missfield-001",
			}
			obs := evaluator.EvaluateStep(context.Background(), step)
			Expect(obs.Suspicious).To(BeTrue())

			store.mu.Lock()
			defer store.mu.Unlock()
			respEvents := filterEvents(store.events, audit.EventTypeShadowLLMResponse)
			Expect(respEvents).To(HaveLen(1))
			Expect(respEvents[0].Data["evaluation_result"]).To(Equal("missing_field"))
			Expect(respEvents[0].Data["total_tokens"]).To(Equal(20))
		})
	})

	Describe("QE-MEDIUM-1: signal.Name fallback when RemediationID is empty", func() {
		It("should use signal.Name as correlationID for shadow events when RemediationID is empty", func() {
			store := &mockAuditStore{}
			innerRes := &katypes.InvestigationResult{RCASummary: "test", Confidence: 0.9}
			inner := &mockInvestigationRunner{result: innerRes}
			shadowClient := &mockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponseWithUsage(5, 10, 15),
				cleanResponseWithUsage(10, 20, 30),
			}}
			evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "", alignment.WithAuditStore(store))

			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				AuditStore:     store,
				Logger:         logr.Discard(),
				Mode:           config.AlignmentModeMonitor,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{
				Name: "alert-crash-loop", Namespace: "ns", Message: "alert",
			}
			_, investigateErr := wrapper.Investigate(context.Background(), sig)
			Expect(investigateErr).NotTo(HaveOccurred())

			store.mu.Lock()
			defer store.mu.Unlock()
			shadowReqEvents := filterEvents(store.events, audit.EventTypeShadowLLMRequest)
			Expect(shadowReqEvents).NotTo(BeEmpty(), "shadow request events expected for signal input step")
			for _, e := range shadowReqEvents {
				Expect(e.CorrelationID).To(Equal("alert-crash-loop"),
					"when RemediationID is empty, signal.Name should be used as correlationID")
			}

			verdictEvents := filterEvents(store.events, audit.EventTypeAlignmentVerdict)
			Expect(verdictEvents).To(HaveLen(1))
			Expect(verdictEvents[0].CorrelationID).To(Equal("alert-crash-loop"))
		})
	})

	Describe("IT-SA-1059-001: Full wrapper flow emits shadow LLM events with correct correlation ID", func() {
		It("should emit shadow.llm.request + shadow.llm.response per evaluated step with matching correlationID", func() {
			store := &mockAuditStore{}
			innerRes := &katypes.InvestigationResult{RCASummary: "root cause found", Confidence: 0.95}
			inner := &mockInvestigationRunnerWithObserver{
				result: innerRes,
				onInvestigate: func(ctx context.Context) {
					obs := alignment.ObserverFromContext(ctx)
					if obs != nil {
						obs.SubmitAsync(ctx, alignment.Step{
							Index: obs.NextStepIndex(), Kind: alignment.StepKindToolResult,
							Tool: "get_pods", Content: "pod-web-1 Running",
						})
						obs.SubmitAsync(ctx, alignment.Step{
							Index: obs.NextStepIndex(), Kind: alignment.StepKindLLMReasoning,
							Content: "The pod is running fine.",
						})
						obs.SubmitAsync(ctx, alignment.Step{
							Index: obs.NextStepIndex(), Kind: alignment.StepKindToolResult,
							Tool: "get_logs", Content: "OOMKilled",
						})
					}
				},
			}

			shadowClient := &mockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponseWithUsage(5, 10, 15),
				cleanResponseWithUsage(10, 20, 30),
				cleanResponseWithUsage(12, 22, 34),
				cleanResponseWithUsage(8, 18, 26),
				cleanResponseWithUsage(15, 25, 40),
			}}
			evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "", alignment.WithAuditStore(store))

			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 10 * time.Second,
				AuditStore:     store,
				Logger:         logr.Discard(),
				Mode:           config.AlignmentModeMonitor,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{
				Name: "alert-oom", Namespace: "prod", Message: "OOMKilled",
				RemediationID: "rr-integ-001",
			}
			result, investigateErr := wrapper.Investigate(context.Background(), sig)
			Expect(investigateErr).NotTo(HaveOccurred())
			Expect(result.RCASummary).To(Equal("root cause found"))

			store.mu.Lock()
			defer store.mu.Unlock()

			shadowReqEvents := filterEvents(store.events, audit.EventTypeShadowLLMRequest)
			shadowRespEvents := filterEvents(store.events, audit.EventTypeShadowLLMResponse)

			Expect(shadowReqEvents).NotTo(BeEmpty())
			Expect(shadowRespEvents).NotTo(BeEmpty())
			Expect(shadowReqEvents).To(HaveLen(len(shadowRespEvents)),
				"each request should have a matching response for successful evaluations")

			for _, e := range shadowReqEvents {
				Expect(e.CorrelationID).To(Equal("rr-integ-001"))
				Expect(e.Data["step_kind"]).NotTo(BeEmpty())
			}
			for _, e := range shadowRespEvents {
				Expect(e.CorrelationID).To(Equal("rr-integ-001"))
				Expect(e.Data["evaluation_result"]).To(Equal("success"))
			}
		})
	})

	Describe("IT-SA-1059-002: Verdict event in full flow includes aggregate shadow token totals", func() {
		It("should sum all observation token usage and include on verdict event", func() {
			store := &mockAuditStore{}
			innerRes := &katypes.InvestigationResult{RCASummary: "test", Confidence: 0.9}
			inner := &mockInvestigationRunnerWithObserver{
				result: innerRes,
				onInvestigate: func(ctx context.Context) {
					obs := alignment.ObserverFromContext(ctx)
					if obs != nil {
						obs.SubmitAsync(ctx, alignment.Step{
							Index: obs.NextStepIndex(), Kind: alignment.StepKindToolResult,
							Tool: "get_pods", Content: "output1",
						})
						obs.SubmitAsync(ctx, alignment.Step{
							Index: obs.NextStepIndex(), Kind: alignment.StepKindToolResult,
							Tool: "get_logs", Content: "output2",
						})
					}
				},
			}

			shadowClient := &mockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponseWithUsage(5, 10, 15),
				cleanResponseWithUsage(10, 20, 30),
				cleanResponseWithUsage(15, 25, 40),
				cleanResponseWithUsage(20, 30, 50),
			}}
			evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "", alignment.WithAuditStore(store))

			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				AuditStore:     store,
				Logger:         logr.Discard(),
				Mode:           config.AlignmentModeMonitor,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{
				Name: "test-signal", Namespace: "ns", Message: "alert",
				RemediationID: "rr-integ-sum-001",
			}
			_, investigateErr := wrapper.Investigate(context.Background(), sig)
			Expect(investigateErr).NotTo(HaveOccurred())

			store.mu.Lock()
			defer store.mu.Unlock()

			shadowRespEvents := filterEvents(store.events, audit.EventTypeShadowLLMResponse)
			var expectedPrompt, expectedCompletion, expectedTotal int
			for _, e := range shadowRespEvents {
				expectedPrompt += e.Data["prompt_tokens"].(int)
				expectedCompletion += e.Data["completion_tokens"].(int)
				expectedTotal += e.Data["total_tokens"].(int)
			}

			verdictEvents := filterEvents(store.events, audit.EventTypeAlignmentVerdict)
			Expect(verdictEvents).To(HaveLen(1))

			v := verdictEvents[0]
			if expectedTotal > 0 {
				Expect(v.Data["shadow_prompt_tokens"]).To(Equal(expectedPrompt),
					"verdict shadow_prompt_tokens should match sum of per-step prompt tokens")
				Expect(v.Data["shadow_completion_tokens"]).To(Equal(expectedCompletion),
					"verdict shadow_completion_tokens should match sum of per-step completion tokens")
				Expect(v.Data["shadow_total_tokens"]).To(Equal(expectedTotal),
					"verdict shadow_total_tokens should match sum of per-step total tokens")
			}
		})
	})
})

func filterEvents(events []*audit.AuditEvent, eventType string) []*audit.AuditEvent {
	var filtered []*audit.AuditEvent
	for _, e := range events {
		if e.EventType == eventType {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
