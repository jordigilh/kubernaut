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
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/observation"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
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

		// UT-FLEET-MCP-NOTFOUND-004 (issue #2349): Get must recognize a
		// remote resources_get's plain-text "not found" message shape and
		// return a typed apierrors.NotFound, so callers relying on
		// apierrors.IsNotFound/client.IgnoreNotFound (e.g.
		// JobExecutor.Cleanup's ownership-check Get) actually match it.
		It("UT-FLEET-MCP-NOTFOUND-004: Get returns a typed apierrors.NotFound for a k8s-style remote not-found", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("prod-east"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"})

			err = c.Get(ctx, client.ObjectKey{Namespace: "kubernaut-workflows", Name: mockgw.NotFoundSentinelName}, obj)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"remote not-found text must be classified as a typed apierrors NotFound")
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
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("prod-west"),
				mockgw.WithStructuredContent(validPodItems("kube-system")),
			)

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
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("prod-west"),
				mockgw.WithStructuredContent(validPodItems("kube-system")),
			)

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

		It("UT-FLEET-MCP-012 [BR-FLEET-002, BR-WORKFLOW-008]: lists Event resources without applying the FMC observation allowlist", func() {
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("event-cluster"),
				mockgw.WithStructuredContent([]map[string]any{{
					"apiVersion": "v1",
					"kind":       "Event",
					"metadata": map[string]any{
						"name":      "failed-mount",
						"namespace": "default",
					},
				}}),
			)

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("event-cluster"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			list := &corev1.EventList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "EventList"})

			Expect(c.List(ctx, list, client.InNamespace("default"))).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items[0].Name).To(Equal("failed-mount"))
			Expect(list.Items[0].Namespace).To(Equal("default"))

			_, err = c.ListObserved(ctx, mcpclient.ListRequest{
				APIVersion: "v1",
				Kind:       "Event",
				Namespace:  "default",
				Limit:      mcpclient.ObservationPageLimit,
			})
			Expect(err).To(HaveOccurred())
			Expect(mcpclient.FailureOf(err)).To(Equal(mcpclient.FailureScope))
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
			gw = mockgw.NewMockGateway(
				mockgw.WithKuadrantCluster("spoke-b", "spoke_b_"),
				mockgw.WithStructuredContent(validPodItems("kube-system")),
			)

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
		It("IT-FLEET-PARSE-001 [AC-4]: List rejects table-format content as an incomplete observation", func() {
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("table-cluster"),
			)

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("table-cluster"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})

			err = c.List(ctx, list, client.InNamespace("kube-system"))
			Expect(err).To(HaveOccurred())
			Expect(list.Items).To(BeEmpty())
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

		It("IT-FMC-PRES-004 [BR-SCOPE-001, SI-10]: List without structured observation fails closed instead of claiming an empty source", func() {
			// Root cause (Issue #54 fleet E2E RCA, CI run 30464667745): real
			// kube-mcp-server omits StructuredContent entirely for a
			// zero-match List call (IsError=false) rather than returning
			// `{"items": []}`. pkg/fleet/fmc's syncer hit this for every
			// resource kind with zero kubernaut.ai/managed=true matches
			// (StatefulSet/DaemonSet/Service), logging "structuredContent
			// required but not present in response" on every 10s sync cycle
			// for the pod's entire lifetime -- a successful "nothing matched"
			// outcome must not be treated as a parse failure.
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("empty-cluster"),
				mockgw.WithEmptyResourcesList(),
			)

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("empty-cluster"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSetList"})

			err = c.List(ctx, list, client.InNamespace("kubernaut-system"))
			Expect(err).To(HaveOccurred(),
				"missing structured content cannot establish source presence or completeness")
			Expect(list.Items).To(BeEmpty())
		})
	})

	Describe("IT-FMC-PRES-001: typed complete-scan observation bridge (BR-SCOPE-001, SI-10)", func() {
		It("follows an opaque continuation and preserves the exact bounded request contract", func() {
			var requestArgs []map[string]any
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("paged-cluster"),
				mockgw.WithListResourcesHandler(func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					var args map[string]any
					if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
						return nil, fmt.Errorf("decode list arguments: %w", err)
					}
					requestArgs = append(requestArgs, args)
					if _, hasContinue := args["continue"]; !hasContinue {
						return observationPageResult([]map[string]any{observedPod("page-one")}, "opaque-next", true), nil
					}
					if args["continue"] != "opaque-next" {
						return nil, fmt.Errorf("unexpected continuation %v", args["continue"])
					}
					return observationPageResult([]map[string]any{observedPod("page-two")}, "", false), nil
				}),
			)

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("paged-cluster"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			result, err := c.ListObserved(ctx, mcpclient.ListRequest{
				APIVersion: "v1",
				Kind:       "Pod",
				Namespace:  "default",
				Limit:      mcpclient.ObservationPageLimit,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Complete).To(BeTrue())
			Expect(result.PageCount).To(Equal(2))
			Expect(result.ItemCount).To(Equal(2))
			Expect(result.Items).To(HaveLen(2))
			Expect(result.Continuation.Present).To(BeFalse(), "an omitted final continuation is distinct from a token")
			Expect(requestArgs).To(HaveLen(2))
			Expect(requestArgs[0]).ToNot(HaveKey("continue"))
			Expect(requestArgs[0]).ToNot(HaveKey("labelSelector"))
			Expect(requestArgs[0]).To(HaveKeyWithValue("apiVersion", "v1"))
			Expect(requestArgs[0]).To(HaveKeyWithValue("kind", "Pod"))
			Expect(requestArgs[0]).To(HaveKeyWithValue("namespace", "default"))
			Expect(requestArgs[0]["limit"]).To(BeNumerically("==", mcpclient.ObservationPageLimit))
			Expect(requestArgs[1]).To(HaveKeyWithValue("continue", "opaque-next"))
			Expect(requestArgs[1]["limit"]).To(BeNumerically("==", mcpclient.ObservationPageLimit))
		})

		It("rejects a present empty continuation token as incomplete", func() {
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("malformed-continuation"),
				mockgw.WithListResourcesHandler(func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return observationPageResultWithRawContinuation([]map[string]any{observedPod("pod")}, ""), nil
				}),
			)

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("malformed-continuation"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			result, err := c.ListObserved(ctx, validObservationRequest())
			Expect(err).To(HaveOccurred())
			Expect(mcpclient.FailureOf(err)).To(Equal(mcpclient.FailureContinuation))
			Expect(result.Complete).To(BeFalse())
			Expect(gw.CallLog()).To(HaveLen(1))
		})

		It("rejects a repeated continuation token instead of looping", func() {
			var calls int
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("repeated-continuation"),
				mockgw.WithListResourcesHandler(func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					calls++
					return observationPageResult([]map[string]any{observedPod(fmt.Sprintf("pod-%d", calls))}, "same-token", true), nil
				}),
			)

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("repeated-continuation"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			result, err := c.ListObserved(ctx, validObservationRequest())
			Expect(err).To(HaveOccurred())
			Expect(mcpclient.FailureOf(err)).To(Equal(mcpclient.FailureContinuation))
			Expect(result.Complete).To(BeFalse())
			Expect(gw.CallLog()).To(HaveLen(2))
		})

		It("stops at the bounded page limit", func() {
			var calls int
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("page-bound"),
				mockgw.WithListResourcesHandler(func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					calls++
					return observationPageResult(nil, fmt.Sprintf("page-token-%d", calls), true), nil
				}),
			)

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("page-bound"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			result, err := c.ListObserved(ctx, validObservationRequest())
			Expect(err).To(HaveOccurred())
			Expect(mcpclient.FailureOf(err)).To(Equal(mcpclient.FailureBounds))
			Expect(result.Complete).To(BeFalse())
			Expect(calls).To(Equal(observation.MaxObservationPages))
		})

		It("stops at the bounded item limit", func() {
			items := make([]map[string]any, observation.MaxObservationItems+1)
			for i := range items {
				items[i] = observedPod(fmt.Sprintf("pod-%d", i))
			}
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("item-bound"),
				mockgw.WithListResourcesHandler(func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return observationPageResult(items, "", false), nil
				}),
			)

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("item-bound"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			result, err := c.ListObserved(ctx, validObservationRequest())
			Expect(err).To(HaveOccurred())
			Expect(mcpclient.FailureOf(err)).To(Equal(mcpclient.FailureBounds))
			Expect(result.Complete).To(BeFalse())
		})

		It("rejects duplicate and conflicting canonical identities", func() {
			duplicateItems := []map[string]any{observedPod("same"), observedPod("same")}
			conflictingItems := []map[string]any{observedPod("same"), observedPodWithLabel("same", scope.ManagedLabelValueFalse)}

			for _, testCase := range []struct {
				name      string
				items     []map[string]any
				failure   mcpclient.ObservationFailure
				clusterID string
			}{
				{name: "duplicate", items: duplicateItems, failure: mcpclient.FailureDuplicate, clusterID: "duplicate-identity"},
				{name: "conflict", items: conflictingItems, failure: mcpclient.FailureConflict, clusterID: "conflicting-identity"},
			} {
				By(testCase.name)
				gw = mockgw.NewMockGateway(
					mockgw.WithMultiCluster(testCase.clusterID),
					mockgw.WithListResourcesHandler(func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
						return observationPageResult(testCase.items, "", false), nil
					}),
				)

				c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID(testCase.clusterID))
				Expect(err).ToNot(HaveOccurred())
				result, listErr := c.ListObserved(ctx, validObservationRequest())
				Expect(listErr).To(HaveOccurred())
				Expect(mcpclient.FailureOf(listErr)).To(Equal(testCase.failure))
				Expect(result.Complete).To(BeFalse())
				Expect(c.Close()).To(Succeed())
				gw.Close()
				gw = nil
			}
		})

		It("preserves absent, empty, false, and invalid managed-label states", func() {
			items := []map[string]any{
				observedPod("absent"),
				observedPodWithLabels("empty", map[string]any{}),
				observedPodWithLabel("false", scope.ManagedLabelValueFalse),
				observedPodWithLabel("invalid", "unexpected"),
			}
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("label-states"),
				mockgw.WithListResourcesHandler(func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return observationPageResult(items, "", false), nil
				}),
			)

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("label-states"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			result, err := c.ListObserved(ctx, validObservationRequest())
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Complete).To(BeTrue())
			Expect(result.Items[0].LabelsPresent).To(BeFalse())
			Expect(result.Items[0].LabelState).To(Equal(scope.LabelStateAbsent))
			Expect(result.Items[1].LabelsPresent).To(BeTrue())
			Expect(result.Items[1].LabelState).To(Equal(scope.LabelStateAbsent))
			Expect(result.Items[2].LabelState).To(Equal(scope.LabelStateFalse))
			Expect(result.Items[3].LabelState).To(Equal(scope.LabelStateInvalid))
		})

		It("returns a typed source failure when a later page fails", func() {
			var calls int
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("source-failure"),
				mockgw.WithListResourcesHandler(func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					calls++
					if calls == 1 {
						return observationPageResult([]map[string]any{observedPod("first")}, "next", true), nil
					}
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: "remote list failed"}},
						IsError: true,
					}, nil
				}),
			)

			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("source-failure"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			result, err := c.ListObserved(ctx, validObservationRequest())
			Expect(err).To(HaveOccurred())
			Expect(mcpclient.FailureOf(err)).To(Equal(mcpclient.FailureTool))
			Expect(result.Complete).To(BeFalse())
			Expect(calls).To(Equal(2))
		})
	})
})

