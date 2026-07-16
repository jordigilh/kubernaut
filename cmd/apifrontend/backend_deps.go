package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/api/meta"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"google.golang.org/genai"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ds"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/resilience"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tlswiring"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

type caWatcherEntry struct {
	name    string
	watcher *hotreload.FileWatcher
}

// backendDeps holds shared backend clients used by both the MCP and A2A handlers.
// Created once by buildBackendDeps and consumed by buildMCPHandler / buildA2AHandler.
type backendDeps struct {
	DSClient              ds.Client
	KAClient              *ka.Client
	MCPClient             ka.MCPClient
	DedicatedClient       ka.MCPClient
	Pool                  *ka.KASessionPool
	Triager               *severity.Triager
	PromClient            prom.Client
	DSResilientTransport  *resilience.CircuitBreakerTransport
	CAWatchers            []caWatcherEntry
	k8sDynClient          dynamic.Interface
	k8sTypedClient        crclient.WithWatch
	K8sCB                 *resilience.K8sCircuitBreaker
	InvestigationRegistry *tools.MonitorRegistry
	Mapper                meta.RESTMapper
	// FleetReaderFactory routes kubectl_get/kubectl_list tool calls with a
	// non-empty cluster_id to remote fleet clusters via the MCP Gateway
	// (BR-FLEET-054). Nil when fleet federation is disabled.
	FleetReaderFactory tools.ResourceReaderFactory
	// FleetClusterRegistry backs the list_clusters tool (BR-FLEET-054). Nil
	// when fleet federation is disabled.
	FleetClusterRegistry registry.ClusterRegistry
	// fleetResilientClient is the underlying MCP Gateway connection; closed
	// on shutdown by run() when non-nil.
	fleetResilientClient *mcpclient.ResilientClient
	// fleetReadinessGate is the #1553 pod-wide readiness gate for Fleet
	// dependencies (ADR-068, BR-FLEET-054); nil when fleet is disabled.
	// Stopped on shutdown by stopBackendDeps.
	fleetReadinessGate *readiness.Gate
}

// FleetResilientClient returns the MCP Gateway connection backing
// FleetReaderFactory, or nil when fleet federation is disabled or its
// endpoint was unreachable at startup. Callers must check for nil before
// closing.
func (d *backendDeps) FleetResilientClient() *mcpclient.ResilientClient {
	return d.fleetResilientClient
}

// FleetReady reports whether Fleet dependencies (MCP Gateway, cluster
// registry) are reachable, for composition into the /readyz ReadyChecker
// chain (#1553, ADR-068). Always true when Fleet is disabled (nil gate) —
// matches the existing "no dependency, no gate" convention used across all
// 7 fail-closed readiness rollout services.
func (d *backendDeps) FleetReady() bool {
	if d.fleetReadinessGate == nil {
		return true
	}
	return d.fleetReadinessGate.Ready()
}

// K8sClient returns the pod service-account scoped dynamic K8s client,
// wrapped with a circuit breaker. Returns nil if K8s API was unreachable
// at startup; callers must check for nil (tools return a clear error).
func (d *backendDeps) K8sClient() dynamic.Interface {
	return d.k8sDynClient
}

// TypedClient returns the controller-runtime typed client for all kubernaut CRDs
// (RR, RAR, EA, AIAnalysis, IS). Returns nil if K8s API was unreachable;
// callers must check for nil (#1428).
func (d *backendDeps) TypedClient() crclient.WithWatch {
	return d.k8sTypedClient
}

func buildBackendDeps(ctx context.Context, cfg *config.Config, metricsReg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) (*backendDeps, error) {
	deps := &backendDeps{}

	if err := buildDSClientDeps(ctx, cfg, deps, metricsReg, auditor, logger); err != nil {
		return nil, err
	}
	if err := buildKAClientDeps(ctx, cfg, deps, metricsReg, auditor, logger); err != nil {
		return nil, err
	}

	// F7+F8: Eager K8s dynamic client init with circuit breaker (WIRE-03).
	// Fail-fast: log clearly on failure instead of returning silent nil.
	buildK8sClientDeps(cfg, deps, metricsReg, logger)

	if err := buildSeverityTriageDeps(ctx, cfg, deps, auditor, logger); err != nil {
		return nil, err
	}

	if err := buildFleetReaderDeps(ctx, cfg, deps, logger); err != nil {
		return nil, fmt.Errorf("fleet reader wiring: %w", err)
	}

	return deps, nil
}

