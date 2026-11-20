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

package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
)

// #2658 E11c: notifications keep the documented two-part "Kind/Name" shape
// (api/notification/v1alpha1/notificationrequest_types.go) and append the API
// group, so a cross-group pair is distinguishable in the notification channel
// (SOC2 CC7.2).
var _ = Describe("Notification target identity (#2658) [SOC2 CC7.2]", func() {
	It("keeps the documented two-part shape for a grouped target", func() {
		ctx := controller.BuildTimeoutContextForTest("rr-1", "Executing", "10m", remediationv1.ResourceIdentifier{
			Kind: "Deployment", Name: "web", Namespace: "prod", APIVersion: "apps/v1",
		})
		Expect(ctx.Target.TargetResource).To(Equal("Deployment/web"),
			"the notification contract's own shape carries no identity annotation")
	})

	It("omits the suffix for a core resource", func() {
		ctx := controller.BuildTimeoutContextForTest("rr-1", "Executing", "10m", remediationv1.ResourceIdentifier{
			Kind: "Pod", Name: "api", Namespace: "prod", APIVersion: "v1",
		})
		Expect(ctx.Target.TargetResource).To(Equal("Pod/api"))
	})

	It("keeps the two-part shape when the group cannot be established", func() {
		ctx := controller.BuildTimeoutContextForTest("rr-1", "Executing", "10m", remediationv1.ResourceIdentifier{
			Kind: "Deployment", Name: "web", Namespace: "prod",
		})
		Expect(ctx.Target.TargetResource).To(Equal("Deployment/web"),
			"an unknown group must not invent a suffix the notification consumers cannot parse")
	})
})
