package enforcement

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
)

func TestBudgetEnforcement(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	var auditEvents []*AuditEvent
	var mu sync.Mutex
	auditSink := func(ctx context.Context, event *AuditEvent) error {
		mu.Lock()
		defer mu.Unlock()
		auditEvents = append(auditEvents, event)
		return nil
	}

	budget := BudgetConfig{
		MaxToolCallsPerTool: 3,
		MaxTotalToolCalls:   5,
		MaxRepeatedFailures: 2,
		ExemptPrefixes:      []string{"todo_"},
	}

	layer := New("test-correlation-001", budget, nil, auditSink, logger)

	mockHandler := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"status":"ok"}`), nil
	}

	layer.RegisterHandler("kubectl_get", mockHandler)
	layer.RegisterHandler("kubectl_list_events", mockHandler)
	layer.RegisterHandler("todo_write", mockHandler)

	wrapped := layer.WrapForRuntime()
	ctx := context.Background()

	t.Run("tool calls within budget succeed", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			result, err := wrapped["kubectl_get"](ctx, json.RawMessage(`{"kind":"Pod"}`))
			if err != nil {
				t.Fatalf("call %d: unexpected error: %v", i, err)
			}
			if string(result) != `{"status":"ok"}` {
				t.Fatalf("call %d: unexpected result: %s", i, result)
			}
		}
	})

	t.Run("per-tool limit triggers rejection", func(t *testing.T) {
		_, err := wrapped["kubectl_get"](ctx, json.RawMessage(`{"kind":"Pod"}`))
		if err == nil {
			t.Fatal("expected per-tool limit error, got nil")
		}
		t.Logf("correctly rejected: %v", err)
	})

	t.Run("total budget triggers rejection", func(t *testing.T) {
		layer.Reset()
		tools := []string{"kubectl_get", "kubectl_list_events"}
		for i := 0; i < budget.MaxTotalToolCalls; i++ {
			tool := tools[i%len(tools)]
			_, err := wrapped[tool](ctx, json.RawMessage(fmt.Sprintf(`{"iter":%d}`, i)))
			if err != nil {
				t.Fatalf("call %d (%s) should succeed (within budget): %v", i, tool, err)
			}
		}

		_, err := wrapped["kubectl_get"](ctx, json.RawMessage(`{"overflow":true}`))
		if err == nil {
			t.Fatal("expected total budget error, got nil")
		}
		t.Logf("correctly rejected: %v", err)
	})

	t.Run("exempt tools bypass budgets", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			_, err := wrapped["todo_write"](ctx, json.RawMessage(`{"task":"plan"}`))
			if err != nil {
				t.Fatalf("exempt tool call %d: unexpected error: %v", i, err)
			}
		}
	})

	t.Run("audit events emitted for all calls", func(t *testing.T) {
		mu.Lock()
		count := len(auditEvents)
		mu.Unlock()
		if count == 0 {
			t.Fatal("expected audit events, got none")
		}
		t.Logf("total audit events: %d", count)

		var successCount, failureCount int
		mu.Lock()
		for _, e := range auditEvents {
			if e.EventOutcome == "success" {
				successCount++
			} else {
				failureCount++
			}
		}
		mu.Unlock()
		t.Logf("success: %d, failure: %d", successCount, failureCount)
		if failureCount < 2 {
			t.Fatalf("expected at least 2 failure events, got %d", failureCount)
		}
	})

	t.Run("reset clears counters", func(t *testing.T) {
		layer.Reset()
		_, err := wrapped["kubectl_get"](ctx, json.RawMessage(`{"kind":"Pod"}`))
		if err != nil {
			t.Fatalf("after reset, call should succeed: %v", err)
		}
	})
}

func TestShadowFeed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	var shadowSteps []struct {
		Index int
		Tool  string
	}
	var mu sync.Mutex

	shadowFeed := func(ctx context.Context, stepIndex int, toolName string, content string) error {
		mu.Lock()
		defer mu.Unlock()
		shadowSteps = append(shadowSteps, struct {
			Index int
			Tool  string
		}{stepIndex, toolName})
		return nil
	}

	layer := New("shadow-test-001", DefaultBudgetConfig(), shadowFeed, nil, logger)

	layer.RegisterHandler("kubectl_get", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"kind":"Pod","status":"Running"}`), nil
	})
	layer.RegisterHandler("prometheus_query", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`[{"metric":"cpu","value":0.85}]`), nil
	})

	wrapped := layer.WrapForRuntime()
	ctx := context.Background()

	wrapped["kubectl_get"](ctx, json.RawMessage(`{}`))
	wrapped["prometheus_query"](ctx, json.RawMessage(`{}`))
	wrapped["kubectl_get"](ctx, json.RawMessage(`{}`))

	mu.Lock()
	defer mu.Unlock()

	if len(shadowSteps) != 3 {
		t.Fatalf("expected 3 shadow steps, got %d", len(shadowSteps))
	}

	if shadowSteps[0].Tool != "kubectl_get" || shadowSteps[0].Index != 1 {
		t.Errorf("step 0: expected kubectl_get/1, got %s/%d", shadowSteps[0].Tool, shadowSteps[0].Index)
	}
	if shadowSteps[1].Tool != "prometheus_query" || shadowSteps[1].Index != 2 {
		t.Errorf("step 1: expected prometheus_query/2, got %s/%d", shadowSteps[1].Tool, shadowSteps[1].Index)
	}
	if shadowSteps[2].Tool != "kubectl_get" || shadowSteps[2].Index != 3 {
		t.Errorf("step 2: expected kubectl_get/3, got %s/%d", shadowSteps[2].Tool, shadowSteps[2].Index)
	}

	t.Log("shadow agent received all tool results in correct order")
}

func TestFailingToolRecordsFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	var auditEvents []*AuditEvent
	var mu sync.Mutex
	auditSink := func(ctx context.Context, event *AuditEvent) error {
		mu.Lock()
		defer mu.Unlock()
		auditEvents = append(auditEvents, event)
		return nil
	}

	layer := New("failure-test-001", DefaultBudgetConfig(), nil, auditSink, logger)
	layer.RegisterHandler("kubectl_get", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return nil, fmt.Errorf("connection refused")
	})

	wrapped := layer.WrapForRuntime()
	ctx := context.Background()

	_, err := wrapped["kubectl_get"](ctx, json.RawMessage(`{"kind":"Pod"}`))
	if err == nil {
		t.Fatal("expected error from failing tool")
	}

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, e := range auditEvents {
		if e.EventOutcome == "failure" && e.Data["tool_name"] == "kubectl_get" {
			found = true
			t.Logf("audit captured failure: %v", e.Data["error"])
		}
	}
	if !found {
		t.Fatal("expected failure audit event for kubectl_get")
	}
}

func TestWrapForRuntimeReturnsAllHandlers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	layer := New("wrap-test", DefaultBudgetConfig(), nil, nil, logger)

	tools := []string{"kubectl_get", "kubectl_list_events", "prometheus_query", "submit_result"}
	for _, name := range tools {
		layer.RegisterHandler(name, func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		})
	}

	wrapped := layer.WrapForRuntime()

	if len(wrapped) != len(tools) {
		t.Fatalf("expected %d wrapped handlers, got %d", len(tools), len(wrapped))
	}

	for _, name := range tools {
		if _, ok := wrapped[name]; !ok {
			t.Errorf("missing wrapped handler for %s", name)
		}
	}

	t.Log("all handlers wrapped successfully for runtime injection")
}
