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

	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
)

// TDD RED Phase: Authenticator Tests
// BR-AUTH-001: Extract authenticated user identity from Kubernetes admission requests
// SOC2 CC8.1 Requirement: Operator attribution for critical actions
//
// Per TESTING_GUIDELINES.md: Unit tests validate business behavior + implementation correctness
// Focus: Business outcomes (prevents unauthenticated operations), not implementation details
//
// Tests written BEFORE implementation exists (TDD RED Phase)

var _ = Describe("BR-AUTH-001: Authenticated User Extraction", func() {
	var (
		authenticator *authwebhook.Authenticator
		ctx           context.Context
	)

	BeforeEach(func() {
		authenticator = authwebhook.NewAuthenticator()
		ctx = context.Background()
	})

	Describe("ExtractUser - SOC2 CC8.1 Operator Attribution", func() {
		// Per TESTING_GUIDELINES.md: Use DescribeTable for similar test scenarios
		// Business Outcome: Prevent critical operations without authenticated operator identity

		Context("when operator provides valid authentication", func() {
			It("should capture operator identity for audit attribution", func() {
				// BUSINESS OUTCOME: Enables SOC2-compliant audit trail showing WHO performed action
				req := &admissionv1.AdmissionRequest{
					UserInfo: authv1.UserInfo{
						Username: "operator@kubernaut.ai",
						UID:      "k8s-user-abc-123",
					},
				}

				authCtx, err := authenticator.ExtractUser(ctx, req)

				Expect(err).ToNot(HaveOccurred(),
					"Authenticated operators can perform critical actions")
				Expect(authCtx.Username).To(Equal("operator@kubernaut.ai"),
					"Username captured for audit trail attribution")
				Expect(authCtx.UID).To(Equal("k8s-user-abc-123"),
					"UID captured for unique operator identification")
			})

			It("should format operator identity for audit trail persistence", func() {
				// BUSINESS OUTCOME: Standardized audit format for compliance reporting
				req := &admissionv1.AdmissionRequest{
					UserInfo: authv1.UserInfo{
						Username: "operator@kubernaut.ai",
						UID:      "k8s-user-abc-123",
					},
				}

				authCtx, err := authenticator.ExtractUser(ctx, req)

				Expect(err).ToNot(HaveOccurred())
				Expect(authCtx.String()).To(Equal("operator@kubernaut.ai (UID: k8s-user-abc-123)"),
					"Standardized format enables consistent audit trail reporting")
			})
		})

		Context("when authentication is missing or incomplete", func() {
			// Per TESTING_GUIDELINES.md: Use DescribeTable for similar test scenarios
			DescribeTable("prevents unauthenticated critical operations",
				func(username, uid string, shouldReject bool, businessOutcome string) {
					req := &admissionv1.AdmissionRequest{
						UserInfo: authv1.UserInfo{
							Username: username,
							UID:      uid,
						},
					}

					authCtx, err := authenticator.ExtractUser(ctx, req)

					if shouldReject {
						Expect(err).To(HaveOccurred(), businessOutcome)
						Expect(authCtx).To(BeNil(), "No authentication context for unauthenticated requests")
					} else {
						Expect(err).ToNot(HaveOccurred(), businessOutcome)
						Expect(authCtx).ToNot(BeNil(), "Authentication context provided for valid requests")
					}
				},

				// BUSINESS PROTECTION: Require complete authentication
				Entry("accepts complete operator authentication",
					"operator@kubernaut.ai", "k8s-user-123", false,
					"Complete authentication enables SOC2-compliant operator attribution"),

				// BUSINESS PROTECTION: Prevent operations without username attribution
				Entry("rejects request missing username to prevent anonymous operations",
					"", "k8s-user-123", true,
					"SOC2 CC8.1 violation: Cannot attribute action without operator username"),

				// BUSINESS PROTECTION: Prevent operations without unique identifier
				Entry("rejects request missing UID to prevent attribution conflicts",
					"operator@kubernaut.ai", "", true,
					"SOC2 CC8.1 violation: Cannot uniquely identify operator without UID"),

				// BUSINESS PROTECTION: Prevent completely unauthenticated operations
				Entry("rejects request missing both username and UID",
					"", "", true,
					"SOC2 CC8.1 violation: Critical operations require authenticated operator"),
			)

			It("should reject malformed webhook requests to prevent bypass", func() {
				// BUSINESS OUTCOME: Fail-safe rejection prevents authentication bypass

				authCtx, err := authenticator.ExtractUser(ctx, nil)

				Expect(err).To(HaveOccurred(),
					"Malformed requests cannot bypass authentication requirement")
				Expect(authCtx).To(BeNil(),
					"No authentication context for malformed requests")
			})
		})
	})
})
