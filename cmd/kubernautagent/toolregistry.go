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
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	dsschema "github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	fleetclient "github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
	fleetregistry "github.com/jordigilh/kubernaut/pkg/fleet/registry"
	amtools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/alertmanager"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/investigation"
	k8stools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	logtools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/logs"
	promtools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/prometheus"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

// buildToolRegistry creates and populates the tool registry with all available tool sets.
func buildToolRegistry(cfg *kaconfig.Config, logger logr.Logger, infra *k8sInfra, ds *dsClients, auditStore audit.AuditStore) *registry.Registry {
	reg := registry.New()

	if infra != nil {
		registerK8sTools(reg, infra, logger, auditStore)
	}

	if cfg.Integrations.Tools.Prometheus.URL != "" {
		registerPrometheusTools(reg, cfg, logger)
	}

	if cfg.Integrations.Tools.Alertmanager.URL != "" {
		registerAlertmanagerTools(reg, cfg, logger)
	}

	if ds != nil {
		custom.RegisterAll(reg, ds.ogenClient, ds.dsAdapter, ds.k8sAdapter, logger)
		logger.Info("registered custom tools", "count", len(custom.AllToolNames))
	}

	reg.Register(investigation.NewTodoWriteTool())
	logger.Info("registered TodoWrite tool")

	logger.Info("tool registry ready", "total_tools", len(reg.All()))
	return reg
}

// buildTLSAwareTransport builds an SA-bearer-authenticated http.RoundTripper
// backed by a custom CA bundle for the given tlsCaFile. Returns nil (fail-open:
// the caller falls back to the client's default transport) and logs an error
// when the CA bundle cannot be loaded — integrations tools are best-effort
// and must not block agent startup.
func buildTLSAwareTransport(tlsCaFile string, logger logr.Logger, label string) http.RoundTripper {
	base, err := sharedtls.NewTLSTransport(tlsCaFile)
	if err != nil {
		logger.Error(err, "failed to create TLS transport", "integration", label, "ca_file", tlsCaFile)
		return nil
	}
	logger.Info("client configured with TLS + SA bearer auth", "integration", label, "ca_file", tlsCaFile)
	return auth.NewAuthTransport(auth.NewDefaultTokenSource(), base)
}

// registerPrometheusTools builds the Prometheus client (with optional custom
// CA bundle) and registers its 8 tools. Best-effort: logs and skips
// registration on any construction failure rather than blocking startup.
func registerPrometheusTools(reg *registry.Registry, cfg *kaconfig.Config, logger logr.Logger) {
	promCfg := promtools.ClientConfig{
		URL:       cfg.Integrations.Tools.Prometheus.URL,
		Timeout:   cfg.Integrations.Tools.Prometheus.Timeout,
		SizeLimit: cfg.Integrations.Tools.Prometheus.SizeLimit,
	}
	if cfg.Integrations.Tools.Prometheus.TLSCaFile != "" {
		promCfg.Transport = buildTLSAwareTransport(cfg.Integrations.Tools.Prometheus.TLSCaFile, logger, "Prometheus")
	}
	promClient, promErr := promtools.NewClient(promCfg)
	if promErr != nil {
		logger.Error(promErr, "failed to create Prometheus client")
		return
	}
	for _, t := range promtools.NewAllTools(promClient) {
		reg.Register(t)
	}
	logger.Info("registered Prometheus tools", "count", len(promtools.AllToolNames))
}

// registerAlertmanagerTools builds the Alertmanager client (with optional
// custom CA bundle) and registers its tools. Best-effort: logs and skips
// registration on any construction failure rather than blocking startup.
func registerAlertmanagerTools(reg *registry.Registry, cfg *kaconfig.Config, logger logr.Logger) {
	amCfg := amtools.ClientConfig{
		URL:       cfg.Integrations.Tools.Alertmanager.URL,
		Timeout:   cfg.Integrations.Tools.Alertmanager.Timeout,
		SizeLimit: cfg.Integrations.Tools.Alertmanager.SizeLimit,
	}
	if cfg.Integrations.Tools.Alertmanager.TLSCaFile != "" {
		amCfg.Transport = buildTLSAwareTransport(cfg.Integrations.Tools.Alertmanager.TLSCaFile, logger, "Alertmanager")
	}
	amClient, amErr := amtools.NewClient(amCfg)
	if amErr != nil {
		logger.Error(amErr, "failed to create Alertmanager client")
		return
	}
	for _, t := range amtools.NewAllTools(amClient) {
		reg.Register(t)
	}
	logger.Info("registered Alertmanager tools", "count", len(amtools.AllToolNames))
}

