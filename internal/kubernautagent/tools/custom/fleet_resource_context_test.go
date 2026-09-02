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

package custom_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
)

// fakeGetTool is a tools.Tool double standing in for the fleet overlay's
// resources_get tool (DD-FLEET-005). It captures the args it was called
// with and returns either canned JSON text or an error.
type fakeGetTool struct {
	responseJSON string
	err          error

	capturedArgs map[string]any
	calls        int
}

func (f *fakeGetTool) Name() string                { return "resources_get" }
func (f *fakeGetTool) Description() string         { return "fake resources_get" }
func (f *fakeGetTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (f *fakeGetTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	f.calls++
	var m map[string]any
	_ = json.Unmarshal(args, &m)
	f.capturedArgs = m
	if f.err != nil {
		return "", f.err
	}
	return f.responseJSON, nil
}

// UT-KA-FLEET-031: overlayClientReader.Get bridges client.Reader.Get to the
// fleet overlay's resources_get tools.Tool.Execute and populates the target
// object via mcpclient.PopulateObject — issue #2306.
var _ = Describe("UT-KA-FLEET-031: overlayClientReader (BR-INTEGRATION-1489)", func() {
	It("sends kind/apiVersion/name/namespace args derived from the target GVK and populates the object", func() {
		getTool := &fakeGetTool{
			responseJSON: `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api-server","namespace":"production","uid":"deploy-uid-1"}}`,
		}
		reader := custom.NewOverlayClientReader(getTool)

		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})

		err := reader.Get(context.Background(), client.ObjectKey{Namespace: "production", Name: "api-server"}, obj)
		Expect(err).NotTo(HaveOccurred())

		Expect(getTool.calls).To(Equal(1))
		Expect(getTool.capturedArgs["kind"]).To(Equal("Deployment"))
		Expect(getTool.capturedArgs["apiVersion"]).To(Equal("apps/v1"))
		Expect(getTool.capturedArgs["name"]).To(Equal("api-server"))
		Expect(getTool.capturedArgs["namespace"]).To(Equal("production"))

		Expect(obj.GetName()).To(Equal("api-server"))
		Expect(obj.GetUID()).To(BeEquivalentTo("deploy-uid-1"))
	})

	It("omits the namespace arg for a cluster-scoped Get", func() {
		getTool := &fakeGetTool{responseJSON: `{"apiVersion":"v1","kind":"Node","metadata":{"name":"worker-1"}}`}
		reader := custom.NewOverlayClientReader(getTool)

		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Node"})

		err := reader.Get(context.Background(), client.ObjectKey{Name: "worker-1"}, obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(getTool.capturedArgs).NotTo(HaveKey("namespace"))
	})

	It("returns a clear error when the target object has no GVK set", func() {
		getTool := &fakeGetTool{}
		reader := custom.NewOverlayClientReader(getTool)

		obj := &unstructured.Unstructured{}
		err := reader.Get(context.Background(), client.ObjectKey{Name: "x"}, obj)
		Expect(err).To(HaveOccurred())
		Expect(getTool.calls).To(Equal(0))
	})

	It("propagates the underlying tool's Execute error", func() {
		getTool := &fakeGetTool{err: fmt.Errorf("remote tool call failed")}
		reader := custom.NewOverlayClientReader(getTool)

		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})
		err := reader.Get(context.Background(), client.ObjectKey{Name: "p"}, obj)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("remote tool call failed"))
	})

	// UT-KA-FLEET-033 (issue #2344): the MCP protocol only carries tool
	// errors as plain text (bridge_tool.go's BridgeTool.Execute renders
	// result.IsError as a formatted string), so a remote cluster's real
	// apierrors.NewNotFound arrives here as text, not a typed
	// *apierrors.StatusError. Get must recognize the k8s-standard
	// "<resource> \"<name>\" not found" message shape and re-wrap it as a
	// typed NotFound so enrichment.IsNotFoundError (and the HardFail /
	// TargetResourceDeleted contract, issue #1039) works identically to the
	// hub-local K8sAdapter path. Without this, every legitimately-absent
	// remote resource was misclassified as a hard failure.
	It("returns a typed apierrors NotFound when the remote tool reports a k8s-style not-found (issue #2344)", func() {
		getTool := &fakeGetTool{
			err: fmt.Errorf("remote tool %q (cluster %q) returned error: failed to get resource: pods %q not found",
				"remote-cluster__resources_get", "remote-cluster", "memory-eater"),
		}
		reader := custom.NewOverlayClientReader(getTool)

		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})
		err := reader.Get(context.Background(), client.ObjectKey{Namespace: "kubernaut-system", Name: "memory-eater"}, obj)

		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsNotFound(err)).To(BeTrue(),
			"UT-KA-FLEET-033: remote not-found text must be classified as a typed apierrors NotFound")
	})

	It("does not misclassify an unrelated error mentioning a different resource as not-found (issue #2344)", func() {
		getTool := &fakeGetTool{
			err: fmt.Errorf(`remote tool "remote-cluster__resources_get" (cluster "remote-cluster") returned error: connection refused`),
		}
		reader := custom.NewOverlayClientReader(getTool)

		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})
		err := reader.Get(context.Background(), client.ObjectKey{Namespace: "kubernaut-system", Name: "memory-eater"}, obj)

		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsNotFound(err)).To(BeFalse(),
			"UT-KA-FLEET-033: a non-not-found error must not be misclassified")
	})

	It("returns a clear not-supported error for List", func() {
		reader := custom.NewOverlayClientReader(&fakeGetTool{})
		err := reader.List(context.Background(), &unstructured.UnstructuredList{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not supported"))
	})
})