// buildDSClientDeps wires the DataStorage ogen client behind a CA-reloadable,
// resilient (retry + circuit breaker) transport. A DS client construction
// failure degrades to a nil DSClient (DS tools return errors at runtime)
// rather than failing AF startup.
func buildDSClientDeps(ctx context.Context, cfg *config.Config, deps *backendDeps, metricsReg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) error {
	dsTransport, dsWatcher, err := tlswiring.CAReloadableTransport(cfg.Agent.DSTLSCaFile, logger.WithName("ds-ca"))
	if err != nil {
		return fmt.Errorf("DS TLS transport: %w", err)
	}
	if dsWatcher != nil {
		if err := dsWatcher.Start(ctx); err != nil {
			return fmt.Errorf("DS CA watcher start: %w", err)
		}
		deps.CAWatchers = append(deps.CAWatchers, caWatcherEntry{name: "ds-ca", watcher: dsWatcher})
	}

	deps.DSResilientTransport = buildResilientTransport(dsTransport, &cfg.Resilience.DS, "ds", metricsReg, auditor)

	var dsAuthTransport http.RoundTripper = deps.DSResilientTransport
	if cfg.Agent.DSBearerTokenFile != "" {
		dsAuthTransport = &bearerTokenTransport{
			base:      deps.DSResilientTransport,
			tokenFile: cfg.Agent.DSBearerTokenFile,
		}
	}

	dsCfg := ds.OgenClientConfig{
		BaseURL:   cfg.Agent.DSBaseURL,
		Timeout:   cfg.Resilience.DS.RequestTimeout,
		Transport: dsAuthTransport,
	}
	if c, err := ds.NewOgenClient(dsCfg); err == nil {
		deps.DSClient = c
	} else {
		logger.Info("DS client unavailable, DS tools will return errors", "error", err)
	}
	return nil
}

// buildKAClientDeps wires both the pooled/dedicated MCP clients (for
// investigation streaming, #1306/#1386) and the plain REST client used for
// non-MCP KA calls.
func buildKAClientDeps(ctx context.Context, cfg *config.Config, deps *backendDeps, metricsReg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) error {
	kaTransport, kaWatcher, err := tlswiring.CAReloadableTransport(cfg.Agent.KATLSCaFile, logger.WithName("ka-ca"))
	if err != nil {
		return fmt.Errorf("KA TLS transport: %w", err)
	}
	if kaWatcher != nil {
		if err := kaWatcher.Start(ctx); err != nil {
			return fmt.Errorf("KA CA watcher start: %w", err)
		}
		deps.CAWatchers = append(deps.CAWatchers, caWatcherEntry{name: "ka-ca", watcher: kaWatcher})
	}

	kaMCPResilient := buildResilientTransport(kaTransport, &cfg.Resilience.KA, "ka-mcp", metricsReg, auditor)
	var kaMCPAuth http.RoundTripper = kaMCPResilient
	if cfg.Agent.KABearerTokenFile != "" {
		kaMCPAuth = &bearerTokenTransport{
			base:      kaMCPResilient,
			tokenFile: cfg.Agent.KABearerTokenFile,
		}
	}
	kaMCPHTTPClient := &http.Client{
		Transport: kaMCPAuth,
		Timeout:   cfg.Resilience.KA.RequestTimeout,
	}
	// #1386: Separate HTTP client for long-lived MCP sessions (SSE streams).
	// Go's http.Client.Timeout is a deadline on the entire response including
	// body reads. For persistent SSE connections, the 30s timeout kills the
	// stream after idle periods, causing "session not found" on next tool call.
	// The MCP SDK manages session lifecycle via context cancellation and
	// session.Close(); no global timeout is needed.
	kaMCPStreamClient := &http.Client{
		Transport: kaMCPAuth,
	}
	mcpClient := ka.NewSDKMCPClient(
		cfg.Agent.KAMCPEndpoint,
		kaMCPHTTPClient,
		kaMCPStreamClient,
		logger,
	)
	mcpClient.WithDownstreamDuration(metricsReg.DownstreamDuration)

	// G2: Persistent MCP sessions (#1306). The pool creates real MCP
	// connections via StreamableClientTransport. Sessions are keyed by
	// (rr_id, username) for user isolation (G9). PooledMCPClient wraps
	// the pool to implement MCPClient, auto-releasing on terminal actions.
	kaMCPEndpoint := cfg.Agent.KAMCPEndpoint
	deps.Pool = ka.NewKASessionPool(ka.PoolConfig{
		Factory: func(ctx context.Context) (ka.PoolSession, error) {
			transport := &mcp.StreamableClientTransport{
				Endpoint:   kaMCPEndpoint,
				HTTPClient: kaMCPStreamClient,
			}
			return mcpClient.ConnectSession(ctx, transport)
		},
		MaxEntries: 100,
		IdleTTL:    10 * time.Minute,
		Logger:     logger.WithName("ka-session-pool"),
	})
	deps.MCPClient = ka.NewPooledMCPClient(deps.Pool, logger)
	deps.DedicatedClient = mcpClient
	deps.InvestigationRegistry = tools.NewMonitorRegistry()

	kaRESTAuth := kaTransport
	if cfg.Agent.KABearerTokenFile != "" {
		kaRESTAuth = &bearerTokenTransport{
			base:      kaTransport,
			tokenFile: cfg.Agent.KABearerTokenFile,
		}
	}

	deps.KAClient = ka.NewClient(ka.Config{
		BaseURL:            cfg.Agent.KABaseURL,
		BaseTransport:      kaRESTAuth,
		Timeout:            cfg.Resilience.KA.RequestTimeout,
		CBMaxRequests:      cfg.Resilience.KA.CBMaxRequests,
		CBInterval:         cfg.Resilience.KA.CBInterval,
		CBTimeout:          cfg.Resilience.KA.CBTimeout,
		CBFailureThreshold: cfg.Resilience.KA.CBFailureThreshold,
		RetryMax:           cfg.Resilience.KA.RetryMax,
		RetryInitBackoff:   cfg.Resilience.KA.RetryInitBackoff,
		RetryMaxBackoff:    cfg.Resilience.KA.RetryMaxBackoff,
		RetryableStatuses:  cfg.Resilience.KA.RetryableStatuses,
		CBAuditFunc:        resilience.CircuitBreakerAuditFunc(auditor),
	}, &ka.ClientMetrics{
		StateGauge:   metricsReg.CircuitBreakerState,
		DurationHist: metricsReg.DownstreamDuration,
	})
	return nil
}

