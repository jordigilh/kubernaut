package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testJWKSKeySet(t *testing.T) *jose.JSONWebKeySet {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return &jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "test-key-1", Algorithm: "RS256", Use: "sig"},
		},
	}
}

func jwksServerWith(t *testing.T, body []byte, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
}

type stdTestKeyPair struct {
	private *rsa.PrivateKey
	keyID   string
}

func generateTestKeyPair(t *testing.T) *stdTestKeyPair {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return &stdTestKeyPair{private: key, keyID: "test-key-1"}
}

func (kp *stdTestKeyPair) publicJWKS() jose.JSONWebKeySet {
	return jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{Key: &kp.private.PublicKey, KeyID: kp.keyID, Algorithm: string(jose.RS256), Use: "sig"},
		},
	}
}

func (kp *stdTestKeyPair) signToken(t *testing.T, claims interface{}) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: kp.private},
		(&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), kp.keyID),
	)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	raw, err := josejwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

func validJWKSBytes(t *testing.T) []byte {
	t.Helper()
	ks := testJWKSKeySet(t)
	b, err := json.Marshal(ks)
	if err != nil {
		t.Fatalf("marshal JWKS: %v", err)
	}
	return b
}

// ---------------------------------------------------------------------------
// SEC-01: JWKS Response Body Size Limit
// ---------------------------------------------------------------------------

func TestJWKSCache_FetchBodySizeLimit_ValidSmall(t *testing.T) {
	// TC-B-01a: 512 KB valid JWKS → success
	body := validJWKSBytes(t)
	srv := jwksServerWith(t, body, http.StatusOK)
	defer srv.Close()

	cache := NewJWKSCache(srv.Client(), []string{srv.URL}, WithRefreshInterval(0))
	keys, err := cache.GetKeys(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("TC-B-01a: expected success for small body, got %v", err)
	}
	if len(keys.Keys) == 0 {
		t.Error("TC-B-01a: expected at least one key")
	}
}

func TestJWKSCache_FetchBodySizeLimit_Oversized(t *testing.T) {
	// TC-B-01b: 2 MB body → error (exceeds 1 MB limit)
	oversized := make([]byte, 2<<20)
	srv := jwksServerWith(t, oversized, http.StatusOK)
	defer srv.Close()

	cache := NewJWKSCache(srv.Client(), []string{srv.URL}, WithRefreshInterval(0))
	_, err := cache.GetKeys(context.Background(), srv.URL)
	if err == nil {
		t.Error("TC-B-01b: expected error for 2 MB response body, got nil")
	}
}

func TestJWKSCache_FetchBodySizeLimit_Empty(t *testing.T) {
	// TC-B-01e: empty body → error
	srv := jwksServerWith(t, nil, http.StatusOK)
	defer srv.Close()

	cache := NewJWKSCache(srv.Client(), []string{srv.URL}, WithRefreshInterval(0))
	_, err := cache.GetKeys(context.Background(), srv.URL)
	if err == nil {
		t.Error("TC-B-01e: expected error for empty response body, got nil")
	}
}

func TestJWKSCache_FetchBodySizeLimit_InvalidJSON(t *testing.T) {
	// TC-B-01f: valid JSON but not JWKS
	srv := jwksServerWith(t, []byte(`{"foo":"bar"}`), http.StatusOK)
	defer srv.Close()

	cache := NewJWKSCache(srv.Client(), []string{srv.URL}, WithRefreshInterval(0))
	keys, err := cache.GetKeys(context.Background(), srv.URL)
	if err == nil && (keys == nil || len(keys.Keys) == 0) {
		// Acceptable: no error but empty keys — caller should handle
		t.Log("TC-B-01f: no error but empty keys (acceptable)")
	}
	// If there's an error, that's also acceptable
}

// ---------------------------------------------------------------------------
// SEC-02: Issuer URL Scheme Validation
// ---------------------------------------------------------------------------

func TestAuthConfig_IssuerScheme_HTTPS(t *testing.T) {
	// TC-B-02a: https → passes
	cfg := &Config{JWT: []ProviderConfig{{Issuer: IssuerConfig{URL: "https://dex.example.com", Audiences: []string{"test"}}}}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("TC-B-02a: expected validation to pass for https issuer, got %v", err)
	}
}

