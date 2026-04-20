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

package k8s_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func int32Ptr(i int32) *int32 { return &i }

func findToolByName(reg *registry.Registry, name string) tools.Tool {
	for _, t := range reg.All() {
		if t.Name() == name {
			return t
		}
	}
	return nil
}

type fakeMetricsClient struct{}

func (f *fakeMetricsClient) ListPodMetrics(_ context.Context, _ string) (*metricsv1beta1.PodMetricsList, error) {
	return &metricsv1beta1.PodMetricsList{
		Items: []metricsv1beta1.PodMetrics{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "api-pod", Namespace: "default"},
				Containers: []metricsv1beta1.ContainerMetrics{
					{
						Name: "api",
						Usage: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
				},
			},
		},
	}, nil
}

func (f *fakeMetricsClient) ListNodeMetrics(_ context.Context) (*metricsv1beta1.NodeMetricsList, error) {
	return &metricsv1beta1.NodeMetricsList{
		Items: []metricsv1beta1.NodeMetrics{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
				Usage: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		},
	}, nil
}

func buildTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = policyv1.AddToScheme(scheme)
	_ = autoscalingv2.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	return scheme
}

func buildTestMapper() *meta.DefaultRESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "batch", Version: "v1"},
		{Group: "policy", Version: "v1"},
		{Group: "autoscaling", Version: "v2"},
		{Group: "networking.k8s.io", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Service"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Secret"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Event"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Node"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}, meta.RESTScopeNamespace)
	return mapper
}

func buildTestKindIndex() map[string]schema.GroupKind {
	return map[string]schema.GroupKind{
		"pod":                     {Kind: "Pod"},
		"deployment":              {Group: "apps", Kind: "Deployment"},
		"replicaset":              {Group: "apps", Kind: "ReplicaSet"},
		"statefulset":             {Group: "apps", Kind: "StatefulSet"},
		"daemonset":               {Group: "apps", Kind: "DaemonSet"},
		"service":                 {Kind: "Service"},
		"configmap":               {Kind: "ConfigMap"},
		"secret":                  {Kind: "Secret"},
		"event":                   {Kind: "Event"},
		"namespace":               {Kind: "Namespace"},
		"node":                    {Kind: "Node"},
		"poddisruptionbudget":     {Group: "policy", Kind: "PodDisruptionBudget"},
		"horizontalpodautoscaler": {Group: "autoscaling", Kind: "HorizontalPodAutoscaler"},
		"networkpolicy":           {Group: "networking.k8s.io", Kind: "NetworkPolicy"},
		"job":                     {Group: "batch", Kind: "Job"},
		"cronjob":                 {Group: "batch", Kind: "CronJob"},
	}
}

