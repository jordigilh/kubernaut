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

// FMC: Fleet Metadata Cache service.
// Writes: Polls remote clusters via MCP Gateway for resources labeled kubernaut.ai/managed=true
// and writes their metadata to Valkey.
// Reads: Serves an HTTP API for federated scope checking (ADR-068), so GW/RO
// query FMC instead of connecting to Valkey directly.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jordigilh/kubernaut/internal/version"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	fmcconfig "github.com/jordigilh/kubernaut/pkg/fleet/fmc/config"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedhealth "github.com/jordigilh/kubernaut/pkg/shared/health"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// fmcDeps bundles the external clients and background components wired at
// FMC startup: the resilient MCP Gateway client, the Valkey read/write
// clients, the cluster registry, and the metadata syncer.
type fmcDeps struct {
	reg             *prometheus.Registry
	metrics         *fmc.Metrics
	mcpClient       *mcpclient.ResilientClient
	writer          *fmc.ValkeyWriter
	cacheReader     *scopecache.ValkeyCacheReader
	clusterRegistry registry.ClusterRegistry
	syncer          *fmc.Syncer

	// Issue #1993 (ADR-068 gap closure, IA-2/AC-3): TokenReview/SAR auth for
	// the GW/RO -> FMC scope-check API. k8sClientset backs auth.NewK8sAuthenticator
	// / auth.NewK8sAuthorizer; releaseNamespace is the SAR target namespace
	// (where the fleetmetadatacache-service Service lives).
	k8sClientset     kubernetes.Interface
	releaseNamespace string
}

// close releases the resources held by deps, in reverse dependency order.
func (d *fmcDeps) close() {
	d.clusterRegistry.Stop()
	_ = d.cacheReader.Close()
	_ = d.writer.Close()
	_ = d.mcpClient.Close()
}

// wireFMCDependencies connects to the MCP Gateway (with reloadable OAuth2
// transport), the Kubernetes API (for the dynamic client used by the
// cluster registry), and Valkey (read/write), then constructs and starts
// the cluster registry and metadata syncer. Exits the process on any
// failure, matching main()'s original fail-fast behavior.
// buildValkeyTLSConfig constructs the *tls.Config for FMC's Valkey client
// connection (DD-PLATFORM-006 Decision Area 13 follow-up, round-16 RCA): the
// chart's own Valkey is TLS-only (Decision Area 8), and unlike the
// APIFrontend replay cache, FMC's Valkey dependency has no fail-open
// fallback -- so a misconfigured TLS setup must fail fast at startup rather
// than surface as an opaque connection error later. Returns nil (plaintext)
// when TLS is disabled.
func buildValkeyTLSConfig(tlsCfg fmcconfig.ValkeyTLSConfig, logger logr.Logger) *tls.Config {
	if !tlsCfg.Enabled {
		return nil
	}
	valkeyTLSConfig, err := sharedtls.BuildTLSConfig(tlsCfg.CAFile, sharedtls.WithClientCert(tlsCfg.CertFile, tlsCfg.KeyFile))
	if err != nil {
		logger.Error(err, "Failed to configure Valkey TLS")
		os.Exit(1)
	}
	return valkeyTLSConfig
}

// buildFMCK8sClients constructs the dynamic client (for the cluster
// registry) and typed clientset (for the #1993 TokenReview/SAR auth
// middleware), tuned for the concurrency the scope-check API path needs, and
// resolves the release namespace (the SAR target namespace for the
// fleetmetadatacache-service Service). Exits the process on failure,
// matching wireFMCDependencies' fail-fast behavior.
func buildFMCK8sClients(logger logr.Logger) (dynamic.Interface, kubernetes.Interface, string) {
	k8sCfg, err := ctrl.GetConfig()
	if err != nil {
		logger.Error(err, "Failed to get Kubernetes config")
		os.Exit(1)
	}

	// Issue #1993: tune for TokenReview/SAR concurrency on the scope-check
	// API path, mirroring cmd/datastorage/main.go:409-418. A spike measured
	// during this issue's planning found client-go's raw QPS=5/Burst=10
	// default (not TokenReview/SAR itself) responsible for multi-second
	// waits under concurrent load; this also benefits dynClient below.
	k8sCfg.Timeout = 30 * time.Second
	k8sCfg.QPS = 1000.0
	k8sCfg.Burst = 2000

	dynClient, err := dynamic.NewForConfig(k8sCfg)
	if err != nil {
		logger.Error(err, "Failed to create dynamic Kubernetes client")
		os.Exit(1)
	}

	k8sClientset, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes clientset for auth middleware")
		os.Exit(1)
	}

	releaseNamespace, err := scope.GetControllerNamespace()
	if err != nil {
		logger.Error(err, "Failed to determine release namespace for auth middleware")
		os.Exit(1)
	}

	return dynClient, k8sClientset, releaseNamespace
}

