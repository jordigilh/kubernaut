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

package aianalysis_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

type recordingWorkflowReader2575 struct {
	client.Reader
	listCalls int
}

func (r *recordingWorkflowReader2575) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	r.listCalls++
	return r.Reader.List(ctx, list, opts...)
}

type completedAgentSession2575 struct {
	session *agentsessionv1.AgentSession
}

func (c *completedAgentSession2575) GetOrCreate(context.Context, *aianalysisv1.AIAnalysis) (*agentsessionv1.AgentSession, error) {
	return c.session, nil
}

func (c *completedAgentSession2575) DeleteForRetry(context.Context, *agentsessionv1.AgentSession) error {
	return nil
}

var _ = Describe("AIAnalysis trusted workflow catalog wiring (#2575)", func() {
	It("UT-AA-2575-002: uses the trusted catalog reader before persisting a selected workflow", func() {
		catalogScheme := runtime.NewScheme()
		Expect(remediationworkflowv1.AddToScheme(catalogScheme)).To(Succeed())

		workflow := &remediationworkflowv1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: "trusted-workflow", Namespace: "default"},
			Spec: remediationworkflowv1.RemediationWorkflowSpec{
				Version:    "1.0.0",
				ActionType: "RestartPod",
				Parameters: []remediationworkflowv1.RemediationWorkflowParameter{
					{Name: "OPTIONAL", Type: "string", Required: false},
				},
			},
		}
		clean, err := contenthash.MarshalCleanCRDContent(workflow)
		Expect(err).NotTo(HaveOccurred())
		workflow.Status.ContentHash = contenthash.ComputeContentHash(string(clean))
		workflow.Status.WorkflowID = contenthash.DeterministicUUID(workflow.Status.ContentHash)

		reader := &recordingWorkflowReader2575{
			Reader: fake.NewClientBuilder().
				WithScheme(catalogScheme).
				WithObjects(workflow).
				Build(),
		}
		session := &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{Name: "completed-session", Namespace: "default"},
			Status: agentsessionv1.AgentSessionStatus{
				Phase: agentsessionv1.AgentSessionPhaseCompleted,
				Result: &agentsessionv1.AgentSessionResult{
					IncidentID: "incident-2575",
					Confidence: 0.95,
					SelectedWorkflow: &agentsessionv1.AgentSessionSelectedWorkflow{
						WorkflowID: workflow.Status.WorkflowID,
						Parameters: sharedtypes.TypedParameterMap{},
					},
				},
			},
		}
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "analysis-2575", Namespace: "default"},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "remediation-2575",
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{SignalName: "TestSignal"},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{Phase: "Investigating"},
		}

		handler := handlers.NewInvestigatingHandler(
			&completedAgentSession2575{session: session},
			logr.Discard(),
			metrics.NewMetrics(),
			&noopAuditClient{},
			handlers.WithWorkflowCatalog(reader, "default"),
		)

		_, err = handler.Handle(context.Background(), analysis)
		Expect(err).NotTo(HaveOccurred())
		Expect(reader.listCalls).To(BeNumerically(">", 0), "trusted catalog reader must be consulted")
		selected := analysis.Status.GetRCAResult().SelectedWorkflow
		Expect(selected).NotTo(BeNil())
		Expect(selected.WorkflowID).To(Equal(workflow.Status.WorkflowID))
	})
})
