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
	fleetclient "github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"
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
	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
)

// coreServices groups the mid-level dependencies constructed between the
// static config load and the investigation-stack wiring: audit, K8s infra,
// DataStorage clients, tool registry, fleet tools, enrichment, sanitization,
// anomaly detection, summarization, and the alignment-wrapped LLM/registry.
// Extracted from main() to keep main() under the funlen statement budget
// (GO-ANTIPATTERN-AUDIT-2026-07-01 complexity remediation, Wave C).
type coreServices struct {
	auditStore           audit.AuditStore
	auditCleanup         func()
	infra                *k8sInfra
	interactiveReadiness *karbac.InteractiveReadiness
	eventEmitter         *karbac.EventEmitter
	ds                   *dsClients
	reg                  *registry.Registry
	fleetClient          *fleetclient.ResilientClient
	enricher             *enrichment.Enricher
	sanitizer            *sanitization.Pipeline
	anomalyDetector      *investigator.AnomalyDetector
	summarizer           *summarizer.Summarizer
	catalogFetcher       investigator.CatalogFetcher
	effectiveLLM         llm.Client
	effectiveReg         registry.ToolRegistry
	alignEvaluator       *alignment.Evaluator
	alignCfg             kaconfig.AlignmentCheckConfig
}

// buildCoreServices wires audit, K8s infra, DataStorage, tool registry,
// fleet tools, enrichment/sanitization/anomaly-detection/summarization, and
// the alignment stack. Mutates phaseTools in place to append any
// fleet-discovered tools to the RCA phase. Terminates the process on the
// fatal "no DataStorage client" condition, matching the original inline
// main() behavior.
func buildCoreServices(
	cfg *kaconfig.Config,
	llmRuntime *kaconfig.LLMRuntimeConfig,
	swappable *llm.SwappableClient,
	dsTokenSource *auth.TokenSource,
	phaseTools katypes.PhaseToolMap,
	logger logr.Logger,
) *coreServices {
	auditStore, auditCleanup := buildAuditStore(cfg, dsTokenSource, logger)
	infra := initK8sInfra(logger)

	// #1288: SSAR impersonate gate removed — KA uses its own SA for all K8s
	// API calls. Interactive readiness is no longer gated on impersonation RBAC.
	interactiveReadiness := karbac.NewInteractiveReadiness()
	var eventEmitter *karbac.EventEmitter
	if cfg.Interactive.Enabled && infra != nil {
		podName, podNS := karbac.DetectPodIdentity()
		eventEmitter = karbac.NewEventEmitter(infra.clientset, podName, podNS)
	}

	ds := initDSClients(cfg, infra, dsTokenSource, logger)
	if ds == nil {
		logger.Error(nil, "FATAL: DataStorage client initialization failed — KA cannot operate without DS (workflow discovery, audit, enrichment all require it)")
		os.Exit(1)
	}
	reg := buildToolRegistry(cfg, logger, infra, ds, auditStore)
	fleetClient, fleetToolNames := registerFleetTools(context.Background(), cfg, reg, logger)
	if len(fleetToolNames) > 0 {
		investigator.AppendFleetToolsToRCA(phaseTools, fleetToolNames)
	}
	enricher := buildEnricher(cfg, ds, infra, auditStore, logger)
	sanitizer := buildSanitizationPipeline(cfg, logger)
	anomalyDetector := buildAnomalyDetector(cfg, logger)
	sum := buildSummarizer(swappable, cfg, logger)

	instrumentedLLM := llm.NewInstrumentedClient(swappable)

	var catalogFetcher investigator.CatalogFetcher
	if ds != nil {
		catalogFetcher = newDSCatalogFetcher(ds, logger)
		logger.Info("workflow catalog fetcher enabled (per-request, DD-HAPI-002)")
	} else {
		logger.Info("workflow catalog fetcher disabled (no DataStorage — dev mode)")
	}

	effectiveLLM, effectiveReg, alignEvaluator, alignCfg := buildAlignmentStack(cfg, llmRuntime, instrumentedLLM, reg, auditStore, logger)

	return &coreServices{
		auditStore: auditStore, auditCleanup: auditCleanup, infra: infra,
		interactiveReadiness: interactiveReadiness, eventEmitter: eventEmitter,
		ds: ds, reg: reg, fleetClient: fleetClient, enricher: enricher,
		sanitizer: sanitizer, anomalyDetector: anomalyDetector, summarizer: sum,
		catalogFetcher: catalogFetcher, effectiveLLM: effectiveLLM, effectiveReg: effectiveReg,
		alignEvaluator: alignEvaluator, alignCfg: alignCfg,
	}
}

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

// buildPinDecorator constructs the #1470 PinDecorator used by the
// PhaseResolver to wrap a pinned per-phase LLM client with the same
// alignment/instrumentation proxies as the primary client. Returns nil when
// alignment is disabled (no shadow evaluator).
func buildPinDecorator(alignEvaluator *alignment.Evaluator) func(llm.Client) llm.Client {
	if alignEvaluator == nil {
		return nil
	}
	return func(pinned llm.Client) llm.Client {
		return alignment.NewLLMProxy(llm.NewInstrumentedClient(pinned))
	}
}