func wireFMCDependencies(ctx context.Context, cfg *fmcconfig.ServiceConfig, logger logr.Logger) *fmcDeps {
	reg := prometheus.NewRegistry()
	metrics := fmc.NewMetrics(reg)

	reloadCfg := mcpclient.ReloadableOAuth2Config{
		TokenURL:         cfg.OAuth2.TokenURL,
		ClientIDPath:     cfg.OAuth2.CredentialsDir + "/client-id",
		ClientSecretPath: cfg.OAuth2.CredentialsDir + "/client-secret",
		Scopes:           cfg.OAuth2.Scopes,
		TokenTimeout:     cfg.OAuth2.TokenTimeout,
		TlsCaFile:        cfg.OAuth2.TlsCaFile,
	}
	opts := []mcpclient.Option{
		mcpclient.WithReloadableOAuth2Transport(reloadCfg, logger), //nolint:contextcheck // OAuth2 token source refresh runs as a background reload, independent of any single request
	}
	logger.Info("OAuth2 authentication configured for MCP Gateway",
		"tokenURL", cfg.OAuth2.TokenURL,
		"credentialsDir", cfg.OAuth2.CredentialsDir)

	resilienceCfg := mcpclient.ResilienceConfigFromFleet(cfg.MCPGateway.Resilience)
	mcpClient, err := mcpclient.NewResilient(ctx, cfg.MCPGateway.Endpoint, resilienceCfg, logger, opts...)
	if err != nil {
		logger.Error(err, "Failed to connect to MCP Gateway")
		os.Exit(1)
	}

	dynClient, k8sClientset, releaseNamespace := buildFMCK8sClients(logger)

	valkeyTLSConfig := buildValkeyTLSConfig(cfg.Valkey.TLS, logger)
	writer := fmc.NewValkeyWriter(cfg.Valkey.Addr, fmc.WithTLSConfig(valkeyTLSConfig))
	cacheReader := scopecache.NewValkeyCacheReader(cfg.Valkey.Addr, scopecache.WithTLSConfig(valkeyTLSConfig))

	clusterRegistry, err := registry.NewClusterRegistry(registry.MCPGatewayType(cfg.MCPGateway.GatewayType), dynClient, registry.RegistryConfig{
		Namespace: cfg.MCPGateway.Namespace,
	}, registry.NewMetricsWithRegistry(reg), logger)
	if err != nil {
		logger.Error(err, "Failed to create cluster registry", "gatewayType", cfg.MCPGateway.GatewayType)
		os.Exit(1)
	}
	if err := clusterRegistry.Start(ctx); err != nil {
		logger.Error(err, "Failed to start cluster registry")
		os.Exit(1)
	}

	syncerConfig := fmc.Config{
		SyncInterval:       cfg.Sync.Interval,
		KeyTTL:             cfg.Sync.KeyTTL,
		ResourceKinds:      cfg.Sync.ResourceKinds,
		WaitForBrokerReady: cfg.Sync.WaitForBrokerReady,
	}

	sessionProvider := mcpClient.SessionProvider()
	readerFactory := fleet.ReaderFactoryFunc(func(_ context.Context, clusterID string) (client.Reader, error) {
		// WithReconnect: SessionProvider() alone only re-reads whatever session
		// mcpClient currently holds -- it cannot repair a session that died from
		// a protocol-level error (e.g. a malformed response during a startup
		// race with the MCP Gateway broker's config reload). Without this, a
		// single early failure permanently breaks every sync cycle for the rest
		// of the FMC pod's lifetime, even after the Gateway becomes healthy.
		opts := []mcpclient.Option{mcpclient.WithReconnect(mcpClient.Reconnect)}
		if info, found := clusterRegistry.Get(clusterID); found && info.ToolPrefix != "" {
			opts = append(opts, mcpclient.WithToolPrefix(info.ToolPrefix))
		}
		return mcpclient.NewFromSessionProvider(sessionProvider, clusterID, opts...), nil
	})
	syncer := fmc.NewSyncerWithReaderFactory(clusterRegistry, readerFactory, writer, syncerConfig, logger, metrics)

	return &fmcDeps{
		reg:              reg,
		metrics:          metrics,
		mcpClient:        mcpClient,
		writer:           writer,
		cacheReader:      cacheReader,
		clusterRegistry:  clusterRegistry,
		syncer:           syncer,
		k8sClientset:     k8sClientset,
		releaseNamespace: releaseNamespace,
	}
}

