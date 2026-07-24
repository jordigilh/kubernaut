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
	"errors"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/workflowcatalog"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	dsschema "github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	amtools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/alertmanager"
	promtools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/prometheus"
	infrastructure "github.com/jordigilh/kubernaut/test/infrastructure"
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
// buildWorkflowMeta / applyParsedSchemaMeta — pure-function unit tests.
//
// #1677 Phase 2g follow-up (DD-WORKFLOW-019): these previously drove
// dsCatalogFetcher.FetchValidator end-to-end via an httptest-mocked
// DataStorage server. Since FetchValidator now reads from KA's own
// workflowcatalog.Catalog (a cache-backed, in-process dependency, not an
// external one), the mapping logic is tested directly against
// models.RemediationWorkflow fixtures here; FetchValidator's own
// orchestration (Catalog.List call, fail-closed-on-empty, error
// propagation) is covered separately below against a real Catalog backed by
// a controller-runtime fake client (proving the actual wiring path).
//
// FedRAMP SI-10 (Information Input Validation): a workflow whose schema
// Content fails to parse must have its Parameters stripped (fail-closed),
// not silently accept unvalidated LLM-supplied parameters for that workflow.
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
// (source for DeclaredParameterNames). Note: a real CRD-sourced
// models.RemediationWorkflow.Content (crdWorkflowToModel, Issue #1677 Phase
// 2b) can never populate execution.resources -- the RemediationWorkflow CRD
// spec has no such field (Resources predates DD-WORKFLOW-018's etcd
// migration and was never ported to the CRD schema). This case is kept to
// lock in dsschema.Parser/applyParsedSchemaMeta's own contract regardless of
// caller, not to claim the CRD pipeline can reach it today.
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

// testCatalogWorkflow builds a minimal, valid models.RemediationWorkflow
// fixture -- the shape Catalog.List returns after crdWorkflowToModel
// conversion (Issue #1677 Phase 2b).
func testCatalogWorkflow(workflowID, content string) *models.RemediationWorkflow {
	bundleDigest := "sha256:aaa"
	sa := "workflow-runner"
	return &models.RemediationWorkflow{
		WorkflowID:            workflowID,
		WorkflowName:          "test-workflow",
		Name:                  "Test Workflow",
		ActionType:            "RestartPod",
		Version:               "1.0.0",
		SchemaVersion:         "1.0",
		Content:               content,
		ExecutionEngine:       models.ExecutionEngineTekton,
		ExecutionBundleDigest: &bundleDigest,
		ServiceAccountName:    &sa,
		Labels: models.MandatoryLabels{
			Severity:    []string{"critical"},
			Component:   []string{"v1/Pod"},
			Environment: []string{"production"},
			Priority:    "P0",
		},
		Status: models.WorkflowStatusActive,
	}
}

var _ = Describe("buildWorkflowMeta", func() {
	It("builds WorkflowMeta with catalog metadata from a valid workflow", func() {
		w := testCatalogWorkflow("550e8400-e29b-41d4-a716-446655440000", validWorkflowSchemaYAML)

		meta := buildWorkflowMeta(w, dsschema.NewParser(), logr.Discard())

		Expect(meta.ExecutionEngine).To(Equal("tekton"))
		Expect(meta.Version).To(Equal("1.0.0"))
		Expect(meta.ServiceAccountName).To(Equal("workflow-runner"))
		Expect(meta.ExecutionBundleDigest).To(Equal("sha256:aaa"))
		Expect(meta.Parameters).To(HaveLen(1))
		Expect(meta.Parameters[0].Name).To(Equal("TARGET_NAMESPACE"))
		Expect(meta.ActionType).To(Equal("RestartPod"))
		Expect(meta.WorkflowName).To(Equal("test-workflow"))
	})

	// UT-KA-337-001 (Issue #1661 Change 11a, DD-WORKFLOW-018): WorkflowMeta must
	// surface Dependencies/Resources/DeclaredParameterNames -- schema-derived
	// data buildWorkflowMeta's schemaParser already computes during parameter
	// validation but previously discarded after populating only Parameters.
	// AA's SelectedWorkflow needs these three fields to build the CRD-embedded
	// execution snapshot (Change 11b-11e) without a second, redundant DS fetch
	// from WorkflowExecution.
	It("UT-KA-337-001: WorkflowMeta surfaces Dependencies/Resources/DeclaredParameterNames from schema content", func() {
		w := testCatalogWorkflow("550e8400-e29b-41d4-a716-446655440010", workflowSchemaYAMLWithDepsAndResources)

		meta := buildWorkflowMeta(w, dsschema.NewParser(), logr.Discard())

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
		// required, non-optional fields on the catalog entry, so
		// buildWorkflowMeta must always copy them verbatim -- no
		// schema-parsing dependency, unlike Dependencies/Resources/
		// DeclaredParameterNames above.
		Expect(meta.ActionType).To(Equal("RestartPod"))
		Expect(meta.WorkflowName).To(Equal("test-workflow"))
	})

	It("strips parameters (fail-closed) when a workflow's schema content is malformed, but keeps catalog metadata", func() {
		// Content is not a valid RemediationWorkflow CRD envelope (missing
		// apiVersion/kind/metadata.name) — dsschema.Parser.Parse must fail.
		w := testCatalogWorkflow("550e8400-e29b-41d4-a716-446655440001", "not: a-valid-workflow-schema")

		meta := buildWorkflowMeta(w, dsschema.NewParser(), logr.Discard())

		Expect(meta.Parameters).To(BeEmpty(), "BR-SECURITY-SI10: expected Parameters to be stripped (fail-closed) when schema Content fails to parse")
		// Non-parameter metadata (sourced from the catalog entry itself, not the
		// parsed schema body) must still be populated.
		Expect(meta.ExecutionEngine).To(Equal("tekton"), "expected ExecutionEngine to still be populated from the catalog entry")
	})
})