// buildK8sClientDeps eagerly initializes the K8s dynamic client (circuit
// breaker wrapped), RESTMapper, and typed client. Each sub-step degrades
// independently and logs clearly on failure rather than failing AF startup —
// K8s-dependent tools return runtime errors instead when unavailable.
func buildK8sClientDeps(cfg *config.Config, deps *backendDeps, metricsReg *metrics.Registry, logger logr.Logger) {
	restCfg, err := ctrl.GetConfig()
	if err != nil {
		logger.Error(err, "K8s dynamic client unavailable — K8s tools will return errors at runtime")
		return
	}
	inner, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		logger.Error(err, "K8s dynamic client creation failed — K8s tools will return errors at runtime")
		return
	}

	k8sCfg := cfg.Resilience.K8s
	deps.K8sCB = resilience.NewK8sCircuitBreaker(resilience.K8sCBConfig{
		Name:             "k8s",
		MaxRequests:      k8sCfg.CBMaxRequests,
		Interval:         k8sCfg.CBInterval,
		Timeout:          k8sCfg.CBTimeout,
		FailureThreshold: k8sCfg.CBFailureThreshold,
		StateGauge:       metricsReg.CircuitBreakerState,
		DependencyName:   "k8s",
	})
	deps.k8sDynClient = resilience.NewResilientDynamicClient(inner, deps.K8sCB)
	logger.Info("K8s dynamic client initialized with circuit breaker")

	buildK8sDiscoveryMapper(restCfg, deps, logger)
	buildK8sTypedClient(restCfg, deps, logger)
}

// buildK8sDiscoveryMapper wires the RESTMapper used for CRD kind resolution.
// Unavailable discovery falls back to AF's static kind table.
func buildK8sDiscoveryMapper(restCfg *rest.Config, deps *backendDeps, logger logr.Logger) {
	disc, discErr := discovery.NewDiscoveryClientForConfig(restCfg)
	if discErr != nil {
		logger.Error(discErr, "K8s discovery client unavailable — CRD kind resolution will use static table only")
		return
	}
	deps.Mapper = restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disc))
	logger.Info("K8s RESTMapper initialized for CRD kind resolution")
}

