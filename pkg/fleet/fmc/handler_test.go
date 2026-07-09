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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// mockScopeChecker implements scope.ScopeChecker for testing.
type mockScopeChecker struct {
	managed map[string]bool
	err     error
}

var _ scope.ScopeChecker = (*mockScopeChecker)(nil)

func (m *mockScopeChecker) IsManagedResource(_ context.Context, r scope.ResourceIdentity) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	key := fmt.Sprintf("%s/%s/%s/%s/%s/%s", r.ClusterID, r.Group, r.Version, r.Kind, r.Namespace, r.Name)
	return m.managed[key], nil
}

// mockClusterRegistry implements registry.ClusterRegistry for testing.
type mockClusterRegistry struct {
	clusters []registry.ClusterInfo
}

var _ registry.ClusterRegistry = (*mockClusterRegistry)(nil)

func (m *mockClusterRegistry) List() []registry.ClusterInfo { return m.clusters }
func (m *mockClusterRegistry) Get(id string) (registry.ClusterInfo, bool) {
	for _, c := range m.clusters {
		if c.ID == id {
			return c, true
		}
	}
	return registry.ClusterInfo{}, false
}
func (m *mockClusterRegistry) WatchClusters() <-chan registry.ClusterEvent { return nil }
func (m *mockClusterRegistry) Ready() bool                                 { return true }
func (m *mockClusterRegistry) Start(_ context.Context) error               { return nil }
func (m *mockClusterRegistry) Stop()                                       {}

// mockPinger implements fmc.Pinger for testing readiness checks.
type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(_ context.Context) error { return m.err }

