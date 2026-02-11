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

package auth_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

var _ = Describe("K8sAuthenticator", func() {
	var (
		fakeClient    *fake.Clientset
		authenticator auth.Authenticator
		ctx           context.Context
	)

	BeforeEach(func() {
		fakeClient = fake.NewSimpleClientset()
		authenticator = auth.NewK8sAuthenticator(fakeClient)
		ctx = context.Background()
	})

	Context("ValidateToken", func() {
		It("should return error for empty token", func() {
			user, err := authenticator.ValidateToken(ctx, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("token cannot be empty"))
			Expect(user).To(BeEmpty())
		})

		It("should validate token and return user identity", func() {
			// Configure fake client to return authenticated token
			fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				createAction := action.(k8stesting.CreateAction)
				review := createAction.GetObject().(*authenticationv1.TokenReview)

				// Simulate successful authentication
				review.Status = authenticationv1.TokenReviewStatus{
					Authenticated: true,
					User: authenticationv1.UserInfo{
						Username: "system:serviceaccount:test:test-sa",
						UID:      "12345",
						Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:test"},
					},
				}
				return true, review, nil
			})

			user, err := authenticator.ValidateToken(ctx, "valid-token")
			Expect(err).ToNot(HaveOccurred())
			Expect(user).To(Equal("system:serviceaccount:test:test-sa"))
		})

		It("should return error for unauthenticated token", func() {
			// Configure fake client to return unauthenticated token
			fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				createAction := action.(k8stesting.CreateAction)
				review := createAction.GetObject().(*authenticationv1.TokenReview)

				// Simulate failed authentication
				review.Status = authenticationv1.TokenReviewStatus{
					Authenticated: false,
				}
				return true, review, nil
			})

			user, err := authenticator.ValidateToken(ctx, "invalid-token")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("token not authenticated"))
			Expect(user).To(BeEmpty())
		})

		It("should return error when user identity is empty", func() {
			// Configure fake client to return authenticated but empty username
			fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				createAction := action.(k8stesting.CreateAction)
				review := createAction.GetObject().(*authenticationv1.TokenReview)

				// Simulate authenticated but no username
				review.Status = authenticationv1.TokenReviewStatus{
					Authenticated: true,
					User: authenticationv1.UserInfo{
						Username: "", // Empty username
					},
				}
				return true, review, nil
			})

			user, err := authenticator.ValidateToken(ctx, "token-without-user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("user identity is empty"))
			Expect(user).To(BeEmpty())
		})
	})
})

var _ = Describe("K8sAuthorizer", func() {
	var (
		fakeClient *fake.Clientset
		authorizer auth.Authorizer
		ctx        context.Context
	)

	BeforeEach(func() {
		fakeClient = fake.NewSimpleClientset()
		authorizer = auth.NewK8sAuthorizer(fakeClient)
		ctx = context.Background()
	})

	Context("CheckAccess", func() {
		It("should return error for empty user", func() {
			allowed, err := authorizer.CheckAccess(ctx, "", "namespace", "services", "svc-name", "create")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("user cannot be empty"))
			Expect(allowed).To(BeFalse())
		})

		It("should return error for empty namespace", func() {
			allowed, err := authorizer.CheckAccess(ctx, "user", "", "services", "svc-name", "create")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("namespace cannot be empty"))
			Expect(allowed).To(BeFalse())
		})

		It("should return error for empty resource", func() {
			allowed, err := authorizer.CheckAccess(ctx, "user", "namespace", "", "svc-name", "create")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resource cannot be empty"))
			Expect(allowed).To(BeFalse())
		})

		It("should return error for empty verb", func() {
			allowed, err := authorizer.CheckAccess(ctx, "user", "namespace", "services", "svc-name", "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("verb cannot be empty"))
			Expect(allowed).To(BeFalse())
		})

		It("should return true when access is allowed", func() {
			// Configure fake client to return allowed
			fakeClient.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				createAction := action.(k8stesting.CreateAction)
				sar := createAction.GetObject().(*authorizationv1.SubjectAccessReview)

				// Simulate allowed access
				sar.Status = authorizationv1.SubjectAccessReviewStatus{
					Allowed: true,
					Reason:  "RBAC permissions granted",
				}
				return true, sar, nil
			})

			allowed, err := authorizer.CheckAccess(
				ctx,
				"system:serviceaccount:test:authorized-sa",
				"kubernaut-system",
				"services",
				"data-storage-service",
				"create",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})

		It("should return false when access is denied", func() {
			// Configure fake client to return denied
			fakeClient.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				createAction := action.(k8stesting.CreateAction)
				sar := createAction.GetObject().(*authorizationv1.SubjectAccessReview)

				// Simulate denied access
				sar.Status = authorizationv1.SubjectAccessReviewStatus{
					Allowed: false,
					Reason:  "RBAC permissions denied",
				}
				return true, sar, nil
			})

			allowed, err := authorizer.CheckAccess(
				ctx,
				"system:serviceaccount:test:unauthorized-sa",
				"kubernaut-system",
				"services",
				"data-storage-service",
				"create",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(allowed).To(BeFalse())
		})

		It("should allow resourceName to be empty (optional parameter)", func() {
			// Configure fake client to return allowed
			fakeClient.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				createAction := action.(k8stesting.CreateAction)
				sar := createAction.GetObject().(*authorizationv1.SubjectAccessReview)

				// Verify resourceName is empty in the SAR request
				Expect(sar.Spec.ResourceAttributes.Name).To(BeEmpty())

				sar.Status = authorizationv1.SubjectAccessReviewStatus{
					Allowed: true,
				}
				return true, sar, nil
			})

			// resourceName can be empty (for list/create operations on resource types)
			allowed, err := authorizer.CheckAccess(
				ctx,
				"system:serviceaccount:test:sa",
				"kubernaut-system",
				"services",
				"", // Empty resourceName
				"list",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})
	})
})

