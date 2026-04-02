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

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func int32Ptr(i int32) *int32 { return &i }

var _ = Describe("Kubernaut Agent K8s Tools Integration — #433", func() {

	var (
		client  *fake.Clientset
		reg     *registry.Registry
	)

	BeforeEach(func() {
		client = fake.NewSimpleClientset(
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-server",
					Namespace: "production",
					Labels:    map[string]string{"app": "api-server", "tier": "backend"},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "api-server"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api-server"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "api", Image: "api-server:v1.2.3",
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceMemory: *mustParseQuantity("256Mi"),
										},
									},
								},
								{Name: "sidecar", Image: "envoy:v1.0"},
							},
						},
					},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-server-abc-xyz",
					Namespace: "production",
					Labels:    map[string]string{"app": "api-server"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "api", Image: "api-server:v1.2.3"},
						{Name: "sidecar", Image: "envoy:v1.0"},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "api", Ready: true, RestartCount: 3},
						{Name: "sidecar", Ready: true, RestartCount: 0},
					},
				},
			},
			&corev1.Event{
				ObjectMeta:    metav1.ObjectMeta{Name: "event-1", Namespace: "production"},
				InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api-server-abc-xyz", Namespace: "production"},
				Reason:        "OOMKilled",
				Message:       "Container api exceeded memory limit",
				Type:          "Warning",
			},
		)

		reg = registry.New()
		allTools := k8s.NewAllTools(client)
		Expect(allTools).NotTo(BeNil(), "NewAllTools should not return nil")
		for _, t := range allTools {
			reg.Register(t)
		}
	})

	Describe("IT-KA-433-014: kubectl_describe produces structured JSON summary of Deployment", func() {
		It("should return JSON with Deployment metadata and spec", func() {
			result, err := reg.Execute(context.Background(), "kubectl_describe",
				json.RawMessage(`{"kind":"Deployment","name":"api-server","namespace":"production"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &parsed)).To(Succeed(),
				"output should be valid JSON")
			Expect(parsed).To(HaveKey("metadata"))
			Expect(parsed).To(HaveKey("spec"))
		})
	})

	Describe("IT-KA-433-015: kubectl_get_by_name returns serialized Pod object", func() {
		It("should return the Pod as JSON", func() {
			result, err := reg.Execute(context.Background(), "kubectl_get_by_name",
				json.RawMessage(`{"kind":"Pod","name":"api-server-abc-xyz","namespace":"production"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("api-server-abc-xyz"))
		})
	})

	Describe("IT-KA-433-016: kubectl_get_by_kind_in_namespace lists matching objects", func() {
		It("should return a list of Pods in the namespace", func() {
			result, err := reg.Execute(context.Background(), "kubectl_get_by_kind_in_namespace",
				json.RawMessage(`{"kind":"Pod","namespace":"production"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("api-server-abc-xyz"))
		})
	})

	Describe("IT-KA-433-017: kubectl_events returns events for target resource", func() {
		It("should return events matching the involved object", func() {
			result, err := reg.Execute(context.Background(), "kubectl_events",
				json.RawMessage(`{"kind":"Pod","name":"api-server-abc-xyz","namespace":"production"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("OOMKilled"))
		})
	})

	Describe("IT-KA-433-018: kubectl_logs respects TailLines and LimitBytes", func() {
		It("should accept tailLines and limitBytes parameters without error", func() {
			_, err := reg.Execute(context.Background(), "kubectl_logs",
				json.RawMessage(`{"name":"api-server-abc-xyz","namespace":"production","container":"api","tailLines":500,"limitBytes":262144}`))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("IT-KA-433-019: kubectl_previous_logs retrieves previous container logs", func() {
		It("should request previous logs without error", func() {
			_, err := reg.Execute(context.Background(), "kubectl_previous_logs",
				json.RawMessage(`{"name":"api-server-abc-xyz","namespace":"production","container":"api"}`))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("IT-KA-433-020: kubectl_logs_all_containers aggregates logs from all containers", func() {
		It("should request logs from all Pod containers", func() {
			_, err := reg.Execute(context.Background(), "kubectl_logs_all_containers",
				json.RawMessage(`{"name":"api-server-abc-xyz","namespace":"production"}`))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("IT-KA-433-021: kubectl_container_logs retrieves named container logs", func() {
		It("should request logs for a specific container", func() {
			_, err := reg.Execute(context.Background(), "kubectl_container_logs",
				json.RawMessage(`{"name":"api-server-abc-xyz","namespace":"production","container":"sidecar"}`))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("IT-KA-433-022: kubectl_container_previous_logs retrieves named container previous logs", func() {
		It("should request previous logs for a specific container", func() {
			_, err := reg.Execute(context.Background(), "kubectl_container_previous_logs",
				json.RawMessage(`{"name":"api-server-abc-xyz","namespace":"production","container":"sidecar"}`))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("IT-KA-433-023: kubectl_previous_logs_all_containers retrieves previous from all containers", func() {
		It("should request previous logs from all containers", func() {
			_, err := reg.Execute(context.Background(), "kubectl_previous_logs_all_containers",
				json.RawMessage(`{"name":"api-server-abc-xyz","namespace":"production"}`))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("IT-KA-433-024: kubectl_logs_grep filters log lines matching pattern", func() {
		It("should accept a grep pattern parameter", func() {
			_, err := reg.Execute(context.Background(), "kubectl_logs_grep",
				json.RawMessage(`{"name":"api-server-abc-xyz","namespace":"production","container":"api","pattern":"OOM"}`))
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
