package policyread_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metadatafake "k8s.io/client-go/metadata/fake"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("BR-SCOPE-001 / UT-AF-2584-005/007: local inventory projection", func() {
	It("uses the allowlist, preserves scope provenance, and paginates by opaque IDs", func() {
		scheme := runtime.NewScheme()
		Expect(metav1.AddMetaToScheme(scheme)).To(Succeed())
		client := metadatafake.NewSimpleMetadataClient(scheme, metadataDeployment("alpha"), metadataDeployment("beta"))
		resolver := &localScopeStub{}
		reader, err := policyread.NewLocalInventoryReader(policyread.LocalInventoryConfig{
			MetadataClient: client,
			ScopeResolver:  resolver,
			Now:            func() time.Time { return time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC) },
			Kinds: []policyread.LocalResourceKind{{
				Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true,
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		page1, err := reader.List(context.Background(), inventory.Filters{ClusterID: "local", Limit: 1})
		Expect(err).NotTo(HaveOccurred())
		Expect(page1.State).To(Equal(inventory.StateSuccess))
		Expect(page1.Items).To(HaveLen(1))
		Expect(page1.Items[0].Scope.Source).To(Equal(inventory.ScopeSourceResourceLabel))
		Expect(page1.Items[0].ID).To(HavePrefix("r-"))
		Expect(page1.NextCursor).NotTo(BeEmpty())
		Expect(page1.Items[0].Name).NotTo(Equal("should-not-appear"))

		page2, err := reader.List(context.Background(), inventory.Filters{ClusterID: "local", Limit: 1, Cursor: page1.NextCursor})
		Expect(err).NotTo(HaveOccurred())
		Expect(page2.Items).To(HaveLen(1))
		Expect(page2.Items[0].Name).NotTo(Equal(page1.Items[0].Name))
		Expect(page2.NextCursor).To(BeEmpty())

		resource, err := reader.Get(context.Background(), "local", page1.Items[0].ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.ID).To(Equal(page1.Items[0].ID))
	})

	It("reuses Namespace label resolution within one list operation", func() {
		scheme := runtime.NewScheme()
		Expect(metav1.AddMetaToScheme(scheme)).To(Succeed())
		namespace := &metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
			ObjectMeta: metav1.ObjectMeta{
				Name:   "payments",
				Labels: map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueTrue},
			},
		}
		alpha := metadataDeployment("alpha")
		alpha.Labels = nil
		beta := metadataDeployment("beta")
		beta.Labels = nil
		client := metadatafake.NewSimpleMetadataClient(scheme, namespace, alpha, beta)
		reader, err := policyread.NewLocalInventoryReader(policyread.LocalInventoryConfig{
			MetadataClient: client,
			ScopeResolver:  scope.NewManager(crfake.NewClientBuilder().WithScheme(scheme).Build()),
			Kinds: []policyread.LocalResourceKind{{
				Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true,
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		page, err := reader.List(context.Background(), inventory.Filters{ClusterID: "local", Limit: 10})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(2))

		namespaceGets := 0
		for _, action := range client.Actions() {
			if action.GetVerb() == "get" && action.GetResource().Resource == "namespaces" {
				namespaceGets++
			}
		}
		Expect(namespaceGets).To(Equal(1))
	})
})

func metadataDeployment(name string) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "payments",
			Labels: map[string]string{"kubernaut.ai/managed": "true"},
		},
	}
}

type localScopeStub struct{}

func (*localScopeStub) Resolve(_ context.Context, resource scope.ResourceIdentity) (scope.Resolution, error) {
	return scope.Resolution{
		Included:       true,
		ResourceLabel:  scope.LabelStateTrue,
		NamespaceLabel: scope.LabelStateTrue,
		Source:         scope.ScopeSourceResourceLabel,
		ClusterScoped:  false,
	}, nil
}

var _ policyread.ScopeResolver = (*localScopeStub)(nil)
