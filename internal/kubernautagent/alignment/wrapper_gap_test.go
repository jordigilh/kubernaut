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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/security/boundary"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("GAP-2: InvestigatorWrapper inner error skips verdict — BR-AI-601", func() {

	Describe("UT-GAP2-001: Inner error returns error without waiting for verdict", func() {
		It("should propagate inner Investigate error and not apply alignment verdict", func() {
			innerErr := errors.New("inner runner failed: context deadline exceeded")
			inner := &mockInvestigationRunner{result: nil, err: innerErr}
			client := &mockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponse(), // canary
				cleanResponse(),      // signal step (prevents fail-closed circuit breaker race)
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				Logger:         logr.Discard(),
				Mode:           config.AlignmentModeMonitor,
			})
			Expect(err).NotTo(HaveOccurred())

			res, err := wrapper.Investigate(context.Background(), katypes.SignalContext{
				Name: "test", Namespace: "ns", Message: "m",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("inner runner failed"))
			Expect(res).To(BeNil())
		})
	})
})

var _ = Describe("GAP-3: JSON parse error then retry success — BR-AI-601", func() {

	Describe("UT-GAP3-001: First attempt returns garbage JSON, second succeeds", func() {
		It("should retry and return the successful parsed response", func() {
			garbageResp := llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: "not json at all"},
			}
			client := &mockLLMClient{
				responses: []llm.ChatResponse{garbageResp, cleanResponse()},
			}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 2,
			}, "")
			step := alignment.Step{Index: 0, Kind: alignment.StepKindToolResult, Content: "data"}

			obs := evaluator.EvaluateStep(context.Background(), step)
			Expect(obs.Suspicious).To(BeFalse(), "second attempt returns clean — should succeed")
			Expect(client.chatCalls()).To(Equal(2), "should have retried once")
		})
	})
})

var _ = Describe("GAP-4: LLMProxy inner Chat error — BR-AI-601", func() {

	Describe("UT-GAP4-001: Inner Chat error propagates and skips shadow submission", func() {
		It("should return the error and not submit any observation", func() {
			inner := &mockLLMClient{errs: []error{errors.New("connection refused")}}
			shadowClient := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())
			ctx := alignment.WithObserver(context.Background(), observer)

			proxy := alignment.NewLLMProxy(inner)
			_, err = proxy.Chat(ctx, llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "ping"}},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection refused"))

			wr := observer.WaitForCompletion(1 * time.Second)
			Expect(wr.Observations).To(BeEmpty(),
				"inner error must not submit observation to shadow")
		})
	})
})

var _ = Describe("GAP-5: LLMProxy empty response content — BR-AI-601", func() {

	Describe("UT-GAP5-001: Empty response content skips shadow submission", func() {
		It("should not submit observation when inner returns empty content", func() {
			inner := &mockLLMClient{responses: []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: ""}},
			}}
			shadowClient := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())
			ctx := alignment.WithObserver(context.Background(), observer)

			proxy := alignment.NewLLMProxy(inner)
			resp, err := proxy.Chat(ctx, llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "ping"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(BeEmpty())

			wr := observer.WaitForCompletion(1 * time.Second)
			Expect(wr.Observations).To(BeEmpty(),
				"empty response content must not submit observation to shadow")
		})
	})
})

var _ = Describe("GAP-6: LLMProxy no observer in context — BR-AI-601", func() {

	Describe("UT-GAP6-001: Chat without observer in context does not panic", func() {
		It("should delegate to inner and return normally without shadow submission", func() {
			inner := &mockLLMClient{responses: []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: "hello"}},
			}}
			proxy := alignment.NewLLMProxy(inner)

			resp, err := proxy.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "ping"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("hello"))
			Expect(inner.chatCalls()).To(Equal(1))
		})
	})
})

var _ = Describe("GAP-7: ToolProxy empty success result — BR-AI-601", func() {

	Describe("UT-GAP7-001: Empty tool result skips shadow submission", func() {
		It("should not submit observation when tool returns empty string with no error", func() {
			inner := &mockToolRegistry{executeResult: "", executeErr: nil}
			shadowClient := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())
			ctx := alignment.WithObserver(context.Background(), observer)

			proxy := alignment.NewToolProxy(inner)
			result, err := proxy.Execute(ctx, "get_pods", json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())

			wr := observer.WaitForCompletion(1 * time.Second)
			Expect(wr.Observations).To(BeEmpty(),
				"empty tool result with no error must not submit observation")
		})
	})
})