// resolveAlignmentCheckConfig returns the effective AlignmentCheckConfig.
// When fleet mode is active and defines its own alignment check, the fleet
// override takes precedence over the global ai.alignmentCheck. This allows
// operators to enforce cross-model shadow evaluation specifically for
// multi-cluster investigations where prompt injection risk is higher.
func resolveAlignmentCheckConfig(cfg *kaconfig.Config) kaconfig.AlignmentCheckConfig {
	fleetCfg := cfg.Integrations.Fleet
	if fleetCfg.GatewayType != "" && fleetCfg.Endpoint != "" && fleetCfg.AlignmentCheck != nil {
		return *fleetCfg.AlignmentCheck
	}
	return cfg.AI.AlignmentCheck
}

// registerFleetTools connects to the MCP Gateway, creates a GatewayDiscoverer
// for the configured gateway type, pre-scopes tools for the target cluster,
// and registers list_clusters + list_tools_for_cluster for LLM-driven discovery.
// Returns the fleet client (must be closed on shutdown) and the registered tool names,
// or nil if fleet is disabled.
//
// Authority: ADR-068 decision #11
func registerFleetTools(ctx context.Context, cfg *kaconfig.Config, reg *registry.Registry, logger logr.Logger) (*fleetclient.ResilientClient, []string) {
	gatewayType := cfg.Integrations.Fleet.GatewayType
	endpoint := cfg.Integrations.Fleet.Endpoint
	if gatewayType == "" || endpoint == "" {
		return nil, nil
	}

	fleetLog := logger.WithName("fleet")
	fleetLog.Info("connecting to MCP Gateway for fleet tool discovery",
		"endpoint", endpoint, "gatewayType", gatewayType)

	var opts []fleetclient.Option
	if cfg.Integrations.Fleet.OAuth2.Enabled {
		basePath := "/etc/kubernautagent/fleet-oauth2"
		if cfg.Integrations.Fleet.OAuth2.CredentialsSecretRef != "" {
			basePath = "/etc/kubernautagent/" + cfg.Integrations.Fleet.OAuth2.CredentialsSecretRef
		}
		reloadCfg := fleetclient.ReloadableOAuth2Config{
			TokenURL:         cfg.Integrations.Fleet.OAuth2.TokenURL,
			ClientIDPath:     basePath + "/client-id",
			ClientSecretPath: basePath + "/client-secret",
			Scopes:           fleetclient.DefaultFleetScopes(cfg.Integrations.Fleet.OAuth2.Scopes),
			TokenTimeout:     10 * time.Second,
		}
		opts = append(opts, fleetclient.WithReloadableOAuth2Transport(reloadCfg, fleetLog)) //nolint:contextcheck // OAuth2 token source refresh runs as a background reload, independent of any single request
		fleetLog.Info("fleet OAuth2 authentication configured (hot-reloadable)",
			"tokenURL", cfg.Integrations.Fleet.OAuth2.TokenURL,
			"secretPath", basePath)
	}

	resilienceCfg := fleetclient.DefaultResilienceConfig()
	resilientClient, err := fleetclient.NewResilient(ctx, endpoint, resilienceCfg, fleetLog, opts...)
	if err != nil {
		// #1553: keep (don't discard) the disconnected client — the fleet
		// readiness gate attaches an MCPClientProber to it so the periodic
		// probe keeps retrying and the "fleet" readyz check correctly
		// reports NotReady until reconnect, instead of the client being
		// silently lost with no path back to healthy short of a restart.
		fleetLog.Error(err, "failed to connect to MCP Gateway at startup; readiness will report NotReady "+
			"and keep retrying in the background; fleet tools unavailable until reconnect")
		return resilientClient, nil
	}

	session := resilientClient.Session()

	discoverer, err := fleetclient.NewDiscoverer(fleetregistry.MCPGatewayType(gatewayType), session)
	if err != nil {
		fleetLog.Error(err, "failed to create GatewayDiscoverer", "gatewayType", gatewayType)
		_ = resilientClient.Close()
		return nil, nil
	}

	listClustersTool := fleetclient.NewListClustersTool(discoverer)
	listToolsTool := fleetclient.NewListToolsForClusterTool(discoverer, reg, session)

	reg.Register(listClustersTool)
	reg.Register(listToolsTool)

	toolNames := make([]string, 0, 2)
	toolNames = append(toolNames, listClustersTool.Name(), listToolsTool.Name())

	fleetLog.Info("registered fleet discovery tools",
		"tools", toolNames, "endpoint", endpoint, "gatewayType", gatewayType)
	return resilientClient, toolNames
}

// fleetReadinessProbeInterval controls how often the #1553 Fleet readiness
// gate re-probes its dependencies once started (mirrors GW/RO/EM/SP/WE/AF).
const fleetReadinessProbeInterval = 15 * time.Second

