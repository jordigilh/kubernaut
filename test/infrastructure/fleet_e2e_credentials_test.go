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
	"encoding/base64"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Issue found 2026-09-01 (FLEET_DEMO_QUICKSTART.md): the docs told users to
// pre-create the llm-credentials-primary Secret before running
// `setup-fleet-demo-infra` at all -- impossible, since the cluster meant to
// hold it doesn't exist yet at that point. SetupFleetCoreInfrastructureWithGateway
// always creates a mock placeholder regardless (createFullPipelineHelmSecrets),
// so the "actionable error if missing" InstallFleetDemoHelmChart's
// checkSecretExists promised was actually unreachable. buildLLMCredentialsSecretManifest
// is the pure builder behind the fix: -llm-credentials-file overwrites that
// placeholder with real content once the cluster exists, closing the gap in
// one command instead of two.
var _ = Describe("buildLLMCredentialsSecretManifest", func() {
	It("UT-INFRA-FLEETDEMO-023: base64-encodes the credentials into both data.api_key and data[credentials.json]", func() {
		manifest := buildLLMCredentialsSecretManifest("kubernaut-system", []byte(`{"type":"authorized_user"}`))
		encoded := base64.StdEncoding.EncodeToString([]byte(`{"type":"authorized_user"}`))
		Expect(manifest).To(ContainSubstring("name: llm-credentials-primary"))
		Expect(manifest).To(ContainSubstring("namespace: kubernaut-system"))
		Expect(manifest).To(ContainSubstring("data:"))
		Expect(manifest).To(ContainSubstring("api_key: " + encoded))
		Expect(manifest).To(ContainSubstring("credentials.json: " + encoded))
	})

	It("UT-INFRA-FLEETDEMO-024: scopes the Secret to the namespace passed in", func() {
		manifest := buildLLMCredentialsSecretManifest("some-other-namespace", []byte("a-plain-api-key"))
		Expect(manifest).To(ContainSubstring("namespace: some-other-namespace"))
		Expect(manifest).NotTo(ContainSubstring("namespace: kubernaut-system"))
	})

	It("UT-INFRA-FLEETDEMO-025: round-trips credential bytes containing YAML-unsafe characters (quotes, newlines)", func() {
		tricky := []byte("{\n  \"private_key\": \"-----BEGIN KEY-----\\nabc\\n-----END KEY-----\"\n}\n")
		manifest := buildLLMCredentialsSecretManifest("kubernaut-system", tricky)
		Expect(manifest).To(ContainSubstring("api_key: " + base64.StdEncoding.EncodeToString(tricky)))
	})
})

// Issue found 2026-09-02 (demo team report): the fleet demo's AlertManager
// runs in a dedicated "monitoring" namespace (DD-EM-005's fleet-wide
// platform-monitoring instance), but Gateway's Service lives in
// "kubernaut-system". DeployAlertManager previously took a single namespace
// argument and used it for BOTH AlertManager's own manifest AND the
// gateway-webhook receiver's URL, so the fleet caller's monitoringNamespace
// leaked into the webhook URL too -- producing an unresolvable
// gateway-service.monitoring.svc.cluster.local address. AlertManager's own
// logs confirmed it: "dial tcp: lookup gateway-service.monitoring.svc.
// cluster.local ... no such host". buildAlertManagerManifest now takes
// gatewayNamespace separately; these tests prove the webhook URL tracks it,
// independent of namespace.
var _ = Describe("buildAlertManagerManifest", func() {
	It("UT-INFRA-FLEETDEMO-026: gateway-webhook URL uses gatewayNamespace, not namespace, when they differ", func() {
		manifest := buildAlertManagerManifest("monitoring", "kubernaut-system", "")
		Expect(manifest).To(ContainSubstring("http://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus"))
		Expect(manifest).NotTo(ContainSubstring("gateway-service.monitoring.svc.cluster.local"))
	})

	It("UT-INFRA-FLEETDEMO-027: AlertManager's own ConfigMap/Deployment/Service still use namespace, not gatewayNamespace", func() {
		manifest := buildAlertManagerManifest("monitoring", "kubernaut-system", "")
		Expect(manifest).To(ContainSubstring("name: alertmanager-config\n  namespace: monitoring"))
		Expect(manifest).To(ContainSubstring("name: alertmanager\n  namespace: monitoring"))
		Expect(manifest).To(ContainSubstring("name: alertmanager-svc\n  namespace: monitoring"))
	})

	It("UT-INFRA-FLEETDEMO-028: single-cluster callers passing the same value for both still resolve correctly", func() {
		manifest := buildAlertManagerManifest("kubernaut-system", "kubernaut-system", "")
		Expect(manifest).To(ContainSubstring("http://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus"))
	})

	It("UT-INFRA-FLEETDEMO-029: BR-GATEWAY-036/037 bearer token is still added to the webhook's http_config when provided", func() {
		manifest := buildAlertManagerManifest("kubernaut-system", "kubernaut-system", "test-token")
		Expect(manifest).To(ContainSubstring("bearer_token: 'test-token'"))
	})
})
