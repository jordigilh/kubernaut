/*
Copyright 2025 Jordi Gil.

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

package remediationorchestrator_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("NotificationCreator", func() {
	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
	})

	Describe("Constructor", func() {
		// Test #1: Constructor returns non-nil NotificationCreator
		It("should return non-nil NotificationCreator", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(fakeClient, scheme)
			Expect(nc).ToNot(BeNil())
		})
	})

	Describe("CreateApprovalNotification", func() {
		var (
			fakeClient *fake.ClientBuilder
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
		})

		Context("BR-ORCH-001: Approval Notification Creation", func() {
			// Test #2: Generates deterministic name
			It("should generate deterministic name nr-approval-{rr.Name}", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme)

				rr := testutil.NewRemediationRequest("test-rr", "default")
				ai := testutil.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("nr-approval-test-rr"))
			})
		})
	})
})

