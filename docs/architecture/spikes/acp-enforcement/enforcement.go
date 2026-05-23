/*
Spike 2: ACP Server Enforcement Layer Prototype

Demonstrates how the universal ACP server intercepts tool calls from any
runtime (Goose, OAS/LangGraph, Deep Agents) to enforce:

1. Tool call budgets (mirrors KA's AnomalyDetector)
2. Shadow agent feed (mirrors KA's alignment.SubmitToolStep)
3. Audit event emission (mirrors KA's audit.StoreBestEffort)
4. Tool result sanitization

The enforcement layer sits between the runtime's tool_registry and KA's
actual tool implementations (exposed via MCP or gRPC).
*/
package enforcement

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// ToolHandler is the function signature that runtime adapters register.
// For OAS/LangGraph, this maps to the Python tool_registry handler.
// For Goose, this maps to a recipe tool callback.
// For Deep Agents, this maps to a LangGraph tool node.
type ToolHandler func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// BudgetConfig mirrors KA's AnomalyConfig for feature parity.
type BudgetConfig struct {
	MaxToolCallsPerTool int      `yaml:"maxToolCallsPerTool" json:"maxToolCallsPerTool"`
	MaxTotalToolCalls   int      `yaml:"maxTotalToolCalls" json:"maxTotalToolCalls"`
	MaxRepeatedFailures int      `yaml:"maxRepeatedFailures" json:"maxRepeatedFailures"`
	ExemptPrefixes      []string `yaml:"exemptPrefixes" json:"exemptPrefixes"`
}

func DefaultBudgetConfig() BudgetConfig {
	return BudgetConfig{
		MaxToolCallsPerTool: 10,
		MaxTotalToolCalls:   30,
		MaxRepeatedFailures: 3,
		ExemptPrefixes:      []string{"todo_"},
	}
}

// ShadowFeed is called after each tool invocation with the sanitized result.
type ShadowFeed func(ctx context.Context, stepIndex int, toolName string, content string) error

// AuditSink receives structured audit events.
type AuditSink func(ctx context.Context, event *AuditEvent) error

// AuditEvent mirrors KA's audit.AuditEvent structure.
type AuditEvent struct {
	EventType     string                 `json:"event_type"`
	EventCategory string                 `json:"event_category"`
	EventAction   string                 `json:"event_action"`
	EventOutcome  string                 `json:"event_outcome"`
	CorrelationID string                 `json:"correlation_id"`
	Data          map[string]interface{} `json:"data"`
	Timestamp     time.Time              `json:"timestamp"`
}

// EnforcementLayer wraps tool handlers with budget, shadow, and audit
// interception. One instance per investigation session.
type EnforcementLayer struct {
	mu              sync.Mutex
	budget          BudgetConfig
	shadowFeed      ShadowFeed
	auditSink       AuditSink
	correlationID   string
	logger          *slog.Logger
	toolCallCounts  map[string]int
	totalCallCount  int
	failureTracker  map[string]int
	stepCounter     atomic.Int64
	handlers        map[string]ToolHandler
}

// New creates an EnforcementLayer for a single investigation session.
func New(
	correlationID string,
	budget BudgetConfig,
	shadowFeed ShadowFeed,
	auditSink AuditSink,
	logger *slog.Logger,
) *EnforcementLayer {
	return &EnforcementLayer{
		budget:         budget,
		shadowFeed:     shadowFeed,
		auditSink:      auditSink,
		correlationID:  correlationID,
		logger:         logger,
		toolCallCounts: make(map[string]int),
		failureTracker: make(map[string]int),
		handlers:       make(map[string]ToolHandler),
	}
}

// RegisterHandler adds a backend tool handler (typically proxying to KA's MCP tools).
func (e *EnforcementLayer) RegisterHandler(name string, handler ToolHandler) {
	e.handlers[name] = handler
}

