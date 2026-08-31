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

package infrastructure

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// keycloakImage is the Keycloak image used for the FMC E2E lane's OIDC +
// RFC 8693 token-exchange provider. Pinned (not :latest) since Keycloak's
// admin REST API and Standard Token Exchange (v2) behavior are sensitive to
// minor version changes -- Spike S17/S18 validated this exact behavior
// empirically against the 26.x line.
const keycloakImage = "quay.io/keycloak/keycloak:26.6.4"

// idpNamespace hosts Keycloak for the fleet DEMO entry point only
// (SetupFleetCoreInfrastructure / `make setup-e2e-fleet-infra`) -- NOT the
// "fleet"/"fullpipeline" Ginkgo suites (provisionFleetCoreInfra's other
// caller), which deliberately keep Keycloak in kubernautSystem unchanged.
// Same platform-infra-is-not-app-infra reasoning as kube-mcp-server's
// "mcp-system" and the monitoring stack's "monitoring" moves: in production
// the IdP is its own namespace, not bundled into the app's.
//
// This is intentionally scoped to the demo path only (user decision,
// 2026-08-30): Keycloak is reached via two different hostnames depending on
// caller (browser, via a static /etc/hosts "keycloak" -> 127.0.0.1 entry;
// in-cluster pods, via real CoreDNS), and splitting it across namespaces
// means oauth2-proxy's OIDC discovery needs a matching frontend/backend URL
// split (console.oauth2Proxy.skipDiscovery et al., see console.yaml) --
// validated empirically against oauth2-proxy v7.15.3's source
// (provider_verifier.go: SkipDiscovery bypasses the discovery HTTP call
// entirely, so there's no issuer-mismatch risk between the two hostnames).
// Touching the shared Ginkgo-suite code path for this would need dedicated
// CI validation the user didn't want to risk right now.
const idpNamespace = "idp"

// keycloakRealmFleetJSON is the kubernaut-fleet realm export. Notably, the
// "k8s-api-audience" client-scope carries a "preferred_username" mapper
// (User Property: username) in addition to its audience mapper: service-
// account (client_credentials) tokens in this realm's minimal --import-realm
// bootstrap otherwise carry NO preferred_username claim at all -- confirmed
// empirically, the exchanged token only has iss/aud/sub/typ/azp/scope
// without it. The Kubernetes API server's OIDC authenticator
// (--oidc-username-claim=preferred_username, see patchAPIServerForOIDCConfig
// call site in fleetmetadatacache_e2e.go) needs that claim just to
// authenticate the exchanged identity at all -- a missing claim is a 401,
// not merely an authorization failure.
//
// This is a hand-authored partial realm, not a real Keycloak admin-console
// export -- so it must explicitly define the "profile"/"email"/"roles"/
// "web-origins" clientScopes that a real export always bakes in. Keycloak's
// --import-realm does NOT auto-seed those standard scopes; referencing them
// by name in a client's defaultClientScopes without a matching clientScopes
// entry gets silently dropped at import (a WARN log, not a failure). This
// went unnoticed for kubernaut-fleet-read/kube-mcp-server -- their
// client_credentials tokens (GetKeycloakClientCredentialsToken) only ever
// request the audience scope, never "profile"/"email" -- until Console's
// oauth2-proxy, the first real Authorization Code browser flow against this
// realm, hit it: oauth2-proxy's default --scope is "openid email profile",
// and Keycloak's authorize endpoint 403s with error=invalid_scope on any
// requested scope name that isn't a real clientScope. Confirmed via manual
// fleet E2E QE run, 2026-08-29. Keep each clientScope's "description" under
// H2's 255-char DESCRIPTION column limit (same constraint that broke the
// "groups" scope earlier) -- Keycloak's own import batches ALL clientScope
// upserts together, so one oversized description fails the whole import.
//
// The realm's ssoSessionMaxLifespan (604800s/7d, up from Keycloak's own
// 36000s/10h default) plus "kubernaut-fleet-read"'s access.token.lifespan
// AND client.session.max.lifespan attributes (also 604800s/7d) are a
// deliberate three-part override: access.token.lifespan alone is silently
// capped by the client-session-max-lifespan ceiling for client_credentials
// grants (every access token is tied to a client session), and
// client.session.max.lifespan itself can't exceed the realm's
// ssoSessionMaxLifespan (Keycloak 400s "Client session max lifespan cannot
// exceed realm SSO session max lifespan" otherwise) -- confirmed
// empirically, 2026-08-29, in that order (each fix exposed the next
// ceiling). This client mints the
// static Bearer token embedded into the Kuadrant broker's
// kube-mcp-server-broker-cred Secret (DeployFleetGatewayInfra), which the
// broker holds in memory for its whole process lifetime with no built-in
// refresh. Confirmed via manual fleet E2E QE run, 2026-08-29: once that
// token expired at the 1h realm default, restarting the broker Deployment
// to force a refresh did NOT help -- Kuadrant v0.7.1's controller doesn't
// re-resolve credentialRef on a Secret change or pod restart (it caches the
// resolved token in a derived mcp-gateway-config Secret), and a bare broker
// restart instead pushed the MCPGatewayExtension/broker pair into a
// circular readiness deadlock (Extension waits on broker Ready; broker's
// credential resolution waits on Extension Ready) that only a full
// teardown+re-apply of the mcp-system/gateway-system stack recovered from.
// A long-lived token sidesteps needing any hot-refresh mechanism for this
// specific client; this is a local/E2E-only realm so the extended lifespan
// carries no production security exposure.
//
//go:embed keycloak-realm-fleet.json
var keycloakRealmFleetJSON string

