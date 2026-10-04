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
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	statusUpdate               = "status-update"
	completed                  = "completed"
	artifactUpdate             = "artifact-update"
	failed                     = "failed"
	investigationSummarySchema = "investigation_summary"
)

// =============================================================================
// E2E-AF-1407: Progressive RCA Emission — Pyramid Invariant E2E tier
//
// Proves the user journey: user prompt → mock-LLM calls kubernaut_investigate
// → AF creates RR with severity triage → bridge waits for KA events →
// severity-only status emitted when no RCA findings are available.
//
// The AF E2E cluster has no AA controller. The deployed AF uses short
// awaitSessionTimeout/bridgeInactivityTimeout values (set in the E2E
// config overlay) so the status-only outcome is delivered promptly.
//
// FedRAMP: SI-4 (audit classification of progressive status), AU-3
// (traceability of grounded status through the streaming pipeline).
//
// Mock-LLM scenario: af_progressive_investigate
// Keyword trigger: "progressive investigate"
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
	// - earlyRCA: genuine RCA status-update events with metadata.schema="early_rca"
	// - severityOnly: status-only outcomes with no RCA findings
	// - allStatuses: all status-update events for lifecycle analysis
	// - allArtifacts: all artifact-update events
	type progressiveResult struct {
		earlyRCAEvents     []map[string]any
		severityOnlyEvents []map[string]any
		allStatuses        []map[string]any
		allArtifacts       []map[string]any
		reachedEnd         bool
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
			case statusUpdate:
				result.allStatuses = append(result.allStatuses, raw)
				meta, _ := raw["metadata"].(map[string]any)
				if meta != nil && meta["schema"] == "early_rca" {
					result.earlyRCAEvents = append(result.earlyRCAEvents, raw)
				}
				status, _ := raw["status"].(map[string]any)
				message, _ := status["message"].(map[string]any)
				parts, _ := message["parts"].([]any)
				for _, rawPart := range parts {
					part, _ := rawPart.(map[string]any)
					text, _ := part["text"].(string)
					if strings.Contains(text, "No root-cause findings are available yet") {
						result.severityOnlyEvents = append(result.severityOnlyEvents, raw)
						break
					}
				}
				if status != nil {
					state, _ := status["state"].(string)
					if state == completed || state == failed {
						result.reachedEnd = true
					}
				}
			case artifactUpdate:
				result.allArtifacts = append(result.allArtifacts, raw)
			}
		}
		return result
	}

	It("E2E-AF-1407-001: SI-4 — severity-only status emitted when progressive investigation has no RCA", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		resp, err := a2aSSEPost(readCtx, a2aMessageStream("e2e-progressive-1407-001", "progressive investigate"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

		result := scanProgressiveSSE(resp)

		GinkgoWriter.Printf("Progressive flow: %d early_rca events, %d severity-only statuses, %d total statuses, %d artifacts\n",
			len(result.earlyRCAEvents), len(result.severityOnlyEvents), len(result.allStatuses), len(result.allArtifacts))

		By("SI-4: status-only outcome must be emitted")
		Expect(result.severityOnlyEvents).NotTo(BeEmpty(),
			"SI-4: progressive flow with no RCA findings must emit severity-only status guidance")

		By("SI-4: status-only event carries the standard status classification")
		statusEvent := result.severityOnlyEvents[0]
		meta, ok := statusEvent["metadata"].(map[string]any)
		Expect(ok).To(BeTrue(), "status-only event must have metadata")
		Expect(meta["type"]).To(Equal("status"), "metadata.type must be 'status'")
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
		Expect(len(result.allStatuses)+len(result.allArtifacts)).To(BeNumerically(">=", 2),
			"AU-3: progressive flow must produce events from both investigation and discovery phases")
	})

	It("E2E-AF-1407-003: AU-3 — severity-only status contains grounded severity without confidence", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		resp, err := a2aSSEPost(readCtx, a2aMessageStream("e2e-progressive-1407-003", "progressive investigate"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		result := scanProgressiveSSE(resp)
		Expect(result.severityOnlyEvents).NotTo(BeEmpty(), "need severity-only status for payload analysis")

		statusEvent := result.severityOnlyEvents[0]
		status, _ := statusEvent["status"].(map[string]any)
		Expect(status).NotTo(BeNil(), "status-only event must have status field")
		msg, _ := status["message"].(map[string]any)
		Expect(msg).NotTo(BeNil(), "status must have message field")
		parts, _ := msg["parts"].([]any)
		Expect(parts).NotTo(BeEmpty(), "message must have parts")
		firstPart, _ := parts[0].(map[string]any)
		text, _ := firstPart["text"].(string)
		Expect(text).To(ContainSubstring("Preliminary severity from resource metadata:"),
			"AU-3: status must carry grounded severity information")
		Expect(strings.ToLower(text)).NotTo(ContainSubstring("confidence"),
			"AU-3: status-only outcome must not claim a confidence value")
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

	// #2245 CI RCA: this flow's investigation_summary artifact only fires
	// once kubernaut_present_decision runs, which AF's phase-guard blocks
	// with reason=discover_workflows_not_yet_succeeded until KA's RCA
	// resolution completes. Under normal load that's ~1-13s (confirmed from
	// a passing run's own Ginkgo timings); under concurrent E2E/CI
	// contention it was observed taking ~2.5min (KA-side phase-guard log:
	// discover_workflows finally succeeded ~150s after session start) --
	// past AF's short interactive.awaitSessionTimeout/bridgeInactivityTimeout
	// (10s/15s, deliberately tight so E2E-AF-1407's status-only path fires
	// promptly when there's no AA controller in this cluster), so the SSE
	// stream can end before a genuine investigation_summary artifact arrives.
	// Loosening that
	// a rare contention spike, so instead this test retries the whole
	// request with a fresh contextId (a brand-new RR/session, not a replay
	// of the stalled one) -- consistent with #1911's precedent that
	// discover_workflows contention under concurrent E2E load is handled
	// with resilience/retries, not by lengthening every deployment's
	// timeouts.
	It("E2E-AF-1408-001: SI-10 — artifact DataPart contains type=investigation_summary and schema_version=1.0", func() {
		const maxAttempts = 2
		var found bool
		var lastArtifactCount int

		for attempt := 1; attempt <= maxAttempts && !found; attempt++ {
			readCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()

			taskID := fmt.Sprintf("e2e-artifact-1408-001-%d", attempt)
			resp, err := a2aSSEPost(readCtx, a2aMessageStream(taskID, "progressive investigate"))
			Expect(err).NotTo(HaveOccurred())

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

				if kind, _ := raw["kind"].(string); kind == artifactUpdate {
					artifactEvents = append(artifactEvents, raw)
				}
			}
			_ = resp.Body.Close()

			GinkgoWriter.Printf("Structured artifact contract (attempt %d/%d): %d artifact events collected\n",
				attempt, maxAttempts, len(artifactEvents))
			lastArtifactCount = len(artifactEvents)

			for _, evt := range artifactEvents {
				artifact, _ := evt["artifact"].(map[string]any)
				if artifact == nil {
					continue
				}
				meta, _ := artifact["metadata"].(map[string]any)
				if meta["schema"] == investigationSummarySchema && meta["schema_version"] == "1.0" {
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
						Expect(dpData).To(HaveKey("summary"),
							"SI-10: investigation_summary must include summary field")
						found = true
						break
					}
				}
				if found {
					break
				}
			}

			if !found && attempt < maxAttempts {
				GinkgoWriter.Printf("  attempt %d/%d did not observe investigation_summary artifact — retrying with a fresh session\n",
					attempt, maxAttempts)
			}
		}

		Expect(found).To(BeTrue(),
			"SI-10: at least one artifact must have metadata schema=investigation_summary with DataPart containing summary "+
				"(last attempt collected %d artifact events)", lastArtifactCount)
	})
})