// ============================================================================
// workflowCatalogFetcher.FetchValidator — wiring + orchestration tests.
//
// FedRAMP SI-10 (Information Input Validation): FetchValidator builds the
// allowlist used later to validate/strip LLM-proposed workflow selections
// (parser.Validator). An empty catalog must refuse to produce a validator
// (no workflow can be silently "allowed" when the catalog can't be
// confirmed empty-by-design).
//
// #1677 Phase 2g follow-up (DD-WORKFLOW-019): these exercise a real
// workflowcatalog.Catalog backed by a controller-runtime fake client
// (workflowcatalog.NewCacheFromReader) rather than a stub, proving the
// actual production dispatch path (bootstrap.go's
// newWorkflowCatalogFetcher(wfCatalog, ...)) end-to-end: CRD -> Cache.
// ListWorkflows -> crdWorkflowToModel -> buildWorkflowMeta -> Validator.
// ============================================================================

// newFakeWorkflowCatalog returns an immediately-Ready *workflowcatalog.
// LazyCatalog (#1677 hardening, DD-WORKFLOW-019: production always wires a
// LazyCatalog, never a bare *workflowcatalog.Catalog) backed by a
// controller-runtime fake client.
func newFakeWorkflowCatalog(t testing.TB, interceptorFuncs interceptor.Funcs, objs ...client.Object) *workflowcatalog.LazyCatalog {
	t.Helper()
	scheme, err := workflowcatalog.NewScheme()
	if err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}

	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}).
		WithInterceptorFuncs(interceptorFuncs)

	cache := workflowcatalog.NewCacheFromReader(builder.Build())
	return workflowcatalog.NewLazyCatalogReady(cache, logr.Discard())
}

// seedFakeWorkflow creates a RemediationWorkflow CRD via the fake client and
// stamps its status (workflowId/contentHash/catalogStatus) exactly as
// AuthWebhook would, reusing the same helper the KA integration test suite
// uses against envtest (test/infrastructure, Issue #1661 Phase 55).
func seedFakeWorkflow(t testing.TB, c client.Client, content string) string {
	t.Helper()
	wfID, err := infrastructure.SeedWorkflowContentViaDirectCRDCreation(context.Background(), c, "", content, GinkgoWriter)
	if err != nil {
		t.Fatalf("failed to seed workflow: %v", err)
	}
	return wfID
}

const fakeCRDWorkflowYAML = `
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: test-workflow
spec:
  version: "1.0.0"
  description:
    what: "test workflow"
    whenToUse: "testing"
  actionType: RestartPod
  labels:
    severity: ["critical"]
    environment: ["production"]
    component: ["v1/Pod"]
    priority: "P0"
  execution:
    engine: tekton
    serviceAccountName: workflow-runner
    bundleDigest: "sha256:aaa"
  parameters:
    - name: TARGET_NAMESPACE
      type: string
      required: true
      description: "target namespace"
`

var _ = Describe("workflowCatalogFetcher.FetchValidator", func() {
	It("BR-SECURITY-SI10: fails closed when the workflow catalog is empty", func() {
		catalog := newFakeWorkflowCatalog(GinkgoTB(), interceptor.Funcs{})
		f := newWorkflowCatalogFetcher(catalog, logr.Discard())

		validator, err := f.FetchValidator(context.Background())

		Expect(err).To(HaveOccurred(), "expected an error when the workflow catalog is empty (fail-closed, no implicit allow-all)")
		Expect(validator).To(BeNil())
	})

	It("returns a wrapped error when the catalog list fails", func() {
		catalog := newFakeWorkflowCatalog(GinkgoTB(), interceptor.Funcs{
			List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
				return errors.New("simulated cache list failure")
			},
		})
		f := newWorkflowCatalogFetcher(catalog, logr.Discard())

		validator, err := f.FetchValidator(context.Background())

		Expect(err).To(HaveOccurred(), "expected an error when the underlying cache list fails")
		Expect(validator).To(BeNil())
	})

	It("builds an allowlist end-to-end from a real CRD via the cache-backed Catalog", func() {
		scheme, err := workflowcatalog.NewScheme()
		Expect(err).NotTo(HaveOccurred())
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}).
			Build()

		wfID := seedFakeWorkflow(GinkgoTB(), c, fakeCRDWorkflowYAML)

		cache := workflowcatalog.NewCacheFromReader(c)
		catalog := workflowcatalog.NewLazyCatalogReady(cache, logr.Discard())
		f := newWorkflowCatalogFetcher(catalog, logr.Discard())

		validator, err := f.FetchValidator(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(validator).NotTo(BeNil())

		meta, ok := validator.GetWorkflowMeta(wfID)
		Expect(ok).To(BeTrue(), "BR-SECURITY-AC6: expected workflow %q to be part of the allowlist", wfID)
		Expect(meta.ExecutionEngine).To(Equal("tekton"))
		Expect(meta.Version).To(Equal("1.0.0"))
		Expect(meta.ServiceAccountName).To(Equal("workflow-runner"))
		Expect(meta.ExecutionBundleDigest).To(Equal("sha256:aaa"))
		Expect(meta.ActionType).To(Equal("RestartPod"))
		Expect(meta.WorkflowName).To(Equal("test-workflow"))
		Expect(meta.Parameters).To(HaveLen(1))
		Expect(meta.Parameters[0].Name).To(Equal("TARGET_NAMESPACE"))
	})
})
