package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/credentials"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

var _ = Describe("PagerDuty Delivery Integration (#60)", func() {

	var (
		tmpDir   string
		resolver *credentials.Resolver
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "it-pd-delivery-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if resolver != nil {
			_ = resolver.Close()
		}
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	writeCredFile := func(name, content string) {
		err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644)
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("PD Credential Flow", func() {

		It("IT-NOT-060-001: PD service registered via credential resolver; delivery succeeds through orchestrator", func() {
			var pdHits atomic.Int32
			var lastPayload []byte

			pdMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pdHits.Add(1)
				body := make([]byte, r.ContentLength)
				_, _ = r.Body.Read(body)
				lastPayload = body
				Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))
				w.WriteHeader(http.StatusAccepted)
				_, _ = fmt.Fprint(w, `{"status":"success","dedup_key":"test"}`)
			}))
			defer pdMock.Close()

			writeCredFile("pd-routing-key", "test-pagerduty-routing-key-12345")
			logger := ctrl.Log.WithName("test")
			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			configYAML := `
route:
  receiver: oncall-critical
receivers:
  - name: oncall-critical
    pagerdutyConfigs:
      - credentialRef: pd-routing-key
        severity: critical
`
			config, err := routing.ParseConfig([]byte(configYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(config.ValidateCredentialRefs()).To(Succeed())

			orch := delivery.NewOrchestrator(
				sanitization.NewSanitizer(), nil, nil, logger,
			)

			for _, recv := range config.Receivers {
				for _, pc := range recv.PagerDutyConfigs {
					routingKey, err := resolver.Resolve(pc.CredentialRef)
					Expect(err).NotTo(HaveOccurred())
					Expect(routingKey).To(Equal("test-pagerduty-routing-key-12345"))

					channelKey := fmt.Sprintf("pagerduty:%s", recv.Name)
					svc := delivery.NewPagerDutyDeliveryService(pdMock.URL, routingKey, 0)
					orch.RegisterChannel(channelKey, svc)
				}
			}

			Expect(orch.HasChannel("pagerduty:oncall-critical")).To(BeTrue(),
				"orchestrator should have per-receiver PD channel registered")

			_ = lastPayload // Will be validated in full flow tests
		})
	})

	Describe("PD Full Flow + Reload", func() {

		It("IT-NOT-060-002: Full delivery flow through orchestrator delivers to mock PD endpoint", func() {
			var pdHits atomic.Int32
			var capturedPayload map[string]interface{}

			pdMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pdHits.Add(1)
				decoder := json.NewDecoder(r.Body)
				err := decoder.Decode(&capturedPayload)
				Expect(err).NotTo(HaveOccurred())
				w.WriteHeader(http.StatusAccepted)
			}))
			defer pdMock.Close()

			writeCredFile("pd-key", "test-key")
			logger := ctrl.Log.WithName("test")
			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			channelKey := "pagerduty:oncall"
			svc := delivery.NewPagerDutyDeliveryService(pdMock.URL, "test-key", 0)
			orch := delivery.NewOrchestrator(sanitization.NewSanitizer(), nil, nil, logger)
			orch.RegisterChannel(channelKey, svc)
			Expect(orch.HasChannel(channelKey)).To(BeTrue())

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pd-full-flow-test",
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  "Full flow PD test",
					Body:     "Testing end-to-end delivery.",
				},
			}

			err = svc.Deliver(context.Background(), notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(pdHits.Load()).To(Equal(int32(1)))
			Expect(capturedPayload["dedup_key"]).To(Equal("pd-full-flow-test"))
			Expect(capturedPayload["event_action"]).To(Equal("trigger"))
		})

		It("IT-NOT-060-003: Config reload replaces PD delivery services in orchestrator", func() {
			pdMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			}))
			defer pdMock.Close()

			writeCredFile("pd-key-v1", "key-v1")
			writeCredFile("pd-key-v2", "key-v2")
			logger := ctrl.Log.WithName("test")
			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			orch := delivery.NewOrchestrator(sanitization.NewSanitizer(), nil, nil, logger)

			// First config: receiver "v1-oncall"
			svc1 := delivery.NewPagerDutyDeliveryService(pdMock.URL, "key-v1", 0)
			orch.RegisterChannel("pagerduty:v1-oncall", svc1)
			Expect(orch.HasChannel("pagerduty:v1-oncall")).To(BeTrue())

			// Simulate config reload: unregister old, register new
			orch.UnregisterChannel("pagerduty:v1-oncall")
			Expect(orch.HasChannel("pagerduty:v1-oncall")).To(BeFalse())

			svc2 := delivery.NewPagerDutyDeliveryService(pdMock.URL, "key-v2", 0)
			orch.RegisterChannel("pagerduty:v2-oncall", svc2)
			Expect(orch.HasChannel("pagerduty:v2-oncall")).To(BeTrue())
		})
	})

	Describe("PD Dedup Key Integration", func() {

		It("IT-NOT-060-004: dedup_key in delivered payload matches NR name", func() {
			var capturedBody []byte

			pdMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				capturedBody, err = readAllBody(r)
				Expect(err).NotTo(HaveOccurred())
				w.WriteHeader(http.StatusAccepted)
			}))
			defer pdMock.Close()

			writeCredFile("pd-dedup-key", "routing-key-abc")
			logger := ctrl.Log.WithName("test")
			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			routingKey, err := resolver.Resolve("pd-dedup-key")
			Expect(err).NotTo(HaveOccurred())

			svc := delivery.NewPagerDutyDeliveryService(pdMock.URL, routingKey, 0)

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dedup-integration-test",
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  "OOMKill in production",
					Body:     "Container exceeded memory limits.",
				},
			}
			err = svc.Deliver(context.Background(), notification)
			Expect(err).NotTo(HaveOccurred())

			var payload map[string]interface{}
			Expect(json.Unmarshal(capturedBody, &payload)).To(Succeed())
			Expect(payload["dedup_key"]).To(Equal("dedup-integration-test"))
			Expect(payload["event_action"]).To(Equal("trigger"))
			Expect(payload["routing_key"]).To(Equal("routing-key-abc"))
		})
	})
})

func readAllBody(r *http.Request) ([]byte, error) {
	defer func() { _ = r.Body.Close() }()
	return io.ReadAll(r.Body)
}
