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
	"crypto/tls"
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
//go:embed keycloak-realm-fleet.json
var keycloakRealmFleetJSON string

// KeycloakFleetTokenConfig holds configuration for obtaining a
// client_credentials token from Keycloak for fleet service-to-service
// authentication (mirrors DexFleetTokenConfig).
type KeycloakFleetTokenConfig struct {
	TokenEndpoint string       // e.g. https://localhost:30557/realms/kubernaut-fleet/protocol/openid-connect/token
	ClientID      string       // e.g. kubernaut-fleet-read
	ClientSecret  string       // e.g. e2e-fleet-secret
	Scopes        []string     // e.g. ["kube-mcp-server-audience"]
	HTTPClient    *http.Client // optional TLS-aware client; defaults to InsecureSkipVerify for HTTPS
}

// DefaultKeycloakFleetReadConfig returns the default Keycloak fleet-read
// client config matching the kubernaut-fleet-read client declared in
// keycloak-realm-fleet.json.
func DefaultKeycloakFleetReadConfig(hostPort int) KeycloakFleetTokenConfig {
	return KeycloakFleetTokenConfig{
		TokenEndpoint: fmt.Sprintf("https://localhost:%d/realms/kubernaut-fleet/protocol/openid-connect/token", hostPort),
		ClientID:      "kubernaut-fleet-read",
		ClientSecret:  "e2e-fleet-secret",
	}
}

// GetKeycloakClientCredentialsToken obtains an access_token from Keycloak
// using the OAuth2 client_credentials grant (mirrors GetDexClientCredentialsToken).
func GetKeycloakClientCredentialsToken(cfg KeycloakFleetTokenConfig) (string, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	}
	if len(cfg.Scopes) > 0 {
		data.Set("scope", strings.Join(cfg.Scopes, " "))
	}

	client := keycloakHTTPClient(cfg.HTTPClient)
	resp, err := client.PostForm(cfg.TokenEndpoint, data)
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
// subjectToken is the caller's own access token (e.g. FMC's client_credentials
// token); requesterClientID/requesterClientSecret identify the party
// performing the exchange (kube-mcp-server); audience is the requested
// token's target audience (e.g. "k8s-api").
//
// subject_token_type is hardcoded to the standard OAuth2 access-token URN:
// Spike S18 found Keycloak rejects the exchange with "invalid_request:
// Parameter 'subject_token_type' required for standard token exchange" if
// this is omitted.
func ExchangeKeycloakToken(tokenEndpoint, requesterClientID, requesterClientSecret, subjectToken, audience string) (string, error) {
	data := url.Values{
		"grant_type":         {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"client_id":          {requesterClientID},
		"client_secret":      {requesterClientSecret},
		"subject_token":      {subjectToken},
		"subject_token_type": {"urn:ietf:params:oauth:token-type:access_token"},
		"audience":           {audience},
	}

	client := keycloakHTTPClient(nil)
	resp, err := client.PostForm(tokenEndpoint, data)
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
func DeployKeycloakInfra(ctx context.Context, namespace, kubeconfigPath string, hostPort int, writer io.Writer) error {
	if err := deployKeycloakInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return err
	}
	return waitForKeycloakReady(hostPort, writer)
}

// deployKeycloakInNamespace deploys Keycloak as an OIDC provider + RFC 8693
// token-exchange IdP in the Kind cluster for E2E testing. The kubernaut-fleet
// realm (clients, audience-mapper client-scopes) is imported at startup from
// the embedded keycloak-realm-fleet.json -- see that file for the client/scope
// design validated in Spike S18.
//
// start-dev mode is used deliberately: this is throwaway E2E infra (in-memory
// H2 storage, no persistence needed across runs), and start-dev skips the
// production-mode preflight checks (external DB, hostname strictness) that
// would otherwise add startup latency without benefit here.
func deployKeycloakInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: keycloak-realm-config
  namespace: %[1]s
data:
  kubernaut-fleet-realm.json: |
%[2]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: keycloak
  namespace: %[1]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: keycloak
  template:
    metadata:
      labels:
        app: keycloak
    spec:
      containers:
      - name: keycloak
        image: %[3]s
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
          readOnly: true
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
          secretName: keycloak-tls
---
apiVersion: v1
kind: Service
metadata:
  name: keycloak
  namespace: %[1]s
spec:
  type: NodePort
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    nodePort: 30557
  selector:
    app: keycloak
`, namespace, indentPEM(keycloakRealmFleetJSON, 4), keycloakImage)

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
func waitForKeycloakReady(hostPort int, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for Keycloak kubernaut-fleet realm to be reachable (HTTPS)...")

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // G402: health check during deployment
			},
		},
	}

	realmURL := fmt.Sprintf("https://localhost:%d/realms/kubernaut-fleet", hostPort)
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(realmURL)
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

// keycloakHTTPClient returns the provided client if non-nil, or a default
// HTTPS client that skips TLS verification (for E2E test token endpoints
// using self-signed certs). Mirrors dexHTTPClient.
func keycloakHTTPClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // G402: E2E test with self-signed certs
			},
		},
	}
}

// PreloadKeycloakImage pulls the Keycloak image and loads it into the Kind cluster.
func PreloadKeycloakImage(clusterName string, writer io.Writer) error {
	return PreloadExternalImage(keycloakImage, clusterName, writer)
}
