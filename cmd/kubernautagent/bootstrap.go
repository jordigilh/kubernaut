/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	alignprompt "github.com/jordigilh/kubernaut/internal/kubernautagent/alignment/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	kametrics "github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
)

// buildLLMClients constructs the primary SwappableClient plus any per-phase
// SwappableClient overrides configured in llmRuntime.PhaseModels. Terminates
// the process (os.Exit(1)) on unrecoverable client construction failures,
// matching the original inline main() behavior
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Phase 4f).
func buildLLMClients(cfg *kaconfig.Config, llmRuntime *kaconfig.LLMRuntimeConfig, logger logr.Logger) (*llm.SwappableClient, map[katypes.Phase]*llm.SwappableClient) {
	llmClient, err := buildLLMClientFromConfig(context.Background(), mergeLLMConfig(cfg.AI.LLM, llmRuntime))
	if err != nil {
		logger.Error(err, "failed to create LLM client", "provider", cfg.AI.LLM.Provider)
		os.Exit(1)
	}

	swappable, err := llm.NewSwappableClient(llmClient, llmRuntime.Model, llm.RuntimeParams{
		Temperature:    llmRuntime.Temperature,
		TimeoutSeconds: llmRuntime.TimeoutSeconds,
		MaxRetries:     llmRuntime.MaxRetries,
	})
	if err != nil {
		logger.Error(err, "failed to create swappable LLM client")
		os.Exit(1)
	}

	// #1470: Build per-phase SwappableClients. The resolver is created after
	// the alignment evaluator, so that the PinDecorator can be set at
	// construction time.
	phaseSwappables := make(map[katypes.Phase]*llm.SwappableClient)
	for phaseName, override := range llmRuntime.PhaseModels {
		phaseLLM, phaseRT := llmRuntime.EffectivePhaseConfig(phaseName, cfg.AI.LLM, *llmRuntime)
		phaseClient, phaseErr := buildLLMClientFromConfig(context.Background(), mergeLLMConfig(phaseLLM, &phaseRT))
		if phaseErr != nil {
			logger.Error(phaseErr, "failed to build phase LLM client",
				"phase", phaseName, "model", override.Model)
			os.Exit(1)
		}
		phaseSw, phaseSwErr := llm.NewSwappableClient(phaseClient, phaseRT.Model, llm.RuntimeParams{
			Temperature:    phaseRT.Temperature,
			TimeoutSeconds: phaseRT.TimeoutSeconds,
			MaxRetries:     phaseRT.MaxRetries,
		})
		if phaseSwErr != nil {
			logger.Error(phaseSwErr, "failed to create phase SwappableClient",
				"phase", phaseName)
			os.Exit(1)
		}
		phaseSwappables[katypes.Phase(phaseName)] = phaseSw
		logger.Info("per-phase LLM client initialized",
			"phase", phaseName, "model", phaseRT.Model, "override_model", override.Model)
	}

	return swappable, phaseSwappables
}

// buildAlignmentStack resolves the shadow-agent alignment-check
// configuration and, when enabled, constructs the dedicated (or shared)
// shadow LLM client and wraps the primary LLM/tool registry in
// alignment-aware proxies. Terminates the process on a fail-closed shadow
// client construction failure, matching the original inline main() behavior
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Phase 4f).
func buildAlignmentStack(
	cfg *kaconfig.Config,
	llmRuntime *kaconfig.LLMRuntimeConfig,
	instrumentedLLM llm.Client,
	reg *registry.Registry,
	auditStore audit.AuditStore,
	logger logr.Logger,
) (effectiveLLM llm.Client, effectiveReg registry.ToolRegistry, alignEvaluator *alignment.Evaluator, alignCfg kaconfig.AlignmentCheckConfig) {
	effectiveLLM = instrumentedLLM
	effectiveReg = reg
	alignCfg = resolveAlignmentCheckConfig(cfg)
	if !alignCfg.Enabled {
		return effectiveLLM, effectiveReg, nil, alignCfg
	}

	var shadowClient llm.Client
	if alignCfg.LLM == nil {
		shadowClient = instrumentedLLM
		logger.Error(nil, "shadow agent shares investigation LLM client — shadow requests will compete with primary investigation; configure ai.alignmentCheck.llm for dedicated shadow model")
	} else {
		alignStaticCfg, alignRtCfg := alignCfg.EffectiveLLM(cfg.AI.LLM, *llmRuntime)
		raw, alignErr := buildLLMClientFromConfig(context.Background(), mergeLLMConfig(alignStaticCfg, &alignRtCfg))
		if alignErr != nil {
			logger.Error(alignErr, "alignment check LLM client failed (fail-closed): alignment is enabled but shadow client unavailable")
			os.Exit(1)
		} else {
			shadowClient = llm.NewInstrumentedClient(raw)
			logger.Info("shadow agent using dedicated LLM client", "model", alignRtCfg.Model)
		}
	}
	if shadowClient != nil {
		alignEvaluator = alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
			Timeout:               alignCfg.Timeout,
			MaxStepTokens:         alignCfg.MaxStepTokens,
			MaxRetries:            alignCfg.MaxRetries,
			MaxConversationTokens: alignCfg.GroundingReview.MaxConversationTokens,
		}, alignprompt.SystemPrompt(), alignment.WithLogger(logger), alignment.WithAuditStore(auditStore))
		effectiveLLM = alignment.NewLLMProxy(instrumentedLLM)
		effectiveReg = alignment.NewToolProxy(reg)
		logger.Info("shadow agent alignment check enabled (shadow LLM audit active: request/response events will be emitted per step)")
	}
	return effectiveLLM, effectiveReg, alignEvaluator, alignCfg
}