// wireFleetReadinessGate builds and starts the #1553 Fleet dependency
// readiness gate (ADR-068 decision #11, BR-INTEGRATION-054): once fleet
// mode is configured, KA's pod-wide readyz must fail closed when the MCP
// Gateway becomes unreachable, instead of the previous fail-open behavior
// of only logging an error. KA has no scope-checker or cluster-registry
// dependency (unlike GW/RO/SP), so its gate only ever carries an
// MCPClientProber. Returns nil when fleetClient is nil (registerFleetTools
// only returns a non-nil client when fleet mode is configured). The
// caller registers the returned Gate's Ready method into readinessHandler
// and must Stop() it on shutdown.
func wireFleetReadinessGate(ctx context.Context, fleetClient *fleetclient.ResilientClient, logger logr.Logger) *readiness.Gate {
	if fleetClient == nil {
		return nil
	}

	prober := &readiness.MCPClientProber{Client: fleetClient}
	gate := readiness.NewGate(fleetReadinessProbeInterval, logger.WithName("fleet-readiness"), prober)
	gate.Start(ctx)
	logger.Info("Fleet readiness gate started", "ready", gate.Ready())
	return gate
}

func registerK8sTools(reg *registry.Registry, infra *k8sInfra, logger logr.Logger, auditStore audit.AuditStore) {
	kindIndex, err := k8stools.BuildKindIndex(infra.clientset.Discovery())
	if err != nil {
		logger.Info("failed to build kind index, using empty index", "error", err)
		kindIndex = make(map[string]schema.GroupKind)
	}
	resolver := k8stools.NewDynamicResolver(infra.dynClient, infra.mapper, kindIndex, logger.WithName("k8s-resolver"),
		k8stools.WithSecretAccessObserver(newSecretAccessObserver(auditStore, logger.WithName("secret-access-audit"))))

	for _, t := range k8stools.NewAllTools(infra.clientset, resolver) {
		reg.Register(t)
	}
	logger.Info("registered K8s tools", "count", len(k8stools.AllToolNames))

	reg.Register(logtools.NewFetchPodLogsTool(infra.clientset))
	logger.Info("registered fetch_pod_logs tool")

	mc, mcErr := metricsclient.NewForConfig(infra.kubeConfig)
	if mcErr != nil {
		logger.Error(mcErr, "failed to create metrics client, metrics tools will not be registered")
	} else {
		for _, t := range k8stools.NewMetricsTools(k8stools.NewMetricsClient(mc)) {
			reg.Register(t)
		}
		logger.Info("registered metrics tools", "count", len(k8stools.MetricsToolNames))
	}

	npc := k8stools.NewNodeProxyClient(infra.clientset)
	for _, t := range k8stools.NewNodeProxyTools(npc, 30000) {
		reg.Register(t)
	}
	logger.Info("registered node proxy tools", "count", len(k8stools.NodeProxyToolNames))
}

// newSecretAccessObserver builds the k8stools.SecretAccessObserver wired into
// the K8s resource resolver (GAP-13, Issue #1505). It is the detective
// control compensating for KubernautAgent's intentionally broad read RBAC on
// Secrets (see docs/services/stateless/kubernaut-agent/security-configuration.md):
// every Secret Get/List — success or failure — becomes an independently
// queryable aiagent.secret.accessed audit event, correlated to the
// investigation via the correlationID set on ctx by
// session.Manager.launchInvestigation.
func newSecretAccessObserver(auditStore audit.AuditStore, logger logr.Logger) func(ctx context.Context, verb, name, namespace string, err error) {
	return func(ctx context.Context, verb, name, namespace string, accessErr error) {
		if auditStore == nil {
			return
		}
		correlationID, _ := audit.CorrelationIDFromContext(ctx)

		event := audit.NewEvent(audit.EventTypeSecretAccessed, correlationID)
		event.EventAction = audit.ActionSecretAccessed
		event.EventOutcome = audit.OutcomeSuccess
		event.Data["verb"] = verb
		if namespace != "" {
			event.Data["namespace"] = namespace
		}
		if name != "" {
			event.Data["secret_name"] = name
		}
		if accessErr != nil {
			event.EventOutcome = audit.OutcomeFailure
			event.Data["outcome_detail"] = accessErr.Error()
		}
		audit.StoreBestEffort(ctx, auditStore, event, logger)
	}
}

// dsCatalogFetcher implements investigator.CatalogFetcher by querying
// DataStorage on every call. This removes the boot-time blocking fetch
// that caused #665 (CrashLoopBackOff when the catalog was not yet seeded).
//
// Per DD-HAPI-002 (v1.1+), KA is the sole workflow validator. The catalog
// is fetched per-request so KA always validates against the current catalog
// without needing a restart when workflows are added/removed.
type dsCatalogFetcher struct {
	ds     *dsClients
	logger logr.Logger
}

