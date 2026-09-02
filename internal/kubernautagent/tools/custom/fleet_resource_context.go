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

package custom

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/shared/hash"
	"github.com/jordigilh/kubernaut/pkg/shared/k8s/ownerchain"
)

// overlayClientReader adapts the fleet overlay's resources_get tools.Tool
// (DD-FLEET-005) to a controller-runtime client.Reader, so
// ownerchain.K8sOwnerResolver — built to walk owner chains via any
// client.Reader — can resolve against a fleet-target cluster the same way it
// already does for Gateway's dedup path (issue #2306).
//
// Get() requires the caller to have already stamped the target GVK onto obj
// (every internal caller in this file does, via SetGroupVersionKind, and
// ownerchain.K8sOwnerResolver always does the same before each Get):
// resources_get requires an explicit apiVersion parameter — there is no
// server-side kind-only discovery to infer it from (confirmed against
// upstream kubernetes-mcp-server, see plan preflight).
type overlayClientReader struct {
	getTool tools.Tool
}

var _ client.Reader = (*overlayClientReader)(nil)

// NewOverlayClientReader builds a client.Reader backed by the fleet
// overlay's resources_get tool. Exported for the fleet_resource_context_test.go
// unit tests; production callers should go through NewOverlayK8sClient.
func NewOverlayClientReader(getTool tools.Tool) client.Reader {
	return &overlayClientReader{getTool: getTool}
}

func (r *overlayClientReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" {
		return fmt.Errorf("overlayClientReader.Get: target object has no GroupVersionKind set")
	}

	args := map[string]any{
		"kind":       gvk.Kind,
		"apiVersion": gvk.GroupVersion().String(),
		"name":       key.Name,
	}
	if key.Namespace != "" {
		args["namespace"] = key.Namespace
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("overlayClientReader.Get: marshal args for %s/%s: %w", gvk.Kind, key.Name, err)
	}

	text, err := r.getTool.Execute(ctx, argsJSON)
	if err != nil {
		if notFound := asRemoteNotFound(err, gvk, key.Name); notFound != nil {
			err = notFound
		}
		return fmt.Errorf("overlayClientReader.Get: %s/%s in %q: %w", gvk.Kind, key.Name, key.Namespace, err)
	}
	fetched, err := mcpclient.ParseUnstructuredResponse(text)
	if err != nil {
		return fmt.Errorf("overlayClientReader.Get: parse response for %s/%s: %w", gvk.Kind, key.Name, err)
	}
	return mcpclient.PopulateObject(fetched, obj)
}

// asRemoteNotFound recognizes the Kubernetes API's standard NotFound message
// shape (apierrors.NewNotFound's `"<resource>[.<group>] %q not found"`) inside
// a fleet overlay tool's error text and converts it to a typed
// *apierrors.StatusError, or returns nil when the text doesn't match.
//
// The MCP protocol only carries tool errors as plain text (BridgeTool.Execute
// renders a remote result.IsError as a formatted string, bridge_tool.go), so
// a remote cluster's real apierrors.NewNotFound -- raised by kube-mcp-server's
// own client-go call -- arrives here as a string, not a *apierrors.StatusError.
// That loses the type enrichment.IsNotFoundError needs to honor the HardFail /
// TargetResourceDeleted contract (issue #1039): every legitimately-absent
// remote resource was misclassified as a hard failure, triggering
// rca_incomplete for any fleet-target investigation whose target doesn't
// exist (issue #2344, caught by E2E-FLEET-004 once #2343 first routed real
// investigations through this path instead of the hub-only client).
//
// Scoped to the exact requested name (`%q not found`), not a blind "not
// found" substring match, so an unrelated error that happens to mention a
// different name isn't misclassified.
func asRemoteNotFound(err error, gvk schema.GroupVersionKind, name string) error {
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%q not found", name)) {
		return nil
	}
	return apierrors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: strings.ToLower(gvk.Kind)}, name)
}

// List is intentionally not supported: overlayClientReader only backs
// ownerchain.K8sOwnerResolver's remote-cluster resolution here, which only
// ever calls Get directly. The resolver's one internal List call (the
// optional Service→Pod special case) is gated on WithFallbackReader, which
// NewOverlayK8sClient deliberately never sets — KA has no local fallback
// reader for a remote cluster — so this path is unreachable in practice,
// not silently degraded.
func (r *overlayClientReader) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return fmt.Errorf("overlayClientReader: List not supported (owner-chain resolution only requires Get)")
}

