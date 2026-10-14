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

package apifrontend_test

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	kasession "github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// BR-KA-016 / AU-3 / SI-10: fixture result -> real KA completion publisher ->
// actual AF complete-event consumer -> schema/gob-safe A2A artifact queue.
// No DS client is constructed. Provider/catalog acceptance and full network
// journeys have separate tests; this spec proves this production delivery seam.
const afITRejected2496 = "rejected"

var _ = Describe("#2496 live completion projection", func() {
	DescribeTable("IT-AF-2496-001 / IT-AF-2496-003: KA completion reaches the live investigation_summary without a history read",
		func(outcome string) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			DeferCleanup(cancel)
			rcaConfidence, confidence, required, matched := .93, 0., true, true
			result := &katypes.InvestigationResult{
				Severity: "critical", RCASummary: "Current memory pressure", CausalChain: []string{"Current limit exceeded"}, RCAConfidence: &rcaConfidence,
				WorkflowID: "catalog-id-2496", WorkflowName: "increase-memory", Confidence: confidence, WorkflowSelectionConfidence: &confidence,
				RepeatJustification:         &sharedtypes.RepeatJustification{EvidenceBasis: "Current limit observed", PriorOutcomeRelevance: "Prior Full zero", ExpectedMechanism: "Increase limit"},
				RepeatJustificationRequired: &required, PriorWorkflowMatch: &matched, RepeatValidationOutcome: "accepted",
				HandoffDigest: "sha256:" + strings.Repeat("b", 64), PriorRemediationUIDs: []string{"private-prior-2496"},
				Parameters: map[string]interface{}{"PASSWORD": "private-parameter-2496"}, ExecutionBundle: "private-bundle-2496",
			}
			switch outcome {
			case afITRejected2496:
				result.WorkflowID, result.ProposedWorkflowID = "", result.WorkflowID
				result.RepeatValidationOutcome, result.RepeatJustification.ExpectedMechanism = afITRejected2496, ""
				result.HumanReviewNeeded = true
			case "no-workflow":
				result = &katypes.InvestigationResult{Severity: "warning", RCASummary: "No suitable workflow", CausalChain: []string{"No suitable strategy"}, RCAConfidence: &rcaConfidence, HumanReviewNeeded: true}
			}
			manager := kasession.NewManager(kasession.NewStore(time.Minute, kasession.WithLogger(logr.Discard())), logr.Discard(), nil, nil)
			ready, proceed := make(chan struct{}), make(chan struct{})
			id, err := manager.StartInteractiveSession(ctx, func(ctx context.Context) (*katypes.InvestigationResult, error) {
				close(ready)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-proceed:
					return result, nil
				}
			}, map[string]string{"remediation_id": "rr-live-2496"})
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.LaunchDeferredInvestigation(id)).To(Succeed())
			Eventually(ready).Should(BeClosed())
			published, err := manager.Subscribe(ctx, id)
			Expect(err).NotTo(HaveOccurred())
			close(proceed)
			var complete kasession.InvestigationEvent
			Eventually(func() bool {
				select {
				case event, ok := <-published:
					if ok && event.Type == kasession.EventTypeComplete {
						complete = event
						return true
					}
				default:
				}
				return false
			}).Should(BeTrue())

			queues := eventqueue.NewInMemoryManager()
			queue, err := queues.GetOrCreate(ctx, "task-2496")
			Expect(err).NotTo(HaveOccurred())
			// A2A broadcasts to other connections, never back to its writer.
			reader, err := queues.GetOrCreate(ctx, "task-2496")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(queues.Destroy(context.Background(), "task-2496")).To(Succeed()) })
			bridgeCtx := launcher.WithEventBridge(ctx, queue, "task-2496", "context-2496", nil)
			events := make(chan ka.InvestigationEvent, 1)
			events <- ka.InvestigationEvent{Type: ka.EventTypeComplete, Data: complete.Data}
			close(events)
			// Exercise the production blocking handler, not just its event
			// collector: the handler owns final investigation_summary emission.
			cfg := &tools.InvestigateConfig{MCPClient: &ka.MockMCPClient{
				StartInvestigationFn: func(context.Context, ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{SessionID: id, Status: "completed", Events: events}, nil
				},
			}}
			parsed, err := tools.HandleInvestigationMCPWithRegistry(bridgeCtx, cfg, tools.InvestigateMCPArgs{RRID: "rr-live-2496"}, true, "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(parsed.RCA).NotTo(BeNil())
			var artifact *a2a.TaskArtifactUpdateEvent
			for artifact == nil {
				event, _, readErr := reader.Read(ctx)
				Expect(readErr).NotTo(HaveOccurred())
				artifact, _ = event.(*a2a.TaskArtifactUpdateEvent)
			}
			Expect(artifact.Artifact.Metadata).To(HaveKeyWithValue("schema", "investigation_summary"))
			Expect(artifact.Artifact.Metadata).To(HaveKeyWithValue("schema_version", "1.0"))
			data, ok := artifact.Artifact.Parts[0].(a2a.DataPart)
			Expect(ok).To(BeTrue())
			Expect(data.Data["rca"]).To(HaveKeyWithValue("confidence", rcaConfidence))
			switch outcome {
			case "selected":
				Expect(data.Data).To(HaveKey("selected_workflow"))
				Expect(data.Data["selected_workflow"]).To(HaveKeyWithValue("workflow_id", "catalog-id-2496"))
				Expect(data.Data["selected_workflow"]).To(HaveKeyWithValue("workflow_selection_confidence", 0.))
				Expect(data.Data).NotTo(HaveKey("selection_assessment"))
			case afITRejected2496:
				Expect(data.Data).NotTo(HaveKey("selected_workflow"))
				Expect(data.Data).To(HaveKey("selection_assessment"))
				Expect(data.Data["selection_assessment"]).To(HaveKeyWithValue("proposed_workflow_id", "catalog-id-2496"))
				Expect(data.Data["selection_assessment"]).NotTo(HaveKey("workflow_id"))
			default:
				Expect(data.Data).NotTo(HaveKey("selected_workflow"))
				Expect(data.Data).NotTo(HaveKey("selection_assessment"))
			}
			wire, err := json.Marshal(data.Data)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(wire)).NotTo(ContainSubstring("private-"))
			Expect(string(wire)).NotTo(ContainSubstring("prior_remediation_uids"))
			var encoded bytes.Buffer
			Expect(gob.NewEncoder(&encoded).Encode(data.Data)).To(Succeed())
			Expect(artifact.Artifact.Parts[1]).To(BeAssignableToTypeOf(a2a.TextPart{}))
		},
		Entry("selected", "selected"), Entry(afITRejected2496, afITRejected2496), Entry("no-workflow", "no-workflow"),
	)

	DescribeTable("IT-AF-2496-003: invalid live selection context uses the observable text fallback (BR-KA-016, SI-10)",
		func(selection any) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			DeferCleanup(cancel)
			queues := eventqueue.NewInMemoryManager()
			writer, err := queues.GetOrCreate(ctx, "task-invalid-2496")
			Expect(err).NotTo(HaveOccurred())
			reader, err := queues.GetOrCreate(ctx, "task-invalid-2496")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(queues.Destroy(context.Background(), "task-invalid-2496")).To(Succeed()) })
			bridgeCtx := launcher.WithEventBridge(ctx, writer, "task-invalid-2496", "context-2496", nil)
			data := map[string]any{"session_id": "rr-2496", "summary": "Current pressure", "rca": map[string]any{"explanation": "Current pressure"}, "selected_workflow": selection}
			Expect(launcher.EmitArtifactSafe(bridgeCtx, data, "Current pressure", map[string]any{"schema": "investigation_summary", "schema_version": "1.0"})).To(Succeed())
			event, _, err := reader.Read(ctx)
			Expect(err).NotTo(HaveOccurred())
			artifact, ok := event.(*a2a.TaskArtifactUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(artifact.Artifact.Parts).To(HaveLen(1), "unsafe structured data must not reach the subscriber")
			Expect(artifact.Artifact.Parts[0]).To(Equal(a2a.TextPart{Text: "Current pressure"}))
		},
		Entry("malformed", "not-an-object"),
		Entry("oversized reasoning", map[string]any{"workflow_id": "wf-2496", "workflow_name": "Increase memory", "repeat_justification": map[string]any{"evidence_basis": strings.Repeat("a", 2001), "prior_outcome_relevance": "Prior Full zero", "expected_mechanism": "Increase limit"}}),
		Entry("private execution fields", map[string]any{"workflow_id": "wf-2496", "workflow_name": "Increase memory", "parameters": map[string]any{"PASSWORD": "private-private"}}),
	)
})