func newDSCatalogFetcher(ds *dsClients, logger logr.Logger) *dsCatalogFetcher {
	return &dsCatalogFetcher{ds: ds, logger: logger}
}

func (f *dsCatalogFetcher) FetchValidator(ctx context.Context) (*parser.Validator, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := f.ds.ogenClient.ListWorkflows(fetchCtx, ogenclient.ListWorkflowsParams{})
	if err != nil {
		return nil, fmt.Errorf("ListWorkflows call failed: %w", err)
	}

	wlr, ok := resp.(*ogenclient.WorkflowListResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected ListWorkflows response type %T", resp)
	}

	ids := make([]string, 0, len(wlr.Workflows))
	for _, w := range wlr.Workflows {
		if w.WorkflowId.Set {
			ids = append(ids, w.WorkflowId.Value.String())
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("workflow catalog returned 0 workflows")
	}

	validator := parser.NewValidator(ids)
	schemaParser := dsschema.NewParser()
	for _, w := range wlr.Workflows {
		if !w.WorkflowId.Set {
			continue
		}
		wfID := w.WorkflowId.Value.String()
		validator.SetWorkflowMeta(wfID, buildWorkflowMeta(w, schemaParser, f.logger))
	}

	f.logger.Info("workflow catalog fetched (DD-HAPI-002: per-request validation)",
		"allowed_workflows", len(ids))
	return validator, nil
}

// buildWorkflowMeta translates a single catalog RemediationWorkflow entry
// into the parser.WorkflowMeta used for parameter/schema validation. Schema
// parse failures are logged and fail-closed (no Parameters set, stripping
// all LLM-supplied params for that workflow) rather than aborting the whole
// catalog fetch.
func buildWorkflowMeta(w ogenclient.RemediationWorkflow, schemaParser *dsschema.Parser, logger logr.Logger) parser.WorkflowMeta {
	meta := parser.WorkflowMeta{
		ExecutionEngine: w.ExecutionEngine,
		Version:         w.Version,
		Component:       append([]string(nil), w.Labels.GetComponent()...),
	}
	if w.ExecutionBundle.Set {
		meta.ExecutionBundle = w.ExecutionBundle.Value
	}
	if w.ExecutionBundleDigest.Set {
		meta.ExecutionBundleDigest = w.ExecutionBundleDigest.Value
	}
	if w.ServiceAccountName.Set {
		meta.ServiceAccountName = w.ServiceAccountName.Value
	}
	if w.Content != "" {
		applyParsedSchemaMeta(&meta, w, schemaParser, logger)
	}
	return meta
}

// applyParsedSchemaMeta parses w.Content and, on success, populates meta's
// schema-derived fields: Parameters/DeclaredParameterNames (always),
// Dependencies (always, nil when the schema declares none), and Resources
// (best-effort -- an ExtractResources failure is logged but does not block
// the other fields, since a malformed execution.resources section is
// independent of parameter/dependency validity). A Parse failure leaves all
// of these fields at their zero value (fail-closed: no unvalidated data).
func applyParsedSchemaMeta(meta *parser.WorkflowMeta, w ogenclient.RemediationWorkflow, schemaParser *dsschema.Parser, logger logr.Logger) {
	wfID := ""
	if w.WorkflowId.Set {
		wfID = w.WorkflowId.Value.String()
	}

	parsed, err := schemaParser.Parse(w.Content)
	if err != nil {
		logger.Error(err, "failed to parse workflow schema Content, parameter validation will strip all LLM params (fail-closed)",
			"workflow_id", wfID)
		return
	}

	meta.Parameters = parsed.Parameters
	meta.DeclaredParameterNames = declaredParameterNames(parsed.Parameters)
	meta.Dependencies = schemaParser.ExtractDependencies(parsed)

	resources, resErr := schemaParser.ExtractResources(parsed)
	if resErr != nil {
		logger.Error(resErr, "failed to extract execution.resources from workflow schema, WorkflowMeta.Resources left nil",
			"workflow_id", wfID)
		return
	}
	meta.Resources = resources
}

// declaredParameterNames builds the defense-in-depth allowlist WorkflowExecution
// uses to strip undeclared parameters before injecting them into execution
// resources (Issue #1661 Change 11a, mirrors #243's WorkflowQuerier semantics:
// nil means no schema available, empty means the schema declares zero params).
func declaredParameterNames(params []models.WorkflowParameter) map[string]bool {
	names := make(map[string]bool, len(params))
	for _, p := range params {
		names[p.Name] = true
	}
	return names
}
