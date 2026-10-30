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

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	demoKeycloakIssuerURL        = "https://keycloak:8443/realms/kubernaut-demo"
	demoKeycloakConsoleLogoutURL = "https://keycloak:8443/realms/kubernaut-demo/protocol/openid-connect/logout?id_token_hint={id_token}"
	demoPostgreSQLImageDefault   = "docker.io/library/postgres:16.4@sha256:e62fbf9d3e2b49816a32c400ed2dba83e3b361e6833e624024309c35d334b412"
	demoValkeyImageDefault       = "docker.io/valkey/valkey:8.0.2@sha256:57bcc49c6ade1813ef25206c571b65b66bb0094235ff7fb767941622892297d9"
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
		ValkeyTLS: &OperatorValkeyTLSOptions{Enabled: true, CASecretName: "valkey-ca", ClientCertSecretName: "valkey-tls-client"},
		LLM: OperatorLLMOptions{
			Provider: opts.LLMProvider, Model: opts.LLMModel, Endpoint: opts.LLMEndpoint,
			CredentialsSecret: demoLLMSecretName, VertexProject: opts.VertexProject,
			VertexLocation: opts.VertexLocation, Reasoning: reasoning,
		},
		TLS:              OperatorTLSOptions{Mode: "manual"},
		PolicyConfigMaps: OperatorPolicyOptions{AIAnalysis: "aianalysis-policy", SignalProcessing: "signalprocessing-policy"},
		Monitoring: OperatorMonitoringOptions{
			PrometheusURL:   "https://prometheus-svc." + monitoringNamespace + ".svc.cluster.local:9090",
			AlertManagerURL: "https://alertmanager-svc." + monitoringNamespace + ".svc.cluster.local:9093",
			TLSCASecretName: monitoringClientCASecretName,
		},
		GatewayEnabled: opts.Autonomous, APIFrontendEnabled: true, ConsoleEnabled: true,
		ConsoleAuthSecretName:    demoConsoleOIDCSecretName,
		ConsoleRedirectURL:       fmt.Sprintf("https://%s:%d/oauth2/callback", demoConsoleHost, DemoHostPort(demoConsolePort)),
		ConsoleProviderLogoutURL: demoKeycloakConsoleLogoutURL,
		IssuerURL:                demoKeycloakIssuerURL, JWKSURL: demoKeycloakJWKSURL(), Audience: "kubernaut-apifrontend",
	}
	if fleetOpts != nil {
		crOptions.Fleet = operatorFleetOptionsFromHelm(fleetOpts)
		crOptions.Monitoring.PrometheusURL = "https://thanos-querier-svc." + monitoringNamespace + ".svc.cluster.local:9090"
	}
	return crOptions
}