// buildK8sTypedClient wires the controller-runtime typed client for all
// kubernaut CRDs (RR, RAR, EA, AIAnalysis, IS, #1428). Unavailable falls back
// to dynamic-only CRD operations.
func buildK8sTypedClient(restCfg *rest.Config, deps *backendDeps, logger logr.Logger) {
	typedScheme := k8sruntime.NewScheme()
	_ = eav1alpha1.AddToScheme(typedScheme)
	_ = remediationv1.AddToScheme(typedScheme)
	_ = aianalysisv1.AddToScheme(typedScheme)
	_ = isv1alpha1.AddToScheme(typedScheme)
	typedClient, tcErr := crclient.NewWithWatch(restCfg, crclient.Options{Scheme: typedScheme})
	if tcErr != nil {
		logger.Error(tcErr, "K8s typed client creation failed — CRD typed operations will fall back to dynamic")
		return
	}
	deps.k8sTypedClient = typedClient
	logger.Info("K8s typed client initialized for all kubernaut CRD operations (#1428)")
}

// buildSeverityTriageDeps wires the optional severity-triage subsystem
// (Prometheus client, LLM triager, rule engine config). No-op when disabled.
func buildSeverityTriageDeps(ctx context.Context, cfg *config.Config, deps *backendDeps, auditor audit.Emitter, logger logr.Logger) error {
	if !cfg.SeverityTriage.Enabled {
		return nil
	}

	promClient, err := buildTriagePrometheusClient(ctx, cfg, deps, logger)
	if err != nil {
		return err
	}
	deps.PromClient = promClient

	llmTriager := buildTriageLLMTriager(ctx, cfg, logger)
	severityCfg := buildTriageSeverityConfig(cfg)

	var triagerOpts []severity.TriagerOption
	triagerOpts = append(triagerOpts, severity.WithAuditor(auditor))
	if deps.k8sDynClient != nil {
		triagerOpts = append(triagerOpts, severity.WithPodResolver(
			severity.NewK8sPodResolver(deps.k8sDynClient, logger.WithName("pod-resolver")),
		))
	}

	deps.Triager = severity.NewTriager(promClient, llmTriager, severityCfg, logger.WithName("severity-triage"), triagerOpts...)
	logger.Info("severity triage enabled", "prometheusURL", cfg.SeverityTriage.PrometheusURL,
		"podResolverEnabled", deps.k8sDynClient != nil)
	return nil
}

// buildTriagePrometheusClient wires the CA-reloadable, optionally
// bearer-authenticated Prometheus HTTP client used by severity triage.
func buildTriagePrometheusClient(ctx context.Context, cfg *config.Config, deps *backendDeps, logger logr.Logger) (prom.Client, error) {
	promTransport, promWatcher, promErr := tlswiring.CAReloadableTransport(cfg.SeverityTriage.PrometheusTLSCaFile, logger.WithName("prom-ca"))
	if promErr != nil {
		return nil, fmt.Errorf("prometheus TLS transport: %w", promErr)
	}
	if promWatcher != nil {
		if err := promWatcher.Start(ctx); err != nil {
			return nil, fmt.Errorf("prometheus CA watcher start: %w", err)
		}
		deps.CAWatchers = append(deps.CAWatchers, caWatcherEntry{name: "prom-ca", watcher: promWatcher})
	}

	if cfg.Resilience.Prometheus.ConnectTimeout > 0 {
		if t, ok := promTransport.(*http.Transport); ok {
			t = t.Clone()
			t.DialContext = (&net.Dialer{Timeout: cfg.Resilience.Prometheus.ConnectTimeout}).DialContext
			promTransport = t
		}
	}
	promHTTPClient := &http.Client{
		Transport: promTransport,
		Timeout:   cfg.Resilience.Prometheus.RequestTimeout,
	}
	if cfg.SeverityTriage.PrometheusBearerTokenFile != "" {
		promHTTPClient.Transport = &bearerTokenTransport{
			base:      promTransport,
			tokenFile: cfg.SeverityTriage.PrometheusBearerTokenFile,
		}
	}

	return prom.NewHTTPClient(cfg.SeverityTriage.PrometheusURL, promHTTPClient), nil
}

