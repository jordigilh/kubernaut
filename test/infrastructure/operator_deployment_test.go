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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("issue #2492 operator deployment contracts", func() {
	It("UT-2492-012 [BR-E2E-001]: mounts writable /tmp only on operator-managed deployments", func() {
		deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app.kubernetes.io/managed-by": "kubernaut-operator"}}}
		deployment.Spec.Template.Spec.Containers = []corev1.Container{{Name: "gateway"}}
		Expect(ensureWritableTempVolume(deployment)).To(BeTrue())
		Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ConsistOf(corev1.VolumeMount{Name: operatorRuntimeTempVolumeName, MountPath: "/tmp"}))
		Expect(deployment.Spec.Template.Spec.Volumes).To(ConsistOf(corev1.Volume{Name: operatorRuntimeTempVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}))
		Expect(ensureWritableTempVolume(deployment)).To(BeFalse())

		unmanaged := &appsv1.Deployment{}
		unmanaged.Spec.Template.Spec.Containers = []corev1.Container{{Name: "other"}}
		Expect(ensureWritableTempVolume(unmanaged)).To(BeFalse())

		existing := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app.kubernetes.io/managed-by": "kubernaut-operator"}}}
		existing.Spec.Template.Spec.Containers = []corev1.Container{{Name: "data-storage", VolumeMounts: []corev1.VolumeMount{{Name: "existing-tmp", MountPath: "/tmp"}}}}
		Expect(ensureWritableTempVolume(existing)).To(BeFalse())
	})
	It("UT-2492-001 [BR-PLATFORM-014]: records exact operator provenance", func() {
		data, err := BuildOperatorProvenanceJSON(OperatorArtifactProvenance{
			Repository:    "jordigilh/kubernaut-operator",
			RequestedRef:  "2c021da02bd62aa9adf5c36d511b33c0483da359",
			ResolvedSHA:   "2c021da02bd62aa9adf5c36d511b33c0483da359",
			ImageRef:      "localhost/kubernaut-operator:cebebf7",
			ImageDigest:   "sha256:operator",
			ChartRevision: "2c021da02bd62aa9adf5c36d511b33c0483da359",
			UpstreamSHA:   "upstream-app-sha",
			WorkflowURL:   "https://github.example/runs/1",
			Profile:       "generic-kind-v1.6",
		})
		Expect(err).NotTo(HaveOccurred())
		var decoded map[string]string
		Expect(json.Unmarshal(data, &decoded)).To(Succeed())
		Expect(decoded).To(HaveKeyWithValue("resolvedSHA", "2c021da02bd62aa9adf5c36d511b33c0483da359"))
		Expect(decoded).To(HaveKeyWithValue("imageDigest", "sha256:operator"))
		Expect(decoded).To(HaveKeyWithValue("profile", "generic-kind-v1.6"))
	})

	It("UT-2492-002 [BR-E2E-001]: accepts the complete 16-key operand map", func() {
		Expect(ValidateOperatorOperandImageMap(testOperatorOperandImages())).To(Succeed())
	})

	It("UT-2492-003 [BR-E2E-001]: rejects an incomplete operand map instead of falling back", func() {
		images := testOperatorOperandImages()
		delete(images, "console")
		Expect(ValidateOperatorOperandImageMap(images)).To(MatchError(ContainSubstring("console")))
	})

	It("UT-2492-004 [BR-PLATFORM-014]: renders exact operator image and relatedImages without latest", func() {
		args := BuildOperatorChartArgs(OperatorChartInstallOptions{
			Namespace:     "kubernaut-system",
			ChartPath:     "oci://example/charts/kubernaut-operator",
			ChartVersion:  "1.6.0-rc9",
			OperatorRef:   "localhost/kubernaut-operator@sha256:operator",
			RelatedImages: testOperatorOperandImages(),
		})
		joined := strings.Join(args, " ")
		Expect(joined).To(ContainSubstring("oci://example/charts/kubernaut-operator"))
		Expect(joined).To(ContainSubstring("--version 1.6.0-rc9"))
		Expect(joined).To(ContainSubstring("relatedImages.GATEWAY"))
		Expect(joined).To(ContainSubstring("sha256:operator"))
		Expect(joined).NotTo(ContainSubstring("latest"))
	})

	It("UT-2492-005 [BR-E2E-001]: bootstrap arguments do not install a CR or application chart", func() {
		args := BuildOperatorBootstrapArgs(OperatorBootstrapOptions{
			Namespace: "kubernaut-system",
			ChartPath: "/artifacts/charts/kubernaut-operator",
		})
		joined := strings.Join(args, " ")
		Expect(joined).To(ContainSubstring("kubernaut-operator"))
		Expect(joined).NotTo(ContainSubstring("charts/kubernaut "))
		Expect(joined).NotTo(ContainSubstring("kind: Kubernaut"))
	})

	It("UT-2492-006 [BR-E2E-001]: dependency arguments own PostgreSQL and Valkey only", func() {
		args := BuildOperatorDependencyArgs(OperatorDependencyOptions{
			Namespace:       "kubernaut-system",
			ChartPath:       "oci://example/kubernaut-development-dependencies",
			ChartVersion:    "0.1.0-run.1",
			PostgreSQLImage: "docker.io/library/postgres:16.4@sha256:postgres",
			ValkeyImage:     "docker.io/valkey/valkey:8.0.2@sha256:valkey",
		})
		joined := strings.Join(args, " ")
		Expect(joined).To(ContainSubstring("--version 0.1.0-run.1"))
		Expect(joined).To(ContainSubstring("kubernaut-development-dependencies"))
		Expect(joined).To(ContainSubstring("postgresql.image.digest"))
		Expect(joined).To(ContainSubstring("valkey.image.digest"))
		Expect(joined).NotTo(ContainSubstring("charts/kubernaut/"))
	})

	It("UT-2492-006B [BR-E2E-001]: renders Valkey mTLS references for the dependency chart", func() {
		options := testOperatorCROptions()
		options.ValkeyTLS = &OperatorValkeyTLSOptions{Enabled: true, CASecretName: "valkey-ca", ClientCertSecretName: "valkey-tls-client"}
		manifest, err := BuildKubernautCRManifest(options)
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).To(ContainSubstring("enabled: true"))
		Expect(manifest).To(ContainSubstring("caSecretName: valkey-ca"))
		Expect(manifest).To(ContainSubstring("clientCertSecretName: valkey-tls-client"))
	})

	It("UT-2492-007 [BR-E2E-001]: renders the minimal required CR plus explicit profile settings", func() {
		manifest, err := BuildKubernautCRManifest(testOperatorCROptions())
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).To(ContainSubstring("apiVersion: kubernaut.ai/v1alpha2"))
		Expect(manifest).To(ContainSubstring("secretName: postgresql-secret"))
		Expect(manifest).To(ContainSubstring("configMapName: aianalysis-policy"))
		Expect(manifest).To(ContainSubstring("configMapName: signalprocessing-policy"))
		Expect(manifest).To(ContainSubstring("provider: openai"))
		Expect(manifest).To(ContainSubstring("kubernautAgent:"))
		Expect(manifest).To(MatchRegexp(`(?m)^\s+apiFrontend:\s*\n\s+enabled: true\s`))
		Expect(manifest).To(ContainSubstring("monitoring:"))
		Expect(manifest).To(ContainSubstring("caCertSecretRef:"))
		Expect(manifest).To(ContainSubstring("name: monitoring-client-ca"))
		Expect(manifest).NotTo(ContainSubstring("pullPolicy:"))
		Expect(manifest).NotTo(ContainSubstring("port: 5432"))
		Expect(manifest).To(ContainSubstring("sslMode: require"))
		Expect(manifest).NotTo(ContainSubstring("maxTurns:"))
		Expect(manifest).NotTo(ContainSubstring("rateLimit:"))
		Expect(manifest).NotTo(ContainSubstring("workflowNamespace:"))
		Expect(strings.Count(manifest, "\n")).To(BeNumerically("<", 60), "the user-authored CR must remain a small contract; operator defaults belong to the operator")
		minimal := testOperatorCROptions()
		minimal.Monitoring = OperatorMonitoringOptions{}
		minimalManifest, minimalErr := BuildKubernautCRManifest(minimal)
		Expect(minimalErr).NotTo(HaveOccurred())
		Expect(minimalManifest).NotTo(ContainSubstring("monitoring:"))
	})

	It("UT-2492-007B [BR-PLATFORM-014]: explicitly enables API Frontend when the demo requests it", func() {
		manifest, err := BuildKubernautCRManifest(testOperatorCROptions())
		Expect(err).NotTo(HaveOccurred())
		var decoded operatorCRManifest
		Expect(yaml.Unmarshal([]byte(manifest), &decoded)).To(Succeed())
		Expect(decoded.Spec.APIFrontend).NotTo(BeNil())
		Expect(decoded.Spec.APIFrontend.Enabled).NotTo(BeNil())
		Expect(*decoded.Spec.APIFrontend.Enabled).To(BeTrue())
	})

	It("UT-2492-007C [BR-E2E-001]: explicitly enables configured monitoring endpoints", func() {
		manifest, err := BuildKubernautCRManifest(testOperatorCROptions())
		Expect(err).NotTo(HaveOccurred())
		var decoded operatorCRManifest
		Expect(yaml.Unmarshal([]byte(manifest), &decoded)).To(Succeed())
		Expect(decoded.Spec.Monitoring).NotTo(BeNil())
		Expect(decoded.Spec.Monitoring.Prometheus).NotTo(BeNil())
		Expect(decoded.Spec.Monitoring.Prometheus.Enabled).NotTo(BeNil())
		Expect(*decoded.Spec.Monitoring.Prometheus.Enabled).To(BeTrue())
		Expect(decoded.Spec.Monitoring.AlertManager).NotTo(BeNil())
		Expect(decoded.Spec.Monitoring.AlertManager.Enabled).NotTo(BeNil())
		Expect(*decoded.Spec.Monitoring.AlertManager.Enabled).To(BeTrue())
	})

	It("UT-2492-TLS-001 [BR-E2E-001]: provides the manual-TLS trust bundle expected by API Frontend", func() {
		manifest := BuildOperatorTLSTrustBundleManifest("kubernaut-system", []byte("CA DATA\n"))
		Expect(manifest).To(ContainSubstring("kind: ConfigMap"))
		Expect(manifest).To(ContainSubstring("name: inter-service-trust-bundle"))
		Expect(manifest).To(ContainSubstring("service-ca.crt:"))
		Expect(manifest).To(ContainSubstring("CA DATA"))
	})

	It("UT-2492-TLS-002 [BR-E2E-001]: preserves the legacy inter-service CA alias for E2E helpers", func() {
		manifest := BuildOperatorInterServiceCAManifest("kubernaut-system", []byte("CA DATA\n"))
		Expect(manifest).To(ContainSubstring("name: inter-service-ca"))
		Expect(manifest).To(ContainSubstring("ca.crt:"))
		Expect(manifest).To(ContainSubstring("CA DATA"))
	})

	It("UT-2492-007A [BR-E2E-001]: renders KA investigation capacity when explicitly requested", func() {
		options := testOperatorCROptions()
		options.MaxConcurrentInvestigations = 50
		manifest, err := BuildKubernautCRManifest(options)
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).To(ContainSubstring("maxConcurrentInvestigations: 50"))
	})

	It("UT-2492-008 [BR-E2E-001]: requires Running, positive conditions, services, and EndpointSlices", func() {
		snapshot := OperatorStatusSnapshot{
			Phase: "Running",
			Conditions: map[string]string{
				"BYOValidated": "True", "MigrationComplete": "True", "ServicesDeployed": "True", "TLSReady": "True",
			},
			Services:       map[string]bool{"gateway": true, "datastorage": true, "apifrontend": true},
			EndpointSlices: map[string]bool{"gateway-service": true, "data-storage-service": true, "apifrontend": true},
		}
		Expect(OperatorStatusReady(snapshot, []string{"gateway", "datastorage", "apifrontend"}, []string{"gateway-service", "data-storage-service", "apifrontend"})).To(Succeed())
		snapshot.Phase = "Ready"
		Expect(OperatorStatusReady(snapshot, []string{"gateway"}, []string{"gateway-service"})).NotTo(Succeed())
	})

	It("UT-2492-009 [BR-E2E-001]: leaves CR image overrides absent unless explicitly requested", func() {
		manifest, err := BuildKubernautCRManifest(testOperatorCROptions())
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).NotTo(ContainSubstring("overrides:"))
		options := testOperatorCROptions()
		options.ImageOverrides = map[string]string{"gateway": "mirror.example/gateway@sha256:custom"}
		manifest, err = BuildKubernautCRManifest(options)
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).To(ContainSubstring("mirror.example/gateway@sha256:custom"))
	})

	It("UT-2492-010 [BR-E2E-001]: references externally prepared manual TLS material", func() {
		manifest, err := BuildKubernautCRManifest(testOperatorCROptions())
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).To(ContainSubstring("mode: manual"))
		Expect(manifest).To(ContainSubstring("internalCASecretName: kubernaut-internal-ca"))
		Expect(manifest).To(ContainSubstring("gateway-tls"))
		Expect(manifest).NotTo(ContainSubstring("extraSANs:"))
	})

	It("UT-2492-010A [BR-E2E-001]: rejects removed TLS source modes instead of silently rewriting them", func() {
		options := testOperatorCROptions()
		options.TLS.Mode = "DevelopmentSelfSigned"
		_, err := BuildKubernautCRManifest(options)
		Expect(err).To(MatchError(`kubernaut CR supports only the v1alpha2 manual TLS mode, got "DevelopmentSelfSigned"`))
	})

	It("UT-2492-010B [BR-PLATFORM-014]: requires an explicit HTTPS Console callback URL", func() {
		options := testOperatorCROptions()
		options.ConsoleEnabled = true
		options.ConsoleAuthSecretName = "kubernaut-console-oidc"
		_, err := BuildKubernautCRManifest(options)
		Expect(err).To(MatchError("kubernaut CR requires an HTTPS Console redirect URL when Console is enabled"))
	})

	It("UT-2492-010C [BR-E2E-001]: includes every operator serving identity in manual TLS", func() {
		options := testOperatorCROptions()
		options.Fleet = &OperatorFleetOptions{Enabled: true, WriteCredentialsSecret: "fleet-write"}
		manifest, err := BuildKubernautCRManifest(options)
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).To(ContainSubstring("authwebhook: authwebhook-tls"))
		Expect(manifest).To(ContainSubstring("fleetmetadatacache: fleetmetadatacache-tls"))
	})

	It("UT-2492-010D [BR-E2E-001]: signs AuthWebhook with both service DNS identities", func() {
		names := qualificationServiceDNSNames("authwebhook-service", "kubernaut-system")
		Expect(names).To(ContainElements(
			"authwebhook-service.kubernaut-system.svc.cluster.local",
			"authwebhook.kubernaut-system.svc.cluster.local",
		))
	})

	It("UT-2492-011 [BR-E2E-001]: renders narrow test-owned adapter selectors and named target ports", func() {
		manifest, err := BuildOperatorExposureManifest(OperatorExposurePlan{
			Namespace: "kubernaut-system",
			Adapters:  []OperatorExposureAdapter{{Name: "kubernaut-e2e-gateway-nodeport", Component: "gateway", ServiceName: "gateway-service", NodePort: 31080, HostPort: 31080}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).To(ContainSubstring("kubernaut.ai/e2e-owner: upstream-2492"))
		Expect(manifest).To(ContainSubstring("app.kubernetes.io/managed-by: kubernaut-operator"))
		Expect(manifest).To(ContainSubstring("targetPort: https"))
		Expect(manifest).NotTo(ContainSubstring("ownerReferences:"))
	})

	It("UT-2492-020 [BR-E2E-001]: renders the Kind host gateway as an ingress CIDR for exposed components", func() {
		options := testOperatorCROptions()
		options.NetworkPolicies = OperatorNetworkPolicyOptions{IngressCIDRs: []string{"10.89.0.1/32"}}
		manifest, err := BuildKubernautCRManifest(options)
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).To(ContainSubstring("networkPolicies:"))
		Expect(manifest).To(ContainSubstring("gateway:"))
		Expect(manifest).To(ContainSubstring("apifrontend:"))
		Expect(manifest).To(ContainSubstring("datastorage:"))
		Expect(manifest).To(ContainSubstring("kubernautAgent:"))
		Expect(manifest).To(ContainSubstring("- 10.89.0.1/32"))
	})

	It("UT-2492-021 [BR-E2E-001]: extracts an IPv4 Kind bridge gateway from the node route", func() {
		gateway, err := parseKindIPv4DefaultGateway("default via 10.89.0.1 dev eth0 proto static metric 100\n")
		Expect(err).NotTo(HaveOccurred())
		Expect(gateway).To(Equal("10.89.0.1"))
	})

	It("UT-2492-034 [BR-SECURITY-001]: isolates the operator manager namespace from application workloads", func() {
		Expect(operatorNamespaceForConfig(OperatorDeploymentConfig{Namespace: "kubernaut-system"})).To(Equal("kubernaut-operator-system"))
		Expect(operatorNamespaceForConfig(OperatorDeploymentConfig{OperatorNamespace: "custom-operator-system"})).To(Equal("custom-operator-system"))
	})

	It("UT-2492-012 [BR-E2E-001]: records CA-verified exposure evidence", func() {
		evidence := OperatorExposureEvidence{NodeAddress: "127.0.0.1", Hostname: "kubernaut-e2e.local", CAPath: "/tmp/inter-service-ca.pem", Adapters: []OperatorExposureAdapter{{Name: "gateway", NodePort: 31080, HostPort: 31080}}}
		data, err := BuildOperatorExposureEvidenceJSON(evidence)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("inter-service-ca.pem"))
		Expect(string(data)).To(ContainSubstring("31080"))
		Expect(string(data)).NotTo(ContainSubstring("InsecureSkipVerify"))
	})

	It("UT-2492-READINESS-001 [BR-PLATFORM-014]: treats non-success readiness responses as not ready", func() {
		Expect(operatorExposureHTTPReady(200)).To(BeTrue())
		Expect(operatorExposureHTTPReady(204)).To(BeTrue())
		Expect(operatorExposureHTTPReady(429)).To(BeFalse())
		Expect(operatorExposureHTTPReady(503)).To(BeFalse())
		Expect(operatorExposureRequiresHTTPReady("datastorage")).To(BeTrue())
		Expect(operatorExposureRequiresHTTPReady("apifrontend")).To(BeTrue())
		Expect(operatorExposureRequiresHTTPReady("gateway")).To(BeFalse())
		Expect(operatorExposureRequiresHTTPReady("kubernautagent")).To(BeFalse())
	})

	It("UT-2492-013 [BR-E2E-001]: appends allocated mappings before Kind creation", func() {
		config, err := appendKindPortMappings("kind: Cluster\nnodes:\n- role: control-plane\n  kubeadmConfigPatches:\n", []KindPortMapping{{ContainerPort: 31080, HostPort: 31080}})
		Expect(err).NotTo(HaveOccurred())
		Expect(config).To(ContainSubstring("containerPort: 31080"))
		Expect(strings.Index(config, "containerPort: 31080")).To(BeNumerically("<", strings.Index(config, "kubeadmConfigPatches")))
	})

	It("UT-2492-014 [BR-FLEET-004]: keeps Fleet read and write OAuth2 references separate", func() {
		options := testOperatorCROptions()
		options.Fleet = &OperatorFleetOptions{Enabled: true, ReadCredentialsSecret: "fleet-read", WriteCredentialsSecret: "fleet-write", GatewayEndpoint: "https://mcp.example/mcp", GatewayType: "eaigw", ScopeBackend: "fleetmetadatacache"}
		manifest, err := BuildKubernautCRManifest(options)
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).To(ContainSubstring("credentialsSecretRef: fleet-read"))
		Expect(manifest).To(ContainSubstring("oauth2CredentialsSecretRef: fleet-write"))
	})

	It("UT-2492-015 [BR-PLATFORM-014]: diagnostic output contains the complete failure contract", func() {
		diagnostic := BuildOperatorDiagnostics(OperatorDiagnosticInput{Scenario: "IT-2492-CR-001", OperatorSHA: "cebebf7", OperatorImage: "operator@sha256:operator", CRManifest: "kind: Kubernaut", Status: "phase: Error", Conditions: "TLSReady=False", Events: "Warning ReconcileFailed", Dependencies: "postgresql Ready=false"})
		Expect(diagnostic).To(ContainSubstring("IT-2492-CR-001"))
		Expect(diagnostic).To(ContainSubstring("operator@sha256:operator"))
		Expect(diagnostic).To(ContainSubstring("kind: Kubernaut"))
		Expect(diagnostic).To(ContainSubstring("postgresql Ready=false"))
	})

	It("UT-2492-017 [BR-PLATFORM-014]: rejects dirty explicit operator source", func() {
		tmpDir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(tmpDir, ".git-dirty-marker"), []byte("x"), 0o600)).To(Succeed())
		Expect(ValidateOperatorSourceCheckout(tmpDir, " M charts/kubernaut-operator/values.yaml")).To(MatchError(ContainSubstring("dirty")))
	})

	It("UT-2492-018 [BR-PLATFORM-014]: qualification manifest links every immutable artifact", func() {
		manifest, err := BuildQualificationManifest(QualificationManifestInput{OperatorImageDigest: "sha256:operator", Artifacts: []QualificationArtifact{{Name: "gateway", Digest: "sha256:gateway", SBOM: "gateway.sbom", Signature: "gateway.sig", Provenance: "gateway.intoto"}}, Results: []QualificationResult{{Lane: "fullpipeline", Passed: true}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(manifest)).To(ContainSubstring("sha256:operator"))
		Expect(string(manifest)).To(ContainSubstring("gateway.sbom"))
		Expect(string(manifest)).To(ContainSubstring("fullpipeline"))
	})

	It("UT-2492-020 [BR-PLATFORM-014]: teardown includes adapter and dependency cleanup", func() {
		cleanup := BuildOperatorCleanupPlan(OperatorCleanupInput{Adapters: []string{"gateway-e2e-nodeport"}, DependencyReleases: []string{"kubernaut-development-dependencies"}})
		Expect(cleanup).To(ConsistOf("delete service/gateway-e2e-nodeport", "helm uninstall kubernaut-development-dependencies"))
	})
})

