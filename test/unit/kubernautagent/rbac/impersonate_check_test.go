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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
)

var _ = Describe("UT-KA-891-001: Startup impersonate permission check (#891)", func() {

	Describe("CheckImpersonatePermission", func() {

		It("should return allowed=true when the SA has impersonate on both users and groups", func() {
			client := fake.NewSimpleClientset()
			client.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, &authorizationv1.SelfSubjectAccessReview{
					Status: authorizationv1.SubjectAccessReviewStatus{
						Allowed: true,
					},
				}, nil
			})

			result, err := karbac.CheckImpersonatePermission(context.Background(), client)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Allowed).To(BeTrue(), "SA with impersonate RBAC on users+groups should be allowed")
			Expect(result.Reason).To(BeEmpty())
		})

		It("should return allowed=false when the SA lacks impersonate on users", func() {
			client := fake.NewSimpleClientset()
			callCount := 0
			client.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				callCount++
				createAction := action.(k8stesting.CreateAction)
				ssar := createAction.GetObject().(*authorizationv1.SelfSubjectAccessReview)
				resource := ssar.Spec.ResourceAttributes.Resource

				if resource == "users" {
					return true, &authorizationv1.SelfSubjectAccessReview{
						Status: authorizationv1.SubjectAccessReviewStatus{
							Allowed: false,
							Reason:  "RBAC: impersonate users denied",
						},
					}, nil
				}
				return true, &authorizationv1.SelfSubjectAccessReview{
					Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true},
				}, nil
			})

			result, err := karbac.CheckImpersonatePermission(context.Background(), client)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Allowed).To(BeFalse())
			Expect(result.Reason).To(ContainSubstring("users"))
			Expect(callCount).To(Equal(2), "should check both users and groups")
		})

		It("should return allowed=false when the SA lacks impersonate on groups", func() {
			client := fake.NewSimpleClientset()
			client.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				createAction := action.(k8stesting.CreateAction)
				ssar := createAction.GetObject().(*authorizationv1.SelfSubjectAccessReview)
				resource := ssar.Spec.ResourceAttributes.Resource

				if resource == "groups" {
					return true, &authorizationv1.SelfSubjectAccessReview{
						Status: authorizationv1.SubjectAccessReviewStatus{
							Allowed: false,
							Reason:  "RBAC: impersonate groups denied",
						},
					}, nil
				}
				return true, &authorizationv1.SelfSubjectAccessReview{
					Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true},
				}, nil
			})

			result, err := karbac.CheckImpersonatePermission(context.Background(), client)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Allowed).To(BeFalse())
			Expect(result.Reason).To(ContainSubstring("groups"))
		})

		It("should return combined reasons when both users and groups are denied", func() {
			client := fake.NewSimpleClientset()
			client.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				createAction := action.(k8stesting.CreateAction)
				ssar := createAction.GetObject().(*authorizationv1.SelfSubjectAccessReview)
				resource := ssar.Spec.ResourceAttributes.Resource

				return true, &authorizationv1.SelfSubjectAccessReview{
					Status: authorizationv1.SubjectAccessReviewStatus{
						Allowed: false,
						Reason:  "RBAC: impersonate " + resource + " denied",
					},
				}, nil
			})

			result, err := karbac.CheckImpersonatePermission(context.Background(), client)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Allowed).To(BeFalse())
			Expect(result.Reason).To(ContainSubstring("users"))
			Expect(result.Reason).To(ContainSubstring("groups"))
		})

		It("should return error when the K8s API is unreachable", func() {
			client := fake.NewSimpleClientset()
			client.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, context.DeadlineExceeded
			})

			_, err := karbac.CheckImpersonatePermission(context.Background(), client)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("deadline"))
		})
	})

	Describe("InteractiveReadiness", func() {

		It("should report enabled when impersonate permission is granted", func() {
			status := karbac.NewInteractiveReadiness()
			status.SetEnabled()
			Expect(status.IsEnabled()).To(BeTrue())
			Expect(status.StatusString()).To(Equal("enabled"))
		})

		It("should report soft-disabled with reason when impersonate permission is denied", func() {
			status := karbac.NewInteractiveReadiness()
			status.SetSoftDisabled("RBAC: impersonate verb denied for SA kubernaut-agent")
			Expect(status.IsEnabled()).To(BeFalse())
			Expect(status.StatusString()).To(Equal("soft_disabled"))
			Expect(status.Reason()).To(ContainSubstring("impersonate"))
		})

		It("should report disabled when interactive mode is not configured", func() {
			status := karbac.NewInteractiveReadiness()
			Expect(status.IsEnabled()).To(BeFalse())
			Expect(status.StatusString()).To(Equal("not_configured"))
		})
	})

	Describe("DetectPodIdentity", func() {

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
})