// fmcServers bundles the federated scope-checking API server (ADR-068), the
// dedicated health-probe server (Issue #753), and the Prometheus metrics
// server, plus the readiness flag backing the /readyz handler's liveness
// signal and the optional TLS cert reloader for the API server (Issue #493).
type fmcServers struct {
	api          *http.Server
	health       *http.Server
	metrics      *http.Server
	ready        *atomic.Bool
	certReloader *sharedtls.CertReloader
	tlsCertDir   string
}

// livenessHandler is FMC's liveness probe: a fixed 200 OK with no backend
// dependency check. Registered exclusively on the dedicated health mux
// (plain HTTP, kubelet-only) -- DD-FLEET-004: it is never registered on the
// API mux. Unlike /healthz, /readyz IS deliberately registered a second
// time on the API mux (see buildFMCServers below) -- DD-PLATFORM-010,
// Issue #2169 -- as the unauthenticated target for fmc.HTTPClient.Ping(),
// GW/RO's fail-closed readiness gate probe (Issue #1553/ADR-068).
func livenessHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// buildFMCServers constructs the federated scope-checking API server
// (ADR-068, Issue #493 conditional TLS), the dedicated health-probe server
// (Issue #753), and the Prometheus metrics server from cfg and deps. ready
// backs the /readyz handler's liveness signal.
func buildFMCServers(cfg *fmcconfig.ServiceConfig, deps *fmcDeps, ready *atomic.Bool, logger logr.Logger) *fmcServers {
	scopeClient := scopecache.NewClient(deps.cacheReader)
	apiHandler := fmc.NewHandler(scopeClient, deps.clusterRegistry, logger)
	apiMux := http.NewServeMux()
	apiHandler.RegisterRoutes(apiMux)

	// Issue #1993 (ADR-068 gap closure, IA-2/AC-3): every GW/RO -> FMC
	// scope-check/clusters request must carry a valid ServiceAccount bearer
	// token (TokenReview) and be authorized (SAR) against this Service --
	// mirrors DataStorage's own inbound-auth precedent (DD-AUTH-014).
	// /healthz is unaffected: DD-FLEET-004 already serves it exclusively on
	// the separate health port, never on apiMux.
	authenticator := auth.NewK8sAuthenticator(deps.k8sClientset)
	authorizer := auth.NewK8sAuthorizer(deps.k8sClientset)
	authMiddleware := auth.NewMiddleware(authenticator, authorizer, auth.MiddlewareConfig{
		Namespace:    deps.releaseNamespace,
		Resource:     "services",
		ResourceName: "fleetmetadatacache-service",
		Verb:         "get",
	}, logger)

	// DD-PLATFORM-010 (Issue #2169): /readyz is registered at the top level
	// of a wrapper mux, deliberately OUTSIDE authMiddleware's wrap, so
	// fmc.HTTPClient.Ping() (GW/RO's fail-closed readiness gate probe,
	// Issue #1553/ADR-068) reaches it without paying a live TokenReview/SAR
	// round-trip on every poll. This reuses the exact same ReadyzHandler
	// that already backs the kubelet probe on the dedicated health port --
	// a second *registration*, not a second *implementation*. Everything
	// else (the real scope-check/clusters API) still goes through
	// authMiddleware via the "/" delegation below.
	topMux := http.NewServeMux()
	topMux.HandleFunc(fmc.ReadyzPath, fmc.ReadyzHandler(ready.Load, deps.cacheReader))
	topMux.Handle("/", authMiddleware.Handler(apiMux))

	apiServer := &http.Server{
		Addr:              cfg.Server.APIAddr,
		Handler:           topMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	var certReloader *sharedtls.CertReloader
	var tlsCertDir string
	if cfg.Server.TLS.Enabled() {
		isTLS, reloader, err := sharedtls.ConfigureConditionalTLS(apiServer, cfg.Server.TLS.CertDir)
		if err != nil {
			logger.Error(err, "Failed to configure TLS for FMC API server", "certDir", cfg.Server.TLS.CertDir)
			os.Exit(1)
		}
		if isTLS {
			certReloader = reloader
			tlsCertDir = cfg.Server.TLS.CertDir
			logger.Info("TLS configured for FMC API server", "certDir", cfg.Server.TLS.CertDir)
		}
	}

	// Issue #753: dedicated health-probe server, always plain HTTP -- kubelet
	// probes never need TLS. DD-PLATFORM-010 (Issue #2169): /readyz is now
	// ALSO registered on the API port above (topMux, unauthenticated) for
	// GW/RO's cross-service Ping(); this health-port registration remains
	// for kubelet's own probe, which never crosses pod boundaries.
	healthServer := sharedhealth.NewHealthServer(
		cfg.Server.HealthAddr,
		livenessHandler,
		fmc.ReadyzHandler(ready.Load, deps.cacheReader),
		true,
	)

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.HandlerFor(deps.reg, promhttp.HandlerOpts{}))

	metricsServer := &http.Server{
		Addr:              cfg.Server.MetricsAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &fmcServers{
		api:          apiServer,
		health:       healthServer,
		metrics:      metricsServer,
		ready:        ready,
		certReloader: certReloader,
		tlsCertDir:   tlsCertDir,
	}
}

// startCertHotReload starts a hotreload.FileWatcher on the API server's TLS
// certificate (Issue #756) so a cert-manager rotation is picked up without a
// pod restart. Returns nil if TLS is not configured (servers.certReloader
// is nil). Exits the process on failure, matching DataStorage's Start()
// fail-fast behavior for a misconfigured/unreadable cert directory.
func startCertHotReload(ctx context.Context, servers *fmcServers, logger logr.Logger) *hotreload.FileWatcher {
	if servers.certReloader == nil {
		return nil
	}
	watcher, err := hotreload.NewFileWatcher(
		filepath.Join(servers.tlsCertDir, "tls.crt"),
		servers.certReloader.ReloadCallback,
		logger.WithName("cert-reloader"),
	)
	if err != nil {
		logger.Error(err, "Failed to create TLS cert file watcher")
		os.Exit(1)
	}
	if err := watcher.Start(ctx); err != nil {
		logger.Error(err, "Failed to start TLS cert file watcher")
		os.Exit(1)
	}
	return watcher
}

// serveAndReport starts srv (TLS or plain HTTP, per useTLS) and reports any
// non-graceful error on errCh. Shared by all three FMC HTTP servers so
// runFMCServers doesn't repeat the same listen/log/error-check block three
// times over (Issue #1683 REFACTOR: extracted to keep gocyclo under the
// project's threshold after the 3-port split added a second server).
func serveAndReport(name string, srv *http.Server, useTLS bool, errCh chan<- error, logger logr.Logger) {
	logger.Info(name+" server listening", "addr", srv.Addr, "tls", useTLS)
	var err error
	if useTLS {
		err = srv.ListenAndServeTLS("", "")
	} else {
		err = srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		errCh <- err
	}
}

// shutdownFMCServers gracefully shuts down all three FMC HTTP servers with a
// shared bounded timeout, logging (not failing) any individual shutdown
// error -- matching runFMCServers' pre-#1683 per-server shutdown behavior.
// Takes the caller's ctx per the codebase's contextcheck convention even
// though it deliberately derives its shutdown deadline from
// context.Background(), not ctx, since ctx is already cancelled by the time
// shutdown begins (see the nolint below).
func shutdownFMCServers(ctx context.Context, servers *fmcServers, logger logr.Logger) {
	_ = ctx
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	for _, s := range []struct {
		name string
		srv  *http.Server
	}{
		{"API", servers.api},
		{"Health", servers.health},
		{"Metrics", servers.metrics},
	} {
		if err := s.srv.Shutdown(shutdownCtx); err != nil { //nolint:contextcheck // shutdown uses a bounded shutdown context, deliberately independent of any request context already cancelled during teardown
			logger.Error(err, s.name+" server shutdown failed")
		}
	}
}

// runFMCServers starts the API, health, and metrics servers and the
// metadata syncer in the background, marks the service ready once the MCP
// client reports readiness, then blocks until a shutdown signal or a server
// failure is observed, gracefully shutting down all three HTTP servers
// before returning.
func runFMCServers(ctx context.Context, cancel context.CancelFunc, sigCh <-chan os.Signal, deps *fmcDeps, servers *fmcServers, logger logr.Logger) {
	certWatcher := startCertHotReload(ctx, servers, logger)
	if certWatcher != nil {
		defer certWatcher.Stop()
	}

	apiErrors := make(chan error, 1)
	go serveAndReport("API", servers.api, servers.api.TLSConfig != nil, apiErrors, logger)

	healthErrors := make(chan error, 1)
	go serveAndReport("Health", servers.health, false, healthErrors, logger)

	metricsErrors := make(chan error, 1)
	go serveAndReport("Metrics", servers.metrics, false, metricsErrors, logger)

	go func() {
		if err := deps.syncer.Run(ctx); err != nil {
			logger.Error(err, "Syncer stopped with error")
			cancel()
		}
	}()

	servers.ready.Store(deps.mcpClient.Ready())
	logger.Info("FMC ready", "mcpConnected", deps.mcpClient.Ready())

	select {
	case <-sigCh:
		logger.Info("Received shutdown signal")
	case err := <-apiErrors:
		logger.Error(err, "API server failed")
	case err := <-healthErrors:
		logger.Error(err, "Health server failed")
	case err := <-metricsErrors:
		logger.Error(err, "Metrics server failed")
	}
	cancel()
	shutdownFMCServers(ctx, servers, logger)
}

func main() {
	// gocritic:exitAfterDefer — run() returns an exit code instead of calling
	// os.Exit directly so deferred cleanup (zapLogger.Sync, cancel, deps.close)
	// always runs.
	os.Exit(run())
}

func run() int {
	var configPath string
	flag.StringVar(&configPath, "config", fmcconfig.DefaultConfigPath, "Path to YAML config file (ADR-030)")
	flag.Parse()

	zapLogger, _ := zap.NewProduction()
	defer func() { _ = zapLogger.Sync() }()
	logger := zapr.NewLogger(zapLogger)

	cfg, err := fmcconfig.LoadFromFile(configPath)
	if err != nil {
		logger.Error(err, "Failed to load configuration", "path", configPath)
		return 1
	}
	if err := cfg.Validate(); err != nil {
		logger.Error(err, "Invalid configuration")
		return 1
	}

	logger.Info("FMC starting",
		"syncInterval", cfg.Sync.Interval,
		"valkeyAddr", cfg.Valkey.Addr,
		"mcpEndpoint", cfg.MCPGateway.Endpoint,
		"gatewayType", cfg.MCPGateway.GatewayType,
		"apiAddr", cfg.Server.APIAddr,
		"healthAddr", cfg.Server.HealthAddr,
		"metricsAddr", cfg.Server.MetricsAddr,
		"version", version.Version,
		"gitCommit", version.GitCommit,
		"buildDate", version.BuildDate,
	)

	// Issue #748: Load OCP TLS security profile from config before any TLS
	// setup (buildFMCServers' ConfigureConditionalTLS call reads this via
	// the process-wide default set here).
	if err := sharedtls.SetDefaultSecurityProfileFromConfig(cfg.TLSProfile); err != nil {
		logger.Error(err, "Invalid TLS security profile in config, using default TLS 1.2")
	} else if cfg.TLSProfile != "" {
		logger.Info("TLS security profile active", "profile", cfg.TLSProfile)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// DD-PLATFORM-009: answer kubelet's startupProbe/livenessProbe truthfully
	// while wireFMCDependencies' blocking MCP Gateway connection
	// (mcpclient.NewResilient) and cluster registry cache sync are still in
	// progress, instead of leaving the health port unbound until they
	// complete or time out.
	bootstrapHealth := sharedhealth.NewBootstrapServer(cfg.Server.HealthAddr)
	go func() {
		if err := bootstrapHealth.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(err, "bootstrap health server failed")
		}
	}()

	deps := wireFMCDependencies(ctx, cfg, logger)
	defer deps.close()

	bootstrapShutdownCtx, bootstrapShutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := bootstrapHealth.Shutdown(bootstrapShutdownCtx); err != nil {
		logger.Error(err, "bootstrap health server shutdown failed")
	}
	bootstrapShutdownCancel()

	var ready atomic.Bool
	servers := buildFMCServers(cfg, deps, &ready, logger)

	runFMCServers(ctx, cancel, sigCh, deps, servers, logger)
	return 0
}
