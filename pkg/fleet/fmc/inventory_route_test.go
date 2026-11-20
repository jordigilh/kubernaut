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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

// Test Plan wiring row 1 (DD-FLEET-017 §8): the inventory route is registered
// by the production FMC handler and served through a real mux and JSON
// round-trip. The production *assembly* (cmd/fleetmetadatacache) is proven in
// IT-FMC-INV-2661-001W; this spec proves the route surface and the shared
// sanitized schema.
var _ = Describe("IT-FMC-INV-2661-001 [BR-API-001, BR-SECURITY-017]: FMC inventory route", func() {
	var (
		server *httptest.Server
		store  *fakeInventoryStore
		reader *fmc.InventoryReader
		mux    *http.ServeMux
	)

	BeforeEach(func() {
		store = newFakeInventoryStore()
		store.setNamespace("payments", "true")
		store.put("payments", "Deployment", "apps", "v1", "api", "true")
		store.put("payments", "Deployment", "apps", "v1", "web", "absent")
		reader, _ = newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})

		mux = http.NewServeMux()
		fmc.NewHandlerWithInventory(&mockScopeChecker{}, &mockClusterRegistry{}, reader, logr.Discard()).RegisterRoutes(mux)
		server = httptest.NewServer(mux)
	})

	AfterEach(func() {
		server.Close()
	})

	get := func(path string) (*http.Response, []byte) {
		response, err := http.Get(server.URL + path) //nolint:noctx // test client, no cancellation needed
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		defer func() { _ = response.Body.Close() }()
		body := make([]byte, 0)
		buffer := make([]byte, 4096)
		for {
			read, readErr := response.Body.Read(buffer)
			body = append(body, buffer[:read]...)
			if readErr != nil {
				break
			}
		}
		return response, body
	}

	It("serves the list page through the production mux", func() {
		response, body := get(fmc.InventoryPath + "?cluster_id=" + inventoryClusterID)
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(response.Header.Get("Content-Type")).To(Equal("application/json"))

		var page inventory.Page
		Expect(json.Unmarshal(body, &page)).To(Succeed())
		Expect(page.Validate()).To(Succeed(), string(body))
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh))
		Expect(page.Items).To(HaveLen(2))

		payload := string(body)
		for _, forbidden := range []string{"kubernaut.ai", "annotations", "_uid", "secret", "token"} {
			Expect(payload).ToNot(ContainSubstring(forbidden), forbidden)
		}
	})

	It("serves an exact read and reports an absent identity as 404", func() {
		pageResponse, body := get(fmc.InventoryPath + "?cluster_id=" + inventoryClusterID)
		Expect(pageResponse.StatusCode).To(Equal(http.StatusOK))
		var page inventory.Page
		Expect(json.Unmarshal(body, &page)).To(Succeed())
		resourceID := page.Items[0].ID

		response, exactBody := get(fmc.InventoryPath + "/" + resourceID + "?cluster_id=" + inventoryClusterID)
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		var resource inventory.Resource
		Expect(json.Unmarshal(exactBody, &resource)).To(Succeed())
		Expect(resource.Validate()).To(Succeed())
		Expect(resource.ID).To(Equal(resourceID))

		missing, _ := get(fmc.InventoryPath + "/r-" + strings.Repeat("f", 64) + "?cluster_id=" + inventoryClusterID)
		Expect(missing.StatusCode).To(Equal(http.StatusNotFound))

		crossCluster, _ := get(fmc.InventoryPath + "/" + resourceID + "?cluster_id=other-east")
		Expect(crossCluster.StatusCode).To(Equal(http.StatusServiceUnavailable),
			"an unregistered cluster is unavailable, not an assertion that the resource does not exist")
	})

	It("reports a malformed cursor or ID as 400 and keeps the method contract", func() {
		invalidCursor, _ := get(fmc.InventoryPath + "?cluster_id=" + inventoryClusterID + "&cursor=../page")
		Expect(invalidCursor.StatusCode).To(Equal(http.StatusBadRequest),
			"a malformed cursor is the only caller error")

		invalidID, _ := get(fmc.InventoryPath + "/opaque-id?cluster_id=" + inventoryClusterID)
		Expect(invalidID.StatusCode).To(Equal(http.StatusBadRequest))

		missingCluster, _ := get(fmc.InventoryPath)
		Expect(missingCluster.StatusCode).To(Equal(http.StatusBadRequest))

		response, err := http.Post(server.URL+fmc.InventoryPath+"?cluster_id="+inventoryClusterID, "application/json", nil) //nolint:noctx // test client
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusMethodNotAllowed))
	})

	It("serves a resource_id filter as a single-item page", func() {
		pageResponse, body := get(fmc.InventoryPath + "?cluster_id=" + inventoryClusterID)
		Expect(pageResponse.StatusCode).To(Equal(http.StatusOK))
		var page inventory.Page
		Expect(json.Unmarshal(body, &page)).To(Succeed())

		response, filtered := get(fmc.InventoryPath + "?cluster_id=" + inventoryClusterID + "&resource_id=" + page.Items[0].ID)
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		var filteredPage inventory.Page
		Expect(json.Unmarshal(filtered, &filteredPage)).To(Succeed())
		Expect(filteredPage.Validate()).To(Succeed())
		Expect(filteredPage.Items).To(HaveLen(1))
		Expect(filteredPage.Items[0].ID).To(Equal(page.Items[0].ID))
	})

	It("keeps unproven and unknown routes fail-closed and unchanged", func() {
		store.state = "missing"
		store.reason = "no_lex_index"
		unavailable, body := get(fmc.InventoryPath + "?cluster_id=" + inventoryClusterID)
		Expect(unavailable.StatusCode).To(Equal(http.StatusServiceUnavailable))
		var page inventory.Page
		Expect(json.Unmarshal(body, &page)).To(Succeed())
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())

		unknown, _ := get("/api/v1/inventory/unknown")
		Expect(unknown.StatusCode).To(Equal(http.StatusNotFound))
	})

	It("does not register the inventory routes without a reader", func() {
		bare := http.NewServeMux()
		fmc.NewHandler(&mockScopeChecker{}, &mockClusterRegistry{}, logr.Discard()).RegisterRoutes(bare)
		server := httptest.NewServer(bare)
		defer server.Close()
		response, err := http.Get(server.URL + fmc.InventoryPath + "?cluster_id=" + inventoryClusterID) //nolint:noctx // test client
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusNotFound),
			"a deployment that serves no projection keeps its previous route surface")
	})
})