var _ = Describe("GAP-8: ToolProxy no observer in context — BR-AI-601", func() {

	Describe("UT-GAP8-001: Execute without observer in context does not panic", func() {
		It("should delegate to inner and return normally without shadow submission", func() {
			inner := &mockToolRegistry{executeResult: `{"status":"ok"}`}
			proxy := alignment.NewToolProxy(inner)

			result, err := proxy.Execute(context.Background(), "get_pods", json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`{"status":"ok"}`))
			Expect(inner.executeCalls).To(Equal(1))
		})
	})
})

var _ = Describe("GAP-9: ContainsEscape + Wrap round-trip — BR-AI-601", func() {

	Describe("UT-GAP9-001: Wrap then ContainsEscape round-trip", func() {
		It("should detect the closing marker in wrapped content", func() {
			token := "abc123deadbeef4567890abcdef01234"
			content := "some tool output"
			wrapped := boundary.Wrap(content, token)
			Expect(wrapped).To(ContainSubstring("<<<EVAL_" + token + ">>>"))
			Expect(wrapped).To(ContainSubstring("<<<END_EVAL_" + token + ">>>"))
			Expect(boundary.ContainsEscape(wrapped, token)).To(BeTrue(),
				"wrapped content must contain the closing marker")
		})
	})

	Describe("UT-GAP9-002: ContainsEscape on benign content returns false", func() {
		It("should not detect escape in content without boundary markers", func() {
			token := "abc123deadbeef4567890abcdef01234"
			Expect(boundary.ContainsEscape("normal tool output", token)).To(BeFalse())
		})
	})
})

var _ = Describe("GAP-10: EvaluateStep empty shadow content — BR-AI-601", func() {

	Describe("UT-GAP10-001: Shadow returns empty content triggers retry/fail-closed", func() {
		It("should treat empty shadow response as parse error and fail-closed after retries", func() {
			emptyResp := llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: ""},
			}
			client := &mockLLMClient{responses: []llm.ChatResponse{emptyResp}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")
			step := alignment.Step{Index: 0, Kind: alignment.StepKindToolResult, Content: "data"}

			obs := evaluator.EvaluateStep(context.Background(), step)
			Expect(obs.Suspicious).To(BeTrue(),
				"empty shadow response should fail-closed")
			Expect(obs.Explanation).To(ContainSubstring("fail-closed"))
		})
	})
})

var _ = Describe("GAP-11: Audit store emission — BR-AI-601", func() {

	Describe("UT-GAP11-001: emitAlignmentAudit calls store for suspicious steps and verdict", func() {
		It("should emit step-level and verdict-level audit events via non-nil store", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "rca", Confidence: 0.9, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunner{result: innerRes}

			client := &mockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponse(), // canary (pass)
				suspiciousResponse(), // signal step → suspicious
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")

			store := &mockAuditStore{}

			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				Logger:         logr.Discard(),
				AuditStore:     store,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{Name: "test-signal", Namespace: "ns", Severity: "high", Message: "injection attempt"}
			_, err = wrapper.Investigate(context.Background(), sig)
			Expect(err).NotTo(HaveOccurred())

			Expect(store.events).To(HaveLen(2),
				"should emit 1 alignment.step event + 1 alignment.verdict event")

			var hasStep, hasVerdict bool
			for _, ev := range store.events {
				if ev.EventType == "aiagent.alignment.step" {
					hasStep = true
					Expect(ev.Data["explanation"]).NotTo(BeEmpty())
				}
				if ev.EventType == "aiagent.alignment.verdict" {
					hasVerdict = true
					Expect(ev.Data["result"]).To(Equal("suspicious"))
				}
			}
			Expect(hasStep).To(BeTrue(), "must emit alignment step event")
			Expect(hasVerdict).To(BeTrue(), "must emit alignment verdict event")
		})
	})

	Describe("UT-GAP11-002: nil audit store does not panic", func() {
		It("should skip audit emission when auditStore is nil", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "rca", Confidence: 0.9, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunner{result: innerRes}
			client := &mockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponse(), // canary
				cleanResponse(),      // signal step
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				Logger:         logr.Discard(),
				AuditStore:     nil,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{Name: "s", Namespace: "ns", Severity: "high", Message: "m"}
			res, err := wrapper.Investigate(context.Background(), sig)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
		})
	})
})

