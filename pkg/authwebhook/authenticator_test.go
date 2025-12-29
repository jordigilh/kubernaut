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

package authwebhook

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
)

// TDD RED Phase: Authenticator Tests
// BR-WE-013: Extract authenticated user from Kubernetes auth context
// Tests written BEFORE implementation exists
var _ = Describe("Authenticator", func() {
	var auth *Authenticator

	BeforeEach(func() {
		auth = NewAuthenticator()
	})

	Context("ExtractUser", func() {
		It("should extract user from valid admission request", func() {
			// BUSINESS OUTCOME: Capture authenticated user identity for audit trail
			req := &admissionv1.AdmissionRequest{
				UserInfo: authenticationv1.UserInfo{
					Username: "operator@example.com",
					UID:      "abc-123",
					Groups:   []string{"system:authenticated", "operators"},
					Extra: map[string]authenticationv1.ExtraValue{
						"department": []string{"platform-engineering"},
					},
				},
			}

			ctx := context.Background()
			authCtx, err := auth.ExtractUser(ctx, req)

			Expect(err).ToNot(HaveOccurred(), "Valid request should extract user")
			Expect(authCtx).ToNot(BeNil())
			Expect(authCtx.Username).To(Equal("operator@example.com"))
			Expect(authCtx.UID).To(Equal("abc-123"))
			Expect(authCtx.Groups).To(ConsistOf("system:authenticated", "operators"))
			Expect(authCtx.Extra).To(HaveKey("department"))
		})

		It("should format authenticated user string correctly", func() {
			// BUSINESS OUTCOME: Standardized user format for audit trail
			req := &admissionv1.AdmissionRequest{
				UserInfo: authenticationv1.UserInfo{
					Username: "admin@example.com",
					UID:      "xyz-789",
				},
			}

			ctx := context.Background()
			authCtx, err := auth.ExtractUser(ctx, req)

			Expect(err).ToNot(HaveOccurred())
			Expect(authCtx.String()).To(Equal("admin@example.com (UID: xyz-789)"))
		})

		It("should fail if username is missing", func() {
			// BUSINESS OUTCOME: Prevent unauthenticated actions (SOC2 CC8.1)
			req := &admissionv1.AdmissionRequest{
				UserInfo: authenticationv1.UserInfo{
					Username: "", // Missing username
					UID:      "abc-123",
				},
			}

			ctx := context.Background()
			_, err := auth.ExtractUser(ctx, req)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no user information"))
		})

		It("should fail if UID is missing", func() {
			// BUSINESS OUTCOME: Prevent ambiguous user identity (SOC2 CC8.1)
			req := &admissionv1.AdmissionRequest{
				UserInfo: authenticationv1.UserInfo{
					Username: "operator@example.com",
					UID:      "", // Missing UID
				},
			}

			ctx := context.Background()
			_, err := auth.ExtractUser(ctx, req)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no user UID"))
		})

		It("should handle service account users", func() {
			// BUSINESS OUTCOME: Support both human and service account identities
			req := &admissionv1.AdmissionRequest{
				UserInfo: authenticationv1.UserInfo{
					Username: "system:serviceaccount:kubernaut-system:workflowexecution-controller",
					UID:      "sa-uuid-123",
					Groups:   []string{"system:serviceaccounts", "system:authenticated"},
				},
			}

			ctx := context.Background()
			authCtx, err := auth.ExtractUser(ctx, req)

			Expect(err).ToNot(HaveOccurred())
			Expect(authCtx.Username).To(Equal("system:serviceaccount:kubernaut-system:workflowexecution-controller"))
			Expect(authCtx.Groups).To(ContainElement("system:serviceaccounts"))
		})

		It("should handle users with no groups", func() {
			// BUSINESS OUTCOME: Support minimal auth context
			req := &admissionv1.AdmissionRequest{
				UserInfo: authenticationv1.UserInfo{
					Username: "operator@example.com",
					UID:      "abc-123",
					Groups:   []string{}, // No groups
				},
			}

			ctx := context.Background()
			authCtx, err := auth.ExtractUser(ctx, req)

			Expect(err).ToNot(HaveOccurred())
			Expect(authCtx.Groups).To(BeEmpty())
		})

		It("should handle users with no extra attributes", func() {
			// BUSINESS OUTCOME: Support minimal auth context
			req := &admissionv1.AdmissionRequest{
				UserInfo: authenticationv1.UserInfo{
					Username: "operator@example.com",
					UID:      "abc-123",
					Extra:    nil, // No extra attributes
				},
			}

			ctx := context.Background()
			authCtx, err := auth.ExtractUser(ctx, req)

			Expect(err).ToNot(HaveOccurred())
			Expect(authCtx.Extra).To(BeNil())
		})

		It("should preserve all extra attributes", func() {
			// BUSINESS OUTCOME: Preserve additional auth context for forensics
			req := &admissionv1.AdmissionRequest{
				UserInfo: authenticationv1.UserInfo{
					Username: "operator@example.com",
					UID:      "abc-123",
					Extra: map[string]authenticationv1.ExtraValue{
						"department":     []string{"platform-engineering"},
						"cost-center":    []string{"eng-123"},
						"incident-id":    []string{"inc-456"},
					},
				},
			}

			ctx := context.Background()
			authCtx, err := auth.ExtractUser(ctx, req)

			Expect(err).ToNot(HaveOccurred())
			Expect(authCtx.Extra).To(HaveLen(3))
			Expect(authCtx.Extra).To(HaveKey("department"))
			Expect(authCtx.Extra).To(HaveKey("cost-center"))
			Expect(authCtx.Extra).To(HaveKey("incident-id"))
		})
	})
})