var _ = Describe("Kubernaut Agent K8s Kind Resolution — #433 Phase 2", func() {

	var reg *registry.Registry

	BeforeEach(func() {
		objects := []runtime.Object{
			&appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{Name: "api-rs", Namespace: "default"},
				Spec: appsv1.ReplicaSetSpec{
					Replicas: int32Ptr(2),
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
					},
				},
			},
			&appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "redis-ss", Namespace: "default"},
				Spec: appsv1.StatefulSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "redis"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "redis"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "redis", Image: "redis:7"}}},
					},
				},
			},
			&appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "fluentd-ds", Namespace: "kube-system"},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "fluentd"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "fluentd"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "fluentd", Image: "fluentd:v1"}}},
					},
				},
			},
			&policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{Name: "api-pdb", Namespace: "default"},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					Selector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
				},
			},
			&autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "api-hpa", Namespace: "default"},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind: "Deployment", Name: "api", APIVersion: "apps/v1",
					},
					MinReplicas: int32Ptr(1),
					MaxReplicas: 10,
				},
			},
			&networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "deny-all", Namespace: "default"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
				},
			},
			&batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "migration-job", Namespace: "default"},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers:    []corev1.Container{{Name: "migrate", Image: "migrate:v1"}},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api-pod", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "api", Image: "api:v1",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					},
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "backup-job", Namespace: "default"},
			Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers:    []corev1.Container{{Name: "backup", Image: "backup:v1"}},
						RestartPolicy: corev1.RestartPolicyNever,
					},
				},
			},
		},
		&batchv1.CronJob{
				ObjectMeta: metav1.ObjectMeta{Name: "cleanup-cron", Namespace: "default"},
				Spec: batchv1.CronJobSpec{
					Schedule: "0 * * * *",
					JobTemplate: batchv1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers:    []corev1.Container{{Name: "cleanup", Image: "cleanup:v1"}},
									RestartPolicy: corev1.RestartPolicyNever,
								},
							},
						},
					},
				},
			},
		}

		typedClient := fake.NewSimpleClientset(objects...)
		scheme := buildTestScheme()
		dynClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
		mapper := buildTestMapper()
		kindIndex := buildTestKindIndex()
		resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex)

		reg = registry.New()
		for _, t := range k8s.NewAllTools(typedClient, resolver) {
			reg.Register(t)
		}
		for _, t := range k8s.NewMetricsTools(&fakeMetricsClient{}) {
			reg.Register(t)
		}
	})

	Describe("UT-KA-433-520: kubectl_describe resolves ReplicaSet", func() {
		It("should return the ReplicaSet as JSON", func() {
			result, err := reg.Execute(context.Background(), "kubectl_describe",
				json.RawMessage(`{"kind":"ReplicaSet","name":"api-rs","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("api-rs"))
		})
	})

	Describe("UT-KA-433-521: kubectl_describe resolves StatefulSet", func() {
		It("should return the StatefulSet as JSON", func() {
			result, err := reg.Execute(context.Background(), "kubectl_describe",
				json.RawMessage(`{"kind":"StatefulSet","name":"redis-ss","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("redis-ss"))
		})
	})

	Describe("UT-KA-433-522: kubectl_describe resolves DaemonSet", func() {
		It("should return the DaemonSet as JSON", func() {
			result, err := reg.Execute(context.Background(), "kubectl_describe",
				json.RawMessage(`{"kind":"DaemonSet","name":"fluentd-ds","namespace":"kube-system"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("fluentd-ds"))
		})
	})

	Describe("UT-KA-433-523: kubectl_describe resolves PodDisruptionBudget", func() {
		It("should return the PDB as JSON", func() {
			result, err := reg.Execute(context.Background(), "kubectl_describe",
				json.RawMessage(`{"kind":"PodDisruptionBudget","name":"api-pdb","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("api-pdb"))
		})
	})

	Describe("UT-KA-433-524: kubectl_describe resolves HorizontalPodAutoscaler", func() {
		It("should return the HPA as JSON", func() {
			result, err := reg.Execute(context.Background(), "kubectl_describe",
				json.RawMessage(`{"kind":"HorizontalPodAutoscaler","name":"api-hpa","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("api-hpa"))
		})
	})

	Describe("UT-KA-433-525: kubectl_describe resolves NetworkPolicy", func() {
		It("should return the NetworkPolicy as JSON", func() {
			result, err := reg.Execute(context.Background(), "kubectl_describe",
				json.RawMessage(`{"kind":"NetworkPolicy","name":"deny-all","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("deny-all"))
		})
	})

	Describe("UT-KA-433-526: kubectl_describe resolves Job", func() {
		It("should return the Job as JSON", func() {
			result, err := reg.Execute(context.Background(), "kubectl_describe",
				json.RawMessage(`{"kind":"Job","name":"migration-job","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("migration-job"))
		})
	})

	Describe("UT-KA-433-527: kubectl_describe resolves CronJob", func() {
		It("should return the CronJob as JSON", func() {
			result, err := reg.Execute(context.Background(), "kubectl_describe",
				json.RawMessage(`{"kind":"CronJob","name":"cleanup-cron","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("cleanup-cron"))
		})
	})

	Describe("UT-KA-433-528: kubectl_get_by_kind_in_namespace lists ReplicaSets", func() {
		It("should return a list containing the seeded ReplicaSet", func() {
			result, err := reg.Execute(context.Background(), "kubectl_get_by_kind_in_namespace",
				json.RawMessage(`{"kind":"ReplicaSet","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("api-rs"))
		})
	})

	Describe("UT-KA-433-529: kubectl_get_by_kind_in_namespace lists Jobs", func() {
		It("should return a list containing the seeded Job", func() {
			result, err := reg.Execute(context.Background(), "kubectl_get_by_kind_in_namespace",
				json.RawMessage(`{"kind":"Job","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("migration-job"))
		})
	})

	Describe("UT-KA-433-530: kubectl_get_by_kind_in_namespace lists CronJobs", func() {
		It("should return a list containing the seeded CronJob", func() {
			result, err := reg.Execute(context.Background(), "kubectl_get_by_kind_in_namespace",
				json.RawMessage(`{"kind":"CronJob","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("cleanup-cron"))
		})
	})

	Describe("UT-KA-433-531: kubectl_get_by_kind_in_namespace lists NetworkPolicies", func() {
		It("should return a list containing the seeded NetworkPolicy", func() {
			result, err := reg.Execute(context.Background(), "kubectl_get_by_kind_in_namespace",
				json.RawMessage(`{"kind":"NetworkPolicy","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("deny-all"))
		})
	})

	Describe("UT-KA-433-532: case-insensitive kind resolution via ResourceResolver", func() {
		It("should resolve lowercase kind names", func() {
			result, err := reg.Execute(context.Background(), "kubectl_describe",
				json.RawMessage(`{"kind":"replicaset","name":"api-rs","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("api-rs"))
		})
	})

	Describe("UT-KA-433-501: kubectl_get_by_kind_in_cluster lists across all namespaces", func() {
		It("should return resources from all namespaces", func() {
			result, err := reg.Execute(context.Background(), "kubectl_get_by_kind_in_cluster",
				json.RawMessage(`{"kind":"DaemonSet"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("fluentd-ds"))
		})
	})

	Describe("UT-KA-433-502: kubectl_get_by_kind_in_cluster has correct schema", func() {
		It("should have kind as the only required parameter", func() {
			tool := findToolByName(reg, "kubectl_get_by_kind_in_cluster")
			Expect(tool).NotTo(BeNil(), "tool must be registered")
			schema := tool.Parameters()
			Expect(schema).NotTo(BeNil())
			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())
			required, ok := parsed["required"].([]interface{})
			Expect(ok).To(BeTrue(), "schema should have required array")
			Expect(required).To(ContainElement("kind"))
			Expect(required).NotTo(ContainElement("namespace"))
		})
	})

	Describe("UT-KA-433-503: kubectl_find_resource filters by keyword", func() {
		It("should return only resources matching the keyword substring", func() {
			result, err := reg.Execute(context.Background(), "kubectl_find_resource",
				json.RawMessage(`{"kind":"Job","keyword":"migration"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("migration-job"))
		})
	})

	Describe("UT-KA-433-504: kubectl_find_resource has correct schema", func() {
		It("should require kind and keyword parameters", func() {
			tool := findToolByName(reg, "kubectl_find_resource")
			Expect(tool).NotTo(BeNil(), "tool must be registered")
			schema := tool.Parameters()
			Expect(schema).NotTo(BeNil())
			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())
			required, ok := parsed["required"].([]interface{})
			Expect(ok).To(BeTrue(), "schema should have required array")
			Expect(required).To(ContainElement("kind"))
			Expect(required).To(ContainElement("keyword"))
		})
	})

	Describe("UT-KA-433-505: kubectl_get_yaml returns YAML output", func() {
		It("should return valid YAML for a Job", func() {
			result, err := reg.Execute(context.Background(), "kubectl_get_yaml",
				json.RawMessage(`{"kind":"Job","name":"migration-job","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("migration-job"))
			Expect(result).To(ContainSubstring("name:"))
			Expect(result).NotTo(HavePrefix("{"), "should be YAML, not JSON")
		})
	})

	Describe("UT-KA-433-506: kubectl_get_yaml has correct schema", func() {
		It("should require kind, name, and namespace", func() {
			tool := findToolByName(reg, "kubectl_get_yaml")
			Expect(tool).NotTo(BeNil(), "tool must be registered")
			schema := tool.Parameters()
			Expect(schema).NotTo(BeNil())
			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())
			required, ok := parsed["required"].([]interface{})
			Expect(ok).To(BeTrue(), "schema should have required array")
			Expect(required).To(ContainElement("kind"))
			Expect(required).To(ContainElement("name"))
		})
	})

	Describe("UT-KA-433-507: kubectl_memory_requests_all_namespaces", func() {
		It("should list memory requests for pods across all namespaces", func() {
			result, err := reg.Execute(context.Background(), "kubectl_memory_requests_all_namespaces",
				json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Mi"))
		})
	})

	Describe("UT-KA-433-508: kubectl_memory_requests_namespace", func() {
		It("should list memory requests for pods in a specific namespace", func() {
			result, err := reg.Execute(context.Background(), "kubectl_memory_requests_namespace",
				json.RawMessage(`{"namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
		})
	})

	Describe("UT-KA-433-509: kubernetes_jq_query applies jq expression", func() {
		It("should filter resources using a jq expression", func() {
			result, err := reg.Execute(context.Background(), "kubernetes_jq_query",
				json.RawMessage(`{"kind":"Pod","jq_expr":".items[].metadata.name"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("api-pod"))
		})
	})

	Describe("UT-KA-433-510: kubernetes_jq_query handles malformed expression", func() {
		It("should return an error for invalid jq syntax", func() {
			_, err := reg.Execute(context.Background(), "kubernetes_jq_query",
				json.RawMessage(`{"kind":"Pod","jq_expr":"..invalid[["}`))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UT-KA-433-511: kubernetes_count returns count + preview", func() {
		It("should return count and preview of matching resources", func() {
			result, err := reg.Execute(context.Background(), "kubernetes_count",
				json.RawMessage(`{"kind":"Pod","jq_expr":".items[].metadata.name"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Count:"))
			Expect(result).To(ContainSubstring("api-pod"))
		})
	})

	Describe("UT-KA-433-512: kubernetes_count has correct schema", func() {
		It("should require kind and jq_expr", func() {
			tool := findToolByName(reg, "kubernetes_count")
			Expect(tool).NotTo(BeNil(), "tool must be registered")
			schema := tool.Parameters()
			Expect(schema).NotTo(BeNil())
			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())
			required, ok := parsed["required"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(required).To(ContainElement("kind"))
			Expect(required).To(ContainElement("jq_expr"))
		})
	})

	Describe("UT-KA-433-600: kubectl_logs_all_containers_grep is registered", func() {
		It("should be a registered tool that accepts pattern parameter", func() {
			tool := findToolByName(reg, "kubectl_logs_all_containers_grep")
			Expect(tool).NotTo(BeNil(), "tool must be registered")
			Expect(tool.Description()).NotTo(BeEmpty())

			_, err := reg.Execute(context.Background(), "kubectl_logs_all_containers_grep",
				json.RawMessage(`{"name":"api-pod","namespace":"default","pattern":"error"}`))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-433-620: kubectl_top_pods is registered with correct schema", func() {
		It("should be a registered tool with namespace parameter", func() {
			tool := findToolByName(reg, "kubectl_top_pods")
			Expect(tool).NotTo(BeNil(), "tool must be registered")
			schema := tool.Parameters()
			Expect(schema).NotTo(BeNil())
			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())
			props := parsed["properties"].(map[string]interface{})
			Expect(props).To(HaveKey("namespace"))
		})

		It("should return formatted pod metrics with CPU and memory", func() {
			tool := findToolByName(reg, "kubectl_top_pods")
			Expect(tool).NotTo(BeNil())
			result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("NAMESPACE"))
			Expect(result).To(ContainSubstring("NAME"))
			Expect(result).To(ContainSubstring("CPU"))
			Expect(result).To(ContainSubstring("MEMORY"))
			Expect(result).To(ContainSubstring("api-pod"))
			Expect(result).To(ContainSubstring("100m"))
			Expect(result).To(ContainSubstring("128Mi"))
		})
	})

	Describe("UT-KA-433-621: kubectl_top_nodes is registered with correct schema", func() {
		It("should be a registered tool with no required parameters", func() {
			tool := findToolByName(reg, "kubectl_top_nodes")
			Expect(tool).NotTo(BeNil(), "tool must be registered")
			Expect(tool.Description()).NotTo(BeEmpty())
		})

		It("should return formatted node metrics with CPU and memory", func() {
			tool := findToolByName(reg, "kubectl_top_nodes")
			Expect(tool).NotTo(BeNil())
			result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("NAME"))
			Expect(result).To(ContainSubstring("CPU"))
			Expect(result).To(ContainSubstring("MEMORY"))
			Expect(result).To(ContainSubstring("node-1"))
			Expect(result).To(ContainSubstring("500m"))
			Expect(result).To(ContainSubstring("1024Mi"))
		})
	})

	// --- C1: Resolver edge-case tests ---

	Describe("UT-KA-433-540: resolver handles cluster-scoped Get (Node)", func() {
		It("should return a Node without requiring a namespace", func() {
			result, err := reg.Execute(context.Background(), "kubectl_describe",
				json.RawMessage(`{"kind":"Node","name":"worker-1","namespace":""}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("worker-1"))
		})
	})

	Describe("UT-KA-433-541: resolver handles cluster-scoped List (Namespace)", func() {
		It("should list Namespaces without requiring a namespace parameter", func() {
			result, err := reg.Execute(context.Background(), "kubectl_get_by_kind_in_cluster",
				json.RawMessage(`{"kind":"Namespace"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("test-namespace"))
		})
	})

	Describe("UT-KA-433-542: resolver fallback when kind absent from kindIndex", func() {
		It("should resolve via RESTMapper when kindIndex misses the kind", func() {
			scheme := buildTestScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme,
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "fallback-cm", Namespace: "default"},
					Data:       map[string]string{"key": "value"},
				},
			)
			mapper := buildTestMapper()
			trimmedIndex := map[string]schema.GroupKind{
				"pod": {Kind: "Pod"},
			}
			resolver := k8s.NewDynamicResolver(dynClient, mapper, trimmedIndex)

			result, err := resolver.Get(context.Background(), "ConfigMap", "fallback-cm", "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("fallback-cm"))
		})
	})

	Describe("UT-KA-433-543: resolver returns error for unknown kind", func() {
		It("should return a descriptive error for an unrecognized kind", func() {
			result, err := reg.Execute(context.Background(), "kubectl_describe",
				json.RawMessage(`{"kind":"FooBarBaz","name":"test","namespace":"default"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported kind"))
			Expect(result).To(BeEmpty())
		})
	})

	// --- C2: findResource keyword filtering ---

	Describe("UT-KA-433-545: kubectl_find_resource keyword filters to matching items only", func() {
		It("should return only items matching the keyword, excluding non-matching items", func() {
			result, err := reg.Execute(context.Background(), "kubectl_find_resource",
				json.RawMessage(`{"kind":"Job","keyword":"migration"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("migration-job"), "matching item must be present")
			Expect(result).NotTo(ContainSubstring("backup-job"), "non-matching item must be excluded")
		})

		It("should return empty array when keyword matches nothing", func() {
			result, err := reg.Execute(context.Background(), "kubectl_find_resource",
				json.RawMessage(`{"kind":"Job","keyword":"zzz-nonexistent-zzz"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("[]"))
		})
	})

	// --- #752: kubectl_get_by_name_in_cluster tool ---

	Describe("UT-KA-752-007: kubectl_get_by_name_in_cluster returns single matching resource", func() {
		It("should return a single Pod by name across all namespaces", func() {
			tool := findToolByName(reg, "kubectl_get_by_name_in_cluster")
			Expect(tool).NotTo(BeNil(), "kubectl_get_by_name_in_cluster must be registered")

			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Pod","name":"api-pod"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("api-pod"),
				"should return the matching Pod")
		})
	})

	Describe("UT-KA-752-008: kubectl_get_by_name_in_cluster returns not-found for nonexistent resource", func() {
		It("should return an error or empty result when resource does not exist", func() {
			tool := findToolByName(reg, "kubectl_get_by_name_in_cluster")
			Expect(tool).NotTo(BeNil(), "kubectl_get_by_name_in_cluster must be registered")

			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Pod","name":"nonexistent-pod-xyz"}`))
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("not found"),
					"error should indicate resource not found")
			} else {
				Expect(result).To(ContainSubstring("not found"),
					"result should indicate resource not found")
			}
		})
	})

	Describe("UT-KA-752-009: kubectl_get_by_name_in_cluster registered in AllToolNames", func() {
		It("should be included in AllToolNames", func() {
			found := false
			for _, name := range k8s.AllToolNames {
				if name == "kubectl_get_by_name_in_cluster" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(),
				"kubectl_get_by_name_in_cluster must be in AllToolNames")
		})

		It("should be discoverable via the registry", func() {
			tool := findToolByName(reg, "kubectl_get_by_name_in_cluster")
			Expect(tool).NotTo(BeNil(),
				"tool must be registered in the registry")
		})
	})
})