// investigationRunnerParams groups the dependencies needed to build the
// investigation stack (investigator, session store/manager, ogen server).
// Extracted per AGENTS.md's 8+-param Options-pattern rule.
type investigationRunnerParams struct {
	cfg             *kaconfig.Config
	llmRuntime      *kaconfig.LLMRuntimeConfig
	swappable       *llm.SwappableClient
	phaseSwappables map[katypes.Phase]*llm.SwappableClient
	promptBuilder   *prompt.Builder
	resultParser    *parser.ResultParser
	phaseTools      katypes.PhaseToolMap
	enricher        *enrichment.Enricher
	auditStore      audit.AuditStore
	effectiveLLM    llm.Client
	effectiveReg    registry.ToolRegistry
	alignEvaluator  *alignment.Evaluator
	alignCfg        kaconfig.AlignmentCheckConfig
	infra           *k8sInfra
	sanitizer       *sanitization.Pipeline
	anomalyDetector *investigator.AnomalyDetector
	catalogFetcher  investigator.CatalogFetcher
	summarizer      *summarizer.Summarizer
	logger          logr.Logger
}

// investigationStack groups the constructed investigation-stack components
// that main() needs after buildInvestigationRunner: the investigator itself
// (needed by buildMCPHandler), the metrics/audit/session infrastructure, and
// the ogen server mounted on the router.
type investigationStack struct {
	agentMetrics      *kametrics.Metrics
	instrumentedAudit audit.AuditStore
	phaseResolver     *investigator.DefaultPhaseResolver
	inv               *investigator.Investigator
	store             *session.Store
	mgr               *session.Manager
	ogenSrv           *agentclient.Server
}

// buildInvestigationRunner wires the investigator, the alignment wrapper
// (when enabled), the session store/manager, and the ogen server. Terminates
// the process on unrecoverable wiring failures, matching the original inline
// main() behavior (GO-ANTIPATTERN-AUDIT-2026-07-01 Phase 4f).
func buildInvestigationRunner(p investigationRunnerParams) *investigationStack {
	agentMetrics := kametrics.NewMetrics()

	instrumentedAudit := audit.NewInstrumentedAuditStore(p.auditStore, agentMetrics.RecordAuditEventEmitted)

	var scopeResolver investigator.ScopeResolver
	if p.infra != nil {
		scopeResolver = investigator.NewMapperScopeResolver(p.infra.mapper)
	}

	// #1470: Build the PhaseResolver with the PinDecorator (if alignment is enabled).
	var pinDecorator func(llm.Client) llm.Client
	if p.alignEvaluator != nil {
		pinDecorator = func(pinned llm.Client) llm.Client {
			return alignment.NewLLMProxy(llm.NewInstrumentedClient(pinned))
		}
	}
	phaseResolver := investigator.NewDefaultPhaseResolver(p.swappable, pinDecorator)
	for phase, phaseSw := range p.phaseSwappables {
		phaseResolver.SetPhaseSwappable(phase, phaseSw)
	}

	invCfg := investigator.Config{
		Client:        p.effectiveLLM,
		Builder:       p.promptBuilder,
		ResultParser:  p.resultParser,
		Enricher:      p.enricher,
		AuditStore:    instrumentedAudit,
		Logger:        p.logger,
		MaxTurns:      p.cfg.AI.Investigation.MaxTurns,
		PhaseTools:    p.phaseTools,
		Registry:      p.effectiveReg,
		ModelName:     p.llmRuntime.Model,
		Swappable:     p.swappable,
		ScopeResolver: scopeResolver,
		Metrics:       agentMetrics,
		PhaseResolver: phaseResolver,
		PinDecorator:  pinDecorator,
		Pipeline: investigator.Pipeline{
			Sanitizer:         p.sanitizer,
			AnomalyDetector:   p.anomalyDetector,
			CatalogFetcher:    p.catalogFetcher,
			Summarizer:        p.summarizer,
			MaxToolOutputSize: p.cfg.AI.Summarizer.MaxToolOutputSize,
		},
	}
	inv := investigator.New(invCfg)

	var investigationRunner kaserver.InvestigationRunner = inv
	if p.alignEvaluator != nil {
		wrapper, wrapErr := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
			Inner:                 inv,
			Evaluator:             p.alignEvaluator,
			VerdictTimeout:        p.alignCfg.VerdictTimeout,
			AuditStore:            instrumentedAudit,
			Logger:                p.logger,
			Mode:                  p.alignCfg.Mode,
			CanaryForceEscalation: p.alignCfg.Canary.ForceEscalation,
			GroundingEnabled:      p.alignCfg.GroundingReview.Enabled,
		})
		if wrapErr != nil {
			p.logger.Error(wrapErr, "failed to create alignment wrapper")
			os.Exit(1)
		}
		investigationRunner = wrapper
	}

	store := session.NewStore(p.cfg.Runtime.Session.TTL,
		session.WithLogger(p.logger.WithName("session-store")),
		session.WithMaxConcurrent(p.cfg.Runtime.Session.MaxConcurrentInvestigations),
	)
	mgr := session.NewManager(store, p.logger, instrumentedAudit, agentMetrics)

	handler := kaserver.NewHandler(mgr, investigationRunner, p.logger, agentMetrics)

	ogenSrv, err := agentclient.NewServer(handler)
	if err != nil {
		p.logger.Error(err, "failed to create ogen server")
		os.Exit(1)
	}

	return &investigationStack{
		agentMetrics:      agentMetrics,
		instrumentedAudit: instrumentedAudit,
		phaseResolver:     phaseResolver,
		inv:               inv,
		store:             store,
		mgr:               mgr,
		ogenSrv:           ogenSrv,
	}
}

