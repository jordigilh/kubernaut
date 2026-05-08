package tools_test

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

func newUnstructuredWithOwner(apiVersion, kind, ns, name, ownerKind, ownerName string, controller bool) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
		},
	}
	if ownerKind != "" {
		if meta, ok := obj.Object["metadata"].(map[string]interface{}); ok {
			meta["ownerReferences"] = []interface{}{
				map[string]interface{}{
					"kind":       ownerKind,
					"name":       ownerName,
					"controller": controller,
				},
			}
		}
	}
	return obj
}

var _ = Describe("af_resolve_owner", func() {
	var (
		podGVR    = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
		rsGVR     = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
		deployGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	)

	It("UT-AF-052-030: resolves Pod -> ReplicaSet -> Deployment chain", func() {
		pod := newUnstructuredWithOwner("v1", "Pod", "ns", "web-abc-123", "ReplicaSet", "web-abc", true)
		rs := newUnstructuredWithOwner("apps/v1", "ReplicaSet", "ns", "web-abc", "Deployment", "web", true)
		deploy := newUnstructuredWithOwner("apps/v1", "Deployment", "ns", "web", "", "", false)

		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				podGVR:    "PodList",
				rsGVR:     "ReplicaSetList",
				deployGVR: "DeploymentList",
			},
			pod, rs, deploy,
		)

		result, err := tools.HandleResolveOwner(context.Background(), client, tools.ResolveOwnerArgs{
			Namespace: "ns", Kind: "Pod", Name: "web-abc-123",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Chain).To(HaveLen(3))
		Expect(result.RootKind).To(Equal("Deployment"))
		Expect(result.RootName).To(Equal("web"))
	})

	It("UT-AF-052-031: single resource with no owner returns chain of 1", func() {
		deploy := newUnstructuredWithOwner("apps/v1", "Deployment", "ns", "standalone", "", "", false)

		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{deployGVR: "DeploymentList"},
			deploy,
		)

		result, err := tools.HandleResolveOwner(context.Background(), client, tools.ResolveOwnerArgs{
			Namespace: "ns", Kind: "Deployment", Name: "standalone",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Chain).To(HaveLen(1))
		Expect(result.RootKind).To(Equal("Deployment"))
	})

	It("UT-AF-052-032: unsupported kind as starting point returns error", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)

		_, err := tools.HandleResolveOwner(context.Background(), client, tools.ResolveOwnerArgs{
			Namespace: "ns", Kind: "CustomThing", Name: "my-custom",
		})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("unsupported kind")))
	})

	It("UT-AF-052-033: empty namespace rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)

		_, err := tools.HandleResolveOwner(context.Background(), client, tools.ResolveOwnerArgs{
			Namespace: "", Kind: "Pod", Name: "foo",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-034: empty name rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)

		_, err := tools.HandleResolveOwner(context.Background(), client, tools.ResolveOwnerArgs{
			Namespace: "ns", Kind: "Pod", Name: "",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-035: empty kind rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)

		_, err := tools.HandleResolveOwner(context.Background(), client, tools.ResolveOwnerArgs{
			Namespace: "ns", Kind: "", Name: "foo",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-036: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleResolveOwner(context.Background(), nil, tools.ResolveOwnerArgs{
			Namespace: "ns", Kind: "Pod", Name: "foo",
		})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-052-037: resource not found at start returns error", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{podGVR: "PodList"})

		_, err := tools.HandleResolveOwner(context.Background(), client, tools.ResolveOwnerArgs{
			Namespace: "ns", Kind: "Pod", Name: "nonexistent",
		})
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-052-038: concurrent calls are safe", func() {
		deploy := newUnstructuredWithOwner("apps/v1", "Deployment", "ns", "web", "", "", false)
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{deployGVR: "DeploymentList"},
			deploy,
		)

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := tools.HandleResolveOwner(context.Background(), client, tools.ResolveOwnerArgs{
					Namespace: "ns", Kind: "Deployment", Name: "web",
				})
				Expect(err).NotTo(HaveOccurred())
			}()
		}
		wg.Wait()
	})

	It("UT-AF-052-039: cycle detection stops traversal (A->B->A)", func() {
		rsA := newUnstructuredWithOwner("apps/v1", "ReplicaSet", "ns", "rs-a", "ReplicaSet", "rs-b", true)
		rsB := newUnstructuredWithOwner("apps/v1", "ReplicaSet", "ns", "rs-b", "ReplicaSet", "rs-a", true)

		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rsGVR: "ReplicaSetList"},
			rsA, rsB,
		)

		result, err := tools.HandleResolveOwner(context.Background(), client, tools.ResolveOwnerArgs{
			Namespace: "ns", Kind: "ReplicaSet", Name: "rs-a",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Chain).To(HaveLen(2))
		Expect(result.RootKind).To(Equal("ReplicaSet"))
	})

	It("UT-AF-052-040: unsupported kind mid-chain returns partial result", func() {
		pod := newUnstructuredWithOwner("v1", "Pod", "ns", "job-pod", "Job", "my-job", true)
		job := newUnstructuredWithOwner("batch/v1", "Job", "ns", "my-job", "CustomKind", "owner", true)

		scheme := runtime.NewScheme()
		jobGVR := schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				podGVR: "PodList",
				jobGVR: "JobList",
			},
			pod, job,
		)

		result, err := tools.HandleResolveOwner(context.Background(), client, tools.ResolveOwnerArgs{
			Namespace: "ns", Kind: "Pod", Name: "job-pod",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Chain).To(HaveLen(2))
		Expect(result.RootKind).To(Equal("Job"))
		Expect(result.RootName).To(Equal("my-job"))
	})

	It("UT-AF-052-041: max depth (10) stops traversal with partial chain", func() {
		scheme := runtime.NewScheme()
		objs := make([]runtime.Object, 12)
		for i := 0; i < 12; i++ {
			name := fmt.Sprintf("rs-%d", i)
			ownerName := ""
			if i < 11 {
				ownerName = fmt.Sprintf("rs-%d", i+1)
			}
			objs[i] = newUnstructuredWithOwner("apps/v1", "ReplicaSet", "ns", name, "ReplicaSet", ownerName, true)
		}

		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rsGVR: "ReplicaSetList"},
			objs...,
		)

		result, err := tools.HandleResolveOwner(context.Background(), client, tools.ResolveOwnerArgs{
			Namespace: "ns", Kind: "ReplicaSet", Name: "rs-0",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(result.Chain)).To(BeNumerically("<=", 10))
	})

	It("UT-AF-052-042: prefers controller owner over non-controller", func() {
		pod := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      "multi-owner",
					"namespace": "ns",
					"ownerReferences": []interface{}{
						map[string]interface{}{
							"kind":       "ReplicaSet",
							"name":       "non-controller-rs",
							"controller": false,
						},
						map[string]interface{}{
							"kind":       "ReplicaSet",
							"name":       "controller-rs",
							"controller": true,
						},
					},
				},
			},
		}
		controllerRS := newUnstructuredWithOwner("apps/v1", "ReplicaSet", "ns", "controller-rs", "", "", false)

		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				podGVR: "PodList",
				rsGVR:  "ReplicaSetList",
			},
			pod, controllerRS,
		)

		result, err := tools.HandleResolveOwner(context.Background(), client, tools.ResolveOwnerArgs{
			Namespace: "ns", Kind: "Pod", Name: "multi-owner",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Chain).To(HaveLen(2))
		Expect(result.Chain[1].Name).To(Equal("controller-rs"))
	})
})