// BuildDemoConsoleIngressManifest creates the administrator-owned browser edge
// for the operator-created Console Service. The operator owns only the
// kubernaut-console Service and workloads; this Ingress remains demo-owned.
func BuildDemoConsoleIngressManifest(namespace string) string {
	return fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: kubernaut-console
  namespace: %s
  labels:
    app.kubernetes.io/part-of: kubernaut-e2e
    kubernaut.ai/e2e-owner: upstream-2492
spec:
  ingressClassName: traefik
  tls:
  - hosts:
    - %s
    secretName: console-tls
  rules:
  - host: %s
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: kubernaut-console
            port:
              name: http
`, namespace, demoConsoleHost, demoConsoleHost)
}

func prepareDemoOperatorFixtureFiles(opts DemoHelmOptions, fixtureDir string) (spPolicyFile, aaPolicyFile string, err error) {
	spPolicyFile = opts.SPPolicyFile
	if spPolicyFile == "" {
		spPolicyFile = filepath.Join(fixtureDir, "signal-processing.rego")
		if err := os.WriteFile(spPolicyFile, []byte(demoDefaultSPPolicyRego), 0o600); err != nil {
			return "", "", fmt.Errorf("write demo SignalProcessing policy: %w", err)
		}
	}
	aaPolicyFile = opts.AAPolicyFile
	if aaPolicyFile == "" {
		aaPolicyFile = filepath.Join(fixtureDir, "ai-analysis.rego")
		if err := os.WriteFile(aaPolicyFile, []byte(demoDefaultAAPolicyRego), 0o600); err != nil {
			return "", "", fmt.Errorf("write demo AIAnalysis policy: %w", err)
		}
	}
	return spPolicyFile, aaPolicyFile, nil
}

func buildDemoOperatorDeploymentConfig(ctx context.Context, kubeconfigPath, kindClusterName string, fleetOpts *FleetHelmOptions, opts DemoHelmOptions, fixtureDir string) (OperatorDeploymentConfig, error) {
	operatorChart, dependencyChart, operatorImage, version, operatorChartVersion, dependencyChartVersion, err := resolveDemoOperatorDeploymentInputs(ctx, opts)
	if err != nil {
		return OperatorDeploymentConfig{}, fmt.Errorf("resolve demo operator artifacts: %w", err)
	}
	relatedImages, err := operatorOperandImagesForDemo(opts, version)
	if err != nil {
		return OperatorDeploymentConfig{}, fmt.Errorf("resolve demo operand images: %w", err)
	}
	resolved, err := ResolveDemoOperatorArtifacts(DemoArtifactResolutionInput{ReleaseLine: os.Getenv("KUBERNAUT_RELEASE_LINE"), Explicit: opts.OperatorArtifacts})
	if err != nil {
		return OperatorDeploymentConfig{}, fmt.Errorf("resolve demo artifact provenance: %w", err)
	}
	releaseLine := strings.TrimSpace(os.Getenv("KUBERNAUT_RELEASE_LINE"))
	if releaseLine == "" {
		releaseLine = version
	}
	demoArtifacts := &DemoArtifactResolutionRecord{
		ReleaseLine: releaseLine, OperatorVersion: operatorChartVersion,
		OperatorRef: resolved.OperatorRef, OperatorImage: operatorImage, OperatorChart: operatorChart,
		DependencyChart: dependencyChart, DependencyChartVersion: dependencyChartVersion,
		KubernautVersion: resolved.KubernautVersion, RelatedImages: relatedImages,
	}
	spPolicyFile, aaPolicyFile, err := prepareDemoOperatorFixtureFiles(opts, fixtureDir)
	if err != nil {
		return OperatorDeploymentConfig{}, err
	}

	config := OperatorDeploymentConfig{
		ClusterName: kindClusterName,
		Namespace:   kubernautSystem, KubeconfigPath: kubeconfigPath, OperatorChartPath: operatorChart, OperatorChartVersion: operatorChartVersion, DependencyChartPath: dependencyChart, DependencyChartVersion: dependencyChartVersion,
		OperatorImage: operatorImage, RelatedImages: relatedImages,
		PostgreSQLImage: demoPostgreSQLImageDefault,
		ValkeyImage:     demoValkeyImageDefault,
		CR:              buildDemoOperatorCROptions(opts, fleetOpts), MonitoringNamespace: monitoringNamespace, Exposure: opts.Exposure, DiagnosticsDir: strings.TrimSpace(os.Getenv("KUBERNAUT_QUALIFICATION_DIR")),
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
		return OperatorDeploymentConfig{}, fmt.Errorf("resolve demo Kind host ingress CIDR: %w", ingressErr)
	} else {
		config.HostIngressCIDR = hostIngressCIDR
	}
	return config, nil
}

func finalizeDemoOperatorApplication(ctx context.Context, config OperatorDeploymentConfig, writer io.Writer) error {
	evidence, err := DeployOperatorManagedApplication(ctx, config, writer)
	if err != nil {
		return err
	}
	RememberOperatorExposure(config.KubeconfigPath, evidence)
	if err := waitForServiceEndpointReady(ctx, config.Namespace, config.KubeconfigPath, "kubernaut-console", 60*time.Second, writer); err != nil {
		return fmt.Errorf("wait for operator Console Service: %w", err)
	}
	kubeClient, err := newKubeconfigControllerClient(config.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("create demo Kubernetes client: %w", err)
	}
	// Remove the legacy demo alias from retained clusters. It was required by
	// the pre-#572 operator-owned Ingress and is no longer part of the exposure
	// contract; only this exact, test-owned Service is targeted.
	if err := deleteLegacyDemoConsoleService(ctx, kubeClient, config.Namespace); err != nil {
		return fmt.Errorf("remove legacy demo Console Service alias: %w", err)
	}
	if err := applyDemoConsoleIngress(ctx, kubeClient, config.Namespace); err != nil {
		return fmt.Errorf("create demo Console Ingress: %w", err)
	}
	if err := waitForDataStorageHTTP(ctx, config.Namespace, config.KubeconfigPath, writer); err != nil {
		return fmt.Errorf("DataStorage did not become ready after operator reconciliation: %w", err)
	}
	if err := BindAFPersonaRBAC(ctx, config.KubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to bind AF persona RBAC: %w", err)
	}
	return nil
}

func deleteLegacyDemoConsoleService(ctx context.Context, kubeClient ctrlclient.Client, namespace string) error {
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "console", Namespace: namespace}}
	return ctrlclient.IgnoreNotFound(kubeClient.Delete(ctx, service))
}

func applyDemoConsoleIngress(ctx context.Context, kubeClient ctrlclient.Client, namespace string) error {
	var desired networkingv1.Ingress
	if err := yaml.Unmarshal([]byte(BuildDemoConsoleIngressManifest(namespace)), &desired); err != nil {
		return fmt.Errorf("parse demo Console Ingress manifest: %w", err)
	}

	var existing networkingv1.Ingress
	key := ctrlclient.ObjectKeyFromObject(&desired)
	if err := kubeClient.Get(ctx, key, &existing); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get demo Console Ingress: %w", err)
		}
		if err := kubeClient.Create(ctx, &desired); err != nil {
			return fmt.Errorf("create demo Console Ingress: %w", err)
		}
		return nil
	}

	existing.Spec = desired.Spec
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for key, value := range desired.Labels {
		existing.Labels[key] = value
	}
	if err := kubeClient.Update(ctx, &existing); err != nil {
		return fmt.Errorf("update demo Console Ingress: %w", err)
	}
	return nil
}

func printDemoOperatorApplicationReady(config OperatorDeploymentConfig, remoteKubeconfigPath string, opts DemoHelmOptions, writer io.Writer) {
	reasoningEnabled, reasoningEffort, reasoningCapabilityOverride := resolveDemoReasoning(opts)
	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ Kubernaut operator demo is ready")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "  Operator image:     %s\n", config.OperatorImage)
	_, _ = fmt.Fprintf(writer, "  LLM reasoning:      enabled=%t", reasoningEnabled)
	if reasoningEffort == "" {
		_, _ = fmt.Fprintln(writer, " effort=(provider default)")
	} else {
		_, _ = fmt.Fprintf(writer, " effort=%s\n", reasoningEffort)
	}
	if reasoningCapabilityOverride != "" {
		_, _ = fmt.Fprintf(writer, "  Capability override: %s\n", reasoningCapabilityOverride)
	}
	_, _ = fmt.Fprintf(writer, "  Trust the demo CA before browser login: %s\n", InterServiceCAPath(config.KubeconfigPath))
	_, _ = fmt.Fprintf(writer, "    Browse to: https://%s:%d\n", demoConsoleHost, DemoHostPort(demoConsolePort))
	_, _ = fmt.Fprintln(writer, "    Login: sre-user / password")
	if opts.Mode == DemoModeFleet {
		_, _ = fmt.Fprintf(writer, "    HUB_KUBECONFIG=%s\n    SPOKE_KUBECONFIG=%s\n", config.KubeconfigPath, remoteKubeconfigPath)
	} else {
		_, _ = fmt.Fprintf(writer, "    KUBECONFIG=%s\n", config.KubeconfigPath)
	}
	_, _ = fmt.Fprintln(writer, "  Teardown ownership:")
	_, _ = fmt.Fprintf(writer, "    kubectl delete kubernaut %s -n %s --ignore-not-found\n", operatorCRName, config.Namespace)
	_, _ = fmt.Fprintf(writer, "    kubectl delete ingress kubernaut-console -n %s --ignore-not-found\n", config.Namespace)
	_, _ = fmt.Fprintf(writer, "    kubectl delete service console -n %s --ignore-not-found\n", config.Namespace)
	_, _ = fmt.Fprintf(writer, "    helm uninstall kubernaut-development-dependencies -n %s --wait\n", config.Namespace)
	_, _ = fmt.Fprintln(writer, "    helm uninstall kubernaut-operator -n kubernaut-operator-system --wait")
	_, _ = fmt.Fprintf(writer, "    kind delete cluster --name %s\n", config.ClusterName)
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
	fixtureDir, err := os.MkdirTemp("", "operator-demo-fixtures-*")
	if err != nil {
		return fmt.Errorf("create demo fixture directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(fixtureDir) }()
	config, err := buildDemoOperatorDeploymentConfig(ctx, kubeconfigPath, kindClusterName, fleetOpts, opts, fixtureDir)
	if err != nil {
		return err
	}
	if err := EnsureOperatorDataStorageClientRBAC(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("prepare operator demo RBAC: %w", err)
	}
	if err := EnsureOperatorApplicationTLS(ctx, kubeconfigPath, config.Namespace, fleetOpts != nil, writer); err != nil {
		return fmt.Errorf("prepare manual operator service TLS: %w", err)
	}
	if err := finalizeDemoOperatorApplication(ctx, config, writer); err != nil {
		return err
	}
	printDemoOperatorApplicationReady(config, remoteKubeconfigPath, opts, writer)
	return nil
}