// buildTriageLLMTriager resolves the effective triage LLM config (BR-AI-1404:
// independent or inherited from the agent) and constructs the corresponding
// triager, falling back to a noop triager when no provider is configured or
// construction fails.
func buildTriageLLMTriager(ctx context.Context, cfg *config.Config, logger logr.Logger) severity.LLMTriager {
	triageLLMCfg := cfg.Agent.LLM
	if cfg.SeverityTriage.LLM != nil {
		triageLLMCfg = *cfg.SeverityTriage.LLM
	}

	if triageLLMCfg.Provider == "" {
		logger.Info("LLM severity triage disabled (no LLM provider configured), using noop triager")
		return severity.NewNoopLLMTriager(logger.WithName("llm-triage"))
	}

	triager, triageErr := newLLMTriagerFromConfig(ctx, triageLLMCfg, logger.WithName("llm-triage"))
	if triageErr != nil {
		logger.Error(triageErr, "failed to create LLM triager, falling back to noop")
		return severity.NewNoopLLMTriager(logger.WithName("llm-triage"))
	}
	logger.Info("LLM severity triage enabled",
		"provider", triageLLMCfg.Provider,
		"model", triageLLMCfg.Model,
		"source", triageLLMSource(cfg))
	return triager
}

// buildTriageSeverityConfig resolves severity.Config from cfg.SeverityTriage,
// applying documented defaults for any zero-valued field.
func buildTriageSeverityConfig(cfg *config.Config) severity.Config {
	severityCfg := severity.Config{
		Enabled:           true,
		MaxQueriesPerCall: cfg.SeverityTriage.MaxQueriesPerCall,
		MaxRulesEvaluated: cfg.SeverityTriage.MaxRulesEvaluated,
		CacheTTLSeconds:   cfg.SeverityTriage.CacheTTLSeconds,
		LLMConfidence:     cfg.SeverityTriage.LLMConfidence,
	}
	if severityCfg.MaxQueriesPerCall == 0 {
		severityCfg.MaxQueriesPerCall = 10
	}
	if severityCfg.MaxRulesEvaluated == 0 {
		severityCfg.MaxRulesEvaluated = 100
	}
	if severityCfg.CacheTTLSeconds == 0 {
		severityCfg.CacheTTLSeconds = 30
	}
	if severityCfg.LLMConfidence == 0 {
		severityCfg.LLMConfidence = 0.7
	}
	return severityCfg
}

// fleetReadinessProbeInterval controls how often the #1553 Fleet readiness
// gate re-probes its dependencies once started (mirrors GW/RO/EM/SP/WE).
const fleetReadinessProbeInterval = 15 * time.Second

