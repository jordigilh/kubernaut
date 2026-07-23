/*
Copyright 2025 Jordi Gil.

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
)

// ========================================
// RESPONSE PROCESSOR SA MAPPING TESTS (#481 / #650 / #1661)
// ========================================
// Authority: DD-WE-005 v2.0 (Per-Workflow ServiceAccount Reference)
//
// History: Issue #650 deliberately stopped extracting service_account_name
// into SelectedWorkflow, because at the time the WFE controller resolved
// ServiceAccountName itself via a direct DataStorage catalog round-trip at
// reconcile time (resolveWorkflowCatalog's pre-#1661 behavior) -- AIAnalysis's
// snapshot was redundant.
//
// #1661 Change 11e (DD-WORKFLOW-018) removed that WFE-side DS round-trip
// entirely: WorkflowRef is now RO's already-validated, CRD-embedded snapshot
// copied verbatim from AIAnalysis.Status.SelectedWorkflow, with "no DS entry
// left to consult" (internal/controller/workflowexecution/workflowexecution_catalog.go).
// That made AIAnalysis.Status.SelectedWorkflow.ServiceAccountName the *sole*
// remaining path for the SA to reach the Job pod spec -- but
// storeSelectedWorkflow (response_processor.go) was never updated to start
// populating it, silently downgrading every job-engine execution to the
// namespace's "default" ServiceAccount (no cross-namespace RBAC), which
// fails fast with Job BackoffLimitExceeded once a Pod actually tries to
// kubectl get/patch its target resource. Root-caused via E2E (fleet)
// E2E-FLEET-014/015 real-cluster failures.
// ========================================

var _ = Describe("Response Processor SA Mapping [DD-WE-005] (#481/#650/#1661)", func() {

	Context("GetStringFromMap utility", func() {

		It("UT-AA-481-001: GetStringFromMap extracts string values from map", func() {
			swMap := map[string]interface{}{
				"workflow_id":          "wf-uuid-123",
				"execution_bundle":     "quay.io/test:v1@sha256:abc",
				"confidence":           0.95,
				"service_account_name": "my-workflow-sa",
			}
			result := handlers.GetStringFromMap(swMap, "service_account_name")
			Expect(result).To(Equal("my-workflow-sa"))
		})

		It("UT-AA-481-002: GetStringFromMap returns empty for absent keys", func() {
			swMap := map[string]interface{}{
				"workflow_id":      "wf-uuid-456",
				"execution_bundle": "quay.io/test:v1@sha256:def",
				"confidence":       0.85,
			}
			result := handlers.GetStringFromMap(swMap, "service_account_name")
			Expect(result).To(BeEmpty())
		})
	})

	Context("storeSelectedWorkflow / ProcessIncidentResponse (production wiring)", func() {
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
					Name:      "test-sa-mapping",
					Namespace: "default",
					UID:       types.UID("test-uid-sa-001"),
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: "test-rr-sa-001",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: aianalysis.PhaseInvestigating,
				},
			}
		})

		It("UT-AA-1661-650-001: propagates service_account_name from the KA response into SelectedWorkflow (DD-WE-005 v2.0, regression guard)", func() {
			kaResp := &agentclient.IncidentResponse{
				IncidentID:       "test-sa-001",
				Analysis:         "Root cause: OOM",
				NeedsHumanReview: agentclient.NewOptBool(false),
				Confidence:       0.93,
				Timestamp:        "2026-07-21T12:00:00Z",
				SelectedWorkflow: agentclient.OptNilIncidentResponseSelectedWorkflow{
					Value: agentclient.IncidentResponseSelectedWorkflow{
						"workflow_id":          jx.Raw(`"oomkill-increase-memory-v1"`),
						"execution_bundle":     jx.Raw(`"quay.io/kubernaut-cicd/test-workflows/oomkill-increase-memory-job:v1.0.0-exec"`),
						"execution_engine":     jx.Raw(`"job"`),
						"confidence":           jx.Raw(`0.93`),
						"service_account_name": jx.Raw(`"workflow-job-executor"`),
					},
					Set: true,
				},
			}

			_, err := processor.ProcessIncidentResponse(ctx, analysis, kaResp)
			Expect(err).ToNot(HaveOccurred())

			sw := analysis.Status.SelectedWorkflow
			Expect(sw).ToNot(BeNil(), "SelectedWorkflow must be populated")
			Expect(sw.ServiceAccountName).To(Equal("workflow-job-executor"),
				"ServiceAccountName must reach AIAnalysis.Status.SelectedWorkflow -- WFE (#1661 Change 11e) "+
					"no longer resolves it from DataStorage itself and copies this snapshot verbatim onto the "+
					"Job pod spec; an empty value here silently downgrades the Job to the namespace's "+
					"'default' SA and fails fast with no cross-namespace RBAC")
		})

		It("UT-AA-1661-650-002: leaves ServiceAccountName empty (not panicking) when absent from the KA response", func() {
			kaResp := &agentclient.IncidentResponse{
				IncidentID:       "test-sa-002",
				Analysis:         "Root cause: crash loop",
				NeedsHumanReview: agentclient.NewOptBool(false),
				Confidence:       0.85,
				Timestamp:        "2026-07-21T12:05:00Z",
				SelectedWorkflow: agentclient.OptNilIncidentResponseSelectedWorkflow{
					Value: agentclient.IncidentResponseSelectedWorkflow{
						"workflow_id":      jx.Raw(`"restart-pod-v1"`),
						"execution_bundle": jx.Raw(`"ghcr.io/kubernaut/restart-pod:v1.0"`),
						"execution_engine": jx.Raw(`"job"`),
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
			Expect(sw.ServiceAccountName).To(BeEmpty())
		})
	})
})
