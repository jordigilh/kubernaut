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
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	demoKeycloakIssuerURL        = "https://keycloak:8443/realms/kubernaut-demo"
	demoKeycloakConsoleLogoutURL = "https://keycloak:8443/realms/kubernaut-demo/protocol/openid-connect/logout?id_token_hint={id_token}"
)

func demoKeycloakJWKSURL() string {
	return fmt.Sprintf("https://keycloak.%s.svc.cluster.local:8443/realms/kubernaut-demo/protocol/openid-connect/certs", idpNamespace)
}

// buildDemoOperatorCROptions is the one user-authored demo contract for both
// topologies. It deliberately leaves operator defaults out of the CR; only
// demo-specific BYO references, endpoints, exposure, and credentials are
// selected here (BR-PLATFORM-014, DD-TEST-015).
func buildDemoOperatorCROptions(opts DemoHelmOptions, fleetOpts *FleetHelmOptions) OperatorCROptions {
	reasoningEnabled, reasoningEffort, reasoningCapabilityOverride := resolveDemoReasoning(opts)
	var reasoning *OperatorLLMReasoning
	if reasoningEnabled || reasoningCapabilityOverride != "" {
		reasoning = &OperatorLLMReasoning{Enabled: reasoningEnabled, Effort: reasoningEffort, CapabilityOverride: reasoningCapabilityOverride}
	}
	crOptions := OperatorCROptions{
		Name: operatorCRName, Namespace: kubernautSystem,
		PostgreSQLHost: "postgresql." + kubernautSystem + ".svc.cluster.local", PostgreSQLSSLMode: "require",
		ValkeyHost:       "valkey." + kubernautSystem + ".svc.cluster.local",
		PostgreSQLSecret: demoPGSecretName, ValkeySecret: demoValkeySecretName,
		LLM: OperatorLLMOptions{
			Provider: opts.LLMProvider, Model: opts.LLMModel, Endpoint: opts.LLMEndpoint,
			CredentialsSecret: demoLLMSecretName, VertexProject: opts.VertexProject,
			VertexLocation: opts.VertexLocation, Reasoning: reasoning,
		},
		TLS:              OperatorTLSOptions{Mode: "DevelopmentSelfSigned", ExtraSANs: []string{"localhost", demoConsoleHost, "keycloak"}},
		PolicyConfigMaps: OperatorPolicyOptions{AIAnalysis: "aianalysis-policy", SignalProcessing: "signalprocessing-policy"},
		Monitoring: OperatorMonitoringOptions{
			PrometheusURL:   "https://prometheus-svc." + monitoringNamespace + ".svc.cluster.local:9090",
			AlertManagerURL: "https://alertmanager-svc." + monitoringNamespace + ".svc.cluster.local:9093",
			TLSSource:       "configMap", TLSCAConfigMapName: monitoringCAConfigMapName,
		},
		GatewayEnabled: opts.Autonomous, APIFrontendEnabled: true, ConsoleEnabled: true,
		ConsoleAuthSecretName: "kubernaut-console-oidc", ConsoleIngressHost: demoConsoleHost,
		ConsoleIngressClass: "traefik", ConsoleTLSSecretName: "console-tls",
		ConsoleProviderLogoutURL: demoKeycloakConsoleLogoutURL,
		IssuerURL:                demoKeycloakIssuerURL, JWKSURL: demoKeycloakJWKSURL(), Audience: "kubernaut-apifrontend",
	}
	if fleetOpts != nil {
		crOptions.Fleet = operatorFleetOptionsFromHelm(fleetOpts)
		crOptions.Monitoring.PrometheusURL = "https://thanos-querier-svc." + monitoringNamespace + ".svc.cluster.local:9090"
	}
	return crOptions
}

