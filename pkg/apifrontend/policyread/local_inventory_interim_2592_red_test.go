package policyread_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metadatafake "k8s.io/client-go/metadata/fake"
)

var _ = Describe("BR-API-001: drift-tolerant local inventory pagination", func() {
	It("UT-AF-2592-CURSOR-DRIFT-001 [BR-API-001]: re-queries after an insert and tolerates deletion of the cursor resource", func() {
		deploymentGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
		scheme := runtime.NewScheme()
		Expect(metav1.AddMetaToScheme(scheme)).To(Succeed())
		client := metadatafake.NewSimpleMetadataClient(scheme, metadataDeployment("alpha"), metadataDeployment("gamma"))
		reader, err := policyread.NewLocalInventoryReader(policyread.LocalInventoryConfig{
			MetadataClient: client,
			ScopeResolver:  &localScopeStub{},
			Now:            func() time.Time { return time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC) },
			Kinds: []policyread.LocalResourceKind{{
				Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true,
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		first, err := reader.List(context.Background(), inventory.Filters{ClusterID: "local", Limit: 1})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Items).To(HaveLen(1))
		Expect(first.Items[0].Name).To(Equal("alpha"))
		cursor := first.NextCursor
		Expect(cursor).NotTo(BeEmpty())

		metadataResource, ok := client.Resource(deploymentGVR).Namespace("payments").(metadatafake.MetadataClient)
		Expect(ok).To(BeTrue())
		_, err = metadataResource.CreateFake(metadataDeployment("beta"), metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		second, err := reader.List(context.Background(), inventory.Filters{ClusterID: "local", Limit: 1, Cursor: cursor})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.State).To(Equal(inventory.StateSuccess))
		Expect(second.Items).To(HaveLen(1))
		Expect(second.Items[0].Name).To(Equal("beta"), "the next request must observe the current sorted local data")

		Expect(metadataResource.Delete(
			context.Background(), "alpha", metav1.DeleteOptions{},
		)).To(Succeed())

		afterDelete, err := reader.List(context.Background(), inventory.Filters{ClusterID: "local", Limit: 1, Cursor: cursor})
		Expect(err).NotTo(HaveOccurred(), "a deleted cursor is ordinary backend drift, not an invalid request")
		Expect(afterDelete.State).To(Equal(inventory.StateSuccess))
		Expect(afterDelete.Items).To(HaveLen(1))
		Expect(afterDelete.Items[0].Name).To(Equal("beta"))
	})
})
