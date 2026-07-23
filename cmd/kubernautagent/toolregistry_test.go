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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

func toolNames(all []tools.Tool) []string {
	names := make([]string, 0, len(all))
	for _, tl := range all {
		names = append(names, tl.Name())
	}
	return names
}

var _ = Describe("buildToolRegistry", func() {
	It("BR-SECURITY-AC6: registers only the baseline tool (least privilege) when no integrations are configured", func() {
		cfg := &kaconfig.Config{}

		reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil, nil)

		all := reg.All()
		Expect(all).To(HaveLen(1), "expected exactly 1 baseline tool (least privilege), got %v", toolNames(all))
		_, err := reg.Get("todo_write")
		Expect(err).NotTo(HaveOccurred(), "expected the baseline todo_write tool to always be registered")
	})

	It("BR-SECURITY-AC6: does not register Prometheus tools when Prometheus is not configured", func() {
		cfg := &kaconfig.Config{}

		reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil, nil)

		for _, name := range promtools.AllToolNames {
			_, err := reg.Get(name)
			Expect(err).To(HaveOccurred(), "tool %q must not be registered when Prometheus is not configured", name)
		}
	})

	It("BR-SECURITY-AC6: registers all Prometheus tools when Prometheus.URL is set", func() {
		cfg := &kaconfig.Config{}
		cfg.Integrations.Tools.Prometheus.URL = "http://localhost:9090"

		reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil, nil)

		for _, name := range promtools.AllToolNames {
			_, err := reg.Get(name)
			Expect(err).NotTo(HaveOccurred(), "expected Prometheus tool %q to be registered when Prometheus.URL is set", name)
		}
	})

	It("SC-8: registers Prometheus tools over a secure transport when a valid TLSCaFile is configured", func() {
		caPath := generateTestCACert(GinkgoTB(), "Prometheus Test CA")
		cfg := &kaconfig.Config{}
		cfg.Integrations.Tools.Prometheus.URL = "https://prometheus.example.com"
		cfg.Integrations.Tools.Prometheus.TLSCaFile = caPath

		reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil, nil)

		for _, name := range promtools.AllToolNames {
			_, err := reg.Get(name)
			Expect(err).NotTo(HaveOccurred(), "expected Prometheus tool %q to be registered when a valid TLSCaFile is configured", name)
		}
	})

	// SC-8 characterization: a broken CA bundle degrades to the default
	// transport (logged, not fatal) rather than refusing to register the
	// tool set. This test locks in that documented behavior.
	It("SC-8: still registers Prometheus tools (fail-open transport, not fail-closed registration) despite an invalid TLSCaFile", func() {
		cfg := &kaconfig.Config{}
		cfg.Integrations.Tools.Prometheus.URL = "https://prometheus.example.com"
		cfg.Integrations.Tools.Prometheus.TLSCaFile = "/nonexistent/ca.crt"

		reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil, nil)

		for _, name := range promtools.AllToolNames {
			_, err := reg.Get(name)
			Expect(err).NotTo(HaveOccurred(), "expected Prometheus tool %q to still be registered despite invalid TLSCaFile", name)
		}
	})

	It("BR-SECURITY-AC6: registers all Alertmanager tools when Alertmanager.URL is set", func() {
		cfg := &kaconfig.Config{}
		cfg.Integrations.Tools.Alertmanager.URL = "http://localhost:9093"

		reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil, nil)

		for _, name := range amtools.AllToolNames {
			_, err := reg.Get(name)
			Expect(err).NotTo(HaveOccurred(), "expected Alertmanager tool %q to be registered when Alertmanager.URL is set", name)
		}
	})

	It("still registers Alertmanager tools (fail-open transport, not fail-closed registration) despite an invalid TLSCaFile", func() {
		cfg := &kaconfig.Config{}
		cfg.Integrations.Tools.Alertmanager.URL = "https://alertmanager.example.com"
		cfg.Integrations.Tools.Alertmanager.TLSCaFile = "/nonexistent/ca.crt"

		reg := buildToolRegistry(cfg, logr.Discard(), nil, nil, nil, nil)

		for _, name := range amtools.AllToolNames {
			_, err := reg.Get(name)
			Expect(err).NotTo(HaveOccurred(), "expected Alertmanager tool %q to still be registered despite invalid TLSCaFile", name)
		}
	})
})

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

