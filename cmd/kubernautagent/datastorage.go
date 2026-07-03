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
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"

	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	sharedtransport "github.com/jordigilh/kubernaut/pkg/shared/transport"
	"github.com/jordigilh/kubernaut/pkg/shared/types"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	k8stools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
)

// dsClients holds DataStorage client instances created once and shared
// between the enricher and the custom tool registry.
type dsClients struct {
	ogenClient *ogenclient.Client
	dsAdapter  *enrichment.DSAdapter
	k8sAdapter *enrichment.K8sAdapter
}

// initDSClients creates the DataStorage adapter clients. Returns nil when
// DataStorage URL is empty or K8s infrastructure is unavailable.
//
// DD-AUTH-014: When a ServiceAccount token is available (sa_token_path config
// or default /var/run/secrets/kubernetes.io/serviceaccount/token), the ogen
// client is configured with a Bearer token transport so that all DS API calls
// (including ListWorkflows for the workflow validator) pass authentication.
func initDSClients(cfg *kaconfig.Config, infra *k8sInfra, dsTokenSource *auth.TokenSource, logger logr.Logger) *dsClients {
	if cfg.Integrations.DataStorage.URL == "" {
		logger.Info("DataStorage URL not configured, DS adapters disabled")
		return nil
	}
	if infra == nil {
		logger.Info("K8s infrastructure unavailable, DS adapters disabled")
		return nil
	}

	dsBase, tlsErr := buildDSBaseTransport(cfg.Integrations.DataStorage.TLS.CAFile, cfg.Integrations.DataStorage.CircuitBreaker)
	if tlsErr != nil {
		logger.Error(tlsErr, "failed to create TLS-aware transport for DS client",
			"ca_file", cfg.Integrations.DataStorage.TLS.CAFile)
		return nil
	}
	if cfg.Integrations.DataStorage.TLS.CAFile != "" {
		logger.Info("DS client configured with custom TLS CA",
			"ca_file", cfg.Integrations.DataStorage.TLS.CAFile)
	}

	const defaultDSClientTimeout = 30 * time.Second

	opts := make([]ogenclient.ClientOption, 0, 1)
	opts = append(opts, ogenclient.WithClient(&http.Client{
		Transport: auth.NewAuthTransport(dsTokenSource, dsBase),
		Timeout:   defaultDSClientTimeout,
	}))

	ogenClient, err := ogenclient.NewClient(cfg.Integrations.DataStorage.URL, opts...)
	if err != nil {
		logger.Error(err, "failed to create DataStorage ogen client", "url", cfg.Integrations.DataStorage.URL)
		return nil
	}
	logger.Info("DataStorage clients initialized", "url", cfg.Integrations.DataStorage.URL)
	return &dsClients{
		ogenClient: ogenClient,
		dsAdapter:  enrichment.NewDSAdapter(ogenClient),
		k8sAdapter: newK8sAdapterWithLogger(infra, logger),
	}
}

func newK8sAdapterWithLogger(infra *k8sInfra, logger logr.Logger) *enrichment.K8sAdapter {
	a := enrichment.NewK8sAdapter(infra.dynClient, infra.mapper)
	a.SetLogger(logger.WithName("k8s-adapter"))
	kindIndex, err := k8stools.BuildKindIndex(infra.clientset.Discovery())
	if err != nil {
		logger.Info("failed to build kind index for K8s adapter, using empty index", "error", err)
		kindIndex = make(map[string]schema.GroupKind)
	}
	a.SetKindIndex(kindIndex)
	return a
}