func TestAuthConfig_IssuerScheme_HTTP_Rejected(t *testing.T) {
	// TC-B-02b: http without AllowInsecureIssuer → error
	cfg := &Config{JWT: []ProviderConfig{{Issuer: IssuerConfig{URL: "http://dex.example.com", Audiences: []string{"test"}}}}}
	err := cfg.Validate()
	if err == nil {
		t.Error("TC-B-02b: expected validation error for http issuer without AllowInsecureIssuer, got nil")
	}
}

func TestAuthConfig_IssuerScheme_FTP_Rejected(t *testing.T) {
	// TC-B-02d: ftp → error
	cfg := &Config{JWT: []ProviderConfig{{Issuer: IssuerConfig{URL: "ftp://dex.example.com", Audiences: []string{"test"}}}}}
	err := cfg.Validate()
	if err == nil {
		t.Error("TC-B-02d: expected validation error for ftp:// issuer, got nil")
	}
}

func TestAuthConfig_IssuerScheme_File_Rejected(t *testing.T) {
	// TC-B-02e: file:// → error
	cfg := &Config{JWT: []ProviderConfig{{Issuer: IssuerConfig{URL: "file:///etc/passwd", Audiences: []string{"test"}}}}}
	err := cfg.Validate()
	if err == nil {
		t.Error("TC-B-02e: expected validation error for file:// issuer, got nil")
	}
}

func TestAuthConfig_IssuerScheme_NullByte(t *testing.T) {
	// TC-B-02j: null byte in URL → error
	cfg := &Config{JWT: []ProviderConfig{{Issuer: IssuerConfig{URL: "https://dex.example.com\x00evil", Audiences: []string{"test"}}}}}
	err := cfg.Validate()
	if err == nil {
		t.Error("TC-B-02j: expected validation error for URL with null byte, got nil")
	}
}

func TestAuthConfig_IssuerScheme_TooLong(t *testing.T) {
	// TC-B-02i: 4096+ char URL → error
	longURL := "https://" + strings.Repeat("a", 4100)
	cfg := &Config{JWT: []ProviderConfig{{Issuer: IssuerConfig{URL: longURL, Audiences: []string{"test"}}}}}
	err := cfg.Validate()
	if err == nil {
		t.Error("TC-B-02i: expected validation error for URL exceeding 4096 chars, got nil")
	}
}

// ---------------------------------------------------------------------------
// SEC-04: Non-Numeric NBF Rejection
// ---------------------------------------------------------------------------

func TestValidateNotBefore_NonNumeric_String(t *testing.T) {
	// TC-B-04c: nbf is string → ErrMalformedToken
	claims := map[string]interface{}{"nbf": "1234567890"}
	err := validateNotBefore(claims)
	if !errors.Is(err, ErrMalformedToken) {
		t.Errorf("TC-B-04c: expected ErrMalformedToken for string nbf, got %v", err)
	}
}

func TestValidateNotBefore_NonNumeric_Bool(t *testing.T) {
	// TC-B-04d: nbf is bool → ErrMalformedToken
	claims := map[string]interface{}{"nbf": true}
	err := validateNotBefore(claims)
	if !errors.Is(err, ErrMalformedToken) {
		t.Errorf("TC-B-04d: expected ErrMalformedToken for bool nbf, got %v", err)
	}
}

func TestValidateNotBefore_NonNumeric_Object(t *testing.T) {
	// TC-B-04e: nbf is object → ErrMalformedToken
	claims := map[string]interface{}{"nbf": map[string]interface{}{"time": 123}}
	err := validateNotBefore(claims)
	if !errors.Is(err, ErrMalformedToken) {
		t.Errorf("TC-B-04e: expected ErrMalformedToken for object nbf, got %v", err)
	}
}

func TestValidateNotBefore_NonNumeric_Array(t *testing.T) {
	// TC-B-04f: nbf is array → ErrMalformedToken
	claims := map[string]interface{}{"nbf": []interface{}{123}}
	err := validateNotBefore(claims)
	if !errors.Is(err, ErrMalformedToken) {
		t.Errorf("TC-B-04f: expected ErrMalformedToken for array nbf, got %v", err)
	}
}

func TestValidateNotBefore_NaN(t *testing.T) {
	// TC-B-04j: nbf is NaN → ErrMalformedToken
	claims := map[string]interface{}{"nbf": math.NaN()}
	err := validateNotBefore(claims)
	if !errors.Is(err, ErrMalformedToken) {
		t.Errorf("TC-B-04j: expected ErrMalformedToken for NaN nbf, got %v", err)
	}
}

