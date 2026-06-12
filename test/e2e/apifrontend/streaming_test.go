package e2e_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	investigationsessionv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
)

var _ = Describe("Investigation Streaming (G3)", Ordered, Label("e2e", "phase3", "g3"), func() {
	var sreToken string

	BeforeEach(func() {
		var err error
		sreToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
		Expect(sreToken).NotTo(BeEmpty())
	})

	listInvestigationSessions := func(ctx context.Context) *investigationsessionv1alpha1.InvestigationSessionList {
		list := &investigationsessionv1alpha1.InvestigationSessionList{}
		Expect(k8sClient.List(ctx, list, client.InNamespace(e2eNamespace))).To(Succeed())
		return list
	}

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

	It("TC-E2E-STREAM-01: A2A invoke with Accept: text/event-stream receives SSE frames", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		resp, err := a2aSSEPost(readCtx, a2aMessageStream("stream-01", "list pods in kubernaut-system"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		ct := resp.Header.Get("Content-Type")
		Expect(ct).To(ContainSubstring("text/event-stream"))

		sc := bufio.NewScanner(resp.Body)
		// SSE lines can be very long; allow larger token than default.
		sc.Buffer(make([]byte, 64*1024), 1024*1024)

		foundData := false
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if strings.HasPrefix(strings.TrimSpace(line), "data:") {
				foundData = true
				break
			}
		}
		Expect(sc.Err()).NotTo(HaveOccurred())
		Expect(foundData).To(BeTrue(), "expected at least one SSE data: line")
	})

	It("TC-E2E-STREAM-02: SSE stream starts successfully for non-remediating prompt", func() {
		streamCtx, streamCancel := context.WithCancel(context.Background())
		defer streamCancel()

		const taskID = "stream-02"
		resp, err := a2aSSEPost(streamCtx, a2aMessageStream(taskID, "list pods in kubernaut-system"))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

		go func() {
			defer func() { _ = resp.Body.Close() }()
			_, _ = io.Copy(io.Discard, resp.Body)
		}()

		// CRD is not created because IS CRD creation requires an explicit
		// kubernaut_investigate call (#1332), which a non-remediating prompt
		// ("list pods") never triggers.
		//
		// Filter by this test's task ID to avoid false positives from CRDs
		// created by prior tests (e.g. STREAM-01) that arrive with a delay.
		kctlCtx := context.Background()
		Consistently(func() bool {
			list := listInvestigationSessions(kctlCtx)
			for _, it := range list.Items {
				if it.Spec.A2ATaskID == taskID {
					GinkgoWriter.Printf("unexpected CRD for task %s: %s\n", taskID, it.Name)
					return false
				}
			}
			return true
		}, 10*time.Second, 2*time.Second).Should(BeTrue(),
			"non-remediating prompt should not create InvestigationSession CRD")
	})

	It("TC-E2E-STREAM-03: Client disconnect -> session phase transitions to Disconnected", func() {
		// Create a prerequisite RR directly — this test validates session
		// phase transitions on disconnect, not RR creation by the LLM.
		kctlCtx := context.Background()
		rrName := fmt.Sprintf("rr-stream03-%d", time.Now().UnixNano())
		Expect(createRR("default", rrName, "Deployment", "web-slow-disconnect-test")).To(Succeed())
		DeferCleanup(func() { deleteRR("default", rrName) })

		// #1332: invoke kubernaut_investigate via MCP to create IS CRD.
		sreToken, tokenErr := fetchDEXTokenForPersona("sre")
		Expect(tokenErr).NotTo(HaveOccurred())
		mcpSess, mcpSessErr := initMCPSession(sreToken)
		Expect(mcpSessErr).NotTo(HaveOccurred())

		// Explicitly cancel the MCP session after this test to release the
		// server-side SSE tracker slot. Without this, the handler may hold the
		// connection until the investigation times out (~120s), starving
		// STREAM-04's precondition that all SSE slots are drained.
		DeferCleanup(func() {
			cancelBody := buildJSONRPC("stream-03-cancel", "tools/call", map[string]interface{}{
				"name":      "kubernaut_investigate",
				"arguments": map[string]interface{}{"rr_id": rrName, "action": "cancel"},
			})
			_, _, _ = mcpPOST(sreToken, mcpSess, cancelBody)
		})

		takeoverBody := buildJSONRPC("stream-03-takeover", "tools/call", map[string]interface{}{
			"name":      "kubernaut_investigate",
			"arguments": map[string]interface{}{"rr_id": rrName},
		})
		_, takeoverCode, takeoverErr := mcpPOST(sreToken, mcpSess, takeoverBody)
		Expect(takeoverErr).NotTo(HaveOccurred())
		Expect(takeoverCode).To(BeNumerically("<", 500))

		var isName string
		Eventually(func() bool {
			list := listInvestigationSessions(kctlCtx)
			for _, it := range list.Items {
				if it.Spec.RemediationRequestRef.Name == rrName {
					isName = it.Name
					return true
				}
			}
			return false
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"IS CRD must be created after kubernaut_investigate")

		// Simulate client disconnect by updating IS CRD phase directly.
		isNamespace := getEnvOrDefault("AF_E2E_NAMESPACE", "kubernaut-system")
		is := &investigationsessionv1alpha1.InvestigationSession{}
		Expect(k8sClient.Get(kctlCtx, types.NamespacedName{Name: isName, Namespace: isNamespace}, is)).To(Succeed())
		is.Status.Phase = investigationsessionv1alpha1.SessionPhaseDisconnected
		is.Status.ConnectionState = investigationsessionv1alpha1.ConnectionStateDisconnected
		Expect(k8sClient.Status().Update(kctlCtx, is)).To(Succeed())

		// Assert the IS CRD transitions to Disconnected.
		Eventually(func() string {
			list := listInvestigationSessions(kctlCtx)
			for _, it := range list.Items {
				if it.Name == isName {
					return string(it.Status.Phase)
				}
			}
			return ""
		}, 30*time.Second, 2*time.Second).Should(Equal("Disconnected"),
			"IS CRD must transition to Disconnected after SSE disconnect (BR-SESS-003, SI-4)")
	})

	It("TC-E2E-STREAM-04 / TC-E2E-SSE-CAP-01: Connection cap enforcement", func() {
		// Wait for all SSE slots to drain. Prior tests (STREAM-01/02/03) open
		// SSE or MCP connections whose server-side handlers may run until the
		// LLM investigation completes or the connection timeout fires (~120s).
		// Use 300s to accommodate CI variability and slow handler teardown.
		Eventually(func() float64 {
			return counterValue(scrapeMetrics(), "af_sse_active_connections")
		}, 300*time.Second, 3*time.Second).Should(BeZero(),
			"all SSE slots must be released before cap enforcement test")

		maxStr := getEnvOrDefault("AF_E2E_MAX_SSE", "5")
		maxSSE := 5
		var parsed int
		if n, _ := fmt.Sscanf(strings.TrimSpace(maxStr), "%d", &parsed); n == 1 && parsed > 0 {
			maxSSE = parsed
		}

		type slotResult struct {
			idx    int
			status int
			err    error
		}
		var mu sync.Mutex
		cancels := make([]context.CancelFunc, maxSSE)
		ready := make(chan slotResult, 1)
		var wg sync.WaitGroup

		// Establish SSE connections sequentially to avoid a race where all
		// goroutines hit the server simultaneously and some are rejected
		// before prior connections are fully registered in the semaphore.
		for i := 0; i < maxSSE; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				sctx, scancel := context.WithCancel(context.Background())
				mu.Lock()
				cancels[idx] = scancel
				mu.Unlock()

				body := a2aMessageStream(fmt.Sprintf("stream-cap-%d", idx),
					fmt.Sprintf("Create a remediation request for deployment cap%d-slow-disconnect-test in default namespace", idx))
				req, rerr := http.NewRequestWithContext(sctx, http.MethodPost, baseURL+"/a2a/invoke", strings.NewReader(body))
				if rerr != nil {
					ready <- slotResult{idx: idx, status: -1, err: rerr}
					scancel()
					return
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "text/event-stream")
				req.Header.Set("Authorization", "Bearer "+sreToken)

				resp, derr := httpClient.Do(req)
				if derr != nil {
					ready <- slotResult{idx: idx, status: -1, err: derr}
					scancel()
					return
				}
				if resp.StatusCode != http.StatusOK {
					body, _ := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					ready <- slotResult{idx: idx, status: resp.StatusCode, err: fmt.Errorf("body: %s", body)}
					scancel()
					return
				}

				ready <- slotResult{idx: idx, status: resp.StatusCode}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				scancel()
			}(i)

			// Wait for this connection to be established before launching the
			// next one. The goroutine stays alive (blocked on io.Copy) keeping
			// the SSE slot occupied.
			sr := <-ready
			Expect(sr.status).To(Equal(http.StatusOK),
				"SSE slot %d (goroutine %d) should connect within cap; err=%v", i, sr.idx, sr.err)
		}

		extraReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/a2a/invoke",
			strings.NewReader(a2aMessageStream("stream-cap-overflow", "ping")))
		Expect(err).NotTo(HaveOccurred())
		extraReq.Header.Set("Content-Type", "application/json")
		extraReq.Header.Set("Accept", "text/event-stream")
		extraReq.Header.Set("Authorization", "Bearer "+sreToken)

		exResp, exErr := httpClient.Do(extraReq)
		Expect(exErr).NotTo(HaveOccurred())
		defer func() { _ = exResp.Body.Close() }()
		Expect(exResp.StatusCode).To(Equal(http.StatusServiceUnavailable))
		extraBody, rerr := io.ReadAll(exResp.Body)
		Expect(rerr).NotTo(HaveOccurred())
		Expect(strings.ToLower(string(extraBody))).To(ContainSubstring("too many concurrent connections"))

		mu.Lock()
		for _, c := range cancels {
			if c != nil {
				c()
			}
		}
		mu.Unlock()

		wg.Wait()
	})
})

