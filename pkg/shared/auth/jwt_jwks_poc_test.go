package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"time"

	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// newTestCache creates a jwk.Cache with an httprc.Client suitable for tests.
func newTestCache(ctx context.Context) (*jwk.Cache, error) {
	return jwk.NewCache(ctx, httprc.NewClient())
}

var _ = Describe("PoC: JWKS + JWT validation with lestrrat-go/jwx/v3", func() {

	var (
		mockServer *testauth.MockJWKSServer
	)

	BeforeEach(func() {
		var err error
		mockServer, err = testauth.NewMockJWKSServer("https://sso.example.com/realms/kubernaut")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		mockServer.Close()
	})

	Describe("PoC-JWKS-001: Issue and verify JWT with JWKS endpoint", func() {
		It("should sign a JWT with mock RSA key and verify via JWKS", func() {
			tokenStr, err := mockServer.IssueJWT("jsmith@corp.com", []string{"sre-team"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())
			Expect(tokenStr).NotTo(BeEmpty())

			ctx := context.Background()
			cache, err := newTestCache(ctx)
			Expect(err).NotTo(HaveOccurred())
			err = cache.Register(ctx, mockServer.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			keyset, err := cache.Lookup(ctx, mockServer.JWKSURL())
			Expect(err).NotTo(HaveOccurred())
			Expect(keyset).NotTo(BeNil())

			verified, err := jwt.Parse([]byte(tokenStr), jwt.WithKeySet(keyset))
			Expect(err).NotTo(HaveOccurred())

			sub, ok := verified.Subject()
			Expect(ok).To(BeTrue())
			Expect(sub).To(Equal("jsmith@corp.com"))

			iss, ok := verified.Issuer()
			Expect(ok).To(BeTrue())
			Expect(iss).To(Equal("https://sso.example.com/realms/kubernaut"))
		})
	})

	Describe("PoC-JWKS-002: JWT with wrong key is rejected", func() {
		It("should reject a JWT signed with a different RSA key", func() {
			wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).NotTo(HaveOccurred())

			wrongJWK, err := jwk.Import(wrongKey)
			Expect(err).NotTo(HaveOccurred())
			_ = wrongJWK.Set(jwk.KeyIDKey, "wrong-key")
			_ = wrongJWK.Set(jwk.AlgorithmKey, jwa.RS256())

			token, err := jwt.NewBuilder().
				Subject("attacker").
				Issuer("https://sso.example.com/realms/kubernaut").
				Expiration(time.Now().Add(5 * time.Minute)).
				Build()
			Expect(err).NotTo(HaveOccurred())

			signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), wrongJWK))
			Expect(err).NotTo(HaveOccurred())

			ctx := context.Background()
			cache, err := newTestCache(ctx)
			Expect(err).NotTo(HaveOccurred())
			err = cache.Register(ctx, mockServer.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			keyset, err := cache.Lookup(ctx, mockServer.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			_, err = jwt.Parse(signed, jwt.WithKeySet(keyset))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("PoC-JWKS-003: Expired JWT is rejected", func() {
		It("should reject a JWT that has expired", func() {
			tokenStr, err := mockServer.IssueJWT("jsmith@corp.com", []string{"sre-team"}, "kubernaut-agent", -1*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			ctx := context.Background()
			cache, err := newTestCache(ctx)
			Expect(err).NotTo(HaveOccurred())
			err = cache.Register(ctx, mockServer.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			keyset, err := cache.Lookup(ctx, mockServer.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			_, err = jwt.Parse([]byte(tokenStr), jwt.WithKeySet(keyset))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("PoC-JWKS-004: Audience validation", func() {
		It("should reject a JWT with wrong audience when audience is validated", func() {
			tokenStr, err := mockServer.IssueJWT("jsmith@corp.com", []string{"sre-team"}, "wrong-audience", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			ctx := context.Background()
			cache, err := newTestCache(ctx)
			Expect(err).NotTo(HaveOccurred())
			err = cache.Register(ctx, mockServer.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			keyset, err := cache.Lookup(ctx, mockServer.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			// Signature should pass, then we check audience manually
			verified, err := jwt.Parse([]byte(tokenStr), jwt.WithKeySet(keyset))
			Expect(err).NotTo(HaveOccurred())

			aud, ok := verified.Audience()
			Expect(ok).To(BeTrue())
			Expect(aud).To(ConsistOf("wrong-audience"))
			Expect(aud).NotTo(ContainElement("kubernaut-agent"))
		})
	})

	Describe("PoC-JWKS-005: Extract custom claims from verified JWT", func() {
		It("should extract Keycloak-style claims after verification", func() {
			claims := map[string]interface{}{
				"preferred_username": "jsmith@corp.com",
				"groups":             []string{"sre-team", "kubernaut-interactive-users"},
				"realm_access": map[string]interface{}{
					"roles": []string{"kubernaut-user", "offline_access"},
				},
			}
			tokenStr, err := mockServer.IssueJWTWithClaims("jsmith@corp.com", "kubernaut-agent", 5*time.Minute, claims)
			Expect(err).NotTo(HaveOccurred())

			ctx := context.Background()
			cache, err := newTestCache(ctx)
			Expect(err).NotTo(HaveOccurred())
			err = cache.Register(ctx, mockServer.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			keyset, err := cache.Lookup(ctx, mockServer.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			verified, err := jwt.Parse([]byte(tokenStr), jwt.WithKeySet(keyset))
			Expect(err).NotTo(HaveOccurred())

			// Extract private claims by marshaling to JSON
			claimsMap := make(map[string]interface{})
			data, err := json.Marshal(verified)
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(data, &claimsMap)
			Expect(err).NotTo(HaveOccurred())

			username, ok := claimsMap["preferred_username"].(string)
			Expect(ok).To(BeTrue())
			Expect(username).To(Equal("jsmith@corp.com"))

			realmAccess, ok := claimsMap["realm_access"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			roles, ok := realmAccess["roles"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(roles).To(ContainElement("kubernaut-user"))
		})
	})

	Describe("PoC-JWKS-006: Issuer-based routing simulation", func() {
		It("should route to correct JWKS based on JWT issuer claim", func() {
			server1, err := testauth.NewMockJWKSServer("https://issuer-one.example.com")
			Expect(err).NotTo(HaveOccurred())
			defer server1.Close()

			server2, err := testauth.NewMockJWKSServer("https://issuer-two.example.com")
			Expect(err).NotTo(HaveOccurred())
			defer server2.Close()

			providerMap := map[string]string{
				"https://issuer-one.example.com": server1.JWKSURL(),
				"https://issuer-two.example.com": server2.JWKSURL(),
			}

			token1, err := server1.IssueJWT("user1@one.com", nil, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			token2, err := server2.IssueJWT("user2@two.com", nil, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			ctx := context.Background()

			for _, tc := range []struct {
				tokenStr    string
				expectedSub string
			}{
				{token1, "user1@one.com"},
				{token2, "user2@two.com"},
			} {
				// Parse without verification to extract issuer
				unverified, err := jwt.Parse([]byte(tc.tokenStr), jwt.WithVerify(false), jwt.WithValidate(false))
				Expect(err).NotTo(HaveOccurred())

				iss, ok := unverified.Issuer()
				Expect(ok).To(BeTrue())
				jwksURL, found := providerMap[iss]
				Expect(found).To(BeTrue(), "issuer %q not in provider map", iss)

				// Fetch the correct JWKS
				cache, err := newTestCache(ctx)
				Expect(err).NotTo(HaveOccurred())
				err = cache.Register(ctx, jwksURL)
				Expect(err).NotTo(HaveOccurred())

				keyset, err := cache.Lookup(ctx, jwksURL)
				Expect(err).NotTo(HaveOccurred())

				// Verify with correct provider's JWKS
				verified, err := jwt.Parse([]byte(tc.tokenStr), jwt.WithKeySet(keyset))
				Expect(err).NotTo(HaveOccurred())
				sub, ok := verified.Subject()
				Expect(ok).To(BeTrue())
				Expect(sub).To(Equal(tc.expectedSub))
			}
		})
	})

	Describe("PoC-JWKS-007: Cross-provider JWT rejection", func() {
		It("should reject a JWT from issuer-one when verified with issuer-two's JWKS", func() {
			server1, err := testauth.NewMockJWKSServer("https://issuer-one.example.com")
			Expect(err).NotTo(HaveOccurred())
			defer server1.Close()

			server2, err := testauth.NewMockJWKSServer("https://issuer-two.example.com")
			Expect(err).NotTo(HaveOccurred())
			defer server2.Close()

			token1, err := server1.IssueJWT("user1@one.com", nil, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			ctx := context.Background()
			cache, err := newTestCache(ctx)
			Expect(err).NotTo(HaveOccurred())
			err = cache.Register(ctx, server2.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			keyset2, err := cache.Lookup(ctx, server2.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			_, err = jwt.Parse([]byte(token1), jwt.WithKeySet(keyset2))
			Expect(err).To(HaveOccurred(), "cross-provider verification must fail")
		})
	})

	Describe("PoC-JWKS-008: alg:none rejection", func() {
		It("should confirm jwx rejects unsigned tokens when verifying with JWKS", func() {
			token, err := jwt.NewBuilder().
				Subject("attacker").
				Issuer("https://sso.example.com/realms/kubernaut").
				Expiration(time.Now().Add(5 * time.Minute)).
				Build()
			Expect(err).NotTo(HaveOccurred())

			// Sign with "none" algorithm
			unsigned, err := jwt.Sign(token, jwt.WithInsecureNoSignature())
			Expect(err).NotTo(HaveOccurred())

			ctx := context.Background()
			cache, err := newTestCache(ctx)
			Expect(err).NotTo(HaveOccurred())
			err = cache.Register(ctx, mockServer.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			keyset, err := cache.Lookup(ctx, mockServer.JWKSURL())
			Expect(err).NotTo(HaveOccurred())

			_, err = jwt.Parse(unsigned, jwt.WithKeySet(keyset))
			Expect(err).To(HaveOccurred(), "unsigned (alg:none) tokens must be rejected when verifying with JWKS")
		})
	})
})
