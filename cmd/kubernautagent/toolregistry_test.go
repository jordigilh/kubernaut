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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	amtools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/alertmanager"
	promtools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/prometheus"
)

// ============================================================================
// buildToolRegistry — characterization tests (RED phase, Wave 5 preflight).
//
// FedRAMP AC-6 (Least Privilege): the tool surface exposed to the LLM MUST be
// scoped to what is actually configured/integrated. An agent with no
// Prometheus/Alertmanager/DataStorage integration configured must not expose
// those tools, minimizing the LLM's action surface.
//
// FedRAMP SC-8 (Transmission Confidentiality): when a TLSCaFile is configured
// for an observability integration, requests must be carried over the
// custom-CA-verified transport. These tests characterize the CURRENT
// behavior when CA loading fails: the error is logged and the tool set is
// still registered using the default transport (fail-open on TLS transport
// construction, not fail-closed on tool registration). This is captured here
// so any future change to that behavior is a deliberate, reviewed decision.
// ============================================================================

func TestBuildToolRegistry_NoIntegrationsConfigured_OnlyRegistersBaselineTool(t *testing.T) {
	cfg := &kaconfig.Config{}

	reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil)

	all := reg.All()
	if len(all) != 1 {
		t.Fatalf("BR-SECURITY-AC6: expected exactly 1 baseline tool (least privilege) when no integrations "+
			"are configured, got %d: %v", len(all), toolNames(all))
	}
	if _, err := reg.Get("todo_write"); err != nil {
		t.Fatalf("expected the baseline todo_write tool to always be registered: %v", err)
	}
}

func TestBuildToolRegistry_PrometheusNotConfigured_DoesNotRegisterPrometheusTools(t *testing.T) {
	cfg := &kaconfig.Config{}

	reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil)

	for _, name := range promtools.AllToolNames {
		if _, err := reg.Get(name); err == nil {
			t.Errorf("BR-SECURITY-AC6: tool %q must not be registered when Prometheus is not configured", name)
		}
	}
}

func TestBuildToolRegistry_PrometheusConfigured_RegistersAllPrometheusTools(t *testing.T) {
	cfg := &kaconfig.Config{}
	cfg.Integrations.Tools.Prometheus.URL = "http://localhost:9090"

	reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil)

	for _, name := range promtools.AllToolNames {
		if _, err := reg.Get(name); err != nil {
			t.Errorf("BR-SECURITY-AC6: expected Prometheus tool %q to be registered when Prometheus.URL is set: %v", name, err)
		}
	}
}

func TestBuildToolRegistry_PrometheusWithValidTLSCaFile_RegistersToolsOverSecureTransport(t *testing.T) {
	caPath := generateTestCACert(t, "Prometheus Test CA")
	cfg := &kaconfig.Config{}
	cfg.Integrations.Tools.Prometheus.URL = "https://prometheus.example.com"
	cfg.Integrations.Tools.Prometheus.TLSCaFile = caPath

	reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil)

	for _, name := range promtools.AllToolNames {
		if _, err := reg.Get(name); err != nil {
			t.Errorf("SC-8: expected Prometheus tool %q to be registered when a valid TLSCaFile is configured: %v", name, err)
		}
	}
}

func TestBuildToolRegistry_PrometheusWithInvalidTLSCaFile_FailsOpenToDefaultTransportButStillRegistersTools(t *testing.T) {
	cfg := &kaconfig.Config{}
	cfg.Integrations.Tools.Prometheus.URL = "https://prometheus.example.com"
	cfg.Integrations.Tools.Prometheus.TLSCaFile = "/nonexistent/ca.crt"

	reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil)

	// SC-8 characterization: a broken CA bundle degrades to the default
	// transport (logged, not fatal) rather than refusing to register the
	// tool set. This test locks in that documented behavior.
	for _, name := range promtools.AllToolNames {
		if _, err := reg.Get(name); err != nil {
			t.Errorf("expected Prometheus tool %q to still be registered despite invalid TLSCaFile (fail-open transport, not fail-closed registration): %v", name, err)
		}
	}
}

