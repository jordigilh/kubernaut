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

package logs_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/logs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("Kubernaut Agent fetch_pod_logs — #433 Phase 3", func() {

	multiContainerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-abc-123", Namespace: "production"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "web:v1"},
				{Name: "sidecar", Image: "envoy:v1"},
			},
		},
	}

	singleContainerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "worker", Image: "worker:v1"},
			},
		},
	}

	Describe("UT-KA-433-610: fetch_pod_logs basic execution", func() {
		It("should execute without error for an existing pod", func() {
			client := fake.NewSimpleClientset(multiContainerPod)
			tool := logs.NewFetchPodLogsTool(client)
			Expect(tool).NotTo(BeNil())
			Expect(tool.Name()).To(Equal("fetch_pod_logs"))
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"pod_name":"web-abc-123","namespace":"production"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("--- fetch_pod_logs metadata ---"))
			Expect(result).To(ContainSubstring("pod: production/web-abc-123"))
		})
	})

	Describe("UT-KA-433-611: fetch_pod_logs schema", func() {
		It("should have pod_name and namespace as required parameters", func() {
			client := fake.NewSimpleClientset(singleContainerPod)
			tool := logs.NewFetchPodLogsTool(client)
			schema := tool.Parameters()
			Expect(schema).NotTo(BeNil())
			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())
			required, ok := parsed["required"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(required).To(ContainElement("pod_name"))
			Expect(required).To(ContainElement("namespace"))
		})

		It("should not include end_time in the schema", func() {
			client := fake.NewSimpleClientset(singleContainerPod)
			tool := logs.NewFetchPodLogsTool(client)
			schema := string(tool.Parameters())
			Expect(schema).NotTo(ContainSubstring("end_time"),
				"end_time was removed because it was not implemented")
		})
	})

	Describe("UT-KA-433-612: fetch_pod_logs accepts filter parameters without error", func() {
		It("should accept filter and exclude_filter parameters", func() {
			client := fake.NewSimpleClientset(singleContainerPod)
			tool := logs.NewFetchPodLogsTool(client)
			_, err := tool.Execute(context.Background(),
				json.RawMessage(`{"pod_name":"worker-pod","namespace":"default","filter":"error","exclude_filter":"debug"}`))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-433-613: fetch_pod_logs accepts time range without error", func() {
		It("should accept start_time parameter", func() {
			client := fake.NewSimpleClientset(singleContainerPod)
			tool := logs.NewFetchPodLogsTool(client)
			_, err := tool.Execute(context.Background(),
				json.RawMessage(`{"pod_name":"worker-pod","namespace":"default","start_time":"-3600"}`))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-433-614: fetch_pod_logs accepts limit parameter", func() {
		It("should accept limit parameter", func() {
			client := fake.NewSimpleClientset(singleContainerPod)
			tool := logs.NewFetchPodLogsTool(client)
			_, err := tool.Execute(context.Background(),
				json.RawMessage(`{"pod_name":"worker-pod","namespace":"default","limit":50}`))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-433-615: fetch_pod_logs metadata footer", func() {
		It("should include metadata footer with pod info", func() {
			client := fake.NewSimpleClientset(singleContainerPod)
			tool := logs.NewFetchPodLogsTool(client)
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"pod_name":"worker-pod","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("--- fetch_pod_logs metadata ---"))
			Expect(result).To(ContainSubstring("pod: default/worker-pod"))
			Expect(result).To(ContainSubstring("lines:"))
		})

		It("should include filter info in metadata when filter is used", func() {
			client := fake.NewSimpleClientset(singleContainerPod)
			tool := logs.NewFetchPodLogsTool(client)
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"pod_name":"worker-pod","namespace":"default","filter":"ERROR","exclude_filter":"noise"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("filter: ERROR"))
			Expect(result).To(ContainSubstring("exclude_filter: noise"))
		})
	})

	Describe("UT-KA-433-616: fetch_pod_logs returns error for missing pod", func() {
		It("should return an error when the pod does not exist", func() {
			client := fake.NewSimpleClientset()
			tool := logs.NewFetchPodLogsTool(client)
			_, err := tool.Execute(context.Background(),
				json.RawMessage(`{"pod_name":"nonexistent","namespace":"default"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent"))
		})
	})

	Describe("UT-KA-433-617: fetch_pod_logs filter functions via ApplyFilters", func() {
		It("should include-filter lines correctly", func() {
			input := []string{
				"2026-03-01T10:00:00Z INFO starting up",
				"2026-03-01T10:00:01Z ERROR connection refused",
				"2026-03-01T10:00:02Z INFO recovered",
				"2026-03-01T10:00:03Z ERROR timeout",
			}
			result := logs.ApplyFilters(input, "ERROR", "", 100)
			Expect(result).To(HaveLen(2))
			Expect(result[0]).To(ContainSubstring("connection refused"))
			Expect(result[1]).To(ContainSubstring("timeout"))
		})

		It("should exclude-filter lines correctly", func() {
			input := []string{
				"2026-03-01T10:00:00Z DEBUG trace",
				"2026-03-01T10:00:01Z ERROR real problem",
				"2026-03-01T10:00:02Z DEBUG more noise",
			}
			result := logs.ApplyFilters(input, "", "DEBUG", 100)
			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(ContainSubstring("real problem"))
		})

		It("should apply both include and exclude filters", func() {
			input := []string{
				"2026-03-01T10:00:00Z ERROR debug-related error",
				"2026-03-01T10:00:01Z ERROR real error",
				"2026-03-01T10:00:02Z INFO normal",
			}
			result := logs.ApplyFilters(input, "ERROR", "debug", 100)
			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(ContainSubstring("real error"))
		})

		It("should truncate to limit (keeping tail)", func() {
			input := []string{"line-A", "line-B", "line-C", "line-D", "line-E"}
			result := logs.ApplyFilters(input, "", "", 3)
			Expect(result).To(HaveLen(3))
			Expect(result[0]).To(Equal("line-C"))
			Expect(result[1]).To(Equal("line-D"))
			Expect(result[2]).To(Equal("line-E"))
		})

		It("should handle case-insensitive filtering", func() {
			input := []string{"ERROR loud", "error quiet", "info normal"}
			result := logs.ApplyFilters(input, "error", "", 100)
			Expect(result).To(HaveLen(2))
		})
	})
})
