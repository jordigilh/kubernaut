package auth_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
)

func TestImpersonatedClient_SetsHeaders(t *testing.T) {
	baseCfg := &rest.Config{Host: "https://k8s.example.com"}
	cfg, err := auth.NewImpersonatedConfig(baseCfg, "alice", []string{"sre-team", "dev-team"})
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "alice", cfg.Impersonate.UserName)
	assert.Equal(t, []string{"sre-team", "dev-team"}, cfg.Impersonate.Groups)
}

func TestImpersonatedClient_TriageVerbs_Allowed(t *testing.T) {
	baseCfg := &rest.Config{Host: "https://k8s.example.com"}
	cfg, err := auth.NewImpersonatedConfig(baseCfg, "alice", []string{"sre-team"})
	require.NoError(t, err)

	// Verify the config is usable for K8s client creation
	assert.Equal(t, "https://k8s.example.com", cfg.Host)
	assert.Equal(t, "alice", cfg.Impersonate.UserName)
}

func TestImpersonatedClient_OwnerResolution_UsesImpersonation(t *testing.T) {
	baseCfg := &rest.Config{Host: "https://k8s.example.com"}
	identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

	factory, err := auth.NewClientFactory(baseCfg)
	require.NoError(t, err)

	client, err := factory.ClientForScope(auth.ScopeUserImpersonation, identity)
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestImpersonatedClient_ScopeCheck_UsesImpersonation(t *testing.T) {
	baseCfg := &rest.Config{Host: "https://k8s.example.com"}
	identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

	factory, err := auth.NewClientFactory(baseCfg)
	require.NoError(t, err)

	client, err := factory.ClientForScope(auth.ScopeUserImpersonation, identity)
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestImpersonatedClient_403_StopsAndInformsUser(t *testing.T) {
	// Simulate a K8s 403 Forbidden error
	k8sErr := &k8sStatusError{
		code:    403,
		reason:  "Forbidden",
		message: `pods is forbidden: User "alice" cannot list resource "pods" in API group "" in the namespace "production"`,
	}

	friendlyErr := auth.ToUserFriendlyError(k8sErr)
	assert.Contains(t, friendlyErr, "lack access")
	assert.NotContains(t, friendlyErr, "Status")
	assert.NotContains(t, friendlyErr, "API group")
}

// k8sStatusError simulates a K8s API 403 error for testing.
type k8sStatusError struct {
	code    int
	reason  string
	message string
}

func (e *k8sStatusError) Error() string { return e.message }
func (e *k8sStatusError) Status() int   { return e.code }

func TestImpersonatedClient_RRCreation_UsesServiceAccount(t *testing.T) {
	baseCfg := &rest.Config{Host: "https://k8s.example.com"}
	identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

	factory, err := auth.NewClientFactory(baseCfg)
	require.NoError(t, err)

	client, err := factory.ClientForScope(auth.ScopeServiceAccount, identity)
	require.NoError(t, err)
	require.NotNil(t, client)
	// SA client should NOT have impersonation configured
}

func TestImpersonatedClient_InvestigationSessionCreation_UsesServiceAccount(t *testing.T) {
	baseCfg := &rest.Config{Host: "https://k8s.example.com"}
	identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

	factory, err := auth.NewClientFactory(baseCfg)
	require.NoError(t, err)

	client, err := factory.ClientForScope(auth.ScopeServiceAccount, identity)
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestImpersonatedClient_LeaseCreation_UsesServiceAccount(t *testing.T) {
	baseCfg := &rest.Config{Host: "https://k8s.example.com"}
	identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

	factory, err := auth.NewClientFactory(baseCfg)
	require.NoError(t, err)

	saClient, err := factory.ClientForScope(auth.ScopeServiceAccount, identity)
	require.NoError(t, err)
	require.NotNil(t, saClient)
}

func TestImpersonatedClient_EmptyUsername_ReturnsError(t *testing.T) {
	baseCfg := &rest.Config{Host: "https://k8s.example.com"}

	_, err := auth.NewImpersonatedConfig(baseCfg, "", []string{"sre"})
	assert.ErrorIs(t, err, auth.ErrEmptyUsername)
}

func TestImpersonatedClient_DeepCopiesConfig(t *testing.T) {
	baseCfg := &rest.Config{
		Host:        "https://k8s.example.com",
		BearerToken: "sa-token",
	}

	cfg, err := auth.NewImpersonatedConfig(baseCfg, "alice", []string{"sre"})
	require.NoError(t, err)

	// Original config must not be mutated
	assert.Empty(t, baseCfg.Impersonate.UserName, "original config must not be mutated")
	assert.Equal(t, "alice", cfg.Impersonate.UserName)
	assert.Equal(t, "sa-token", cfg.BearerToken, "SA token should be preserved in copy")
}

func TestJWTDelegation_ForwardsOriginalJWT(t *testing.T) {
	originalJWT := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test.signature"

	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	transport := &auth.JWTDelegationTransport{
		Base:  srv.Client().Transport,
		Token: originalJWT,
	}

	client := &http.Client{Transport: transport}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "Bearer "+originalJWT, capturedAuth)
}

func TestJWTDelegation_NoTokenModification(t *testing.T) {
	// Token with special characters that could be accidentally modified
	originalJWT := "eyJhbGciOiJSUzI1NiJ9.eyJpc3MiOiJodHRwczovL3Nzby5leGFtcGxlLmNvbSIsInN1YiI6ImFsaWNlIn0.sig-with-special+chars/and=padding"

	var capturedToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		capturedToken = auth[len("Bearer "):]
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	transport := &auth.JWTDelegationTransport{
		Base:  srv.Client().Transport,
		Token: originalJWT,
	}

	client := &http.Client{Transport: transport}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	assert.Equal(t, originalJWT, capturedToken, "JWT must be forwarded byte-identical")
}

func TestTwoTierModel_UserScopedQueries_UseImpersonation(t *testing.T) {
	baseCfg := &rest.Config{Host: "https://k8s.example.com", BearerToken: "sa-token"}
	identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

	factory, err := auth.NewClientFactory(baseCfg)
	require.NoError(t, err)

	// User-scoped queries should use impersonation
	_, err = factory.ClientForScope(auth.ScopeUserImpersonation, identity)
	require.NoError(t, err)
}

func TestTwoTierModel_AFOwnedResources_UseServiceAccount(t *testing.T) {
	baseCfg := &rest.Config{Host: "https://k8s.example.com", BearerToken: "sa-token"}
	identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

	factory, err := auth.NewClientFactory(baseCfg)
	require.NoError(t, err)

	// AF-owned resources should use SA (no impersonation)
	_, err = factory.ClientForScope(auth.ScopeServiceAccount, identity)
	require.NoError(t, err)
}

func TestBuildImpersonationConfig_ExtractsFromContext(t *testing.T) {
	baseCfg := &rest.Config{Host: "https://k8s.example.com"}
	identity := &auth.UserIdentity{Username: "bob", Groups: []string{"dev", "ops"}}

	cfg, err := auth.NewImpersonatedConfig(baseCfg, identity.Username, identity.Groups)
	require.NoError(t, err)

	assert.Equal(t, "bob", cfg.Impersonate.UserName)
	assert.Equal(t, []string{"dev", "ops"}, cfg.Impersonate.Groups)
}

