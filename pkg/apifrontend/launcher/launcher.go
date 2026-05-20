package launcher

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

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
	Logger         *slog.Logger
	Auditor        audit.Emitter

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

func (c A2AConfig) logger() *slog.Logger { //nolint:gocritic // hugeParam: value copy intentional
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// NewA2AHandler creates an http.Handler that serves the A2A JSON-RPC protocol.
// It wraps the ADK executor in the a2a-go JSON-RPC transport layer.
// The handler respects context cancellation for graceful shutdown.
func NewA2AHandler(cfg A2AConfig) (http.Handler, error) { //nolint:gocritic // hugeParam: called once at startup
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid A2A config: %w", err)
	}

	log := cfg.logger().With("component", "a2a-launcher")

	execCfg := adka2a.ExecutorConfig{
		RunnerConfig: runner.Config{
			AppName:           cfg.AppName,
			Agent:             cfg.Agent,
			SessionService:    cfg.SessionService,
			AutoCreateSession: true,
		},
		BeforeExecuteCallback: buildBeforeExecuteCallback(cfg.BeforeExecute, cfg.Auditor),
		AfterExecuteCallback:  buildAfterExecuteCallback(log, cfg.Auditor),
		GenAIPartConverter:    buildPartConverter(),
		OutputMode:            adka2a.OutputArtifactPerEvent,
	}

	executor := adka2a.NewExecutor(execCfg)
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
			detail := map[string]string{"method": "message/send"}
			if reqCtx != nil {
				detail["task_id"] = string(reqCtx.TaskID)
			}
			auditor.Emit(ctx, &audit.Event{
				Type:   audit.EventA2ATaskStarted,
				UserID: username,
				Detail: detail,
			})
			auditor.Emit(ctx, &audit.Event{
				Type:   audit.EventTriageStarted,
				UserID: username,
				Detail: detail,
			})
		}

		// Inject session creation context for the ServiceDecorator.
		// The decorator reads this to build CreateConfig with task/user metadata.
		if reqCtx != nil {
			sc := &session.CreateContext{
				TaskID: string(reqCtx.TaskID),
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
func buildAfterExecuteCallback(log *slog.Logger, auditor audit.Emitter) adka2a.AfterExecuteCallback {
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
			log.ErrorContext(ctx, "a2a task execution failed",
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
			auditor.Emit(ctx, &audit.Event{
				Type:   audit.EventTriageCompleted,
				UserID: username,
				Detail: map[string]string{"task_id": taskID},
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
