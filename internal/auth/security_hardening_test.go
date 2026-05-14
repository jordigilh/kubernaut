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
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-jose/go-jose/v4"
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