// BuildDemoConsoleServiceAliasManifest bridges the pinned operator's static
// Ingress backend name (console) to its generated Service name
// (kubernaut-console). It is demo-owned and never becomes an operator owner.
func BuildDemoConsoleServiceAliasManifest(namespace string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: console
  namespace: %s
  labels:
    app.kubernetes.io/part-of: kubernaut-e2e
    kubernaut.ai/e2e-owner: upstream-2492
spec:
  selector:
    app: kubernaut-console
  ports:
  - name: http
    port: 4180
    targetPort: http
`, namespace)
}

// InstallDemoOperatorApplication deploys the demo through the operator
// bootstrap chart and a user-owned Kubernaut CR. It never installs the
// upstream application chart and does not build images on the normal path.
func InstallDemoOperatorApplication(ctx context.Context, kubeconfigPath, remoteKubeconfigPath, kindClusterName string, fleetOpts *FleetHelmOptions, opts DemoHelmOptions, writer io.Writer) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if err := loadDemoImagesToKind(ctx, kindClusterName, opts, writer); err != nil {
		return err
	}

	reasoningEnabled, reasoningEffort, reasoningCapabilityOverride := resolveDemoReasoning(opts)
	operatorChart, dependencyChart, operatorImage, version, operatorChartVersion, dependencyChartVersion, err := resolveDemoOperatorDeploymentInputs(ctx, opts)
	if err != nil {
		return fmt.Errorf("resolve demo operator artifacts: %w", err)
	}
	relatedImages, err := operatorOperandImagesForDemo(opts, version)
	if err != nil {
		return fmt.Errorf("resolve demo operand images: %w", err)
	}
	resolved, err := ResolveDemoOperatorArtifacts(DemoArtifactResolutionInput{ReleaseLine: os.Getenv("KUBERNAUT_RELEASE_LINE"), Explicit: opts.OperatorArtifacts})
	if err != nil {
		return fmt.Errorf("resolve demo artifact provenance: %w", err)
	}
	releaseLine := strings.TrimSpace(os.Getenv("KUBERNAUT_RELEASE_LINE"))
	if releaseLine == "" {
		releaseLine = version
	}
	demoArtifacts := &DemoArtifactResolutionRecord{
		ReleaseLine: releaseLine, OperatorVersion: operatorChartVersion,
		OperatorRef: resolved.OperatorRef, OperatorImage: operatorImage, OperatorChart: operatorChart,
		DependencyChart: dependencyChart, KubernautVersion: resolved.KubernautVersion, RelatedImages: relatedImages,
	}

	namespace := kubernautSystem
	fixtureDir, err := os.MkdirTemp("", "operator-demo-fixtures-*")
	if err != nil {
		return fmt.Errorf("create demo fixture directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(fixtureDir) }()
	spPolicyFile := opts.SPPolicyFile
	if spPolicyFile == "" {
		spPolicyFile = filepath.Join(fixtureDir, "signal-processing.rego")
		if err := os.WriteFile(spPolicyFile, []byte(demoDefaultSPPolicyRego), 0o600); err != nil {
			return fmt.Errorf("write demo SignalProcessing policy: %w", err)
		}
	}
	aaPolicyFile := opts.AAPolicyFile
	if aaPolicyFile == "" {
		aaPolicyFile = filepath.Join(fixtureDir, "ai-analysis.rego")
		if err := os.WriteFile(aaPolicyFile, []byte(demoDefaultAAPolicyRego), 0o600); err != nil {
			return fmt.Errorf("write demo AIAnalysis policy: %w", err)
		}
	}

	crOptions := buildDemoOperatorCROptions(opts, fleetOpts)

	config := OperatorDeploymentConfig{
		ClusterName: kindClusterName,
		Namespace:   namespace, KubeconfigPath: kubeconfigPath, OperatorChartPath: operatorChart, OperatorChartVersion: operatorChartVersion, DependencyChartPath: dependencyChart, DependencyChartVersion: dependencyChartVersion,
		OperatorImage: operatorImage, RelatedImages: relatedImages,
		PostgreSQLImage: "docker.io/library/postgres:16.4@sha256:e62fbf9d3e2b49816a32c400ed2dba83e3b361e6833e624024309c35d334b412",
		ValkeyImage:     "docker.io/valkey/valkey:8.0.2@sha256:57bcc49c6ade1813ef25206c571b65b66bb0094235ff7fb767941622892297d9",
		CR:              crOptions, MonitoringNamespace: monitoringNamespace, Exposure: opts.Exposure, DiagnosticsDir: strings.TrimSpace(os.Getenv("KUBERNAUT_QUALIFICATION_DIR")),
		FixtureFiles:  OperatorFixtureFiles{LLMCredentials: opts.LLMCredentialsFile, AIAnalysisPolicy: aaPolicyFile, SignalProcessingPolicy: spPolicyFile},
		Provenance:    OperatorArtifactProvenance{Repository: operatorRepository, RequestedRef: opts.OperatorArtifacts.OperatorRef, ImageRef: operatorImage, Profile: "demo-v1.6"},
		DemoArtifacts: demoArtifacts,
	}
	if config.Provenance.RequestedRef == "" {
		config.Provenance.RequestedRef = version
	}
	if resolved.OperatorRef != "" {
		config.Provenance.RequestedRef = resolved.OperatorRef
		if len(resolved.OperatorRef) == 40 {
			config.Provenance.ResolvedSHA = resolved.OperatorRef
		}
	}
	if hostIngressCIDR, ingressErr := KindHostIngressCIDR(ctx, kindClusterName); ingressErr != nil {
		return fmt.Errorf("resolve demo Kind host ingress CIDR: %w", ingressErr)
	} else {
		config.HostIngressCIDR = hostIngressCIDR
	}
	if err := EnsureOperatorDataStorageClientRBAC(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("prepare operator demo RBAC: %w", err)
	}
	if evidence, err := DeployOperatorManagedApplication(ctx, config, writer); err != nil {
		return err
	} else {
		RememberOperatorExposure(kubeconfigPath, evidence)
	}
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, BuildDemoConsoleServiceAliasManifest(namespace)); err != nil {
		return fmt.Errorf("create demo Console Service alias: %w", err)
	}
	if err := waitForServiceEndpointReady(ctx, namespace, kubeconfigPath, "console", 60*time.Second, writer); err != nil {
		return fmt.Errorf("wait for demo Console Service alias: %w", err)
	}

	if err := waitForDataStorageHTTP(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("DataStorage did not become ready after operator reconciliation: %w", err)
	}
	if err := BindAFPersonaRBAC(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to bind AF persona RBAC: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ Kubernaut operator demo is ready")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "  Operator image:     %s\n", operatorImage)
	_, _ = fmt.Fprintf(writer, "  LLM reasoning:      enabled=%t", reasoningEnabled)
	if reasoningEffort == "" {
		_, _ = fmt.Fprintln(writer, " effort=(provider default)")
	} else {
		_, _ = fmt.Fprintf(writer, " effort=%s\n", reasoningEffort)
	}
	if reasoningCapabilityOverride != "" {
		_, _ = fmt.Fprintf(writer, "  Capability override: %s\n", reasoningCapabilityOverride)
	}
	_, _ = fmt.Fprintf(writer, "  Trust the demo CA before browser login: %s\n", InterServiceCAPath(kubeconfigPath))
	_, _ = fmt.Fprintf(writer, "    Browse to: https://%s:%d\n", demoConsoleHost, demoConsolePort)
	_, _ = fmt.Fprintln(writer, "    Login: sre-user / password")
	if opts.Mode == DemoModeFleet {
		_, _ = fmt.Fprintf(writer, "    HUB_KUBECONFIG=%s\n    SPOKE_KUBECONFIG=%s\n", kubeconfigPath, remoteKubeconfigPath)
	} else {
		_, _ = fmt.Fprintf(writer, "    KUBECONFIG=%s\n", kubeconfigPath)
	}
	_, _ = fmt.Fprintln(writer, "  Teardown ownership:")
	_, _ = fmt.Fprintf(writer, "    kubectl delete kubernaut %s -n %s --ignore-not-found\n", operatorCRName, namespace)
	_, _ = fmt.Fprintf(writer, "    helm uninstall kubernaut-dev-bootstrap -n %s --wait\n", namespace)
	_, _ = fmt.Fprintln(writer, "    helm uninstall kubernaut-operator -n kubernaut-operator-system --wait")
	_, _ = fmt.Fprintf(writer, "    kind delete cluster --name %s\n", kindClusterName)
	return nil
}