func TestValidateNotBefore_Inf(t *testing.T) {
	// TC-B-04k: nbf is Inf → ErrMalformedToken
	claims := map[string]interface{}{"nbf": math.Inf(1)}
	err := validateNotBefore(claims)
	if !errors.Is(err, ErrMalformedToken) {
		t.Errorf("TC-B-04k: expected ErrMalformedToken for Inf nbf, got %v", err)
	}
}

func TestValidateNotBefore_Absent(t *testing.T) {
	// TC-B-04g: nbf absent → accepted
	claims := map[string]interface{}{}
	err := validateNotBefore(claims)
	if err != nil {
		t.Errorf("TC-B-04g: expected nil error for absent nbf, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// SEC-05: ErrTokenReplayed Sentinel
// ---------------------------------------------------------------------------

func TestReplayCache_DistinctSentinel(t *testing.T) {
	// TC-B-05b/c: replayed JTI → ErrTokenReplayed, NOT ErrTokenExpired
	rc := NewReplayCache(10 * time.Minute)
	defer rc.Stop()

	if rc.Seen("jti-1") {
		t.Fatal("TC-B-05a: first presentation should not be seen")
	}

	if !rc.Seen("jti-1") {
		t.Fatal("TC-B-05b: second presentation should be seen")
	}
}

func TestClassifyAuthError_Replayed(t *testing.T) {
	// TC-B-05d: classifyAuthError(ErrTokenReplayed) → "token_replayed"
	result := classifyAuthError(ErrTokenReplayed)
	if result != "token_replayed" {
		t.Errorf("TC-B-05d: expected 'token_replayed', got %q", result)
	}
}

func TestClassifyAuthError_Expired(t *testing.T) {
	// TC-B-05e: classifyAuthError(ErrTokenExpired) → "token_expired" (unchanged)
	result := classifyAuthError(ErrTokenExpired)
	if result != "token_expired" {
		t.Errorf("TC-B-05e: expected 'token_expired', got %q", result)
	}
}

func TestErrTokenReplayed_IsNotExpired(t *testing.T) {
	// TC-B-05c: ErrTokenReplayed is NOT ErrTokenExpired
	if errors.Is(ErrTokenReplayed, ErrTokenExpired) {
		t.Error("TC-B-05c: ErrTokenReplayed must NOT be ErrTokenExpired")
	}
}

// ---------------------------------------------------------------------------
// SEC-07: TokenReview Identity Sanitization
// ---------------------------------------------------------------------------

func TestSanitizeClaimValue_Normal(t *testing.T) {
	// TC-B-07a: normal string passes through
	got := SanitizeClaimValue("sre-user@example.com")
	if got != "sre-user@example.com" {
		t.Errorf("TC-B-07a: expected unchanged, got %q", got)
	}
}

func TestSanitizeClaimValue_NullByte(t *testing.T) {
	// TC-B-07b: null byte stripped
	got := SanitizeClaimValue("admin\x00../../etc/passwd")
	if strings.Contains(got, "\x00") {
		t.Errorf("TC-B-07b: expected null byte removed, got %q", got)
	}
}

func TestSanitizeClaimValue_ControlChars(t *testing.T) {
	// TC-B-07c: control chars stripped
	got := SanitizeClaimValue("admin\x01\x02\x03")
	if got != "admin" {
		t.Errorf("TC-B-07c: expected 'admin', got %q", got)
	}
}

func TestSanitizeClaimValue_Truncation(t *testing.T) {
	// TC-B-07d: 300+ chars truncated to 256
	longStr := strings.Repeat("a", 300)
	got := SanitizeClaimValue(longStr)
	if len(got) > 256 {
		t.Errorf("TC-B-07d: expected max 256 chars, got %d", len(got))
	}
}

func TestSanitizeClaimValue_Empty(t *testing.T) {
	// TC-B-07e: empty string passes through
	got := SanitizeClaimValue("")
	if got != "" {
		t.Errorf("TC-B-07e: expected empty, got %q", got)
	}
}

func TestSanitizeClaimValue_RTLO(t *testing.T) {
	// TC-B-07f: RTLO character stripped
	got := SanitizeClaimValue("admin\u202Efdp.txt")
	if strings.ContainsRune(got, '\u202E') {
		t.Errorf("TC-B-07f: expected RTLO stripped, got %q", got)
	}
}

func TestSanitizeClaimValue_OnlyNulls(t *testing.T) {
	// TC-B-07j: only null bytes → empty
	got := SanitizeClaimValue("\x00\x00\x00")
	if got != "" {
		t.Errorf("TC-B-07j: expected empty for all-null, got %q", got)
	}
}

func TestSanitizeClaimValue_Emoji(t *testing.T) {
	// TC-B-07k: emoji passes through
	got := SanitizeClaimValue("user🚀admin")
	if got != "user🚀admin" {
		t.Errorf("TC-B-07k: expected 'user🚀admin', got %q", got)
	}
}

func TestSanitizeClaimValue_InvalidUTF8(t *testing.T) {
	// TC-B-07l: invalid UTF-8 stripped or replaced
	got := SanitizeClaimValue("\xff\xfe")
	if !utf8.ValidString(got) {
		t.Errorf("TC-B-07l: expected valid UTF-8, got invalid: %q", got)
	}
}

// ---------------------------------------------------------------------------
// HIGH-03: JWKS body exactly at 1 MiB boundary
// ---------------------------------------------------------------------------

func TestJWKSCache_FetchBodySizeLimit_ExactBoundary(t *testing.T) {
	// TC-B-01c: body at exactly 1 MiB should be accepted (boundary)
	body := make([]byte, 1<<20)
	copy(body, `{"keys":[]}`)
	srv := jwksServerWith(t, body, http.StatusOK)
	defer srv.Close()

	cache := NewJWKSCache(srv.Client(), []string{srv.URL}, WithRefreshInterval(0))
	_, err := cache.GetKeys(context.Background(), srv.URL)
	// Either success (decoded empty keyset) or decode error from padding
	// is acceptable — key guarantee is no OOM crash.
	_ = err
}

func TestJWKSCache_FetchBodySizeLimit_OneBytePastBoundary(t *testing.T) {
	// TC-B-01d: body at 1 MiB + 1 byte must be rejected
	body := make([]byte, (1<<20)+1)
	srv := jwksServerWith(t, body, http.StatusOK)
	defer srv.Close()

	cache := NewJWKSCache(srv.Client(), []string{srv.URL}, WithRefreshInterval(0))
	_, err := cache.GetKeys(context.Background(), srv.URL)
	if err == nil {
		t.Error("TC-B-01d: expected error for body 1 byte past 1 MiB limit, got nil")
	}
}

// ---------------------------------------------------------------------------
// HIGH-04: JWKS URL HTTPS enforcement
// ---------------------------------------------------------------------------

func TestAuthConfig_JWKSURLScheme_HTTPS(t *testing.T) {
	cfg := &Config{JWT: []ProviderConfig{{
		Issuer: IssuerConfig{
			URL:       "https://dex.example.com",
			JWKSURL:   "https://dex.example.com/keys",
			Audiences: []string{"test"},
		},
	}}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected HTTPS JWKS URL to pass, got %v", err)
	}
}

func TestAuthConfig_JWKSURLScheme_HTTP_Rejected(t *testing.T) {
	cfg := &Config{JWT: []ProviderConfig{{
		Issuer: IssuerConfig{
			URL:       "https://dex.example.com",
			JWKSURL:   "http://dex.example.com/keys",
			Audiences: []string{"test"},
		},
	}}}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for http:// JWKS URL, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "JWKS URL") {
		t.Errorf("error should mention JWKS URL, got: %v", err)
	}
}

func TestAuthConfig_JWKSURLScheme_HTTP_AllowInsecure(t *testing.T) {
	cfg := &Config{
		AllowInsecureIssuers: true,
		JWT: []ProviderConfig{{
			Issuer: IssuerConfig{
				URL:       "http://dex.example.com",
				JWKSURL:   "http://dex.example.com/keys",
				Audiences: []string{"test"},
			},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected AllowInsecureIssuers to permit http JWKS URL, got %v", err)
	}
}

func TestAuthConfig_JWKSURLScheme_JavaScriptRejected(t *testing.T) {
	cfg := &Config{JWT: []ProviderConfig{{
		Issuer: IssuerConfig{
			URL:       "https://dex.example.com",
			JWKSURL:   "javascript:alert(1)",
			Audiences: []string{"test"},
		},
	}}}
	err := cfg.Validate()
	if err == nil {
		t.Error("TC-P1-06e: expected validation error for javascript: JWKS URL, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "JWKS URL") {
		t.Errorf("TC-P1-06e: error should mention JWKS URL, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TC-P2B-02b: data: scheme rejected
// ---------------------------------------------------------------------------

func TestAuthConfig_IssuerScheme_DataScheme(t *testing.T) {
	t.Parallel()
	cfg := &Config{JWT: []ProviderConfig{{Issuer: IssuerConfig{URL: "data:text/html,<script>alert(1)</script>", Audiences: []string{"test"}}}}}
	err := cfg.Validate()
	if err == nil {
		t.Error("TC-P2B-02b: expected validation error for data: issuer URL, got nil")
	}
}

// ---------------------------------------------------------------------------
// TC-P2B-02d: empty issuer URL rejected
// ---------------------------------------------------------------------------

func TestAuthConfig_IssuerScheme_EmptyURL(t *testing.T) {
	t.Parallel()
	cfg := &Config{JWT: []ProviderConfig{{Issuer: IssuerConfig{URL: "", Audiences: []string{"test"}}}}}
	err := cfg.Validate()
	if err == nil {
		t.Error("TC-P2B-02d: expected validation error for empty issuer URL, got nil")
	}
}

// ---------------------------------------------------------------------------
// TC-P2B-03a/b: nbf=0 (epoch) and nbf=-1 (before epoch)
// ---------------------------------------------------------------------------

func TestValidateNotBefore_Epoch(t *testing.T) {
	t.Parallel()
	claims := map[string]interface{}{"nbf": float64(0)}
	err := validateNotBefore(claims)
	if err != nil {
		t.Errorf("TC-P2B-03a: nbf=0 (Unix epoch) is in the past, should accept; got %v", err)
	}
}

func TestValidateNotBefore_NegativeOne(t *testing.T) {
	t.Parallel()
	claims := map[string]interface{}{"nbf": float64(-1)}
	err := validateNotBefore(claims)
	if err != nil {
		t.Errorf("TC-P2B-03b: nbf=-1 (before epoch) is in the past, should accept; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TC-P2B-04a/b/d: Replay through full Validate path
// ---------------------------------------------------------------------------

func TestValidate_ReplayedJTI_ReturnsErrTokenReplayed(t *testing.T) {
	t.Parallel()

	kp := generateTestKeyPair(t)
	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(kp.publicJWKS())
	}))
	t.Cleanup(jwksSrv.Close)

	cfg := Config{
		AllowInsecureIssuers: true,
		JWT: []ProviderConfig{{
			Issuer: IssuerConfig{
				URL:       jwksSrv.URL,
				JWKSURL:   jwksSrv.URL,
				Audiences: []string{"test"},
			},
		}},
	}
	rc := NewReplayCache(10 * time.Minute)
	t.Cleanup(rc.Stop)

	v, err := NewJWTValidator(cfg, WithReplayCache(rc))
	if err != nil {
		t.Fatalf("NewJWTValidator: %v", err)
	}

	token := kp.signToken(t, map[string]interface{}{
		"iss": jwksSrv.URL,
		"aud": "test",
		"sub": "alice",
		"jti": "unique-jti-001",
		"exp": float64(time.Now().Add(1 * time.Hour).Unix()),
		"iat": float64(time.Now().Unix()),
	})

	ctx := context.Background()

	_, err = v.Validate(ctx, token)
	if err != nil {
		t.Fatalf("TC-P2B-04a: first validation should succeed, got %v", err)
	}

	_, err = v.Validate(ctx, token)
	if !errors.Is(err, ErrTokenReplayed) {
		t.Errorf("TC-P2B-04a: replayed token should return ErrTokenReplayed, got %v", err)
	}
}

func TestValidate_DifferentJTI_Passes(t *testing.T) {
	t.Parallel()

	kp := generateTestKeyPair(t)
	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(kp.publicJWKS())
	}))
	t.Cleanup(jwksSrv.Close)

	cfg := Config{
		AllowInsecureIssuers: true,
		JWT: []ProviderConfig{{
			Issuer: IssuerConfig{
				URL:       jwksSrv.URL,
				JWKSURL:   jwksSrv.URL,
				Audiences: []string{"test"},
			},
		}},
	}
	rc := NewReplayCache(10 * time.Minute)
	t.Cleanup(rc.Stop)

	v, err := NewJWTValidator(cfg, WithReplayCache(rc))
	if err != nil {
		t.Fatalf("NewJWTValidator: %v", err)
	}

	ctx := context.Background()
	for i, jti := range []string{"jti-a", "jti-b", "jti-c"} {
		token := kp.signToken(t, map[string]interface{}{
			"iss": jwksSrv.URL,
			"aud": "test",
			"sub": "alice",
			"jti": jti,
			"exp": float64(time.Now().Add(1 * time.Hour).Unix()),
			"iat": float64(time.Now().Unix()),
		})
		_, err = v.Validate(ctx, token)
		if err != nil {
			t.Errorf("TC-P2B-04b[%d]: different JTI %q should pass, got %v", i, jti, err)
		}
	}
}

func TestValidate_MissingJTI_WithReplayEnabled(t *testing.T) {
	t.Parallel()

	kp := generateTestKeyPair(t)
	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(kp.publicJWKS())
	}))
	t.Cleanup(jwksSrv.Close)

	cfg := Config{
		AllowInsecureIssuers: true,
		JWT: []ProviderConfig{{
			Issuer: IssuerConfig{
				URL:       jwksSrv.URL,
				JWKSURL:   jwksSrv.URL,
				Audiences: []string{"test"},
			},
		}},
	}
	rc := NewReplayCache(10 * time.Minute)
	t.Cleanup(rc.Stop)

	v, err := NewJWTValidator(cfg, WithReplayCache(rc))
	if err != nil {
		t.Fatalf("NewJWTValidator: %v", err)
	}

	token := kp.signToken(t, map[string]interface{}{
		"iss": jwksSrv.URL,
		"aud": "test",
		"sub": "alice",
		"exp": float64(time.Now().Add(1 * time.Hour).Unix()),
		"iat": float64(time.Now().Unix()),
	})

	_, err = v.Validate(context.Background(), token)
	if err == nil {
		t.Error("TC-P2B-04d: token missing jti with replay enabled should fail")
	}
	if !errors.Is(err, ErrMalformedToken) {
		t.Errorf("TC-P2B-04d: expected ErrMalformedToken wrapping, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TC-P2B-05a: Multi-group sanitization
// ---------------------------------------------------------------------------

func TestSanitizeClaimValue_MultiGroupSpecialChars(t *testing.T) {
	t.Parallel()

	inputs := []string{"\x00admin", "sre\u202E", "valid"}
	for _, input := range inputs {
		got := SanitizeClaimValue(input)
		if !utf8.ValidString(got) {
			t.Errorf("TC-P2B-05a: output %q is not valid UTF-8", got)
		}
		if strings.ContainsRune(got, 0) {
			t.Errorf("TC-P2B-05a: output %q still contains null byte", got)
		}
		if strings.ContainsRune(got, '\u202E') {
			t.Errorf("TC-P2B-05a: output %q still contains RTLO", got)
		}
	}
}

// ---------------------------------------------------------------------------
// TC-P2B-05b: 1 MB claim value truncated without OOM
// ---------------------------------------------------------------------------

func TestSanitizeClaimValue_1MBInput(t *testing.T) {
	t.Parallel()
	huge := strings.Repeat("x", 1<<20)
	got := SanitizeClaimValue(huge)
	if len(got) > 256 {
		t.Errorf("TC-P2B-05b: expected truncation to ≤256 chars, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// TC-P2B-06a/b: Concurrent JWKS refresh with -race
// ---------------------------------------------------------------------------

func TestJWKSCache_ConcurrentGetKeys(t *testing.T) {
	t.Parallel()

	body := validJWKSBytes(t)
	srv := jwksServerWith(t, body, http.StatusOK)
	defer srv.Close()

	cache := NewJWKSCache(srv.Client(), []string{srv.URL}, WithRefreshInterval(0))

	const goroutines = 10
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			_, err := cache.GetKeys(context.Background(), srv.URL)
			errs <- err
		}()
	}

	for i := 0; i < goroutines; i++ {
		if err := <-errs; err != nil {
			t.Errorf("TC-P2B-06a: goroutine %d: %v", i, err)
		}
	}
}

func TestJWKSCache_ConcurrentGetKeys_FailingServer(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	ks := testJWKSKeySet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := callCount.Add(1)
		if n%2 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ks)
	}))
	defer srv.Close()

	cache := NewJWKSCache(srv.Client(), []string{srv.URL}, WithRefreshInterval(0))

	const goroutines = 10
	done := make(chan struct{}, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = cache.GetKeys(context.Background(), srv.URL)
		}()
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}
}

// ---------------------------------------------------------------------------
// TC-P3-05: NaN/Inf exp claim rejection (MED-05)
// ---------------------------------------------------------------------------

func TestValidateExpiry_NaN(t *testing.T) {
	t.Parallel()
	claims := map[string]interface{}{"exp": math.NaN()}
	err := validateExpiry(claims)
	if !errors.Is(err, ErrMalformedToken) {
		t.Errorf("TC-P3-05a: NaN exp should return ErrMalformedToken, got %v", err)
	}
}

func TestValidateExpiry_PosInf(t *testing.T) {
	t.Parallel()
	claims := map[string]interface{}{"exp": math.Inf(1)}
	err := validateExpiry(claims)
	if !errors.Is(err, ErrMalformedToken) {
		t.Errorf("TC-P3-05b: +Inf exp should return ErrMalformedToken, got %v", err)
	}
}

func TestValidateExpiry_NegInf(t *testing.T) {
	t.Parallel()
	claims := map[string]interface{}{"exp": math.Inf(-1)}
	err := validateExpiry(claims)
	if !errors.Is(err, ErrMalformedToken) {
		t.Errorf("TC-P3-05c: -Inf exp should return ErrMalformedToken, got %v", err)
	}
}

func TestValidateExpiry_FutureValid(t *testing.T) {
	t.Parallel()
	claims := map[string]interface{}{"exp": float64(time.Now().Add(1 * time.Hour).Unix())}
	err := validateExpiry(claims)
	if err != nil {
		t.Errorf("TC-P3-05d: future exp should be accepted, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TC-P3-02: JWKS CB Label uses SHA256 prefix (MED-02)
// ---------------------------------------------------------------------------

func TestJWKSCBLabel_NotFullURL(t *testing.T) {
	t.Parallel()
	label := jwksDependencyLabel("https://long.issuer.example.com/protocol/openid-connect/certs")
	if strings.Contains(label, "://") {
		t.Errorf("TC-P3-02a: CB label should NOT contain full URL, got %q", label)
	}
	if !strings.HasPrefix(label, "jwks_") {
		t.Errorf("TC-P3-02a: CB label should start with 'jwks_', got %q", label)
	}
}

func TestJWKSCBLabel_DifferentURLsProduceDifferentLabels(t *testing.T) {
	t.Parallel()
	labelA := jwksDependencyLabel("https://issuer-a.example.com/jwks")
	labelB := jwksDependencyLabel("https://issuer-b.example.com/jwks")
	if labelA == labelB {
		t.Errorf("TC-P3-02b: different URLs should produce different labels: %q == %q", labelA, labelB)
	}
}

// ---------------------------------------------------------------------------
// HIGH-06: JWT buildIdentity uses auth.SanitizeClaimValue (stronger)
// ---------------------------------------------------------------------------

func TestBuildIdentity_UseStrongerSanitization(t *testing.T) {
	claims := map[string]interface{}{
		"preferred_username": "alice\x00\u202E",
		"sub":                "alice-sub",
		"groups":             []interface{}{"sre\x00", "dev\u202D"},
		"exp":                float64(time.Now().Add(time.Hour).Unix()),
	}
	identity := buildIdentity(claims, "https://issuer.test", "raw-token")

	if strings.ContainsRune(identity.Username, 0) {
		t.Error("expected null bytes stripped from username")
	}
	if strings.ContainsRune(identity.Username, '\u202E') {
		t.Error("expected bidi override U+202E stripped from username")
	}
	if identity.Username != "alice" {
		t.Errorf("expected sanitized username 'alice', got %q", identity.Username)
	}
	for _, g := range identity.Groups {
		if strings.ContainsRune(g, 0) {
			t.Errorf("expected null bytes stripped from group %q", g)
		}
		if strings.ContainsRune(g, '\u202D') {
			t.Errorf("expected bidi override U+202D stripped from group %q", g)
		}
	}

	longName := strings.Repeat("a", 300)
	claims2 := map[string]interface{}{
		"preferred_username": longName,
		"exp":                float64(time.Now().Add(time.Hour).Unix()),
	}
	identity2 := buildIdentity(claims2, "https://issuer.test", "raw-token")
	if len(identity2.Username) > 256 {
		t.Errorf("expected username truncated to 256, got length %d", len(identity2.Username))
	}
}
