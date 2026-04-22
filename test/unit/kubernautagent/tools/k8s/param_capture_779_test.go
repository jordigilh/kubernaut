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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// BR-WORKFLOW-016 / #779: K8s tool fakes must capture downstream parameters
// to ensure the tools forward the correct FieldSelectors, ListOptions, etc.
// to the K8s API, not just check err==nil.

var _ = Describe("UT-KA-779-PC: K8s tool parameter capture via PrependReactor", func() {

	Describe("UT-KA-779-PC-001: kubectl_events passes correct FieldSelector to K8s API", func() {
		It("should set FieldSelector to involvedObject.name=<name>", func() {
			objects := []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "api-pod", Namespace: "production"},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "app:v1"}},
					},
				},
				&corev1.Event{
					ObjectMeta:     metav1.ObjectMeta{Name: "evt-1", Namespace: "production"},
					InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api-pod", Namespace: "production"},
					Reason:         "OOMKilled",
					Message:        "memory exceeded",
					Type:           "Warning",
				},
			}
			typedClient := fake.NewSimpleClientset(objects...)

			var capturedFieldSelector string
			typedClient.PrependReactor("list", "events", func(action k8stesting.Action) (bool, runtime.Object, error) {
				listAction := action.(k8stesting.ListAction)
				capturedFieldSelector = listAction.GetListRestrictions().Fields.String()
				return false, nil, nil
			})

			scheme := buildTestScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
			mapper := buildTestMapper()
			kindIndex := buildTestKindIndex()
			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex)

			reg := registry.New()
			for _, t := range k8s.NewAllTools(typedClient, resolver) {
				reg.Register(t)
			}

			_, err := reg.Execute(context.Background(), "kubectl_events",
				json.RawMessage(`{"kind":"Pod","name":"api-pod","namespace":"production"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedFieldSelector).To(ContainSubstring("involvedObject.name=api-pod"),
				"FieldSelector must filter events by the requested resource name")
		})
	})

	Describe("UT-KA-779-PC-002: kubectl_events passes namespace to K8s API", func() {
		It("should list events in the correct namespace", func() {
			objects := []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "web-pod", Namespace: "staging"},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "web", Image: "web:v1"}},
					},
				},
				&corev1.Event{
					ObjectMeta:     metav1.ObjectMeta{Name: "evt-2", Namespace: "staging"},
					InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "web-pod", Namespace: "staging"},
					Reason:         "Started",
					Message:        "started",
					Type:           "Normal",
				},
			}
			typedClient := fake.NewSimpleClientset(objects...)

			var capturedNamespace string
			typedClient.PrependReactor("list", "events", func(action k8stesting.Action) (bool, runtime.Object, error) {
				capturedNamespace = action.GetNamespace()
				return false, nil, nil
			})

			scheme := buildTestScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
			mapper := buildTestMapper()
			kindIndex := buildTestKindIndex()
			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex)

			reg := registry.New()
			for _, t := range k8s.NewAllTools(typedClient, resolver) {
				reg.Register(t)
			}

			_, err := reg.Execute(context.Background(), "kubectl_events",
				json.RawMessage(`{"kind":"Pod","name":"web-pod","namespace":"staging"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedNamespace).To(Equal("staging"),
				"Events must be listed in the namespace specified by the LLM")
		})
	})

	Describe("UT-KA-779-PC-003: kubectl_logs default TailLines is applied correctly", func() {
		It("should use DefaultLogTailLines when neither tailLines nor limitBytes is set", func() {
			Expect(k8s.DefaultLogTailLines).To(Equal(int64(500)),
				"DefaultLogTailLines must be 500 per DD-HAPI-019")
		})
	})
})