// buildFleetReaderDeps wires BR-FLEET-054 multi-cluster kubectl routing when
// fleet federation is enabled: connects to the MCP Gateway, discovers
// managed clusters via ClusterRegistry, and adapts the resulting
// fleet.ReaderFactory (client.Reader) into a tools.ResourceReaderFactory
// (dynamic-style ResourceReader) for kubectl_get/kubectl_list/list_clusters.
// A connectivity failure degrades gracefully to single-cluster mode for the
// tool-routing surface (mirrors GW's registerAdapters contract,
// cmd/gateway/main.go:246-284) rather than blocking AF startup — Fleet MCP
// Gateway connections are lazy/async. #1553/ADR-068: it also wires a
// readiness.Gate (deps.fleetReadinessGate, consumed via deps.FleetReady())
// so that unreachability is surfaced as pod-wide /readyz=NotReady instead
// of only this log line, closing the fail-open gap for AF.
func buildFleetReaderDeps(ctx context.Context, cfg *config.Config, deps *backendDeps, logger logr.Logger) error {
	if !cfg.Fleet.Enabled || cfg.Fleet.MCPGatewayEndpoint == "" {
		return nil
	}
	if deps.k8sDynClient == nil {
		logger.Info("K8s dynamic client unavailable, fleet cluster routing disabled")
		return nil
	}

	fleetLog := logger.WithName("fleet-mcp")
	var opts []mcpclient.Option
	if cfg.Fleet.OAuth2.Enabled {
		basePath := "/etc/apifrontend/fleet-oauth2"
		if cfg.Fleet.OAuth2.CredentialsSecretRef != "" {
			basePath = "/etc/apifrontend/" + cfg.Fleet.OAuth2.CredentialsSecretRef
		}
		reloadCfg := mcpclient.ReloadableOAuth2Config{
			TokenURL:         cfg.Fleet.OAuth2.TokenURL,
			ClientIDPath:     basePath + "/client-id",
			ClientSecretPath: basePath + "/client-secret",
			Scopes:           cfg.Fleet.OAuth2.Scopes,
			TlsCaFile:        cfg.Fleet.OAuth2.TLSCAFile,
		}
		opts = append(opts, mcpclient.WithReloadableOAuth2Transport(reloadCfg, fleetLog))
	}

	resilienceCfg := mcpclient.DefaultResilienceConfig()
	mcpFleetClient, err := mcpclient.NewResilient(ctx, cfg.Fleet.MCPGatewayEndpoint, resilienceCfg, fleetLog, opts...)
	if err != nil {
		// #1553: keep (don't discard) the disconnected client — the fleet
		// readiness gate attaches an MCPClientProber to it so the periodic
		// probe keeps retrying and the "fleet" readyz check correctly
		// reports NotReady until reconnect, instead of the client being
		// silently lost with no path back to healthy short of a restart.
		logger.Error(err, "Fleet MCP Gateway connection failed at startup; readiness will report NotReady "+
			"and keep retrying in the background; remote cluster routing disabled until reconnect",
			"endpoint", cfg.Fleet.MCPGatewayEndpoint)
		deps.fleetResilientClient = mcpFleetClient
		deps.fleetReadinessGate = wireFleetReadinessGate(ctx, mcpFleetClient, nil, logger)
		return nil
	}
	deps.fleetResilientClient = mcpFleetClient

	clusterRegistry, err := registry.NewClusterRegistry(
		cfg.Fleet.EffectiveMCPGatewayType(),
		deps.k8sDynClient,
		registry.RegistryConfig{},
		registry.NewMetrics(),
		fleetLog,
	)
	if err != nil {
		return fmt.Errorf("create fleet cluster registry (gatewayType=%s): %w", cfg.Fleet.MCPGatewayType, err)
	}
	if err := clusterRegistry.Start(ctx); err != nil {
		return fmt.Errorf("start fleet cluster registry: %w", err)
	}
	deps.FleetClusterRegistry = clusterRegistry

	readerFactory := mcpclient.NewMCPReaderFactoryWithProvider(
		deps.k8sTypedClient, mcpFleetClient.SessionProvider(), registry.NewToolPrefixAdapter(clusterRegistry))
	deps.FleetReaderFactory = adaptFleetReaderFactory(readerFactory)
	deps.fleetReadinessGate = wireFleetReadinessGate(ctx, mcpFleetClient, clusterRegistry, logger)

	logger.Info("Fleet MCP Gateway connected, multi-cluster kubectl routing enabled",
		"endpoint", cfg.Fleet.MCPGatewayEndpoint, "gatewayType", cfg.Fleet.MCPGatewayType)
	return nil
}

// wireFleetReadinessGate builds and starts the #1553 Fleet dependency
// readiness gate (ADR-068, BR-FLEET-054): once Fleet is enabled, AF's
// pod-wide readyz must fail closed when the MCP Gateway or cluster
// registry becomes unreachable, instead of the previous fail-open
// behavior of only logging an error. AF has no scope-checker dependency
// (unlike GW/RO), so its gate carries an MCPClientProber and, when
// available, a ClusterRegistryProber. fleetClient is always non-nil when
// called (buildFleetReaderDeps only calls this after Fleet.Enabled +
// endpoint checks); clusterRegistry may be nil (initial connection
// failure). The caller (buildFleetReaderDeps) stores the returned Gate on
// deps.fleetReadinessGate; stopBackendDeps must Stop() it on shutdown.
func wireFleetReadinessGate(ctx context.Context, fleetClient *mcpclient.ResilientClient, clusterRegistry registry.ClusterRegistry, logger logr.Logger) *readiness.Gate {
	probers := []readiness.Prober{&readiness.MCPClientProber{Client: fleetClient}}
	if clusterRegistry != nil {
		probers = append(probers, &readiness.ClusterRegistryProber{Registry: clusterRegistry})
	}

	gate := readiness.NewGate(fleetReadinessProbeInterval, logger.WithName("fleet-readiness"), probers...)
	gate.Start(ctx)
	logger.Info("Fleet readiness gate started", "prober_count", len(probers), "ready", gate.Ready())
	return gate
}

