package acm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/acm"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

const emptySearchResponse = `{"data":{"searchResult":[{"count":0,"items":[]}]}}`

var _ = Describe("IT-FLEET-2584-002: ACM canonical inventory projection", func() {
	It("UT-ACM-2593-IDENTITY-001 [BR-SCOPE-001]: projects a live-shaped managed resource without exposing its ACM UID", func() {
		var requestBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			body, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			if respondNamespaceFixture(w, body) {
				return
			}
			requestBody = body
			_, err = io.WriteString(w, `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","apiversion":"v1","kind":"ConfigMap","namespace":"payments","name":"checkout","label":"other=value; kubernaut.ai/managed=true","_uid":"prod-east/12345"}]}]}}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)

		page, err := acm.NewClient(server.URL).List(context.Background(), inventory.Filters{ClusterID: "prod-east"})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].Scope.Included).To(BeTrue())
		Expect(page.Items[0].Scope.ResourceLabel).To(Equal(inventory.LabelTrue))
		Expect(page.Items[0].ID).To(Equal(inventory.CanonicalResourceID(scope.ResourceIdentity{ClusterID: "prod-east", Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "checkout"})))
		Expect(page.Items[0].ID).NotTo(ContainSubstring("/"))
		Expect(string(requestBody)).To(ContainSubstring(`"property":"label","values":["kubernaut.ai/managed=true"]`))
	})

	It("UT-ACM-2593-IDENTITY-002 [BR-SECURITY-016]: rejects a Search item without its own cluster identity", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := io.WriteString(w, `{"data":{"searchResult":[{"count":1,"items":[{"apiversion":"v1","kind":"ConfigMap","namespace":"payments","name":"checkout","label":"kubernaut.ai/managed=true"}]}]}}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)

		page, err := acm.NewClient(server.URL).List(context.Background(), inventory.Filters{ClusterID: "prod-east"})
		Expect(err).To(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
	})

	It("UT-ACM-2593-UNION-001 [BR-SCOPE-001]: combines namespace inheritance with explicit resource overrides", func() {
		objects := []map[string]string{
			{"cluster": "prod-east", "apiversion": "v1", "kind": "Namespace", "name": "payments", "label": "kubernaut.ai/managed=true", "_uid": "prod-east/ns-payments"},
			{"cluster": "prod-east", "apiversion": "v1", "kind": "Namespace", "name": "staging", "label": "kubernaut.ai/managed=false", "_uid": "prod-east/ns-staging"},
			{"cluster": "prod-east", "apiversion": "v1", "kind": "ConfigMap", "namespace": "payments", "name": "inherited", "_uid": "prod-east/inherited"},
			{"cluster": "prod-east", "apiversion": "v1", "kind": "ConfigMap", "namespace": "payments", "name": "invalid-inherits", "label": "kubernaut.ai/managed=invalid", "_uid": "prod-east/invalid"},
			{"cluster": "prod-east", "apiversion": "v1", "kind": "ConfigMap", "namespace": "payments", "name": "opted-out", "label": "kubernaut.ai/managed=false", "_uid": "prod-east/opted-out"},
			{"cluster": "prod-east", "apiversion": "v1", "kind": "ConfigMap", "namespace": "payments", "name": "opted-in", "label": "kubernaut.ai/managed=true", "_uid": "prod-east/opted-in"},
			{"cluster": "prod-east", "apiversion": "v1", "kind": "ConfigMap", "namespace": "staging", "name": "explicit-override", "label": "kubernaut.ai/managed=true", "_uid": "prod-east/override"},
			{"cluster": "prod-east", "apiversion": "v1", "kind": "ConfigMap", "namespace": "staging", "name": "unmanaged", "_uid": "prod-east/unmanaged"},
			{"cluster": "prod-east", "apiversion": "v1", "kind": "Node", "name": "worker", "label": "kubernaut.ai/managed=true", "_uid": "prod-east/worker"},
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request struct {
				Variables struct {
					Input []struct {
						Filters []struct {
							Property string   `json:"property"`
							Values   []string `json:"values"`
						} `json:"filters"`
					} `json:"input"`
				} `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Variables.Input) != 1 {
				http.Error(w, "invalid Search request", http.StatusBadRequest)
				return
			}
			items := make([]map[string]string, 0, len(objects))
			for _, object := range objects {
				matches := true
				for _, filter := range request.Variables.Input[0].Filters {
					if len(filter.Values) != 1 || object[filter.Property] != filter.Values[0] {
						matches = false
						break
					}
				}
				if matches {
					items = append(items, object)
				}
			}
			if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"searchResult": []interface{}{map[string]interface{}{"count": len(items), "items": items}}}}); err != nil {
				GinkgoWriter.Printf("write Search response: %v\n", err)
			}
		}))
		DeferCleanup(server.Close)

		page, err := acm.NewClient(server.URL).List(context.Background(), inventory.Filters{ClusterID: "prod-east", Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		var names []string
		for _, item := range page.Items {
			Expect(item.ClusterID).To(Equal("prod-east"))
			Expect(item.Scope.Included).To(BeTrue())
			Expect(item.ID).To(HavePrefix("r-"))
			Expect(item.ID).NotTo(ContainSubstring("/"))
			names = append(names, item.Name)
		}
		Expect(names).To(ConsistOf("payments", "inherited", "invalid-inherits", "opted-in", "explicit-override", "worker"))

		first, err := acm.NewClient(server.URL).List(context.Background(), inventory.Filters{ClusterID: "prod-east", Limit: 2})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Items).To(HaveLen(2))
		Expect(first.NextCursor).NotTo(BeEmpty())
		second, err := acm.NewClient(server.URL).List(context.Background(), inventory.Filters{ClusterID: "prod-east", Limit: 2, Cursor: first.NextCursor})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Items).To(HaveLen(2))
		Expect(second.Items[0].ID).NotTo(Equal(first.Items[0].ID))
		Expect(second.Items[0].ID).NotTo(Equal(first.Items[1].ID))

		filtered, err := acm.NewClient(server.URL).List(context.Background(), inventory.Filters{ClusterID: "prod-east", Namespace: "payments", Kind: "ConfigMap", Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		filteredNames := make([]string, 0, len(filtered.Items))
		for _, item := range filtered.Items {
			filteredNames = append(filteredNames, item.Name)
		}
		Expect(filteredNames).To(ConsistOf("inherited", "invalid-inherits", "opted-in"))
		invalidPage, err := acm.NewClient(server.URL).List(context.Background(), inventory.Filters{ClusterID: "prod-east", Cursor: "r-missing", Limit: 2})
		Expect(err).To(MatchError(ContainSubstring("cursor is not present")))
		Expect(invalidPage.State).To(Equal(inventory.StateUnavailable))
		Expect(invalidPage.Items).To(BeEmpty())

		inheritedID := inventory.CanonicalResourceID(scope.ResourceIdentity{ClusterID: "prod-east", Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "inherited"})
		inherited, err := acm.NewClient(server.URL).Get(context.Background(), "prod-east", inheritedID)
		Expect(err).NotTo(HaveOccurred())
		Expect(inherited.Scope.Source).To(Equal(inventory.ScopeSourceNamespaceLabel))
		Expect(inherited.Name).To(Equal("inherited"))
	})

	It("UT-ACM-2593-VISIBILITY-001 [BR-SECURITY-016]: refuses an explicit resource when its namespace cannot be verified", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			response := `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","apiversion":"v1","kind":"ConfigMap","namespace":"payments","name":"checkout","label":"kubernaut.ai/managed=true"}]}]}}`
			if strings.Contains(string(body), `"property":"kind","values":["Namespace"]`) {
				response = emptySearchResponse
			}
			_, err = io.WriteString(w, response)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		page, err := acm.NewClient(server.URL).List(context.Background(), inventory.Filters{ClusterID: "prod-east"})
		Expect(err).To(MatchError(ContainSubstring("namespace is not uniquely visible")))
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
	})

	It("UT-ACM-2593-CONSISTENCY-001 [BR-SCOPE-001]: rejects conflicting labels across Search queries", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			response := `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","apiversion":"v1","kind":"ConfigMap","namespace":"payments","name":"checkout","label":"kubernaut.ai/managed=true"}]}]}}`
			switch {
			case strings.Contains(string(body), `"property":"kind","values":["Namespace"]`):
				response = `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","apiversion":"v1","kind":"Namespace","name":"payments","label":"kubernaut.ai/managed=true"}]}]}}`
			case strings.Contains(string(body), `"property":"namespace","values":["payments"]`):
				response = `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","apiversion":"v1","kind":"ConfigMap","namespace":"payments","name":"checkout","label":"kubernaut.ai/managed=false"}]}]}}`
			}
			_, err = io.WriteString(w, response)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)

		page, err := acm.NewClient(server.URL).List(context.Background(), inventory.Filters{ClusterID: "prod-east"})
		Expect(err).To(MatchError(ContainSubstring("inconsistent resource labels")))
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
	})

	It("UT-ACM-2593-BOUND-001 [BR-SCOPE-001]: fails closed when an ACM Search batch fills the traversal bound", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			items := make([]map[string]string, 1000)
			for i := range items {
				items[i] = map[string]string{"cluster": "prod-east", "apiversion": "v1", "kind": "Namespace", "name": fmt.Sprintf("ns-%d", i), "label": "kubernaut.ai/managed=true"}
			}
			if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"searchResult": []interface{}{map[string]interface{}{"count": len(items), "items": items}}}}); err != nil {
				GinkgoWriter.Printf("write Search response: %v\n", err)
			}
		}))
		DeferCleanup(server.Close)

		page, err := acm.NewClient(server.URL).List(context.Background(), inventory.Filters{ClusterID: "prod-east"})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StatePartial))
		Expect(page.Items).To(BeEmpty())
		_, err = acm.NewClient(server.URL).Get(context.Background(), "prod-east", "opaque-id")
		Expect(err).To(MatchError(inventory.ErrUnavailable))
	})

	It("UT-ACM-2593-FILTER-001 [BR-SCOPE-001]: applies namespace, name and kind filters before the Search bound", func() {
		objects := []map[string]string{
			{"cluster": "prod-east", "apiversion": "v1", "kind": "Namespace", "name": "payments", "label": "kubernaut.ai/managed=true"},
			{"cluster": "prod-east", "apiversion": "v1", "kind": "Deployment", "namespace": "payments", "name": "checkout", "label": "kubernaut.ai/managed=true"},
		}
		for i := 0; i < 1000; i++ {
			objects = append(objects, map[string]string{"cluster": "prod-east", "apiversion": "v1", "kind": "ConfigMap", "namespace": "other", "name": fmt.Sprintf("noise-%d", i), "label": "kubernaut.ai/managed=true"})
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request struct {
				Variables struct {
					Input []struct {
						Limit   int `json:"limit"`
						Filters []struct {
							Property string   `json:"property"`
							Values   []string `json:"values"`
						} `json:"filters"`
					} `json:"input"`
				} `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Variables.Input) != 1 {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			input := request.Variables.Input[0]
			items := make([]map[string]string, 0)
			for _, object := range objects {
				matches := true
				for _, filter := range input.Filters {
					if len(filter.Values) != 1 || object[filter.Property] != filter.Values[0] {
						matches = false
						break
					}
				}
				if matches {
					items = append(items, object)
				}
			}
			count := len(items)
			if input.Limit > 0 && len(items) > input.Limit {
				items = items[:input.Limit]
			}
			if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"searchResult": []interface{}{map[string]interface{}{"count": count, "items": items}}}}); err != nil {
				GinkgoWriter.Printf("write Search response: %v\n", err)
			}
		}))
		DeferCleanup(server.Close)

		for _, filters := range []inventory.Filters{
			{ClusterID: "prod-east", Namespace: "payments"},
			{ClusterID: "prod-east", Name: "checkout"},
			{ClusterID: "prod-east", Kind: "Deployment"},
		} {
			page, err := acm.NewClient(server.URL).List(context.Background(), filters)
			Expect(err).NotTo(HaveOccurred())
			Expect(page.State).To(Equal(inventory.StateSuccess), "filter %+v should narrow ACM Search before traversal", filters)
			Expect(page.Items).To(HaveLen(1))
			Expect(page.Items[0].Name).To(Equal("checkout"))
		}
	})
	It("UT-ACM-2584-LIVE-003 [BR-SECURITY-016]: returns partial without records when Search reports truncated items", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := io.WriteString(w, `{"data":{"searchResult":[{"count":2,"items":[{"cluster":"prod-east","apigroup":"apps","apiversion":"v1","kind":"Deployment","namespace":"payments","name":"checkout","label.kubernaut.ai/managed":"true"}]}]}}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		page, err := acm.NewClient(server.URL).ListManagedResources(context.Background(), inventory.Filters{ClusterID: "prod-east", Limit: 1})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StatePartial))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
	})

	It("UT-ACM-2584-LIVE-007 [BR-SECURITY-016]: never treats a truncated exact Search as not found", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := io.WriteString(w, `{"data":{"searchResult":[{"count":2,"items":[]}]}}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		_, err := acm.NewClient(server.URL).Get(context.Background(), "prod-east", "opaque-id")
		Expect(err).To(MatchError(inventory.ErrUnavailable))
	})

	It("UT-ACM-2593-EXACT-001 [BR-SCOPE-001]: finds a canonical opaque ID without querying ACM by that hash", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request struct {
				Variables struct {
					Input []struct {
						Filters []struct {
							Property string   `json:"property"`
							Values   []string `json:"values"`
						} `json:"filters"`
					} `json:"input"`
				} `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Variables.Input) != 1 {
				http.Error(w, "invalid Search request", http.StatusBadRequest)
				return
			}
			isNamespace, managed := false, false
			for _, filter := range request.Variables.Input[0].Filters {
				if filter.Property == "resource_id" {
					_, _ = io.WriteString(w, emptySearchResponse)
					return
				}
				if filter.Property == "kind" && len(filter.Values) == 1 && filter.Values[0] == "Namespace" {
					isNamespace = true
				}
				if filter.Property == "label" {
					managed = true
				}
			}
			if isNamespace {
				if managed {
					_, _ = io.WriteString(w, emptySearchResponse)
				} else {
					_, _ = io.WriteString(w, `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","apiversion":"v1","kind":"Namespace","name":"payments","label":"kubernaut.ai/managed=false"}]}]}}`)
				}
				return
			}
			_, _ = io.WriteString(w, `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","apiversion":"v1","kind":"ConfigMap","namespace":"payments","name":"checkout","label":"kubernaut.ai/managed=true"}]}]}}`)
		}))
		DeferCleanup(server.Close)

		id := inventory.CanonicalResourceID(scope.ResourceIdentity{ClusterID: "prod-east", Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "checkout"})
		resource, err := acm.NewClient(server.URL).Get(context.Background(), "prod-east", id)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.ID).To(Equal(id))
	})

	It("UT-ACM-2593-EXACT-002 [BR-SCOPE-001]: returns not found for an absent ID in a fully visible empty fixture", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := io.WriteString(w, emptySearchResponse)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)

		id := inventory.CanonicalResourceID(scope.ResourceIdentity{ClusterID: "prod-east", Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "absent"})
		resource, err := acm.NewClient(server.URL).Get(context.Background(), "prod-east", id)
		Expect(err).To(MatchError(inventory.ErrNotFound))
		Expect(resource).To(Equal(inventory.Resource{}))
	})

	It("UT-ACM-2584-LIVE-008 [BR-SECURITY-016]: rejects contradictory Search counts instead of reporting empty", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := io.WriteString(w, `{"data":{"searchResult":[{"count":0,"items":[{"cluster":"prod-east","apigroup":"apps","apiversion":"v1","kind":"Deployment","name":"checkout","label.kubernaut.ai/managed":"true"}]}]}}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		page, err := acm.NewClient(server.URL).List(context.Background(), inventory.Filters{ClusterID: "prod-east"})
		Expect(err).To(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
	})
	It("maps allowlisted Search items to the FMC-compatible page model", func() {
		var requestBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			if respondNamespaceFixture(w, body) {
				return
			}
			requestBody = body
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","apigroup":"apps","apiversion":"v1","kind":"Deployment","namespace":"payments","name":"checkout","label.kubernaut.ai/managed":"true","secret":"must-not-be-projected"}]}]}}`)
		}))
		DeferCleanup(server.Close)

		client := acm.NewClient(server.URL)
		page, err := client.ListManagedResources(context.Background(), inventory.Filters{
			ClusterID: "prod-east",
			Limit:     10,
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].ClusterID).To(Equal("prod-east"))
		Expect(page.Items[0].Group).To(Equal("apps"))
		Expect(page.Items[0].Scope.Included).To(BeTrue())
		Expect(page.Items[0].Scope.ResourceLabel).To(Equal(inventory.LabelTrue))
		serialized, err := json.Marshal(page.Items[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(string(serialized)).NotTo(ContainSubstring("must-not-be-projected"))

		Expect(string(requestBody)).To(ContainSubstring("items"))
		Expect(string(requestBody)).To(ContainSubstring("prod-east"))
	})

	It("returns a state-only unavailable error for GraphQL/backend failure", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"errors":[{"message":"unauthorized"}]}`)
		}))
		DeferCleanup(server.Close)

		client := acm.NewClient(server.URL)
		page, err := client.ListManagedResources(context.Background(), inventory.Filters{ClusterID: "prod-east"})

		Expect(err).To(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(strings.ToLower(err.Error())).NotTo(ContainSubstring("secret"))
	})

	It("rejects an item attributed to a different cluster", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"other-east","apigroup":"apps","apiversion":"v1","kind":"Deployment","namespace":"payments","name":"checkout","label.kubernaut.ai/managed":"true"}]}]}}`)
		}))
		DeferCleanup(server.Close)

		client := acm.NewClient(server.URL)
		page, err := client.ListManagedResources(context.Background(), inventory.Filters{ClusterID: "prod-east"})

		Expect(err).To(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
	})

	It("BR-SECURITY-016: rejects a mismatched cluster on exact reads", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := io.WriteString(w, `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"other-east","resource_id":"opaque-id","apigroup":"apps","apiversion":"v1","kind":"Deployment","namespace":"payments","name":"checkout","label.kubernaut.ai/managed":"true"}]}]}}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		resource, err := acm.NewClient(server.URL).Get(context.Background(), "prod-east", "opaque-id")
		Expect(err).To(MatchError(ContainSubstring("cluster does not match")))
		Expect(resource.ID).To(BeEmpty())
	})

})

func respondNamespaceFixture(w http.ResponseWriter, body []byte) bool {
	if !strings.Contains(string(body), `"property":"kind","values":["Namespace"]`) {
		return false
	}
	response := emptySearchResponse
	if strings.Contains(string(body), `"property":"name","values":["payments"]`) {
		response = `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","apiversion":"v1","kind":"Namespace","name":"payments"}]}]}}`
	}
	if _, err := io.WriteString(w, response); err != nil {
		GinkgoWriter.Printf("write namespace fixture: %v\n", err)
	}
	return true
}