func TestBuildToolRegistry_AlertmanagerConfigured_RegistersAllAlertmanagerTools(t *testing.T) {
	cfg := &kaconfig.Config{}
	cfg.Integrations.Tools.Alertmanager.URL = "http://localhost:9093"

	reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil)

	for _, name := range amtools.AllToolNames {
		if _, err := reg.Get(name); err != nil {
			t.Errorf("BR-SECURITY-AC6: expected Alertmanager tool %q to be registered when Alertmanager.URL is set: %v", name, err)
		}
	}
}

func TestBuildToolRegistry_AlertmanagerWithInvalidTLSCaFile_FailsOpenToDefaultTransportButStillRegistersTools(t *testing.T) {
	cfg := &kaconfig.Config{}
	cfg.Integrations.Tools.Alertmanager.URL = "https://alertmanager.example.com"
	cfg.Integrations.Tools.Alertmanager.TLSCaFile = "/nonexistent/ca.crt"

	reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil)

	for _, name := range amtools.AllToolNames {
		if _, err := reg.Get(name); err != nil {
			t.Errorf("expected Alertmanager tool %q to still be registered despite invalid TLSCaFile (fail-open transport, not fail-closed registration): %v", name, err)
		}
	}
}

func toolNames(all []tools.Tool) []string {
	names := make([]string, 0, len(all))
	for _, tl := range all {
		names = append(names, tl.Name())
	}
	return names
}

// ============================================================================
// dsCatalogFetcher.FetchValidator — characterization tests (RED phase, Wave 5).
//
// FedRAMP SI-10 (Information Input Validation): FetchValidator builds the
// allowlist + parameter schema used later to validate/strip LLM-proposed
// workflow selections and parameters (parser.Validator). Both fail-closed
// paths matter for compliance:
//   - an empty catalog must refuse to produce a validator (no workflow can be
//     silently "allowed" when the catalog can't be confirmed empty-by-design)
//   - a workflow whose schema Content fails to parse must have its
//     Parameters stripped (fail-closed), not silently accept unvalidated
//     LLM-supplied parameters for that workflow.
// ============================================================================

const validWorkflowSchemaYAML = `
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: test-workflow
spec:
  version: "1.0.0"
  parameters:
    - name: TARGET_NAMESPACE
      type: string
      required: true
`

// workflowJSON returns a minimal RemediationWorkflow JSON payload satisfying
// the ogen decoder's required-field validation for GET /api/v1/workflows.
func workflowJSON(workflowID, content string) map[string]interface{} {
	return map[string]interface{}{
		"workflowId":            workflowID,
		"workflowName":          "test-workflow",
		"actionType":            "RestartPod",
		"version":               "1.0.0",
		"schemaVersion":         "1.0",
		"name":                  "Test Workflow",
		"description":           map[string]string{"what": "test", "whenToUse": "test"},
		"content":               content,
		"contentHash":           "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"executionEngine":       "tekton",
		"executionBundleDigest": "sha256:aaa",
		"serviceAccountName":    "workflow-runner",
		"labels": map[string]interface{}{
			"severity":    []string{"critical"},
			"component":   []string{"v1/Pod"},
			"environment": []string{"production"},
			"priority":    "P0",
		},
		"status": "Active",
	}
}

func newDSCatalogFetcherForTest(t *testing.T, handler http.Handler) *dsCatalogFetcher {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ogenC, err := ogenclient.NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to build ogen client: %v", err)
	}
	return newDSCatalogFetcher(&dsClients{ogenClient: ogenC}, logr.Discard())
}

