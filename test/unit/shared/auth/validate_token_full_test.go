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

package auth_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

var _ = Describe("ValidateTokenFull — #703", func() {
	var (
		fakeClient    *fake.Clientset
		authenticator *auth.K8sAuthenticator
		ctx           context.Context
	)

	BeforeEach(func() {
		fakeClient = fake.NewSimpleClientset()
		authenticator = auth.NewK8sAuthenticator(fakeClient)
		ctx = context.Background()
	})

	Describe("UT-KA-703-B01: ValidateTokenFull returns UserInfo with Username and Groups", func() {
		It("should return full user info including groups from TokenReview", func() {
			fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, &authenticationv1.TokenReview{
					Status: authenticationv1.TokenReviewStatus{
						Authenticated: true,
						User: authenticationv1.UserInfo{
							Username: "system:serviceaccount:kubernaut-system:interactive-user",
							Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:kubernaut-system"},
						},
					},
				}, nil
			})

			userInfo, err := authenticator.ValidateTokenFull(ctx, "valid-token")
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("system:serviceaccount:kubernaut-system:interactive-user"))
			Expect(userInfo.Groups).To(ConsistOf("system:serviceaccounts", "system:serviceaccounts:kubernaut-system"))
		})
	})

	Describe("UT-KA-703-B02: ValidateTokenFull with no-groups token returns empty slice", func() {
		It("should return empty groups slice (not nil) when token has no groups", func() {
			fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, &authenticationv1.TokenReview{
					Status: authenticationv1.TokenReviewStatus{
						Authenticated: true,
						User: authenticationv1.UserInfo{
							Username: "system:serviceaccount:default:no-groups-sa",
						},
					},
				}, nil
			})

			userInfo, err := authenticator.ValidateTokenFull(ctx, "no-groups-token")
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("system:serviceaccount:default:no-groups-sa"))
			Expect(userInfo.Groups).NotTo(BeNil())
			Expect(userInfo.Groups).To(BeEmpty())
		})
	})

	Describe("UT-KA-703-B03: ValidateTokenFull with invalid token returns ErrTokenInvalid", func() {
		It("should return ErrTokenInvalid when token is rejected", func() {
			fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, &authenticationv1.TokenReview{
					Status: authenticationv1.TokenReviewStatus{
						Authenticated: false,
					},
				}, nil
			})

			_, err := authenticator.ValidateTokenFull(ctx, "invalid-token")
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("token")))
			Expect(err).To(MatchError(auth.ErrTokenInvalid))
		})
	})

	Describe("UT-KA-703-B04: Existing ValidateToken still works (backward compatibility)", func() {
		It("should continue to return username string from ValidateToken", func() {
			fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, &authenticationv1.TokenReview{
					Status: authenticationv1.TokenReviewStatus{
						Authenticated: true,
						User: authenticationv1.UserInfo{
							Username: "system:serviceaccount:kubernaut-system:compat-sa",
							Groups:   []string{"system:serviceaccounts"},
						},
					},
				}, nil
			})

			username, err := authenticator.ValidateToken(ctx, "compat-token")
			Expect(err).NotTo(HaveOccurred())
			Expect(username).To(Equal("system:serviceaccount:kubernaut-system:compat-sa"))
		})
	})
})
