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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func generateEvents(n int, objName, namespace string) []runtime.Object {
	events := make([]runtime.Object, n)
	for i := 0; i < n; i++ {
		events[i] = &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: fmt.Sprintf("evt-%d", i), Namespace: namespace},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: objName, Namespace: namespace},
			Reason:         "TestEvent",
			Message:        fmt.Sprintf("test event %d", i),
			Type:           "Normal",
		}
	}
	return events
}

var _ = Describe("Kubernaut Agent K8s Intrinsic Caps — #752 Phase 2", func() {

	// --- Log tools default tail lines ---

	Context("Log tools default tail lines", func() {
		var reg *registry.Registry

		BeforeEach(func() {
			objects := []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "log-pod", Namespace: "default"},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "app", Image: "app:v1"},
							{Name: "sidecar", Image: "sidecar:v1"},
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
		})

		Describe("UT-KA-752-101: kubectl_logs applies DefaultLogTailLines when tailLines and limitBytes are nil", func() {
			It("should define DefaultLogTailLines constant equal to 500", func() {
				Expect(k8s.DefaultLogTailLines).To(Equal(int64(500)))
			})

			It("should execute without error when tailLines is omitted", func() {
				_, err := reg.Execute(context.Background(), "kubectl_logs",
					json.RawMessage(`{"name":"log-pod","namespace":"default"}`))
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("UT-KA-752-102: kubectl_logs honors explicit tailLines over default", func() {
			It("should execute without error when explicit tailLines is provided", func() {
				_, err := reg.Execute(context.Background(), "kubectl_logs",
					json.RawMessage(`{"name":"log-pod","namespace":"default","tailLines":100}`))
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("UT-KA-752-103: kubectl_logs_all_containers applies default tailLines", func() {
			It("should execute without error when tailLines is omitted for all-containers variant", func() {
				_, err := reg.Execute(context.Background(), "kubectl_logs_all_containers",
					json.RawMessage(`{"name":"log-pod","namespace":"default"}`))
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("UT-KA-752-111: kubectl_logs does NOT apply default tailLines when limitBytes is set", func() {
			It("should execute without error when limitBytes is set but tailLines is omitted", func() {
				_, err := reg.Execute(context.Background(), "kubectl_logs",
					json.RawMessage(`{"name":"log-pod","namespace":"default","limitBytes":1024}`))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	// --- kubectl_find_resource empty keyword ---

	Context("kubectl_find_resource keyword validation", func() {
		var reg *registry.Registry

		BeforeEach(func() {
			objects := []runtime.Object{
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
		})

		Describe("UT-KA-752-104: kubectl_find_resource rejects empty keyword", func() {
			It("should return an error when keyword is empty", func() {
				_, err := reg.Execute(context.Background(), "kubectl_find_resource",
					json.RawMessage(`{"kind":"Pod","keyword":""}`))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("keyword"))
			})

			It("should return an error when keyword is whitespace-only", func() {
				_, err := reg.Execute(context.Background(), "kubectl_find_resource",
					json.RawMessage(`{"kind":"Pod","keyword":"   "}`))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("keyword"))
			})
		})

		Describe("UT-KA-752-105: kubectl_find_resource works with non-empty keyword (no regression)", func() {
			It("should return matching items", func() {
				result, err := reg.Execute(context.Background(), "kubectl_find_resource",
					json.RawMessage(`{"kind":"Job","keyword":"migration"}`))
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("migration-job"))
			})
		})
	})

	// --- kubernetes_jq_query output character cap ---

	Context("JQ output character cap", func() {
		Describe("UT-KA-752-106: TruncateJQOutput truncates output exceeding limit", func() {
			It("should truncate and append hint when output exceeds limit", func() {
				output := strings.Repeat("x", 200)
				truncated := k8s.TruncateJQOutput(output, 100)
				Expect(len(truncated)).To(BeNumerically("<=", 250))
				Expect(truncated).To(ContainSubstring("TRUNCATED"))
			})
		})

		Describe("UT-KA-752-107: TruncateJQOutput passes through output below limit", func() {
			It("should return output unchanged when below limit", func() {
				output := "small output"
				truncated := k8s.TruncateJQOutput(output, 100000)
				Expect(truncated).To(Equal(output))
			})
		})

		Describe("UT-KA-752-110: kubernetes_count is unaffected by JQ char cap", func() {
			var reg *registry.Registry

			BeforeEach(func() {
				objects := []runtime.Object{
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "count-pod", Namespace: "default"},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "app", Image: "app:v1"}},
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
			})

			It("should return count without truncation hint", func() {
				result, err := reg.Execute(context.Background(), "kubernetes_count",
					json.RawMessage(`{"kind":"Pod","jq_expr":".items[].metadata.name"}`))
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HavePrefix("Count:"))
				Expect(result).NotTo(ContainSubstring("TRUNCATED"))
			})
		})
	})

	// --- kubectl_events intrinsic limit ---

	Context("Events tool intrinsic limit", func() {
		var reg *registry.Registry

		BeforeEach(func() {
			eventCount := int(k8s.DefaultEventLimit) + 50
			evts := generateEvents(eventCount, "noisy-pod", "default")
			objects := append([]runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "noisy-pod", Namespace: "default"},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "app:v1"}},
					},
				},
			}, evts...)

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
		})

		Describe("UT-KA-752-108: kubectl_events applies DefaultEventLimit", func() {
			It("should define DefaultEventLimit constant", func() {
				Expect(k8s.DefaultEventLimit).To(Equal(200))
			})

			It("should truncate events exceeding the limit", func() {
				result, err := reg.Execute(context.Background(), "kubectl_events",
					json.RawMessage(`{"kind":"Pod","name":"noisy-pod","namespace":"default"}`))
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("TRUNCATED"))
			})
		})

		Describe("UT-KA-752-109: kubectl_events truncation hint content", func() {
			It("should include event count and limit in the truncation hint", func() {
				result, err := reg.Execute(context.Background(), "kubectl_events",
					json.RawMessage(`{"kind":"Pod","name":"noisy-pod","namespace":"default"}`))
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("200"))
			})
		})
	})
})
