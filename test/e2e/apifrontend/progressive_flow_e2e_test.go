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
// E2E-AF-1407: Progressive RCA Emission — Pyramid Invariant E2E tier
//
// Proves the full user journey: user prompt → mock-LLM calls kubernaut_investigate
// → investigation completes with RCA → early_rca decision event emitted via SSE →
// mock-LLM auto-proceeds to kubernaut_discover_workflows (no user intervention).
//
// FedRAMP: SI-4 (audit classification of early RCA), AU-3 (traceability of
// progressive events through the streaming pipeline).
//
// Mock-LLM scenario: af_progressive_investigate
// Keyword trigger: "progressive investigate"
// Chain: kubernaut_investigate → next_tool_call → kubernaut_discover_workflows
// =============================================================================

var _ = Describe("Progressive RCA Flow E2E — #1407", Ordered, Label("e2e", "progressive-rca", "1407"), func() {
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

	// scanProgressiveEvents reads the SSE stream and collects:
	// - earlyRCA: status-update events with metadata.schema="early_rca"
	// - allStatuses: all status-update events for lifecycle analysis
	// - allArtifacts: all artifact-update events
	type progressiveResult struct {
		earlyRCAEvents []map[string]any
		allStatuses    []map[string]any
		allArtifacts   []map[string]any
		reachedEnd     bool
	}

	scanProgressiveSSE := func(resp *http.Response) progressiveResult {
		var result progressiveResult
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)

		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if !strings.HasPrefix(strings.TrimSpace(line), "data:") {
				continue
			}
			data := strings.TrimPrefix(strings.TrimSpace(line), "data:")
			data = strings.TrimSpace(data)
			if data == "" || !strings.HasPrefix(data, "{") {
				continue
			}

			var envelope struct {
				Result json.RawMessage `json:"result"`
			}
			if json.Unmarshal([]byte(data), &envelope) != nil || len(envelope.Result) == 0 {
				continue
			}

			var raw map[string]any
			if json.Unmarshal(envelope.Result, &raw) != nil {
				continue
			}

			kind, _ := raw["kind"].(string)
			switch kind {
			case "status-update":
				result.allStatuses = append(result.allStatuses, raw)
				meta, _ := raw["metadata"].(map[string]any)
				if meta != nil && meta["schema"] == "early_rca" {
					result.earlyRCAEvents = append(result.earlyRCAEvents, raw)
				}
				status, _ := raw["status"].(map[string]any)
				if status != nil {
					state, _ := status["state"].(string)
					if state == "completed" || state == "failed" {
						result.reachedEnd = true
					}
				}
			case "artifact-update":
				result.allArtifacts = append(result.allArtifacts, raw)
			}
		}
		return result
	}

	It("E2E-AF-1407-001: SI-4 — early RCA decision event emitted during progressive investigate flow", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		resp, err := a2aSSEPost(readCtx, a2aMessageStream("e2e-progressive-1407-001", "progressive investigate"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

		result := scanProgressiveSSE(resp)

		GinkgoWriter.Printf("Progressive flow: %d early_rca events, %d total statuses, %d artifacts\n",
			len(result.earlyRCAEvents), len(result.allStatuses), len(result.allArtifacts))

		By("SI-4: early_rca decision event must be emitted")
		Expect(result.earlyRCAEvents).NotTo(BeEmpty(),
			"SI-4: progressive flow must emit at least one early_rca decision event")

		By("SI-4: early_rca event carries correct metadata for audit classification")
		earlyRCA := result.earlyRCAEvents[0]
		meta, ok := earlyRCA["metadata"].(map[string]any)
		Expect(ok).To(BeTrue(), "early_rca event must have metadata")
		Expect(meta["type"]).To(Equal("decision"), "metadata.type must be 'decision'")
		Expect(meta["schema"]).To(Equal("early_rca"), "metadata.schema must be 'early_rca'")
		Expect(meta["schema_version"]).To(Equal("1.0"), "metadata.schema_version must be '1.0'")
	})

	It("E2E-AF-1407-002: AU-3 — progressive flow reaches terminal state without user intervention", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		resp, err := a2aSSEPost(readCtx, a2aMessageStream("e2e-progressive-1407-002", "progressive investigate"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		result := scanProgressiveSSE(resp)

		By("AU-3: stream must reach terminal state (completed/failed) in a single SSE connection")
		Expect(result.reachedEnd).To(BeTrue(),
			"AU-3: progressive flow must reach terminal state without user intervention")

		By("AU-3: total event count confirms multi-phase execution (investigate + discover)")
		Expect(len(result.allStatuses) + len(result.allArtifacts)).To(BeNumerically(">=", 2),
			"AU-3: progressive flow must produce events from both investigation and discovery phases")
	})

	It("E2E-AF-1407-003: AU-3 — early_rca payload contains severity and confidence for traceability", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		resp, err := a2aSSEPost(readCtx, a2aMessageStream("e2e-progressive-1407-003", "progressive investigate"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		result := scanProgressiveSSE(resp)
		Expect(result.earlyRCAEvents).NotTo(BeEmpty(), "need early_rca event for payload analysis")

		earlyRCA := result.earlyRCAEvents[0]
		status, _ := earlyRCA["status"].(map[string]any)
		Expect(status).NotTo(BeNil(), "early_rca event must have status field")

		msg, _ := status["message"].(map[string]any)
		Expect(msg).NotTo(BeNil(), "status must have message field")

		parts, _ := msg["parts"].([]any)
		Expect(parts).NotTo(BeEmpty(), "message must have parts")

		firstPart, _ := parts[0].(map[string]any)
		text, _ := firstPart["text"].(string)
		Expect(text).NotTo(BeEmpty(), "AU-3: early_rca must carry structured text payload")

		By("AU-3: payload must contain severity and confidence for audit traceability")
		var payload map[string]any
		err = json.Unmarshal([]byte(text), &payload)
		Expect(err).NotTo(HaveOccurred(), "early_rca payload must be valid JSON")
		Expect(payload).To(HaveKey("severity"), "AU-3: payload must include severity")
		Expect(payload).To(HaveKey("confidence"), "AU-3: payload must include confidence")
	})
})

// =============================================================================
// E2E-AF-1408: Structured investigation_summary Artifact Contract
//
// Proves the full contract: mock-LLM calls kubernaut_present_decision →
// AF emits TaskArtifactUpdateEvent with DataPart containing type=investigation_summary,
// schema_version=1.0 → SSE stream delivers compliant artifact to Console.
//
// FedRAMP: SI-4 (structured audit classification), AU-3 (schema traceability),
// SI-10 (data integrity through schema validation).
//
// Mock-LLM scenario: af_progressive_investigate (reuses #1407 scenario since
// present_decision is called after discovery completes).
// =============================================================================

var _ = Describe("Structured Artifact Contract E2E — #1408", Ordered, Label("e2e", "structured-artifact", "1408"), func() {
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

	It("E2E-AF-1408-001: SI-10 — artifact DataPart contains type=investigation_summary and schema_version=1.0", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		resp, err := a2aSSEPost(readCtx, a2aMessageStream("e2e-artifact-1408-001", "progressive investigate"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

		var artifactEvents []map[string]any
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)

		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if !strings.HasPrefix(strings.TrimSpace(line), "data:") {
				continue
			}
			data := strings.TrimPrefix(strings.TrimSpace(line), "data:")
			data = strings.TrimSpace(data)
			if data == "" || !strings.HasPrefix(data, "{") {
				continue
			}

			var envelope struct {
				Result json.RawMessage `json:"result"`
			}
			if json.Unmarshal([]byte(data), &envelope) != nil || len(envelope.Result) == 0 {
				continue
			}

			var raw map[string]any
			if json.Unmarshal(envelope.Result, &raw) != nil {
				continue
			}

			if kind, _ := raw["kind"].(string); kind == "artifact-update" {
				artifactEvents = append(artifactEvents, raw)
			}
		}

		GinkgoWriter.Printf("Structured artifact contract: %d artifact events collected\n", len(artifactEvents))
		Expect(artifactEvents).NotTo(BeEmpty(),
			"SI-10: progressive flow must emit at least one artifact-update event")

		By("SI-10: artifact must contain DataPart with schema self-identification fields")
		found := false
		for _, evt := range artifactEvents {
			artifact, _ := evt["artifact"].(map[string]any)
			if artifact == nil {
				continue
			}
			parts, _ := artifact["parts"].([]any)
			for _, p := range parts {
				part, _ := p.(map[string]any)
				if part == nil {
					continue
				}
				dpData, _ := part["data"].(map[string]any)
				if dpData == nil {
					continue
				}
				if dpData["type"] == "investigation_summary" && dpData["schema_version"] == "1.0" {
					found = true
					Expect(dpData).To(HaveKey("summary"),
						"SI-10: investigation_summary must include summary field")
					break
				}
			}
			if found {
				break
			}
		}
		Expect(found).To(BeTrue(),
			"SI-10: at least one artifact must contain DataPart with type=investigation_summary and schema_version=1.0")
	})
})