// =============================================================================
// E2E-AF-1922: session_active status visibility
//
// Proves the rejected-driver journey: two concurrent "progressive investigate"
// calls target the same fixture resource (af-investigate-e2e/af-investigate-target),
// so the fingerprint-based RR reuse (createOrReuseRR) routes both through the
// same RRID. The first caller acquires KA's single-driver session; the second
// caller's kubernaut_investigate call is rejected with session_active
// (BR-INTERACTIVE-004) and must still receive visible status guidance rather
// than a synthetic RCA-shaped artifact.
//
// FedRAMP: AC-4 (information flow enforcement — the rejected caller's session
// is still observable through the same audit-traceable status channel).
//
// Mock-LLM scenario: af_progressive_investigate
// Keyword trigger: "progressive investigate"
// =============================================================================

var _ = Describe("session_active Status Visibility — #1922", Ordered, Label("e2e", "session-active-status", "1922"), func() {
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

	// findSessionActiveStatus scans an SSE response for the status-only
	// session_active guidance emitted by API Frontend. It deliberately does not
	// look for investigation_summary: the rejected caller has no RCA of its own.
	findSessionActiveStatus := func(resp *http.Response) bool {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)

		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if !strings.HasPrefix(strings.TrimSpace(line), "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "data:"))
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
			if kind, _ := raw["kind"].(string); kind != "status-update" {
				continue
			}
			meta, _ := raw["metadata"].(map[string]any)
			if meta == nil || meta["type"] != "status" {
				continue
			}
			status, _ := raw["status"].(map[string]any)
			message, _ := status["message"].(map[string]any)
			parts, _ := message["parts"].([]any)
			for _, p := range parts {
				part, _ := p.(map[string]any)
				text, _ := part["text"].(string)
				if strings.Contains(text, "already in progress") {
					return true
				}
			}
		}
		return false
	}

	It("E2E-AF-1922-001: AC-4 — rejected concurrent driver's session_active response carries visible status guidance", func() {
		firstCtx, firstCancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer firstCancel()

		firstStarted := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			resp, err := a2aSSEPost(firstCtx, a2aMessageStream("e2e-1922-first", "progressive investigate"))
			close(firstStarted)
			if err != nil {
				return
			}
			defer func() { _ = resp.Body.Close() }()
			// Drain to completion in the background so KA holds the
			// single-driver session for the duration of the test, exactly
			// as a real first responder would.
			sc := bufio.NewScanner(resp.Body)
			sc.Buffer(make([]byte, 64*1024), 1024*1024)
			for sc.Scan() { //nolint:revive // drain-only loop, no per-line action needed
			}
		}()

		select {
		case <-firstStarted:
		case <-time.After(10 * time.Second):
			Fail("first investigate call did not start streaming within 10s")
		}
		// Give the first call time to create/reuse the RR and acquire KA's
		// single-driver session before the second (contending) call arrives.
		time.Sleep(3 * time.Second)

		secondCtx, secondCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer secondCancel()
		resp, err := a2aSSEPost(secondCtx, a2aMessageStream("e2e-1922-second", "progressive investigate"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		statusFound := findSessionActiveStatus(resp)

		GinkgoWriter.Printf("second (contending) caller: session_active status found=%v\n", statusFound)

		By("AC-4: the rejected concurrent driver must receive visible session_active status guidance")
		Expect(statusFound).To(BeTrue(),
			"second caller (rejected via session_active) must receive status guidance (#1922)")
	})
})