// workflowSchemaYAMLWithDepsAndResources exercises the three fields
// UT-KA-337-001 covers: schema.Dependencies (secrets/configMaps),
// execution.resources (requests/limits), and a multi-entry parameter list
// (source for DeclaredParameterNames).
const workflowSchemaYAMLWithDepsAndResources = `
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: test-workflow-deps-resources
spec:
  version: "1.0.0"
  execution:
    engine: job
    resources:
      requests:
        cpu: "100m"
      limits:
        cpu: "500m"
  dependencies:
    secrets:
      - name: db-creds
  parameters:
    - name: TARGET_NAMESPACE
      type: string
      required: true
    - name: REPLICAS
      type: integer
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

func newDSCatalogFetcherForTest(t testing.TB, handler http.Handler) *dsCatalogFetcher {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ogenC, err := ogenclient.NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to build ogen client: %v", err)
	}
	return newDSCatalogFetcher(&dsClients{ogenClient: ogenC}, logr.Discard())
}

var _ = Describe("dsCatalogFetcher.FetchValidator", func() {
	It("BR-SECURITY-SI10: fails closed when the workflow catalog is empty", func() {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"workflows": []interface{}{}})
		})

		f := newDSCatalogFetcherForTest(GinkgoTB(), mux)
		validator, err := f.FetchValidator(context.Background())

		Expect(err).To(HaveOccurred(), "expected an error when the workflow catalog is empty (fail-closed, no implicit allow-all)")
		Expect(validator).To(BeNil())
	})

	It("returns a wrapped error when the DataStorage ListWorkflows call fails", func() {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		f := newDSCatalogFetcherForTest(GinkgoTB(), mux)
		validator, err := f.FetchValidator(context.Background())

		Expect(err).To(HaveOccurred(), "expected an error when the DataStorage ListWorkflows call fails")
		Expect(validator).To(BeNil())
	})

	It("builds an allowlist with catalog metadata from a valid catalog", func() {
		const wfID = "550e8400-e29b-41d4-a716-446655440000"
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"workflows": []interface{}{workflowJSON(wfID, validWorkflowSchemaYAML)},
			})
		})

		f := newDSCatalogFetcherForTest(GinkgoTB(), mux)
		validator, err := f.FetchValidator(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(validator).NotTo(BeNil())

		meta, ok := validator.GetWorkflowMeta(wfID)
		Expect(ok).To(BeTrue(), "BR-SECURITY-AC6: expected workflow %q to be part of the allowlist", wfID)
		Expect(meta.ExecutionEngine).To(Equal("tekton"))
		Expect(meta.Version).To(Equal("1.0.0"))
		Expect(meta.ServiceAccountName).To(Equal("workflow-runner"))
		Expect(meta.ExecutionBundleDigest).To(Equal("sha256:aaa"))
		Expect(meta.Parameters).To(HaveLen(1))
		Expect(meta.Parameters[0].Name).To(Equal("TARGET_NAMESPACE"))
	})

	// UT-KA-337-001 (Issue #1661 Change 11a, DD-WORKFLOW-018): WorkflowMeta must
	// surface Dependencies/Resources/DeclaredParameterNames -- schema-derived
	// data buildWorkflowMeta's schemaParser already computes during parameter
	// validation but previously discarded after populating only Parameters.
	// AA's SelectedWorkflow needs these three fields to build the CRD-embedded
	// execution snapshot (Change 11b-11e) without a second, redundant DS fetch
	// from WorkflowExecution.
	It("UT-KA-337-001: WorkflowMeta surfaces Dependencies/Resources/DeclaredParameterNames from schema content", func() {
		const wfID = "550e8400-e29b-41d4-a716-446655440010"
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"workflows": []interface{}{workflowJSON(wfID, workflowSchemaYAMLWithDepsAndResources)},
			})
		})

		f := newDSCatalogFetcherForTest(GinkgoTB(), mux)
		validator, err := f.FetchValidator(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(validator).NotTo(BeNil())

		meta, ok := validator.GetWorkflowMeta(wfID)
		Expect(ok).To(BeTrue())

		Expect(meta.Dependencies).NotTo(BeNil(), "Dependencies must be extracted from schema content")
		Expect(meta.Dependencies.Secrets).To(HaveLen(1))
		Expect(meta.Dependencies.Secrets[0].Name).To(Equal("db-creds"))

		Expect(meta.Resources).NotTo(BeNil(), "Resources must be extracted from schema content")
		Expect(meta.Resources.Requests.Cpu().String()).To(Equal("100m"))
		Expect(meta.Resources.Limits.Cpu().String()).To(Equal("500m"))

		Expect(meta.DeclaredParameterNames).To(HaveLen(2))
		Expect(meta.DeclaredParameterNames).To(HaveKey("TARGET_NAMESPACE"))
		Expect(meta.DeclaredParameterNames).To(HaveKey("REPLICAS"))

		// Issue #1661 Change 12 (DD-WORKFLOW-018): ActionType/WorkflowName are
		// required, non-optional fields on the DS catalog response (see
		// workflowJSON's actionType/workflowName), so buildWorkflowMeta must
		// always copy them verbatim -- no schema-parsing dependency, unlike
		// Dependencies/Resources/DeclaredParameterNames above.
		Expect(meta.ActionType).To(Equal("RestartPod"))
		Expect(meta.WorkflowName).To(Equal("test-workflow"))
	})

	It("strips parameters (fail-closed) when a workflow's schema content is malformed, but keeps it in the allowlist", func() {
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

		f := newDSCatalogFetcherForTest(GinkgoTB(), mux)
		validator, err := f.FetchValidator(context.Background())
		Expect(err).NotTo(HaveOccurred(), "BR-SECURITY-SI10: a single workflow's malformed schema must not fail the whole catalog fetch")

		meta, ok := validator.GetWorkflowMeta(wfID)
		Expect(ok).To(BeTrue(), "expected workflow %q to still be present in the allowlist (identity is not affected by schema parse failure)", wfID)
		Expect(meta.Parameters).To(BeEmpty(), "BR-SECURITY-SI10: expected Parameters to be stripped (fail-closed) when schema Content fails to parse")
		// Non-parameter metadata (sourced from the catalog entry itself, not the
		// parsed schema body) must still be populated.
		Expect(meta.ExecutionEngine).To(Equal("tekton"), "expected ExecutionEngine to still be populated from the catalog entry")
	})
})
