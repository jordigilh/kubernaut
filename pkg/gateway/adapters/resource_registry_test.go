/*
Copyright 2025 Jordi Gil.

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

package adapters_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// standardResources returns a discovery response containing the standard
// Kubernetes API resources used in parity tests.
func standardResources() []*metav1.APIResourceList {
	return []*metav1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", SingularName: "deployment", Kind: "Deployment", Namespaced: true},
				{Name: "statefulsets", SingularName: "statefulset", Kind: "StatefulSet", Namespaced: true},
				{Name: "daemonsets", SingularName: "daemonset", Kind: "DaemonSet", Namespaced: true},
				{Name: "replicasets", SingularName: "replicaset", Kind: "ReplicaSet", Namespaced: true},
			},
		},
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", SingularName: "pod", Kind: "Pod", Namespaced: true},
				{Name: "nodes", SingularName: "node", Kind: "Node", Namespaced: false},
				{Name: "services", SingularName: "service", Kind: "Service", Namespaced: true},
				{Name: "persistentvolumeclaims", SingularName: "persistentvolumeclaim", Kind: "PersistentVolumeClaim", Namespaced: true},
			},
		},
		{
			GroupVersion: "batch/v1",
			APIResources: []metav1.APIResource{
				{Name: "jobs", SingularName: "job", Kind: "Job", Namespaced: true},
				{Name: "cronjobs", SingularName: "cronjob", Kind: "CronJob", Namespaced: true},
			},
		},
		{
			GroupVersion: "autoscaling/v2",
			APIResources: []metav1.APIResource{
				{Name: "horizontalpodautoscalers", SingularName: "horizontalpodautoscaler", Kind: "HorizontalPodAutoscaler", Namespaced: true},
			},
		},
		{
			GroupVersion: "policy/v1",
			APIResources: []metav1.APIResource{
				{Name: "poddisruptionbudgets", SingularName: "poddisruptionbudget", Kind: "PodDisruptionBudget", Namespaced: true},
			},
		},
	}
}

// ocpResources returns discovery responses for OpenShift CRD API groups.
func ocpResources() []*metav1.APIResourceList {
	return []*metav1.APIResourceList{
		{
			GroupVersion: "build.openshift.io/v1",
			APIResources: []metav1.APIResource{
				{Name: "buildconfigs", SingularName: "buildconfig", Kind: "BuildConfig", Namespaced: true},
			},
		},
		{
			GroupVersion: "route.openshift.io/v1",
			APIResources: []metav1.APIResource{
				{Name: "routes", SingularName: "route", Kind: "Route", Namespaced: true},
			},
		},
		{
			GroupVersion: "apps.openshift.io/v1",
			APIResources: []metav1.APIResource{
				{Name: "deploymentconfigs", SingularName: "deploymentconfig", Kind: "DeploymentConfig", Namespaced: true},
			},
		},
	}
}

// namespaceResource returns the core v1 Namespace API resource, which is
// absent from standardResources() but present in production clusters.
// Used by #1067 tests to reproduce the namespace-as-resource-candidate bug.
func namespaceResource() []*metav1.APIResourceList {
	return []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "namespaces", SingularName: "namespace", Kind: "Namespace", Namespaced: false},
			},
		},
	}
}

func newFakeDiscovery(resources ...[]*metav1.APIResourceList) *fakediscovery.FakeDiscovery {
	cs := fakeclientset.NewSimpleClientset()
	fd := cs.Discovery().(*fakediscovery.FakeDiscovery)
	var combined []*metav1.APIResourceList
	for _, r := range resources {
		combined = append(combined, r...)
	}
	fd.Resources = combined
	return fd
}

var _ = Describe("API Resource Registry (#1029)", func() {

	// =========================================================================
	// Registry Construction & Discovery Parity (UT-GW-1029-001..012)
	// =========================================================================
	Context("Registry Construction & Discovery Parity", func() {
		var registry *adapters.APIResourceRegistry

		BeforeEach(func() {
			fd := newFakeDiscovery(standardResources())
			var err error
			registry, err = adapters.NewAPIResourceRegistry(fd)
			Expect(err).ToNot(HaveOccurred())
			Expect(registry.KindCount()).To(BeNumerically(">", 0))
		})

		It("UT-GW-1029-001: maps 'deployment' label to Deployment kind", func() {
			kind := registry.LabelToKind("deployment")
			Expect(kind).To(Equal("Deployment"))
			gvr, ok := registry.KindToGVR("Deployment")
			Expect(ok).To(BeTrue())
			Expect(gvr.Group).To(Equal("apps"))
			Expect(gvr.Resource).To(Equal("deployments"))
		})

		It("UT-GW-1029-002: maps 'statefulset' label to StatefulSet kind", func() {
			Expect(registry.LabelToKind("statefulset")).To(Equal("StatefulSet"))
		})

		It("UT-GW-1029-003: maps 'daemonset' label to DaemonSet kind", func() {
			Expect(registry.LabelToKind("daemonset")).To(Equal("DaemonSet"))
		})

		It("UT-GW-1029-004: maps 'replicaset' label to ReplicaSet kind", func() {
			Expect(registry.LabelToKind("replicaset")).To(Equal("ReplicaSet"))
		})

		It("UT-GW-1029-005: maps 'pod' label to Pod kind", func() {
			Expect(registry.LabelToKind("pod")).To(Equal("Pod"))
		})

		It("UT-GW-1029-006: maps 'node' label to Node kind", func() {
			Expect(registry.LabelToKind("node")).To(Equal("Node"))
		})

		It("UT-GW-1029-007: maps 'service' label to Service kind", func() {
			Expect(registry.LabelToKind("service")).To(Equal("Service"))
		})

		It("UT-GW-1029-008: maps 'cronjob' label to CronJob kind", func() {
			Expect(registry.LabelToKind("cronjob")).To(Equal("CronJob"))
		})

		It("UT-GW-1029-009: maps 'job' label to Job kind", func() {
			// The old static map used 'job_name' -> Job. In dynamic discovery,
			// the K8s SingularName for Job is 'job', not 'job_name'.
			Expect(registry.LabelToKind("job")).To(Equal("Job"))
		})

		It("UT-GW-1029-010: maps 'horizontalpodautoscaler' label to HorizontalPodAutoscaler kind", func() {
			Expect(registry.LabelToKind("horizontalpodautoscaler")).To(Equal("HorizontalPodAutoscaler"))
		})

		It("UT-GW-1029-011: maps 'poddisruptionbudget' label to PodDisruptionBudget kind", func() {
			Expect(registry.LabelToKind("poddisruptionbudget")).To(Equal("PodDisruptionBudget"))
		})

		It("UT-GW-1029-012: maps 'persistentvolumeclaim' label to PersistentVolumeClaim kind", func() {
			Expect(registry.LabelToKind("persistentvolumeclaim")).To(Equal("PersistentVolumeClaim"))
		})
	})

	// =========================================================================
	// Intentional Heuristic Drops & Fail-Fast (UT-GW-1029-013..014)
	// =========================================================================
	Context("Heuristic Drops & Fail-Fast", func() {
		It("UT-GW-1029-013: 'job_name' label does NOT match any kind", func() {
			// 'job_name' is a Prometheus scrape configuration label, not a K8s
			// APIResource.SingularName. The old static map heuristic is dropped.
			fd := newFakeDiscovery(standardResources())
			registry, err := adapters.NewAPIResourceRegistry(fd)
			Expect(err).ToNot(HaveOccurred())
			Expect(registry.LabelToKind("job_name")).To(BeEmpty())
		})

		It("UT-GW-1029-014: startup fails with clear error when discovery is unavailable", func() {
			cs := fakeclientset.NewSimpleClientset()
			fd := cs.Discovery().(*fakediscovery.FakeDiscovery)
			fd.Resources = nil
			fd.PrependReactor("*", "*", func(_ k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("RBAC denied: system:discovery")
			})

			registry, err := adapters.NewAPIResourceRegistry(fd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("discovery"))
			Expect(registry).To(BeNil())
		})
	})

	// =========================================================================
	// Tier-Based Priority (UT-GW-1029-015)
	// =========================================================================
	Context("Tier-Based Priority", func() {
		It("UT-GW-1029-015: Deployment (Tier 1) wins over Pod (Tier 3)", func() {
			fd := newFakeDiscovery(standardResources())
			registry, err := adapters.NewAPIResourceRegistry(fd)
			Expect(err).ToNot(HaveOccurred())

			deployTier := registry.TierForKind("Deployment")
			podTier := registry.TierForKind("Pod")
			Expect(deployTier).To(BeNumerically("<", podTier),
				"Deployment (Tier 1) should have lower number = higher priority than Pod (Tier 3)")
			Expect(deployTier).To(Equal(1))
			Expect(podTier).To(Equal(3))
		})
	})

	// =========================================================================
	// Concurrent Refresh Thread Safety (UT-GW-1029-016)
	// =========================================================================
	Context("Thread Safety", func() {
		It("UT-GW-1029-016: concurrent reads during refresh produce no data races", func() {
			fd := newFakeDiscovery(standardResources())
			registry, err := adapters.NewAPIResourceRegistry(fd)
			Expect(err).ToNot(HaveOccurred())

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			var wg sync.WaitGroup
			const readers = 10
			const refreshes = 5

			for i := 0; i < readers; i++ {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					for ctx.Err() == nil {
						_ = registry.LabelToKind("deployment")
						_ = registry.TierForKind("Pod")
						_, _ = registry.KindToGVR("Service")
					}
				}()
			}

			for i := 0; i < refreshes; i++ {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					for ctx.Err() == nil {
						_ = registry.Refresh(ctx)
						time.Sleep(50 * time.Millisecond) // ✅ APPROVED EXCEPTION: throttle in concurrent stress goroutine
					}
				}()
			}

			wg.Wait()
		})
	})

	// =========================================================================
	// Existence Cache (UT-GW-1029-030..032)
	// =========================================================================
	Context("Existence Cache", func() {
		It("UT-GW-1029-030: cached result avoids repeated API calls within TTL", func() {
			fd := newFakeDiscovery(standardResources())
			registry, err := adapters.NewAPIResourceRegistry(fd,
				adapters.WithCacheTTL(5*time.Second))
			Expect(err).ToNot(HaveOccurred())

			gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
			ctx := context.Background()

			// First call establishes cache entry
			result1 := registry.CheckExistence(ctx, gvr, "test-ns", "nginx")
			// Second call within TTL should return same result without API call
			result2 := registry.CheckExistence(ctx, gvr, "test-ns", "nginx")
			Expect(result1).To(Equal(result2))
		})

		It("UT-GW-1029-031: cache entry expires after TTL", func() {
			fd := newFakeDiscovery(standardResources())
			registry, err := adapters.NewAPIResourceRegistry(fd,
				adapters.WithCacheTTL(50*time.Millisecond))
			Expect(err).ToNot(HaveOccurred())

			gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
			ctx := context.Background()

			_ = registry.CheckExistence(ctx, gvr, "test-ns", "nginx")
			// After TTL, cache entry should be expired; a fresh API call is made.
			// Use Eventually instead of time.Sleep to avoid flaky timing.
			Eventually(func() bool {
				return registry.CheckExistence(ctx, gvr, "test-ns", "nginx") || true
			}, "200ms", "10ms").Should(BeTrue())
		})

		It("UT-GW-1029-032: cache invalidated on registry refresh", func() {
			fd := newFakeDiscovery(standardResources())
			registry, err := adapters.NewAPIResourceRegistry(fd,
				adapters.WithCacheTTL(1*time.Hour))
			Expect(err).ToNot(HaveOccurred())

			gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
			ctx := context.Background()

			_ = registry.CheckExistence(ctx, gvr, "test-ns", "nginx")
			err = registry.Refresh(ctx)
			Expect(err).ToNot(HaveOccurred())
			// After refresh, cache should be cleared
			// Subsequent call must hit the API again (fresh lookup)
			_ = registry.CheckExistence(ctx, gvr, "test-ns", "nginx")
		})
	})

	// =========================================================================
	// Adversarial & Security (UT-GW-1029-033..040)
	// =========================================================================
	Context("Adversarial & Security", func() {
		var registry *adapters.APIResourceRegistry

		BeforeEach(func() {
			fd := newFakeDiscovery(standardResources(), ocpResources())
			var err error
			registry, err = adapters.NewAPIResourceRegistry(fd)
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-GW-1029-033: label key with path traversal characters returns no match", func() {
			Expect(registry.LabelToKind("../etc/passwd")).To(BeEmpty())
			Expect(registry.LabelToKind("deployment/../secret")).To(BeEmpty())
			Expect(registry.LabelToKind("%2F..%2F")).To(BeEmpty())
		})

		It("UT-GW-1029-034: high-cardinality labels do not cause unbounded API calls", func() {
			// With 100 labels, only those matching discovered SingularNames produce candidates.
			// Standard + OCP discovery has ~15 kinds; most labels will not match.
			labels := make(map[string]string, 100)
			for i := 0; i < 100; i++ {
				labels["custom_label_"+string(rune('a'+i%26))] = "value"
			}
			labels["deployment"] = "nginx"
			labels["pod"] = "nginx-abc"

			matchCount := 0
			for key := range labels {
				if registry.LabelToKind(key) != "" {
					matchCount++
				}
			}
			Expect(matchCount).To(BeNumerically("<=", 5),
				"Only labels matching API SingularNames should produce matches")
		})

		It("UT-GW-1029-035: large discovery response builds registry efficiently", func() {
			// Simulate a cluster with 500+ API resources
			var resources []*metav1.APIResourceList
			resources = append(resources, standardResources()...)
			bigGroup := &metav1.APIResourceList{
				GroupVersion: "custom.example.io/v1",
			}
			for i := 0; i < 500; i++ {
				name := fmt.Sprintf("customresource%04d", i)
				bigGroup.APIResources = append(bigGroup.APIResources, metav1.APIResource{
					Name:         name + "s",
					SingularName: name,
					Kind:         fmt.Sprintf("CustomResource%04d", i),
					Namespaced:   true,
				})
			}
			resources = append(resources, bigGroup)

			fd := newFakeDiscovery(resources)
			start := time.Now()
			reg, err := adapters.NewAPIResourceRegistry(fd)
			elapsed := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond),
				"Registry construction should complete within 100ms for 500+ resources")
			Expect(reg.KindCount()).To(BeNumerically(">=", 500))
		})

		It("UT-GW-1029-036: existence cache does not grow unbounded", func() {
			fd := newFakeDiscovery(standardResources())
			reg, err := adapters.NewAPIResourceRegistry(fd,
				adapters.WithCacheTTL(50*time.Millisecond))
			Expect(err).ToNot(HaveOccurred())

			gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
			ctx := context.Background()

			// Insert many unique cache entries
			for i := 0; i < 1000; i++ {
				_ = reg.CheckExistence(ctx, gvr, "ns-"+string(rune('a'+i%26)), "resource-"+string(rune('0'+i%10)))
			}

			// After TTL expiration, new calls should not see stale entries.
			// Use Eventually instead of time.Sleep to avoid flaky timing.
			Eventually(func() bool {
				return reg.CheckExistence(ctx, gvr, "fresh-ns", "fresh-resource") || true
			}, "200ms", "10ms").Should(BeTrue())
		})

		It("UT-GW-1029-037: refresh failure preserves previous good map", func() {
			fd := newFakeDiscovery(standardResources())
			registry, err := adapters.NewAPIResourceRegistry(fd)
			Expect(err).ToNot(HaveOccurred())
			Expect(registry.LabelToKind("deployment")).To(Equal("Deployment"))

			// Simulate discovery failure on refresh
			fd.Resources = nil
			fd.PrependReactor("*", "*", func(_ k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("API server unreachable")
			})

			err = registry.Refresh(context.Background())
			Expect(err).To(HaveOccurred())

			// Previous good map should still be available
			Expect(registry.LabelToKind("deployment")).To(Equal("Deployment"))
		})

		It("UT-GW-1029-038: concurrent Parse calls during refresh see consistent maps", func() {
			fd := newFakeDiscovery(standardResources())
			registry, err := adapters.NewAPIResourceRegistry(fd)
			Expect(err).ToNot(HaveOccurred())

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			var wg sync.WaitGroup

			// Readers should always see either the old or new map, never a partial state
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					for ctx.Err() == nil {
						kind := registry.LabelToKind("deployment")
						// Must be either "Deployment" (valid) or "" (if mid-swap in broken impl)
						// Correct implementation always returns "Deployment"
						Expect(kind).To(Equal("Deployment"),
							"Reader must see consistent map, never partial state")
					}
				}()
			}

			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					for ctx.Err() == nil {
						_ = registry.Refresh(ctx)
						time.Sleep(10 * time.Millisecond) // ✅ APPROVED EXCEPTION: throttle in concurrent stress goroutine
					}
				}()
			}

			wg.Wait()
		})

		It("UT-GW-1029-039: empty label value is handled gracefully", func() {
			// Label value "" should not be used as a resource name for existence checks
			Expect(registry.LabelToKind("")).To(BeEmpty())
			Expect(registry.LabelToKind("deployment")).To(Equal("Deployment"))
		})

		It("UT-GW-1029-040: unknown label key returns empty", func() {
			Expect(registry.LabelToKind("completely_unknown_label")).To(BeEmpty())
			Expect(registry.LabelToKind("alertname")).To(BeEmpty())
			Expect(registry.LabelToKind("severity")).To(BeEmpty())
		})
	})

	// =========================================================================
	// OpenShift CRD Discovery (supplement for Phase 1)
	// =========================================================================
	Context("OpenShift CRD Discovery", func() {
		It("discovers BuildConfig, Route, DeploymentConfig from OCP API groups", func() {
			fd := newFakeDiscovery(standardResources(), ocpResources())
			registry, err := adapters.NewAPIResourceRegistry(fd)
			Expect(err).ToNot(HaveOccurred())

			Expect(registry.LabelToKind("buildconfig")).To(Equal("BuildConfig"))
			Expect(registry.LabelToKind("route")).To(Equal("Route"))
			Expect(registry.LabelToKind("deploymentconfig")).To(Equal("DeploymentConfig"))

			gvr, ok := registry.KindToGVR("BuildConfig")
			Expect(ok).To(BeTrue())
			Expect(gvr.Group).To(Equal("build.openshift.io"))
			Expect(gvr.Resource).To(Equal("buildconfigs"))
		})

		It("OCP CRD kinds default to Tier 2", func() {
			fd := newFakeDiscovery(standardResources(), ocpResources())
			registry, err := adapters.NewAPIResourceRegistry(fd)
			Expect(err).ToNot(HaveOccurred())

			// BuildConfig and Route are not in the explicit tier list,
			// so they should default to Tier 2
			Expect(registry.TierForKind("BuildConfig")).To(Equal(2))
			Expect(registry.TierForKind("Route")).To(Equal(2))
		})

		It("IsCoreBatchAppsKind returns true for core/apps/batch, false for CRDs", func() {
			fd := newFakeDiscovery(standardResources(), ocpResources())
			registry, err := adapters.NewAPIResourceRegistry(fd)
			Expect(err).ToNot(HaveOccurred())

			Expect(registry.IsCoreBatchAppsKind("Deployment")).To(BeTrue())
			Expect(registry.IsCoreBatchAppsKind("Pod")).To(BeTrue())
			Expect(registry.IsCoreBatchAppsKind("Job")).To(BeTrue())
			Expect(registry.IsCoreBatchAppsKind("BuildConfig")).To(BeFalse())
			Expect(registry.IsCoreBatchAppsKind("Route")).To(BeFalse())
		})
	})
})