// buildPhaseResolver constructs the #1470 DefaultPhaseResolver, resolving
// the scope resolver from the K8s infra (when available) and wiring in any
// configured per-phase SwappableClient overrides.
func buildPhaseResolver(p investigationRunnerParams, pinDecorator func(llm.Client) llm.Client) (investigator.ScopeResolver, *investigator.DefaultPhaseResolver) {
	var scopeResolver investigator.ScopeResolver
	if p.infra != nil {
		scopeResolver = investigator.NewMapperScopeResolver(p.infra.mapper)
	}
	phaseResolver := investigator.NewDefaultPhaseResolver(p.swappable, pinDecorator)
	for phase, phaseSw := range p.phaseSwappables {
		phaseResolver.SetPhaseSwappable(phase, phaseSw)
	}
	return scopeResolver, phaseResolver
}

// buildInvestigator constructs the Investigator from the runner params and
// the previously-resolved metrics/audit/scope/phase-resolver dependencies.
func buildInvestigator(
	p investigationRunnerParams,
	agentMetrics *kametrics.Metrics,
	instrumentedAudit audit.AuditStore,
	scopeResolver investigator.ScopeResolver,
	phaseResolver *investigator.DefaultPhaseResolver,
	pinDecorator func(llm.Client) llm.Client,
) *investigator.Investigator {
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
	return investigator.New(invCfg)
}

// wrapWithAlignment wraps inv in the shadow-agent alignment InvestigatorWrapper
// when alignment is enabled, otherwise returns inv unchanged. Terminates the
// process on unrecoverable wrapper construction failure, matching the
// original inline main() behavior.
func wrapWithAlignment(
	inv *investigator.Investigator,
	p investigationRunnerParams,
	instrumentedAudit audit.AuditStore,
) kaserver.InvestigationRunner {
	if p.alignEvaluator == nil {
		return inv
	}
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
	return wrapper
}

// buildInvestigationRunner wires the investigator, the alignment wrapper
// (when enabled), the session store/manager, and the ogen server. Terminates
// the process on unrecoverable wiring failures, matching the original inline
// main() behavior (GO-ANTIPATTERN-AUDIT-2026-07-01 Phase 4f).
func buildInvestigationRunner(p investigationRunnerParams) *investigationStack {
	agentMetrics := kametrics.NewMetrics()

	instrumentedAudit := audit.NewInstrumentedAuditStore(p.auditStore, agentMetrics.RecordAuditEventEmitted)

	// #1470: Build the PhaseResolver with the PinDecorator (if alignment is enabled).
	pinDecorator := buildPinDecorator(p.alignEvaluator)
	scopeResolver, phaseResolver := buildPhaseResolver(p, pinDecorator)

	inv := buildInvestigator(p, agentMetrics, instrumentedAudit, scopeResolver, phaseResolver, pinDecorator)
	investigationRunner := wrapWithAlignment(inv, p, instrumentedAudit)

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
// wireServerTLS configures conditional TLS on httpServer (Issue #493) and,
// when TLS is enabled, starts a FileWatcher for hot-reloading the server
// certificate (Issue #756). Returns a stopper for the cert watcher, or nil
// when TLS is disabled. Terminates the process on unrecoverable failures.
func wireServerTLS(ctx context.Context, cfg *kaconfig.Config, httpServer *http.Server, logger logr.Logger) func() {
	if !cfg.Runtime.Server.TLS.Enabled() {
		return nil
	}
	isTLS, reloader, tlsErr := sharedtls.ConfigureConditionalTLS(httpServer, cfg.Runtime.Server.TLS.CertDir)
	if tlsErr != nil {
		logger.Error(tlsErr, "Failed to configure TLS")
		os.Exit(1)
	}
	if !isTLS {
		return nil
	}
	logger.Info("TLS configured for HTTP server", "certDir", cfg.Runtime.Server.TLS.CertDir)

	certWatcher, watchErr := hotreload.NewFileWatcher(
		filepath.Join(cfg.Runtime.Server.TLS.CertDir, "tls.crt"),
		reloader.ReloadCallback,
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
	return certWatcher.Stop
}

// wireLLMRuntimeWatcher starts the Issue #916 FileWatcher for LLM runtime
// config hot-reload. Failures to create or start the watcher are logged but
// non-fatal (hot-reload is a best-effort convenience). Returns a stopper, or
// nil when the watcher could not be started.
func wireLLMRuntimeWatcher(
	ctx context.Context,
	cfg *kaconfig.Config,
	llmRuntimePath string,
	swappable *llm.SwappableClient,
	phaseResolver *investigator.DefaultPhaseResolver,
	logger logr.Logger,
) func() {
	rtCallback := llmRuntimeReloadCallback(cfg, swappable, logger, phaseResolver)
	rtWatcher, rtWatchErr := hotreload.NewFileWatcher(
		llmRuntimePath,
		rtCallback,
		logger.WithName("llm-runtime-reloader"),
	)
	if rtWatchErr != nil {
		logger.Info("llm runtime file watcher not started", "error", rtWatchErr)
		return nil
	}
	if err := rtWatcher.Start(ctx); err != nil {
		logger.Info("llm runtime file watcher failed to start", "error", err)
		return nil
	}
	logger.Info("llm runtime hot-reload enabled (#916)", "path", llmRuntimePath)
	return rtWatcher.Stop
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

	if stop := wireServerTLS(ctx, cfg, httpServer, logger); stop != nil {
		stoppers = append(stoppers, stop)
	}

	if stop := wireLLMRuntimeWatcher(ctx, cfg, llmRuntimePath, swappable, phaseResolver, logger); stop != nil {
		stoppers = append(stoppers, stop)
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
