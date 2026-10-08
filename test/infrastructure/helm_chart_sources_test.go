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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("fleet demo Helm chart sources [BR-PLATFORM-014]", func() {
	It("UT-INFRA-FLEETDEMO-HELM-001: resolves Prometheus directly from its repository URL", func() {
		Expect(prometheusOperatorHelmInstallArgs()).To(Equal([]string{
			"--repo", "https://prometheus-community.github.io/helm-charts",
			"--version", "88.1.5",
			"--set", "prometheusOperator.fullnameOverride=prometheus-operator",
			"--set", "defaultRules.create=false",
			"--set", "alertmanager.enabled=false",
			"--set", "grafana.enabled=false",
			"--set", "kubeStateMetrics.enabled=false",
			"--set", "nodeExporter.enabled=false",
			"--set", "prometheus.enabled=false",
		}))
	})

	It("UT-INFRA-FLEETDEMO-HELM-002: resolves Traefik directly from its repository URL", func() {
		Expect(traefikHelmInstallArgs()).To(Equal([]string{
			"--repo", "https://traefik.github.io/charts",
			"--set", "service.type=NodePort",
			"--set=ports.web.nodePort=30880",
			"--set=ports.websecure.nodePort=30843",
			"--set", "ingressClass.enabled=true",
			"--set", "ingressClass.isDefaultClass=true",
		}))
	})

	It("UT-INFRA-FLEETDEMO-HELM-003: resolves Istio directly from its repository URL", func() {
		Expect(istioHelmInstallArgs()).To(Equal([]string{
			"--repo", "https://istio-release.storage.googleapis.com/charts",
			"--version", "1.30.2",
		}))
	})
})
