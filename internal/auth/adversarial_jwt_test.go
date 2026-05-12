package auth_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
)

var _ = Describe("Adversarial OIDC", func() {
	var (
		kp      *testKeyPair
		jwksSrv *httptest.Server
		cfg     auth.Config
	)

	BeforeEach(func() {
		kp = newTestKeyPair("adv-key-1")
		jwksSrv = newJWKSServer(kp.jwks())
		cfg = auth.Config{
			JWT: []auth.ProviderConfig{
				{
					Issuer: auth.IssuerConfig{
						URL:       jwksSrv.URL,
						Audiences: []string{"kubernaut-agent"},
					},
				},
			},
		}
	})

	Describe("Cryptographic attacks", func() {
		It("ADV-001: wrong signing key is rejected", func() {
			// Business outcome: a token signed with an unknown key cannot authenticate
			attackerKey := newTestKeyPair("attacker-key")
			claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			token := attackerKey.signToken(claims)

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			_, err = validator.Validate(context.Background(), token)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(auth.ErrMalformedToken))
		})

		It("ADV-002: alg:none token is rejected", func() {
			// Business outcome: unsigned tokens cannot bypass authentication
			claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			payload, err := json.Marshal(claims)
			Expect(err).NotTo(HaveOccurred())

			// Craft alg:none token manually (header.payload.)
			header := `{"alg":"none","typ":"JWT"}`
			token := base64url([]byte(header)) + "." + base64url(payload) + "."

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			_, err = validator.Validate(context.Background(), token)
			Expect(err).To(HaveOccurred())
		})

		It("ADV-003: alg:HS256 symmetric confusion is rejected", func() {
			// Business outcome: symmetric algorithms are not accepted (only RS256/ES256)
			claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			payload, err := json.Marshal(claims)
			Expect(err).NotTo(HaveOccurred())

			header := `{"alg":"HS256","typ":"JWT"}`
			token := base64url([]byte(header)) + "." + base64url(payload) + ".fake-sig"

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			_, err = validator.Validate(context.Background(), token)
			Expect(err).To(HaveOccurred())
		})

		It("ADV-004: ES256 token verifies correctly", func() {
			// Business outcome: ECDSA-signed tokens work alongside RSA tokens
			ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			Expect(err).NotTo(HaveOccurred())

			ecJWKS := jose.JSONWebKeySet{
				Keys: []jose.JSONWebKey{
					{Key: &ecKey.PublicKey, KeyID: "ec-key-1", Algorithm: string(jose.ES256), Use: "sig"},
				},
			}
			ecSrv := newJWKSServer(ecJWKS)

			ecCfg := auth.Config{
				JWT: []auth.ProviderConfig{
					{Issuer: auth.IssuerConfig{URL: ecSrv.URL, Audiences: []string{"kubernaut-agent"}}},
				},
			}

			signer, err := jose.NewSigner(
				jose.SigningKey{Algorithm: jose.ES256, Key: ecKey},
				(&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), "ec-key-1"),
			)
			Expect(err).NotTo(HaveOccurred())

			claims := standardClaims(ecSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			raw, err := josejwt.Signed(signer).Claims(claims).Serialize()
			Expect(err).NotTo(HaveOccurred())

			validator, err := auth.NewJWTValidator(ecCfg, auth.WithHTTPClient(ecSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			identity, err := validator.Validate(context.Background(), raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(identity.Username).To(Equal("alice"))
		})
	})

	Describe("Claim manipulation", func() {
		It("ADV-005: issuer spoofing is rejected", func() {
			// Business outcome: valid signature but wrong issuer cannot access a different provider's realm
			claims := standardClaims("https://evil-issuer.example.com", "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			token := kp.signToken(claims)

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			_, err = validator.Validate(context.Background(), token)
			Expect(err).To(MatchError(auth.ErrUnknownIssuer))
		})

		It("ADV-006: nbf in the future is rejected", func() {
			// Business outcome: time-gated tokens cannot be used prematurely
			claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			claims["nbf"] = time.Now().Add(time.Hour).Unix()
			token := kp.signToken(claims)

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			_, err = validator.Validate(context.Background(), token)
			Expect(err).To(MatchError(auth.ErrNotYetValid))
		})

		It("ADV-007: audience array with extra entries still matches", func() {
			// Business outcome: if any configured audience is present, token is accepted
			claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent", "other-service"}, nil, time.Now().Add(time.Hour))
			token := kp.signToken(claims)

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			identity, err := validator.Validate(context.Background(), token)
			Expect(err).NotTo(HaveOccurred())
			Expect(identity.Username).To(Equal("alice"))
		})

		It("ADV-008: single-string aud is accepted (RFC 7519 compat)", func() {
			// Business outcome: RFC 7519 allows aud as a single string
			claims := map[string]interface{}{
				"iss":                jwksSrv.URL,
				"sub":                "alice",
				"aud":                "kubernaut-agent", // single string, not array
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "alice",
			}
			token := kp.signToken(claims)

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			identity, err := validator.Validate(context.Background(), token)
			Expect(err).NotTo(HaveOccurred())
			Expect(identity.Username).To(Equal("alice"))
		})

		It("ADV-009: empty sub claim is handled gracefully", func() {
			// Business outcome: tokens with empty/null sub use the claim value without panic
			claims := map[string]interface{}{
				"iss": jwksSrv.URL,
				"sub": "",
				"aud": []string{"kubernaut-agent"},
				"exp": time.Now().Add(time.Hour).Unix(),
			}
			token := kp.signToken(claims)

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			identity, err := validator.Validate(context.Background(), token)
			Expect(err).NotTo(HaveOccurred())
			Expect(identity).NotTo(BeNil())
		})

		It("ADV-010: extremely long sub (>1KB) does not crash", func() {
			// Business outcome: DoS via oversized claims is mitigated
			longSub := strings.Repeat("a", 2048)
			claims := standardClaims(jwksSrv.URL, longSub, []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			token := kp.signToken(claims)

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			// Should not panic — may succeed or fail on sanitization
			_, _ = validator.Validate(context.Background(), token)
		})

		It("ADV-011: groups with control characters are sanitized", func() {
			// Business outcome: control characters in group names are stripped
			// to prevent log injection and header corruption
			claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"},
				[]string{"sre\r\nX-Injected: yes", "admin\x00root"}, time.Now().Add(time.Hour))
			token := kp.signToken(claims)

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			identity, err := validator.Validate(context.Background(), token)
			Expect(err).NotTo(HaveOccurred())
			for _, g := range identity.Groups {
				Expect(g).NotTo(ContainSubstring("\r"))
				Expect(g).NotTo(ContainSubstring("\n"))
				Expect(g).NotTo(ContainSubstring("\x00"))
			}
		})

		It("ADV-012: trailing garbage after base64 segments is rejected", func() {
			// Business outcome: malformed tokens cannot bypass parsing
			claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			token := kp.signToken(claims) + "GARBAGE"

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			_, err = validator.Validate(context.Background(), token)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Protocol/transport attacks", func() {
		It("ADV-013: non-Bearer auth scheme is rejected by middleware", func() {
			// Business outcome: only Bearer scheme is accepted
			claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			token := kp.signToken(claims)

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			handler := auth.MiddlewareWithConfig(auth.MiddlewareConfig{Validator: validator})(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))

			schemes := []string{"Basic " + token, "bearer " + token, "MAC " + token, ""}
			for _, scheme := range schemes {
				req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
				if scheme != "" {
					req.Header.Set("Authorization", scheme)
				}
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				Expect(rec.Code).To(BeNumerically(">=", 400),
					"scheme %q should be rejected", scheme)
			}
		})

		It("ADV-014: extremely long token (64KB) does not hang", func() {
			// Business outcome: oversized tokens fail fast without resource exhaustion
			longToken := strings.Repeat("eyJ", 21845) // ~64KB

			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			_, err = validator.Validate(context.Background(), longToken)
			Expect(err).To(HaveOccurred())
		})

		It("ADV-015: empty/whitespace token after Bearer is rejected", func() {
			// Business outcome: whitespace-only tokens cannot bypass auth
			validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			handler := auth.MiddlewareWithConfig(auth.MiddlewareConfig{Validator: validator})(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))

			tokens := []string{"Bearer ", "Bearer  ", "Bearer \t"}
			for _, hdr := range tokens {
				req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
				req.Header.Set("Authorization", hdr)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusUnauthorized),
					"header %q should be rejected", hdr)
			}
		})

		It("ADV-016: duplicate kid across providers routes correctly", func() {
			// Business outcome: key ID collision between providers doesn't cause cross-contamination
			kp2 := newTestKeyPair("adv-key-1") // same kid as first provider
			jwksSrv2 := newJWKSServer(kp2.jwks())

			multiCfg := auth.Config{
				JWT: []auth.ProviderConfig{
					{Issuer: auth.IssuerConfig{URL: jwksSrv.URL, Audiences: []string{"kubernaut-agent"}}},
					{Issuer: auth.IssuerConfig{URL: jwksSrv2.URL, Audiences: []string{"kubernaut-agent"}}},
				},
			}

			validator, err := auth.NewJWTValidator(multiCfg, auth.WithHTTPClient(jwksSrv.Client()))
			Expect(err).NotTo(HaveOccurred())

			// Token from provider 1 should validate against provider 1's key
			claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			token := kp.signToken(claims)

			identity, err := validator.Validate(context.Background(), token)
			Expect(err).NotTo(HaveOccurred())
			Expect(identity.Username).To(Equal("alice"))

			// Token from provider 2 signed with kp2's key
			claims2 := standardClaims(jwksSrv2.URL, "bob", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			token2 := kp2.signToken(claims2)

			identity2, err := validator.Validate(context.Background(), token2)
			Expect(err).NotTo(HaveOccurred())
			Expect(identity2.Username).To(Equal("bob"))
		})
	})

	Describe("Replay protection", func() {
		It("ADV-017: replayed token with same jti is rejected", func() {
			// Business outcome: token replay attacks are detected and blocked
			rc := auth.NewReplayCache(1 * time.Minute)
			DeferCleanup(rc.Stop)

			validator, err := auth.NewJWTValidator(cfg,
				auth.WithHTTPClient(jwksSrv.Client()),
				auth.WithReplayCache(rc))
			Expect(err).NotTo(HaveOccurred())

			claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			claims["jti"] = "unique-request-id-adv-017"
			token := kp.signToken(claims)

			identity, err := validator.Validate(context.Background(), token)
			Expect(err).NotTo(HaveOccurred())
			Expect(identity.Username).To(Equal("alice"))

			_, err = validator.Validate(context.Background(), token)
			Expect(err).To(HaveOccurred())
		})

		It("ADV-018: replay protection is per-process only (documented limitation)", func() {
			// Business outcome: separate validator instances have independent replay state
			rc1 := auth.NewReplayCache(1 * time.Minute)
			DeferCleanup(rc1.Stop)
			rc2 := auth.NewReplayCache(1 * time.Minute)
			DeferCleanup(rc2.Stop)

			v1, err := auth.NewJWTValidator(cfg,
				auth.WithHTTPClient(jwksSrv.Client()),
				auth.WithReplayCache(rc1))
			Expect(err).NotTo(HaveOccurred())

			v2, err := auth.NewJWTValidator(cfg,
				auth.WithHTTPClient(jwksSrv.Client()),
				auth.WithReplayCache(rc2))
			Expect(err).NotTo(HaveOccurred())

			claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
			claims["jti"] = "cross-instance-jti"
			token := kp.signToken(claims)

			// Both validators accept the same token (no distributed cache)
			_, err = v1.Validate(context.Background(), token)
			Expect(err).NotTo(HaveOccurred())
			_, err = v2.Validate(context.Background(), token)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

// base64url encodes bytes without padding for crafting JWT segments.
func base64url(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
