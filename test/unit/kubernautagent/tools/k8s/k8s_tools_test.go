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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func toolByName(allTools []tools.Tool, name string) tools.Tool {
	for _, t := range allTools {
		if t.Name() == name {
			return t
		}
	}
	return nil
}

var _ = Describe("K8s Tools — #433 Phase 2", func() {
	var (
		client   *fake.Clientset
		allTools []tools.Tool
	)

	BeforeEach(func() {
		client = fake.NewSimpleClientset(
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "api-pod", Namespace: "default", Labels: map[string]string{"app": "api"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "app", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")},
						Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("256Mi")},
					}},
				}},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-pod", Namespace: "default", Labels: map[string]string{"app": "worker"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "main", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("64Mi")},
						Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")},
					}},
				}},
			},
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "api-deploy", Namespace: "default"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
				},
			},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "production"}},
		)
		allTools = k8s.NewAllTools(client)
	})

	Describe("UT-KA-433-550: AllToolNames contains 18 entries", func() {
		It("should have exactly 18 K8s tool names", func() {
			Expect(k8s.AllToolNames).To(HaveLen(18))
		})
	})

	Describe("UT-KA-433-501: kubectl_get_by_kind_in_cluster", func() {
		It("should list resources across all namespaces", func() {
			t := toolByName(allTools, "kubectl_get_by_kind_in_cluster")
			Expect(t).NotTo(BeNil())
			result, err := t.Execute(context.Background(), json.RawMessage(`{"kind":"Pod"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("api-pod"))
			Expect(result).To(ContainSubstring("worker-pod"))
		})
	})

	Describe("UT-KA-433-503: kubectl_find_resource", func() {
		It("should find resources by label selector", func() {
			t := toolByName(allTools, "kubectl_find_resource")
			Expect(t).NotTo(BeNil())
			result, err := t.Execute(context.Background(), json.RawMessage(`{"kind":"Pod","namespace":"default","label_selector":"app=api"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("api-pod"))
			Expect(result).NotTo(ContainSubstring("worker-pod"))
		})
	})

	Describe("UT-KA-433-505: kubectl_get_yaml", func() {
		It("should return resource as YAML", func() {
			t := toolByName(allTools, "kubectl_get_yaml")
			Expect(t).NotTo(BeNil())
			result, err := t.Execute(context.Background(), json.RawMessage(`{"kind":"Pod","name":"api-pod","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("name: api-pod"))
			Expect(result).NotTo(ContainSubstring(`"name"`))
		})
	})

	Describe("UT-KA-433-507: kubectl_get_memory_requests", func() {
		It("should return container memory requests/limits for pod", func() {
			t := toolByName(allTools, "kubectl_get_memory_requests")
			Expect(t).NotTo(BeNil())
			result, err := t.Execute(context.Background(), json.RawMessage(`{"name":"api-pod","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("128Mi"))
			Expect(result).To(ContainSubstring("256Mi"))
		})
	})

	Describe("UT-KA-433-508: kubectl_get_deployment_memory_requests", func() {
		It("should return memory for all pods in deployment", func() {
			t := toolByName(allTools, "kubectl_get_deployment_memory_requests")
			Expect(t).NotTo(BeNil())
			result, err := t.Execute(context.Background(), json.RawMessage(`{"name":"api-deploy","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("api-pod"))
			Expect(result).To(ContainSubstring("128Mi"))
		})
	})

	Describe("UT-KA-433-509: kubernetes_jq_query", func() {
		It("should apply jq expression to resource JSON", func() {
			t := toolByName(allTools, "kubernetes_jq_query")
			Expect(t).NotTo(BeNil())
			result, err := t.Execute(context.Background(), json.RawMessage(`{"kind":"Pod","namespace":"default","jq_expression":".items | length"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("2"))
		})
	})

	Describe("UT-KA-433-511: kubernetes_count", func() {
		It("should count resources matching kind in namespace", func() {
			t := toolByName(allTools, "kubernetes_count")
			Expect(t).NotTo(BeNil())
			result, err := t.Execute(context.Background(), json.RawMessage(`{"kind":"Pod","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("2"))
		})
	})

	Describe("UT-KA-433-512: kubernetes_count with jq_filter", func() {
		It("should apply post-filter before counting", func() {
			t := toolByName(allTools, "kubernetes_count")
			Expect(t).NotTo(BeNil())
			result, err := t.Execute(context.Background(), json.RawMessage(`{"kind":"Pod","namespace":"default","jq_filter":".metadata.name == \"api-pod\""}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("1"))
		})
	})

	Describe("UT-KA-433-502/504/506/510: JSON parameter schemas", func() {
		It("should have valid non-empty parameter schemas for all new tools", func() {
			for _, name := range []string{
				"kubectl_get_by_kind_in_cluster",
				"kubectl_find_resource",
				"kubectl_get_yaml",
				"kubernetes_jq_query",
				"kubernetes_count",
				"kubectl_get_memory_requests",
				"kubectl_get_deployment_memory_requests",
			} {
				t := toolByName(allTools, name)
				Expect(t).NotTo(BeNil(), "tool %s should exist", name)
				params := t.Parameters()
				Expect(params).NotTo(BeNil(), "%s Parameters() should not be nil", name)
				Expect(string(params)).NotTo(Equal("{}"), "%s should have non-empty schema", name)

				var schema map[string]interface{}
				Expect(json.Unmarshal(params, &schema)).To(Succeed(), "%s should have valid JSON schema", name)
				Expect(schema).To(HaveKey("type"), "%s schema should have type", name)
			}
		})
	})
})