func testOperatorOperandImages() map[string]string {
	return map[string]string{
		"gateway": "localhost/gateway@sha256:gateway", "datastorage": "localhost/datastorage@sha256:datastorage", "aianalysis": "localhost/aianalysis@sha256:aianalysis", "signalprocessing": "localhost/signalprocessing@sha256:signalprocessing", "remediationorchestrator": "localhost/remediationorchestrator@sha256:ro", "workflowexecution": "localhost/workflowexecution@sha256:we", "effectivenessmonitor": "localhost/effectivenessmonitor@sha256:em", "notification": "localhost/notification@sha256:notification", "kubernautagent": "localhost/kubernautagent@sha256:ka", "authwebhook": "localhost/authwebhook@sha256:authwebhook", "apifrontend": "localhost/apifrontend@sha256:af", "db-migrate": "localhost/db-migrate@sha256:migrate", "console": "quay.io/kubernaut-ai/kubernaut-console@sha256:console", "fleetmetadatacache": "localhost/fleetmetadatacache@sha256:fmc", "init-ubi-minimal": "registry.access.redhat.com/ubi10/ubi-minimal@sha256:init", "oauth2-proxy": "quay.io/oauth2-proxy/oauth2-proxy@sha256:oauth",
	}
}

func testOperatorCROptions() OperatorCROptions {
	return OperatorCROptions{
		Name: "kubernaut", Namespace: "kubernaut-system", PostgreSQLHost: "postgresql.kubernaut-system.svc.cluster.local", PostgreSQLSSLMode: "require", ValkeyHost: "valkey.kubernaut-system.svc.cluster.local", PostgreSQLSecret: "postgresql-secret", ValkeySecret: "valkey-secret", ValkeyTLS: &OperatorValkeyTLSOptions{Enabled: true, CASecretName: "valkey-ca", ClientCertSecretName: "valkey-tls-client"}, LLM: OperatorLLMOptions{Provider: "openai", Model: "mock-model", Endpoint: "http://mock-llm.kubernaut-system.svc.cluster.local:8080", CredentialsSecret: "llm-credentials-primary"}, TLS: OperatorTLSOptions{Mode: "manual", ManualInternalCASecret: "kubernaut-internal-ca"}, PolicyConfigMaps: OperatorPolicyOptions{AIAnalysis: "aianalysis-policy", SignalProcessing: "signalprocessing-policy"}, Monitoring: OperatorMonitoringOptions{PrometheusURL: "https://prometheus-svc.monitoring.svc.cluster.local:9090", AlertManagerURL: "https://alertmanager-svc.monitoring.svc.cluster.local:9093", TLSCASecretName: "monitoring-client-ca"}, GatewayEnabled: true, APIFrontendEnabled: true, IssuerURL: "https://idp.example.com/realms/kubernaut", Audience: "kubernaut-apifrontend",
	}
}
