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
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
)

var _ = Describe("FederatedScopeChecker — Spike S4", func() {

	Describe("UT-FLEET-S4-001: local cluster routes to local ScopeChecker", func() {
		It("should delegate to local checker when clusterPrefix is empty", func() {
			local := &mockScopeChecker{managed: true}
			checker := mcpclient.NewFederatedScopeChecker(local, nil, "", logr.Discard())

			managed, err := checker.IsManaged(context.Background(), "default", "Pod", "test-pod")
			Expect(err).NotTo(HaveOccurred())
			Expect(managed).To(BeTrue())
		})
	})

	Describe("UT-FLEET-S4-002: remote cluster checks resource label first", func() {
		It("should return managed when resource has kubernaut.ai/managed=true", func() {
			remote := &mockMCPResourceClient{
				labelsByKey: map[string]map[string]string{
					"cluster_a_Pod/default/test-pod": {"kubernaut.ai/managed": "true"},
				},
			}
			checker := mcpclient.NewFederatedScopeChecker(nil, remote, "cluster_a_", logr.Discard())

			managed, err := checker.IsManaged(context.Background(), "default", "Pod", "test-pod")
			Expect(err).NotTo(HaveOccurred())
			Expect(managed).To(BeTrue())
		})
	})

	Describe("UT-FLEET-S4-003: remote cluster falls through to namespace label", func() {
		It("should check namespace label when resource label is absent", func() {
			remote := &mockMCPResourceClient{
				labelsByKey: map[string]map[string]string{
					"cluster_a_Pod/default/test-pod":      {},
					"cluster_a_Namespace//default": {"kubernaut.ai/managed": "true"},
				},
			}
			checker := mcpclient.NewFederatedScopeChecker(nil, remote, "cluster_a_", logr.Discard())

			managed, err := checker.IsManaged(context.Background(), "default", "Pod", "test-pod")
			Expect(err).NotTo(HaveOccurred())
			Expect(managed).To(BeTrue())
		})
	})

	Describe("UT-FLEET-S4-004: unmanaged when neither label is set", func() {
		It("should return false (safe default) when no labels match", func() {
			remote := &mockMCPResourceClient{
				labelsByKey: map[string]map[string]string{
					"cluster_a_Pod/default/test-pod":      {"app": "nginx"},
					"cluster_a_Namespace//default": {"env": "dev"},
				},
			}
			checker := mcpclient.NewFederatedScopeChecker(nil, remote, "cluster_a_", logr.Discard())

			managed, err := checker.IsManaged(context.Background(), "default", "Pod", "test-pod")
			Expect(err).NotTo(HaveOccurred())
			Expect(managed).To(BeFalse())
		})
	})

	Describe("UT-FLEET-S4-005: graceful fallthrough when resource Get fails", func() {
		It("should try namespace check when resource label fetch fails", func() {
			remote := &mockMCPResourceClient{
				labelsByKey: map[string]map[string]string{
					"cluster_a_Namespace//default": {"kubernaut.ai/managed": "true"},
				},
				errOnMissing: true,
			}
			checker := mcpclient.NewFederatedScopeChecker(nil, remote, "cluster_a_", logr.Discard())

			managed, err := checker.IsManaged(context.Background(), "default", "Pod", "test-pod")
			Expect(err).NotTo(HaveOccurred())
			Expect(managed).To(BeTrue())
		})
	})

	Describe("UT-FLEET-S4-006: error propagated when namespace check fails", func() {
		It("should return error when both resource and namespace fetches fail", func() {
			remote := &mockMCPResourceClient{
				errOnMissing: true,
			}
			checker := mcpclient.NewFederatedScopeChecker(nil, remote, "cluster_a_", logr.Discard())

			_, err := checker.IsManaged(context.Background(), "default", "Pod", "test-pod")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("checking namespace"))
		})
	})
})

var _ = Describe("MCPResourceClient parsing — Spike S4", func() {

	Describe("UT-FLEET-S4-007: parseUnstructured handles standard K8s response", func() {
		It("should parse a JSON resource into Unstructured", func() {
			json := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"test","namespace":"default","labels":{"app":"nginx"}}}`
			obj, err := mcpclient.ParseUnstructuredPublic(json)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.GetName()).To(Equal("test"))
			Expect(obj.GetNamespace()).To(Equal("default"))
			Expect(obj.GetLabels()).To(HaveKeyWithValue("app", "nginx"))
		})
	})

	Describe("UT-FLEET-S4-008: parseUnstructuredList handles items array", func() {
		It("should parse a list response with items field", func() {
			json := `{"apiVersion":"v1","kind":"PodList","items":[{"metadata":{"name":"pod-1"}},{"metadata":{"name":"pod-2"}}]}`
			items, err := mcpclient.ParseUnstructuredListPublic(json)
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(2))
		})
	})

	Describe("UT-FLEET-S4-009: parseUnstructured handles empty response", func() {
		It("should return error for empty text", func() {
			_, err := mcpclient.ParseUnstructuredPublic("")
			Expect(err).To(HaveOccurred())
		})
	})
})

// --- Test Doubles ---

type mockScopeChecker struct {
	managed bool
	err     error
}

func (m *mockScopeChecker) IsManaged(_ context.Context, _, _, _ string) (bool, error) {
	return m.managed, m.err
}

type mockMCPResourceClient struct {
	labelsByKey  map[string]map[string]string
	errOnMissing bool
}

func (m *mockMCPResourceClient) Get(_ context.Context, clusterPrefix, kind, namespace, name string) (*unstructured.Unstructured, error) {
	key := clusterPrefix + kind + "/" + namespace + "/" + name
	labels, ok := m.labelsByKey[key]
	if !ok && m.errOnMissing {
		return nil, errors.New("not found")
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    toAnyMap(labels),
		},
	}}
	return obj, nil
}

func (m *mockMCPResourceClient) List(_ context.Context, _, _, _ string, _ map[string]string) ([]unstructured.Unstructured, error) {
	return nil, nil
}

func (m *mockMCPResourceClient) GetLabels(ctx context.Context, clusterPrefix, kind, namespace, name string) (map[string]string, error) {
	key := clusterPrefix + kind + "/" + namespace + "/" + name
	labels, ok := m.labelsByKey[key]
	if !ok && m.errOnMissing {
		return nil, errors.New("not found")
	}
	if !ok {
		return map[string]string{}, nil
	}
	return labels, nil
}

func toAnyMap(m map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
