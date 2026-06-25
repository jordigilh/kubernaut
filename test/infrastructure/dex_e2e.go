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

// dexImage is the DEX OIDC provider image used for E2E JWT testing.
// master (>= v2.46.0) includes client_credentials grant support
// for service-to-service fleet authentication (BR-INTEGRATION-054).
const dexImage = "ghcr.io/dexidp/dex:v2.46.0"

// DexE2EConfig holds the DEX E2E user credentials for token acquisition.
type DexE2EConfig struct {
	TokenEndpoint string // e.g. http://localhost:5556/dex/token
	ClientID      string
	ClientSecret  string
	Username      string // email for static password user
	Password      string
}

// DexFleetTokenConfig holds configuration for obtaining a client_credentials
// token from DEX for fleet service-to-service authentication.
type DexFleetTokenConfig struct {
	TokenEndpoint string   // e.g. http://localhost:5556/dex/token
	ClientID      string   // e.g. kubernaut-fleet-read
	ClientSecret  string   // e.g. e2e-fleet-secret
	Scopes        []string // e.g. ["openid", "groups"]
}

// DefaultDexE2EConfig returns the default DEX config matching the static
// passwords and clients deployed by deployDexInNamespace.
func DefaultDexE2EConfig() DexE2EConfig {
	return DexE2EConfig{
		TokenEndpoint: "http://localhost:5556/dex/token",
		ClientID:      "kubernaut-agent",
		ClientSecret:  "e2e-client-secret",
		Username:      "e2e-user@kubernaut.test",
		Password:      "e2e-test-password",
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

	resp, err := http.PostForm(cfg.TokenEndpoint, data)
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

// DefaultDexFleetReadConfig returns the default DEX fleet-read client config
// matching the static clients deployed by deployDexInNamespace.
func DefaultDexFleetReadConfig() DexFleetTokenConfig {
	return DexFleetTokenConfig{
		TokenEndpoint: "http://localhost:5556/dex/token",
		ClientID:      "kubernaut-fleet-read",
		ClientSecret:  "e2e-fleet-secret",
		Scopes:        []string{"openid", "groups"},
	}
}

// GetDexClientCredentialsToken obtains an access_token from DEX using the
// OAuth2 client_credentials grant. This is the service-to-service auth flow
// used by fleet services to authenticate to the MCP Gateway.
func GetDexClientCredentialsToken(cfg DexFleetTokenConfig) (string, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"scope":         {strings.Join(cfg.Scopes, " ")},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	}

	resp, err := http.PostForm(cfg.TokenEndpoint, data)
	if err != nil {
		return "", fmt.Errorf("DEX client_credentials token request failed: %w", err)
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
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parse DEX token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("DEX token response missing access_token: %s", string(body))
	}

	return tokenResp.AccessToken, nil
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
    issuer: http://dex:5556/dex
    storage:
      type: memory
    web:
      http: 0.0.0.0:5556
    enablePasswordDB: true
    oauth2:
      passwordConnector: local
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
      - id: kubernaut-fleet-read
        name: 'Kubernaut Fleet Read (E2E)'
        secret: e2e-fleet-secret
        grantTypes:
          - client_credentials
        clientCredentialsClaims:
          groups:
            - mcp-read
      - id: kubernaut-fleet-write
        name: 'Kubernaut Fleet Write (E2E)'
        secret: e2e-fleet-secret
        grantTypes:
          - client_credentials
        clientCredentialsClaims:
          groups:
            - mcp-write
            - mcp-read
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
        env:
        - name: DEX_CLIENT_CREDENTIAL_GRANT_ENABLED_BY_DEFAULT
          value: "true"
        ports:
        - name: http
          containerPort: 5556
        volumeMounts:
        - name: config
          mountPath: /etc/dex
          readOnly: true
        readinessProbe:
          httpGet:
            path: /dex/healthz
            port: 5556
          initialDelaySeconds: 3
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /dex/healthz
            port: 5556
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
---
apiVersion: v1
kind: Service
metadata:
  name: dex
  namespace: %s
spec:
  type: NodePort
  ports:
  - name: http
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
// or the timeout is reached.
func waitForDexReady(writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for DEX OIDC endpoint to be reachable...")

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://localhost:5556/dex/healthz")
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			_, _ = fmt.Fprintln(writer, "  ✅ DEX OIDC endpoint reachable")
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

// PreloadDexImage pulls the DEX image and loads it into the Kind cluster.
func PreloadDexImage(clusterName string, writer io.Writer) error {
	return PreloadExternalImage(dexImage, clusterName, writer)
}
