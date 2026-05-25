package launcher

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/server/adka2a"
	adksession "google.golang.org/adk/session"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// A2AConfig holds the configuration for the A2A JSON-RPC handler.
type A2AConfig struct {
	Agent          agent.Agent
	SessionService adksession.Service
	AppName        string
	Logger         logr.Logger
	Auditor        audit.Emitter
	BridgeMetrics  BridgeMetrics

	// BeforeExecute is called before each A2A execution with the request context.
	// The context already contains the UserIdentity from auth middleware.
	BeforeExecute func(ctx context.Context) (context.Context, error)
}

func (c A2AConfig) validate() error { //nolint:gocritic // hugeParam: value copy intentional for validation
	if c.Agent == nil {
		return fmt.Errorf("agent is required")
	}
	if c.SessionService == nil {
		return fmt.Errorf("session service is required")
	}
	if c.AppName == "" {
		return fmt.Errorf("app name is required")
	}
	return nil
}

func (c A2AConfig) logger() logr.Logger { //nolint:gocritic // hugeParam: value copy intentional
	if c.Logger.GetSink() != nil {
		return c.Logger
	}
	return logr.Discard()
}

// NewA2AHandler creates an http.Handler that serves the A2A JSON-RPC protocol.
// It wraps the ADK executor in the a2a-go JSON-RPC transport layer.
// The handler respects context cancellation for graceful shutdown.
func NewA2AHandler(cfg A2AConfig) (http.Handler, error) { //nolint:gocritic // hugeParam: called once at startup
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid A2A config: %w", err)
	}

	log := cfg.logger().WithValues("component", "a2a-launcher")

	execCfg := adka2a.ExecutorConfig{
		RunnerConfig: runner.Config{
			AppName:           cfg.AppName,
			Agent:             cfg.Agent,
			SessionService:    cfg.SessionService,
			AutoCreateSession: true,
		},
		BeforeExecuteCallback: buildBeforeExecuteCallback(cfg.BeforeExecute, cfg.Auditor),
		AfterExecuteCallback:  buildAfterExecuteCallback(log, cfg.Auditor),
		GenAIPartConverter:    buildStreamingPartConverter(),
		OutputMode:            adka2a.OutputArtifactPerEvent,
	}

	inner := adka2a.NewExecutor(execCfg)
	executor := NewStreamingExecutor(inner, log, cfg.BridgeMetrics)
	reqHandler := a2asrv.NewHandler(executor)
	httpHandler := a2asrv.NewJSONRPCHandler(reqHandler)

	return httpHandler, nil
}

// buildBeforeExecuteCallback wraps the user-supplied callback and emits an
// audit event when an A2A task starts (AU-2 compliance).
// It also injects CreateContext so the decorator can enrich session creation.
func buildBeforeExecuteCallback(userCb func(ctx context.Context) (context.Context, error), auditor audit.Emitter) adka2a.BeforeExecuteCallback {
	return func(ctx context.Context, reqCtx *a2asrv.RequestContext) (context.Context, error) {
		user := auth.UserIdentityFromContext(ctx)
		username := ""
		if user != nil {
			username = user.Username
		}

		if auditor != nil {
			detail := map[string]string{"method": resolveA2AMethod(ctx)}
			if reqCtx != nil {
				detail["task_id"] = string(reqCtx.TaskID)
			}
			auditor.Emit(ctx, &audit.Event{
				Type:   audit.EventA2ATaskStarted,
				UserID: username,
				Detail: detail,
			})

			triageDetail := map[string]string{
				"persona": resolvePersona(user),
			}
			if reqCtx != nil {
				triageDetail["task_id"] = string(reqCtx.TaskID)
				triageDetail["session_id"] = reqCtx.ContextID
			}
			auditor.Emit(ctx, &audit.Event{
				Type:   audit.EventTriageStarted,
				UserID: username,
				Detail: triageDetail,
			})
		}

		// Inject session creation context for the ServiceDecorator.
		// The decorator reads this to build CreateConfig with task/user metadata.
		// SessionID = ContextID because ADK maps A2A ContextID to ADK session ID
		// (see adka2a.Executor.prepareSession). The callback for af_create_rr
		// reads SessionID to drive deferred CRD materialization (G6).
		if reqCtx != nil {
			sc := &session.CreateContext{
				TaskID:    string(reqCtx.TaskID),
				SessionID: reqCtx.ContextID,
			}
			ctx = session.WithCreateContext(ctx, sc)
		}

		if userCb != nil {
			return userCb(ctx)
		}
		return ctx, nil
	}
}