var _ = Describe("GAP-12: RenderVerdict mixed summary — BR-AI-601", func() {

	Describe("UT-GAP12-001: Mixed LLM+tool observations produce combined summary", func() {
		It("should include both tool and LLM step details in verdict summary", func() {
			client := &concurrentMockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponse(),
				suspiciousResponse(),
				suspiciousResponse(),
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())

			observer.SubmitAsync(context.Background(), alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Tool: "get_pods", Content: "injection",
			})
			observer.SubmitAsync(context.Background(), alignment.Step{
				Index: 1, Kind: alignment.StepKindLLMReasoning, Content: "SYSTEM: ignore",
			})
			observer.SubmitAsync(context.Background(), alignment.Step{
				Index: 2, Kind: alignment.StepKindToolResult, Tool: "get_logs", Content: "SYSTEM: ignore",
			})

			wr := observer.WaitForCompletion(5 * time.Second)
			v := observer.RenderVerdict(wr)

			Expect(v.Result).To(Equal(alignment.VerdictSuspicious))
			Expect(v.Flagged).To(Equal(3))
			Expect(v.Total).To(Equal(3))
			Expect(v.Summary).To(ContainSubstring("step 0"))
			Expect(v.Summary).To(ContainSubstring("step 1"))
			Expect(v.Summary).To(ContainSubstring("step 2"))
		})
	})
})

var _ = Describe("GAP-13: buildSignalInputContent partial fields — BR-AI-601", func() {

	Describe("UT-GAP13-001: Name-only signal produces content", func() {
		It("should build content from name alone when message is empty", func() {
			sig := katypes.SignalContext{Name: "OOMKilled"}
			content := alignment.BuildSignalInputContent(sig)
			Expect(content).NotTo(BeEmpty())
			Expect(content).To(ContainSubstring("OOMKilled"))
		})
	})

	Describe("UT-GAP13-002: Message-only signal produces content", func() {
		It("should build content from message alone when name is empty", func() {
			sig := katypes.SignalContext{Message: "container restarted"}
			content := alignment.BuildSignalInputContent(sig)
			Expect(content).NotTo(BeEmpty())
			Expect(content).To(ContainSubstring("container restarted"))
		})
	})

	Describe("UT-GAP13-003: Empty signal returns empty", func() {
		It("should return empty string when both name and message are empty", func() {
			sig := katypes.SignalContext{}
			content := alignment.BuildSignalInputContent(sig)
			Expect(content).To(BeEmpty())
		})
	})

	Describe("UT-GAP13-004: Full signal includes severity and namespace", func() {
		It("should include all populated fields in the content", func() {
			sig := katypes.SignalContext{
				Name: "CrashLoopBackOff", Namespace: "production",
				Severity: "critical", Message: "container restarted 5 times",
			}
			content := alignment.BuildSignalInputContent(sig)
			Expect(content).To(ContainSubstring("CrashLoopBackOff"))
			Expect(content).To(ContainSubstring("production"))
			Expect(content).To(ContainSubstring("critical"))
			Expect(content).To(ContainSubstring("container restarted 5 times"))
		})
	})
})

