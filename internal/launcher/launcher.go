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

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
)

// A2AConfig holds the configuration for the A2A JSON-RPC handler.
type A2AConfig struct {
	Agent          agent.Agent
	SessionService adksession.Service
	AppName        string
	Logger         *slog.Logger

	// BeforeExecute is called before each A2A execution with the request context.
	// The context already contains the UserIdentity from auth middleware.
	BeforeExecute func(ctx context.Context) (context.Context, error)
}

func (c A2AConfig) validate() error {
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

func (c A2AConfig) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// NewA2AHandler creates an http.Handler that serves the A2A JSON-RPC protocol.
// It wraps the ADK executor in the a2a-go JSON-RPC transport layer.
// The handler respects context cancellation for graceful shutdown.
func NewA2AHandler(cfg A2AConfig) (http.Handler, error) {
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
		BeforeExecuteCallback: buildBeforeExecuteCallback(cfg.BeforeExecute),
		AfterExecuteCallback:  buildAfterExecuteCallback(log),
	}

	executor := adka2a.NewExecutor(execCfg)
	reqHandler := a2asrv.NewHandler(executor)
	httpHandler := a2asrv.NewJSONRPCHandler(reqHandler)

	return httpHandler, nil
}

// buildBeforeExecuteCallback wraps the user-supplied callback so that the
// UserIdentity from auth middleware is always present in the executor context.
func buildBeforeExecuteCallback(userCb func(ctx context.Context) (context.Context, error)) adka2a.BeforeExecuteCallback {
	return func(ctx context.Context, _ *a2asrv.RequestContext) (context.Context, error) {
		_ = auth.UserIdentityFromContext(ctx)

		if userCb != nil {
			return userCb(ctx)
		}
		return ctx, nil
	}
}

// buildAfterExecuteCallback logs task completion with structured context for SRE observability.
func buildAfterExecuteCallback(log *slog.Logger) adka2a.AfterExecuteCallback {
	return func(ctx adka2a.ExecutorContext, finalEvent *a2a.TaskStatusUpdateEvent, err error) error {
		if err != nil {
			user := auth.UserIdentityFromContext(ctx)
			username := ""
			if user != nil {
				username = user.Username
			}
			log.ErrorContext(ctx, "a2a task execution failed",
				"error", err,
				"user", username,
			)
		}
		_ = finalEvent
		return nil
	}
}
