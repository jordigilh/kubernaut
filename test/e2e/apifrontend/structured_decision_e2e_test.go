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

			if frame.Result.Kind == "status-update" {
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

	It("E2E-AF-1395-001: SI-10 — structured decision payload > 512 chars arrives intact via SSE", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		resp, err := a2aSSEPost(readCtx, a2aMessageStream("e2e-structured-001", "present structured rca decision"))
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

		resp, err := a2aSSEPost(readCtx, a2aMessageStream("e2e-structured-002", "present structured rca decision"))
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
		Expect(payload.RCA.Target).To(Equal("Deployment/data-processor in production"))
		Expect(payload.RCA.ToolCallsCount).To(Equal(19))
		Expect(payload.RCA.LLMTurns).To(Equal(17))

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

		resp, err := a2aSSEPost(readCtx, a2aMessageStream("e2e-structured-003", "present structured rca decision"))
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
})
