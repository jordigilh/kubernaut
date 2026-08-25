package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/codeany-ai/open-agent-sdk-go/agent"
	"github.com/codeany-ai/open-agent-sdk-go/hooks"
	"github.com/codeany-ai/open-agent-sdk-go/types"
	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/spikes/oas-runtime/internal/acp"
)

// Config holds OAS Runtime configuration.
type Config struct {
	// Model is the LLM model name (e.g., "sonnet-4-6", "gpt-4o").
	Model string

	// APIKey for the LLM provider.
	APIKey string

	// BaseURL overrides the default API endpoint (for OpenAI-compatible providers).
	BaseURL string

	// MCPServers maps MCP server names to their connection config.
	MCPServers map[string]types.MCPServerConfig

	// SystemPrompt is prepended to every run's instructions.
	SystemPrompt string

	// MaxTurns limits the agent's reasoning loop depth.
	MaxTurns int

	// PermissionHook is called when the agent requests tool permission.
	// Return true to allow, false to deny.
	PermissionHook func(toolName string, input map[string]interface{}) bool

	Logger *slog.Logger
}

// Executor implements acp.AgentExecutor using open-agent-sdk-go.
type Executor struct {
	config Config
	logger *slog.Logger

	mu   sync.RWMutex
	runs map[string]*runState
}

type runState struct {
	run    *acp.Run
	cancel context.CancelFunc
}

func NewExecutor(cfg Config) *Executor {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{
		config: cfg,
		logger: logger,
		runs:   make(map[string]*runState),
	}
}

func (e *Executor) Manifest(name string) (*acp.AgentManifest, error) {
	return &acp.AgentManifest{
		Name:        name,
		Description: "Kubernaut OAS Runtime agent — executes investigation phases via open-agent-sdk-go",
		InputContentTypes:  []string{"text/plain", "application/json"},
		OutputContentTypes: []string{"text/plain", "application/json"},
		Metadata: &acp.Metadata{
			ProgrammingLang: "Go",
			Framework:       "open-agent-sdk-go",
			License:         "Apache-2.0",
			Capabilities: []acp.Capability{
				{Name: "Investigation", Description: "Execute Kubernetes investigation phases with MCP tools"},
				{Name: "Streaming", Description: "Stream agent reasoning and tool calls via SSE"},
				{Name: "PermissionGating", Description: "Gate tool calls for orchestrator approval"},
			},
		},
	}, nil
}

func (e *Executor) ExecuteSync(ctx context.Context, req acp.RunCreateRequest) (*acp.Run, error) {
	a, err := e.createAgent()
	if err != nil {
		return nil, fmt.Errorf("creating agent: %w", err)
	}
	defer a.Close()

	if err := a.Init(ctx); err != nil {
		return nil, fmt.Errorf("initializing agent MCP connections: %w", err)
	}

	prompt := extractPromptText(req.Input)
	runID := uuid.New().String()
	now := time.Now().UTC()

	run := &acp.Run{
		AgentName: req.AgentName,
		RunID:     runID,
		Status:    acp.RunStatusInProgress,
		Output:    []acp.Message{},
		CreatedAt: now,
	}

	e.storeRun(runID, run, nil)

	result, err := a.Prompt(ctx, prompt)
	if err != nil {
		run.Status = acp.RunStatusFailed
		run.Error = &acp.Error{Code: "server_error", Message: err.Error()}
		finished := time.Now().UTC()
		run.FinishedAt = &finished
		e.storeRun(runID, run, nil)
		return run, nil
	}

	finished := time.Now().UTC()
	run.Status = acp.RunStatusCompleted
	run.FinishedAt = &finished
	run.Output = []acp.Message{
		{
			Role: "agent",
			Parts: []acp.MessagePart{
				{
					ContentType: "text/plain",
					Content:     result.Text,
				},
			},
		},
	}

	if result.Cost > 0 {
		e.logger.Info("run completed",
			"run_id", runID,
			"input_tokens", result.Usage.InputTokens,
			"output_tokens", result.Usage.OutputTokens,
			"cost_usd", result.Cost,
		)
	}

	e.storeRun(runID, run, nil)
	return run, nil
}

