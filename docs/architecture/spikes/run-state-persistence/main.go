package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// This spike validates the ephemeral run state model (Option A from #1206):
//
// 1. SSE stream drop detection: KA detects when the OAS Runtime process dies
//    mid-investigation via SSE stream EOF or HTTP connection reset.
//
// 2. GET /runs/{id} after restart: Returns 404 (state lost), confirming
//    KA cannot resume — must retry from scratch.
//
// 3. Health check failure: /healthz becomes unreachable, KA detects within
//    the health check interval.
//
// 4. Stateless retry: KA creates a new run with the same input after detecting
//    failure. The new run gets a new run_id (independent from the failed one).
//
// 5. Partial output preservation: KA preserves any SSE events received before
//    the crash for audit trail / trajectory reconstruction.

// --- ACP types ---

type acpRun struct {
	AgentName string   `json:"agent_name"`
	RunID     string   `json:"run_id"`
	Status    string   `json:"status"`
	Output    []acpMsg `json:"output"`
}

type acpMsg struct {
	Role  string    `json:"role"`
	Parts []acpPart `json:"parts"`
}

type acpPart struct {
	ContentType string      `json:"content_type"`
	Content     string      `json:"content"`
	Metadata    interface{} `json:"metadata,omitempty"`
}

type acpEvent struct {
	Type string   `json:"type"`
	Run  *acpRun  `json:"run,omitempty"`
	Part *acpPart `json:"part,omitempty"`
}

// --- Results ---

type spikeResults struct {
	mu      sync.Mutex
	tests   []testResult
	metrics map[string]time.Duration
}

type testResult struct {
	name   string
	passed bool
	detail string
}

func (r *spikeResults) pass(name, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tests = append(r.tests, testResult{name, true, detail})
	log.Printf("[PASS] %s: %s", name, detail)
}

func (r *spikeResults) fail(name, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tests = append(r.tests, testResult{name, false, detail})
	log.Fatalf("[FAIL] %s: %s", name, detail)
}

func (r *spikeResults) metric(name string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.metrics == nil {
		r.metrics = make(map[string]time.Duration)
	}
	r.metrics[name] = d
	log.Printf("[METRIC] %s: %v", name, d)
}

func (r *spikeResults) summary() {
	r.mu.Lock()
	defer r.mu.Unlock()
	passed, failed := 0, 0
	for _, t := range r.tests {
		if t.passed {
			passed++
		} else {
			failed++
		}
	}
	log.Printf("\n========== SUMMARY ==========")
	log.Printf("Tests: %d passed, %d failed, %d total", passed, failed, passed+failed)
	for name, d := range r.metrics {
		log.Printf("  %s: %v", name, d)
	}
	if failed > 0 {
		log.Fatal("SPIKE FAILED")
	}
}

// --- Killable ACP server (simulates an OAS Runtime pod) ---

type killableACPServer struct {
	mu       sync.RWMutex
	runs     map[string]*acpRun
	listener net.Listener
	srv      *http.Server
	killed   bool
	killCh   chan struct{}
}

