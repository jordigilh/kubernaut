package e2e_test

import (
	"context"
	"io"
	"strings"
	"time"

	investigationsessionv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Fleet current invocation barrier [BR-INTERACTIVE-004, BR-INTEGRATION-065]", func() {
	DescribeTable("UT-AF-FLEET-2593-004: retained sessions cannot establish this caller's action=start", func(rr, ns, correlation string, bound bool, want bool) {
		binding := fleetAFInvocation{ContextID: "ctx-current"}
		if bound {
			binding.TaskID, binding.RRID = "task-emitted", "rr-current"
		}
		session := investigationsessionv1alpha1.InvestigationSession{
			Spec: investigationsessionv1alpha1.InvestigationSessionSpec{
				RemediationRequestRef: investigationsessionv1alpha1.ObjectRef{Namespace: ns, Name: rr},
				A2ATaskID:             "a2a-" + rr,
			},
			Status: investigationsessionv1alpha1.InvestigationSessionStatus{KACorrelationID: correlation},
		}
		Expect(fleetAFStartedSession(binding, "kubernaut-system", []investigationsessionv1alpha1.InvestigationSession{session})).To(Equal(want))
	},
		Entry("no live binding", "rr-old", "kubernaut-system", "ka-old", false, false),
		Entry("retained target RR", "rr-old", "kubernaut-system", "ka-old", true, false),
		Entry("wrong RR namespace", "rr-current", "other", "ka-old", true, false),
		Entry("current action has not started", "rr-current", "kubernaut-system", "", true, false),
		Entry("exact RR and actual KA correlation; synthetic CRD task differs", "rr-current", "kubernaut-system", "ka-current", true, true),
	)

	DescribeTable("UT-AF-FLEET-2593-005: terminal/refused first stream fails before polling the API", func(data string) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		updates, done := readFleetAFLive(ctx, strings.NewReader(data))
		_, err := waitFleetAFCurrentStart(ctx, fleetAFInvocation{ContextID: "ctx-current"}, updates, nil, "kubernaut-system")
		Expect(err).To(HaveOccurred())
		Expect(ctx.Err()).NotTo(HaveOccurred(), "completed refusal must not wait for the 90-second E2E timeout")
		Eventually(done, time.Second).Should(BeClosed())
	},
		Entry("completed refusal", `data: {"result":{"kind":"status-update","contextId":"ctx-current","taskId":"task-emitted","status":{"state":"completed"}}}`+"\n"),
		Entry("JSON-RPC error", `data: {"error":{"code":-32603,"message":"scope unavailable"}}`+"\n"),
		Entry("invalid JSON", "data: not-json\n"),
		Entry("EOF", ""),
	)

	It("UT-AF-FLEET-2593-006: uses emitted context/task/RR IDs, never the JSON-RPC envelope ID", func() {
		binding := fleetAFInvocation{ContextID: "ctx-current"}
		err := scanFleetAFEvents(strings.NewReader(`data: {"id":"request-not-task","result":{"kind":"status-update","contextId":"ctx-current","taskId":"actual-task","metadata":{"rr_id":"rr-current"}}}`+"\n"), binding.observe)
		Expect(err).NotTo(HaveOccurred())
		Expect(binding.TaskID).To(Equal("actual-task"))
		Expect(binding.RRID).To(Equal("rr-current"))
		Expect(binding.observe(a2aSSEEvent{Kind: "status-update", ContextID: "ctx-retained", TaskID: "actual-task"})).To(HaveOccurred())
		Expect(binding.observe(a2aSSEEvent{Kind: "status-update", ContextID: "ctx-current", TaskID: "other-task"})).To(HaveOccurred())
	})

	It("UT-AF-FLEET-2593-009: even an exact RR's pre-invocation correlation is not new action=start proof", func() {
		binding := fleetAFInvocation{ContextID: "ctx-current", TaskID: "task-current", RRID: "rr-current",
			PreviouslyStarted: map[string]bool{"kubernaut-system/rr-current": true}}
		updates := make(chan fleetAFStreamUpdate, 1)
		updates <- fleetAFStreamUpdate{Event: a2aSSEEvent{Kind: "status-update", ContextID: binding.ContextID, TaskID: binding.TaskID}}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err := waitFleetAFCurrentStart(ctx, binding, updates, nil, "kubernaut-system")
		Expect(err).To(MatchError(ContainSubstring("previously correlated")))
		Expect(ctx.Err()).NotTo(HaveOccurred())
	})

	DescribeTable("UT-AF-FLEET-2593-010: the live parser and persisted exact-RR session together release the start barrier", func(namespace string) {
		scheme := runtime.NewScheme()
		Expect(investigationsessionv1alpha1.AddToScheme(scheme)).To(Succeed())
		session := &investigationsessionv1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "is-current"},
			Spec: investigationsessionv1alpha1.InvestigationSessionSpec{
				RemediationRequestRef: investigationsessionv1alpha1.ObjectRef{Namespace: namespace, Name: "rr-current"},
				A2ATaskID:             "a2a-rr-current",
			},
			Status: investigationsessionv1alpha1.InvestigationSessionStatus{KACorrelationID: "ka-real-correlation"},
		}
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(session).Build()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		updates, done := readFleetAFLive(ctx, strings.NewReader(`data: {"id":"not-a-task","result":{"kind":"status-update","contextId":"ctx-current","taskId":"emitted-task","metadata":{"rr_id":"rr-current"}}}`+"\n"))
		binding, err := waitFleetAFCurrentStart(ctx, fleetAFInvocation{ContextID: "ctx-current"}, updates, reader, namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(binding.RRID).To(Equal("rr-current"))
		Expect(binding.TaskID).To(Equal("emitted-task"))
		Eventually(done, time.Second).Should(BeClosed())
	}, Entry("controller namespace", "kubernaut-system"), Entry("different namespace stays exact", "isolated"))

	It("UT-AF-FLEET-2593-007: cancellation and body close join a blocked live reader", func() {
		ctx, cancel := context.WithCancel(context.Background())
		body, writer := io.Pipe()
		defer func() { Expect(writer.Close()).To(Succeed()) }()
		_, done := readFleetAFLive(ctx, body)
		cancel()
		Expect(body.Close()).To(Succeed())
		Eventually(done, time.Second).Should(BeClosed())
	})

	It("UT-AF-FLEET-2593-008: accepts only bound session_active guidance and no early RCA or summary", func() {
		binding := fleetAFInvocation{ContextID: "ctx-second", TaskID: "first-task", RRID: "rr-current"}
		event := a2aSSEEvent{Kind: "status-update", ContextID: "ctx-second", TaskID: "second-task", Metadata: map[string]any{"type": "status", "rr_id": "rr-current"},
			Status: &a2aSSEStatus{Message: &a2aSSEMessage{Parts: []a2aSSEPart{{Text: "An investigation is already in progress. Do not retry kubernaut_investigate. Use kubernaut_get_remediation with rr_id rr-current to check its status."}}}}}
		Expect(validateFleetAFLeaseGuidance([]a2aSSEEvent{event}, binding)).To(Succeed())
		event.Status.State = failed
		Expect(validateFleetAFLeaseGuidance([]a2aSSEEvent{event}, binding)).To(HaveOccurred())
		event.Status.State = ""
		delete(event.Metadata, "rr_id")
		Expect(validateFleetAFLeaseGuidance([]a2aSSEEvent{event}, binding)).To(HaveOccurred())
		event.Metadata["rr_id"] = "rr-current"
		for _, schema := range []string{fleetAFEarlyRCASchema, investigationSummarySchema} {
			finding := a2aSSEEvent{Kind: "artifact-update", Artifact: &a2aSSEArtifact{Metadata: map[string]any{"schema": schema}}}
			Expect(validateFleetAFLeaseGuidance([]a2aSSEEvent{event, finding}, binding)).To(HaveOccurred())
		}
		for _, field := range []string{"rca", "summary", "confidence", "causal_chain"} {
			finding := a2aSSEEvent{Kind: "artifact-update", Artifact: &a2aSSEArtifact{Parts: []a2aSSEPart{{Data: []byte(`{"` + field + `":null}`)}}}}
			Expect(validateFleetAFLeaseGuidance([]a2aSSEEvent{event, finding}, binding)).To(HaveOccurred())
		}
		event.ContextID = "ctx-old"
		Expect(validateFleetAFLeaseGuidance([]a2aSSEEvent{event}, binding)).To(HaveOccurred())
		event.ContextID = "ctx-second"
		for _, text := range []string{"scope unavailable", "conflict", "already in progress for rr-other"} {
			event.Status.Message.Parts[0].Text = text
			Expect(validateFleetAFLeaseGuidance([]a2aSSEEvent{event}, binding)).To(HaveOccurred())
		}
	})
})