// UT-KA-FLEET-032: overlayK8sClient wraps the moved ownerchain.K8sOwnerResolver
// for GetOwnerChain, and ownerchain.KindToGroup() for GetSpecHash's apiVersion
// resolution — issue #2306.
var _ = Describe("UT-KA-FLEET-032: overlayK8sClient (BR-INTEGRATION-1489)", func() {
	Describe("GetOwnerChain", func() {
		It("wraps ResolveTopLevelOwner's result into a single-entry chain", func() {
			getTool := &fakeGetTool{
				responseJSON: `{"apiVersion":"apps/v1","kind":"ReplicaSet","metadata":{"name":"api-server-abc","namespace":"production",` +
					`"ownerReferences":[{"apiVersion":"apps/v1","kind":"Deployment","name":"api-server","uid":"deploy-uid-1","controller":true}]}}`,
			}
			c := custom.NewOverlayK8sClient(getTool, logr.Discard())

			chain, err := c.GetOwnerChain(context.Background(), "ReplicaSet", "api-server-abc", "production", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(1))
			Expect(chain[0].Kind).To(Equal("Deployment"))
			Expect(chain[0].Name).To(Equal("api-server"))
		})

		It("returns the resource itself when it has no controller owner", func() {
			getTool := &fakeGetTool{
				responseJSON: `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"standalone-pod","namespace":"default"}}`,
			}
			c := custom.NewOverlayK8sClient(getTool, logr.Discard())

			chain, err := c.GetOwnerChain(context.Background(), "Pod", "standalone-pod", "default", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(1))
			Expect(chain[0].Kind).To(Equal("Pod"))
			Expect(chain[0].Name).To(Equal("standalone-pod"))
		})
	})

	Describe("GetSpecHash", func() {
		It("resolves apiVersion via ownerchain.KindToGroup for a known kind and computes a hash", func() {
			getTool := &fakeGetTool{
				responseJSON: `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api-server","namespace":"production"},"spec":{"replicas":3}}`,
			}
			c := custom.NewOverlayK8sClient(getTool, logr.Discard())

			h, err := c.GetSpecHash(context.Background(), "Deployment", "api-server", "production", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(h).NotTo(BeEmpty())
			Expect(getTool.capturedArgs["apiVersion"]).To(Equal("apps/v1"))
		})

		It("returns a clear error for an unknown kind with no apiVersion given", func() {
			c := custom.NewOverlayK8sClient(&fakeGetTool{}, logr.Discard())

			_, err := c.GetSpecHash(context.Background(), "CustomResource", "my-cr", "test", "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("CustomResource"))
		})
	})
})