// wireHotReload configures conditional server TLS (with hot-reload), the
// LLM-runtime config watcher, the OCP TLS security profile, and the
// CA-file watcher. Mutates httpServer.TLSConfig in place when TLS is
// enabled. Terminates the process on unrecoverable wiring failures, matching
// the original inline main() behavior.
//
// Returns a single cleanup function that stops every watcher that was
// successfully started; the caller must defer it in its own scope so the
// watchers live for the server's lifetime, not just this function's call
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Phase 4f — same pattern as
// watchLoopState.stopEAWatcher in Phase 4e).
func wireHotReload(
	ctx context.Context,
	cfg *kaconfig.Config,
	httpServer *http.Server,
	llmRuntimePath string,
	swappable *llm.SwappableClient,
	phaseResolver *investigator.DefaultPhaseResolver,
	logger logr.Logger,
) func() {
	var stoppers []func()

	// Issue #493: Conditional TLS for the HTTP server
	// Issue #756: CertReloader enables hot-reload of server certificates
	var certReloader *sharedtls.CertReloader
	if cfg.Runtime.Server.TLS.Enabled() {
		isTLS, reloader, tlsErr := sharedtls.ConfigureConditionalTLS(httpServer, cfg.Runtime.Server.TLS.CertDir)
		if tlsErr != nil {
			logger.Error(tlsErr, "Failed to configure TLS")
			os.Exit(1)
		}
		if isTLS {
			certReloader = reloader
			logger.Info("TLS configured for HTTP server", "certDir", cfg.Runtime.Server.TLS.CertDir)
		}
	}

	// Issue #756: Wire FileWatcher for server cert hot-reload
	if certReloader != nil {
		certWatcher, watchErr := hotreload.NewFileWatcher(
			filepath.Join(cfg.Runtime.Server.TLS.CertDir, "tls.crt"),
			certReloader.ReloadCallback,
			logger.WithName("cert-reloader"),
		)
		if watchErr != nil {
			logger.Error(watchErr, "Failed to create cert file watcher")
			os.Exit(1)
		}
		if err := certWatcher.Start(ctx); err != nil {
			logger.Error(err, "Failed to start cert file watcher")
			os.Exit(1)
		}
		stoppers = append(stoppers, certWatcher.Stop)
	}

	// Issue #916: Wire FileWatcher for LLM runtime config hot-reload
	rtCallback := llmRuntimeReloadCallback(cfg, swappable, logger, phaseResolver)
	rtWatcher, rtWatchErr := hotreload.NewFileWatcher(
		llmRuntimePath,
		rtCallback,
		logger.WithName("llm-runtime-reloader"),
	)
	if rtWatchErr != nil {
		logger.Info("llm runtime file watcher not started", "error", rtWatchErr)
	} else {
		if err := rtWatcher.Start(ctx); err != nil {
			logger.Info("llm runtime file watcher failed to start", "error", err)
		} else {
			stoppers = append(stoppers, rtWatcher.Stop)
			logger.Info("llm runtime hot-reload enabled (#916)", "path", llmRuntimePath)
		}
	}

	// Issue #748: Load OCP TLS security profile from config before any TLS setup
	if err := sharedtls.SetDefaultSecurityProfileFromConfig(cfg.Runtime.Server.TLSProfile); err != nil {
		logger.Error(err, "Invalid TLS security profile in config, using default TLS 1.2")
	} else if cfg.Runtime.Server.TLSProfile != "" {
		logger.Info("TLS security profile active", "profile", cfg.Runtime.Server.TLSProfile)
	}

	// Issue #756: Start CA file watcher for client-side TLS hot-reload
	caWatcher, caWatchErr := sharedtls.StartCAFileWatcher(ctx, logger)
	if caWatchErr != nil {
		logger.Error(caWatchErr, "Failed to start CA file watcher")
		os.Exit(1)
	}
	if caWatcher != nil {
		stoppers = append(stoppers, caWatcher.Stop)
	}

	return func() {
		for _, stop := range stoppers {
			stop()
		}
	}
}
