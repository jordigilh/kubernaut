package apifrontend_test

import (
	"context"
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

var _ = Describe("af_create_rr wiring (#1282)", func() {
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

		result, err := tools.HandleCreateRR(ctx, dynamicClient, ns, &tools.CreateRRArgs{
			Namespace:   ns,
			Kind:        "Deployment",
			Name:        "web-w01",
			Description: "IT wiring test",
		}, "it-user", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).To(HavePrefix(ns + "/"))
		Expect(result.AlreadyExists).To(BeFalse())

		rrName := result.RRID[len(ns)+1:]
		created, getErr := dynamicClient.Resource(rrGVR).Namespace(ns).Get(ctx, rrName, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		metaNS := created.GetNamespace()
		Expect(metaNS).To(Equal(ns), "CRD metadata.namespace = controllerNS (ADR-057)")

		targetNS, _, _ := unstructured.NestedString(created.Object, "spec", "targetResource", "namespace")
		Expect(targetNS).To(Equal(ns), "targetResource.namespace = workloadNS (same-NS case per ADR-057)")

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(ns).Delete(ctx, rrName, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-1282-W02: created RR has signalSource=a2a-agent in envtest", func() {
		ctx := context.Background()

		result, err := tools.HandleCreateRR(ctx, dynamicClient, "default", &tools.CreateRRArgs{
			Namespace:   "default",
			Kind:        "Deployment",
			Name:        "web-w02",
			Description: "signal source check",
		}, "it-user", nil, nil)
		Expect(err).NotTo(HaveOccurred())

		rrName := result.RRID[len("default")+1:]
		created, getErr := dynamicClient.Resource(rrGVR).Namespace("default").Get(ctx, rrName, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		source, _, _ := unstructured.NestedString(created.Object, "spec", "signalSource")
		Expect(source).To(Equal("a2a-agent"))

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace("default").Delete(ctx, rrName, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-1282-W03: signalName falls back to unknown in envtest", func() {
		ctx := context.Background()

		result, err := tools.HandleCreateRR(ctx, dynamicClient, "default", &tools.CreateRRArgs{
			Namespace:   "default",
			Kind:        "Deployment",
			Name:        "web-w03",
			Description: "signal name check",
		}, "it-user", nil, nil)
		Expect(err).NotTo(HaveOccurred())

		rrName := result.RRID[len("default")+1:]
		created, getErr := dynamicClient.Resource(rrGVR).Namespace("default").Get(ctx, rrName, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
		Expect(signalName).To(Equal("unknown"),
			"with no triager and no events, fallback should be unknown")

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace("default").Delete(ctx, rrName, metav1.DeleteOptions{})
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
					"namespace": "default",
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
		_, err := dynamicClient.Resource(eventsGVR).Namespace("default").Create(ctx, ev, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = dynamicClient.Resource(eventsGVR).Namespace("default").Delete(ctx, "oom-event-w03b", metav1.DeleteOptions{})
		})

		result, err := tools.HandleCreateRR(ctx, dynamicClient, "default", &tools.CreateRRArgs{
			Namespace:   "default",
			Kind:        "Deployment",
			Name:        "web-w03b",
			Description: "OOM event in envtest",
		}, "it-user", nil, nil)
		Expect(err).NotTo(HaveOccurred())

		rrName := result.RRID[len("default")+1:]
		created, getErr := dynamicClient.Resource(rrGVR).Namespace("default").Get(ctx, rrName, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
		Expect(signalName).To(Equal("OOMKilling"), "K8s OOMKilling event should drive signalName")

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace("default").Delete(ctx, rrName, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-1282-W04: severity triage wires through HandleCreateRR in envtest", func() {
		ctx := context.Background()
		noopLLM := severity.NewNoopLLMTriager(logr.Discard())
		cfg := severity.DefaultConfig()
		triager := severity.NewTriager(&noopPromClientIT{}, noopLLM, cfg, logr.Discard())

		result, err := tools.HandleCreateRR(ctx, dynamicClient, "default", &tools.CreateRRArgs{
			Namespace:   "default",
			Kind:        "Deployment",
			Name:        "web-w04",
			Description: "triage wiring IT",
		}, "it-user", triager, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).NotTo(BeEmpty())
		Expect(result.Severity).NotTo(BeEmpty())

		DeferCleanup(func() {
			rrName := result.RRID[len("default")+1:]
			_ = dynamicClient.Resource(rrGVR).Namespace("default").Delete(ctx, rrName, metav1.DeleteOptions{})
		})
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

		result, err := tools.HandleCreateRR(ctx, dynamicClient, controllerNS, &tools.CreateRRArgs{
			Namespace:   workloadNS,
			Kind:        "Deployment",
			Name:        "web-1292-w01",
			Description: "ADR-057 namespace split IT",
		}, "it-user", nil, nil)
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
		instruction := agentpkg.BuildInstruction("kubernaut-system")

		Expect(instruction).To(ContainSubstring("provide: namespace, kind, name, description"),
			"prompt must list namespace as an LLM-provided field for af_create_rr")
		Expect(instruction).To(ContainSubstring("namespace is the workload namespace"),
			"prompt must clarify that namespace is the workload namespace")
		Expect(instruction).NotTo(ContainSubstring("namespace: from AF's deployment context"),
			"old single-namespace wording must be removed (ADR-057)")
	})

	It("IT-AF-1282-W05: BuildInstruction contains Tool Usage Rules with resolved namespace", func() {
		resolvedNS := agentpkg.ResolveNamespace("", "/nonexistent/path")
		Expect(resolvedNS).To(Equal("default"))

		dir := GinkgoT().TempDir()
		nsFile := dir + "/namespace"
		Expect(os.WriteFile(nsFile, []byte("it-namespace"), 0o644)).To(Succeed())
		resolvedNS = agentpkg.ResolveNamespace("", nsFile)
		Expect(resolvedNS).To(Equal("it-namespace"))

		instruction := agentpkg.BuildInstruction(resolvedNS)
		Expect(instruction).To(ContainSubstring("## Tool Usage Rules"))
		Expect(instruction).To(ContainSubstring("kubernaut MCP tools"))
		Expect(instruction).To(ContainSubstring("NEVER use kubectl"))
		Expect(instruction).To(ContainSubstring("it-namespace"))
		Expect(instruction).To(ContainSubstring("namespace is the workload namespace"))
	})

	It("IT-AF-1282-W06: audit events emitted on RR creation in envtest", func() {
		ctx := context.Background()
		auditRecorder.Reset()

		result, err := tools.HandleCreateRR(ctx, dynamicClient, "default", &tools.CreateRRArgs{
			Namespace:   "default",
			Kind:        "Deployment",
			Name:        "web-w06",
			Description: "audit IT",
		}, "audit-user", nil, auditRecorder)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.AlreadyExists).To(BeFalse())

		events := auditRecorder.EventsOfType(audit.EventRRCreated)
		Expect(events).To(HaveLen(1))
		Expect(events[0].UserID).To(Equal("audit-user"))
		Expect(events[0].Detail).To(HaveKeyWithValue("namespace", "default"))

		DeferCleanup(func() {
			rrName := result.RRID[len("default")+1:]
			_ = dynamicClient.Resource(rrGVR).Namespace("default").Delete(ctx, rrName, metav1.DeleteOptions{})
		})
	})
})
