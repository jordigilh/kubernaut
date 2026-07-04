package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ratelimit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

func buildMCPHandler(d *handlerDeps) (http.Handler, func() bool, error) {
	var sessFinalizer handler.ISPhaseFinalizer
	var sessInitializer handler.ISSessionInitializer
	if d.SessInfra != nil && d.SessInfra.SessionService != nil {
		sessFinalizer = d.SessInfra.SessionService
		sessInitializer = d.SessInfra.SessionService
	}
	bridgeCfg := &handler.MCPBridgeConfig{
		K8sClient:             d.Backends.K8sClient(),
		TypedClient:           d.Backends.TypedClient(),
		Namespace:             d.Cfg.Session.Namespace,
		KAMCPClient:           d.Backends.MCPClient,
		KADedicatedClient:     d.Backends.DedicatedClient,
		InvestigationRegistry: d.Backends.InvestigationRegistry,
		DSClient:              d.Backends.DSClient,
		PromClient:            d.Backends.PromClient,
		Triager:               d.Backends.Triager,
		Authorizer:            d.Authorizer,
		Auditor:               d.Auditor,
		Logger:                d.Logger.WithName("bridge"),
		Metrics:               bridgeMetricsFrom(d.MetricsReg),
		ToolTimeout:           d.Cfg.MCP.ToolTimeout,
		ToolTimeouts:          d.Cfg.MCP.ToolTimeouts,
		MaxConcurrentTools:    10,
		UserLimiter:           d.UserLimiter,
		SessionFinalizer:      sessFinalizer,
		SessionInitializer:    sessInitializer,
		InteractiveEnabled:    d.Cfg.Interactive.Enabled,
		ActiveContextRegistry: d.ActiveCtxRegistry,
		RESTMapper:            d.Backends.Mapper,
	}

	mcpSessionTimeout := d.Cfg.MCP.SessionIdleTimeout
	if mcpSessionTimeout == 0 {
		mcpSessionTimeout = 30 * time.Minute
	}
	h, err := handler.NewMCPHandler(handler.MCPConfig{
		ServerName:     "kubernaut-apifrontend",
		ServerVersion:  version(),
		Enabled:        d.Cfg.MCP.Enabled,
		Bridge:         bridgeCfg,
		Auditor:        d.Auditor,
		SessionTimeout: mcpSessionTimeout,
	})
	if err != nil {
		return nil, nil, err
	}

	depsReady := handler.AllReady(
		d.Backends.KAClient.Healthy,
		d.Backends.DSResilientTransport.Healthy,
		d.Backends.FleetReady, // #1553, ADR-068: fail closed on Fleet unreachability
	)
	return h, depsReady, nil
}

// buildA2AHandler creates the A2A JSON-RPC handler when an LLM provider is
// configured. Returns a 501 stub when provider is empty, preserving backward
// compatibility for deployments that don't set it.
//
// The LLM model and transport chain are built once at startup and are NOT
// reloaded when the ConfigMap changes. Changes to agent.llm fields require
// a pod restart (consistent with KA's LLM wiring pattern).
func buildA2AHandler(ctx context.Context, d *handlerDeps) (http.Handler, error) {
	if d.Cfg.Agent.LLM.Provider == "" {
		d.Logger.Info("LLM provider not configured — A2A handler returns 501")
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "A2A not configured", http.StatusNotImplemented)
		}), nil
	}

	llmModel, err := launcher.NewModelFromConfig(ctx, d.Cfg.Agent.LLM)
	if err != nil {
		return nil, fmt.Errorf("create LLM model: %w", err)
	}
	warnIfUnsupportedLLMTransport(d)

	sessionSvcForAgent := sessionServiceForAgent(d)
	rootAgent, _, err := agentpkg.NewRootAgent(buildRootAgentConfig(d, llmModel, sessionSvcForAgent))
	if err != nil {
		return nil, fmt.Errorf("create root agent: %w", err)
	}

	h, err := launcher.NewA2AHandler(buildA2AConfig(d, rootAgent, sessionSvcForAgent))
	if err != nil {
		return nil, fmt.Errorf("create A2A handler: %w", err)
	}

	d.Logger.Info("A2A handler wired with LLM backend",
		"provider", d.Cfg.Agent.LLM.Provider,
		"model", d.Cfg.Agent.LLM.Model,
	)
	return h, nil
}

