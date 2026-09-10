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

package e2e_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// =============================================================================
// E2E-AF-1395-1396: Structured Decision Payload — Pyramid Invariant E2E tier
//
// These tests prove the full user journey: user prompt → mock-LLM → AF tool
// dispatch → A2A SSE emission with structured RCA + extended workflow options.
// They validate that:
//   1. JSON payloads > 512 chars are NOT truncated (#1395)
//   2. RCA fields flow through the decision event (#1396)
//   3. Extended WorkflowOption fields (Parameters, RuledOutReason) survive (#1396)
//
// Mock-LLM scenario: af_structured_decision
// Keyword trigger: "structured decision" or "present structured rca decision"
// =============================================================================

var _ = Describe("Structured Decision Payload E2E — #1395 #1396", Ordered, Label("e2e", "structured-decision"), func() {
	var sreToken string

	BeforeEach(func() {
		var err error
		sreToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token required")
		Expect(sreToken).NotTo(BeEmpty())
	})

	a2aSSEPost := func(ctx context.Context, body string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/a2a/invoke", strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+sreToken)
		return httpClient.Do(req)
	}

	// scanDecisionEvent reads SSE frames until it finds a decision event and
	// returns the structured JSON payload text and metadata.
	// Checks artifact-update events first (EmitArtifact path, A2A v1.0),
	// then falls back to status-update events (EmitStructuredMeta path).
	scanDecisionEvent := func(resp *http.Response) (string, map[string]any) {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)

		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if !strings.HasPrefix(strings.TrimSpace(line), "data:") {
				continue
			}
			data := strings.TrimPrefix(strings.TrimSpace(line), "data:")
			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}

			var frame struct {
				Result struct {
					Kind   string `json:"kind"`
					Status struct {
						Message struct {
							Parts []struct {
								Text string `json:"text"`
							} `json:"parts"`
						} `json:"message"`
					} `json:"status"`
					Artifact struct {
						Parts []struct {
							Data json.RawMessage `json:"data,omitempty"`
							Text string          `json:"text,omitempty"`
						} `json:"parts"`
						Metadata map[string]any `json:"metadata"`
					} `json:"artifact"`
					Metadata map[string]any `json:"metadata"`
				} `json:"result"`
			}
			if json.Unmarshal([]byte(data), &frame) != nil {
				continue
			}

			if frame.Result.Kind == "artifact-update" {
				if frame.Result.Artifact.Metadata == nil || frame.Result.Artifact.Metadata["type"] != "decision" {
					continue
				}
				for _, p := range frame.Result.Artifact.Parts {
					if len(p.Data) > 0 {
						return string(p.Data), frame.Result.Artifact.Metadata
					}
				}
				continue
			}

			if frame.Result.Kind == statusUpdate {
				if frame.Result.Metadata == nil || frame.Result.Metadata["type"] != "decision" {
					continue
				}
				if len(frame.Result.Status.Message.Parts) == 0 {
					continue
				}
				text := frame.Result.Status.Message.Parts[0].Text
				if text == "" || strings.Contains(text, "Presenting decision") {
					continue
				}
				return text, frame.Result.Metadata
			}
		}
		return "", nil
	}

	// groundSession establishes #2023's grounding-guard prerequisite: a
	// successful kubernaut_investigate turn in the same A2A session
	// (contextID), so the later present_decision turn's before-callback
	// (enforceGroundingGuard, phase_guard.go) finds
	// session.StateKeyGroundedContentAvailable=true and lets the mock-LLM's
	// scripted RCA/options through instead of overwriting them with the
	// fail-closed "no investigation content" fallback. Targets a dedicated
	// af-structured-decision-e2e/structured-decision-target fixture (managed
	// namespace, StructuredDecisionGrounding synthetic alert,
	// apifrontend_prometheus_e2e.go) rather than the shared
	// af-investigate-e2e/af-investigate-target fixture used by
	// af_investigate/af_progressive_investigate/af_investigate_resume: with
	// Ginkgo --procs>1 those scenarios' concurrently-running specs can hold
	// that fixture's KA session open when this call lands, and
	// af_create_rr.go's checkExistingRRByFingerprint dedups by target, so
	// this call would nondeterministically inherit their in-flight session
	// and hit KA's session_active/early_rca fallback instead of a clean
	// grounded result -- silently starving present_decision of grounding
	// and emitting no SSE decision event at all (CI run 31320575553,
	// this test, "not to be empty").
	groundSession := func(ctx context.Context, contextID string) {
		resp, err := a2aSSEPost(ctx, a2aMessageStreamWithContext(contextID+"-ground", contextID,
			"seed grounding context for pod structured-decision-target in af-structured-decision-e2e"))
		Expect(err).NotTo(HaveOccurred(), "grounding kubernaut_investigate call must succeed")
		defer func() { _ = resp.Body.Close() }()
		_, _ = scanDecisionEvent(resp) // drain to EOF; no decision event expected here
	}

	// groundSessionAlt is groundSession's twin for E2E-AF-1396-002 alone,
	// targeting a SECOND dedicated fixture (structured-decision-target-2,
	// StructuredDecisionGrounding2 in apifrontend_prometheus_e2e.go / the
	// "seed alt fixture context 1396 002" mock-LLM keyword). This Describe
	// block is Ordered and its 3 Its run sequentially; #1395-001 and
	// #1396-001 above both call groundSession against the SAME
	// structured-decision-target fixture, and af_create_rr.go's
	// checkExistingRRByFingerprint dedups by target, so by this 3rd call
	// the RR already has a KA interactive session opened by one of the
	// prior two Its. That nondeterministically surfaces as "session_active"
	// instead of a clean grounded result, which per
	// investigateHasGroundedContent (phase_guard.go, UT-AF-2023-004)
	// deterministically fails present_decision's grounding guard closed --
	// clearing options to [] (CI run 31335085795, "AC-6: ALL options must
	// be presented"). A fixture this test alone uses eliminates the
	// contention outright rather than relying on lease timing.
	groundSessionAlt := func(ctx context.Context, contextID string) {
		resp, err := a2aSSEPost(ctx, a2aMessageStreamWithContext(contextID+"-ground", contextID,
			"seed alt fixture context 1396 002 for pod structured-decision-target-2 in af-structured-decision-e2e"))
		Expect(err).NotTo(HaveOccurred(), "grounding kubernaut_investigate call must succeed")
		defer func() { _ = resp.Body.Close() }()
		_, _ = scanDecisionEvent(resp) // drain to EOF; no decision event expected here
	}

	// groundSessionBeta is groundSession's/groundSessionAlt's third sibling,
	// dedicated to E2E-AF-1396-001 alone (structured-decision-target-3,
	// StructuredDecisionGrounding3 in apifrontend_prometheus_e2e.go / the
	// "seed third fixture context 1396 001" mock-LLM keyword).
	// groundSessionAlt above only closed contention for the 3rd It
	// (1396-002) relative to the first two -- it did not address that the
	// 2nd It (1396-001) still shares groundSession's ORIGINAL target with
	// the 1st It (1395-001), so 1396-001's own grounding call could just as
	// nondeterministically inherit 1395-001's still-open KA session and
	// observe session_active instead of a clean grounded result, which per
	// investigateHasGroundedContent (phase_guard.go) deterministically
	// fails present_decision's grounding guard closed -- surfacing as an
	// empty payload.RCA.Severity instead of the scripted "critical" (CI run
	// 31351842574, "severity must flow from mock-LLM through AF to SSE").
	// A third dedicated target removes the last remaining shared fixture in
	// this Ordered block.
	groundSessionBeta := func(ctx context.Context, contextID string) {
		resp, err := a2aSSEPost(ctx, a2aMessageStreamWithContext(contextID+"-ground", contextID,
			"seed third fixture context 1396 001 for pod structured-decision-target-3 in af-structured-decision-e2e"))
		Expect(err).NotTo(HaveOccurred(), "grounding kubernaut_investigate call must succeed")
		defer func() { _ = resp.Body.Close() }()
		_, _ = scanDecisionEvent(resp) // drain to EOF; no decision event expected here
	}

	// groundSessionDelta is the fourth sibling, dedicated to E2E-AF-2387-002
	// alone (structured-decision-target-4, StructuredDecisionGrounding4 /
	// the "seed fourth fixture context 2387 002" mock-LLM keyword). Same
	// intra-suite session_active contention rationale as the three siblings:
	// each It in this Ordered block grounds against its own RR so no It can
	// inherit another's still-open KA session.
	groundSessionDelta := func(ctx context.Context, contextID string) {
		resp, err := a2aSSEPost(ctx, a2aMessageStreamWithContext(contextID+"-ground", contextID,
			"seed fourth fixture context 2387 002 for pod structured-decision-target-4 in af-structured-decision-e2e"))
		Expect(err).NotTo(HaveOccurred(), "grounding kubernaut_investigate call must succeed")
		defer func() { _ = resp.Body.Close() }()
		_, _ = scanDecisionEvent(resp) // drain to EOF; no decision event expected here
	}

	It("E2E-AF-1395-001: SI-10 — structured decision payload > 512 chars arrives intact via SSE", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		const ctxID = "ctx-e2e-structured-001"
		groundSession(readCtx, ctxID)

		resp, err := a2aSSEPost(readCtx, a2aMessageStreamWithContext("e2e-structured-001-decision", ctxID, "present structured rca decision"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

		text, meta := scanDecisionEvent(resp)
		Expect(text).NotTo(BeEmpty(), "should receive decision event with JSON payload")
		Expect(meta["type"]).To(Equal("decision"))

		Expect(len(text)).To(BeNumerically(">", 512),
			"#1395: structured JSON payload must NOT be truncated at 512 chars")
		Expect(text).NotTo(HaveSuffix("..."),
			"#1395: structured JSON must not have truncation ellipsis")

		var payload map[string]any
		err = json.Unmarshal([]byte(text), &payload)
		Expect(err).NotTo(HaveOccurred(),
			"SI-10: decision payload must be valid JSON after transmission")
	})

	It("E2E-AF-1396-001: AU-3 — RCA fields flow end-to-end through decision event", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		const ctxID = "ctx-e2e-structured-002"
		groundSessionBeta(readCtx, ctxID)

		resp, err := a2aSSEPost(readCtx, a2aMessageStreamWithContext("e2e-structured-002-decision", ctxID, "present structured rca decision"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		text, _ := scanDecisionEvent(resp)
		Expect(text).NotTo(BeEmpty())

		var payload struct {
			SessionID string `json:"session_id"`
			Summary   string `json:"summary"`
			RCA       struct {
				Severity       string   `json:"severity"`
				Confidence     float64  `json:"confidence"`
				CausalChain    []string `json:"causal_chain"`
				Target         string   `json:"target"`
				ToolCallsCount int      `json:"tool_calls_count"`
				LLMTurns       int      `json:"llm_turns"`
			} `json:"rca"`
			Options []struct {
				WorkflowID     string            `json:"workflow_id"`
				Name           string            `json:"name"`
				Description    string            `json:"description"`
				Risk           string            `json:"risk"`
				Recommended    bool              `json:"recommended"`
				Parameters     map[string]string `json:"parameters"`
				RuledOutReason string            `json:"ruled_out_reason"`
			} `json:"options"`
		}
		err = json.Unmarshal([]byte(text), &payload)
		Expect(err).NotTo(HaveOccurred(), "AU-3: payload must parse for audit trail")

		By("Verifying RCA fields")
		Expect(payload.RCA.Severity).To(Equal("critical"),
			"AU-3: severity must flow from mock-LLM through AF to SSE")
		Expect(payload.RCA.Confidence).To(BeNumerically("~", 0.92, 0.01))
		Expect(payload.RCA.CausalChain).To(HaveLen(3))
		Expect(payload.RCA.Target).To(Equal("Deployment/data-processor in production")) // BR-KA-OBSERVABILITY-001: substituted target survives to the artifact
		// #2073/#2387: tool_calls_count/llm_turns are mock-LLM-scripted
		// values in af_structured_decision's present_decision tool_call
		// (mock-llm.yaml) -- exactly the LLM-invented bookkeeping
		// enforceGroundingGuard's substitution now overrides with the
		// grounding investigation's server-computed totals (phase_guard.go
		// canonicalGroundedRCA). The grounding call runs a REAL synchronous
		// Investigate (#1818 Gap 3), so GroundedRCA is genuinely populated.
		// Lane-proven on helios08 (E2E-AF-2387-002 triage): the AF agent
		// issues kubernaut_investigate TWICE ~25s apart for one RR and the
		// second entry resets the per-RR scope mid-flight, so the artifact
		// reads 1 turn / 0 tools on the reset landing vs 3 turns / 1 tool
		// with no reset (duplicate-Investigate reset race -- product
		// follow-up filed; the audit trail confirms identical executed work
		// and token sums, 1100/150, on both landings). Either landing proves
		// a live investigation; pre-#2387 both fields read hard 0 with zero
		// tokens. severity/confidence/causal_chain/target above remain
		// LLM-authored pass-through and are unaffected.
		Expect(payload.RCA.ToolCallsCount).To(BeElementOf(0, 1))
		Expect(payload.RCA.LLMTurns).To(BeElementOf(1, 2, 3))

		By("Verifying extended workflow options")
		Expect(payload.Options).To(HaveLen(3))
		Expect(payload.Options[0].Recommended).To(BeTrue())
		Expect(payload.Options[0].Parameters).To(HaveKeyWithValue("namespace", "production"))
		Expect(payload.Options[0].Parameters).To(HaveKeyWithValue("deployment", "data-processor"))
		Expect(payload.Options[2].RuledOutReason).To(ContainSubstring("No previous revision"))
	})

	It("E2E-AF-1396-002: AC-6 — all 3 workflow options present for human review", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		const ctxID = "ctx-e2e-structured-003"
		groundSessionAlt(readCtx, ctxID)

		resp, err := a2aSSEPost(readCtx, a2aMessageStreamWithContext("e2e-structured-003-decision", ctxID, "present structured rca decision"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		text, _ := scanDecisionEvent(resp)
		Expect(text).NotTo(BeEmpty())

		var payload struct {
			Options []struct {
				WorkflowID string `json:"workflow_id"`
				Name       string `json:"name"`
			} `json:"options"`
		}
		err = json.Unmarshal([]byte(text), &payload)
		Expect(err).NotTo(HaveOccurred())

		Expect(payload.Options).To(HaveLen(3),
			"AC-6: ALL options must be presented for human review — no hidden automated choices")
		workflowIDs := make([]string, len(payload.Options))
		for i, opt := range payload.Options {
			workflowIDs[i] = opt.WorkflowID
		}
		Expect(workflowIDs).To(ConsistOf("wf-restart-pod", "wf-increase-memory", "wf-rollback"))
	})

	It("E2E-AF-2387-002: AU-3 — grounded artifact serves the investigation's real call-level counts and token sums", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		const ctxID = "ctx-e2e-structured-004"
		groundSessionDelta(readCtx, ctxID)

		resp, err := a2aSSEPost(readCtx, a2aMessageStreamWithContext("e2e-structured-004-decision", ctxID, "present structured rca decision"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		text, _ := scanDecisionEvent(resp)
		Expect(text).NotTo(BeEmpty())

		var payload struct {
			RCA struct {
				Severity         string   `json:"severity"`
				Confidence       float64  `json:"confidence"`
				CausalChain      []string `json:"causal_chain"`
				Target           string   `json:"target"`
				ToolCallsCount   int      `json:"tool_calls_count"`
				LLMTurns         int      `json:"llm_turns"`
				PromptTokens     int      `json:"prompt_tokens"`
				CompletionTokens int      `json:"completion_tokens"`
				TotalTokens      int      `json:"total_tokens"`
			} `json:"rca"`
		}
		err = json.Unmarshal([]byte(text), &payload)
		Expect(err).NotTo(HaveOccurred(), "AU-3: payload must parse for audit trail")

		By("Verifying substituted RCA content (same script as 1396-001's fixture)")
		Expect(payload.RCA.Severity).To(Equal("critical")) // BR-KA-OBSERVABILITY-001: substitution serves the scripted fixture RCA
		Expect(payload.RCA.Confidence).To(BeNumerically("~", 0.92, 0.01))
		Expect(payload.RCA.CausalChain).To(HaveLen(3))
		Expect(payload.RCA.Target).To(Equal("Deployment/data-processor in production")) // BR-KA-OBSERVABILITY-001: substituted target survives to the artifact

		By("Verifying server-computed counts are real, not honest zeros")
		// #2387: pre-fix TotalToolCalls/TotalLLMTurns were hard zero, so a
		// grounded artifact carried 0/0 exactly like the #2073 fallback.
		// The ground_4 investigation really runs (synchronous Investigate
		// per #1818 Gap 3), and every leg records at least one LLM turn, so
		// llm_turns must be non-zero here.
		//
		// tool_calls_count is 0 or 1, both honest (lane-proven on helios08):
		// KA's first turns match the AF-side af_investigate keyword scenario
		// (keyword shadowing, not the ground_4 signal scenario) and the
		// resulting kubernaut_investigate call is dispatched by KA, errors
		// with "tool not found", and counts as one dispatched call.
		// Whether it survives into the artifact depends on a duplicate-
		// Investigate race: the AF agent issues kubernaut_investigate TWICE
		// ~25s apart for one RR, and the second entry resets the per-RR
		// scope (resetMetrics at Investigate entry) while the first flight
		// is still accumulating. Reset landing first wipes the errored
		// dispatch (0); no-reset landing keeps it (1). Both values prove a
		// live investigation (a #2387 regression reads hard 0/0 with zero
		// tokens, which the equalities below reject). Product follow-up:
		// re-entry reset vs in-flight accumulation needs a design decision
		// (idempotent start or generation-scoped accounting).
		GinkgoWriter.Printf("  grounded counts: llm_turns=%d tool_calls_count=%d prompt=%d completion=%d total=%d\n",
			payload.RCA.LLMTurns, payload.RCA.ToolCallsCount,
			payload.RCA.PromptTokens, payload.RCA.CompletionTokens, payload.RCA.TotalTokens)
		Expect(payload.RCA.LLMTurns).To(BeNumerically(">=", 1),
			"E2E-AF-2387-002: grounded artifact must serve the investigation's real turn count, not 0")
		Expect(payload.RCA.ToolCallsCount).To(BeElementOf(0, 1),
			"E2E-AF-2387-002: exactly one real dispatch ever occurs (the errored cross-talk call); reset landing decides 0 vs 1")

		By("Verifying token sums tie exactly to the reported turn count")
		// Per-turn usage is mock-scripted and builder-deterministic:
		// tool-call responses report (500, 50, 550), text responses
		// (100, 50, 150) — completion is 50 either way, so N (the reported
		// turn count) must satisfy completion == 50*N exactly, and the
		// prompt excess over 100*N must come in units of 400 (500-100),
		// i.e. whole tool-response turns. This ties the two independent
		// counters (turns vs tokens) together: a fix that counted turns
		// but still dropped streamed usage (or vice versa) cannot satisfy
		// all three equalities at once.
		n := payload.RCA.LLMTurns
		Expect(payload.RCA.CompletionTokens).To(Equal(50*n),
			"E2E-AF-2387-002: completion tokens must equal 50 per reported turn exactly")
		promptExcess := payload.RCA.PromptTokens - 100*n
		Expect(promptExcess).To(BeNumerically(">=", 0),
			"E2E-AF-2387-002: prompt tokens must cover at least 100 per reported turn")
		Expect(promptExcess%400).To(Equal(0),
			"E2E-AF-2387-002: prompt excess over 100/turn must decompose into whole 500-prompt tool-response turns")
		Expect(payload.RCA.TotalTokens).To(Equal(payload.RCA.PromptTokens+payload.RCA.CompletionTokens),
			"E2E-AF-2387-002: total tokens must equal prompt + completion exactly")
	})
})