func (e *Executor) ExecuteStream(ctx context.Context, req acp.RunCreateRequest) (<-chan acp.Event, error) {
	a, err := e.createAgent()
	if err != nil {
		return nil, fmt.Errorf("creating agent: %w", err)
	}

	if err := a.Init(ctx); err != nil {
		a.Close()
		return nil, fmt.Errorf("initializing agent MCP connections: %w", err)
	}

	prompt := extractPromptText(req.Input)
	runID := uuid.New().String()
	now := time.Now().UTC()

	run := &acp.Run{
		AgentName: req.AgentName,
		RunID:     runID,
		Status:    acp.RunStatusCreated,
		Output:    []acp.Message{},
		CreatedAt: now,
	}

	streamCtx, cancel := context.WithCancel(ctx)
	e.storeRun(runID, run, cancel)

	out := make(chan acp.Event, 64)

	go func() {
		defer close(out)
		defer a.Close()
		defer cancel()

		run.Status = acp.RunStatusInProgress
		out <- acp.Event{
			Type: acp.EventRunCreated,
			Run:  copyRun(run),
		}
		out <- acp.Event{
			Type: acp.EventRunInProgress,
			Run:  copyRun(run),
		}

		sdkEvents, errs := a.Query(streamCtx, prompt)

		for event := range sdkEvents {
			acpEvents := sdkEventToACP(event, run)
			for _, acpEvent := range acpEvents {
				out <- *acpEvent
			}
		}

		if err := <-errs; err != nil {
			run.Status = acp.RunStatusFailed
			run.Error = &acp.Error{Code: "server_error", Message: err.Error()}
			finished := time.Now().UTC()
			run.FinishedAt = &finished
			out <- acp.Event{Type: acp.EventRunFailed, Run: copyRun(run)}
		} else {
			run.Status = acp.RunStatusCompleted
			finished := time.Now().UTC()
			run.FinishedAt = &finished
			out <- acp.Event{Type: acp.EventRunCompleted, Run: copyRun(run)}
		}

		e.storeRun(runID, run, nil)
	}()

	return out, nil
}

func (e *Executor) Resume(ctx context.Context, runID string, req acp.RunResumeRequest) (*acp.Run, error) {
	e.mu.RLock()
	rs, ok := e.runs[runID]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("run %q not found", runID)
	}
	if rs.run.Status != acp.RunStatusAwaiting {
		return nil, fmt.Errorf("run %q is not in awaiting state (current: %s)", runID, rs.run.Status)
	}

	rs.run.Status = acp.RunStatusInProgress
	e.storeRun(runID, rs.run, rs.cancel)
	return rs.run, nil
}

func (e *Executor) Cancel(ctx context.Context, runID string) error {
	e.mu.RLock()
	rs, ok := e.runs[runID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("run %q not found", runID)
	}

	if rs.cancel != nil {
		rs.cancel()
	}

	rs.run.Status = acp.RunStatusCancelled
	finished := time.Now().UTC()
	rs.run.FinishedAt = &finished
	e.storeRun(runID, rs.run, nil)
	return nil
}

func (e *Executor) GetRun(ctx context.Context, runID string) (*acp.Run, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rs, ok := e.runs[runID]
	if !ok {
		return nil, fmt.Errorf("run %q not found", runID)
	}
	return rs.run, nil
}