var _ = Describe("MockAuthenticator (from mock_auth.go)", func() {
	var (
		authenticator *auth.MockAuthenticator
		ctx           context.Context
	)

	BeforeEach(func() {
		authenticator = &auth.MockAuthenticator{
			ValidUsers: map[string]string{
				"test-token-1": "system:serviceaccount:test:sa-1",
				"test-token-2": "system:serviceaccount:test:sa-2",
			},
		}
		ctx = context.Background()
	})

	It("should return user for valid token", func() {
		user, err := authenticator.ValidateToken(ctx, "test-token-1")
		Expect(err).ToNot(HaveOccurred())
		Expect(user).To(Equal("system:serviceaccount:test:sa-1"))
		Expect(authenticator.CallCount).To(Equal(1))
	})

	It("should return error for invalid token", func() {
		user, err := authenticator.ValidateToken(ctx, "invalid-token")
		Expect(err).To(HaveOccurred())
		Expect(user).To(BeEmpty())
		Expect(authenticator.CallCount).To(Equal(1))
	})

	It("should track call count", func() {
		_, _ = authenticator.ValidateToken(ctx, "test-token-1")
		_, _ = authenticator.ValidateToken(ctx, "test-token-2")
		_, _ = authenticator.ValidateToken(ctx, "invalid")

		Expect(authenticator.CallCount).To(Equal(3))
	})

	It("should return configured error when ErrorToReturn is set", func() {
		authenticator.ErrorToReturn = fmt.Errorf("simulated API error")

		user, err := authenticator.ValidateToken(ctx, "test-token-1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("simulated API error"))
		Expect(user).To(BeEmpty())
	})
})

var _ = Describe("MockAuthorizer (from mock_auth.go)", func() {
	var (
		authorizer *auth.MockAuthorizer
		ctx        context.Context
	)

	BeforeEach(func() {
		authorizer = &auth.MockAuthorizer{
			AllowedUsers: map[string]bool{
				"system:serviceaccount:test:authorized-sa": true,
				"system:serviceaccount:test:denied-sa":     false,
			},
		}
		ctx = context.Background()
	})

	It("should return true for allowed user", func() {
		allowed, err := authorizer.CheckAccess(
			ctx,
			"system:serviceaccount:test:authorized-sa",
			"namespace",
			"services",
			"svc",
			"create",
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(allowed).To(BeTrue())
		Expect(authorizer.CallCount).To(Equal(1))
	})

	It("should return false for denied user", func() {
		allowed, err := authorizer.CheckAccess(
			ctx,
			"system:serviceaccount:test:denied-sa",
			"namespace",
			"services",
			"svc",
			"create",
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(allowed).To(BeFalse())
	})

	It("should return false for unknown user (secure default)", func() {
		allowed, err := authorizer.CheckAccess(
			ctx,
			"system:serviceaccount:test:unknown-sa",
			"namespace",
			"services",
			"svc",
			"create",
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(allowed).To(BeFalse())
	})

	It("should track call count", func() {
		_, _ = authorizer.CheckAccess(ctx, "user1", "ns", "svc", "name", "create")
		_, _ = authorizer.CheckAccess(ctx, "user2", "ns", "svc", "name", "create")
		Expect(authorizer.CallCount).To(Equal(2))
	})

	It("should return configured error when ErrorToReturn is set", func() {
		authorizer.ErrorToReturn = fmt.Errorf("simulated SAR API error")

		allowed, err := authorizer.CheckAccess(
			ctx,
			"system:serviceaccount:test:authorized-sa",
			"namespace",
			"services",
			"svc",
			"create",
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("simulated SAR API error"))
		Expect(allowed).To(BeFalse())
	})

	Context("with PerResourceDecisions", func() {
		BeforeEach(func() {
			authorizer.PerResourceDecisions = map[string]map[string]bool{
				"kubernaut-system/services/data-storage-service/create": {
					"system:serviceaccount:test:specific-sa": true,
				},
			}
		})

		It("should use PerResourceDecisions for specific resource", func() {
			allowed, err := authorizer.CheckAccess(
				ctx,
				"system:serviceaccount:test:specific-sa",
				"kubernaut-system",
				"services",
				"data-storage-service",
				"create",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})

		It("should fall back to AllowedUsers for other resources", func() {
			allowed, err := authorizer.CheckAccess(
				ctx,
				"system:serviceaccount:test:authorized-sa",
				"other-namespace",
				"pods",
				"pod-name",
				"get",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})
	})
})