func TestFetchValidator_EmptyCatalog_FailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"workflows": []interface{}{}})
	})

	f := newDSCatalogFetcherForTest(t, mux)
	validator, err := f.FetchValidator(context.Background())

	if err == nil {
		t.Fatal("BR-SECURITY-SI10: expected an error when the workflow catalog is empty (fail-closed, no implicit allow-all)")
	}
	if validator != nil {
		t.Fatal("expected a nil validator on empty catalog")
	}
}

func TestFetchValidator_ListWorkflowsFails_ReturnsWrappedError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	f := newDSCatalogFetcherForTest(t, mux)
	validator, err := f.FetchValidator(context.Background())

	if err == nil {
		t.Fatal("expected an error when the DataStorage ListWorkflows call fails")
	}
	if validator != nil {
		t.Fatal("expected a nil validator when ListWorkflows fails")
	}
}

func TestFetchValidator_ValidCatalog_BuildsAllowlistWithCatalogMetadata(t *testing.T) {
	const wfID = "550e8400-e29b-41d4-a716-446655440000"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"workflows": []interface{}{workflowJSON(wfID, validWorkflowSchemaYAML)},
		})
	})

	f := newDSCatalogFetcherForTest(t, mux)
	validator, err := f.FetchValidator(context.Background())
	if err != nil {
		t.Fatalf("expected no error building the validator from a valid catalog: %v", err)
	}
	if validator == nil {
		t.Fatal("expected a non-nil validator")
	}

	meta, ok := validator.GetWorkflowMeta(wfID)
	if !ok {
		t.Fatalf("BR-SECURITY-AC6: expected workflow %q to be part of the allowlist", wfID)
	}
	if meta.ExecutionEngine != "tekton" {
		t.Errorf("expected ExecutionEngine=tekton, got %q", meta.ExecutionEngine)
	}
	if meta.Version != "1.0.0" {
		t.Errorf("expected Version=1.0.0, got %q", meta.Version)
	}
	if meta.ServiceAccountName != "workflow-runner" {
		t.Errorf("expected ServiceAccountName=workflow-runner, got %q", meta.ServiceAccountName)
	}
	if meta.ExecutionBundleDigest != "sha256:aaa" {
		t.Errorf("expected ExecutionBundleDigest=sha256:aaa, got %q", meta.ExecutionBundleDigest)
	}
	if len(meta.Parameters) != 1 || meta.Parameters[0].Name != "TARGET_NAMESPACE" {
		t.Errorf("expected the schema's TARGET_NAMESPACE parameter to be present, got %+v", meta.Parameters)
	}
}

func TestFetchValidator_MalformedSchemaContent_StripsParametersFailClosed(t *testing.T) {
	const wfID = "550e8400-e29b-41d4-a716-446655440001"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			// Content is not a valid RemediationWorkflow CRD envelope (missing
			// apiVersion/kind/metadata.name) — dsschema.Parser.Parse must fail.
			"workflows": []interface{}{workflowJSON(wfID, "not: a-valid-workflow-schema")},
		})
	})

	f := newDSCatalogFetcherForTest(t, mux)
	validator, err := f.FetchValidator(context.Background())
	if err != nil {
		t.Fatalf("BR-SECURITY-SI10: a single workflow's malformed schema must not fail the whole catalog fetch: %v", err)
	}

	meta, ok := validator.GetWorkflowMeta(wfID)
	if !ok {
		t.Fatalf("expected workflow %q to still be present in the allowlist (identity is not affected by schema parse failure)", wfID)
	}
	if len(meta.Parameters) != 0 {
		t.Errorf("BR-SECURITY-SI10: expected Parameters to be stripped (fail-closed) when schema Content fails to parse, got %+v", meta.Parameters)
	}
	// Non-parameter metadata (sourced from the catalog entry itself, not the
	// parsed schema body) must still be populated.
	if meta.ExecutionEngine != "tekton" {
		t.Errorf("expected ExecutionEngine to still be populated from the catalog entry, got %q", meta.ExecutionEngine)
	}
}
