package tools_test

import (
	"context"
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
})
