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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// dexImage is the DEX OIDC provider image used for E2E JWT testing
// (KA's CompositeAuthenticator JWT/JWKS/SAR authorization, BR-INTEGRATION-054).
const dexImage = "ghcr.io/dexidp/dex:latest"

// DexE2EConfig holds the DEX E2E user credentials for token acquisition.
type DexE2EConfig struct {
	TokenEndpoint  string // e.g. https://localhost:5556/dex/token
	ClientID       string
	ClientSecret   string
	Username       string // email for static password user
	Password       string
	HTTPClient     *http.Client // optional TLS-aware client override; if nil, one is built from KubeconfigPath
	KubeconfigPath string       // locates the inter-service CA (GenerateInterServiceTLS) that signed DEX's leaf cert; required when HTTPClient is nil
}

// DefaultDexE2EConfig returns the default DEX config matching the static
// passwords and clients deployed by deployDexInNamespace. kubeconfigPath
// locates the inter-service CA (GenerateInterServiceTLS) so the client
// dexHTTPClient builds verifies DEX's certificate instead of skipping
// verification.
func DefaultDexE2EConfig(kubeconfigPath string) DexE2EConfig {
	return DexE2EConfig{
		TokenEndpoint:  "https://localhost:5556/dex/token",
		ClientID:       "kubernaut-agent",
		ClientSecret:   "e2e-client-secret",
		Username:       "e2e-user@kubernaut.test",
		Password:       "e2e-test-password",
		KubeconfigPath: kubeconfigPath,
	}
}

// GetDexIDToken obtains an OIDC id_token from DEX using the Resource Owner
// Password Credentials grant. The id_token is a JWT signed by DEX's keys.
func GetDexIDToken(cfg DexE2EConfig) (string, error) {
	data := url.Values{
		"grant_type":    {"password"},
		"scope":         {"openid email groups profile"},
		"username":      {cfg.Username},
		"password":      {cfg.Password},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	}

	client, err := dexHTTPClient(cfg.HTTPClient, cfg.KubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to build DEX HTTP client: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, cfg.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to build DEX token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("DEX token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read DEX token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DEX token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parse DEX token response: %w", err)
	}
	if tokenResp.IDToken == "" {
		return "", fmt.Errorf("DEX token response missing id_token: %s", string(body))
	}

	return tokenResp.IDToken, nil
}

// deployDexInNamespace deploys DEX as an OIDC provider in the Kind cluster
// for E2E JWT/OIDC testing. DEX is configured with:
//   - Static password user (e2e-user@kubernaut.test)
//   - Static client (kubernaut-agent)
//   - Password grant enabled
//   - In-memory storage (stateless, E2E only)
//
// DD-AUTH-MCP-001 v2.0: Validates full OIDC flow with real JWT issuance.
func deployDexInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// bcrypt hash of "e2e-test-password" at cost 10
	const passwordHash = "$2a$10$aFeIluCoQjRFg1RqPxXLZeBV4vLXyL0PauEX0ZaAaq/afg2mOYsg."

	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: dex-config
  namespace: %s
data:
  config.yaml: |
    issuer: https://dex:5556/dex
    storage:
      type: memory
    web:
      https: 0.0.0.0:5556
      tlsCert: /etc/dex/tls/tls.crt
      tlsKey: /etc/dex/tls/tls.key
    enablePasswordDB: true
    oauth2:
      passwordConnector: local
      grantTypes:
        - authorization_code
        - password
        - client_credentials
        - refresh_token
      responseTypes: ["code", "token", "id_token"]
      skipApprovalScreen: true
    staticPasswords:
      - email: "e2e-user@kubernaut.test"
        hash: "%s"
        username: "e2e-user"
        userID: "e2e-user-001"
    staticClients:
      - id: kubernaut-agent
        redirectURIs:
          - 'http://localhost/callback'
        name: 'Kubernaut Agent E2E'
        secret: e2e-client-secret
        grantTypes:
          - authorization_code
          - password
          - client_credentials
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dex
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dex
  template:
    metadata:
      labels:
        app: dex
    spec:
      containers:
      - name: dex
        image: %s
        command: ["dex", "serve", "/etc/dex/config.yaml"]
        ports:
        - name: https
          containerPort: 5556
        volumeMounts:
        - name: config
          mountPath: /etc/dex
          readOnly: true
        - name: tls-certs
          mountPath: /etc/dex/tls
          readOnly: true
        readinessProbe:
          httpGet:
            path: /dex/healthz
            port: 5556
            scheme: HTTPS
          initialDelaySeconds: 3
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /dex/healthz
            port: 5556
            scheme: HTTPS
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            memory: "32Mi"
            cpu: "25m"
          limits:
            memory: "64Mi"
            cpu: "100m"
      volumes:
      - name: config
        configMap:
          name: dex-config
      - name: tls-certs
        secret:
          secretName: dex-tls