// KeycloakFleetTokenConfig holds configuration for obtaining a
// client_credentials token from Keycloak for fleet service-to-service
// authentication.
type KeycloakFleetTokenConfig struct {
	TokenEndpoint  string       // e.g. https://localhost:30557/realms/kubernaut-fleet/protocol/openid-connect/token
	ClientID       string       // e.g. kubernaut-fleet-read
	ClientSecret   string       // e.g. e2e-fleet-secret
	Scopes         []string     // e.g. ["kube-mcp-server-audience"]
	HTTPClient     *http.Client // optional TLS-aware client override; if nil, one is built from KubeconfigPath
	KubeconfigPath string       // locates the inter-service CA (GenerateInterServiceTLS) that signed Keycloak's leaf cert; required when HTTPClient is nil
}

// DefaultKeycloakFleetReadConfig returns the default Keycloak fleet-read
// client config matching the kubernaut-fleet-read client declared in
// keycloak-realm-fleet.json. kubeconfigPath locates the inter-service CA
// (GenerateInterServiceTLS) so the client keycloakHTTPClient builds verifies
// Keycloak's certificate instead of skipping verification.
func DefaultKeycloakFleetReadConfig(hostPort int, kubeconfigPath string) KeycloakFleetTokenConfig {
	return KeycloakFleetTokenConfig{
		TokenEndpoint:  fmt.Sprintf("https://localhost:%d/realms/kubernaut-fleet/protocol/openid-connect/token", hostPort),
		ClientID:       "kubernaut-fleet-read",
		ClientSecret:   "e2e-fleet-secret",
		KubeconfigPath: kubeconfigPath,
	}
}

// GetKeycloakClientCredentialsToken obtains an access_token from Keycloak
// using the OAuth2 client_credentials grant.
func GetKeycloakClientCredentialsToken(ctx context.Context, cfg KeycloakFleetTokenConfig) (string, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	}
	if len(cfg.Scopes) > 0 {
		data.Set("scope", strings.Join(cfg.Scopes, " "))
	}

	client, err := keycloakHTTPClient(cfg.HTTPClient, cfg.KubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to build Keycloak HTTP client: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to build keycloak client_credentials token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("keycloak client_credentials token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read Keycloak token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("keycloak token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parse Keycloak token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("keycloak token response missing access_token: %s", string(body))
	}

	return tokenResp.AccessToken, nil
}