// warnIfUnsupportedLLMTransport logs a warning when mTLS/OAuth2 transport
// config is set for a provider whose upstream ADK wrapper cannot apply it
// (blocked by issue #1342).
func warnIfUnsupportedLLMTransport(d *handlerDeps) {
	hasCustomTransport := d.Cfg.Agent.LLM.TLSCaFile != "" || d.Cfg.Agent.LLM.OAuth2.Enabled
	if hasCustomTransport && (d.Cfg.Agent.LLM.Provider == types.LLMProviderVertexAI || d.Cfg.Agent.LLM.Provider == types.LLMProviderAnthropic) {
		d.Logger.Info("WARNING: mTLS/OAuth2 transport config is set but CANNOT be applied to " + d.Cfg.Agent.LLM.Provider +
			" — upstream ADK wrapper lacks HTTP client injection (blocked by issue #1342)")
	}
}

// sessionServiceForAgent returns the CRD-backed session service when session
// infrastructure is available, or nil otherwise.
func sessionServiceForAgent(d *handlerDeps) *session.CRDSessionService {
	if d.SessInfra != nil {
		return d.SessInfra.SessionService
	}
	return nil
}

// buildRootAgentConfig assembles the AgentConfig passed to agentpkg.NewRootAgent.
func buildRootAgentConfig(d *handlerDeps, llmModel model.LLM, sessionSvcForAgent *session.CRDSessionService) agentpkg.AgentConfig {
	return agentpkg.AgentConfig{
		Instruction:           agentpkg.BuildInstruction(d.Cfg.Session.Namespace),
		InstructionProvider:   agentpkg.NewInstructionProvider(d.Cfg.Session.Namespace),
		LLMModel:              llmModel,
		Namespace:             d.Cfg.Session.Namespace,
		K8sClient:             d.Backends.K8sClient(),
		TypedClient:           d.Backends.TypedClient(),
		DSClient:              d.Backends.DSClient,
		MCPClient:             d.Backends.MCPClient,
		DedicatedClient:       d.Backends.DedicatedClient,
		InvestigationRegistry: d.Backends.InvestigationRegistry,
		Pool:                  d.Backends.Pool,
		Authorizer:            d.Authorizer,
		Auditor:               d.Auditor,
		Triager:               d.Backends.Triager,
		RESTMapper:            d.Backends.Mapper,
		SessionService:        sessionSvcForAgent,
		ToolCallsTotal:        d.MetricsReg.ToolCallsTotal,
		ToolCallDuration:      d.MetricsReg.ToolCallDuration,
		UserLimiter:           d.UserLimiter,
		ActiveContextRegistry: d.ActiveCtxRegistry,
		InteractiveEnabled:    d.Cfg.Interactive.Enabled,
		PromClient:            d.Backends.PromClient,
		FleetReaderFactory:    d.Backends.FleetReaderFactory,
		ClusterRegistry:       d.Backends.FleetClusterRegistry,
	}
}

// buildA2AConfig assembles the A2AConfig passed to launcher.NewA2AHandler.
func buildA2AConfig(d *handlerDeps, rootAgent agent.Agent, sessionSvcForAgent *session.CRDSessionService) launcher.A2AConfig {
	var sessionSvc adksession.Service
	if d.SessInfra != nil && d.SessInfra.SessionService != nil {
		sessionSvc = session.NewServiceDecorator(d.SessInfra.SessionService)
	} else {
		sessionSvc = adksession.InMemoryService()
	}

	return launcher.A2AConfig{
		Agent:               rootAgent,
		SessionService:      sessionSvc,
		AppName:             "kubernaut-apifrontend",
		Logger:              d.Logger.WithName("a2a-launcher"),
		Auditor:             d.Auditor,
		BridgeMetrics:       d.MetricsReg,
		SessionPhaseUpdater: sessionSvcForAgent,
		SessionInterceptor: launcher.NewSessionInterceptor(
			d.ActiveCtxRegistry, d.Logger.WithName("session-interceptor"),
		),
		LLMSemaphore: ratelimit.NewLLMSemaphore(d.Cfg.RateLimit.MaxConcurrentSessions),
	}
}
