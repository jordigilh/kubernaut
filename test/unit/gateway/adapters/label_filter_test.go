/*
Copyright 2025 Jordi Gil.

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

package adapters

import (
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

var _ = Describe("BR-GATEWAY-184 FR-5: Monitoring Metadata Label Filtering", func() {
	var filter adapters.LabelFilter

	BeforeEach(func() {
		filter = adapters.NewMonitoringMetadataFilter(logr.Discard())
	})

	Context("GW-RE-09: service label filtered for monitoring infrastructure patterns", func() {
		DescribeTable("should detect monitoring infrastructure service names",
			func(serviceValue string) {
				Expect(filter.IsMonitoringMetadata("service", serviceValue)).To(BeTrue(),
					"BR-GATEWAY-184 FR-5: '%s' should be recognized as monitoring infrastructure", serviceValue)
			},
			Entry("kube-prometheus-stack service", "kube-prometheus-stack-kube-state-metrics"),
			Entry("prometheus substring", "prometheus-operator"),
			Entry("alertmanager substring", "alertmanager-main"),
			Entry("grafana substring", "grafana-dashboards"),
			Entry("thanos substring", "thanos-query"),
			Entry("exporter substring", "node-exporter"),
			Entry("victoria prefix", "victoriametrics-single"),
			Entry("loki prefix", "loki-gateway"),
			Entry("jaeger prefix", "jaeger-collector"),
			Entry("-operator suffix", "cert-manager-operator"),
			Entry("case insensitive", "Kube-Prometheus-Stack-Kube-State-Metrics"),
		)
	})

	Context("GW-RE-10: service label passes through for non-monitoring values", func() {
		DescribeTable("should NOT filter legitimate workload service names",
			func(serviceValue string) {
				Expect(filter.IsMonitoringMetadata("service", serviceValue)).To(BeFalse(),
					"BR-GATEWAY-184 FR-5: '%s' should pass through as a legitimate workload service", serviceValue)
			},
			Entry("application service", "payment-api"),
			Entry("user-facing service", "web-frontend"),
			Entry("database service", "postgres-primary"),
			Entry("queue service", "rabbitmq-ha"),
			Entry("cache service", "redis-cluster"),
		)

		It("should not filter non-service label keys", func() {
			Expect(filter.IsMonitoringMetadata("deployment", "kube-prometheus-stack")).To(BeFalse(),
				"BR-GATEWAY-184 FR-5: Filter must only act on 'service' label key")
			Expect(filter.IsMonitoringMetadata("pod", "prometheus-pod-abc123")).To(BeFalse(),
				"BR-GATEWAY-184 FR-5: Filter must only act on 'service' label key")
			Expect(filter.IsMonitoringMetadata("node", "grafana-node")).To(BeFalse(),
				"BR-GATEWAY-184 FR-5: Filter must only act on 'service' label key")
		})
	})
})
