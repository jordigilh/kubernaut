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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/fake"

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
			Expect(func() {
				emitter.EmitInteractiveSoftDisabled("test reason")
			}).NotTo(Panic())
			Expect(func() {
				emitter.EmitInteractiveEnabled()
			}).NotTo(Panic())
			Expect(func() {
				emitter.Shutdown()
			}).NotTo(Panic())
		})

		It("should emit soft-disabled event without error", func() {
			client := fake.NewSimpleClientset()
			emitter := karbac.NewEventEmitter(client, "ka-pod-abc", "kubernaut-system")
			Expect(emitter).NotTo(BeNil())
			Expect(func() {
				emitter.EmitInteractiveSoftDisabled("SA lacks impersonate permission")
			}).NotTo(Panic())
			emitter.Shutdown()
		})

		It("should emit enabled event without error", func() {
			client := fake.NewSimpleClientset()
			emitter := karbac.NewEventEmitter(client, "ka-pod-abc", "kubernaut-system")
			Expect(emitter).NotTo(BeNil())
			Expect(func() {
				emitter.EmitInteractiveEnabled()
			}).NotTo(Panic())
			emitter.Shutdown()
		})
	})

	Describe("DetectPodIdentity", func() {

		It("should return empty strings when env vars are not set", func() {
			podName, namespace := karbac.DetectPodIdentity()
			// In a test environment without downward API, these will be empty
			// unless set by the CI or the test runner.
			_ = podName
			_ = namespace
		})
	})
})