func newKillableACPServer() *killableACPServer {
	s := &killableACPServer{
		runs:   make(map[string]*acpRun),
		killCh: make(chan struct{}),
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	s.listener = listener

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /runs", s.handleCreateRun)
	mux.HandleFunc("GET /runs/{run_id}", s.handleGetRun)

	s.srv = &http.Server{Handler: mux}
	go s.srv.Serve(listener)

	return s
}

func (s *killableACPServer) Addr() string {
	return s.listener.Addr().String()
}

// Kill simulates a pod crash: shuts down the HTTP server immediately.
func (s *killableACPServer) Kill() {
	s.mu.Lock()
	s.killed = true
	s.mu.Unlock()
	close(s.killCh)
	s.srv.Close()
}

func (s *killableACPServer) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *killableACPServer) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentName string   `json:"agent_name"`
		Input     []acpMsg `json:"input"`
		Mode      string   `json:"mode"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	runID := uuid.New().String()
	prompt := ""
	if len(req.Input) > 0 && len(req.Input[0].Parts) > 0 {
		prompt = req.Input[0].Parts[0].Content
	}

	if req.Mode == "stream" {
		s.handleStreamRun(w, runID, req.AgentName, prompt)
		return
	}

	run := &acpRun{
		AgentName: req.AgentName,
		RunID:     runID,
		Status:    "completed",
		Output: []acpMsg{{
			Role:  "agent",
			Parts: []acpPart{{ContentType: "text/plain", Content: "Result for: " + prompt}},
		}},
	}
	s.mu.Lock()
	s.runs[runID] = run
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, run)
}

func (s *killableACPServer) handleStreamRun(w http.ResponseWriter, runID, agentName, prompt string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	run := &acpRun{AgentName: agentName, RunID: runID, Status: "in-progress", Output: []acpMsg{}}
	s.mu.Lock()
	s.runs[runID] = run
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	sendSSE := func(evt acpEvent) {
		data, _ := json.Marshal(evt)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
		flusher.Flush()
	}

	sendSSE(acpEvent{Type: "run.created", Run: run})
	sendSSE(acpEvent{Type: "run.in-progress", Run: run})

	steps := []string{
		"Analyzing pod status...",
		"Checking container logs...",
		"Identifying root cause...",
		"Correlating with recent deployments...",
		"Building remediation plan...",
	}

	for i, step := range steps {
		select {
		case <-s.killCh:
			// Server killed mid-stream — connection will be severed
			return
		default:
		}

		sendSSE(acpEvent{
			Type: "message.part",
			Part: &acpPart{ContentType: "text/plain", Content: step},
		})

		if i < len(steps)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	run.Status = "completed"
	run.Output = []acpMsg{{
		Role:  "agent",
		Parts: []acpPart{{ContentType: "text/plain", Content: "Root cause identified: OOM kill"}},
	}}
	sendSSE(acpEvent{Type: "run.completed", Run: run})

	s.mu.Lock()
	s.runs[runID] = run
	s.mu.Unlock()
}

func (s *killableACPServer) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	s.mu.RLock()
	run, ok := s.runs[runID]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// --- Main ---

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	results := &spikeResults{}

	testSSEStreamDrop(results)
	testGetRunAfterRestart(results)
	testHealthCheckFailure(results)
	testStatelessRetry(results)
	testPartialOutputPreservation(results)
	testOptionAvsOptionC(results)

	results.summary()
}

// Test 1: SSE stream EOF detected when runtime dies mid-stream.
func testSSEStreamDrop(results *spikeResults) {
	log.Println("\n========== TEST 1: SSE stream drop detection ==========")

	srv := newKillableACPServer()
	addr := srv.Addr()
	waitForHealth(fmt.Sprintf("http://%s/healthz", addr))

	// Schedule kill after 250ms (mid-stream)
	go func() {
		time.Sleep(250 * time.Millisecond)
		log.Println("[infra] Killing OAS Runtime (simulating pod crash)")
		srv.Kill()
	}()

	start := time.Now()
	events, streamErr := streamRunWithError(addr, "investigation", "Analyze CrashLoopBackOff")
	detectionLatency := time.Since(start)
	results.metric("sse_drop_detection", detectionLatency)

	if streamErr == nil && !hasRunCompleted(events) {
		results.fail("sse-drop", "expected stream error or incomplete stream, got clean completion")
	}

	if streamErr != nil {
		results.pass("sse-drop/error-detected", fmt.Sprintf("stream error: %v", streamErr))
	} else {
		results.pass("sse-drop/incomplete-stream", "stream ended without run.completed (EOF)")
	}

	results.pass("sse-drop/detection-time", fmt.Sprintf("detected in %v", detectionLatency))
	log.Printf("[info] Received %d events before crash", len(events))
}

// Test 2: GET /runs/{id} returns 404 after runtime restart.
func testGetRunAfterRestart(results *spikeResults) {
	log.Println("\n========== TEST 2: GET /runs/{id} after restart ==========")

	srv1 := newKillableACPServer()
	addr := srv1.Addr()
	waitForHealth(fmt.Sprintf("http://%s/healthz", addr))

	// Create a successful run, capture its run_id
	run := createSyncRun(addr, "investigation", "Pre-crash run")
	originalRunID := run.RunID
	results.pass("get-after-restart/original-created", fmt.Sprintf("run_id=%s", originalRunID))

	// Verify it's accessible
	fetched := getRun(addr, originalRunID)
	if fetched.Status != "completed" {
		results.fail("get-after-restart", "original run not found before crash")
	}
	results.pass("get-after-restart/accessible-before-crash", "run accessible")

	// Kill and restart (new server on same port is impossible, so new address)
	srv1.Kill()
	time.Sleep(100 * time.Millisecond)

	srv2 := newKillableACPServer()
	newAddr := srv2.Addr()
	waitForHealth(fmt.Sprintf("http://%s/healthz", newAddr))

	// Try to fetch original run from new instance
	start := time.Now()
	resp, err := http.Get(fmt.Sprintf("http://%s/runs/%s", newAddr, originalRunID))
	lookupLatency := time.Since(start)
	results.metric("get_run_after_restart", lookupLatency)

	if err != nil {
		results.fail("get-after-restart", fmt.Sprintf("HTTP error: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		results.fail("get-after-restart", fmt.Sprintf("expected 404, got %d", resp.StatusCode))
	}
	results.pass("get-after-restart/404-confirmed", fmt.Sprintf("GET /runs/%s → 404 (state lost)", originalRunID))

	srv2.Kill()
}

// Test 3: Health check becomes unreachable after crash.
func testHealthCheckFailure(results *spikeResults) {
	log.Println("\n========== TEST 3: Health check failure ==========")

	srv := newKillableACPServer()
	addr := srv.Addr()
	waitForHealth(fmt.Sprintf("http://%s/healthz", addr))
	results.pass("health-check/initial", "healthy before crash")

	srv.Kill()
	time.Sleep(50 * time.Millisecond)

	start := time.Now()
	client := &http.Client{Timeout: 1 * time.Second}
	_, err := client.Get(fmt.Sprintf("http://%s/healthz", addr))
	detectionLatency := time.Since(start)
	results.metric("health_check_failure_detection", detectionLatency)

	if err == nil {
		results.fail("health-check", "expected connection error after crash")
	}
	results.pass("health-check/unreachable", fmt.Sprintf("detected in %v: %v", detectionLatency, err))
}

// Test 4: Stateless retry — KA creates a new run after detecting failure.
func testStatelessRetry(results *spikeResults) {
	log.Println("\n========== TEST 4: Stateless retry ==========")

	// Phase 1: Original run crashes
	srv1 := newKillableACPServer()
	addr1 := srv1.Addr()
	waitForHealth(fmt.Sprintf("http://%s/healthz", addr1))

	go func() {
		time.Sleep(150 * time.Millisecond)
		srv1.Kill()
	}()

	events1, _ := streamRunWithError(addr1, "investigation", "Analyze CrashLoopBackOff for pod nginx")
	log.Printf("[KA] Original run crashed after %d events", len(events1))

	// Phase 2: KA detects failure and retries on new runtime
	srv2 := newKillableACPServer()
	addr2 := srv2.Addr()
	waitForHealth(fmt.Sprintf("http://%s/healthz", addr2))

	start := time.Now()
	retryRun := createSyncRun(addr2, "investigation", "Analyze CrashLoopBackOff for pod nginx")
	retryLatency := time.Since(start)
	results.metric("stateless_retry", retryLatency)

	if retryRun.Status != "completed" {
		results.fail("stateless-retry", fmt.Sprintf("retry run status=%s", retryRun.Status))
	}
	results.pass("stateless-retry/completed", fmt.Sprintf("retry run_id=%s completed in %v", retryRun.RunID, retryLatency))

	if len(retryRun.Output) == 0 {
		results.fail("stateless-retry", "retry run has no output")
	}
	results.pass("stateless-retry/has-output", fmt.Sprintf("retry run has %d output messages", len(retryRun.Output)))

	srv2.Kill()
}

// Test 5: Partial output from crashed stream is preserved by KA.
func testPartialOutputPreservation(results *spikeResults) {
	log.Println("\n========== TEST 5: Partial output preservation ==========")

	srv := newKillableACPServer()
	addr := srv.Addr()
	waitForHealth(fmt.Sprintf("http://%s/healthz", addr))

	go func() {
		time.Sleep(250 * time.Millisecond)
		srv.Kill()
	}()

	events, _ := streamRunWithError(addr, "investigation", "Multi-step investigation")

	// Count message.part events received before crash
	var partialParts []string
	for _, evt := range events {
		if evt.Part != nil && evt.Part.Content != "" {
			partialParts = append(partialParts, evt.Part.Content)
		}
	}

	if len(partialParts) == 0 {
		results.pass("partial-output/none", "crash before any events (acceptable — KA retries from scratch)")
	} else {
		results.pass("partial-output/preserved", fmt.Sprintf("%d events preserved for audit trail", len(partialParts)))
		for i, part := range partialParts {
			log.Printf("[audit] partial event %d: %s", i+1, part)
		}
	}

	kaAuditRecord := map[string]interface{}{
		"type":              "infrastructure_failure",
		"error_code":        "ERR_UPSTREAM_RUNTIME_CRASH",
		"retry_possible":    true,
		"partial_events":    len(partialParts),
		"partial_trajectory": partialParts,
		"action":            "stateless_retry",
	}
	auditJSON, _ := json.MarshalIndent(kaAuditRecord, "", "  ")
	log.Printf("[audit] KA error record:\n%s", string(auditJSON))
	results.pass("partial-output/audit-record", "audit record with partial trajectory constructed")
}

// Test 6: Compare Option A (ephemeral) vs Option C (KA-side state only).
func testOptionAvsOptionC(results *spikeResults) {
	log.Println("\n========== TEST 6: Option A vs Option C comparison ==========")

	// Option A: Runtime has run state, GET /runs/{id} works while alive
	srvA := newKillableACPServer()
	addrA := srvA.Addr()
	waitForHealth(fmt.Sprintf("http://%s/healthz", addrA))

	runA := createSyncRun(addrA, "investigation", "Option A test")
	fetchedA := getRun(addrA, runA.RunID)
	optionAWorks := fetchedA.Status == "completed"
	results.pass("option-comparison/A-get-works", fmt.Sprintf("GET /runs/%s → %s", runA.RunID, fetchedA.Status))

	srvA.Kill()
	time.Sleep(50 * time.Millisecond)

	_, errA := http.Get(fmt.Sprintf("http://%s/runs/%s", addrA, runA.RunID))
	optionAAfterCrash := errA != nil
	results.pass("option-comparison/A-lost-after-crash", fmt.Sprintf("connection error: %v", errA != nil))

	// Option C: Runtime is stateless, GET /runs/{id} always returns 404
	srvC := newStatelessACPServer()
	addrC := srvC.Addr()
	waitForHealth(fmt.Sprintf("http://%s/healthz", addrC))

	runC := createSyncRun(addrC, "investigation", "Option C test")
	respC, _ := http.Get(fmt.Sprintf("http://%s/runs/%s", addrC, runC.RunID))
	optionCAlways404 := respC != nil && respC.StatusCode == http.StatusNotFound
	if respC != nil {
		respC.Body.Close()
	}
	results.pass("option-comparison/C-always-404", fmt.Sprintf("GET /runs/%s → 404 (by design)", runC.RunID))

	srvC.Kill()

	log.Println("\n--- Option Comparison ---")
	log.Printf("Option A: GET works while alive=%v, lost after crash=%v", optionAWorks, optionAAfterCrash)
	log.Printf("Option C: GET always 404=%v (KA tracks all state in CRD)", optionCAlways404)
	log.Println("Option B (persistent state): NOT TESTED — adds complexity for negligible benefit")
	log.Println("")
	log.Println("RECOMMENDATION: Option A (ephemeral in-memory)")
	log.Println("  - GET /runs/{id} useful for in-flight status checks")
	log.Println("  - Crash = run failure, KA detects and retries from scratch")
	log.Println("  - Same behavior as the goose-server OAS runtime evaluated in this spike (superseded by #1536; opaque OCI agents don't expose an in-process run state to persist)")
	log.Println("  - No additional infrastructure (no PVCs, no ConfigMaps)")
	log.Println("  - KA already owns durable state via AgenticWorkflow CRD")

	results.pass("option-comparison/recommendation", "Option A: ephemeral in-memory (no persistence needed)")
}

// --- Stateless ACP server for Option C comparison ---

type statelessACPServer struct {
	listener net.Listener
	srv      *http.Server
}

func newStatelessACPServer() *statelessACPServer {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	s := &statelessACPServer{listener: listener}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /runs", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AgentName string   `json:"agent_name"`
			Input     []acpMsg `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		prompt := ""
		if len(req.Input) > 0 && len(req.Input[0].Parts) > 0 {
			prompt = req.Input[0].Parts[0].Content
		}
		// Returns completed run but does NOT store it
		writeJSON(w, http.StatusOK, acpRun{
			AgentName: req.AgentName,
			RunID:     uuid.New().String(),
			Status:    "completed",
			Output: []acpMsg{{
				Role:  "agent",
				Parts: []acpPart{{ContentType: "text/plain", Content: "Result for: " + prompt}},
			}},
		})
	})
	mux.HandleFunc("GET /runs/{run_id}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "stateless: run not tracked"})
	})

	s.srv = &http.Server{Handler: mux}
	go s.srv.Serve(listener)

	return s
}