// buildDSBaseTransport creates the base HTTP transport for the DataStorage
// client. When caFile is set, uses a custom TLS transport with the specified CA;
// otherwise falls back to the shared default transport with retry.
// Issue #951: Wire DataStorageConfig.TLS.CAFile to DS HTTP client.
func buildDSBaseTransport(caFile string, cbCfg types.LLMCircuitBreaker) (http.RoundTripper, error) {
	var base http.RoundTripper
	if caFile != "" {
		tlsTransport, err := sharedtls.NewTLSTransport(caFile)
		if err != nil {
			return nil, fmt.Errorf("DS TLS transport: %w", err)
		}
		base = sharedtransport.NewRetryTransport(tlsTransport, sharedtransport.DefaultRetryConfig())
	} else {
		var err error
		base, err = sharedtls.DefaultBaseTransportWithRetry()
		if err != nil {
			return nil, err
		}
	}
	return sharedtransport.NewCircuitBreakerTransport(base, sharedtransport.CircuitBreakerConfig{
		Enabled:          cbCfg.Enabled,
		Name:             "datastorage",
		MaxRequests:      cbCfg.MaxRequests,
		Interval:         cbCfg.Interval,
		Timeout:          cbCfg.Timeout,
		FailureThreshold: cbCfg.FailureThreshold,
		FailureRatio:     cbCfg.FailureRatio,
	}), nil
}

// buildEnricher creates the enrichment.Enricher when DS clients are available.
// ADR-056: attaches LabelDetector so detected_labels are populated during enrichment.
// #704: wires RetryConfig from config for HAPI-aligned owner chain retry+fail-hard.
func buildEnricher(cfg *kaconfig.Config, ds *dsClients, infra *k8sInfra, auditStore audit.AuditStore, logger logr.Logger) *enrichment.Enricher {
	if ds == nil {
		return nil
	}
	e := enrichment.NewEnricher(ds.k8sAdapter, ds.dsAdapter, auditStore, logger)
	if infra != nil && infra.dynClient != nil {
		e.WithLabelDetector(enrichment.NewLabelDetector(infra.dynClient, infra.mapper, logger.WithName("label-detector")))
		logger.Info("label detector enabled (ADR-056)")
	}
	e.WithRetryConfig(enrichment.RetryConfig{
		MaxRetries:  cfg.AI.Enrichment.MaxRetries,
		BaseBackoff: cfg.AI.Enrichment.BaseBackoff,
	})
	logger.Info("enrichment retry config wired (#704)",
		"max_retries", cfg.AI.Enrichment.MaxRetries,
		"base_backoff", cfg.AI.Enrichment.BaseBackoff,
	)
	return e
}

// buildSanitizationPipeline creates the sanitization pipeline with G4 (credential scrub),
// K8S-SECRET (JSON Secret redaction), and I1 (injection patterns) stages per DD-HAPI-019-003.
// Returns nil when all stages are disabled.
func buildSanitizationPipeline(cfg *kaconfig.Config, logger logr.Logger) *sanitization.Pipeline {
	var stages []sanitization.Stage
	if cfg.AI.Safety.Sanitization.CredentialScrubEnabled {
		stages = append(stages, sanitization.NewCredentialSanitizer())
	}
	if cfg.AI.Safety.Sanitization.SecretRedactionEnabled {
		stages = append(stages, sanitization.NewSecretSanitizer())
	}
	if cfg.AI.Safety.Sanitization.InjectionPatternsEnabled {
		stages = append(stages, sanitization.NewInjectionSanitizer(nil))
	}
	if len(stages) == 0 {
		logger.Info("sanitization pipeline disabled")
		return nil
	}
	logger.Info("sanitization pipeline enabled", "stages", len(stages))
	return sanitization.NewPipeline(stages...)
}

