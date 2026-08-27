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

package registry_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

var _ = Describe("KuadrantRegistry.Start (BR-INTEGRATION-065)", func() {
	// UT-FLEET-2299-001: port of UT-FLEET-003-006 (EAIGWRegistry) to
	// KuadrantRegistry. Guards against the data race between Start()'s read
	// of w.clusters (for the "started and synced" log line) and the
	// mutex-protected write in onAdd(). client-go's WaitForCacheSync only
	// guarantees the reflector's initial List has populated the informer's
	// internal store; it does NOT guarantee that the sharedProcessor has
	// finished dispatching AddFunc for every pre-existing object (handler
	// dispatch runs on separate processorListener goroutines -- see
	// k8s.io/client-go/tools/cache/shared_informer.go). Exercising Start()
	// with pre-existing MCPServerRegistrations under `go test -race` is the
	// regression guard for that race, mirroring EAIGWRegistry's existing
	// UT-FLEET-003-006 (PR #1539 post-merge run).
	//
	// RED-phase outcome (documented, #2299): run under
	// `go test -race -count=50 ./pkg/fleet/registry/... -run TestFleetRegistry`
	// prior to GREEN -- passed all 50 iterations with no race detected and
	// no missing clusters. This matches the throwaway spike's finding that
	// the WaitForCacheSync/onAdd-dispatch gap requires handler-side delay
	// (absent here, and absent in production onAdd) to manifest reliably;
	// it is not a flag that a fast dynamicfake client trips on its own. The
	// race is still real (confirmed by direct client-go source review and
	// the spike's 30/30-with-delay result) -- this test's value is as a
	// permanent regression guard once GREEN's seed-from-indexer fix lands,
	// not as a test that fails today.
	It("UT-FLEET-2299-001: Start() succeeds and does not race when pre-existing MCPServerRegistrations are synced", func() {
		scheme := runtime.NewScheme()
		fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				registry.MCPServerRegistrationGVR: "MCPServerRegistrationList",
			},
		)

		for i := 0; i < 20; i++ {
			reg := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "mcp.kuadrant.io/v1alpha1",
					"kind":       "MCPServerRegistration",
					"metadata": map[string]interface{}{
						"name":      fmt.Sprintf("cluster-%d", i),
						"namespace": "kuadrant-system",
						"labels": map[string]interface{}{
							"kubernaut.ai/managed": "true",
						},
					},
					"spec": map[string]interface{}{
						"prefix": fmt.Sprintf("cluster_%d_", i),
					},
				},
			}
			_, err := fakeClient.Resource(registry.MCPServerRegistrationGVR).Namespace("kuadrant-system").Create(
				context.Background(), reg, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		}

		reg := registry.NewKuadrantRegistry(fakeClient, registry.EAIGWRegistryConfig{}, nil, logr.Discard())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		Expect(reg.Start(ctx)).To(Succeed())
		reg.Stop()
	})
})

var _ = Describe("KuadrantRegistry.Ready (BR-INTEGRATION-065)", func() {
	// UT-FLEET-2299-002: Ready() must reflect Start()'s actual completion
	// state -- false beforehand, true only once Start() has returned
	// successfully. Previously untested (0% coverage per #2300's gap
	// analysis).
	It("UT-FLEET-2299-002 [SI-4]: Ready() reports true only after Start() completes", func() {
		scheme := runtime.NewScheme()
		fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				registry.MCPServerRegistrationGVR: "MCPServerRegistrationList",
			},
		)

		reg := registry.NewKuadrantRegistry(fakeClient, registry.EAIGWRegistryConfig{}, nil, logr.Discard())
		Expect(reg.Ready()).To(BeFalse(), "Ready() must be false before Start() is called")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		Expect(reg.Start(ctx)).To(Succeed())
		Expect(reg.Ready()).To(BeTrue(), "Ready() must be true once Start() returns successfully")
		reg.Stop()
	})
})