// overlayK8sClient implements enrichment.K8sClient by resolving owner-chain
// and spec-hash queries against a fleet-target cluster via the overlay's
// resources_get tool, reusing Gateway's own proven K8sOwnerResolver algorithm
// (pkg/shared/k8s/ownerchain, moved from pkg/gateway/adapters — issue #2306)
// instead of re-authoring a walk loop against a second client type.
type overlayK8sClient struct {
	reader client.Reader
	logger logr.Logger
}

var _ enrichment.K8sClient = (*overlayK8sClient)(nil)

// NewOverlayK8sClient builds an enrichment.K8sClient wrapping getTool (the
// fleet overlay's resources_get tool — see cmd/kubernautagent/toolregistry.go's
// genericNameTool). No WithRegistry/WithFallbackReader: KA has neither a
// live APIResourceRegistry nor a local fallback client for a remote cluster.
func NewOverlayK8sClient(getTool tools.Tool, logger logr.Logger) enrichment.K8sClient {
	return &overlayK8sClient{reader: NewOverlayClientReader(getTool), logger: logger}
}

// GetOwnerChain resolves the top-level controller owner of kind/name/namespace
// against the target cluster and wraps it as a single-entry chain. Both
// resourceContextTools callers only ever read chain[len(chain)-1] (the root
// entry), never intermediate hops, so ownerchain.K8sOwnerResolver's
// "top owner only" contract is a precise fit here, not a lossy simplification.
func (c *overlayK8sClient) GetOwnerChain(ctx context.Context, kind, name, namespace, _ string) ([]enrichment.OwnerChainEntry, error) {
	resolver := ownerchain.NewK8sOwnerResolver(c.reader, c.logger)
	ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(ctx, namespace, kind, name)
	if err != nil {
		return nil, err
	}
	return []enrichment.OwnerChainEntry{{Kind: ownerKind, Name: ownerName, Namespace: namespace}}, nil
}

// GetSpecHash resolves apiVersion for kind via the moved ownerchain.KindToGroup
// table (the same lookup GetOwnerChain's resolver uses internally when no
// registry is configured), fetches the resource via the shared
// overlayClientReader, then reuses the exact hash.CanonicalResourceFingerprint
// function K8sAdapter.GetSpecHash already calls for the hub-local path —
// keeping hash values comparable between hub-local and fleet-target
// investigations.
func (c *overlayK8sClient) GetSpecHash(ctx context.Context, kind, name, namespace, apiVersion string) (string, error) {
	if apiVersion == "" {
		group, known := ownerchain.KindToGroup()[kind]
		if !known {
			return "", fmt.Errorf("overlayK8sClient.GetSpecHash: unknown kind %q for fleet-target resolution "+
				"(not in core/apps/batch and no apiVersion given)", kind)
		}
		apiVersion = schema.GroupVersion{Group: group, Version: "v1"}.String()
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.FromAPIVersionAndKind(apiVersion, kind))
	if err := c.reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		return "", fmt.Errorf("overlayK8sClient.GetSpecHash: get %s/%s in %q: %w", kind, name, namespace, err)
	}

	h, err := hash.CanonicalResourceFingerprint(obj.Object)
	if err != nil {
		return "", fmt.Errorf("overlayK8sClient.GetSpecHash: compute fingerprint for %s/%s in %q: %w", kind, name, namespace, err)
	}
	return h, nil
}

// noopK8sClient always returns a clear "not available" error for both
// enrichment.K8sClient methods. Selected only when a fleet-target
// investigation's overlay does not publish resources_get (e.g. a toolset
// misconfiguration) — distinct from silently degrading to hub-bound
// resolution or an empty result, so the underlying cause stays diagnosable.
type noopK8sClient struct {
	clusterID string
}

var _ enrichment.K8sClient = noopK8sClient{}

func (n noopK8sClient) GetOwnerChain(_ context.Context, _, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	return nil, fmt.Errorf("owner-chain resolution not available for cluster %q: fleet overlay does not publish resources_get", n.clusterID)
}

func (n noopK8sClient) GetSpecHash(_ context.Context, _, _, _, _ string) (string, error) {
	return "", fmt.Errorf("spec-hash resolution not available for cluster %q: fleet overlay does not publish resources_get", n.clusterID)
}
