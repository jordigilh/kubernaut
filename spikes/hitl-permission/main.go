package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// --- ACP types ---

type acpRun struct {
	AgentName    string        `json:"agent_name"`
	RunID        string        `json:"run_id"`
	SessionID    string        `json:"session_id,omitempty"`
	Status       string        `json:"status"`
	AwaitRequest *awaitRequest `json:"await_request,omitempty"`
	Output       []acpMsg      `json:"output"`
}

type awaitRequest struct {
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data"`
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
	Type    string   `json:"type"`
	Run     *acpRun  `json:"run,omitempty"`
	Part    *acpPart `json:"part,omitempty"`
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

// --- Mock ACP server with PermissionGate ---

type hitlACPServer struct {
	mu      sync.RWMutex
	runs    map[string]*acpRun
	gate    *PermissionGate
	sseLog  []acpEvent // recorded SSE events for verification
}

func newHITLACPServer(gate *PermissionGate) *hitlACPServer {
	return &hitlACPServer{
		runs: make(map[string]*acpRun),
		gate: gate,
	}
}

func (s *hitlACPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /runs", s.handleCreateRun)
	mux.HandleFunc("GET /runs/{run_id}", s.handleGetRun)
	mux.HandleFunc("POST /runs/{run_id}", s.handleResumeRun)
	mux.ServeHTTP(w, r)
}

func (s *hitlACPServer) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *hitlACPServer) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentName string   `json:"agent_name"`
		SessionID string   `json:"session_id"`
		Input     []acpMsg `json:"input"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	runID := uuid.New().String()
	run := &acpRun{
		AgentName: req.AgentName,
		RunID:     runID,
		SessionID: req.SessionID,
		Status:    "in-progress",
		Output:    []acpMsg{},
	}

	s.mu.Lock()
	s.runs[runID] = run
	s.mu.Unlock()

	prompt := ""
	if len(req.Input) > 0 && len(req.Input[0].Parts) > 0 {
		prompt = req.Input[0].Parts[0].Content
	}

	go s.simulateAgentWithToolCalls(runID, prompt)

	writeJSON(w, http.StatusAccepted, run)
}

// simulateAgentWithToolCalls mimics an OAS Runtime agent that makes tool calls
// gated by the PermissionGate. Each tool call blocks until KA decides.
func (s *hitlACPServer) simulateAgentWithToolCalls(runID, prompt string) {
	toolCalls := []struct {
		name  string
		input map[string]interface{}
	}{
		{"kubectl_get", map[string]interface{}{"resource": "pods", "namespace": "default"}},
		{"kubectl_logs", map[string]interface{}{"pod": "nginx-abc123", "namespace": "default", "tail": "50"}},
		{"kubectl_delete", map[string]interface{}{"resource": "pod", "name": "nginx-abc123", "namespace": "default"}},
	}

	var output []acpMsg
	for _, tc := range toolCalls {
		denial, err := s.gate.RequestPermission(runID, tc.name, tc.input)
		if err != nil {
			s.mu.Lock()
			s.runs[runID].Status = "failed"
			s.mu.Unlock()
			return
		}

		if denial != "" {
			output = append(output, acpMsg{
				Role: "agent",
				Parts: []acpPart{{
					ContentType: "text/plain",
					Content:     fmt.Sprintf("Tool %s denied: %s. Skipping.", tc.name, denial),
				}},
			})
			continue
		}

		output = append(output, acpMsg{
			Role: "agent",
			Parts: []acpPart{{
				ContentType: "application/json",
				Content:     fmt.Sprintf(`{"tool":"%s","result":"executed successfully"}`, tc.name),
				Metadata: map[string]interface{}{
					"kind":      "trajectory",
					"tool_name": tc.name,
				},
			}},
		})
	}

	s.mu.Lock()
	run := s.runs[runID]
	run.Output = output
	run.Status = "completed"
	run.AwaitRequest = nil
	s.mu.Unlock()
}

func (s *hitlACPServer) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	s.mu.RLock()
	run, ok := s.runs[runID]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *hitlACPServer) handleResumeRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AwaitResume bool `json:"await_resume"`
		Input       []struct {
			Parts []struct {
				Content string `json:"content"`
			} `json:"parts"`
		} `json:"input"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Extract the decision from the input payload
	content := ""
	if len(req.Input) > 0 && len(req.Input[0].Parts) > 0 {
		content = req.Input[0].Parts[0].Content
	}

	var decision struct {
		RequestID string `json:"request_id"`
		Approved  bool   `json:"approved"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid decision payload: " + err.Error()})
		return
	}

	if err := s.gate.Decide(decision.RequestID, PermissionDecision{
		Approved: decision.Approved,
		Reason:   decision.Reason,
	}); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	runID := r.PathValue("run_id")
	s.mu.Lock()
	if run, ok := s.runs[runID]; ok {
		run.Status = "in-progress"
		run.AwaitRequest = nil
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (s *hitlACPServer) recordSSE(evt acpEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sseLog = append(s.sseLog, evt)
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

	testApproveAllTools(results)
	testDenyDestructiveTool(results)
	testTimeout(results)
	testConcurrentToolCalls(results)
	testAwaitRequestPayload(results)

	results.summary()
}

// Test 1: KA approves all three tool calls.
func testApproveAllTools(results *spikeResults) {
	log.Println("\n========== TEST 1: Approve all tools ==========")

	var awaitEvents []PermissionRequest
	var mu sync.Mutex

	gate := NewPermissionGate(5*time.Second, func(req PermissionRequest) {
		mu.Lock()
		awaitEvents = append(awaitEvents, req)
		mu.Unlock()
	})

	srv := newHITLACPServer(gate)
	addr := startServer(srv)
	waitForHealth(fmt.Sprintf("http://%s/healthz", addr))

	// Create a run (async — agent will block on permission gates)
	runID := createRun(addr, "investigation", "Analyze CrashLoopBackOff")

	// Approve each tool call as it arrives
	start := time.Now()
	for i := 0; i < 3; i++ {
		req := waitForPendingRequest(gate, 3*time.Second)
		if req == nil {
			results.fail("approve-all", fmt.Sprintf("no pending request for tool call #%d", i+1))
		}
		log.Printf("[KA] approving tool: %s (request_id=%s)", req.ToolName, req.RequestID)

		decision := fmt.Sprintf(`{"request_id":"%s","approved":true,"reason":"within permission ceiling"}`, req.RequestID)
		resp := resumeRun(addr, runID, decision)
		if resp != "resumed" {
			results.fail("approve-all", fmt.Sprintf("resume failed for tool %s", req.ToolName))
		}
	}
	elapsed := time.Since(start)
	results.metric("approve_all_3_tools", elapsed)

	// Wait for run to complete
	time.Sleep(200 * time.Millisecond)
	run := getRun(addr, runID)

	if run.Status != "completed" {
		results.fail("approve-all", fmt.Sprintf("expected completed, got %s", run.Status))
	}
	results.pass("approve-all/completed", fmt.Sprintf("run completed with %d output messages", len(run.Output)))

	executedTools := 0
	for _, msg := range run.Output {
		for _, part := range msg.Parts {
			if part.ContentType == "application/json" {
				executedTools++
			}
		}
	}
	if executedTools != 3 {
		results.fail("approve-all", fmt.Sprintf("expected 3 executed tools, got %d", executedTools))
	}
	results.pass("approve-all/all-executed", "all 3 tool calls executed")

	mu.Lock()
	if len(awaitEvents) != 3 {
		results.fail("approve-all", fmt.Sprintf("expected 3 await events, got %d", len(awaitEvents)))
	}
	results.pass("approve-all/await-events", fmt.Sprintf("%d run.awaiting events emitted", len(awaitEvents)))
	mu.Unlock()
}

// Test 2: KA denies the destructive kubectl_delete tool.
func testDenyDestructiveTool(results *spikeResults) {
	log.Println("\n========== TEST 2: Deny destructive tool ==========")

	gate := NewPermissionGate(5*time.Second, func(req PermissionRequest) {})
	srv := newHITLACPServer(gate)
	addr := startServer(srv)
	waitForHealth(fmt.Sprintf("http://%s/healthz", addr))

	runID := createRun(addr, "investigation", "Analyze and clean up")

	for i := 0; i < 3; i++ {
		req := waitForPendingRequest(gate, 3*time.Second)
		if req == nil {
			results.fail("deny-destructive", fmt.Sprintf("no pending request for tool call #%d", i+1))
		}

		approved := true
		reason := "within permission ceiling"
		if req.ToolName == "kubectl_delete" {
			approved = false
			reason = "destructive action blocked by permission ceiling"
			log.Printf("[KA] DENYING tool: %s", req.ToolName)
		} else {
			log.Printf("[KA] approving tool: %s", req.ToolName)
		}

		decision := fmt.Sprintf(`{"request_id":"%s","approved":%v,"reason":"%s"}`, req.RequestID, approved, reason)
		resumeRun(addr, runID, decision)
	}

	time.Sleep(200 * time.Millisecond)
	run := getRun(addr, runID)

	if run.Status != "completed" {
		results.fail("deny-destructive", fmt.Sprintf("expected completed, got %s", run.Status))
	}

	deniedFound := false
	executedCount := 0
	for _, msg := range run.Output {
		for _, part := range msg.Parts {
			if part.ContentType == "application/json" {
				executedCount++
			}
			if part.ContentType == "text/plain" && containsStr(part.Content, "kubectl_delete denied") {
				deniedFound = true
			}
		}
	}

	if !deniedFound {
		results.fail("deny-destructive", "denial message for kubectl_delete not found in output")
	}
	results.pass("deny-destructive/denial-recorded", "kubectl_delete denial recorded in agent output")

	if executedCount != 2 {
		results.fail("deny-destructive", fmt.Sprintf("expected 2 executed tools, got %d", executedCount))
	}
	results.pass("deny-destructive/selective", "2 approved, 1 denied — agent continued gracefully")
}

// Test 3: Permission request times out (fail-closed → denied).
func testTimeout(results *spikeResults) {
	log.Println("\n========== TEST 3: Permission timeout ==========")

	shortTimeout := 500 * time.Millisecond
	gate := NewPermissionGate(shortTimeout, func(req PermissionRequest) {})
	srv := newHITLACPServer(gate)
	addr := startServer(srv)
	waitForHealth(fmt.Sprintf("http://%s/healthz", addr))

	createRun(addr, "investigation", "Timeout test")

	// Do NOT respond — let all 3 tool calls timeout
	start := time.Now()
	// Wait for all 3 timeouts to fire (3 * 500ms = 1.5s max)
	time.Sleep(2 * time.Second)
	elapsed := time.Since(start)

	results.metric("timeout_3_tools", elapsed)

	// After timeouts, the gate should have no pending requests
	if gate.PendingCount() != 0 {
		results.fail("timeout", fmt.Sprintf("expected 0 pending, got %d", gate.PendingCount()))
	}
	results.pass("timeout/cleaned-up", "all pending requests cleaned up after timeout")
	results.pass("timeout/fail-closed", fmt.Sprintf("3 tools timed out in %v (fail-closed: denied)", elapsed))
}

// Test 4: Multiple tool calls pending concurrently from different runs.
func testConcurrentToolCalls(results *spikeResults) {
	log.Println("\n========== TEST 4: Concurrent tool calls ==========")

	var awaitEvents []PermissionRequest
	var mu sync.Mutex

	gate := NewPermissionGate(5*time.Second, func(req PermissionRequest) {
		mu.Lock()
		awaitEvents = append(awaitEvents, req)
		mu.Unlock()
	})

	srv := newHITLACPServer(gate)
	addr := startServer(srv)
	waitForHealth(fmt.Sprintf("http://%s/healthz", addr))

	// Start two runs concurrently
	runID1 := createRun(addr, "investigation-1", "Run 1")
	runID2 := createRun(addr, "investigation-2", "Run 2")

	// Wait until both runs have their first tool call pending
	time.Sleep(200 * time.Millisecond)

	pending := gate.PendingRequests()
	if len(pending) < 2 {
		results.fail("concurrent", fmt.Sprintf("expected >=2 pending, got %d", len(pending)))
	}
	results.pass("concurrent/multi-pending", fmt.Sprintf("%d tool calls pending concurrently", len(pending)))

	// Approve all pending requests from both runs
	start := time.Now()
	for i := 0; i < 6; i++ { // 3 tools × 2 runs
		req := waitForPendingRequest(gate, 3*time.Second)
		if req == nil {
			break // may have fewer if timeouts kicked in
		}
		decision := fmt.Sprintf(`{"request_id":"%s","approved":true}`, req.RequestID)
		resumeRun(addr, req.RunID, decision)
	}
	elapsed := time.Since(start)
	results.metric("concurrent_6_approvals", elapsed)

	time.Sleep(300 * time.Millisecond)

	run1 := getRun(addr, runID1)
	run2 := getRun(addr, runID2)

	if run1.Status != "completed" {
		results.fail("concurrent", fmt.Sprintf("run1 status=%s", run1.Status))
	}
	if run2.Status != "completed" {
		results.fail("concurrent", fmt.Sprintf("run2 status=%s", run2.Status))
	}
	results.pass("concurrent/both-completed", "both runs completed independently")

	mu.Lock()
	results.pass("concurrent/await-events", fmt.Sprintf("%d total await events from 2 runs", len(awaitEvents)))
	mu.Unlock()
}

// Test 5: AwaitRequest payload contains tool name, input, and request ID.
func testAwaitRequestPayload(results *spikeResults) {
	log.Println("\n========== TEST 5: AwaitRequest payload ==========")

	var capturedReq *PermissionRequest
	var mu sync.Mutex

	gate := NewPermissionGate(5*time.Second, func(req PermissionRequest) {
		mu.Lock()
		if capturedReq == nil {
			reqCopy := req
			capturedReq = &reqCopy
		}
		mu.Unlock()
	})

	srv := newHITLACPServer(gate)
	addr := startServer(srv)
	waitForHealth(fmt.Sprintf("http://%s/healthz", addr))

	createRun(addr, "investigation", "Payload test")

	// Wait for first tool call
	req := waitForPendingRequest(gate, 3*time.Second)
	if req == nil {
		results.fail("await-payload", "no pending request received")
	}

	mu.Lock()
	captured := capturedReq
	mu.Unlock()

	if captured == nil {
		results.fail("await-payload", "onAwait callback not invoked")
	}

	if captured.RequestID == "" {
		results.fail("await-payload", "request_id is empty")
	}
	results.pass("await-payload/request-id", fmt.Sprintf("request_id=%s", captured.RequestID))

	if captured.RunID == "" {
		results.fail("await-payload", "run_id is empty")
	}
	results.pass("await-payload/run-id", fmt.Sprintf("run_id=%s", captured.RunID))

	if captured.ToolName == "" {
		results.fail("await-payload", "tool_name is empty")
	}
	results.pass("await-payload/tool-name", fmt.Sprintf("tool_name=%s", captured.ToolName))

	if captured.ToolInput == nil || len(captured.ToolInput) == 0 {
		results.fail("await-payload", "tool_input is empty")
	}
	results.pass("await-payload/tool-input", fmt.Sprintf("tool_input has %d fields", len(captured.ToolInput)))

	if captured.CreatedAt.IsZero() {
		results.fail("await-payload", "created_at is zero")
	}
	results.pass("await-payload/created-at", "timestamp present")

	// Approve remaining tools to let the run complete cleanly
	for i := 0; i < 3; i++ {
		r := waitForPendingRequest(gate, 2*time.Second)
		if r == nil {
			break
		}
		decision := fmt.Sprintf(`{"request_id":"%s","approved":true}`, r.RequestID)
		resumeRun(addr, r.RunID, decision)
	}
}

// --- Helpers ---

func startServer(handler http.Handler) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	go (&http.Server{Handler: handler}).Serve(listener)
	return listener.Addr().String()
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

func createRun(addr, agentName, prompt string) string {
	body := map[string]interface{}{
		"agent_name": agentName,
		"input":      []acpMsg{{Parts: []acpPart{{ContentType: "text/plain", Content: prompt}}}},
	}
	data, _ := json.Marshal(body)
	resp, err := http.Post(fmt.Sprintf("http://%s/runs", addr), "application/json", bytes.NewReader(data))
	if err != nil {
		log.Fatalf("create run: %v", err)
	}
	defer resp.Body.Close()
	var run acpRun
	json.NewDecoder(resp.Body).Decode(&run)
	return run.RunID
}

func resumeRun(addr, runID, decisionJSON string) string {
	body := map[string]interface{}{
		"await_resume": true,
		"input": []map[string]interface{}{
			{"parts": []map[string]string{{"content": decisionJSON}}},
		},
	}
	data, _ := json.Marshal(body)
	resp, err := http.Post(fmt.Sprintf("http://%s/runs/%s", addr, runID), "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("[warn] resume: %v", err)
		return ""
	}
	defer resp.Body.Close()
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	return result["status"]
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

func waitForPendingRequest(gate *PermissionGate, timeout time.Duration) *PermissionRequest {
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			return nil
		default:
			reqs := gate.PendingRequests()
			if len(reqs) > 0 {
				return &reqs[0]
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstr(s, substr))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
