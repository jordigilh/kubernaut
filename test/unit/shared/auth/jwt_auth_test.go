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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("JWTAuthenticator — #1009", func() {
	var (
		mockServer *testauth.MockJWKSServer
		jwtAuth    *auth.JWTAuthenticator
		ctx        context.Context
	)

	BeforeEach(func() {
		var err error
		mockServer, err = testauth.NewMockJWKSServer("https://keycloak.example.com/realms/kubernaut")
		Expect(err).NotTo(HaveOccurred())

		jwtAuth, err = auth.NewJWTAuthenticator([]auth.JWTProviderEntry{
			{
				Issuer:        "https://keycloak.example.com/realms/kubernaut",
				JWKSURL:       mockServer.JWKSURL(),
				Audience:      "kubernaut-agent",
				UsernameClaim: "preferred_username",
				GroupsClaim:   "groups",
			},
		}, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	AfterEach(func() {
		if mockServer != nil {
			mockServer.Close()
		}
	})

	Describe("UT-KA-1009-001: Valid JWT accepted with correct identity extraction", func() {
		It("should return correct UserInfo from a valid JWT", func() {
			token, err := mockServer.IssueJWT("user-a@corp", []string{"kubernaut-interactive-users"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			userInfo, err := jwtAuth.ValidateTokenFull(ctx, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("user-a@corp"))
			Expect(userInfo.Groups).To(ConsistOf("kubernaut-interactive-users"))
			Expect(userInfo.ProviderType).To(HavePrefix("jwt:"))
		})
	})

	Describe("UT-KA-1009-002: Username extracted from preferred_username claim", func() {
		It("should use the configured username claim path", func() {
			token, err := mockServer.IssueJWTWithClaims("sub-value", "kubernaut-agent", 5*time.Minute, map[string]interface{}{
				"preferred_username": "display-name@corp",
				"groups":             []string{"group-a"},
			})
			Expect(err).NotTo(HaveOccurred())

			userInfo, err := jwtAuth.ValidateTokenFull(ctx, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("display-name@corp"))
		})
	})

	Describe("UT-KA-1009-003: Groups extracted from nested claim path", func() {
		It("should extract groups from a dot-notation path like realm_access.roles", func() {
			nestedAuth, err := auth.NewJWTAuthenticator([]auth.JWTProviderEntry{
				{
					Issuer:        "https://keycloak.example.com/realms/kubernaut",
					JWKSURL:       mockServer.JWKSURL(),
					Audience:      "kubernaut-agent",
					UsernameClaim: "preferred_username",
					GroupsClaim:   "realm_access.roles",
				},
			}, logr.Discard())
			Expect(err).NotTo(HaveOccurred())

			token, err := mockServer.IssueJWTWithClaims("user-a", "kubernaut-agent", 5*time.Minute, map[string]interface{}{
				"preferred_username": "user-a@corp",
				"realm_access": map[string]interface{}{
					"roles": []interface{}{"admin", "operator"},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			userInfo, err := nestedAuth.ValidateTokenFull(ctx, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Groups).To(ConsistOf("admin", "operator"))
		})
	})

	Describe("UT-KA-1009-005: Top-level groups claim extracted", func() {
		It("should extract groups from a simple top-level claim", func() {
			token, err := mockServer.IssueJWTWithClaims("user-b", "kubernaut-agent", 5*time.Minute, map[string]interface{}{
				"preferred_username": "user-b@corp",
				"groups":             []interface{}{"sre-team", "kubernaut-users"},
			})
			Expect(err).NotTo(HaveOccurred())

			userInfo, err := jwtAuth.ValidateTokenFull(ctx, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Groups).To(ConsistOf("sre-team", "kubernaut-users"))
		})
	})

	Describe("UT-KA-1009-006: Expired JWT rejected", func() {
		It("should return ErrTokenInvalid for an expired token", func() {
			token, err := mockServer.IssueJWT("user-a@corp", []string{"group-a"}, "kubernaut-agent", -1*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			_, err = jwtAuth.ValidateTokenFull(ctx, token)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, auth.ErrTokenInvalid)).To(BeTrue())
		})
	})

	Describe("UT-KA-1009-007: JWT with wrong audience rejected", func() {
		It("should return ErrTokenInvalid when audience doesn't match", func() {
			token, err := mockServer.IssueJWT("user-a@corp", []string{"group-a"}, "wrong-audience", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			_, err = jwtAuth.ValidateTokenFull(ctx, token)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, auth.ErrTokenInvalid)).To(BeTrue())
		})
	})

	Describe("UT-KA-1009-008: JWT with alg:none rejected (alg confusion attack)", func() {
		It("should reject an unsigned JWT with alg:none", func() {
			header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
			payload, err := json.Marshal(map[string]interface{}{
				"iss":                "https://keycloak.example.com/realms/kubernaut",
				"sub":               "attacker",
				"aud":               "kubernaut-agent",
				"exp":               time.Now().Add(5 * time.Minute).Unix(),
				"iat":               time.Now().Unix(),
				"preferred_username": "attacker@corp",
				"groups":            []string{"admin"},
			})
			Expect(err).NotTo(HaveOccurred())
			algNoneToken := fmt.Sprintf("%s.%s.", header, base64.RawURLEncoding.EncodeToString(payload))

			_, err = jwtAuth.ValidateTokenFull(ctx, algNoneToken)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, auth.ErrTokenInvalid)).To(BeTrue(),
				"alg:none tokens must be rejected as invalid, not accepted")
		})
	})

	Describe("UT-KA-1009-009: JWT signed by unknown key rejected", func() {
		It("should return ErrTokenInvalid when signature doesn't match JWKS", func() {
			otherServer, err := testauth.NewMockJWKSServer("https://keycloak.example.com/realms/kubernaut")
			Expect(err).NotTo(HaveOccurred())
			defer otherServer.Close()

			token, err := otherServer.IssueJWT("user-a@corp", []string{"group-a"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			_, err = jwtAuth.ValidateTokenFull(ctx, token)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, auth.ErrTokenInvalid)).To(BeTrue())
		})
	})

	Describe("UT-KA-1009-010: JWT from unknown issuer returns ErrIssuerNotFound", func() {
		It("should return ErrIssuerNotFound (not ErrTokenInvalid) for an unknown issuer", func() {
			unknownServer, err := testauth.NewMockJWKSServer("https://unknown-issuer.example.com")
			Expect(err).NotTo(HaveOccurred())
			defer unknownServer.Close()

			token, err := unknownServer.IssueJWT("user-a@corp", []string{"group-a"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			_, err = jwtAuth.ValidateTokenFull(ctx, token)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, auth.ErrIssuerNotFound)).To(BeTrue())
			Expect(errors.Is(err, auth.ErrTokenInvalid)).To(BeFalse())
		})
	})

	Describe("UT-KA-1009-011: JWT with missing username claim rejected", func() {
		It("should return an error when the configured username claim is absent", func() {
			token, err := mockServer.IssueJWTWithClaims("sub-only", "kubernaut-agent", 5*time.Minute, map[string]interface{}{
				"groups": []string{"group-a"},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = jwtAuth.ValidateTokenFull(ctx, token)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("preferred_username"))
		})
	})

	Describe("UT-KA-1009-012: Multi-issuer tokens routed correctly", func() {
		It("should route tokens to the correct JWKS endpoint by issuer claim", func() {
			secondServer, err := testauth.NewMockJWKSServer("https://issuer-two.example.com")
			Expect(err).NotTo(HaveOccurred())
			defer secondServer.Close()

			multiAuth, err := auth.NewJWTAuthenticator([]auth.JWTProviderEntry{
				{
					Issuer:        "https://keycloak.example.com/realms/kubernaut",
					JWKSURL:       mockServer.JWKSURL(),
					Audience:      "kubernaut-agent",
					UsernameClaim: "preferred_username",
					GroupsClaim:   "groups",
				},
				{
					Issuer:        "https://issuer-two.example.com",
					JWKSURL:       secondServer.JWKSURL(),
					Audience:      "kubernaut-agent",
					UsernameClaim: "preferred_username",
					GroupsClaim:   "groups",
				},
			}, logr.Discard())
			Expect(err).NotTo(HaveOccurred())

			token1, err := mockServer.IssueJWT("user-from-kc@corp", []string{"g1"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			token2, err := secondServer.IssueJWT("user-from-two@corp", []string{"g2"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			info1, err := multiAuth.ValidateTokenFull(ctx, token1)
			Expect(err).NotTo(HaveOccurred())
			Expect(info1.Username).To(Equal("user-from-kc@corp"))

			info2, err := multiAuth.ValidateTokenFull(ctx, token2)
			Expect(err).NotTo(HaveOccurred())
			Expect(info2.Username).To(Equal("user-from-two@corp"))
		})
	})

	Describe("UT-KA-1009-013: Cross-provider rejection", func() {
		It("should reject a token signed by issuer-A when routed to issuer-B's JWKS", func() {
			secondServer, err := testauth.NewMockJWKSServer("https://issuer-two.example.com")
			Expect(err).NotTo(HaveOccurred())
			defer secondServer.Close()

			multiAuth, err := auth.NewJWTAuthenticator([]auth.JWTProviderEntry{
				{
					Issuer:        "https://keycloak.example.com/realms/kubernaut",
					JWKSURL:       mockServer.JWKSURL(),
					Audience:      "kubernaut-agent",
					UsernameClaim: "preferred_username",
					GroupsClaim:   "groups",
				},
				{
					Issuer:        "https://issuer-two.example.com",
					JWKSURL:       secondServer.JWKSURL(),
					Audience:      "kubernaut-agent",
					UsernameClaim: "preferred_username",
					GroupsClaim:   "groups",
				},
			}, logr.Discard())
			Expect(err).NotTo(HaveOccurred())

			// Token signed by issuer-two's key but claiming to be from keycloak
			tokenFromTwo, err := secondServer.IssueJWTWithClaims("attacker", "kubernaut-agent", 5*time.Minute, map[string]interface{}{
				"preferred_username": "attacker@corp",
				"groups":             []string{"admin"},
			})
			Expect(err).NotTo(HaveOccurred())

			// The issuer claim is "https://issuer-two.example.com" so it routes to
			// issuer-two's JWKS. But if we forge the iss claim to keycloak, it
			// should fail because the signature won't match keycloak's JWKS.
			// This test validates correct routing by iss claim.
			info, err := multiAuth.ValidateTokenFull(ctx, tokenFromTwo)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Username).To(Equal("attacker@corp"))

			// Now create a token with keycloak issuer but signed by issuer-two's key
			// (This simulates a cross-provider attack)
			forgedToken, err := secondServer.IssueJWTWithClaims("attacker", "kubernaut-agent", 5*time.Minute, map[string]interface{}{
				"preferred_username": "attacker@corp",
				"groups":             []string{"admin"},
				"iss":                "https://keycloak.example.com/realms/kubernaut",
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = multiAuth.ValidateTokenFull(ctx, forgedToken)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, auth.ErrTokenInvalid)).To(BeTrue())
		})
	})

	Describe("UT-KA-1009-014: ProviderType set on successful validation", func() {
		It("should set ProviderType to jwt:<issuer>", func() {
			token, err := mockServer.IssueJWT("user-a@corp", []string{"g1"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			userInfo, err := jwtAuth.ValidateTokenFull(ctx, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.ProviderType).To(Equal("jwt:https://keycloak.example.com/realms/kubernaut"))
		})
	})

	Describe("UT-KA-1009-ValidateToken: ValidateToken convenience wrapper", func() {
		It("should return username string from ValidateToken", func() {
			token, err := mockServer.IssueJWT("user-a@corp", []string{"g1"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			username, err := jwtAuth.ValidateToken(ctx, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(username).To(Equal("user-a@corp"))
		})
	})
})
