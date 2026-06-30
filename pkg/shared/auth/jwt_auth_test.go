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
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
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

	// QE-9 / UT-KA-1009-004: Full ValidateTokenFull with double-nested Keycloak claims
	Describe("UT-KA-1009-004: Groups extracted from double-nested claim path via ValidateTokenFull", func() {
		It("should extract groups from resource_access.<client>.roles end-to-end", func() {
			doubleNestedAuth, err := auth.NewJWTAuthenticator([]auth.JWTProviderEntry{
				{
					Issuer:        "https://keycloak.example.com/realms/kubernaut",
					JWKSURL:       mockServer.JWKSURL(),
					Audience:      "kubernaut-agent",
					UsernameClaim: "preferred_username",
					GroupsClaim:   "resource_access.kubernaut-agent.roles",
				},
			}, logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			defer doubleNestedAuth.Close()

			token, err := mockServer.IssueJWTWithClaims("user-kc", "kubernaut-agent", 5*time.Minute, map[string]interface{}{
				"preferred_username": "user-kc@corp",
				"resource_access": map[string]interface{}{
					"kubernaut-agent": map[string]interface{}{
						"roles": []interface{}{"interactive-user", "viewer"},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			userInfo, err := doubleNestedAuth.ValidateTokenFull(ctx, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(userInfo.Username).To(Equal("user-kc@corp"))
			Expect(userInfo.Groups).To(ConsistOf("interactive-user", "viewer"))
			Expect(userInfo.ProviderType).To(Equal("jwt:https://keycloak.example.com/realms/kubernaut"))
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

	// QE-5: JWKS runtime failure — cached keys survive server shutdown (defense-in-depth)
	Describe("QE-5: JWKS cache resilience after server shutdown", func() {
		It("should still validate tokens using cached keys after JWKS server goes down", func() {
			runtimeServer, err := testauth.NewMockJWKSServer("https://runtime-fail.example.com")
			Expect(err).NotTo(HaveOccurred())

			runtimeAuth, err := auth.NewJWTAuthenticator([]auth.JWTProviderEntry{
				{
					Issuer:        "https://runtime-fail.example.com",
					JWKSURL:       runtimeServer.JWKSURL(),
					Audience:      "kubernaut-agent",
					UsernameClaim: "preferred_username",
					GroupsClaim:   "groups",
				},
			}, logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			defer runtimeAuth.Close()

			token, err := runtimeServer.IssueJWT("user@corp", []string{"g1"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			info, err := runtimeAuth.ValidateTokenFull(ctx, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Username).To(Equal("user@corp"))

			runtimeServer.Close()

			// Cached JWKS keys survive — existing tokens still validate
			info, err = runtimeAuth.ValidateTokenFull(ctx, token)
			Expect(err).NotTo(HaveOccurred(),
				"cached JWKS keys should allow validation even after server shutdown")
			Expect(info.Username).To(Equal("user@corp"))
		})
	})

	// QE-6: Concurrent ValidateTokenFull stress test
	Describe("QE-6: Concurrent token validation (race condition check)", func() {
		It("should handle concurrent validations without races", func() {
			const goroutines = 20
			tokens := make([]string, goroutines)
			for i := 0; i < goroutines; i++ {
				t, err := mockServer.IssueJWT(
					fmt.Sprintf("user-%d@corp", i),
					[]string{"g1"},
					"kubernaut-agent",
					5*time.Minute,
				)
				Expect(err).NotTo(HaveOccurred())
				tokens[i] = t
			}

			errs := make(chan error, goroutines)
			for i := 0; i < goroutines; i++ {
				go func(idx int) {
					_, err := jwtAuth.ValidateTokenFull(ctx, tokens[idx])
					errs <- err
				}(i)
			}

			for i := 0; i < goroutines; i++ {
				err := <-errs
				Expect(err).NotTo(HaveOccurred(), "goroutine %d should succeed", i)
			}
		})
	})

	// QE-7: nbf (not-before) enforcement
	Describe("QE-7: JWT with future nbf rejected", func() {
		It("should reject a token whose not-before is in the future", func() {
			now := time.Now()
			token, err := mockServer.IssueJWTWithClaims("user@corp", "kubernaut-agent", 5*time.Minute, map[string]interface{}{
				"preferred_username": "user@corp",
				"groups":             []string{"g1"},
				"nbf":               now.Add(1 * time.Hour).Unix(),
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = jwtAuth.ValidateTokenFull(ctx, token)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, auth.ErrTokenInvalid)).To(BeTrue(),
				"future nbf should be rejected as ErrTokenInvalid")
		})
	})

	// QE-8: aud as array (Keycloak multi-audience)
	Describe("QE-8: JWT with aud as array accepted", func() {
		It("should accept a token with audience as array containing the expected value", func() {
			now := time.Now()
			builder := jwt.NewBuilder().
				Subject("user@corp").
				Issuer("https://keycloak.example.com/realms/kubernaut").
				IssuedAt(now).
				NotBefore(now).
				Expiration(now.Add(5 * time.Minute)).
				Audience([]string{"kubernaut-agent", "another-service"})

			token, err := builder.Build()
			Expect(err).NotTo(HaveOccurred())
			Expect(token.Set("preferred_username", "user@corp")).To(Succeed())
			Expect(token.Set("groups", []string{"g1"})).To(Succeed())

			privJWK, err := jwk.Import(mockServer.PrivateKey)
			Expect(err).NotTo(HaveOccurred())
			Expect(privJWK.Set(jwk.KeyIDKey, "test-key-1")).To(Succeed())
			Expect(privJWK.Set(jwk.AlgorithmKey, jwa.RS256())).To(Succeed())

			signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), privJWK))
			Expect(err).NotTo(HaveOccurred())

			info, err := jwtAuth.ValidateTokenFull(ctx, string(signed))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Username).To(Equal("user@corp"))
		})
	})
})

// Regression coverage for a production incident (apifrontend E2E, CI run
// 28479808170): a JWT-provider config patch + rollout restart caused the
// kubernaut-agent deployment to never become ready. Root cause: a single
// failed JWKS fetch attempt during NewJWTAuthenticator's pre-warm caused
// jwk.Cache.Register to block on an unbounded context (httprc's
// Controller.Add docs: "the Ready() call will NOT timeout unless you
// configure your context object with context.WithTimeout"), combined with
// httprc's default 15-minute retry backoff after a failed fetch
// (httprc.DefaultMinInterval). The fix bounds Register's wait with the same
// jwksPreWarmTimeout used for the warm-up Lookup, and adds TLSCAFile support
// so JWKS endpoints behind a private CA (e.g. Dex behind the inter-service
// CA) can be verified instead of failing every fetch attempt.
var _ = Describe("JWTAuthenticator — JWKS pre-warm robustness (CI incident: AF E2E hang)", func() {
	Describe("UT-KA-1009-015: unreachable JWKS endpoint fails fast instead of hanging", func() {
		It("should return an error within the pre-warm timeout window, not hang indefinitely", func() {
			// Reserve a port, then close it immediately so the connection is
			// refused -- simulating a permanently unreachable JWKS endpoint.
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).NotTo(HaveOccurred())
			unreachableURL := fmt.Sprintf("http://%s/jwks.json", listener.Addr().String())
			Expect(listener.Close()).To(Succeed())

			done := make(chan struct{})
			var authErr error
			start := time.Now()
			go func() {
				defer close(done)
				_, authErr = auth.NewJWTAuthenticator([]auth.JWTProviderEntry{
					{
						Issuer:        "https://unreachable.example.com",
						JWKSURL:       unreachableURL,
						Audience:      "kubernaut-agent",
						UsernameClaim: "preferred_username",
						GroupsClaim:   "groups",
					},
				}, logr.Discard())
			}()

			// httprc's default retry backoff after a failed fetch is 15
			// minutes (DefaultMinInterval); before the fix, Register()'s
			// wait had no deadline and would block for that entire window.
			// 25s gives generous headroom over the 15s jwksPreWarmTimeout
			// while remaining far shorter than the 15-minute regression.
			Eventually(done, 25*time.Second, 100*time.Millisecond).Should(BeClosed(),
				"NewJWTAuthenticator must fail fast on a permanently unreachable JWKS endpoint, not hang for httprc's 15-minute retry backoff")
			Expect(authErr).To(HaveOccurred())
			Expect(time.Since(start)).To(BeNumerically("<", 20*time.Second),
				"pre-warm should be bounded by jwksPreWarmTimeout, not httprc's default retry interval")
		})
	})

	Describe("UT-KA-1009-016: custom CA file used to verify JWKS endpoint over HTTPS", func() {
		It("should successfully fetch JWKS over HTTPS when the server cert is trusted via TLSCAFile", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"keys":[]}`))
			})
			tlsServer := httptest.NewTLSServer(mux)
			defer tlsServer.Close()

			// httptest.NewTLSServer's certificate is self-signed; trusting the
			// exact leaf certificate as a root is a standard test technique
			// for exercising a custom CA pool without building a full PKI chain.
			leafCert := tlsServer.Certificate()
			caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafCert.Raw})

			tmpDir, err := os.MkdirTemp("", "jwt-auth-ca-test")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()
			caFile := filepath.Join(tmpDir, "ca.crt")
			Expect(os.WriteFile(caFile, caPEM, 0600)).To(Succeed())

			authedClient, err := auth.NewJWTAuthenticator([]auth.JWTProviderEntry{
				{
					Issuer:        "https://dex.example.com",
					JWKSURL:       tlsServer.URL + "/jwks.json",
					Audience:      "kubernaut-agent",
					UsernameClaim: "preferred_username",
					GroupsClaim:   "groups",
					TLSCAFile:     caFile,
				},
			}, logr.Discard())
			Expect(err).NotTo(HaveOccurred(),
				"JWKS fetch must succeed when the server's TLS cert is trusted via the configured CA file")
			defer authedClient.Close()
		})

		It("should fail fast (not hang) when TLSCAFile is omitted and the JWKS endpoint uses an untrusted cert", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"keys":[]}`))
			})
			tlsServer := httptest.NewTLSServer(mux)
			defer tlsServer.Close()

			done := make(chan struct{})
			var authErr error
			go func() {
				defer close(done)
				_, authErr = auth.NewJWTAuthenticator([]auth.JWTProviderEntry{
					{
						Issuer:        "https://dex.example.com",
						JWKSURL:       tlsServer.URL + "/jwks.json",
						Audience:      "kubernaut-agent",
						UsernameClaim: "preferred_username",
						GroupsClaim:   "groups",
						// TLSCAFile intentionally omitted: the server cert is
						// self-signed and untrusted by the system pool.
					},
				}, logr.Discard())
			}()

			Eventually(done, 25*time.Second, 100*time.Millisecond).Should(BeClosed(),
				"an untrusted TLS cert must fail the pre-warm fast, not hang for httprc's 15-minute retry backoff")
			Expect(authErr).To(HaveOccurred())
		})
	})
})
