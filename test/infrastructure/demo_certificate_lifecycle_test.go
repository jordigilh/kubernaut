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
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Issue #2602 long-lived demo certificate lifecycle", func() {
	It("UT-INFRA-2602-ROOT-006 [BR-PLATFORM-014]: resolves the repository root from the working directory before a trimmed caller path", func() {
		root := getProjectRoot()
		resolved := projectRootFrom(root, "/test/infrastructure/shared_integration_utils.go")
		Expect(resolved).To(Equal(root))

		datastorageSource, err := os.ReadFile(filepath.Join(root, "test", "infrastructure", "datastorage.go"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(datastorageSource)).To(ContainSubstring("workspaceRoot = getProjectRoot()"))
	})

	It("UT-INFRA-2602-MONITORING-001 [BR-PLATFORM-014]: keeps the shared demo monitoring CA valid for at least one year", func() {
		ca, err := generateMonitoringCAWithValidity(365 * 24 * time.Hour)
		Expect(err).NotTo(HaveOccurred())
		Expect(ca.certificate.NotAfter.Sub(ca.certificate.NotBefore)).To(BeNumerically(">=", 364*24*time.Hour))

		manifest := BuildDemoMonitoringCAIssuerSecretManifest("monitoring", ca.certificatePEM, []byte("private-key"))
		Expect(manifest).To(ContainSubstring("name: monitoring-ca-issuer"))
		Expect(manifest).To(ContainSubstring("tls.crt:"))
		Expect(manifest).To(ContainSubstring("tls.key:"))
	})

	It("UT-INFRA-2602-MONITORING-002 [BR-PLATFORM-014]: renders independently renewable monitoring leaves and preserves web configuration", func() {
		manifest := BuildDemoMonitoringTLSManifest("monitoring")
		expectValidYAMLDocuments(manifest)

		Expect(manifest).To(ContainSubstring("kind: Secret"))
		Expect(manifest).To(ContainSubstring("name: monitoring-web-config"))
		Expect(manifest).To(ContainSubstring("web-config.yml:"))
		Expect(manifest).To(ContainSubstring("kind: Issuer"))
		Expect(manifest).To(ContainSubstring("name: monitoring-ca-issuer"))
		Expect(manifest).To(ContainSubstring("kind: Certificate"))
		Expect(manifest).To(ContainSubstring("secretName: monitoring-tls"))
		Expect(manifest).To(ContainSubstring("duration: 2160h"))
		Expect(manifest).To(ContainSubstring("renewBefore: 720h"))
		Expect(manifest).To(ContainSubstring("rotationPolicy: Always"))
		Expect(manifest).To(ContainSubstring("prometheus-svc.monitoring.svc.cluster.local"))
		Expect(manifest).To(ContainSubstring("alertmanager-svc.monitoring.svc.cluster.local"))
		Expect(manifest).To(ContainSubstring("thanos-querier-svc.monitoring.svc.cluster.local"))

		trustManifest := BuildDemoMonitoringTrustManifest("kubernaut-system", "monitoring", []byte("CA DATA\n"))
		expectValidYAMLDocuments(trustManifest)
		Expect(strings.Count(trustManifest, "name: monitoring-ca")).To(Equal(2))
		Expect(trustManifest).To(ContainSubstring("name: monitoring-client-ca"))
		Expect(trustManifest).To(ContainSubstring("CA DATA"))
	})

	It("UT-INFRA-2602-MONITORING-003 [BR-PLATFORM-014]: rejects expired, short-lived, or mismatched reused monitoring CAs", func() {
		ca, err := generateMonitoringCAWithValidity(365 * 24 * time.Hour)
		Expect(err).NotTo(HaveOccurred())
		keyDER, err := x509.MarshalECPrivateKey(ca.key)
		Expect(err).NotTo(HaveOccurred())
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

		_, err = parseMonitoringCACertificate(ca.certificatePEM, keyPEM)
		Expect(err).NotTo(HaveOccurred())

		otherCA, err := generateMonitoringCAWithValidity(365 * 24 * time.Hour)
		Expect(err).NotTo(HaveOccurred())
		otherKeyDER, err := x509.MarshalECPrivateKey(otherCA.key)
		Expect(err).NotTo(HaveOccurred())
		otherKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: otherKeyDER})
		_, err = parseMonitoringCACertificate(ca.certificatePEM, otherKeyPEM)
		Expect(err).To(MatchError(ContainSubstring("does not match")))

		shortLivedCA, err := generateMonitoringCAWithValidity(time.Minute)
		Expect(err).NotTo(HaveOccurred())
		shortKeyDER, err := x509.MarshalECPrivateKey(shortLivedCA.key)
		Expect(err).NotTo(HaveOccurred())
		shortKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: shortKeyDER})
		_, err = parseMonitoringCACertificate(shortLivedCA.certificatePEM, shortKeyPEM)
		Expect(err).To(MatchError(ContainSubstring("validity")))
	})

	It("UT-INFRA-2602-DATASTORAGE-003 [BR-PLATFORM-014]: renders the approved RSA signing Certificate contract", func() {
		manifest := BuildDemoDataStorageCertificateManifest("kubernaut-system")
		expectValidYAMLDocuments(manifest)

		Expect(manifest).To(ContainSubstring("apiVersion: cert-manager.io/v1"))
		Expect(manifest).To(ContainSubstring("kind: Certificate"))
		Expect(manifest).To(ContainSubstring("name: datastorage-signing-cert"))
		Expect(manifest).To(ContainSubstring("secretName: datastorage-signing-cert"))
		Expect(manifest).To(ContainSubstring("duration: 8760h"))
		Expect(manifest).To(ContainSubstring("renewBefore: 720h"))
		Expect(manifest).To(ContainSubstring("algorithm: RSA"))
		Expect(manifest).To(ContainSubstring("size: 2048"))
		Expect(manifest).To(ContainSubstring("rotationPolicy: Always"))
	})

	It("IT-INFRA-2602-SCOPE-005 [BR-PLATFORM-014]: wires lifecycle ownership only through the long-lived demo path", func() {
		fleetSource, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "fleet_e2e.go"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(fleetSource)).To(ContainSubstring("EnsureDemoMonitoringTLS("))
		Expect(string(fleetSource)).NotTo(ContainSubstring("ReplicateMonitoringTLS(ctx, kubeconfigPath, remoteKubeconfigPath"))

		demoSource, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "demo_operator.go"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(demoSource)).To(ContainSubstring("EnsureDemoDataStorageSigningCertificate("))
		Expect(string(demoSource)).To(ContainSubstring("EnsureOperatorApplicationTLSForDemo("))
	})

	It("IT-INFRA-2602-RELOAD-004 [BR-PLATFORM-014]: annotates every certificate-consuming demo workload for renewal", func() {
		alertManagerManifest := buildDemoAlertManagerManifest("monitoring", "kubernaut-system", "token")
		expectValidYAMLDocuments(alertManagerManifest)
		Expect(alertManagerManifest).To(ContainSubstring(`secret.reloader.stakater.com/reload: "monitoring-tls"`))
		Expect(alertManagerManifest).To(ContainSubstring("name: monitoring-web-config"))
		Expect(alertManagerManifest).To(ContainSubstring("--web.config.file=/etc/monitoring-web-config/web-config.yml"))
		Expect(alertManagerManifest).To(ContainSubstring("mountPath: /etc/monitoring-web-config"))
		Expect(alertManagerManifest).NotTo(ContainSubstring("mountPath: /etc/monitoring-tls/web-config.yml"))

		shortLivedAlertManagerManifest := buildAlertManagerManifest("monitoring", "kubernaut-system", "token", true)
		Expect(shortLivedAlertManagerManifest).NotTo(ContainSubstring("secret.reloader.stakater.com/reload"))

		prometheusManifest, err := buildManagedPrometheusManifestWithOptionsChecked("monitoring", managedPrometheusOptions{
			clusterLabel:         "hub",
			alertManagerTarget:   "10.0.0.1:30903",
			prometheusName:       managedPrometheusName,
			statefulSetName:      managedPrometheusStatefulSet,
			tlsEnabled:           true,
			reloadOnSecretChange: true,
		})
		Expect(err).NotTo(HaveOccurred())
		expectValidYAMLDocuments(prometheusManifest)
		Expect(prometheusManifest).To(ContainSubstring(`secret.reloader.stakater.com/reload: "monitoring-tls"`))

		demoSource, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "demo_operator.go"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(demoSource)).To(ContainSubstring("annotateDemoEffectivenessMonitorCertificateReload"))
		operatorTLSSource, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "operator_fleet_tls.go"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(operatorTLSSource)).To(ContainSubstring(`secret.reloader.stakater.com/reload":"monitoring-client-ca"`))

		effectivenessSource, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "effectivenessmonitor_e2e.go"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(effectivenessSource)).NotTo(ContainSubstring("configmap.reloader.stakater.com/reload"))

		thanosSource, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "thanos_e2e.go"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(thanosSource)).To(ContainSubstring("--http.config=/etc/monitoring-web-config/web-config.yml"))
		Expect(string(thanosSource)).To(ContainSubstring("mountPath: /etc/monitoring-web-config"))
		Expect(string(thanosSource)).NotTo(ContainSubstring("mountPath: /etc/monitoring-tls/web-config.yml"))
	})
})
