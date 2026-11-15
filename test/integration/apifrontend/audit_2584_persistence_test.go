package apifrontend_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/zapr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("BR-AUDIT-005 / IT-AF-2584-012: Console audit persistence", func() {
	It("writes a typed read event that is retrievable by correlation and request ID without response data", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		transport, err := sharedtls.NewTLSTransport(infrastructure.BootstrapTLSCAFile("https://localhost:18096"))
		Expect(err).NotTo(HaveOccurred())
		auditClient, err := sharedaudit.NewOpenAPIClientAdapterWithTransport(
			"https://localhost:18096", 2*time.Second,
			&bearerTransport{token: serviceAccountToken, base: transport},
		)
		Expect(err).NotTo(HaveOccurred())
		store, err := sharedaudit.NewBufferedStore(auditClient, sharedaudit.Config{
			BufferSize: 100, BatchSize: 10, FlushInterval: 50 * time.Millisecond, MaxRetries: 1,
		}, "apifrontend", zapr.NewLogger(zap.NewNop()))
		Expect(err).NotTo(HaveOccurred())
		emitter := audit.NewStoreAdapter(store, zapr.NewLogger(zap.NewNop()))
		DeferCleanup(func() { Expect(emitter.Close(context.Background())).To(Succeed()) })

		correlationID := "console-2584-" + uuid.NewString()
		requestID := "request-2584-" + uuid.NewString()
		fingerprint := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		emitter.Emit(ctx, &audit.Event{
			Type:          audit.EventConsolePolicyScopeRead,
			CorrelationID: correlationID,
			RequestID:     requestID,
			UserID:        "console-user-2584",
			ClusterID:     "local",
			Detail: map[string]string{
				"result_state":       "success",
				"filter_fingerprint": fingerprint,
				"raw_policy":         "must-not-persist",
				"response_item":      "must-not-persist",
			},
		})

		Expect(store.Flush(ctx)).To(Succeed())

		queryClient := newAuthenticatedDataStorageClient()
		var byCorrelation, byRequest []ogenclient.AuditEvent
		Eventually(func(g Gomega) {
			response, queryErr := queryClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventType:     ogenclient.NewOptString("apifrontend.console.policy_scope_read"),
				Limit:         ogenclient.NewOptInt(100),
			})
			g.Expect(queryErr).NotTo(HaveOccurred())
			byCorrelation = response.Data
			g.Expect(byCorrelation).To(HaveLen(1))
		}, 10*time.Second, 50*time.Millisecond).Should(Succeed())

		Eventually(func(g Gomega) {
			response, queryErr := queryClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				EventType:   ogenclient.NewOptString("apifrontend.console.policy_scope_read"),
				DetailKey:   ogenclient.NewOptString("request_id"),
				DetailValue: ogenclient.NewOptString(requestID),
				Limit:       ogenclient.NewOptInt(100),
			})
			g.Expect(queryErr).NotTo(HaveOccurred())
			byRequest = response.Data
			g.Expect(byRequest).To(HaveLen(1))
		}, 10*time.Second, 50*time.Millisecond).Should(Succeed())

		event := byCorrelation[0]
		Expect(event.CorrelationID).To(Equal(correlationID))
		Expect(event.ActorType.Value).To(Equal("user"))
		Expect(event.ActorID.Value).To(Equal("console-user-2584"))
		Expect(event.EventAction).To(Equal("read"))
		Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeSuccess))
		Expect(event.EventData.Validate()).To(Succeed())
		payload, ok := event.EventData.GetApifrontendConsolePolicyScopeReadPayload()
		Expect(ok).To(BeTrue())
		Expect(payload.ClusterID.Value).To(Equal("local"))
		Expect(string(payload.ResultState)).To(Equal("success"))
		Expect(payload.FilterFingerprint.Value).To(Equal(fingerprint))
		Expect(payload.RequestID.Value).To(Equal(requestID))

		encoded, err := json.Marshal(event.EventData)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring("must-not-persist"))
	})

	It("surfaces a DataStorage persistence failure through Flush and structured logs", func() {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "audit backend unavailable")
		}))
		DeferCleanup(backend.Close)

		core, logs := observer.New(zapcore.DebugLevel)
		logger := zapr.NewLogger(zap.New(core))
		auditClient, err := sharedaudit.NewOpenAPIClientAdapterWithTransport(backend.URL, time.Second, http.DefaultTransport)
		Expect(err).NotTo(HaveOccurred())
		store, err := sharedaudit.NewBufferedStore(auditClient, sharedaudit.Config{
			BufferSize: 10, BatchSize: 1, FlushInterval: time.Hour, MaxRetries: 1,
		}, "apifrontend", logger)
		Expect(err).NotTo(HaveOccurred())
		emitter := audit.NewStoreAdapter(store, logger)

		emitter.Emit(context.Background(), &audit.Event{
			Type:          audit.EventConsoleManagedResourceListRead,
			CorrelationID: "console-2584-failure-" + uuid.NewString(),
			RequestID:     "request-2584-failure",
			UserID:        "console-user-2584",
			ClusterID:     "local",
			Detail:        map[string]string{"result_state": "success"},
		})

		Expect(store.Flush(context.Background())).To(HaveOccurred())
		Expect(logs.FilterLevelExact(zapcore.ErrorLevel).Len()).To(BeNumerically(">", 0))
		Expect(emitter.Close(context.Background())).To(HaveOccurred())
	})
})

func newAuthenticatedDataStorageClient() *ogenclient.Client {
	transport, err := sharedtls.NewTLSTransport(infrastructure.BootstrapTLSCAFile("https://localhost:18096"))
	Expect(err).NotTo(HaveOccurred())
	client, err := ogenclient.NewClient("https://localhost:18096", ogenclient.WithClient(&http.Client{
		Transport: &bearerTransport{token: serviceAccountToken, base: transport},
		Timeout:   2 * time.Second,
	}))
	Expect(err).NotTo(HaveOccurred())
	return client
}