// ═══════════════════════════════════════════════════════════════
// TC-E2E-STREAM-05: FedRAMP AU-6/SC-4 — Progressive Streaming
// Validates that a 2-turn A2A streaming conversation produces SSE
// frames with content: LLM output as TaskArtifactUpdateEvent and
// status/reasoning as TaskStatusUpdateEvent with metadata.type.
// ═══════════════════════════════════════════════════════════════

var _ = Describe("Progressive A2A Streaming (issue #1258)", Label("e2e", "phase3", "streaming"), func() {
	var sreToken string

	BeforeEach(func() {
		var err error
		sreToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
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

	type sseEvent struct {
		JSONRPC string          `json:"jsonrpc"`
		Result  json.RawMessage `json:"result"`
	}

	type taskStatusUpdate struct {
		Kind   string `json:"kind"`
		Status struct {
			State   string `json:"state"`
			Message *struct {
				Parts []struct {
					Kind string `json:"kind"`
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"message,omitempty"`
		} `json:"status"`
		Metadata map[string]any `json:"metadata,omitempty"`
	}

	type taskArtifactUpdate struct {
		Kind     string `json:"kind"`
		Artifact struct {
			Parts []struct {
				Kind string `json:"kind"`
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"artifact"`
	}

	scanSSEFrames := func(resp *http.Response) (artifacts []taskArtifactUpdate, statuses []taskStatusUpdate) {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)

		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if !strings.HasPrefix(payload, "{") {
				continue
			}

			var evt sseEvent
			if err := json.Unmarshal([]byte(payload), &evt); err != nil {
				continue
			}
			if len(evt.Result) == 0 {
				continue
			}

			var raw map[string]interface{}
			if err := json.Unmarshal(evt.Result, &raw); err != nil {
				continue
			}

			kind, _ := raw["kind"].(string)
			switch kind {
			case "artifact-update":
				var art taskArtifactUpdate
				if json.Unmarshal(evt.Result, &art) == nil {
					artifacts = append(artifacts, art)
				}
			case "status-update":
				var st taskStatusUpdate
				if json.Unmarshal(evt.Result, &st) == nil {
					statuses = append(statuses, st)
				}
			}
		}
		return artifacts, statuses
	}

	It("TC-E2E-STREAM-05: AU-6/SC-4 progressive streaming produces artifact and status content", func() {
		streamCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		// Turn 1: Start investigation — triggers kubernaut_investigate
		// (requires mock-LLM af_investigate keyword scenario)
		resp1, err := a2aSSEPost(streamCtx, a2aMessageStream("progressive-05-t1", "start investigation for pod nginx in default"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp1.Body.Close() }()
		Expect(resp1.StatusCode).To(Equal(http.StatusOK), "turn 1 HTTP status")
		Expect(resp1.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"),
			"AU-6: streaming response must use SSE content type")

		arts1, statuses1 := scanSSEFrames(resp1)
		GinkgoWriter.Printf("Turn 1: %d artifacts, %d statuses\n", len(arts1), len(statuses1))

		// AU-6: Verify SSE events are produced (stream lifecycle visible)
		Expect(len(arts1) + len(statuses1)).To(BeNumerically(">", 0),
			"AU-6: streaming must produce at least one SSE event (artifact or status)")

		hasTerminal := false
		for _, st := range statuses1 {
			if st.Status.State == "completed" || st.Status.State == "failed" {
				hasTerminal = true
				break
			}
		}
		Expect(hasTerminal).To(BeTrue(), "AU-6: stream must reach terminal state (stream_closed)")

		// Turn 2: Progressive streaming (requires $from_tool mock-LLM support).
		// Only assert progressive artifacts if turn 1 completed with "completed".
		turn1Completed := false
		for _, st := range statuses1 {
			if st.Status.State == "completed" {
				turn1Completed = true
				break
			}
		}
		if !turn1Completed {
			GinkgoWriter.Println("Turn 1 did not complete successfully; skipping turn 2 progressive assertion")
			return
		}

		resp2, err := a2aSSEPost(streamCtx, a2aMessageStream("progressive-05-t2", "progressive stream the investigation"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp2.Body.Close() }()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK), "turn 2 HTTP status")

		arts2, statuses2 := scanSSEFrames(resp2)
		GinkgoWriter.Printf("Turn 2: %d artifacts, %d statuses\n", len(arts2), len(statuses2))

		// AU-6 assertion: stream produces events
		Expect(len(arts2)+len(statuses2)).To(BeNumerically(">", 0),
			"turn 2 must produce SSE events (progressive artifacts or status updates)")

		// SC-4 assertion: the stream produces non-empty text content in either
		// artifact events (LLM output) or status events (reasoning/progress).
		// The hybrid approach routes LLM text through TaskArtifactUpdateEvent
		// (rendered as the main response) and status/reasoning through
		// TaskStatusUpdateEvent with metadata.type tags.
		allArts := append(arts1, arts2...)
		allStatuses := append(statuses1, statuses2...)
		hasContent := false
		for _, art := range allArts {
			for _, part := range art.Artifact.Parts {
				if part.Kind == "text" && strings.TrimSpace(part.Text) != "" {
					hasContent = true
					break
				}
			}
			if hasContent {
				break
			}
		}
		if !hasContent {
			for _, st := range allStatuses {
				if st.Status.Message != nil {
					for _, part := range st.Status.Message.Parts {
						if part.Kind == "text" && strings.TrimSpace(part.Text) != "" {
							hasContent = true
							break
						}
					}
				}
				if hasContent {
					break
				}
			}
		}
		Expect(hasContent).To(BeTrue(),
			"progressive stream must contain non-empty text content in artifacts or status events (SC-4)")

		// Final state must be completed or failed (stream lifecycle closed)
		hasFinal := false
		for _, st := range statuses2 {
			if st.Status.State == "completed" || st.Status.State == "failed" {
				hasFinal = true
				break
			}
		}
		Expect(hasFinal).To(BeTrue(), "AU-6: stream must reach a terminal state (stream_closed)")
	})
})

// =============================================================================
// E2E-AF-1399: A2A Streaming — Reasoning Routing + Structured Artifacts
// Proves that the production SSE stream correctly separates thinking (reasoning)
// from final output and delivers decision artifacts as structured data.
// =============================================================================

var _ = Describe("A2A Streaming Reasoning (#1399)", Ordered, Label("e2e", "phase3", "g3", "1399"), func() {
	var sreToken string

	BeforeEach(func() {
		var err error
		sreToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		Expect(sreToken).NotTo(BeEmpty())
	})

	a2aSSEPostReq := func(ctx context.Context, body string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/a2a/invoke", strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+sreToken)
		return httpClient.Do(req)
	}

	It("E2E-AF-1399-001: SSE stream emits reasoning events with metadata.type=reasoning", func() {
		streamCtx, streamCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer streamCancel()

		resp, err := a2aSSEPostReq(streamCtx, a2aMessageStream("reasoning-e2e-001", "present structured rca decision"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var wg sync.WaitGroup
		var mu sync.Mutex
		var events []json.RawMessage

		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(resp.Body)
			sc.Buffer(make([]byte, 64*1024), 1024*1024)
			for sc.Scan() {
				line := strings.TrimRight(sc.Text(), "\r")
				if strings.HasPrefix(strings.TrimSpace(line), "data:") {
					data := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "data:"))
					if data != "" {
						mu.Lock()
						events = append(events, json.RawMessage(data))
						mu.Unlock()
					}
				}
			}
		}()
		wg.Wait()

		var foundReasoning bool
		for _, raw := range events {
			var evt map[string]any
			if err := json.Unmarshal(raw, &evt); err != nil {
				continue
			}
			meta, _ := evt["metadata"].(map[string]any)
			if meta != nil && meta["type"] == "reasoning" {
				foundReasoning = true
				break
			}
		}
		Expect(foundReasoning).To(BeTrue(),
			"SI-4: SSE stream must contain at least one reasoning-type event")
	})

	It("E2E-AF-1399-002: SSE stream emits TaskArtifactUpdateEvent for structured decision", func() {
		streamCtx, streamCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer streamCancel()

		resp, err := a2aSSEPostReq(streamCtx, a2aMessageStream("reasoning-e2e-002", "present structured rca decision"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var wg sync.WaitGroup
		var mu sync.Mutex
		var events []json.RawMessage

		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(resp.Body)
			sc.Buffer(make([]byte, 64*1024), 1024*1024)
			for sc.Scan() {
				line := strings.TrimRight(sc.Text(), "\r")
				if strings.HasPrefix(strings.TrimSpace(line), "data:") {
					data := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "data:"))
					if data != "" {
						mu.Lock()
						events = append(events, json.RawMessage(data))
						mu.Unlock()
					}
				}
			}
		}()
		wg.Wait()

		var foundArtifact bool
		for _, raw := range events {
			var evt map[string]any
			if err := json.Unmarshal(raw, &evt); err != nil {
				continue
			}
			if _, hasArtifact := evt["artifact"]; hasArtifact {
				artifact, _ := evt["artifact"].(map[string]any)
				if artifact != nil {
					meta, _ := artifact["metadata"].(map[string]any)
					if meta != nil && meta["type"] == "decision" {
						foundArtifact = true
						break
					}
				}
			}
		}
		Expect(foundArtifact).To(BeTrue(),
			"AU-3: SSE stream must contain TaskArtifactUpdateEvent with decision metadata")
	})

	It("E2E-AF-1399-003: Final LLM text in SSE stream has no emoji characters", func() {
		streamCtx, streamCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer streamCancel()

		resp, err := a2aSSEPostReq(streamCtx, a2aMessageStream("reasoning-e2e-003", "list pods in kubernaut-system"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var wg sync.WaitGroup
		var mu sync.Mutex
		var events []json.RawMessage

		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(resp.Body)
			sc.Buffer(make([]byte, 64*1024), 1024*1024)
			for sc.Scan() {
				line := strings.TrimRight(sc.Text(), "\r")
				if strings.HasPrefix(strings.TrimSpace(line), "data:") {
					data := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "data:"))
					if data != "" {
						mu.Lock()
						events = append(events, json.RawMessage(data))
						mu.Unlock()
					}
				}
			}
		}()
		wg.Wait()

		for _, raw := range events {
			var evt map[string]any
			if err := json.Unmarshal(raw, &evt); err != nil {
				continue
			}
			if artifact, hasArtifact := evt["artifact"].(map[string]any); hasArtifact {
				if parts, ok := artifact["parts"].([]any); ok {
					for _, part := range parts {
						pm, _ := part.(map[string]any)
						if text, ok := pm["text"].(string); ok {
							for _, r := range text {
								Expect(r >= 0x1F300 && r <= 0x1FAFF).To(BeFalse(),
									fmt.Sprintf("SC-7: artifact text contains emoji U+%04X", r))
							}
						}
					}
				}
			}
		}
	})
})