// adaptFleetReaderFactory adapts a fleet.ReaderFactory (client.Reader) into a
// tools.ResourceReaderFactory (dynamic-style ResourceReader) so kubectl_get/
// kubectl_list can consume BR-FLEET-054 remote cluster reads. Only ever
// invoked with a non-empty clusterID: AF's kubectl tools call the local
// dynamic client directly for local reads (see tools.NewKubectlGetTool).
func adaptFleetReaderFactory(rf fleet.ReaderFactory) tools.ResourceReaderFactory {
	return func(ctx context.Context, clusterID string) (tools.ResourceReader, error) {
		r, err := rf.ReaderFor(ctx, clusterID)
		if err != nil {
			return nil, err
		}
		return &tools.ClientResourceReader{Reader: r}, nil
	}
}

// newLLMTriagerFromConfig creates a provider-aware LLMTriager based on the resolved
// LLM configuration. Routes by provider + model family:
//   - vertex_ai + claude-* model → AnthropicTriager (Anthropic SDK + Vertex)
//   - vertex_ai + other model → GenAITriager (Google GenAI SDK)
//   - gemini → GenAITriager (Gemini API)
//   - anthropic → AnthropicTriager (direct Anthropic API)
//   - openai / openai_compatible → OpenAICompatibleTriager (shared openaicompat
//     client — OpenAI, Azure OpenAI, vLLM, Ollama, LlamaStack, self-hosted; #1618)
func newLLMTriagerFromConfig(ctx context.Context, llmCfg types.LLMConfig, logger logr.Logger) (severity.LLMTriager, error) {
	switch llmCfg.Provider {
	case types.LLMProviderVertexAI:
		if severity.IsAnthropicModel(llmCfg.Model) {
			return newAnthropicTriagerForVertex(ctx, llmCfg, logger)
		}
		return newGenAITriagerForVertex(ctx, llmCfg, logger)
	case types.LLMProviderGemini:
		return newGenAITriagerForGemini(ctx, llmCfg, logger)
	case types.LLMProviderAnthropic:
		return newAnthropicTriagerDirect(ctx, llmCfg, logger)
	case types.LLMProviderOpenAI, types.LLMProviderOpenAICompatible:
		return newOpenAICompatibleTriager(llmCfg, logger)
	default:
		return nil, fmt.Errorf("unsupported triage LLM provider: %q", llmCfg.Provider)
	}
}

func newGenAITriagerForVertex(ctx context.Context, llmCfg types.LLMConfig, logger logr.Logger) (severity.LLMTriager, error) {
	clientCfg := &genai.ClientConfig{
		Project:  llmCfg.VertexProject,
		Location: llmCfg.VertexLocation,
		Backend:  genai.BackendVertexAI,
	}
	if llmCfg.Endpoint != "" {
		clientCfg.HTTPOptions = genai.HTTPOptions{BaseURL: llmCfg.Endpoint}
	}
	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("vertex_ai GenAI client: %w", err)
	}
	return severity.NewGenAITriager(severity.GenAITriagerConfig{
		Client: client,
		Model:  llmCfg.Model,
		Logger: logger,
	}), nil
}

func newGenAITriagerForGemini(ctx context.Context, llmCfg types.LLMConfig, logger logr.Logger) (severity.LLMTriager, error) {
	clientCfg := &genai.ClientConfig{
		APIKey:  llmCfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	}
	if llmCfg.Endpoint != "" {
		clientCfg.HTTPOptions = genai.HTTPOptions{BaseURL: llmCfg.Endpoint}
	}
	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("gemini GenAI client: %w", err)
	}
	return severity.NewGenAITriager(severity.GenAITriagerConfig{
		Client: client,
		Model:  llmCfg.Model,
		Logger: logger,
	}), nil
}

func newAnthropicTriagerForVertex(ctx context.Context, llmCfg types.LLMConfig, logger logr.Logger) (severity.LLMTriager, error) {
	client, err := severity.NewAnthropicVertexClient(ctx, llmCfg.VertexProject, llmCfg.VertexLocation)
	if err != nil {
		return nil, fmt.Errorf("vertex_ai Anthropic client: %w", err)
	}
	return severity.NewAnthropicTriager(severity.AnthropicTriagerConfig{
		Client: client,
		Model:  llmCfg.Model,
		Logger: logger,
	}), nil
}

func newAnthropicTriagerDirect(_ context.Context, llmCfg types.LLMConfig, logger logr.Logger) (severity.LLMTriager, error) {
	client, err := severity.NewAnthropicDirectClient(llmCfg.APIKey)
	if err != nil {
		return nil, fmt.Errorf("anthropic direct client: %w", err)
	}
	return severity.NewAnthropicTriager(severity.AnthropicTriagerConfig{
		Client: client,
		Model:  llmCfg.Model,
		Logger: logger,
	}), nil
}

