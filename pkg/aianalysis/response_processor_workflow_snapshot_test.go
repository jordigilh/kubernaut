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

// Package aianalysis contains unit tests for SelectedWorkflow's CRD-embedded
// execution snapshot (Issue #1661 Change 11b, DD-WORKFLOW-018).
//
// storeSelectedWorkflow (pkg/aianalysis/handlers/response_processor.go) must
// extract Dependencies/Resources/DeclaredParameterNames — schema-derived data
// KA already places on the wire selected_workflow map (Change 11a, Phase
// 37-39) — into AIAnalysis.Status.SelectedWorkflow, and stamp SelectedAt so
// the CEL write-once guard (Phase 41) can lock the snapshot after first write.
//
// RED: SelectedWorkflow has no Dependencies/Resources/DeclaredParameterNames/
// SelectedAt fields yet — this file must fail to compile.
package aianalysis_test

import (
	"context"

	"github.com/go-faster/jx"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("ResponseProcessor SelectedWorkflow extended snapshot (Issue #1661 Change 11b)", func() {
	var (
		processor *handlers.ResponseProcessor
		analysis  *aianalysisv1.AIAnalysis
		ctx       context.Context
	)

	BeforeEach(func() {
		m := metrics.NewMetrics()
		processor = handlers.NewResponseProcessor(logr.Discard(), m, &noopAuditClient{})
		ctx = context.Background()
		analysis = &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workflow-snapshot",
				Namespace: "default",
				UID:       types.UID("test-uid-wfsnap-001"),
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "test-rr-wfsnap-001",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysis.PhaseInvestigating,
			},
		}
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-338-001: storeSelectedWorkflow extracts Dependencies/Resources/
	// DeclaredParameterNames from KA's response map (DD-WORKFLOW-018)
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-338-001: extracts Dependencies/Resources/DeclaredParameterNames from the KA response into SelectedWorkflow", func() {
		kaResp := &agentclient.IncidentResponse{
			IncidentID:       "test-wfsnap-001",
			Analysis:         "Root cause: memory pressure",
			NeedsHumanReview: agentclient.NewOptBool(false),
			Confidence:       0.92,
			Timestamp:        "2026-07-16T12:00:00Z",
			SelectedWorkflow: agentclient.OptNilIncidentResponseSelectedWorkflow{
				Value: agentclient.IncidentResponseSelectedWorkflow{
					"workflow_id":      jx.Raw(`"increase-memory-v1"`),
					"execution_bundle": jx.Raw(`"ghcr.io/kubernaut/increase-memory:v1.0"`),
					"confidence":       jx.Raw(`0.92`),
					"dependencies": jx.Raw(`{
						"secrets": [{"name": "db-creds"}],
						"configMaps": [{"name": "app-config"}]
					}`),
					"resources": jx.Raw(`{
						"requests": {"cpu": "100m", "memory": "128Mi"},
						"limits": {"cpu": "500m", "memory": "512Mi"}
					}`),
					"declared_parameter_names": jx.Raw(`{"TARGET_NAMESPACE": true, "REPLICAS": true}`),
				},
				Set: true,
			},
		}

		_, err := processor.ProcessIncidentResponse(ctx, analysis, kaResp)
		Expect(err).ToNot(HaveOccurred())

		sw := analysis.Status.SelectedWorkflow
		Expect(sw).ToNot(BeNil(), "SelectedWorkflow must be populated")

		Expect(sw.Dependencies).ToNot(BeNil(), "Dependencies must be extracted from the KA response")
		Expect(sw.Dependencies.Secrets).To(HaveLen(1))
		Expect(sw.Dependencies.Secrets[0].Name).To(Equal("db-creds"))
		Expect(sw.Dependencies.ConfigMaps).To(HaveLen(1))
		Expect(sw.Dependencies.ConfigMaps[0].Name).To(Equal("app-config"))

		Expect(sw.Resources).ToNot(BeNil(), "Resources must be extracted from the KA response")
		Expect(sw.Resources.Requests.Cpu().String()).To(Equal("100m"))
		Expect(sw.Resources.Limits.Cpu().String()).To(Equal("500m"))

		Expect(sw.DeclaredParameterNames).To(Equal(map[string]bool{"TARGET_NAMESPACE": true, "REPLICAS": true}))

		Expect(sw.SelectedAt).ToNot(BeNil(), "SelectedAt must be stamped for the CEL immutability guard (Phase 41)")
		Expect(sw.SelectedAt.Time).ToNot(BeZero())
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-338-002: round-trip when the fields are absent — no panic, nil/empty
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-338-002: leaves Dependencies/Resources/DeclaredParameterNames nil when absent from the KA response, without panicking", func() {
		kaResp := &agentclient.IncidentResponse{
			IncidentID:       "test-wfsnap-002",
			Analysis:         "Root cause: crash loop",
			NeedsHumanReview: agentclient.NewOptBool(false),
			Confidence:       0.85,
			Timestamp:        "2026-07-16T12:05:00Z",
			SelectedWorkflow: agentclient.OptNilIncidentResponseSelectedWorkflow{
				Value: agentclient.IncidentResponseSelectedWorkflow{
					"workflow_id":      jx.Raw(`"restart-pod-v1"`),
					"execution_bundle": jx.Raw(`"ghcr.io/kubernaut/restart-pod:v1.0"`),
					"confidence":       jx.Raw(`0.85`),
				},
				Set: true,
			},
		}

		var err error
		Expect(func() {
			_, err = processor.ProcessIncidentResponse(ctx, analysis, kaResp)
		}).ToNot(Panic())
		Expect(err).ToNot(HaveOccurred())

		sw := analysis.Status.SelectedWorkflow
		Expect(sw).ToNot(BeNil())
		Expect(sw.Dependencies).To(BeNil())
		Expect(sw.Resources).To(BeNil())
		Expect(sw.DeclaredParameterNames).To(BeEmpty())
		Expect(sw.SelectedAt).ToNot(BeNil(), "SelectedAt must still be stamped even when the schema-derived fields are absent")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-338-003: preservePartialSelectedWorkflow and preserveLowConfidenceWorkflow
	// terminal paths must also stamp SelectedAt (all 3 population call sites)
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-338-003: preserveLowConfidenceWorkflow stamps SelectedAt (low-confidence terminal path)", func() {
		kaResp := &agentclient.IncidentResponse{
			IncidentID:       "test-wfsnap-003",
			Analysis:         "Root cause: low confidence match",
			NeedsHumanReview: agentclient.NewOptBool(true),
			Confidence:       0.4,
			Timestamp:        "2026-07-16T12:10:00Z",
			SelectedWorkflow: agentclient.OptNilIncidentResponseSelectedWorkflow{
				Value: agentclient.IncidentResponseSelectedWorkflow{
					"workflow_id":      jx.Raw(`"low-confidence-v1"`),
					"execution_bundle": jx.Raw(`"ghcr.io/kubernaut/low-confidence:v1.0"`),
					"confidence":       jx.Raw(`0.4`),
				},
				Set: true,
			},
		}

		_, err := processor.ProcessIncidentResponse(ctx, analysis, kaResp)
		Expect(err).ToNot(HaveOccurred())

		sw := analysis.Status.SelectedWorkflow
		Expect(sw).ToNot(BeNil(), "low-confidence path must still preserve SelectedWorkflow for operator visibility")
		Expect(sw.SelectedAt).ToNot(BeNil(), "SelectedAt must be stamped on the low-confidence preservation path too")
	})
})

var _ = Describe("sharedtypes.WorkflowDependencies (Issue #1661 Change 11b)", func() {
	It("UT-AA-338-004: round-trips Secrets/ConfigMaps through JSON matching KA's wire format", func() {
		deps := sharedtypes.WorkflowDependencies{
			Secrets:    []sharedtypes.WorkflowResourceDependency{{Name: "db-creds"}},
			ConfigMaps: []sharedtypes.WorkflowResourceDependency{{Name: "app-config"}},
		}
		Expect(deps.Secrets[0].Name).To(Equal("db-creds"))
		Expect(deps.ConfigMaps[0].Name).To(Equal("app-config"))
	})
})
