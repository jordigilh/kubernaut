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

package datastorage_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

// UT-DS-2658-001 / UT-DS-2658-002 / IT-DS-2658-301 / IT-DS-2658-302: the
// remediation-history endpoint requires an explicit apiVersion (#2658) and
// composes exactly the canonical identity RO writes:
// namespace/group/version/Kind/name.
var _ = Describe("Remediation history target identity (#2658) [BR-ORCH-042, SI-10, CC7.2]", func() {
	var (
		handler *server.Handler
		rec     *httptest.ResponseRecorder
		mock    *mockRemediationHistoryQuerier
	)

	BeforeEach(func() {
		mock = &mockRemediationHistoryQuerier{}
		handler = server.NewHandler(server.WithRemediationHistoryQuerier(mock))
		rec = httptest.NewRecorder()
	})

	// historyURL always carries the mandatory fields; extra holds apiVersion.
	historyURL := func(apiVersionParam string) string {
		query := url.Values{}
		query.Set("targetKind", "Deployment")
		query.Set("targetName", "nginx")
		query.Set("targetNamespace", "prod")
		query.Set("currentSpecHash", "sha256:abc123")
		if apiVersionParam != "" {
			query.Set("targetApiVersion", apiVersionParam)
		}
		return "/api/v1/remediation-history/context?" + query.Encode()
	}

	captureQuery := func() *repository.RemediationHistoryQuery {
		var captured repository.RemediationHistoryQuery
		mock.queryROEventsBySpecHashFn = func(_ context.Context, query repository.RemediationHistoryQuery) ([]repository.RawAuditRow, error) {
			captured = query
			return nil, nil
		}
		return &captured
	}

	Describe("UT-DS-2658-001: targetApiVersion is required and strictly validated", func() {
		It("rejects an absent targetApiVersion with 400", func() {
			handler.HandleGetRemediationHistoryContext(rec, httptest.NewRequest("GET", historyURL(""), nil))
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(rec.Body.String()).To(ContainSubstring("targetApiVersion"))
		})

		It("rejects an empty targetApiVersion with 400 (empty is not core)", func() {
			handler.HandleGetRemediationHistoryContext(rec, httptest.NewRequest("GET", historyURL(" "), nil))
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})

		DescribeTable("composes the canonical identity from the wire spellings",
			func(apiVersion, wantIdentity string) {
				captured := captureQuery()
				handler.HandleGetRemediationHistoryContext(rec, httptest.NewRequest("GET", historyURL(apiVersion), nil))
				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(captured.TargetResource).To(Equal(wantIdentity))
			},
			Entry("grouped", "apps/v1", "prod/apps/v1/Deployment/nginx"),
			Entry("core", "v1", "prod/core/v1/Deployment/nginx"),
			Entry("core/v1 alias", "core/v1", "prod/core/v1/Deployment/nginx"),
			Entry("unresolved sentinel", "unresolved", "prod/unresolved/unresolved/Deployment/nginx"),
			// CRD versions are DNS1035 labels, not necessarily v<N> (#2658
			// review): scope.ParseAPIVersion accepts them, so the wire contract
			// and the identity must too.
			Entry("CRD version that is not v<N>", "example.com/stable", "prod/example.com/stable/Deployment/nginx"),
		)

		It("rejects a malformed apiVersion with 400", func() {
			handler.HandleGetRemediationHistoryContext(rec, httptest.NewRequest("GET", historyURL("Not A Version"), nil))
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})

		DescribeTable("rejects wildcard characters in the identity parameters (LIKE-injection guard)",
			func(kind, name, namespace string) {
				query := url.Values{}
				query.Set("targetKind", kind)
				query.Set("targetName", name)
				query.Set("targetNamespace", namespace)
				query.Set("targetApiVersion", "apps/v1")
				query.Set("currentSpecHash", "sha256:abc123")
				handler.HandleGetRemediationHistoryContext(rec, httptest.NewRequest("GET", "/api/v1/remediation-history/context?"+query.Encode(), nil))
				Expect(rec.Code).To(Equal(http.StatusBadRequest))
			},
			Entry("percent in name", "Deployment", "ng%inx", "prod"),
			Entry("underscore in name", "Deployment", "ng_inx", "prod"),
			Entry("invalid kind", "de/ployment", "nginx", "prod"),
			Entry("invalid namespace", "Deployment", "nginx", "PR OD"),
		)

		It("accepts an empty targetNamespace for a cluster-scoped target", func() {
			captured := captureQuery()
			query := url.Values{}
			query.Set("targetKind", "Node")
			query.Set("targetName", "worker-1")
			query.Set("targetNamespace", "")
			query.Set("targetApiVersion", "v1")
			query.Set("currentSpecHash", "sha256:abc123")
			handler.HandleGetRemediationHistoryContext(rec, httptest.NewRequest("GET", "/api/v1/remediation-history/context?"+query.Encode(), nil))
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(captured.TargetResource).To(Equal("core/v1/Node/worker-1"),
				"a cluster-scoped identity omits the namespace part entirely")
		})
	})

	Describe("UT-DS-2658-002 / IT-DS-2658-302: the DS join key equals the RO emission key", func() {
		DescribeTable("composes the same identity creator.TargetIdentity.AuditKey writes",
			func(targetKind, targetName, targetNamespace, apiVersion string, identity creator.TargetIdentity) {
				captured := captureQuery()
				query := url.Values{}
				query.Set("targetKind", targetKind)
				query.Set("targetName", targetName)
				query.Set("targetNamespace", targetNamespace)
				query.Set("targetApiVersion", apiVersion)
				query.Set("currentSpecHash", "sha256:abc123")
				handler.HandleGetRemediationHistoryContext(rec, httptest.NewRequest("GET", "/api/v1/remediation-history/context?"+query.Encode(), nil))
				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(captured.TargetResource).To(Equal(identity.AuditKey()))
			},
			Entry("namespaced grouped", "Deployment", "web", "prod", "apps/v1",
				creator.TargetIdentity{Namespace: "prod", Kind: "Deployment", Name: "web", APIVersion: "apps/v1"}),
			Entry("namespaced core", "Pod", "api", "prod", "v1",
				creator.TargetIdentity{Namespace: "prod", Kind: "Pod", Name: "api", APIVersion: "v1"}),
			Entry("cluster-scoped grouped", "Node", "worker-1", "", "metrics.k8s.io/v1",
				creator.TargetIdentity{Kind: "Node", Name: "worker-1", APIVersion: "metrics.k8s.io/v1"}),
			Entry("cluster-scoped core", "Node", "worker-1", "", "v1",
				creator.TargetIdentity{Kind: "Node", Name: "worker-1", APIVersion: "v1"}),
			Entry("unresolved", "Deployment", "web", "prod", "unresolved",
				creator.TargetIdentity{Namespace: "prod", Kind: "Deployment", Name: "web"}),
		)
	})
})