---
apiVersion: v1
kind: Service
metadata:
  name: dex
  namespace: %s
spec:
  type: NodePort
  ports:
  - name: https
    port: 5556
    targetPort: 5556
    nodePort: 30556
  selector:
    app: dex
`, namespace, passwordHash, namespace, dexImage, namespace)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy DEX: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ DEX OIDC provider deployed")

	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for DEX pod to be ready...")
	waitCmd := exec.CommandContext(ctx, "kubectl", "rollout", "status", "deployment/dex",
		"-n", namespace, "--kubeconfig", kubeconfigPath, "--timeout=120s")
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("DEX deployment rollout failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ DEX pod ready")
	return nil
}

// waitForDexReady polls the DEX health endpoint via NodePort until it responds
// or the timeout is reached. Uses a TLS-skipping client because the inter-service
// CA may not yet be available at this point (health check runs during deployment).
//
// hostPort is the Kind extraPortMappings host port that maps to the Dex
// NodePort (30556) in the running cluster. Kind configs are inconsistent here:
// kind-kubernautagent-config.yaml maps host 5556, while kind-fullpipeline-config.yaml
// and kind-fleetmetadatacache-config.yaml map host 30556 directly (matching the
// NodePort number). Callers must pass the value used in their own Kind config.
func waitForDexReady(ctx context.Context, kubeconfigPath string, hostPort int, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for DEX OIDC endpoint to be reachable (HTTPS)...")

	client, err := NewTLSAwareClient(kubeconfigPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to build TLS-aware client for DEX health check: %w", err)
	}

	healthzURL := fmt.Sprintf("https://localhost:%d/dex/healthz", hostPort)
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, healthzURL, http.NoBody)
		if reqErr != nil {
			return fmt.Errorf("failed to build DEX healthz request: %w", reqErr)
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			_, _ = fmt.Fprintln(writer, "  ✅ DEX OIDC endpoint reachable (HTTPS)")
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("DEX health endpoint not responsive after 90 seconds")
}

// createDexUserRBAC creates Kubernetes RBAC that allows the DEX-authenticated
// user (e2e-user@kubernaut.test) to access the Kubernaut Agent API via SAR.
// This mirrors the SA RBAC but for an OIDC user identity.
func createDexUserRBAC(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	rbacYAML := fmt.Sprintf(`---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: dex-e2e-user-ka-access
  namespace: %s
  labels:
    app: kubernaut-agent
    component: e2e-testing
    authorization: dd-auth-mcp-001-v2
rules:
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["kubernaut-agent"]
    verbs: ["create", "get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: dex-e2e-user-ka-access
  namespace: %s
  labels:
    app: kubernaut-agent
    component: e2e-testing
    authorization: dd-auth-mcp-001-v2
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: dex-e2e-user-ka-access
subjects:
  - kind: User
    name: e2e-user@kubernaut.test
    apiGroup: rbac.authorization.k8s.io
`, namespace, namespace)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(rbacYAML)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply DEX user RBAC: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ DEX E2E user RBAC created")
	return nil
}

// dexHTTPClient returns the provided client if non-nil, or a client that
// trusts the inter-service CA (GenerateInterServiceTLS) via kubeconfigPath
// for E2E test token endpoints -- DEX's leaf cert (SANs include "localhost")
// is signed by that same CA, so this verifies rather than skips TLS.
func dexHTTPClient(c *http.Client, kubeconfigPath string) (*http.Client, error) {
	if c != nil {
		return c, nil
	}
	return NewTLSAwareClient(kubeconfigPath, 10*time.Second)
}

// PreloadDexImage pulls the DEX image and loads it into the Kind cluster.
func PreloadDexImage(ctx context.Context, clusterName string, writer io.Writer) error {
	return PreloadExternalImage(ctx, dexImage, clusterName, writer)
}