func validPodItems(namespace string) []map[string]any {
	return []map[string]any{
		{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name":      "item-1",
				"namespace": namespace,
			},
		},
		{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name":      "item-2",
				"namespace": namespace,
			},
		},
	}
}

func validObservationRequest() mcpclient.ListRequest {
	return mcpclient.ListRequest{
		APIVersion: "v1",
		Kind:       "Pod",
		Namespace:  "default",
		Limit:      mcpclient.ObservationPageLimit,
	}
}

func observedPod(name string) map[string]any {
	return map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "default",
		},
	}
}

func observedPodWithLabel(name, value string) map[string]any {
	return observedPodWithLabels(name, map[string]any{scope.ManagedLabelKey: value})
}

func observedPodWithLabels(name string, labels map[string]any) map[string]any {
	item := observedPod(name)
	metadata := item["metadata"].(map[string]any)
	metadata["labels"] = labels
	return item
}

func observationPageResult(items []map[string]any, continuation string, includeContinuation bool) *mcp.CallToolResult {
	structured := map[string]any{"items": mapItemsAsAny(items)}
	if includeContinuation {
		structured["continue"] = continuation
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: "structured page"}},
		StructuredContent: structured,
	}
}

func observationPageResultWithRawContinuation(items []map[string]any, continuation any) *mcp.CallToolResult {
	structured := map[string]any{
		"items":    mapItemsAsAny(items),
		"continue": continuation,
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: "structured page"}},
		StructuredContent: structured,
	}
}

func mapItemsAsAny(items []map[string]any) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	return result
}