func doGet(mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func doPost(mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func decodeScopeCheck(w *httptest.ResponseRecorder) fmc.ScopeCheckResponse {
	var resp fmc.ScopeCheckResponse
	ExpectWithOffset(1, json.NewDecoder(w.Body).Decode(&resp)).To(Succeed())
	return resp
}

var _ = Describe("FMC HTTP Handler (BR-INTEGRATION-065, ADR-068)", func() {
	var (
		mux     *http.ServeMux
		checker *mockScopeChecker
		reg     *mockClusterRegistry
	)

	BeforeEach(func() {
		checker = &mockScopeChecker{managed: make(map[string]bool)}
		reg = &mockClusterRegistry{
			clusters: []registry.ClusterInfo{
				{ID: "prod-east"},
			},
		}
		handler := fmc.NewHandler(checker, reg, logr.Discard())
		mux = http.NewServeMux()
		handler.RegisterRoutes(mux)
	})

	Describe("GET /api/v1/scope/check", func() {
		It("UT-FMC-API-001 [AC-4]: scope check returns managed=true for in-scope resource", func() {
			checker.managed["prod-east/apps/v1/Deployment/default/nginx"] = true

			w := doGet(mux, "/api/v1/scope/check?cluster=prod-east&group=apps&version=v1&kind=Deployment&namespace=default&name=nginx")

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(decodeScopeCheck(w).Managed).To(BeTrue())
		})

		It("UT-FMC-API-002 [AC-4, SC-7]: scope check returns managed=false for out-of-scope resource", func() {
			w := doGet(mux, "/api/v1/scope/check?cluster=prod-east&group=apps&version=v1&kind=Deployment&namespace=default&name=missing")

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(decodeScopeCheck(w).Managed).To(BeFalse())
		})

		DescribeTable("UT-FMC-API-003..005 [SI-10]: rejects incomplete scope queries with missing required params",
			func(path string) {
				w := doGet(mux, path)
				Expect(w.Code).To(Equal(http.StatusBadRequest))
			},
			Entry("missing cluster", "/api/v1/scope/check?kind=Deployment&name=nginx"),
			Entry("missing kind", "/api/v1/scope/check?cluster=prod-east&name=nginx"),
			Entry("missing name", "/api/v1/scope/check?cluster=prod-east&kind=Deployment"),
		)

		It("UT-FMC-API-006 [SI-10]: rejects non-GET method on scope check endpoint", func() {
			w := doPost(mux, "/api/v1/scope/check?cluster=prod-east&kind=Deployment&name=nginx")
			Expect(w.Code).To(Equal(http.StatusMethodNotAllowed))
		})

		It("UT-FMC-API-007 [SC-7]: cache error falls back to managed=false — boundary conservative under failure", func() {
			checker.err = fmt.Errorf("valkey: connection refused")

			w := doGet(mux, "/api/v1/scope/check?cluster=prod-east&group=apps&version=v1&kind=Deployment&namespace=default&name=nginx")

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(decodeScopeCheck(w).Managed).To(BeFalse(),
				"checker errors must fall back to unmanaged (fail-safe)")
		})

		It("UT-FMC-API-008 [AC-4]: core group resources (empty group) are queryable", func() {
			checker.managed["prod-east//v1/Pod/default/nginx"] = true

			w := doGet(mux, "/api/v1/scope/check?cluster=prod-east&version=v1&kind=Pod&namespace=default&name=nginx")

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(decodeScopeCheck(w).Managed).To(BeTrue(),
				"core group resources (empty group) must be queryable")
		})

		It("UT-FMC-API-015 [SI-10]: excessively long cluster param does not panic or produce incorrect result", func() {
			longCluster := strings.Repeat("a", 10000)
			w := doGet(mux, "/api/v1/scope/check?cluster="+longCluster+"&kind=Deployment&name=nginx")

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(decodeScopeCheck(w).Managed).To(BeFalse(),
				"oversized input must not panic and must return unmanaged")
		})

		It("UT-FMC-API-016 [SI-10]: special characters in params handled safely", func() {
			w := doGet(mux, "/api/v1/scope/check?cluster=prod-east&kind=Deployment&name=../../etc/passwd&namespace=default&group=apps&version=v1")

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(decodeScopeCheck(w).Managed).To(BeFalse(),
				"path-traversal-like input must not cause errors and must return unmanaged")
		})

		It("UT-FMC-API-020 [SC-7]: returns managed=false for unknown cluster without hitting cache", func() {
			reg.clusters = []registry.ClusterInfo{
				{ID: "prod-east"},
			}
			checker.managed["unknown-cluster/apps/v1/Deployment/default/nginx"] = true

			w := doGet(mux, "/api/v1/scope/check?cluster=unknown-cluster&group=apps&version=v1&kind=Deployment&namespace=default&name=nginx")

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(decodeScopeCheck(w).Managed).To(BeFalse(),
				"unknown cluster must return unmanaged without consulting cache")
		})

		It("UT-FMC-API-021 [AC-4]: returns managed=true for known cluster with cached resource", func() {
			checker.managed["prod-east/apps/v1/Deployment/default/nginx"] = true

			w := doGet(mux, "/api/v1/scope/check?cluster=prod-east&group=apps&version=v1&kind=Deployment&namespace=default&name=nginx")

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(decodeScopeCheck(w).Managed).To(BeTrue(),
				"known cluster with cached resource must return managed=true")
		})

		It("UT-FMC-API-022 [SC-7]: empty registry rejects all clusters", func() {
			reg.clusters = nil
			checker.managed["prod-east/apps/v1/Deployment/default/nginx"] = true

			w := doGet(mux, "/api/v1/scope/check?cluster=prod-east&group=apps&version=v1&kind=Deployment&namespace=default&name=nginx")

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(decodeScopeCheck(w).Managed).To(BeFalse(),
				"with empty registry, all clusters must be rejected")
		})
	})

	Describe("/readyz readiness probe", func() {
		It("UT-FMC-API-014 [SC-7]: returns 200 when ready and Valkey reachable", func() {
			readyzMux := http.NewServeMux()
			readyzMux.HandleFunc("/readyz", fmc.ReadyzHandler(func() bool { return true }, &mockPinger{err: nil}))
			w := httptest.NewRecorder()
			readyzMux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).To(Equal("ok"))
		})

		It("UT-FMC-API-014b [SC-7]: returns 503 when not ready", func() {
			readyzMux := http.NewServeMux()
			readyzMux.HandleFunc("/readyz", fmc.ReadyzHandler(func() bool { return false }, &mockPinger{err: nil}))
			w := httptest.NewRecorder()
			readyzMux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
			Expect(w.Body.String()).To(Equal("not ready"))
		})

		It("UT-FMC-API-014c [SC-7]: returns 503 when Valkey unreachable", func() {
			readyzMux := http.NewServeMux()
			readyzMux.HandleFunc("/readyz", fmc.ReadyzHandler(func() bool { return true }, &mockPinger{err: fmt.Errorf("connection refused")}))
			w := httptest.NewRecorder()
			readyzMux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
			Expect(w.Body.String()).To(Equal("valkey unreachable"))
		})
	})

	Describe("GET /api/v1/clusters", func() {
		It("UT-FMC-API-010 [CM-6]: empty cluster list when none registered — configuration audit reflects actual state", func() {
			reg.clusters = nil

			w := doGet(mux, "/api/v1/clusters")

			Expect(w.Code).To(Equal(http.StatusOK))
			var resp fmc.ClusterListResponse
			Expect(json.NewDecoder(w.Body).Decode(&resp)).To(Succeed())
			Expect(resp.Clusters).To(BeEmpty())
		})

		It("UT-FMC-API-011 [CM-6]: cluster list returns registered clusters — federated configuration is auditable", func() {
			reg.clusters = []registry.ClusterInfo{
				{ID: "prod-east"},
				{ID: "prod-west"},
			}

			w := doGet(mux, "/api/v1/clusters")

			Expect(w.Code).To(Equal(http.StatusOK))
			var resp fmc.ClusterListResponse
			Expect(json.NewDecoder(w.Body).Decode(&resp)).To(Succeed())
			Expect(resp.Clusters).To(HaveLen(2))
			Expect(resp.Clusters[0].ID).To(Equal("prod-east"))
			Expect(resp.Clusters[1].ID).To(Equal("prod-west"))
		})

		It("UT-FMC-API-012 [SI-10]: rejects non-GET method on cluster listing endpoint", func() {
			w := doPost(mux, "/api/v1/clusters")
			Expect(w.Code).To(Equal(http.StatusMethodNotAllowed))
		})

		// Issue #1651: ClusterInfoResponse.Name was removed — non-unique,
		// unsafe for disambiguation. ID-only.
		It("UT-FMC-1651-001: Name field has been removed from ClusterInfoResponse", func() {
			_, found := reflect.TypeOf(fmc.ClusterInfoResponse{}).FieldByName("Name")
			Expect(found).To(BeFalse(), "ClusterInfoResponse.Name must not exist (issue #1651: non-unique, unsafe for disambiguation)")
		})

		It("UT-FMC-API-013 [SI-10]: response Content-Type is application/json", func() {
			w := doGet(mux, "/api/v1/clusters")
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
		})
	})
})
