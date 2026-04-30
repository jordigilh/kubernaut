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

package mcp_test

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

var _ = Describe("extractEffectiveUser — #703", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("UT-KA-703-D01: Pattern A (direct) -- Bearer token resolves via ValidateTokenFull", func() {
		It("should return UserInfo from token validation when no impersonation headers present", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsersFull: map[string]auth.UserInfo{
					"user-token": {
						Username: "developer@example.com",
						Groups:   []string{"dev-team", "viewers"},
					},
				},
			}
			authorizer := &auth.MockAuthorizer{}

			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/mcp/v1/sse", nil)
			req.Header.Set("Authorization", "Bearer user-token")

			userInfo, err := mcp.ExtractEffectiveUser(ctx, req, authenticator, authorizer)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("developer@example.com"))
			Expect(userInfo.Groups).To(ConsistOf("dev-team", "viewers"))
		})
	})

	Describe("UT-KA-703-D02: Pattern A with injected Impersonate-User header -- defense-in-depth rejection", func() {
		It("should ignore impersonation headers for non-SA users (Pattern A)", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsersFull: map[string]auth.UserInfo{
					"user-token": {
						Username: "developer@example.com",
						Groups:   []string{"dev-team"},
					},
				},
			}
			authorizer := &auth.MockAuthorizer{}

			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/mcp/v1/sse", nil)
			req.Header.Set("Authorization", "Bearer user-token")
			req.Header.Set("Impersonate-User", "admin@evil.com")

			userInfo, err := mcp.ExtractEffectiveUser(ctx, req, authenticator, authorizer)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("developer@example.com"),
				"non-SA callers must never trigger delegation pattern")
		})
	})

	Describe("UT-KA-703-D03: Pattern B (delegated) -- SA token + SAR confirms impersonate RBAC", func() {
		It("should return delegated UserInfo when SA has impersonate permission", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsersFull: map[string]auth.UserInfo{
					"sa-token": {
						Username: "system:serviceaccount:kubernaut-system:gateway",
						Groups:   []string{"system:serviceaccounts"},
					},
				},
			}
			authorizer := &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{
					"system:serviceaccount:kubernaut-system:gateway": true,
				},
			}

			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/mcp/v1/sse", nil)
			req.Header.Set("Authorization", "Bearer sa-token")
			req.Header.Set("Impersonate-User", "real-user@company.com")
			req.Header.Set("Impersonate-Group", "engineering")

			userInfo, err := mcp.ExtractEffectiveUser(ctx, req, authenticator, authorizer)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("real-user@company.com"))
			Expect(userInfo.Groups).To(ContainElement("engineering"))
		})
	})

	Describe("UT-KA-703-D04: Pattern B without impersonate RBAC -- REJECTED (403)", func() {
		It("should return error when SA lacks impersonate permission", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsersFull: map[string]auth.UserInfo{
					"sa-token": {
						Username: "system:serviceaccount:default:unprivileged",
						Groups:   []string{"system:serviceaccounts"},
					},
				},
			}
			authorizer := &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{
					"system:serviceaccount:default:unprivileged": false,
				},
			}

			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/mcp/v1/sse", nil)
			req.Header.Set("Authorization", "Bearer sa-token")
			req.Header.Set("Impersonate-User", "target-user@company.com")

			_, err := mcp.ExtractEffectiveUser(ctx, req, authenticator, authorizer)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("impersonate"))
		})
	})

	Describe("UT-KA-703-D05: Pattern B with SA token but missing Impersonate-User -- REJECTED", func() {
		It("should fall back to Pattern A when SA does not provide impersonation headers", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsersFull: map[string]auth.UserInfo{
					"sa-token": {
						Username: "system:serviceaccount:kubernaut-system:gateway",
						Groups:   []string{"system:serviceaccounts"},
					},
				},
			}
			authorizer := &auth.MockAuthorizer{}

			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/mcp/v1/sse", nil)
			req.Header.Set("Authorization", "Bearer sa-token")

			userInfo, err := mcp.ExtractEffectiveUser(ctx, req, authenticator, authorizer)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("system:serviceaccount:kubernaut-system:gateway"),
				"without Impersonate-User header, SA identity is used directly (Pattern A fallback)")
		})
	})

	Describe("UT-KA-703-D06: No Bearer token -- REJECTED (401)", func() {
		It("should return error when no Authorization header is present", func() {
			authenticator := &auth.MockAuthenticator{}
			authorizer := &auth.MockAuthorizer{}

			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/mcp/v1/sse", nil)

			_, err := mcp.ExtractEffectiveUser(ctx, req, authenticator, authorizer)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("authorization"))
		})
	})

	Describe("UT-KA-703-D07: Deterministic -- no fallthrough between patterns", func() {
		It("should exclusively use Pattern B when SA identity + Impersonate-User detected", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsersFull: map[string]auth.UserInfo{
					"sa-token": {
						Username: "system:serviceaccount:kubernaut-system:gateway",
						Groups:   []string{"system:serviceaccounts"},
					},
				},
			}
			authorizer := &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{
					"system:serviceaccount:kubernaut-system:gateway": true,
				},
			}

			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/mcp/v1/sse", nil)
			req.Header.Set("Authorization", "Bearer sa-token")
			req.Header.Set("Impersonate-User", "delegated-user@corp.com")

			userInfo, err := mcp.ExtractEffectiveUser(ctx, req, authenticator, authorizer)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("delegated-user@corp.com"),
				"Pattern B must return delegated identity, not SA identity")
			Expect(userInfo.Username).NotTo(Equal("system:serviceaccount:kubernaut-system:gateway"),
				"SA identity must not leak through")
		})
	})
})
