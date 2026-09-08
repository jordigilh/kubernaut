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
	"fmt"
	"io"
)

// OIDCConsoleHelmOptions describes the provider-specific OIDC endpoints and
// the optional Console frontend that consume them. It is shared by demo and
// full-pipeline Helm installers; provider lifecycle provisioning remains
// provider-specific.
type OIDCConsoleHelmOptions struct {
	IssuerURL        string
	JWKSURL          string
	Audience         string
	IDPPort          int
	ConsoleEnabled   bool
	ConsoleSecret    string
	ConsoleHost      string
	ConsolePort      int
	SkipDiscovery    bool
	LoginURL         string
	RedeemURL        string
	ConsoleJWKSURL   string
	IngressNamespace string
}

func appendOIDCConsoleHelmArgs(args []string, opts OIDCConsoleHelmOptions) []string {
	args = append(args,
		"--set", "apifrontend.config.auth.issuerURL="+opts.IssuerURL,
		"--set", "apifrontend.config.auth.jwksURL="+opts.JWKSURL,
		"--set", "apifrontend.config.auth.audience="+opts.Audience,
	)
	if opts.IDPPort > 0 {
		args = append(args, "--set", fmt.Sprintf("networkPolicies.idp.port=%d", opts.IDPPort))
	}
	if !opts.ConsoleEnabled {
		return args
	}

	args = append(args,
		"--set", "console.enabled=true",
		"--set", "console.auth.secretName="+opts.ConsoleSecret,
		"--set", "console.ingress.enabled=true",
		"--set", "console.ingress.className=traefik",
		"--set", "console.ingress.host="+opts.ConsoleHost,
		"--set", fmt.Sprintf("console.ingress.port=%d", opts.ConsolePort),
	)
	if opts.IngressNamespace != "" {
		args = append(args, "--set", "networkPolicies.console.ingressNamespaces[0]="+opts.IngressNamespace)
	}
	if !opts.SkipDiscovery {
		return args
	}

	return append(args,
		"--set", "console.oauth2Proxy.skipDiscovery=true",
		"--set", "console.oauth2Proxy.loginURL="+opts.LoginURL,
		"--set", "console.oauth2Proxy.redeemURL="+opts.RedeemURL,
		"--set", "console.oauth2Proxy.jwksURL="+opts.ConsoleJWKSURL,
	)
}

// SetupDemoOIDCInfrastructure provisions the shared browser-authentication
// dependencies used by local and fleet demo installations. Fleet-only MCP
// Gateway, spoke, and API-server OIDC wiring remain outside this helper.
func SetupDemoOIDCInfrastructure(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	appNamespace := kubernautSystem
	if err := provisionInterServiceCA(ctx, kubeconfigPath, appNamespace, writer); err != nil {
		return fmt.Errorf("inter-service CA provisioning failed: %w", err)
	}
	if err := CreateTestNamespace(ctx, idpNamespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create %s namespace for Keycloak: %w", idpNamespace, err)
	}
	if err := InstallCertManager(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("cert-manager installation failed: %w", err)
	}
	if err := WaitForCertManagerReady(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("cert-manager readiness check failed: %w", err)
	}
	if err := installReloader(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("reloader installation failed: %w", err)
	}
	if err := ensureKeycloakCertManagerIssuer(ctx, kubeconfigPath, appNamespace, idpNamespace, writer); err != nil {
		return fmt.Errorf("keycloak TLS provisioning failed: %w", err)
	}
	if err := DeployKeycloakInfra(ctx, idpNamespace, kubeconfigPath, keycloakHostPortDemo, true, writer); err != nil {
		return fmt.Errorf("Keycloak deployment failed: %w", err)
	}
	if err := deployTraefikForKind(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("Traefik installation failed: %w", err)
	}
	return nil
}
