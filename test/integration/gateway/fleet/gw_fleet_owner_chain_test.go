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

package fleet_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// IT-GW-P1-001/002: GW Fleet Remote Owner Chain Resolution via MCP Gateway
// Proves the wiring path: PrometheusAdapter.Parse → resolverForCluster →
// K8sOwnerResolver(mcpClient.Reader) → MCP Gateway → owner chain resolved
var _ = Describe("GW Fleet Remote Owner Chain Resolution (BR-INTEGRATION-065)", func() {
	var (
		ctx context.Context
		gw  *mockgw.MockGateway
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if gw != nil {
			gw.Close()
		}
	})

	It("IT-GW-P1-001: Parse resolves remote owner chain through MCP Gateway", func() {
		gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east-1"))

		mcpClient, err := mcpclient.New(ctx, gw.URL())
		Expect(err).ToNot(HaveOccurred())
		defer mcpClient.Close()

		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		localClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		readerFactory := mcpclient.NewMCPReaderFactory(localClient, mcpClient.Session())

		logger := zap.New(zap.UseDevMode(true))
		localResolver := adapters.NewK8sOwnerResolver(localClient, logger)
		adapter := adapters.NewPrometheusAdapter(localResolver, nil, logger)
		adapter.SetReaderFactory(readerFactory)

		payload := buildWebhookPayload("prod-east-1", "prod", "Pod", "api-server-abc")
		signal, err := adapter.Parse(ctx, payload)
		Expect(err).ToNot(HaveOccurred())
		Expect(signal).ToNot(BeNil())
		Expect(signal.ClusterID).To(Equal("prod-east-1"),
			"IT-GW-P1-001: ClusterID must propagate to the signal")
	})

	It("IT-GW-P1-002: Parse falls back to local resolver when MCP Gateway is unavailable", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		localClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		logger := zap.New(zap.UseDevMode(true))
		localResolver := adapters.NewK8sOwnerResolver(localClient, logger)
		adapter := adapters.NewPrometheusAdapter(localResolver, nil, logger)

		payload := buildWebhookPayload("prod-east-1", "prod", "Pod", "api-server-abc")
		signal, err := adapter.Parse(ctx, payload)
		Expect(err).ToNot(HaveOccurred())
		Expect(signal).ToNot(BeNil(),
			"IT-GW-P1-002: should fall back to local resolver when readerFactory is nil")
		Expect(signal.ClusterID).To(Equal("prod-east-1"))
	})
})

func buildWebhookPayload(clusterID, namespace, kind, name string) []byte {
	commonLabels := map[string]string{}
	if clusterID != "" {
		commonLabels["cluster"] = clusterID
	}

	labels := map[string]string{
		"alertname": "HighCPU",
		"severity":  "warning",
		"namespace": namespace,
	}
	switch kind {
	case "Pod":
		labels["pod"] = name
	default:
		labels["pod"] = name
	}

	payload := map[string]any{
		"status": "firing",
		"alerts": []map[string]any{
			{
				"status": "firing",
				"labels": labels,
			},
		},
		"commonLabels": commonLabels,
	}
	data, _ := json.Marshal(payload)
	return data
}
