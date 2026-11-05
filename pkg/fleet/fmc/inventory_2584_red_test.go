package fmc_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("IT-FLEET-2584-001: FMC canonical inventory projection", func() {
	It("returns only the canonical managed-resource page and preserves cursor/freshness", func() {
		observedAt := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodGet))
			Expect(r.URL.Path).To(Equal(fmc.InventoryPath))
			Expect(r.URL.Query().Get("cluster_id")).To(Equal("prod-east"))
			Expect(r.URL.Query().Get("limit")).To(Equal("2"))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(inventory.Page{
				State:        inventory.StateSuccess,
				ObservedAt:   observedAt,
				Completeness: inventory.CompletenessComplete,
				Freshness: inventory.Freshness{
					State: inventory.FreshnessFresh,
					AsOf:  observedAt,
				},
				NextCursor: "opaque-next-cursor",
				Items: []inventory.Resource{{
					ID:        "opaque-resource-id",
					ClusterID: "prod-east",
					Group:     "apps",
					Version:   "v1",
					Kind:      "Deployment",
					Namespace: "payments",
					Name:      "checkout",
					Scope: inventory.Scope{
						Included:       true,
						ResourceLabel:  inventory.LabelTrue,
						NamespaceLabel: inventory.LabelAbsent,
						Source:         inventory.ScopeSourceResourceLabel,
						ClusterScoped:  false,
					},
					ObservedAt: observedAt,
				}},
			})
		}))
		DeferCleanup(server.Close)

		client := fmc.NewHTTPClient(server.URL)
		page, err := client.ListManagedResources(context.Background(), inventory.Filters{
			ClusterID: "prod-east",
			Limit:     2,
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].Name).To(Equal("checkout"))
		Expect(page.Items[0].Scope.Source).To(Equal(inventory.ScopeSourceResourceLabel))
		Expect(page.NextCursor).To(Equal("opaque-next-cursor"))
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh))
	})

	It("rejects a widening page limit before making a control-plane request", func() {
		client := fmc.NewHTTPClient("http://127.0.0.1:1")
		_, err := client.ListManagedResources(context.Background(), inventory.Filters{
			ClusterID: "prod-east",
			Limit:     101,
		})

		Expect(err).To(MatchError(ContainSubstring("limit")))
	})

	It("BR-SECURITY-016: sends and validates the cluster on exact reads", func() {
		cluster := "prod-east"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Query().Get("cluster_id")).To(Equal("prod-east"))
			Expect(r.URL.Path).To(Equal(fmc.InventoryPath + "/opaque-id"))
			Expect(json.NewEncoder(w).Encode(inventory.Resource{ID: "opaque-id", ClusterID: cluster})).To(Succeed())
		}))
		DeferCleanup(server.Close)
		client := fmc.NewHTTPClient(server.URL)
		resource, err := client.Get(context.Background(), "prod-east", "opaque-id")
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.ClusterID).To(Equal("prod-east"))
		cluster = "other-east"
		resource, err = client.Get(context.Background(), "prod-east", "opaque-id")
		Expect(err).To(MatchError(ContainSubstring("mismatched resource identity")))
		Expect(resource.ID).To(BeEmpty())
	})

	It("UT-FMC-2584-STRICT-001 [BR-SECURITY-016]: strict scope check preserves backend failure", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		DeferCleanup(server.Close)
		client := fmc.NewHTTPClient(server.URL)
		strict, ok := interface{}(client).(scope.StrictScopeChecker)
		Expect(ok).To(BeTrue())
		managed, err := strict.IsManagedResourceStrict(context.Background(), scope.ResourceIdentity{ClusterID: "prod-east", Kind: "Deployment", Name: "checkout"})
		Expect(managed).To(BeFalse())
		Expect(err).To(HaveOccurred())
		managed, err = client.IsManagedResource(context.Background(), scope.ResourceIdentity{ClusterID: "prod-east", Kind: "Deployment", Name: "checkout"})
		Expect(err).NotTo(HaveOccurred())
		Expect(managed).To(BeFalse())
	})
})
