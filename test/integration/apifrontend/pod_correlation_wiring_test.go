package apifrontend_test

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

type podCorrelationPromClient struct {
	alerts []prom.Alert
}

func (p *podCorrelationPromClient) GetAlerts(_ context.Context) ([]prom.Alert, error) {
	return p.alerts, nil
}
func (p *podCorrelationPromClient) GetRules(_ context.Context) ([]prom.RuleGroup, error) {
	return nil, nil
}
func (p *podCorrelationPromClient) InstantQuery(_ context.Context, _ string) (*prom.QueryResult, error) {
	return &prom.QueryResult{}, nil
}

var _ = Describe("Pod Correlation Wiring (#triage)", func() {
	rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}

	It("IT-AF-TRIAGE-010: HandleCreateRR resolves severity from firing alert via pod correlation in envtest", func() {
		ctx := context.Background()
		ns := "it-triage-" + uuid.New().String()[:8]

		nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
		Expect(k8sClient.Create(ctx, nsObj)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, nsObj) })

		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "worker"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "worker"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deploy)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, deploy) })

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-abc-xyz", Namespace: ns,
				Labels: map[string]string{"app": "worker"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, pod) })

		promClient := &podCorrelationPromClient{
			alerts: []prom.Alert{
				{
					Labels: map[string]string{
						"alertname": "KubePodCrashLooping",
						"pod":       "worker-abc-xyz",
						"container": "c",
						"namespace": ns,
						"severity":  "warning",
					},
					State: "firing",
				},
			},
		}

		resolver := severity.NewK8sPodResolver(dynamicClient, logr.Discard())
		triager := severity.NewTriager(
			promClient,
			severity.NewNoopLLMTriager(logr.Discard()),
			severity.DefaultConfig(),
			logr.Discard(),
			severity.WithPodResolver(resolver),
		)

		result, err := tools.HandleCreateRR(ctx, dynamicClient, ns, &tools.CreateRRArgs{
			Namespace:   ns,
			Kind:        "Deployment",
			Name:        "worker",
			Description: "CrashLoopBackOff on worker pod",
		}, "it-user", triager, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).To(HavePrefix("rr-"))
		Expect(result.Severity).To(Equal("warning"))
		Expect(result.SeveritySource).To(Equal("firing_alert"))

		created, getErr := dynamicClient.Resource(rrGVR).Namespace(ns).Get(ctx, result.RRID, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		sev, _, _ := unstructured.NestedString(created.Object, "spec", "severity")
		Expect(sev).To(Equal("warning"), "RR spec.severity should be from the firing alert")

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(ns).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-SEV-W02: spec.signalName resolves to Prometheus alert name via pod correlation (not BackOff)", func() {
		ctx := context.Background()
		ns := "it-signal-" + uuid.New().String()[:8]

		nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
		Expect(k8sClient.Create(ctx, nsObj)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, nsObj) })

		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api-server", Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "api-server"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api-server"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "busybox"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deploy)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, deploy) })

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api-server-rs-pod1", Namespace: ns,
				Labels: map[string]string{"app": "api-server"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "busybox"}}},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, pod) })

		promClient := &podCorrelationPromClient{
			alerts: []prom.Alert{
				{
					Labels: map[string]string{
						"alertname": "KubePodCrashLooping",
						"pod":       "api-server-rs-pod1",
						"namespace": ns,
						"severity":  "critical",
					},
					State: "firing",
				},
			},
		}

		resolver := severity.NewK8sPodResolver(dynamicClient, logr.Discard())
		triager := severity.NewTriager(
			promClient,
			severity.NewNoopLLMTriager(logr.Discard()),
			severity.DefaultConfig(),
			logr.Discard(),
			severity.WithPodResolver(resolver),
		)

		result, err := tools.HandleCreateRR(ctx, dynamicClient, ns, &tools.CreateRRArgs{
			Namespace:   ns,
			Kind:        "Deployment",
			Name:        "api-server",
			Description: "Pod crash looping",
		}, "it-user", triager, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).To(HavePrefix("rr-"))

		created, getErr := dynamicClient.Resource(rrGVR).Namespace(ns).Get(ctx, result.RRID, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
		Expect(signalName).To(Equal("KubePodCrashLooping"),
			"spec.signalName must be the Prometheus alert name, not a generic K8s event like 'BackOff'")

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(ns).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-TRIAGE-W01: WithPodResolver wiring pattern (main.go production path) works with envtest", func() {
		ctx := context.Background()
		ns := "it-triage-w-" + uuid.New().String()[:8]

		nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
		Expect(k8sClient.Create(ctx, nsObj)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, nsObj) })

		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "web"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deploy)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, deploy) })

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-rs-abc", Namespace: ns,
				Labels: map[string]string{"app": "web"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, pod) })

		promClient := &podCorrelationPromClient{
			alerts: []prom.Alert{
				{
					Labels: map[string]string{
						"alertname": "HighMemoryUsage",
						"pod":       "web-rs-abc",
						"namespace": ns,
						"severity":  "high",
					},
					State: "firing",
				},
			},
		}

		triager := severity.NewTriager(
			promClient,
			severity.NewNoopLLMTriager(logr.Discard()),
			severity.DefaultConfig(),
			logr.Discard(),
			severity.WithPodResolver(severity.NewK8sPodResolver(dynamicClient, logr.Discard())),
		)

		result, err := tools.HandleCreateRR(ctx, dynamicClient, ns, &tools.CreateRRArgs{
			Namespace:   ns,
			Kind:        "Deployment",
			Name:        "web",
			Description: "High memory usage",
		}, "it-user", triager, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Severity).To(Equal("high"))
		Expect(result.SeveritySource).To(Equal("firing_alert"))

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(ns).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})
})