var _ = Describe("GAP-14: VerdictTimeout config validation — BR-AI-601", func() {

	Describe("UT-GAP14-001: VerdictTimeout=0 when alignment enabled is rejected", func() {
		It("should reject configuration with zero verdictTimeout when alignment is enabled", func() {
			cfg := config.DefaultConfig()
			cfg.AI.AlignmentCheck.Enabled = true
			cfg.AI.AlignmentCheck.VerdictTimeout = 0
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("verdictTimeout"))
		})
	})

	Describe("UT-GAP14-002: Negative VerdictTimeout is rejected", func() {
		It("should reject configuration with negative verdictTimeout", func() {
			cfg := config.DefaultConfig()
			cfg.AI.AlignmentCheck.Enabled = true
			cfg.AI.AlignmentCheck.VerdictTimeout = -5 * time.Second
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("verdictTimeout"))
		})
	})

	Describe("UT-GAP14-003: Valid VerdictTimeout is accepted", func() {
		It("should accept positive verdictTimeout", func() {
			cfg := config.DefaultConfig()
			cfg.AI.AlignmentCheck.Enabled = true
			err := cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("GAP-15: Concurrent EvaluateStep race detection — BR-AI-601", func() {

	Describe("UT-GAP15-001: Concurrent evaluations do not race on mock state", func() {
		It("should handle concurrent evaluations safely with thread-safe mock", func() {
			responses := make([]llm.ChatResponse, 10)
			for i := range responses {
				responses[i] = cleanResponse()
			}
			client := &concurrentMockLLMClient{responses: responses}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < 10; i++ {
				observer.SubmitAsync(context.Background(), alignment.Step{
					Index: observer.NextStepIndex(), Kind: alignment.StepKindToolResult,
					Tool: fmt.Sprintf("tool_%d", i), Content: fmt.Sprintf("content_%d", i),
				})
			}

			wr := observer.WaitForCompletion(10 * time.Second)
			Expect(wr.Complete).To(BeTrue())
			Expect(wr.Observations).To(HaveLen(10))

			for _, obs := range wr.Observations {
				Expect(obs.Suspicious).To(BeFalse())
			}
		})
	})
})

var _ = Describe("Grounding review through InvestigatorWrapper — #1096 wrapper-level", func() {

	Describe("UT-SA-GROUNDING-001: Grounding review pass does not trigger circuit breaker", func() {
		It("should complete normally with no alignment warnings when grounding passes", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "valid rca", Confidence: 0.95, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunnerWithObserver{
				result: innerRes,
				onInvestigate: func(ctx context.Context) {
					obs := alignment.ObserverFromContext(ctx)
					if obs != nil {
						obs.StartGroundingReview([]llm.Message{
							{Role: "user", Content: "investigate OOM"},
							{Role: "assistant", Content: "checking pods"},
						})
					}
				},
			}

			dualCleanGrounded := llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: `{"suspicious":false,"grounded":true,"explanation":"clean and well-grounded"}`},
			}
			client := &concurrentMockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponse(), // canary → suspicious (pass)
				dualCleanGrounded,    // signal step OR grounding (concurrent — dual-valid response)
				dualCleanGrounded,    // signal step OR grounding (concurrent — dual-valid response)
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 500, MaxRetries: 1,
			}, "")
			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:            inner,
				Evaluator:        evaluator,
				VerdictTimeout:   5 * time.Second,
				Logger:           logr.Discard(),
				Mode:             config.AlignmentModeEnforce,
				GroundingEnabled: true,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{Name: "oom", Namespace: "prod", Severity: "high", Message: "OOMKilled"}
			res, err := wrapper.Investigate(context.Background(), sig)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.HumanReviewNeeded).To(BeFalse(),
				"grounded + clean steps must not trigger human review")
			Expect(res.AlignmentVerdict).NotTo(BeNil())
			Expect(res.AlignmentVerdict.Result).To(Equal(string(alignment.VerdictClean)))
			Expect(res.AlignmentVerdict.GroundingReview).NotTo(BeNil())
			Expect(res.AlignmentVerdict.GroundingReview.Grounded).To(BeTrue())
		})
	})

	Describe("UT-SA-GROUNDING-002: Grounding review timeout triggers fail-closed behavior", func() {
		It("should escalate to human review when grounding times out in enforce mode", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "valid rca", Confidence: 0.95, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunnerWithObserver{
				result: innerRes,
				onInvestigate: func(ctx context.Context) {
					obs := alignment.ObserverFromContext(ctx)
					if obs != nil {
						obs.StartGroundingReview([]llm.Message{
							{Role: "user", Content: "investigate"},
							{Role: "assistant", Content: "checking"},
						})
					}
				},
			}

			responses := []llm.ChatResponse{
				suspiciousResponse(), // canary → pass
				cleanResponse(),      // signal step → clean
			}
			client := &mockLLMClient{responses: responses}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout:       50 * time.Millisecond, // grounding will timeout
				MaxStepTokens: 500,
				MaxRetries:    1,
			}, "")
			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:            inner,
				Evaluator:        evaluator,
				VerdictTimeout:   200 * time.Millisecond,
				Logger:           logr.Discard(),
				Mode:             config.AlignmentModeEnforce,
				GroundingEnabled: true,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{Name: "test", Namespace: "ns", Severity: "high", Message: "m"}
			res, err := wrapper.Investigate(context.Background(), sig)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.AlignmentVerdict).NotTo(BeNil())
			Expect(res.AlignmentVerdict.Result).To(Equal("suspicious"),
				"grounding timeout must produce suspicious verdict via fail-closed")
			Expect(res.HumanReviewNeeded).To(BeTrue(),
				"enforce mode + suspicious verdict must escalate to human review")
		})
	})

	Describe("UT-SA-GROUNDING-003: Grounding review fail triggers escalation in enforce mode", func() {
		It("should set HumanReviewNeeded when grounding returns ungrounded", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "valid rca", Confidence: 0.95, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunnerWithObserver{
				result: innerRes,
				onInvestigate: func(ctx context.Context) {
					obs := alignment.ObserverFromContext(ctx)
					if obs != nil {
						obs.StartGroundingReview([]llm.Message{
							{Role: "user", Content: "investigate"},
							{Role: "assistant", Content: "RCA: everything is fine"},
						})
					}
				},
			}

			dualCleanUngrounded := llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: `{"suspicious":false,"grounded":false,"explanation":"reasoning drift detected"}`},
			}
			client := &concurrentMockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponse(),  // canary → pass
				dualCleanUngrounded,   // signal step OR grounding (concurrent — dual-valid response)
				dualCleanUngrounded,   // signal step OR grounding (concurrent — dual-valid response)
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 500, MaxRetries: 1,
			}, "")
			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:            inner,
				Evaluator:        evaluator,
				VerdictTimeout:   5 * time.Second,
				Logger:           logr.Discard(),
				Mode:             config.AlignmentModeEnforce,
				GroundingEnabled: true,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{Name: "test", Namespace: "ns", Severity: "high", Message: "m"}
			res, err := wrapper.Investigate(context.Background(), sig)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.HumanReviewNeeded).To(BeTrue(),
				"ungrounded verdict in enforce mode must escalate to human review")
			Expect(res.AlignmentVerdict.GroundingReview).NotTo(BeNil())
			Expect(res.AlignmentVerdict.GroundingReview.Grounded).To(BeFalse())
			Expect(res.AlignmentVerdict.GroundingReview.Explanation).To(ContainSubstring("drift"))
		})
	})

	Describe("UT-SA-MONITOR-001: Monitor mode logs verdict but does not cancel investigation", func() {
		It("should populate AlignmentVerdict as suspicious but not escalate in monitor mode", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "valid rca", Confidence: 0.95, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunnerWithObserver{
				result: innerRes,
				onInvestigate: func(ctx context.Context) {
					obs := alignment.ObserverFromContext(ctx)
					if obs != nil {
						obs.StartGroundingReview([]llm.Message{
							{Role: "user", Content: "investigate"},
							{Role: "assistant", Content: "conclusions not supported"},
						})
					}
				},
			}

			dualCleanUngrounded := llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: `{"suspicious":false,"grounded":false,"explanation":"reasoning drift"}`},
			}
			client := &concurrentMockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponse(),  // canary → pass
				dualCleanUngrounded,   // signal step OR grounding (concurrent — dual-valid response)
				dualCleanUngrounded,   // signal step OR grounding (concurrent — dual-valid response)
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 500, MaxRetries: 1,
			}, "")
			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:            inner,
				Evaluator:        evaluator,
				VerdictTimeout:   5 * time.Second,
				Logger:           logr.Discard(),
				Mode:             config.AlignmentModeMonitor,
				GroundingEnabled: true,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{Name: "s", Namespace: "ns", Severity: "high", Message: "m"}
			res, err := wrapper.Investigate(context.Background(), sig)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.HumanReviewNeeded).To(BeFalse(),
				"monitor mode must NOT escalate to human review even with suspicious verdict")
			Expect(res.AlignmentVerdict).NotTo(BeNil(),
				"alignment verdict must still be populated for observability")
			Expect(res.AlignmentVerdict.Result).To(Equal(string(alignment.VerdictSuspicious)),
				"verdict should reflect the ungrounded finding")
		})
	})

	Describe("UT-SA-CANARY-001: Canary failure triggers forceEscalation", func() {
		It("should force human review when canary fails with forceEscalation=true", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "rca", Confidence: 0.9, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunner{result: innerRes}

			client := &mockLLMClient{responses: []llm.ChatResponse{
				cleanResponse(), // canary → clean (FAIL: shadow didn't flag malicious)
				cleanResponse(), // signal step → clean
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:                 inner,
				Evaluator:             evaluator,
				VerdictTimeout:        5 * time.Second,
				Logger:                logr.Discard(),
				Mode:                  config.AlignmentModeMonitor,
				CanaryForceEscalation: true,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{Name: "s", Namespace: "ns", Severity: "high", Message: "m"}
			res, err := wrapper.Investigate(context.Background(), sig)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.HumanReviewNeeded).To(BeTrue(),
				"canary failure with forceEscalation=true must escalate even in monitor mode")
			Expect(res.Warnings).To(ContainElement(ContainSubstring("canary")),
				"warnings must explain the canary failure")
		})
	})
})