// buildAuditStore creates a BufferedDSAuditStore (DD-AUDIT-002 aligned) when audit
// is enabled and DS is available, falling back to NopAuditStore otherwise.
// Uses the same OpenAPIClientAdapter + BufferedAuditStore stack as every other
// platform service. Auth transport is shared with initDSClients (same SA token)
// to guarantee identical authentication behavior.
func buildAuditStore(cfg *kaconfig.Config, dsTokenSource *auth.TokenSource, logger logr.Logger) (audit.AuditStore, func()) {
	nop := func() {}
	if !cfg.Runtime.Audit.Enabled || cfg.Integrations.DataStorage.URL == "" {
		logger.Info("audit store disabled (nop)")
		return audit.NopAuditStore{}, nop
	}

	auditBase, tlsErr := sharedtls.DefaultBaseTransport()
	if tlsErr != nil {
		logger.Error(tlsErr, "failed to create TLS-aware transport for audit store")
		return audit.NopAuditStore{}, nop
	}

	dsClient, err := sharedaudit.NewOpenAPIClientAdapterWithTransport(
		cfg.Integrations.DataStorage.URL, 5*time.Second, auth.NewAuthTransport(dsTokenSource, auditBase),
	)
	if err != nil {
		logger.Error(err, "failed to create DS audit client, falling back to nop")
		return audit.NopAuditStore{}, nop
	}

	var storeOpts []audit.BufferedDSAuditStoreOption
	if cfg.Runtime.Audit.FlushIntervalSeconds > 0 {
		storeOpts = append(storeOpts, audit.WithFlushInterval(
			time.Duration(cfg.Runtime.Audit.FlushIntervalSeconds*float64(time.Second))))
	}
	if cfg.Runtime.Audit.BufferSize > 0 {
		storeOpts = append(storeOpts, audit.WithBufferSize(cfg.Runtime.Audit.BufferSize))
	}
	if cfg.Runtime.Audit.BatchSize > 0 {
		storeOpts = append(storeOpts, audit.WithBatchSize(cfg.Runtime.Audit.BatchSize))
	}

	store, err := audit.NewBufferedDSAuditStore(dsClient, logger, storeOpts...)
	if err != nil {
		logger.Error(err, "failed to create buffered audit store, falling back to nop")
		return audit.NopAuditStore{}, nop
	}
	logger.Info("audit store enabled (buffered, DD-AUDIT-002 aligned)",
		"ds_url", cfg.Integrations.DataStorage.URL)
	return store, func() {
		if closeErr := store.Close(); closeErr != nil {
			logger.Error(closeErr, "audit store close error")
		}
	}
}

// buildSummarizer creates a tool output summarizer when the threshold is positive.
// When MaxToolOutputSize is configured, it enables pre-truncation to prevent
// the summarizer's own LLM call from exceeding context window limits (#752).
func buildSummarizer(llmClient llm.Client, cfg *kaconfig.Config, logger logr.Logger) *summarizer.Summarizer {
	if cfg.AI.Summarizer.Threshold <= 0 {
		logger.Info("summarizer disabled (threshold <= 0)")
		return nil
	}
	if cfg.AI.Summarizer.MaxToolOutputSize > 0 {
		logger.Info("summarizer enabled with pre-truncation",
			"threshold", cfg.AI.Summarizer.Threshold,
			"max_tool_output_size", cfg.AI.Summarizer.MaxToolOutputSize)
		return summarizer.NewWithMaxInput(llmClient, cfg.AI.Summarizer.Threshold, cfg.AI.Summarizer.MaxToolOutputSize)
	}
	logger.Info("summarizer enabled", "threshold", cfg.AI.Summarizer.Threshold)
	return summarizer.New(llmClient, cfg.AI.Summarizer.Threshold)
}

// buildAnomalyDetector creates the I7 anomaly detector from config thresholds.
func buildAnomalyDetector(cfg *kaconfig.Config, logger logr.Logger) *investigator.AnomalyDetector {
	ac := investigator.AnomalyConfig{
		MaxToolCallsPerTool: cfg.AI.Safety.Anomaly.MaxToolCallsPerTool,
		MaxTotalToolCalls:   cfg.AI.Safety.Anomaly.MaxTotalToolCalls,
		MaxRepeatedFailures: cfg.AI.Safety.Anomaly.MaxRepeatedFailures,
		ExemptPrefixes:      cfg.AI.Safety.Anomaly.ExemptPrefixes,
	}
	logger.Info("anomaly detector enabled",
		"maxToolCallsPerTool", ac.MaxToolCallsPerTool,
		"maxTotalToolCalls", ac.MaxTotalToolCalls,
		"maxRepeatedFailures", ac.MaxRepeatedFailures,
		"exemptPrefixes", ac.ExemptPrefixes,
	)
	return investigator.NewAnomalyDetector(ac, nil)
}
