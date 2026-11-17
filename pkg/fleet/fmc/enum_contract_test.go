package fmc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Enum wire contract [BR-SCOPE-001, BR-INTEGRATION-065]", func() {
	DescribeTable("S13/S16 preserve validated domain outcomes with exact status",
		func(state scope.ScopeState, source scope.ScopeSource, status, attempts int) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"state":"` + string(state) + `","provenance":"` + string(source) + `"}`))
			}))
			defer server.Close()
			result, err := fmc.NewHTTPClient(server.URL).Check(context.Background(), scope.ResourceIdentity{ClusterID: "hub", Version: "v1", Kind: "Pod", Namespace: "payments", Name: "api"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.State).To(Equal(state))
			Expect(result.Provenance).To(Equal(source))
			Expect(calls.Load()).To(Equal(int32(attempts)))
		},
		Entry("resource managed", scope.ScopeVerifiedManaged, scope.ScopeSourceResourceLabel, 200, 1),
		Entry("namespace managed", scope.ScopeVerifiedManaged, scope.ScopeSourceNamespaceLabel, 200, 1),
		Entry("resource unmanaged", scope.ScopeVerifiedUnmanaged, scope.ScopeSourceResourceLabel, 200, 1),
		Entry("namespace unmanaged", scope.ScopeVerifiedUnmanaged, scope.ScopeSourceNamespaceLabel, 200, 1),
		Entry("absent observed opt-in", scope.ScopeVerifiedUnmanaged, scope.ScopeSourceNone, 200, 1),
		Entry("invalid request", scope.ScopeInvalid, scope.ScopeSourceUnavailable, 400, 1),
		Entry("invalid observation", scope.ScopeInvalid, scope.ScopeSourceUnavailable, 422, 1),
		Entry("unsupported", scope.ScopeUnsupported, scope.ScopeSourceUnavailable, 422, 1),
		Entry("typed denial", scope.ScopeUnauthorized, scope.ScopeSourceUnavailable, 403, 1),
		Entry("unavailable retries only three times", scope.ScopeUnavailable, scope.ScopeSourceUnavailable, 503, 3),
	)

	DescribeTable("S14-S16 reject nonauthoritative wire without retry",
		func(body, media string, status int) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", media)
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			_, err := fmc.NewHTTPClient(server.URL).Check(context.Background(), scope.ResourceIdentity{ClusterID: "hub", Version: "v1", Kind: "Pod", Namespace: "payments", Name: "api"})
			Expect(err).To(HaveOccurred())
			Expect(calls.Load()).To(Equal(int32(1)))
		},
		Entry("missing members", `{}`, "application/json", 200),
		Entry("legacy bool", `{"managed":true}`, "application/json", 200),
		Entry("unknown state", `{"state":"new","provenance":"resource-label"}`, "application/json", 200),
		Entry("duplicate", `{"state":"verified_managed","state":"verified_unmanaged","provenance":"resource-label"}`, "application/json", 200),
		Entry("unknown field", `{"state":"verified_managed","provenance":"resource-label","reason":"private"}`, "application/json", 200),
		Entry("null", `{"state":null,"provenance":"resource-label"}`, "application/json", 200),
		Entry("wrong type", `{"state":true,"provenance":"resource-label"}`, "application/json", 200),
		Entry("trailing object", `{"state":"verified_managed","provenance":"resource-label"}{}`, "application/json", 200),
		Entry("absent opt-in cannot authorize", `{"state":"verified_managed","provenance":"none"}`, "application/json", 200),
		Entry("wrong media", `{"state":"verified_managed","provenance":"resource-label"}`, "text/plain", 200),
		Entry("unavailable in success", `{"state":"unavailable","provenance":"unavailable"}`, "application/json", 200),
		Entry("authorization proxy body", `{"state":"verified_managed","provenance":"resource-label"}`, "application/json", 403),
	)
})
