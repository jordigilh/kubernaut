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

package mcpclient

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceClient provides K8s-compatible read access to resources on a remote
// cluster via the MCP Gateway. It embeds client.Reader so that any component
// accepting client.Reader (e.g. K8sOwnerResolver) can transparently use an MCP-
// backed implementation for remote clusters. The target cluster is bound at
// construction time via WithClusterID, mirroring how a K8s client.Reader is
// bound to a single cluster by its kubeconfig.
//
// Supported object types for Get: *unstructured.Unstructured and
// *metav1.PartialObjectMetadata. Typed objects (e.g. *corev1.Pod) are not
// supported and will return an error — use unstructured access instead.
//
// Authority: Issue #54, Fleet Federation Roadmap Phase 0 (BR-FLEET-002)
type ResourceClient interface {
	client.Reader
	Close() error
}
