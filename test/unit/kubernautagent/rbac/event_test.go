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

package rbac_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
)

var _ = Describe("UT-KA-891-002: K8s Event emission for interactive mode (#891)", func() {

	Describe("NewEventEmitter", func() {

		It("should return nil when pod identity is empty", func() {
			client := fake.NewSimpleClientset()
			emitter := karbac.NewEventEmitter(client, "", "")
			Expect(emitter).To(BeNil())
		})

		It("should return nil when only podName is empty", func() {
			client := fake.NewSimpleClientset()
			emitter := karbac.NewEventEmitter(client, "", "kubernaut-system")
			Expect(emitter).To(BeNil())
		})

		It("should return a non-nil emitter when pod identity is available", func() {
			client := fake.NewSimpleClientset()
			emitter := karbac.NewEventEmitter(client, "ka-pod-abc", "kubernaut-system")
			Expect(emitter).NotTo(BeNil())
			emitter.Shutdown()
		})

		It("should not panic when emitting events on a nil emitter", func() {
			var emitter *karbac.EventEmitter
			Expect(func() { emitter.EmitInteractiveSoftDisabled("test reason") }).NotTo(Panic())
			Expect(func() { emitter.EmitInteractiveEnabled() }).NotTo(Panic())
			Expect(func() { emitter.Shutdown() }).NotTo(Panic())
		})

		It("should look up pod UID when the pod exists", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ka-pod-abc",
					Namespace: "kubernaut-system",
					UID:       types.UID("test-uid-12345"),
				},
			}
			client := fake.NewSimpleClientset(pod)
			emitter := karbac.NewEventEmitter(client, "ka-pod-abc", "kubernaut-system")
			Expect(emitter).NotTo(BeNil())

			podGetActions := filterActions(client.Actions(), "get", "pods")
			Expect(podGetActions).NotTo(BeEmpty(), "should attempt Pod GET to resolve UID")
			emitter.Shutdown()
		})
	})

	Describe("EmitInteractiveSoftDisabled", func() {

		It("should record a Warning event with InteractiveSoftDisabled reason", func() {
			client := fake.NewSimpleClientset()
			emitter := karbac.NewEventEmitter(client, "ka-pod-abc", "kubernaut-system")
			Expect(emitter).NotTo(BeNil())

			emitter.EmitInteractiveSoftDisabled("SA lacks impersonate permission on groups")

			Eventually(func() []k8stesting.Action {
				return filterActions(client.Actions(), "create", "events")
			}, 2*time.Second, 100*time.Millisecond).ShouldNot(BeEmpty(),
				"expected a create events action from the recorder")

			eventActions := filterActions(client.Actions(), "create", "events")
			Expect(eventActions).NotTo(BeEmpty())
			event := eventActions[0].(k8stesting.CreateAction).GetObject().(*corev1.Event)
			Expect(event.Reason).To(Equal(karbac.ReasonInteractiveSoftDisabled))
			Expect(event.Type).To(Equal(corev1.EventTypeWarning))
			Expect(event.Message).To(ContainSubstring("impersonate"))
			Expect(event.InvolvedObject.Name).To(Equal("ka-pod-abc"))
			Expect(event.InvolvedObject.Namespace).To(Equal("kubernaut-system"))

			emitter.Shutdown()
		})
	})

	Describe("EmitInteractiveEnabled", func() {

		It("should record a Normal event with InteractiveEnabled reason", func() {
			client := fake.NewSimpleClientset()
			emitter := karbac.NewEventEmitter(client, "ka-pod-abc", "kubernaut-system")
			Expect(emitter).NotTo(BeNil())

			emitter.EmitInteractiveEnabled()

			Eventually(func() []k8stesting.Action {
				return filterActions(client.Actions(), "create", "events")
			}, 2*time.Second, 100*time.Millisecond).ShouldNot(BeEmpty(),
				"expected a create events action from the recorder")

			eventActions := filterActions(client.Actions(), "create", "events")
			Expect(eventActions).NotTo(BeEmpty())
			event := eventActions[0].(k8stesting.CreateAction).GetObject().(*corev1.Event)
			Expect(event.Reason).To(Equal(karbac.ReasonInteractiveEnabled))
			Expect(event.Type).To(Equal(corev1.EventTypeNormal))
			Expect(event.Message).To(ContainSubstring("impersonate"))
			Expect(event.InvolvedObject.Name).To(Equal("ka-pod-abc"))

			emitter.Shutdown()
		})
	})
})

func filterActions(actions []k8stesting.Action, verb, resource string) []k8stesting.Action {
	var filtered []k8stesting.Action
	for _, a := range actions {
		if a.GetVerb() == verb && a.GetResource().Resource == resource {
			filtered = append(filtered, a)
		}
	}
	return filtered
}