// ExchangeKeycloakToken performs an RFC 8693 Standard Token Exchange against
// Keycloak, exactly as kube-mcp-server's passthrough+STS auth mode does
// internally (pkg/kubernetes/sts.go, ExternalAccountTokenExchange) -- see
// Spike S17/S18. Tests use this to drive the real exchange directly and
// confirm the resulting token is honored by the real Kubernetes API server
// (E2E-FMC-054-014), rather than only proving it indirectly through FMC's
// sync journey.
//
// kubeconfigPath locates the inter-service CA (GenerateInterServiceTLS) that
// signed Keycloak's leaf cert, so the request verifies Keycloak's
// certificate instead of skipping verification. subjectToken is the
// caller's own access token (e.g. FMC's client_credentials token);
// requesterClientID/requesterClientSecret identify the party performing the
// exchange (kube-mcp-server); audience is the requested token's target
// audience (e.g. "k8s-api").
//
// subject_token_type is hardcoded to the standard OAuth2 access-token URN:
// Spike S18 found Keycloak rejects the exchange with "invalid_request:
// Parameter 'subject_token_type' required for standard token exchange" if
// this is omitted.
func ExchangeKeycloakToken(kubeconfigPath, tokenEndpoint, requesterClientID, requesterClientSecret, subjectToken, audience string) (string, error) {
	data := url.Values{
		"grant_type":         {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"client_id":          {requesterClientID},
		"client_secret":      {requesterClientSecret},
		"subject_token":      {subjectToken},
		"subject_token_type": {"urn:ietf:params:oauth:token-type:access_token"},
		"audience":           {audience},
	}

	client, err := keycloakHTTPClient(nil, kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to build Keycloak HTTP client: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to build keycloak token-exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("keycloak token-exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read Keycloak token-exchange response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("keycloak token-exchange endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parse Keycloak token-exchange response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("keycloak token-exchange response missing access_token: %s", string(body))
	}

	return tokenResp.AccessToken, nil
}

// DeployKeycloakInfra deploys Keycloak (with the kubernaut-fleet realm
// pre-imported) and waits for it to be ready. This is the exported entry
// point for the FMC E2E lane, replacing DeployDexInfra so that kube-mcp-server
// passthrough + RFC 8693 token exchange can be validated against a real
// Keycloak provider (Spike S17/S18; Dex does not implement RFC 8693 Standard
// Token Exchange).
//
// hostPort must match the Kind extraPortMappings host port for the Keycloak
// NodePort (30557) in the caller's Kind config -- see waitForKeycloakReady.
//
// persistent gates BR-PLATFORM-014's demo-only PersistentVolumeClaim (see
// deployKeycloakInNamespace) -- false for the FMC and Ginkgo "fleet"/
// "fullpipeline" suites, which are short-lived and unaffected by state loss
// on restart; true for the demo entry point (SetupFleetCoreInfrastructure),
// where a restart without persistence cascades into every dependent
// service's cached JWKS going stale.
func DeployKeycloakInfra(ctx context.Context, namespace, kubeconfigPath string, hostPort int, persistent bool, writer io.Writer) error {
	if err := deployKeycloakInNamespace(ctx, namespace, kubeconfigPath, persistent, writer); err != nil {
		return err
	}
	return waitForKeycloakReady(ctx, kubeconfigPath, hostPort, writer)
}

// deployKeycloakInNamespace deploys Keycloak as an OIDC provider + RFC 8693
// token-exchange IdP in the Kind cluster for E2E testing. The kubernaut-fleet
// realm (clients, audience-mapper client-scopes) is imported at startup from
// the embedded keycloak-realm-fleet.json -- see that file for the client/scope
// design validated in Spike S18.
//
// start-dev mode is used deliberately in every case (persistent or not):
// start-dev skips the production-mode preflight checks (external DB,
// hostname strictness) that would otherwise add startup latency without
// benefit here. With persistent=false (the FMC and Ginkgo "fleet"/
// "fullpipeline" suites' unchanged behavior), the default dev-file H2
// database lives in the container's writable layer with no
// PersistentVolumeClaim -- fine for throwaway, short-lived E2E infra.
//
// persistent=true (BR-PLATFORM-014, demo entry point only) adds a
// PersistentVolumeClaim mounted at Keycloak's dev-file H2 database path
// (/opt/keycloak/data/h2) so realm state (signing keys, imported realm,
// client secrets) survives pod restarts -- otherwise a restart regenerates
// all of that from the --import-realm fixture, invalidating every
// dependent service's cached JWKS. It also annotates the pod template so
// stakater/Reloader (installed by ensureKeycloakCertManagerIssuer's demo
// call site) auto-restarts Keycloak when cert-manager renews keycloak-tls
// -- safe now that a restart no longer loses state.
func deployKeycloakInNamespace(ctx context.Context, namespace, kubeconfigPath string, persistent bool, writer io.Writer) error {
	var pvcBlock, annotationsBlock, dataVolumeMount, dataVolume string
	if persistent {
		pvcBlock = fmt.Sprintf(`---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: keycloak-data
  namespace: %s
spec:
  accessModes: ["ReadWriteOnce"]
  resources:
    requests:
      storage: 1Gi
`, namespace)
		annotationsBlock = `
      annotations:
        secret.reloader.stakater.com/reload: "keycloak-tls"`
		dataVolumeMount = `
        - name: keycloak-data
          mountPath: /opt/keycloak/data/h2`
		dataVolume = `
      - name: keycloak-data
        persistentVolumeClaim:
          claimName: keycloak-data`
	}

	manifest := fmt.Sprintf(`%[1]s---
apiVersion: v1
kind: ConfigMap
metadata:
  name: keycloak-realm-config
  namespace: %[2]s
data:
  kubernaut-fleet-realm.json: |
%[3]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: keycloak
  namespace: %[2]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: keycloak
  template:
    metadata:
      labels:
        app: keycloak%[4]s
    spec:
      containers:
      - name: keycloak
        image: %[5]s
        args: ["start-dev", "--import-realm"]
        env:
        - name: KC_BOOTSTRAP_ADMIN_USERNAME
          value: "admin"
        - name: KC_BOOTSTRAP_ADMIN_PASSWORD
          value: "admin"
        - name: KC_HTTPS_CERTIFICATE_FILE
          value: /etc/keycloak-tls/tls.crt
        - name: KC_HTTPS_CERTIFICATE_KEY_FILE
          value: /etc/keycloak-tls/tls.key
        - name: KC_HOSTNAME
          value: "https://keycloak:8443"
        - name: KC_HOSTNAME_STRICT_HTTPS
          value: "false"
        ports:
        - name: https
          containerPort: 8443
        volumeMounts:
        - name: realm-config
          mountPath: /opt/keycloak/data/import
          readOnly: true
        - name: tls-certs
          mountPath: /etc/keycloak-tls
          readOnly: true%[6]s
        readinessProbe:
          httpGet:
            path: /realms/master
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 15
          periodSeconds: 5
          failureThreshold: 24
        livenessProbe:
          httpGet:
            path: /realms/master
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 30
          periodSeconds: 10
          failureThreshold: 12
        resources:
          requests:
            memory: "768Mi"
            cpu: "250m"
          limits:
            memory: "1536Mi"
            cpu: "1000m"
      volumes:
      - name: realm-config
        configMap:
          name: keycloak-realm-config
      - name: tls-certs
        secret:
          secretName: keycloak-tls%[7]s
---
apiVersion: v1
kind: Service
metadata:
  name: keycloak
  namespace: %[2]s
spec:
  type: NodePort
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    nodePort: 30557
  selector:
    app: keycloak
`, pvcBlock, namespace, indentPEM(keycloakRealmFleetJSON), annotationsBlock, keycloakImage, dataVolumeMount, dataVolume)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy Keycloak: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Keycloak OIDC provider deployed")

	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for Keycloak pod to be ready (Keycloak startup + realm import is slower than DEX)...")
	waitCmd := exec.CommandContext(ctx, "kubectl", "rollout", "status", "deployment/keycloak",
		"-n", namespace, "--kubeconfig", kubeconfigPath, "--timeout=180s")
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("keycloak deployment rollout failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Keycloak pod ready")
	return nil
}

// waitForKeycloakReady polls the Keycloak realm endpoint via NodePort until it
// responds or the timeout is reached, confirming both that Keycloak is up AND
// that the kubernaut-fleet realm was successfully imported (mirrors
// waitForDexReady).
//
// hostPort is the Kind extraPortMappings host port that maps to the Keycloak
// NodePort (30557) in the running cluster.
func waitForKeycloakReady(ctx context.Context, kubeconfigPath string, hostPort int, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for Keycloak kubernaut-fleet realm to be reachable (HTTPS)...")

	client, err := NewTLSAwareClient(kubeconfigPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to build TLS-aware client for Keycloak health check: %w", err)
	}

	realmURL := fmt.Sprintf("https://localhost:%d/realms/kubernaut-fleet", hostPort)
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, realmURL, http.NoBody)
		if reqErr != nil {
			return fmt.Errorf("failed to build Keycloak realm request: %w", reqErr)
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			_, _ = fmt.Fprintln(writer, "  ✅ Keycloak kubernaut-fleet realm reachable (HTTPS)")
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(3 * time.Second)
	}

	return fmt.Errorf("keycloak kubernaut-fleet realm not responsive after 150 seconds")
}

// keycloakHTTPClient returns the provided client if non-nil, or a client
// that trusts the inter-service CA (GenerateInterServiceTLS) via
// kubeconfigPath for E2E test token endpoints -- Keycloak's leaf cert (SANs
// include "localhost") is signed by that same CA, so this verifies rather
// than skips TLS. Mirrors dexHTTPClient.
func keycloakHTTPClient(c *http.Client, kubeconfigPath string) (*http.Client, error) {
	if c != nil {
		return c, nil
	}
	return NewTLSAwareClient(kubeconfigPath, 10*time.Second)
}

// PreloadKeycloakImage pulls the Keycloak image and loads it into the Kind cluster.
func PreloadKeycloakImage(ctx context.Context, clusterName string, writer io.Writer) error {
	return PreloadExternalImage(ctx, keycloakImage, clusterName, writer)
}