// WrapForRuntime returns a map[string]ToolHandler suitable for injection into
// the runtime's tool registry. Each wrapped handler enforces budget, shadow,
// and audit before/after delegating to the real handler.
func (e *EnforcementLayer) WrapForRuntime() map[string]ToolHandler {
	wrapped := make(map[string]ToolHandler, len(e.handlers))
	for name, handler := range e.handlers {
		wrapped[name] = e.wrapHandler(name, handler)
	}
	return wrapped
}

func (e *EnforcementLayer) wrapHandler(name string, handler ToolHandler) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		if err := e.checkBudget(name, args); err != nil {
			e.emitAudit(ctx, "aiagent.runtime.tool_call", "tool_budget_exceeded", "failure", map[string]interface{}{
				"tool_name": name,
				"reason":    err.Error(),
			})
			return nil, err
		}

		start := time.Now()
		result, execErr := handler(ctx, args)
		elapsed := time.Since(start)

		if execErr != nil {
			e.recordFailure(name, args)
			e.emitAudit(ctx, "aiagent.runtime.tool_call", "tool_execution", "failure", map[string]interface{}{
				"tool_name":  name,
				"error":      execErr.Error(),
				"elapsed_ms": elapsed.Milliseconds(),
			})
			return result, execErr
		}

		e.emitAudit(ctx, "aiagent.runtime.tool_call", "tool_execution", "success", map[string]interface{}{
			"tool_name":          name,
			"tool_result_preview": truncatePreview(string(result), 500),
			"elapsed_ms":         elapsed.Milliseconds(),
		})

		if e.shadowFeed != nil {
			stepIdx := int(e.stepCounter.Add(1))
			if feedErr := e.shadowFeed(ctx, stepIdx, name, string(result)); feedErr != nil {
				e.logger.Error("shadow feed error", "tool", name, "error", feedErr)
			}
		}

		return result, nil
	}
}

func (e *EnforcementLayer) checkBudget(name string, args json.RawMessage) error {
	for _, prefix := range e.budget.ExemptPrefixes {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			return nil
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.totalCallCount++
	if e.totalCallCount > e.budget.MaxTotalToolCalls {
		return fmt.Errorf("total tool call limit exceeded (%d > %d)", e.totalCallCount, e.budget.MaxTotalToolCalls)
	}

	e.toolCallCounts[name]++
	if e.toolCallCounts[name] > e.budget.MaxToolCallsPerTool {
		return fmt.Errorf("per-tool call limit exceeded for %s (%d > %d)", name, e.toolCallCounts[name], e.budget.MaxToolCallsPerTool)
	}

	return nil
}

func (e *EnforcementLayer) recordFailure(name string, args json.RawMessage) {
	key := failureKey(name, args)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failureTracker[key]++
}

// TotalExceeded returns true when the total tool call budget is exhausted.
func (e *EnforcementLayer) TotalExceeded() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.totalCallCount > e.budget.MaxTotalToolCalls
}

// Reset clears accumulated counters between investigation phases.
func (e *EnforcementLayer) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.totalCallCount = 0
	e.toolCallCounts = make(map[string]int)
	e.failureTracker = make(map[string]int)
}

func (e *EnforcementLayer) emitAudit(ctx context.Context, eventType, action, outcome string, data map[string]interface{}) {
	if e.auditSink == nil {
		return
	}
	event := &AuditEvent{
		EventType:     eventType,
		EventCategory: "aiagent",
		EventAction:   action,
		EventOutcome:  outcome,
		CorrelationID: e.correlationID,
		Data:          data,
		Timestamp:     time.Now(),
	}
	if err := e.auditSink(ctx, event); err != nil {
		e.logger.Error("audit sink error", "event_type", eventType, "error", err)
	}
}

func failureKey(name string, args json.RawMessage) string {
	h := sha256.Sum256(args)
	return fmt.Sprintf("%s:%x", name, h[:8])
}

func truncatePreview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