func (e *Executor) createAgent() (*agent.Agent, error) {
	opts := agent.Options{
		Model:      e.config.Model,
		APIKey:     e.config.APIKey,
		MCPServers: e.config.MCPServers,
	}

	if e.config.BaseURL != "" {
		opts.BaseURL = e.config.BaseURL
	}

	if e.config.SystemPrompt != "" {
		opts.SystemPrompt = e.config.SystemPrompt
	}

	if e.config.MaxTurns > 0 {
		opts.MaxTurns = e.config.MaxTurns
	}

	if e.config.PermissionHook != nil {
		opts.Hooks = hooks.HookConfig{
			PermissionRequest: []hooks.HookRule{{
				Matcher: "*",
				Hooks: []hooks.HookFn{
					func(ctx context.Context, tool string, input map[string]interface{}) (string, error) {
						if e.config.PermissionHook(tool, input) {
							return "", nil // allow
						}
						return "denied by orchestrator permission ceiling", nil
					},
				},
			}},
			PreToolUse: []hooks.HookRule{{
				Matcher: "*",
				Hooks: []hooks.HookFn{
					func(ctx context.Context, tool string, input map[string]interface{}) (string, error) {
						inputJSON, _ := json.Marshal(input)
						e.logger.Info("tool invocation",
							"tool", tool,
							"input_size", len(inputJSON),
						)
						return "", nil
					},
				},
			}},
		}
	}

	return agent.New(opts), nil
}

func (e *Executor) storeRun(runID string, run *acp.Run, cancel context.CancelFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if existing, ok := e.runs[runID]; ok && cancel == nil {
		cancel = existing.cancel
	}
	e.runs[runID] = &runState{run: run, cancel: cancel}
}

func extractPromptText(messages []acp.Message) string {
	var parts []string
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if part.Content != "" {
				parts = append(parts, part.Content)
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// sdkEventToACP converts an OAS Go SDK streaming event into zero or more ACP events.
// The SDK streams SDKMessage values with Type: assistant, progress, or result.
// Tool calls and results are embedded as ContentBlocks inside assistant messages.
func sdkEventToACP(event types.SDKMessage, run *acp.Run) []*acp.Event {
	var events []*acp.Event

	switch event.Type {
	case types.MessageTypeAssistant:
		if event.Message == nil {
			return nil
		}
		for _, block := range event.Message.Content {
			switch block.Type {
			case types.ContentBlockText:
				if block.Text == "" {
					continue
				}
				part := acp.MessagePart{
					ContentType: "text/plain",
					Content:     block.Text,
				}
				run.Output = append(run.Output, acp.Message{
					Role:  "agent",
					Parts: []acp.MessagePart{part},
				})
				events = append(events, &acp.Event{
					Type: acp.EventMessagePart,
					Part: &part,
				})

			case types.ContentBlockToolUse:
				inputBytes, _ := json.Marshal(block.Input)
				part := acp.MessagePart{
					ContentType: "application/json",
					Content:     string(inputBytes),
					Metadata: &acp.TrajectoryMetadata{
						Kind:      "trajectory",
						ToolName:  block.Name,
						ToolInput: block.Input,
					},
				}
				events = append(events, &acp.Event{
					Type: acp.EventMessagePart,
					Part: &part,
				})

			case types.ContentBlockToolResult:
				resultText := ""
				if len(block.Content) > 0 {
					resultText = block.Content[0].Text
				}
				outputMap := map[string]interface{}{"result": resultText}
				part := acp.MessagePart{
					ContentType: "application/json",
					Content:     resultText,
					Metadata: &acp.TrajectoryMetadata{
						Kind:       "trajectory",
						ToolOutput: outputMap,
					},
				}
				events = append(events, &acp.Event{
					Type: acp.EventMessagePart,
					Part: &part,
				})
			}
		}

	case types.MessageTypeResult:
		if event.Text != "" {
			part := acp.MessagePart{
				ContentType: "text/plain",
				Content:     event.Text,
			}
			run.Output = append(run.Output, acp.Message{
				Role:  "agent",
				Parts: []acp.MessagePart{part},
			})
			events = append(events, &acp.Event{
				Type: acp.EventMessageDone,
				Message: &acp.Message{
					Role:  "agent",
					Parts: []acp.MessagePart{part},
				},
			})
		}

	case types.MessageTypeProgress:
		if event.Text != "" {
			part := acp.MessagePart{
				ContentType: "text/plain",
				Content:     event.Text,
			}
			events = append(events, &acp.Event{
				Type: acp.EventMessagePart,
				Part: &part,
			})
		}
	}

	return events
}

func copyRun(r *acp.Run) *acp.Run {
	cp := *r
	cp.Output = make([]acp.Message, len(r.Output))
	copy(cp.Output, r.Output)
	return &cp
}