// buildAfterExecuteCallback logs task completion with structured context for
// SRE observability and emits audit events (AU-2 compliance).
// Issue #1189 AC 12: enriches EventA2ATaskCompleted/Failed with rr_name and
// rr_namespace if af_create_rr populated the shared CreateContext during the task.
func buildAfterExecuteCallback(log logr.Logger, auditor audit.Emitter) adka2a.AfterExecuteCallback {
	return func(ctx adka2a.ExecutorContext, finalEvent *a2a.TaskStatusUpdateEvent, err error) error {
		user := auth.UserIdentityFromContext(ctx)
		username := ""
		if user != nil {
			username = user.Username
		}

		taskID := ""
		if finalEvent != nil {
			taskID = string(finalEvent.TaskID)
		}

		if err != nil {
			log.Error(nil, "a2a task execution failed",
				"error", security.RedactError(err),
				"user", username,
				"task_id", taskID,
			)
			if auditor != nil {
				detail := map[string]string{
					"task_id": taskID,
					"error":   security.RedactError(err),
				}
				enrichRRDetail(ctx, detail)
				auditor.Emit(ctx, &audit.Event{
					Type:   audit.EventA2ATaskFailed,
					UserID: username,
					Detail: detail,
				})
			}
			// Return nil — the framework has already produced the TaskStateFailed
			// status event. Returning an error here would prevent it from being
			// written to the client queue (ARCH-3 verification).
			return nil
		} else if auditor != nil {
			detail := map[string]string{"task_id": taskID}
			enrichRRDetail(ctx, detail)
			auditor.Emit(ctx, &audit.Event{
				Type:   audit.EventA2ATaskCompleted,
				UserID: username,
				Detail: detail,
			})

			triageOutcome := "no_issue_found"
			if detail["rr_name"] != "" {
				triageOutcome = "rr_created"
			}
			auditor.Emit(ctx, &audit.Event{
				Type:   audit.EventTriageCompleted,
				UserID: username,
				Detail: map[string]string{
					"task_id":       taskID,
					"triage_outcome": triageOutcome,
				},
			})
		}
		return nil
	}
}

// enrichRRDetail adds rr_name and rr_namespace to the detail map if the
// shared CreateContext was populated by the AfterToolCallback during the task.
func enrichRRDetail(ctx context.Context, detail map[string]string) {
	sc := session.CreateContextFromContext(ctx)
	if sc != nil && sc.RRName != "" {
		detail["rr_name"] = sc.RRName
		detail["rr_namespace"] = sc.RRNamespace
	}
}

// resolvePersona maps the authenticated user's group membership to the
// OpenAPI persona enum used in triage audit events.
func resolvePersona(user *auth.UserIdentity) string {
	if user == nil {
		return "sre"
	}
	for _, g := range user.Groups {
		switch g {
		case "sre":
			return "sre"
		case "ai-orchestrator":
			return "orchestrator"
		case "cicd":
			return "cicd"
		case "observability":
			return "dashboard"
		case "l3-audit":
			return "audit"
		case "remediation-approver":
			return "approver"
		}
	}
	return "sre"
}

// resolveA2AMethod maps the a2asrv CallContext method name to the corresponding
// A2A JSON-RPC method string for audit events (AU-2/AU-3 compliance).
func resolveA2AMethod(ctx context.Context) string {
	callCtx, ok := a2asrv.CallContextFrom(ctx)
	if !ok {
		return "message/send"
	}
	switch callCtx.Method() {
	case "OnSendMessageStream":
		return "message/stream"
	case "OnSendMessage":
		return "message/send"
	default:
		return "message/send"
	}
}
