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
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ResourceClient provides typed access to Kubernetes resources on remote clusters
// via the MCP Gateway. Shared by GW, SP, EM, and WE for federated resource access.
//
// Authority: Issue #54, Fleet Federation Roadmap Phase 0 (BR-FLEET-002)
type ResourceClient interface {
	Get(ctx context.Context, clusterID, kind, namespace, name string) (*unstructured.Unstructured, error)
	List(ctx context.Context, clusterID, kind, namespace string, labels map[string]string) ([]unstructured.Unstructured, error)
	GetLabels(ctx context.Context, clusterID, kind, namespace, name string) (map[string]string, error)
	Close() error
}
