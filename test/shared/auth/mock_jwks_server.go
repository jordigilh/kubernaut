package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// MockJWKSServer is a test OIDC provider that generates RSA key pairs,
// serves a JWKS endpoint, and issues signed JWTs. Used for unit and
// integration testing of JWTAuthenticator without a real OIDC provider.
type MockJWKSServer struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  jwk.Key
	Issuer     string
	Server     *httptest.Server
}

// NewMockJWKSServer creates a mock JWKS server with a fresh RSA key pair.
// The server serves /.well-known/jwks.json with the public key.
func NewMockJWKSServer(issuer string) (*MockJWKSServer, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	pubJWK, err := jwk.PublicKeyOf(privKey)
	if err != nil {
		return nil, err
	}
	if err := pubJWK.Set(jwk.KeyIDKey, "test-key-1"); err != nil {
		return nil, err
	}
	if err := pubJWK.Set(jwk.AlgorithmKey, jwa.RS256()); err != nil {
		return nil, err
	}
	if err := pubJWK.Set(jwk.KeyUsageKey, "sig"); err != nil {
		return nil, err
	}

	m := &MockJWKSServer{
		PrivateKey: privKey,
		PublicKey:  pubJWK,
		Issuer:     issuer,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/jwks.json", m.handleJWKS)
	m.Server = httptest.NewServer(mux)

	return m, nil
}

func (m *MockJWKSServer) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	set := jwk.NewSet()
	_ = set.AddKey(m.PublicKey)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(set)
}

// JWKSURL returns the URL of the JWKS endpoint.
func (m *MockJWKSServer) JWKSURL() string {
	return m.Server.URL + "/.well-known/jwks.json"
}

// IssueJWT creates a signed JWT with the given claims.
func (m *MockJWKSServer) IssueJWT(sub string, groups []string, aud string, exp time.Duration) (string, error) {
	claims := map[string]interface{}{
		"preferred_username": sub,
		"groups":             groups,
	}
	return m.IssueJWTWithClaims(sub, aud, exp, claims)
}

// IssueJWTWithClaims creates a signed JWT with custom claims merged with standard fields.
func (m *MockJWKSServer) IssueJWTWithClaims(sub, aud string, exp time.Duration, custom map[string]interface{}) (string, error) {
	now := time.Now()

	builder := jwt.NewBuilder().
		Subject(sub).
		Issuer(m.Issuer).
		IssuedAt(now).
		NotBefore(now).
		Expiration(now.Add(exp))

	if aud != "" {
		builder = builder.Audience([]string{aud})
	}

	token, err := builder.Build()
	if err != nil {
		return "", err
	}

	for k, v := range custom {
		if err := token.Set(k, v); err != nil {
			return "", err
		}
	}

	privJWK, err := jwk.Import(m.PrivateKey)
	if err != nil {
		return "", err
	}
	if err := privJWK.Set(jwk.KeyIDKey, "test-key-1"); err != nil {
		return "", err
	}
	if err := privJWK.Set(jwk.AlgorithmKey, jwa.RS256()); err != nil {
		return "", err
	}

	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), privJWK))
	if err != nil {
		return "", err
	}
	return string(signed), nil
}

// Close shuts down the mock server.
func (m *MockJWKSServer) Close() {
	m.Server.Close()
}
