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

package fmc_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

func createBackend(ctx context.Context, name, displayName string) {
	GinkgoHelper()
	backend := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.envoyproxy.io/v1alpha1",
			"kind":       "Backend",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "default",
				"labels": map[string]interface{}{
					"kubernaut.ai/managed": "true",
				},
				"annotations": map[string]interface{}{
					"kubernaut.ai/cluster-name": displayName,
				},
			},
		},
	}
	_, err := dynClient.Resource(registry.BackendGVR).Namespace("default").Create(ctx, backend, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "Backend %s should be created in envtest", name)
}

func deleteBackend(ctx context.Context, name string) {
	_ = dynClient.Resource(registry.BackendGVR).Namespace("default").Delete(ctx, name, metav1.DeleteOptions{})
}

// localAlwaysFalse is a stub local scope checker that always returns false,
// isolating the remote (Valkey/FMC) path under test.
type localAlwaysFalse struct{}

func (l *localAlwaysFalse) IsManagedResource(_ context.Context, _ scope.ResourceIdentity) (bool, error) {
	return false, nil
}