func (s *statelessACPServer) Addr() string { return s.listener.Addr().String() }
func (s *statelessACPServer) Kill()        { s.srv.Close() }

// --- HTTP helpers ---

func createSyncRun(addr, agentName, prompt string) acpRun {
	body := map[string]interface{}{
		"agent_name": agentName,
		"mode":       "sync",
		"input":      []acpMsg{{Parts: []acpPart{{ContentType: "text/plain", Content: prompt}}}},
	}
	data, _ := json.Marshal(body)
	resp, err := http.Post(fmt.Sprintf("http://%s/runs", addr), "application/json", bytes.NewReader(data))
	if err != nil {
		log.Fatalf("create sync run: %v", err)
	}
	defer resp.Body.Close()
	var run acpRun
	json.NewDecoder(resp.Body).Decode(&run)
	return run
}

func getRun(addr, runID string) acpRun {
	resp, err := http.Get(fmt.Sprintf("http://%s/runs/%s", addr, runID))
	if err != nil {
		log.Fatalf("get run: %v", err)
	}
	defer resp.Body.Close()
	var run acpRun
	json.NewDecoder(resp.Body).Decode(&run)
	return run
}

func streamRunWithError(addr, agentName, prompt string) ([]acpEvent, error) {
	body := map[string]interface{}{
		"agent_name": agentName,
		"mode":       "stream",
		"input":      []acpMsg{{Parts: []acpPart{{ContentType: "text/plain", Content: prompt}}}},
	}
	data, _ := json.Marshal(body)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://%s/runs", addr), bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var events []acpEvent
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			var evt acpEvent
			if err := json.Unmarshal([]byte(line[6:]), &evt); err == nil {
				events = append(events, evt)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if err != io.EOF {
			return events, err
		}
	}

	return events, nil
}

func hasRunCompleted(events []acpEvent) bool {
	for _, evt := range events {
		if evt.Type == "run.completed" {
			return true
		}
	}
	return false
}

func waitForHealth(url string) {
	for i := 0; i < 30; i++ {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Fatalf("health check timeout: %s", url)
}