// BR-KA-016 / DD-AA-KA-001 / FedRAMP AC-6: shared-envtest must exercise the
// same authenticated dispatch/catalog identity as the production KA service.
// Serial specs run on process 1, which owns this shared API server and token.
var _ = Describe("IT-AF-2496-004: authenticated KA fixture permissions [BR-KA-016]", Serial, func() {
	DescribeTable("allows required reads and dispatch writes, but not unrelated writes or cross-namespace Leases",
		func(group, resource, subresource, targetNamespace, verb string, allowed bool) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			DeferCleanup(cancel)
			Expect(kaFixtureClient).NotTo(BeNil())
			Eventually(func(g Gomega) {
				review, err := kaFixtureClient.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx,
					&authorizationv1.SelfSubjectAccessReview{Spec: authorizationv1.SelfSubjectAccessReviewSpec{
						ResourceAttributes: &authorizationv1.ResourceAttributes{
							Group: group, Resource: resource, Subresource: subresource, Namespace: targetNamespace, Verb: verb,
						},
					}}, metav1.CreateOptions{})
				g.Expect(err).NotTo(HaveOccurred(), "authorization must use the KA bearer identity")
				g.Expect(review.Status.EvaluationError).To(BeEmpty())
				g.Expect(review.Status.Allowed).To(Equal(allowed))
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
		},
		Entry("AgentSession list", "kubernaut.ai", "agentsessions", "", defaultFixture, "list", true),
		Entry("AgentSession watch", "kubernaut.ai", "agentsessions", "", defaultFixture, "watch", true),
		Entry("AgentSession finalizer patch", "kubernaut.ai", "agentsessions", "", defaultFixture, "patch", true),
		Entry("AgentSession status update", "kubernaut.ai", "agentsessions", "status", defaultFixture, "update", true),
		Entry("InvestigationSession read", "kubernaut.ai", "investigationsessions", "", defaultFixture, "get", true),
		Entry("RemediationWorkflow watch", "kubernaut.ai", "remediationworkflows", "", "", "watch", true),
		Entry("ActionType list", "kubernaut.ai", "actiontypes", "", "", "list", true),
		Entry("RemediationRequest read", "kubernaut.ai", "remediationrequests", "", defaultFixture, "get", true),
		Entry("target Deployment read", "apps", "deployments", "", defaultFixture, "get", true),
		Entry("dispatch Lease create", "coordination.k8s.io", "leases", "", defaultFixture, "create", true),
		Entry("dispatch Lease list", "coordination.k8s.io", "leases", "", defaultFixture, "list", true),
		Entry("cross-namespace Lease denied", "coordination.k8s.io", "leases", "", "outside-ka-fixture", "create", false),
		Entry("cluster-wide Lease list denied", "coordination.k8s.io", "leases", "", "", "list", false),
		Entry("AgentSession create denied", "kubernaut.ai", "agentsessions", "", defaultFixture, "create", false),
		Entry("InvestigationSession update denied", "kubernaut.ai", "investigationsessions", "", defaultFixture, "update", false),
		Entry("RemediationWorkflow update denied", "kubernaut.ai", "remediationworkflows", "", "", "update", false),
		Entry("RemediationRequest create denied", "kubernaut.ai", "remediationrequests", "", defaultFixture, "create", false),
		Entry("target Deployment patch denied", "apps", "deployments", "", defaultFixture, "patch", false),
	)

	DescribeTable("lists actual dispatch and catalog resources using the KA bearer identity",
		func(resource, targetNamespace string) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			DeferCleanup(cancel)
			Expect(kaFixtureDynamic).NotTo(BeNil())
			_, err := kaFixtureDynamic.Resource(schema.GroupVersionResource{
				Group: "kubernaut.ai", Version: "v1alpha1", Resource: resource,
			}).Namespace(targetNamespace).List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred(), "real informer LIST permission must work")
		},
		Entry("AgentSession", "agentsessions", defaultFixture),
		Entry("InvestigationSession", "investigationsessions", defaultFixture),
		Entry("RemediationWorkflow", "remediationworkflows", ""),
		Entry("ActionType", "actiontypes", ""),
	)
})
