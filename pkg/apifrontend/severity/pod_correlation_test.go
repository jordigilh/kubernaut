package severity_test

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
)

func newPodResolver(client *dynamicfake.FakeDynamicClient) *severity.K8sPodResolver {
	return severity.NewK8sPodResolver(client, logr.Discard())
}

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = appsv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	return s
}

var _ = Describe("Pod-Based Alert Correlation", func() {

	Describe("K8sPodResolver", func() {
		It("UT-AF-TRIAGE-004: returns pod names for Deployment via selector labels", func() {
			scheme := newScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "default"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "worker"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "worker"}},
					},
				},
			}
			pod1 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-abc-xyz", Namespace: "default",
					Labels: map[string]string{"app": "worker"},
				},
			}
			pod2 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-def-123", Namespace: "default",
					Labels: map[string]string{"app": "worker"},
				},
			}
			unrelatedPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-api-789", Namespace: "default",
					Labels: map[string]string{"app": "worker-api"},
				},
			}

			client := dynamicfake.NewSimpleDynamicClient(scheme, deploy, pod1, pod2, unrelatedPod)
			resolver := newPodResolver(client)

			names, err := resolver.ResolvePodNames(context.Background(), "default", "Deployment", "worker")
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(ConsistOf("worker-abc-xyz", "worker-def-123"))
			Expect(names).NotTo(ContainElement("worker-api-789"))
		})

		It("UT-AF-TRIAGE-005: returns empty for unsupported kind (graceful degradation)", func() {
			scheme := newScheme()
			client := dynamicfake.NewSimpleDynamicClient(scheme)
			resolver := newPodResolver(client)

			names, err := resolver.ResolvePodNames(context.Background(), "default", "CronJob", "batch-job")
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(BeNil())
		})

		It("UT-AF-TRIAGE-006: returns empty when workload not found (graceful degradation)", func() {
			scheme := newScheme()
			client := dynamicfake.NewSimpleDynamicClient(scheme)
			resolver := newPodResolver(client)

			names, err := resolver.ResolvePodNames(context.Background(), "default", "Deployment", "nonexistent")
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(BeNil())
		})

		It("UT-AF-TRIAGE-004b: returns pod names for StatefulSet via selector labels", func() {
			scheme := newScheme()
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "default"},
				Spec: appsv1.StatefulSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "redis"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "redis"}},
					},
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "redis-0", Namespace: "default",
					Labels: map[string]string{"app": "redis"},
				},
			}

			client := dynamicfake.NewSimpleDynamicClient(scheme, sts, pod)
			resolver := newPodResolver(client)

			names, err := resolver.ResolvePodNames(context.Background(), "default", "StatefulSet", "redis")
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(ConsistOf("redis-0"))
		})

		It("UT-AF-TRIAGE-004c: returns pod names for DaemonSet via selector labels", func() {
			scheme := newScheme()
			ds := &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "fluentd", Namespace: "kube-system"},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "fluentd"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "fluentd"}},
					},
				},
			}
			pod1 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fluentd-node1", Namespace: "kube-system",
					Labels: map[string]string{"app": "fluentd"},
				},
			}
			pod2 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fluentd-node2", Namespace: "kube-system",
					Labels: map[string]string{"app": "fluentd"},
				},
			}

			client := dynamicfake.NewSimpleDynamicClient(scheme, ds, pod1, pod2)
			resolver := newPodResolver(client)

			names, err := resolver.ResolvePodNames(context.Background(), "kube-system", "DaemonSet", "fluentd")
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(ConsistOf("fluentd-node1", "fluentd-node2"))
		})

		It("UT-AF-TRIAGE-004d: returns pod names for Deployment with matchExpressions selector (M7)", func() {
			scheme := newScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "mixed-selector", Namespace: "default"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "worker"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "tier",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"backend", "worker"},
							},
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "worker", "tier": "backend"}},
					},
				},
			}
			matchingPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-match", Namespace: "default",
					Labels: map[string]string{"app": "worker", "tier": "backend"},
				},
			}
			nonMatchingPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-nomatch", Namespace: "default",
					Labels: map[string]string{"app": "worker", "tier": "frontend"},
				},
			}

			client := dynamicfake.NewSimpleDynamicClient(scheme, deploy, matchingPod, nonMatchingPod)
			resolver := newPodResolver(client)

			names, err := resolver.ResolvePodNames(context.Background(), "default", "Deployment", "mixed-selector")
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(ConsistOf("worker-match"))
			Expect(names).NotTo(ContainElement("worker-nomatch"))
		})

		It("UT-AF-TRIAGE-008: returns empty when selector is empty (M8 guard)", func() {
			scheme := newScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-selector", Namespace: "default"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{},
					Template: corev1.PodTemplateSpec{},
				},
			}
			client := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			resolver := newPodResolver(client)

			names, err := resolver.ResolvePodNames(context.Background(), "default", "Deployment", "empty-selector")
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(BeNil())
		})
	})

	Describe("Tier 1 Pod Correlation via Triager", func() {
		It("UT-AF-TRIAGE-001: firing alert matches via pod name when kind/name labels don't overlap", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{
						Labels: map[string]string{
							"alertname": "KubePodCrashLooping",
							"pod":       "worker-abc-xyz",
							"container": "worker",
							"namespace": "default",
							"severity":  "warning",
						},
						State: "firing",
					},
				},
			}

			input := severity.TriageInput{
				Namespace:   "default",
				Kind:        "Deployment",
				Name:        "worker",
				Description: "CrashLoopBackOff on worker",
				Labels:      map[string]string{"namespace": "default", "kind": "Deployment", "name": "worker"},
				PodNames:    []string{"worker-abc-xyz", "worker-def-123"},
			}

			triager := severity.NewTriager(mockProm, &mockLLM{}, severity.DefaultConfig(), logr.Discard())
			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("warning"))
			Expect(result.Source).To(Equal(severity.SourceFiringAlert))
			Expect(result.AlertName).To(Equal("KubePodCrashLooping"))
		})

		It("UT-AF-TRIAGE-002: pod matches but namespace differs — no correlation (M3 guard)", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{
						Labels: map[string]string{
							"alertname": "KubePodCrashLooping",
							"pod":       "worker-abc-xyz",
							"namespace": "staging",
							"severity":  "warning",
						},
						State: "firing",
					},
				},
			}
			mockLLM := &mockLLM{
				pureResult: severity.TriageResult{Severity: "medium", Source: severity.SourceLLMTriage},
			}

			input := severity.TriageInput{
				Namespace:   "default",
				Kind:        "Deployment",
				Name:        "worker",
				Description: "CrashLoopBackOff on worker",
				Labels:      map[string]string{"namespace": "default", "kind": "Deployment", "name": "worker"},
				PodNames:    []string{"worker-abc-xyz"},
			}

			triager := severity.NewTriager(mockProm, mockLLM, severity.DefaultConfig(), logr.Discard())
			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).NotTo(Equal(severity.SourceFiringAlert))
		})

		It("UT-AF-TRIAGE-003: existing key-overlap path still works (no regression)", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{
						Labels: map[string]string{
							"alertname": "HighCPU",
							"namespace": "prod",
							"kind":      "Deployment",
							"name":      "web-api",
							"severity":  "critical",
						},
						State: "firing",
					},
				},
			}

			input := severity.TriageInput{
				Namespace:   "prod",
				Kind:        "Deployment",
				Name:        "web-api",
				Description: "High CPU usage",
				Labels:      map[string]string{"namespace": "prod", "kind": "Deployment", "name": "web-api"},
			}

			triager := severity.NewTriager(mockProm, &mockLLM{}, severity.DefaultConfig(), logr.Discard())
			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
			Expect(result.Source).To(Equal(severity.SourceFiringAlert))
		})

		It("UT-AF-TRIAGE-007: Triager auto-resolves pods via injected resolver and Tier 1 matches", func() {
			scheme := newScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "default"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "worker"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "worker"}},
					},
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-abc-xyz", Namespace: "default",
					Labels: map[string]string{"app": "worker"},
				},
			}

			k8sClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, pod)
			resolver := newPodResolver(k8sClient)

			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{
						Labels: map[string]string{
							"alertname": "KubePodCrashLooping",
							"pod":       "worker-abc-xyz",
							"namespace": "default",
							"severity":  "warning",
						},
						State: "firing",
					},
				},
			}

			input := severity.TriageInput{
				Namespace:   "default",
				Kind:        "Deployment",
				Name:        "worker",
				Description: "CrashLoopBackOff",
				Labels:      map[string]string{"namespace": "default", "kind": "Deployment", "name": "worker"},
			}

			triager := severity.NewTriager(mockProm, &mockLLM{}, severity.DefaultConfig(), logr.Discard(),
				severity.WithPodResolver(resolver))
			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("warning"))
			Expect(result.Source).To(Equal(severity.SourceFiringAlert))
			Expect(result.AlertName).To(Equal("KubePodCrashLooping"))
		})

		It("UT-AF-TRIAGE-009: multiple pod-correlated alerts returns highest severity", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{
						Labels: map[string]string{
							"alertname": "KubePodCrashLooping",
							"pod":       "worker-abc-xyz",
							"namespace": "default",
							"severity":  "warning",
						},
						State: "firing",
					},
					{
						Labels: map[string]string{
							"alertname": "KubePodOOMKilled",
							"pod":       "worker-def-123",
							"namespace": "default",
							"severity":  "critical",
						},
						State: "firing",
					},
					{
						Labels: map[string]string{
							"alertname": "UnrelatedAlert",
							"pod":       "other-pod",
							"namespace": "default",
							"severity":  "high",
						},
						State: "firing",
					},
				},
			}

			input := severity.TriageInput{
				Namespace:   "default",
				Kind:        "Deployment",
				Name:        "worker",
				Description: "Multiple issues on worker",
				Labels:      map[string]string{"namespace": "default", "kind": "Deployment", "name": "worker"},
				PodNames:    []string{"worker-abc-xyz", "worker-def-123"},
			}

			triager := severity.NewTriager(mockProm, &mockLLM{}, severity.DefaultConfig(), logr.Discard())
			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
			Expect(result.Source).To(Equal(severity.SourceFiringAlert))
			Expect(result.AlertName).To(Equal("KubePodOOMKilled"))
		})
	})

	Describe("Tier 1 Pending Alert Pod Correlation", func() {
		It("UT-AF-TRIAGE-010: pending alert matches via pod correlation when no firing alert exists [IR-4]", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{
						Labels: map[string]string{
							"alertname": "KubePodCrashLooping",
							"pod":       "worker-abc-xyz",
							"container": "worker",
							"namespace": "default",
							"severity":  "warning",
						},
						State: "pending",
					},
				},
			}

			input := severity.TriageInput{
				Namespace:   "default",
				Kind:        "Deployment",
				Name:        "worker",
				Description: "CrashLoopBackOff on worker",
				Labels:      map[string]string{"namespace": "default", "kind": "Deployment", "name": "worker"},
				PodNames:    []string{"worker-abc-xyz", "worker-def-123"},
			}

			triager := severity.NewTriager(mockProm, &mockLLM{}, severity.DefaultConfig(), logr.Discard())
			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("warning"))
			Expect(result.Source).To(Equal(severity.SourcePendingAlert),
				"IR-4: pending alert with pod correlation must be resolved before falling to LLM/K8s events")
			Expect(result.AlertName).To(Equal("KubePodCrashLooping"))
		})

		It("UT-AF-TRIAGE-011: firing alert takes priority over pending alert for same pod", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{
						Labels: map[string]string{
							"alertname": "KubePodNotReady",
							"pod":       "worker-abc-xyz",
							"namespace": "default",
							"severity":  "warning",
						},
						State: "firing",
					},
					{
						Labels: map[string]string{
							"alertname": "KubePodCrashLooping",
							"pod":       "worker-abc-xyz",
							"namespace": "default",
							"severity":  "critical",
						},
						State: "pending",
					},
				},
			}

			input := severity.TriageInput{
				Namespace:   "default",
				Kind:        "Deployment",
				Name:        "worker",
				Description: "CrashLoopBackOff on worker",
				Labels:      map[string]string{"namespace": "default", "kind": "Deployment", "name": "worker"},
				PodNames:    []string{"worker-abc-xyz"},
			}

			triager := severity.NewTriager(mockProm, &mockLLM{}, severity.DefaultConfig(), logr.Discard())
			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceFiringAlert),
				"firing alert must take priority over pending alert")
			Expect(result.AlertName).To(Equal("KubePodNotReady"))
		})

		It("UT-AF-TRIAGE-012: pending alert with pod correlation wins over LLM fallback [IR-4]", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{
						Labels: map[string]string{
							"alertname": "KubePodOOMKilled",
							"pod":       "worker-def-123",
							"namespace": "default",
							"severity":  "critical",
						},
						State: "pending",
					},
				},
			}
			mockLLM := &mockLLM{
				pureResult: severity.TriageResult{Severity: "medium", Source: severity.SourceLLMTriage},
			}

			input := severity.TriageInput{
				Namespace:   "default",
				Kind:        "Deployment",
				Name:        "worker",
				Description: "OOM on worker",
				Labels:      map[string]string{"namespace": "default", "kind": "Deployment", "name": "worker"},
				PodNames:    []string{"worker-def-123"},
			}

			triager := severity.NewTriager(mockProm, mockLLM, severity.DefaultConfig(), logr.Discard())
			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourcePendingAlert),
				"IR-4: pending alert must win over LLM fallback")
			Expect(result.AlertName).To(Equal("KubePodOOMKilled"))
			Expect(result.Severity).To(Equal("critical"))
		})
	})

	Describe("PodResolver Error Handling", func() {
		It("pod resolver error degrades gracefully — namespace-scoped fallback catches alert (#1369)", func() {
			failingResolver := &failingPodResolver{err: errors.New("k8s unavailable")}

			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{
						Labels: map[string]string{
							"alertname": "KubePodCrashLooping",
							"pod":       "worker-abc",
							"namespace": "default",
							"severity":  "warning",
						},
						State: "firing",
					},
				},
			}
			mockLLM := &mockLLM{
				pureResult: severity.TriageResult{Severity: "medium", Source: severity.SourceLLMTriage},
			}

			input := severity.TriageInput{
				Namespace:   "default",
				Kind:        "Deployment",
				Name:        "worker",
				Description: "issue",
				Labels:      map[string]string{"namespace": "default", "kind": "Deployment", "name": "worker"},
			}

			triager := severity.NewTriager(mockProm, mockLLM, severity.DefaultConfig(), logr.Discard(),
				severity.WithPodResolver(failingResolver))
			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceNSFiringAlert),
				"#1369: namespace-scoped fallback should catch the alert when pod resolution fails")
			Expect(result.AlertName).To(Equal("KubePodCrashLooping"))
		})

		It("WithPodResolver option injects resolver at construction", func() {
			scheme := newScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "prod"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "web"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
					},
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "web-rs-abc", Namespace: "prod",
					Labels: map[string]string{"app": "web"},
				},
			}

			k8sClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, pod)

			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{
						Labels: map[string]string{
							"alertname": "HighMemory",
							"pod":       "web-rs-abc",
							"namespace": "prod",
							"severity":  "high",
						},
						State: "firing",
					},
				},
			}

			input := severity.TriageInput{
				Namespace:   "prod",
				Kind:        "Deployment",
				Name:        "web",
				Description: "OOM",
				Labels:      map[string]string{"namespace": "prod", "kind": "Deployment", "name": "web"},
			}

			triager := severity.NewTriager(mockProm, &mockLLM{}, severity.DefaultConfig(), logr.Discard(),
				severity.WithPodResolver(newPodResolver(k8sClient)))

			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("high"))
			Expect(result.Source).To(Equal(severity.SourceFiringAlert))
		})
	})
})

type failingPodResolver struct {
	err error
}

func (f *failingPodResolver) ResolvePodNames(_ context.Context, _, _, _ string) ([]string, error) {
	return nil, f.err
}
