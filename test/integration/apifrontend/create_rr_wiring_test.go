package apifrontend_test

import (
	"context"
	"errors"
	"os"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

type noopPromClientIT struct{}

func (n *noopPromClientIT) GetAlerts(_ context.Context) ([]prom.Alert, error) {
	return nil, nil
}
func (n *noopPromClientIT) GetRules(_ context.Context) ([]prom.RuleGroup, error) {
	return nil, nil
}
func (n *noopPromClientIT) InstantQuery(_ context.Context, _ string) (*prom.QueryResult, error) {
	return &prom.QueryResult{}, nil
}

// alwaysFiringPromClientIT returns a single cluster-scoped firing alert (no
// namespace/kind/name labels), so it label-matches any target resource. See
// the tools_test package's alwaysFiringPromClient for the full rationale.
//
// #1839/DD-AF-010: a nil Triager now fails closed. IT tests that don't care
// about the specific severity value but need HandleCreateRR to succeed use
// this fixture via defaultTestTriagerIT().
type alwaysFiringPromClientIT struct{}

func (a *alwaysFiringPromClientIT) GetAlerts(_ context.Context) ([]prom.Alert, error) {
	return []prom.Alert{{State: "firing", Labels: map[string]string{"alertname": "TestDefaultAlert", "severity": "warning"}}}, nil
}
func (a *alwaysFiringPromClientIT) GetRules(_ context.Context) ([]prom.RuleGroup, error) {
	return nil, nil
}
func (a *alwaysFiringPromClientIT) InstantQuery(_ context.Context, _ string) (*prom.QueryResult, error) {
	return &prom.QueryResult{}, nil
}

func defaultTestTriagerIT() *severity.Triager {
	return severity.NewTriager(&alwaysFiringPromClientIT{}, severity.NewNoopLLMTriager(logr.Discard()), severity.DefaultConfig(), logr.Discard())
}

// unnamedAlertTestTriagerIT resolves a severity from a resource-matching
// firing alert with no "alertname" label, so signalNameFromTriage() falls
// through to "" -- for IT tests proving the signalName K8s-events/"unknown"
// fallback still triggers even though severity triage itself succeeds.
func unnamedAlertTestTriagerIT(namespace, kind, name string) *severity.Triager {
	mockProm := &podCorrelationPromClient{
		alerts: []prom.Alert{
			{State: "firing", Labels: map[string]string{
				"namespace": namespace, "kind": kind, "name": name, "severity": "warning",
			}},
		},
	}
	return severity.NewTriager(mockProm, severity.NewNoopLLMTriager(logr.Discard()), severity.DefaultConfig(), logr.Discard())
}

var _ = Describe("kubernaut_remediate wiring (#1282, #1332)", func() {
	rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}
	eventsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

	It("IT-AF-1282-W01: HandleCreateRR creates RR in AF-resolved namespace via envtest", func() {
		ctx := context.Background()
		ns := "kubernaut-system"

		nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
		err := k8sClient.Create(ctx, nsObj)
		if err != nil {
			// namespace may already exist
			GinkgoWriter.Printf("namespace create: %v (may already exist)\n", err)
		}
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, nsObj)
		})

		result, err := tools.HandleCreateRR(ctx, &tools.ToolDeps{Client: k8sClient, DynClient: dynamicClient, ControllerNS: ns, Triager: defaultTestTriagerIT()}, &tools.CreateRRArgs{
			Namespace:   ns,
			Kind:        "Deployment",
			Name:        "web-w01",
			Description: "IT wiring test",
		}, "it-user")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).To(HavePrefix("rr-"))
		Expect(result.AlreadyExists).To(BeFalse())

		created, getErr := dynamicClient.Resource(rrGVR).Namespace(ns).Get(ctx, result.RRID, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		metaNS := created.GetNamespace()
		Expect(metaNS).To(Equal(ns), "CRD metadata.namespace = controllerNS (ADR-057)")

		targetNS, _, _ := unstructured.NestedString(created.Object, "spec", "targetResource", "namespace")
		Expect(targetNS).To(Equal(ns), "targetResource.namespace = workloadNS (same-NS case per ADR-057)")

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(ns).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-1282-W02: created RR has signalSource=a2a-agent in envtest", func() {
		ctx := context.Background()

		result, err := tools.HandleCreateRR(ctx, &tools.ToolDeps{Client: k8sClient, DynClient: dynamicClient, ControllerNS: defaultFixture, Triager: defaultTestTriagerIT()}, &tools.CreateRRArgs{
			Namespace:   defaultFixture,
			Kind:        "Deployment",
			Name:        "web-w02",
			Description: "signal source check",
		}, "it-user")
		Expect(err).NotTo(HaveOccurred())

		created, getErr := dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Get(ctx, result.RRID, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		source, _, _ := unstructured.NestedString(created.Object, "spec", "signalSource")
		Expect(source).To(Equal("a2a-agent"))

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-1282-W03: signalName falls back to unknown in envtest", func() {
		ctx := context.Background()

		result, err := tools.HandleCreateRR(ctx, &tools.ToolDeps{Client: k8sClient, DynClient: dynamicClient, ControllerNS: defaultFixture, Triager: unnamedAlertTestTriagerIT(defaultFixture, "Deployment", "web-w03")}, &tools.CreateRRArgs{
			Namespace:   defaultFixture,
			Kind:        "Deployment",
			Name:        "web-w03",
			Description: "signal name check",
		}, "it-user")
		Expect(err).NotTo(HaveOccurred())

		created, getErr := dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Get(ctx, result.RRID, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
		Expect(signalName).To(Equal("unknown"),
			"with an unnamed triage result and no events, fallback should be unknown")

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-1282-W03b: K8s Warning event drives signalName via envtest", func() {
		ctx := context.Background()

		ev := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Event",
				"metadata": map[string]interface{}{
					"name":      "oom-event-w03b",
					"namespace": defaultFixture,
				},
				"reason":  "OOMKilling",
				"message": "Container killed due to OOM",
				"type":    "Warning",
				"involvedObject": map[string]interface{}{
					"kind": "Deployment",
					"name": "web-w03b",
				},
				"count":         int64(3),
				"lastTimestamp": "2026-05-25T00:00:00Z",
			},
		}
		_, err := dynamicClient.Resource(eventsGVR).Namespace(defaultFixture).Create(ctx, ev, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = dynamicClient.Resource(eventsGVR).Namespace(defaultFixture).Delete(ctx, "oom-event-w03b", metav1.DeleteOptions{})
		})

		result, err := tools.HandleCreateRR(ctx, &tools.ToolDeps{Client: k8sClient, DynClient: dynamicClient, ControllerNS: defaultFixture, Triager: unnamedAlertTestTriagerIT(defaultFixture, "Deployment", "web-w03b")}, &tools.CreateRRArgs{
			Namespace:   defaultFixture,
			Kind:        "Deployment",
			Name:        "web-w03b",
			Description: "OOM event in envtest",
		}, "it-user")
		Expect(err).NotTo(HaveOccurred())

		created, getErr := dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Get(ctx, result.RRID, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
		Expect(signalName).To(Equal("OOMKilling"), "K8s OOMKilling event should drive signalName")

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-1282-W04: severity triage wires through HandleCreateRR in envtest", func() {
		ctx := context.Background()
		noopLLM := severity.NewNoopLLMTriager(logr.Discard())
		cfg := severity.DefaultConfig()
		// #1839: Tier 3 (pure-LLM, zero-evidence) fallback was removed, so
		// this must supply a real firing alert to reach a resolvable
		// severity through the production pipeline -- an ungrounded call
		// now correctly fails closed (see IT-AF-1839-001 below).
		promClient := &podCorrelationPromClient{
			alerts: []prom.Alert{
				{State: "firing", Labels: map[string]string{
					"alertname": "HighErrorRate", "namespace": defaultFixture, "kind": "Deployment", "name": "web-w04", "severity": "critical",
				}},
			},
		}
		triager := severity.NewTriager(promClient, noopLLM, cfg, logr.Discard())

		result, err := tools.HandleCreateRR(ctx, &tools.ToolDeps{Client: k8sClient, DynClient: dynamicClient, ControllerNS: defaultFixture, Triager: triager}, &tools.CreateRRArgs{
			Namespace:   defaultFixture,
			Kind:        "Deployment",
			Name:        "web-w04",
			Description: "triage wiring IT",
		}, "it-user")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).NotTo(BeEmpty())
		Expect(result.Severity).To(Equal("critical"), "severity must come from the real firing alert via the production Triager wiring")

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-1839-001: HandleCreateRR fails closed via envtest when no alert or rule correlates to the resource", func() {
		ctx := context.Background()
		noopLLM := severity.NewNoopLLMTriager(logr.Discard())
		cfg := severity.DefaultConfig()
		triager := severity.NewTriager(&noopPromClientIT{}, noopLLM, cfg, logr.Discard())

		result, err := tools.HandleCreateRR(ctx, &tools.ToolDeps{Client: k8sClient, DynClient: dynamicClient, ControllerNS: defaultFixture, Triager: triager}, &tools.CreateRRArgs{
			Namespace:   defaultFixture,
			Kind:        "Deployment",
			Name:        "web-1839-nogrounding",
			Description: "no alert or rule exists for this resource",
		}, "it-user")
		Expect(errors.Is(err, severity.ErrSeverityUndetermined)).To(BeTrue(),
			"#1839: production wiring must propagate ErrSeverityUndetermined, not fabricate a severity")
		Expect(result.RRID).To(BeEmpty())

		list, listErr := dynamicClient.Resource(rrGVR).Namespace(defaultFixture).List(ctx, metav1.ListOptions{})
		Expect(listErr).NotTo(HaveOccurred())
		for _, item := range list.Items {
			name, _, _ := unstructured.NestedString(item.Object, "spec", "targetResource", "name")
			Expect(name).NotTo(Equal("web-1839-nogrounding"),
				"no RemediationRequest should be created in envtest when severity cannot be grounded")
		}
	})

	It("IT-AF-1292-W01: envtest creates RR in controllerNS with targetResource in workloadNS (BR-PLATFORM-057)", func() {
		ctx := context.Background()
		controllerNS := "it-ctrl-" + uuid.New().String()[:8]
		workloadNS := "it-wl-" + uuid.New().String()[:8]

		ctrlNSObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: controllerNS}}
		Expect(k8sClient.Create(ctx, ctrlNSObj)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, ctrlNSObj)
		})

		workloadNSObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: workloadNS}}
		Expect(k8sClient.Create(ctx, workloadNSObj)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, workloadNSObj)
		})

		result, err := tools.HandleCreateRR(ctx, &tools.ToolDeps{Client: k8sClient, DynClient: dynamicClient, ControllerNS: controllerNS, Triager: defaultTestTriagerIT()}, &tools.CreateRRArgs{
			Namespace:   workloadNS,
			Kind:        "Deployment",
			Name:        "web-1292-w01",
			Description: "ADR-057 namespace split IT",
		}, "it-user")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).To(HavePrefix("rr-"))

		created, getErr := dynamicClient.Resource(rrGVR).Namespace(controllerNS).Get(ctx, result.RRID, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		metaNS := created.GetNamespace()
		Expect(metaNS).To(Equal(controllerNS), "CRD metadata.namespace must be controllerNS")

		targetNS, _, _ := unstructured.NestedString(created.Object, "spec", "targetResource", "namespace")
		Expect(targetNS).To(Equal(workloadNS),
			"spec.targetResource.namespace must be workloadNS, not controllerNS")

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(controllerNS).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-1292-W02: prompt includes workload namespace instruction and rejects old single-NS wording (BR-PLATFORM-057, CM-6)", func() {
		instruction := agentpkg.BuildInstruction("kubernaut-system", true)

		Expect(instruction).To(ContainSubstring("provide: api_version, namespace, kind, name, description"),
			"prompt must list namespace as an LLM-provided field for kubernaut_remediate")
		Expect(instruction).To(ContainSubstring("namespace is the workload namespace"),
			"prompt must clarify that namespace is the workload namespace")
		Expect(instruction).NotTo(ContainSubstring("namespace: from AF's deployment context"),
			"old single-namespace wording must be removed (ADR-057)")
	})

	It("IT-AF-1282-W05: BuildInstruction contains Tool Usage Rules with resolved namespace", func() {
		resolvedNS := agentpkg.ResolveNamespace("", "/nonexistent/path")
		Expect(resolvedNS).To(Equal(defaultFixture))

		dir := GinkgoT().TempDir()
		nsFile := dir + "/namespace"
		Expect(os.WriteFile(nsFile, []byte("it-namespace"), 0o644)).To(Succeed())
		resolvedNS = agentpkg.ResolveNamespace("", nsFile)
		Expect(resolvedNS).To(Equal("it-namespace"))

		instruction := agentpkg.BuildInstruction(resolvedNS, true)
		Expect(instruction).To(ContainSubstring("## Tool Usage Rules"))
		Expect(instruction).To(ContainSubstring("kubernaut MCP tools"))
		Expect(instruction).To(ContainSubstring("NEVER use kubectl"))
		Expect(instruction).To(ContainSubstring("it-namespace"))
		Expect(instruction).To(ContainSubstring("namespace is the workload namespace"))
	})

	It("IT-FLEET-004: HandleCreateRR with ClusterID produces RR with cluster fields in envtest (BR-INTEGRATION-065)", func() {
		ctx := context.Background()

		result, err := tools.HandleCreateRR(ctx, &tools.ToolDeps{Client: k8sClient, DynClient: dynamicClient, ControllerNS: defaultFixture, Triager: defaultTestTriagerIT()}, &tools.CreateRRArgs{
			Namespace:   defaultFixture,
			Kind:        "Deployment",
			Name:        "web-fleet-004-" + uuid.New().String()[:6],
			Description: "fleet cluster wiring IT",
			ClusterID:   "prod-east-1",
		}, "fleet-user")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).To(HavePrefix("rr-"))

		created, getErr := dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Get(ctx, result.RRID, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		clusterID, _, _ := unstructured.NestedString(created.Object, "spec", "clusterID")
		Expect(clusterID).To(Equal("prod-east-1"),
			"ClusterID must be persisted on the RR CRD via envtest K8s API")
		// Issue #1651: clusterName was removed — non-unique, unsafe for disambiguation.
		_, found, _ := unstructured.NestedString(created.Object, "spec", "clusterName")
		Expect(found).To(BeFalse(), "spec.clusterName must not be persisted (issue #1651)")

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})

	It("IT-FLEET-005: same resource on different clusters produces distinct RRs in envtest (BR-INTEGRATION-065)", func() {
		ctx := context.Background()
		baseName := "web-fleet-005-" + uuid.New().String()[:6]

		result1, err := tools.HandleCreateRR(ctx, &tools.ToolDeps{Client: k8sClient, DynClient: dynamicClient, ControllerNS: defaultFixture, Triager: defaultTestTriagerIT()}, &tools.CreateRRArgs{
			Namespace:   defaultFixture,
			Kind:        "Deployment",
			Name:        baseName,
			Description: "east cluster",
			ClusterID:   "cluster-east",
		}, "fleet-user")
		Expect(err).NotTo(HaveOccurred())
		Expect(result1.AlreadyExists).To(BeFalse())

		result2, err := tools.HandleCreateRR(ctx, &tools.ToolDeps{Client: k8sClient, DynClient: dynamicClient, ControllerNS: defaultFixture, Triager: defaultTestTriagerIT()}, &tools.CreateRRArgs{
			Namespace:   defaultFixture,
			Kind:        "Deployment",
			Name:        baseName,
			Description: "west cluster",
			ClusterID:   "cluster-west",
		}, "fleet-user")
		Expect(err).NotTo(HaveOccurred())
		Expect(result2.AlreadyExists).To(BeFalse(),
			"same resource on a different cluster must NOT be deduplicated (ADR-065)")
		Expect(result2.RRID).NotTo(Equal(result1.RRID),
			"different clusters must produce different RR IDs")

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Delete(ctx, result1.RRID, metav1.DeleteOptions{})
			_ = dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Delete(ctx, result2.RRID, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-1282-W06: audit events emitted on RR creation in envtest", func() {
		ctx := context.Background()
		auditRecorder.Reset()

		result, err := tools.HandleCreateRR(ctx, &tools.ToolDeps{Client: k8sClient, DynClient: dynamicClient, ControllerNS: defaultFixture, Auditor: auditRecorder, Triager: defaultTestTriagerIT()}, &tools.CreateRRArgs{
			Namespace:   defaultFixture,
			Kind:        "Deployment",
			Name:        "web-w06",
			Description: "audit IT",
		}, "audit-user")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.AlreadyExists).To(BeFalse())

		events := auditRecorder.EventsOfType(audit.EventRRCreated)
		Expect(events).To(HaveLen(1))
		Expect(events[0].UserID).To(Equal("audit-user"))
		Expect(events[0].Detail).To(HaveKeyWithValue("namespace", defaultFixture))

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})
})
