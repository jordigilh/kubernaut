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
	"errors"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CompositeAuthenticator — #1009", func() {
	var (
		mockJWKS   *testauth.MockJWKSServer
		jwtAuth    *auth.JWTAuthenticator
		mockK8s    *auth.MockAuthenticator
		composite  *auth.CompositeAuthenticator
		ctx        context.Context
	)

	BeforeEach(func() {
		var err error
		mockJWKS, err = testauth.NewMockJWKSServer("https://keycloak.example.com/realms/kubernaut")
		Expect(err).NotTo(HaveOccurred())

		jwtAuth, err = auth.NewJWTAuthenticator([]auth.JWTProviderEntry{
			{
				Issuer:        "https://keycloak.example.com/realms/kubernaut",
				JWKSURL:       mockJWKS.JWKSURL(),
				Audience:      "kubernaut-agent",
				UsernameClaim: "preferred_username",
				GroupsClaim:   "groups",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		mockK8s = &auth.MockAuthenticator{
			ValidUsers: map[string]string{
				"sa-token-valid": "system:serviceaccount:kubernaut-system:apifrontend",
			},
			ValidUsersFull: map[string]auth.UserInfo{
				"sa-token-valid": {
					Username:     "system:serviceaccount:kubernaut-system:apifrontend",
					Groups:       []string{"system:serviceaccounts"},
					ProviderType: "k8s:tokenreview",
				},
			},
		}

		composite = auth.NewCompositeAuthenticator(jwtAuth, mockK8s)
		ctx = context.Background()
	})

	AfterEach(func() {
		if mockJWKS != nil {
			mockJWKS.Close()
		}
	})

	Describe("UT-KA-1009-015: JWT-shaped token routed to JWTAuthenticator", func() {
		It("should route a valid JWT to JWTAuthenticator and return correct identity", func() {
			token, err := mockJWKS.IssueJWT("user-a@corp", []string{"interactive-users"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			userInfo, err := composite.ValidateTokenFull(ctx, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("user-a@corp"))
			Expect(userInfo.Groups).To(ConsistOf("interactive-users"))
			Expect(userInfo.ProviderType).To(HavePrefix("jwt:"))
			Expect(mockK8s.CallCount).To(Equal(0))
		})
	})

	Describe("UT-KA-1009-016: Known issuer with invalid signature → fail-closed", func() {
		It("should NOT fallback to K8s when JWT has known issuer but invalid signature", func() {
			otherServer, err := testauth.NewMockJWKSServer("https://keycloak.example.com/realms/kubernaut")
			Expect(err).NotTo(HaveOccurred())
			defer otherServer.Close()

			token, err := otherServer.IssueJWT("attacker@corp", []string{"admin"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			_, err = composite.ValidateTokenFull(ctx, token)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, auth.ErrTokenInvalid)).To(BeTrue())
			Expect(mockK8s.CallCount).To(Equal(0))
		})
	})

	Describe("UT-KA-1009-017: Unknown issuer → fallback to K8sAuthenticator", func() {
		It("should fallback to K8s when JWT issuer is not in configured providers", func() {
			unknownServer, err := testauth.NewMockJWKSServer("https://unknown-issuer.example.com")
			Expect(err).NotTo(HaveOccurred())
			defer unknownServer.Close()

			token, err := unknownServer.IssueJWT("user-x@corp", []string{"g1"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			mockK8s.ValidUsers[token] = "system:serviceaccount:kubernaut-system:unknown-sa"
			mockK8s.ValidUsersFull[token] = auth.UserInfo{
				Username:     "system:serviceaccount:kubernaut-system:unknown-sa",
				Groups:       []string{"system:serviceaccounts"},
				ProviderType: "k8s:tokenreview",
			}

			userInfo, err := composite.ValidateTokenFull(ctx, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("system:serviceaccount:kubernaut-system:unknown-sa"))
			Expect(userInfo.ProviderType).To(Equal("k8s:tokenreview"))
			Expect(mockK8s.CallCount).To(Equal(1))
		})
	})

	Describe("UT-KA-1009-018: Opaque token → direct K8sAuthenticator", func() {
		It("should route opaque (non-JWT) tokens directly to K8sAuthenticator", func() {
			userInfo, err := composite.ValidateTokenFull(ctx, "sa-token-valid")
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("system:serviceaccount:kubernaut-system:apifrontend"))
			Expect(userInfo.ProviderType).To(Equal("k8s:tokenreview"))
			Expect(mockK8s.CallCount).To(Equal(1))
		})
	})

	Describe("UT-KA-1009-019: Empty token → error", func() {
		It("should return an error for empty token without fallback", func() {
			_, err := composite.ValidateTokenFull(ctx, "")
			Expect(err).To(HaveOccurred())
			Expect(mockK8s.CallCount).To(Equal(0))
		})
	})

	Describe("UT-KA-1009-020: K8sAuthenticator error after fallback → propagated", func() {
		It("should propagate K8s auth errors when falling back from unknown issuer", func() {
			unknownServer, err := testauth.NewMockJWKSServer("https://unknown-issuer.example.com")
			Expect(err).NotTo(HaveOccurred())
			defer unknownServer.Close()

			token, err := unknownServer.IssueJWT("user-x@corp", []string{"g1"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			_, err = composite.ValidateTokenFull(ctx, token)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, auth.ErrTokenInvalid)).To(BeTrue())
		})
	})

	Describe("UT-KA-1009-021: Malformed JWT → fallback to K8s", func() {
		It("should treat a malformed 3-part token as opaque and try K8s", func() {
			malformedJWT := "not-base64.not-base64.not-base64"

			mockK8s.ValidUsers[malformedJWT] = "system:serviceaccount:kubernaut-system:some-sa"
			mockK8s.ValidUsersFull[malformedJWT] = auth.UserInfo{
				Username:     "system:serviceaccount:kubernaut-system:some-sa",
				Groups:       []string{"system:serviceaccounts"},
				ProviderType: "k8s:tokenreview",
			}

			userInfo, err := composite.ValidateTokenFull(ctx, malformedJWT)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("system:serviceaccount:kubernaut-system:some-sa"))
			Expect(mockK8s.CallCount).To(Equal(1))
		})
	})

	Describe("ValidateToken convenience wrapper", func() {
		It("should return username string via ValidateToken", func() {
			token, err := mockJWKS.IssueJWT("user-a@corp", []string{"g1"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			username, err := composite.ValidateToken(ctx, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(username).To(Equal("user-a@corp"))
		})
	})
})
