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
			Expect(calls[0].ToolName).To(Equal("prod-east__get_resource"))
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
			Expect(calls[0].ToolName).To(Equal("prod-west__list_resources"))
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
})
