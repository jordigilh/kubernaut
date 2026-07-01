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

package mcpclient_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

func TestMCPClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet MCPResourceClient Suite")
}

var _ = Describe("ResourceClient (BR-FLEET-002, Phase 0)", func() {
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

	Describe("Connect", func() {
		It("UT-FLEET-MCP-001: connects to MCP Gateway and returns a usable client", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-a"))

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("cluster-a"))
			Expect(err).ToNot(HaveOccurred())
			Expect(c).ToNot(BeNil())
			Expect(c.Close()).To(Succeed())
		})

		It("UT-FLEET-MCP-002: returns error for unreachable endpoint", func() {
			_, err := mcpclient.New(ctx, "http://localhost:1")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Get (client.Reader)", func() {
		It("UT-FLEET-P0-001: populates *unstructured.Unstructured via client.Reader.Get", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("prod-east"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})

			err = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.GetKind()).To(Equal("Pod"))
			Expect(obj.GetName()).To(Equal("nginx"))
			Expect(obj.GetNamespace()).To(Equal("default"))
			Expect(obj.GetLabels()).To(HaveKeyWithValue("kubernaut.ai/managed", "true"))

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("prod-east__resources_get"))
		})

		It("UT-FLEET-P0-006: populates *metav1.PartialObjectMetadata with ownerReferences", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("prod-east"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			pom := &metav1.PartialObjectMetadata{}
			pom.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})

			err = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, pom)
			Expect(err).ToNot(HaveOccurred())
			Expect(pom.Name).To(Equal("nginx"))
			Expect(pom.Namespace).To(Equal("default"))
			Expect(pom.Labels).To(HaveKeyWithValue("kubernaut.ai/managed", "true"))
		})

		It("UT-FLEET-MCP-004: returns error for unknown cluster tool", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("unknown-cluster"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})

			err = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, obj)
			Expect(err).To(HaveOccurred())
		})

		It("UT-FLEET-P0-007: returns error when GVK Kind is not set", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("prod-east"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			obj := &unstructured.Unstructured{}
			err = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("GVK Kind must be set"))
		})
	})

	Describe("List (client.Reader)", func() {
		It("UT-FLEET-P0-002: populates UnstructuredList via client.Reader.List", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-west"))

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("prod-west"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})

			err = c.List(ctx, list, client.InNamespace("kube-system"))
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(2))
			Expect(list.Items[0].GetKind()).To(Equal("Pod"))
			Expect(list.Items[0].GetName()).To(Equal("item-1"))

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("prod-west__resources_list"))
		})

		It("UT-FLEET-P0-003: passes MatchingLabels to list_resources", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-west"))

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("prod-west"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})

			err = c.List(ctx, list,
				client.InNamespace("kube-system"),
				client.MatchingLabels{"app": "nginx"},
			)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			var args map[string]any
			Expect(json.Unmarshal(calls[0].Arguments, &args)).To(Succeed())
			Expect(args["labelSelector"]).To(Equal("app=nginx"))
		})
	})

	Describe("Close", func() {
		It("UT-FLEET-MCP-006: Close is idempotent", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-x"))

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("cluster-x"))
			Expect(err).ToNot(HaveOccurred())

			Expect(c.Close()).To(Succeed())
			Expect(c.Close()).To(Succeed())
		})
	})

	Describe("Options", func() {
		It("UT-FLEET-MCP-007: accepts WithHTTPClient option", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-opt"))

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("cluster-opt"), mcpclient.WithTimeout(5))
			Expect(err).ToNot(HaveOccurred())
			Expect(c).ToNot(BeNil())
			Expect(c.Close()).To(Succeed())
		})
	})

	Describe("Get with Kuadrant prefix (WithToolPrefix)", func() {
		It("UT-FLEET-KUA-001: uses Kuadrant prefix for Get tool call instead of EAIGW convention", func() {
			gw = mockgw.NewMockGateway(mockgw.WithKuadrantCluster("spoke-a", "spoke_a_"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			child := mcpclient.NewFromSession(parentClient.Session(), "spoke-a",
				mcpclient.WithToolPrefix("spoke_a_"))

			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})

			err = child.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.GetName()).To(Equal("nginx"))

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("spoke_a_resources_get"),
				"Kuadrant prefix must produce '{prefix}resources_get', not '{id}__resources_get'")
		})
	})

	Describe("List with Kuadrant prefix (WithToolPrefix)", func() {
		It("UT-FLEET-KUA-002: uses Kuadrant prefix for List tool call", func() {
			gw = mockgw.NewMockGateway(mockgw.WithKuadrantCluster("spoke-b", "spoke_b_"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			child := mcpclient.NewFromSession(parentClient.Session(), "spoke-b",
				mcpclient.WithToolPrefix("spoke_b_"))

			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})

			err = child.List(ctx, list, client.InNamespace("kube-system"))
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(2))

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("spoke_b_resources_list"),
				"Kuadrant prefix must produce '{prefix}resources_list'")
		})
	})

	Describe("resolveToolName fallback: no toolPrefix uses EAIGW convention", func() {
		It("UT-FLEET-KUA-003: without WithToolPrefix, falls back to EAIGW {id}__ convention", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("fallback-cluster"))

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("fallback-cluster"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})

			err = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, obj)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("fallback-cluster__resources_get"),
				"without WithToolPrefix, must use EAIGW {id}__ convention")
		})
	})

	Describe("NewFromSession (Phase A)", func() {
		It("UT-FLEET-P0-008 [SC-10]: creates a Client from an existing session without re-connecting, preserving session lifecycle", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-session"))

			parent, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("cluster-session"))
			Expect(err).ToNot(HaveOccurred())
			defer parent.Close()

			session := parent.Session()
			Expect(session).ToNot(BeNil())

			child := mcpclient.NewFromSession(session, "cluster-session")
			Expect(child).ToNot(BeNil())
			Expect(child.ClusterID()).To(Equal("cluster-session"),
				"NewFromSession must bind the clusterID at construction time")

			var reader client.Reader = child
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})

			err = reader.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.GetName()).To(Equal("nginx"),
				"NewFromSession client must be functional using the parent's session (SC-10: network disconnect does not corrupt state)")
		})
	})

	Describe("Session-provider Client reconnect-on-failure (CI incident: FMC sync permanently broken)", func() {
		// Regression coverage for a production incident (Fleet E2E, local repro):
		// FMC's syncer builds per-cluster readers via NewFromSessionProvider,
		// wrapping ResilientClient.SessionProvider(). SessionProvider() only
		// re-reads whatever session ResilientClient currently holds -- it does
		// NOT repair a session that died from a protocol-level error. Once the
		// very first tools/call raced the MCP Gateway during startup and failed,
		// every subsequent call kept using the same dead session forever, even
		// after the Gateway became healthy seconds later. FMC's Valkey cache was
		// never populated, so every fleet-scoped Gateway signal was rejected as
		// "not managed" (BR-INTEGRATION-054, ADR-068).
		It("UT-FLEET-MCP-010 [AC-3]: Get recovers via WithReconnect after the underlying session dies", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.MaxElapsedTime = 5 * time.Second
			rc, err := mcpclient.NewResilient(ctx, gw.URL(), cfg, logr.Discard(), mcpclient.WithClusterID("prod-east"))
			Expect(err).ToNot(HaveOccurred())
			defer rc.Close()

			child := mcpclient.NewFromSessionProvider(rc.SessionProvider(), "prod-east",
				mcpclient.WithReconnect(rc.Reconnect))

			// Simulate the session dying from a protocol-level error while the
			// Gateway itself stays healthy -- exactly the observed production
			// scenario (broker healthy, client session dead).
			Expect(rc.Session().Close()).ToNot(HaveOccurred())

			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})

			err = child.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, obj)
			Expect(err).ToNot(HaveOccurred(),
				"Get must recover by reconnecting once the session is dead, not fail forever")
			Expect(obj.GetName()).To(Equal("nginx"))
		})

		It("UT-FLEET-MCP-011 [AC-3]: without WithReconnect, a dead session fails every call (no silent recovery)", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-west"))

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.MaxElapsedTime = 5 * time.Second
			rc, err := mcpclient.NewResilient(ctx, gw.URL(), cfg, logr.Discard(), mcpclient.WithClusterID("prod-west"))
			Expect(err).ToNot(HaveOccurred())
			defer rc.Close()

			child := mcpclient.NewFromSessionProvider(rc.SessionProvider(), "prod-west")

			Expect(rc.Session().Close()).ToNot(HaveOccurred())

			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})

			err = child.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, obj)
			Expect(err).To(HaveOccurred(),
				"without a reconnect callback there is no way to repair a dead session")
		})
	})

	Describe("ResilientClient interface compliance", func() {
		It("UT-FLEET-P0-005: ResilientClient satisfies ResourceClient (embeds client.Reader)", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-res"))

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.MaxElapsedTime = 5 * time.Second
			rc, err := mcpclient.NewResilient(ctx, gw.URL(), cfg, logr.Discard(), mcpclient.WithClusterID("cluster-res"))
			Expect(err).ToNot(HaveOccurred())
			defer rc.Close()

			var reader client.Reader = rc
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})

			err = reader.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.GetName()).To(Equal("nginx"))
		})
	})

	Describe("IT-FLEET-PARSE: Structured content parsing wiring (BR-FLEET-002)", func() {
		It("IT-FLEET-PARSE-001 [AC-4]: List normalizes table-format structuredContent into proper Unstructured", func() {
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("table-cluster"),
			)

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("table-cluster"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})

			err = c.List(ctx, list, client.InNamespace("kube-system"))
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(2))
			Expect(list.Items[0].GetKind()).To(Equal("Pod"))
			Expect(list.Items[0].GetAPIVersion()).To(Equal("v1"))
			Expect(list.Items[0].GetName()).To(Equal("item-1"))
			Expect(list.Items[0].GetNamespace()).To(Equal("kube-system"))
			Expect(list.Items[1].GetName()).To(Equal("item-2"))
		})

		It("IT-FLEET-PARSE-003 [AC-4]: List with StructuredContent returns items from structured data", func() {
			structuredData := []map[string]any{
				{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name":      "sc-pod-1",
						"namespace": "sc-ns",
						"labels":    map[string]any{"app": "structured"},
					},
				},
			}
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("sc-cluster"),
				mockgw.WithStructuredContent(structuredData),
			)

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("sc-cluster"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})

			err = c.List(ctx, list, client.InNamespace("sc-ns"))
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items[0].GetName()).To(Equal("sc-pod-1"))
			Expect(list.Items[0].GetNamespace()).To(Equal("sc-ns"))
			Expect(list.Items[0].GetLabels()).To(HaveKeyWithValue("app", "structured"))
		})
	})
})
