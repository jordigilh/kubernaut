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

package tools_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var isGVR = schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "investigationsessions"}

func newUnstructuredIS(ns, name, rrName, phase string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "InvestigationSession",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"remediationRequestRef": map[string]interface{}{
					"name":      rrName,
					"namespace": ns,
				},
				"a2aTaskID": name,
				"userIdentity": map[string]interface{}{
					"username": "admin",
				},
				"joinMode": "takeover",
			},
		},
	}
	if phase != "" {
		obj.Object["status"] = map[string]interface{}{
			"phase": phase,
		}
	}
	return obj
}

func newISClient(objects ...*unstructured.Unstructured) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			isGVR: "InvestigationSessionList",
		})
	for _, obj := range objects {
		ns := obj.GetNamespace()
		_, _ = client.Resource(isGVR).Namespace(ns).Create(context.Background(), obj, metav1.CreateOptions{})
	}
	return client
}

var _ = Describe("AwaitISPhaseActive — BR-INTERACTIVE-010 IS phase polling", func() {

	Describe("UT-AF-IS-POLL-001: returns true immediately when IS is already Active", func() {
		It("should detect Active phase without delay", func() {
			is := newUnstructuredIS("kubernaut-system", "isess-001", "rr-poll-001", "Active")
			client := newISClient(is)

			start := time.Now()
			result := tools.AwaitISPhaseActive(context.Background(), client, "kubernaut-system", "rr-poll-001")
			elapsed := time.Since(start)

			Expect(result).To(BeTrue())
			Expect(elapsed).To(BeNumerically("<", 2*time.Second))
		})
	})

	Describe("UT-AF-IS-POLL-002: returns false on timeout when phase stays empty", func() {
		It("should timeout when IS exists but phase is never set to Active", func() {
			is := newUnstructuredIS("kubernaut-system", "isess-002", "rr-poll-002", "")
			client := newISClient(is)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			start := time.Now()
			result := tools.AwaitISPhaseActive(ctx, client, "kubernaut-system", "rr-poll-002")
			elapsed := time.Since(start)

			Expect(result).To(BeFalse())
			Expect(elapsed).To(BeNumerically(">=", 1500*time.Millisecond))
		})
	})

	Describe("UT-AF-IS-POLL-003: returns false when no IS CRD exists", func() {
		It("should timeout gracefully when no IS exists for the given RR", func() {
			client := newISClient()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			result := tools.AwaitISPhaseActive(ctx, client, "kubernaut-system", "rr-missing")
			Expect(result).To(BeFalse())
		})
	})

	Describe("UT-AF-IS-POLL-004: returns false with nil client", func() {
		It("should return false immediately when client is nil", func() {
			result := tools.AwaitISPhaseActive(context.Background(), nil, "kubernaut-system", "rr-nil")
			Expect(result).To(BeFalse())
		})
	})

	Describe("UT-AF-IS-POLL-005: returns false with empty namespace", func() {
		It("should return false immediately when namespace is empty", func() {
			client := newISClient()
			result := tools.AwaitISPhaseActive(context.Background(), client, "", "rr-empty-ns")
			Expect(result).To(BeFalse())
		})
	})

	Describe("UT-AF-IS-POLL-006: returns false with empty RR name", func() {
		It("should return false immediately when RR name is empty", func() {
			client := newISClient()
			result := tools.AwaitISPhaseActive(context.Background(), client, "kubernaut-system", "")
			Expect(result).To(BeFalse())
		})
	})

	Describe("UT-AF-IS-POLL-007: ignores IS CRDs for different RR", func() {
		It("should not match an Active IS belonging to a different RR", func() {
			is := newUnstructuredIS("kubernaut-system", "isess-other", "rr-other", "Active")
			client := newISClient(is)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			result := tools.AwaitISPhaseActive(ctx, client, "kubernaut-system", "rr-mine")
			Expect(result).To(BeFalse())
		})
	})

	Describe("UT-AF-IS-POLL-008: ignores terminal phase IS CRDs", func() {
		It("should not match a Completed IS for the same RR", func() {
			is := newUnstructuredIS("kubernaut-system", "isess-done", "rr-done", "Completed")
			client := newISClient(is)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			result := tools.AwaitISPhaseActive(ctx, client, "kubernaut-system", "rr-done")
			Expect(result).To(BeFalse())
		})
	})

	Describe("UT-AF-IS-POLL-009: detects Active phase set asynchronously", func() {
		It("should detect when IS phase transitions to Active during polling", func() {
			is := newUnstructuredIS("kubernaut-system", "isess-async", "rr-async", "")
			client := newISClient(is)

			// Simulate AA setting the phase after 1 second
			go func() {
				time.Sleep(1 * time.Second)
				updated := newUnstructuredIS("kubernaut-system", "isess-async", "rr-async", "Active")
				_, _ = client.Resource(isGVR).Namespace("kubernaut-system").Update(
					context.Background(), updated, metav1.UpdateOptions{})
			}()

			start := time.Now()
			result := tools.AwaitISPhaseActive(context.Background(), client, "kubernaut-system", "rr-async")
			elapsed := time.Since(start)

			Expect(result).To(BeTrue())
			Expect(elapsed).To(BeNumerically(">=", 800*time.Millisecond))
			Expect(elapsed).To(BeNumerically("<", 5*time.Second))
		})
	})

	Describe("UT-AF-IS-POLL-010: respects parent context cancellation", func() {
		It("should return false when parent context is cancelled", func() {
			is := newUnstructuredIS("kubernaut-system", "isess-cancel", "rr-cancel", "")
			client := newISClient(is)

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			start := time.Now()
			result := tools.AwaitISPhaseActive(ctx, client, "kubernaut-system", "rr-cancel")
			elapsed := time.Since(start)

			Expect(result).To(BeFalse())
			Expect(elapsed).To(BeNumerically("<", 3*time.Second))
		})
	})
})
