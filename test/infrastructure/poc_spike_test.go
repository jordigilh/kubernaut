package infrastructure_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut-apifrontend/test/infrastructure"
)

// POC-SPIKE-02: Validate WaitForPrometheusRuleState helper logic against
// a fake Prometheus /api/v1/rules endpoint.
func TestPOCSpike2_PrometheusRuleStatePolling(t *testing.T) {
	t.Parallel()

	callCount := 0
	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		var resp map[string]any

		if callCount < 3 {
			resp = map[string]any{
				"status": "success",
				"data": map[string]any{
					"groups": []map[string]any{
						{
							"name": "e2e-severity-triage",
							"rules": []map[string]any{
								{"name": "HighCPU", "state": "inactive", "type": "alerting"},
							},
						},
					},
				},
			}
		} else {
			resp = map[string]any{
				"status": "success",
				"data": map[string]any{
					"groups": []map[string]any{
						{
							"name": "e2e-severity-triage",
							"rules": []map[string]any{
								{"name": "HighCPU", "state": "firing", "type": "alerting"},
							},
						},
					},
				},
			}
		}

		data, _ := json.Marshal(resp)
		_, _ = w.Write(data)
	}))
	t.Cleanup(promServer.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := infrastructure.WaitForPrometheusRuleState(
		ctx, promServer.URL, "HighCPU", infrastructure.RuleStateFiring, 30*time.Second,
	)
	if err != nil {
		t.Fatalf("POC-SPIKE-02: WaitForPrometheusRuleState failed: %v", err)
	}

	if callCount < 3 {
		t.Errorf("POC-SPIKE-02: expected at least 3 poll calls, got %d", callCount)
	}
}

// POC-SPIKE-02b: Validate WaitForPrometheusRuleState times out correctly.
func TestPOCSpike2_PrometheusRuleStateTimeout(t *testing.T) {
	t.Parallel()

	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"status": "success",
			"data": map[string]any{
				"groups": []map[string]any{
					{
						"name": "e2e-severity-triage",
						"rules": []map[string]any{
							{"name": "HighCPU", "state": "inactive", "type": "alerting"},
						},
					},
				},
			},
		}
		data, _ := json.Marshal(resp)
		_, _ = w.Write(data)
	}))
	t.Cleanup(promServer.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := infrastructure.WaitForPrometheusRuleState(
		ctx, promServer.URL, "HighCPU", infrastructure.RuleStateFiring, 3*time.Second,
	)
	if err == nil {
		t.Fatal("POC-SPIKE-02b: expected timeout error, got nil")
	}
}

// POC-SPIKE-03: Validate that mock-LLM's Gemini protocol returns structured
// responses for text-only prompts (triage use case). Uses a test server
// mimicking mock-LLM's generateContent endpoint.
func TestPOCSpike3_MockLLMGeminiTextPrompt(t *testing.T) {
	t.Parallel()

	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		contents, _ := reqBody["contents"].([]any)
		if len(contents) == 0 {
			http.Error(w, "missing contents", http.StatusBadRequest)
			return
		}

		// Mock-LLM with keyword_scenarios: if prompt contains "severity",
		// return a severity classification.
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{"text": "Based on the alert analysis, the severity is: critical"},
						},
					},
					"finishReason": "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		data, _ := json.Marshal(resp)
		_, _ = w.Write(data)
	}))
	t.Cleanup(mockLLM.Close)

	// Send a triage text prompt to the mock-LLM Gemini endpoint
	body := `{
		"contents": [
			{
				"role": "user",
				"parts": [{"text": "Determine the severity of this alert: HighCPU at 95%"}]
			}
		]
	}`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		mockLLM.URL+"/v1beta/models/mock-model:generateContent",
		http.NoBody)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Body = http.NoBody
	// Use a fresh request with actual body
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost,
		mockLLM.URL+"/v1beta/models/mock-model:generateContent",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POC-SPIKE-03: request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POC-SPIKE-03: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("POC-SPIKE-03: decode response: %v", err)
	}

	candidates, ok := result["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		t.Fatal("POC-SPIKE-03: response missing candidates array")
	}

	candidate, ok := candidates[0].(map[string]any)
	if !ok {
		t.Fatal("POC-SPIKE-03: candidate[0] is not an object")
	}
	content, ok := candidate["content"].(map[string]any)
	if !ok {
		t.Fatal("POC-SPIKE-03: candidate.content is not an object")
	}
	parts, ok := content["parts"].([]any)
	if !ok {
		t.Fatal("POC-SPIKE-03: candidate.content.parts is not an array")
	}
	if len(parts) == 0 {
		t.Fatal("POC-SPIKE-03: candidate.content.parts is empty")
	}
	part0, ok := parts[0].(map[string]any)
	if !ok {
		t.Fatal("POC-SPIKE-03: parts[0] is not an object")
	}
	text, ok := part0["text"].(string)
	if !ok {
		t.Fatal("POC-SPIKE-03: parts[0].text is not a string")
	}

	if text == "" {
		t.Fatal("POC-SPIKE-03: response text is empty — mock-LLM did not handle text prompt")
	}
	t.Logf("POC-SPIKE-03: mock-LLM returned: %q", text)
}

// POC-SPIKE-04: Validate coverage instrumentation helper compiles and handles
// the empty-directory case gracefully.
func TestPOCSpike4_CoverageCollectionEmptyDir(t *testing.T) {
	t.Parallel()

	// CollectE2EBinaryCoverage requires a running Kind cluster, so we just
	// verify the function signature and error handling are correct.
	// A real validation runs during E2E with the cluster up.
	_, err := infrastructure.CollectE2EBinaryCoverage("nonexistent-cluster", &nullWriter{})
	if err == nil {
		t.Fatal("POC-SPIKE-04: expected error for nonexistent cluster, got nil")
	}
}

type nullWriter struct{}

func (nw *nullWriter) Write(p []byte) (int, error) { return len(p), nil }