// newOpenAICompatibleTriager builds an LLMTriager backed by the shared
// openaicompat client (#1618), reusing the same TLS/OAuth2/custom-header/
// circuit-breaker transport chain as AF's main-agent OpenAI-compatible
// model (launcher.BuildLLMHTTPClient) so severity triage gets identical
// resilience/auth behavior to the agent it triages for.
func newOpenAICompatibleTriager(llmCfg types.LLMConfig, logger logr.Logger) (severity.LLMTriager, error) {
	httpClient, err := launcher.BuildLLMHTTPClient(llmCfg)
	if err != nil {
		return nil, fmt.Errorf("build HTTP client: %w", err)
	}

	var opts []openaicompat.Option
	if httpClient != nil {
		opts = append(opts, openaicompat.WithHTTPClient(httpClient))
	}
	if llmCfg.AzureAPIVersion != "" {
		opts = append(opts, openaicompat.WithAzureAPIVersion(llmCfg.AzureAPIVersion))
	}

	client := openaicompat.New(llmCfg.Model, llmCfg.Endpoint, llmCfg.APIKey, opts...)
	return severity.NewOpenAICompatibleTriager(severity.OpenAICompatibleTriagerConfig{
		Client: client,
		Model:  llmCfg.Model,
		Logger: logger,
	}), nil
}

// triageLLMSource returns a human-readable label indicating whether the triage
// LLM config was explicitly set or inherited from the agent.
func triageLLMSource(cfg *config.Config) string {
	if cfg.SeverityTriage.LLM != nil {
		return "severityTriage.llm (explicit)"
	}
	return "agent.llm (inherited)"
}

// buildResilientTransport wraps a base transport with retry + circuit breaker.
// ConnectTimeout is applied via net.Dialer on the underlying http.Transport.
// Returns the CB transport for health checking.
func buildResilientTransport(base http.RoundTripper, depCfg *config.DependencyConfig, name string, reg *metrics.Registry, auditor audit.Emitter) *resilience.CircuitBreakerTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	if depCfg.ConnectTimeout > 0 {
		if t, ok := base.(*http.Transport); ok {
			t = t.Clone()
			t.DialContext = (&net.Dialer{Timeout: depCfg.ConnectTimeout}).DialContext
			base = t
		}
	}
	retryRT := resilience.NewRetryTransport(base, &resilience.RetryConfig{
		MaxAttempts:       depCfg.RetryMax + 1,
		InitialBackoff:    depCfg.RetryInitBackoff,
		MaxBackoff:        depCfg.RetryMaxBackoff,
		RetryableStatuses: depCfg.RetryableStatuses,
		DependencyName:    name,
	})
	cbMaxReqs := depCfg.CBMaxRequests
	if cbMaxReqs == 0 {
		cbMaxReqs = 1
	}
	cbInterval := depCfg.CBInterval
	if cbInterval == 0 {
		cbInterval = 30 * time.Second
	}
	cbTimeout := depCfg.CBTimeout
	if cbTimeout == 0 {
		cbTimeout = 10 * time.Second
	}
	cbFailureThreshold := depCfg.CBFailureThreshold
	if cbFailureThreshold == 0 {
		cbFailureThreshold = 5
	}
	cbt := resilience.NewCircuitBreakerTransport(retryRT, &resilience.CircuitBreakerConfig{
		Name:             name,
		DependencyName:   name,
		MaxRequests:      cbMaxReqs,
		Interval:         cbInterval,
		Timeout:          cbTimeout,
		FailureThreshold: cbFailureThreshold,
		StateGauge:       reg.CircuitBreakerState,
		DurationHist:     reg.DownstreamDuration,
		AuditFunc:        resilience.CircuitBreakerAuditFunc(auditor),
	})
	return cbt
}

// bridgeMetricsFrom wires the global metrics registry counters into
// the bridge metrics struct — single instances shared across the process.
func bridgeMetricsFrom(reg *metrics.Registry) *handler.MCPBridgeMetrics {
	return &handler.MCPBridgeMetrics{
		ToolCallsTotal:   reg.ToolCallsTotal,
		ToolCallDuration: reg.ToolCallDuration,
	}
}
