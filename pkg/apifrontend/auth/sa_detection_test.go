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

	authv1 "k8s.io/api/authentication/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// BR-INTERACTIVE-010 SC-5: ServiceAccount detection tests.
// TokenReview path sets IsServiceAccount=true; JWT path leaves it false (Go zero value).
var _ = Describe("ServiceAccount Detection [BR-INTERACTIVE-010 SC-5]", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("TokenReviewer.Validate", func() {
		It("UT-AF-1293-001: SA token sets IsServiceAccount=true", func() {
			fakeClient := k8sfake.NewSimpleClientset()
			fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				review := action.(k8stesting.CreateAction).GetObject().(*authv1.TokenReview)
				review.Status = authv1.TokenReviewStatus{
					Authenticated: true,
					User: authv1.UserInfo{
						Username: "system:serviceaccount:prod:deployer",
						Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:prod"},
					},
				}
				return true, review, nil
			})

			reviewer := auth.NewTokenReviewer(fakeClient)
			identity, err := reviewer.Validate(ctx, "sa-token-xyz")

			Expect(err).NotTo(HaveOccurred())
			Expect(identity).NotTo(BeNil())
			Expect(identity.IsServiceAccount).To(BeTrue())
			Expect(identity.Username).To(Equal("system:serviceaccount:prod:deployer"))
			Expect(identity.Groups).To(ContainElement("system:serviceaccounts"))
		})

		It("UT-AF-1293-002: unauthenticated token returns error, not false positive", func() {
			fakeClient := k8sfake.NewSimpleClientset()
			fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				review := action.(k8stesting.CreateAction).GetObject().(*authv1.TokenReview)
				review.Status = authv1.TokenReviewStatus{Authenticated: false}
				return true, review, nil
			})

			reviewer := auth.NewTokenReviewer(fakeClient)
			identity, err := reviewer.Validate(ctx, "invalid-token")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not authenticated"))
			Expect(identity).To(BeNil())
		})

		It("nil client returns error", func() {
			reviewer := auth.NewTokenReviewer(nil)
			identity, err := reviewer.Validate(ctx, "any-token")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not configured"))
			Expect(identity).To(BeNil())
		})
	})

	Describe("JWT path (human tokens)", func() {
		It("human identity has IsServiceAccount=false by default", func() {
			identity := &auth.UserIdentity{
				Username: "alice",
				Groups:   []string{"sre"},
			}

			Expect(identity.IsServiceAccount).To(BeFalse())
		})
	})
})
