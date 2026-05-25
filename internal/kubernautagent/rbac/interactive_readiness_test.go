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

	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
)

var _ = Describe("InteractiveReadiness (#1288)", func() {
	It("should report enabled when set", func() {
		status := karbac.NewInteractiveReadiness()
		status.SetEnabled()
		Expect(status.IsEnabled()).To(BeTrue())
		Expect(status.StatusString()).To(Equal("enabled"))
	})

	It("should report soft-disabled with reason", func() {
		status := karbac.NewInteractiveReadiness()
		status.SetSoftDisabled("MCP mount failed")
		Expect(status.IsEnabled()).To(BeFalse())
		Expect(status.StatusString()).To(Equal("soft_disabled"))
		Expect(status.Reason()).To(ContainSubstring("MCP mount failed"))
	})

	It("should report not_configured by default", func() {
		status := karbac.NewInteractiveReadiness()
		Expect(status.IsEnabled()).To(BeFalse())
		Expect(status.StatusString()).To(Equal("not_configured"))
	})
})

var _ = Describe("DetectPodIdentity", func() {
	It("should return values from POD_NAME and POD_NAMESPACE env vars", func() {
		GinkgoT().Setenv("POD_NAME", "ka-test-pod-xyz")
		GinkgoT().Setenv("POD_NAMESPACE", "kubernaut-test-ns")

		podName, namespace := karbac.DetectPodIdentity()
		Expect(podName).To(Equal("ka-test-pod-xyz"))
		Expect(namespace).To(Equal("kubernaut-test-ns"))
	})

	It("should return empty strings when env vars are not set", func() {
		podName, namespace := karbac.DetectPodIdentity()
		Expect(podName).To(BeEmpty())
		Expect(namespace).To(BeEmpty())
	})
})
