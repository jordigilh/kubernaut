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
			It("AUTH-001: should capture operator identity for audit attribution", func() {
				// BUSINESS OUTCOME: Enables SOC2-compliant audit trail showing WHO performed action
				// Test Plan Reference: AUTH-001 - Extract Valid User Info
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

			It("AUTH-011: should format operator identity for audit trail persistence", func() {
				// BUSINESS OUTCOME: Standardized audit format for compliance reporting
				// Test Plan Reference: AUTH-011 - Format Operator Identity
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
			// Test Plan Reference: AUTH-002, AUTH-003
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

				// AUTH-002: Reject Missing Username
				Entry("AUTH-002: rejects request missing username to prevent anonymous operations",
					"", "k8s-user-123", true,
					"SOC2 CC8.1 violation: Cannot attribute action without operator username"),

			// AUTH-003: Accept Empty UID (envtest/kubeconfig contexts)
			Entry("AUTH-003: accepts request with missing UID (username is sufficient for SOC2 attribution)",
				"operator@kubernaut.ai", "", false,
				"Username provides sufficient attribution in test/kubeconfig contexts"),

				// Additional Edge Case: Both missing (not in original plan, discovered during implementation)
				Entry("AUTH-002+003: rejects request missing both username and UID",
					"", "", true,
					"SOC2 CC8.1 violation: Critical operations require authenticated operator"),
			)
		})

		Context("when handling special authentication scenarios", func() {
			// Test Plan Reference: AUTH-004, AUTH-009, AUTH-010
			DescribeTable("extracts identity from various user types",
				func(username, uid string, groups []string, expectedGroupCount int, businessOutcome string) {
					req := &admissionv1.AdmissionRequest{
						UserInfo: authv1.UserInfo{
							Username: username,
							UID:      uid,
							Groups:   groups,
						},
					}

					authCtx, err := authenticator.ExtractUser(ctx, req)

					Expect(err).ToNot(HaveOccurred(), businessOutcome)
					Expect(authCtx).ToNot(BeNil())
					Expect(authCtx.Username).To(Equal(username))
					Expect(authCtx.UID).To(Equal(uid))
					Expect(authCtx.Groups).To(HaveLen(expectedGroupCount))

					// Verify all groups are preserved
					for _, group := range groups {
						Expect(authCtx.Groups).To(ContainElement(group),
							"All groups must be preserved for RBAC audit trail")
					}
				},

				// AUTH-004: Extract Multiple Groups
				Entry("AUTH-004: extracts user with multiple group memberships",
					"operator@kubernaut.ai",
					"k8s-user-123",
					[]string{"system:authenticated", "operators", "admins", "sre-team"},
					4,
					"SOC2 CC8.1: All group memberships preserved for RBAC audit trail"),

				// AUTH-009: Extract User with No Groups
				Entry("AUTH-009: extracts user with empty groups list",
					"operator@kubernaut.ai",
					"k8s-user-123",
					[]string{},
					0,
					"SOC2 CC8.1: Empty groups list is acceptable for audit attribution"),

				// AUTH-010: Extract Service Account User
				Entry("AUTH-010: extracts Kubernetes ServiceAccount user identity",
					"system:serviceaccount:kubernaut-system:webhook-controller",
					"sa-uid-789",
					[]string{"system:serviceaccounts", "system:authenticated"},
					2,
					"SOC2 CC8.1: Service account identities supported for audit trail"),
			)

			It("AUTH-012: should reject malformed webhook requests to prevent bypass", func() {
				// BUSINESS OUTCOME: Fail-safe rejection prevents authentication bypass
				// Test Plan Reference: AUTH-012 - Reject Malformed Requests

				authCtx, err := authenticator.ExtractUser(ctx, nil)

				Expect(err).To(HaveOccurred(),
					"Malformed requests cannot bypass authentication requirement")
				Expect(authCtx).To(BeNil(),
					"No authentication context for malformed requests")
			})
		})
	})
})
