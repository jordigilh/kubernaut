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
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	discoveryfake "k8s.io/client-go/discovery/fake"
	clientgotesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/fleet/fleettest"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/shared/k8s/ownerchain"
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
		localResolver := ownerchain.NewK8sOwnerResolver(localClient, logger)
		adapter := adapters.NewPrometheusAdapter(localResolver, adapters.NewTestAPIResourceRegistry(), logger)
		adapter.SetReaderFactory(readerFactory)

		payload := buildWebhookPayload("prod-east-1", "prod", "Pod", "api-server-abc")
		signal, err := adapter.Parse(ctx, payload)
		Expect(err).ToNot(HaveOccurred())
		Expect(signal).ToNot(BeNil())
		Expect(signal.ClusterID).To(Equal("prod-east-1"),
			"IT-GW-P1-001: ClusterID must propagate to the signal")
		Expect(signal.Resource.Kind).To(Equal("Pod"))
		Expect(signal.Resource.APIVersion).To(Equal("v1"))
	})

	It("IT-GW-2593-GVK-001 [BR-GATEWAY-004 BR-SCOPE-002 AC-4 SI-10]: Parse and ParseBatch hand off remote owner authority", func() {
		gw = mockgw.NewMockGateway(mockgw.WithTool("remote__resources_get", "get remote resource",
			json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"apiVersion":{"type":"string"},"namespace":{"type":"string"},"name":{"type":"string"}}}`),
			func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var args struct{ Kind, APIVersion, Namespace, Name string }
				if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
					return nil, err
				}
				metadata := map[string]any{"name": args.Name, "namespace": args.Namespace}
				if args.Kind == "Pod" {
					metadata["ownerReferences"] = []map[string]any{{"apiVersion": "apps/v1", "kind": "Deployment", "name": "remote-parent", "controller": true}}
				}
				return &mcp.CallToolResult{StructuredContent: map[string]any{"apiVersion": args.APIVersion, "kind": args.Kind, "metadata": metadata}}, nil
			}))
		mcpClient, err := mcpclient.New(ctx, gw.URL())
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(mcpClient.Close()).To(Succeed()) }()
		discovery := &discoveryfake.FakeDiscovery{Fake: &clientgotesting.Fake{Resources: []*metav1.APIResourceList{{
			GroupVersion: "v1", APIResources: []metav1.APIResource{{Name: "pods", Kind: "Pod", Namespaced: true}},
		}}}}
		registry, err := adapters.NewAPIResourceRegistry(discovery)
		Expect(err).NotTo(HaveOccurred())
		adapter := adapters.NewPrometheusAdapter(nil, registry)
		adapter.SetReaderFactory(mcpclient.NewMCPReaderFactory(nil, mcpClient.Session()))
		payload := buildWebhookPayload("remote", "prod", "Pod", "child")
		signal, err := adapter.Parse(ctx, payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(signal.ClusterID).To(Equal("remote"))
		Expect(signal.Resource.Kind).To(Equal("Deployment"))
		Expect(signal.Resource.Name).To(Equal("remote-parent"))
		Expect(signal.Resource.APIVersion).To(Equal("apps/v1"))
		signals, err := adapter.ParseBatch(ctx, payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(signals).To(HaveLen(1))
		Expect(signals[0].Resource).To(Equal(signal.Resource))
		Expect(signals[0].Fingerprint).To(Equal(signal.Fingerprint))
		calls := gw.CallLog()
		Expect(calls).To(HaveLen(4))
		for i, call := range calls {
			Expect(call.ToolName).To(Equal("remote__resources_get"))
			var args map[string]string
			Expect(json.Unmarshal(call.Arguments, &args)).To(Succeed())
			if i%2 == 0 {
				Expect(args["apiVersion"]).To(Equal("v1"))
			} else {
				Expect(args["apiVersion"]).To(Equal("apps/v1"))
			}
		}
	})

	It("IT-GW-2430-002 [AC-4, AC-6, ASVS V4.1.5]: Parse fails closed when MCP Gateway is unavailable", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		localClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		logger := zap.New(zap.UseDevMode(true))
		localResolver := ownerchain.NewK8sOwnerResolver(localClient, logger)
		adapter := adapters.NewPrometheusAdapter(localResolver, nil, logger)
		adapter.SetReaderFactory(&fleettest.StubReaderFactory{
			Err: fmt.Errorf("MCP Gateway unavailable"),
		})

		payload := buildWebhookPayload("prod-east-1", "prod", "Pod", "api-server-abc")
		signal, err := adapter.Parse(ctx, payload)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("remote owner resolver"))
		Expect(signal).To(BeNil())
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
