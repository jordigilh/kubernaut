package notification

import (
	"context"
	"encoding/json"
	"fmt"
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

var _ = Describe("Microsoft Teams Delivery Integration (#593)", func() {

	var (
		tmpDir   string
		resolver *credentials.Resolver
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "it-teams-delivery-*")
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

	Describe("Teams Credential Flow", func() {

		It("IT-NOT-593-001: Teams service registered via credential resolver; delivery succeeds", func() {
			var teamsHits atomic.Int32

			teamsMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				teamsHits.Add(1)
				Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))
				w.WriteHeader(http.StatusOK)
			}))
			defer teamsMock.Close()

			writeCredFile("teams-webhook-url", teamsMock.URL)
			logger := ctrl.Log.WithName("test")
			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			configYAML := `
route:
  receiver: sre-alerts
receivers:
  - name: sre-alerts
    teamsConfigs:
      - credentialRef: teams-webhook-url
`
			config, err := routing.ParseConfig([]byte(configYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(config.ValidateCredentialRefs()).To(Succeed())

			orch := delivery.NewOrchestrator(
				sanitization.NewSanitizer(), nil, nil, logger,
			)

			for _, recv := range config.Receivers {
				for _, tc := range recv.TeamsConfigs {
					webhookURL, err := resolver.Resolve(tc.CredentialRef)
					Expect(err).NotTo(HaveOccurred())
					Expect(webhookURL).To(Equal(teamsMock.URL))

					channelKey := fmt.Sprintf("teams:%s", recv.Name)
					svc := delivery.NewTeamsDeliveryService(webhookURL, 0)
					orch.RegisterChannel(channelKey, svc)
				}
			}

			Expect(orch.HasChannel("teams:sre-alerts")).To(BeTrue(),
				"orchestrator should have per-receiver Teams channel registered")
		})
	})

	Describe("Teams Full Flow + Reload", func() {

		It("IT-NOT-593-002: Full delivery flow through orchestrator delivers to mock Teams endpoint", func() {
			var teamsHits atomic.Int32

			teamsMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				teamsHits.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer teamsMock.Close()

			logger := ctrl.Log.WithName("test")

			channelKey := "teams:sre-alerts"
			svc := delivery.NewTeamsDeliveryService(teamsMock.URL, 0)
			orch := delivery.NewOrchestrator(sanitization.NewSanitizer(), nil, nil, logger)
			orch.RegisterChannel(channelKey, svc)
			Expect(orch.HasChannel(channelKey)).To(BeTrue())

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "teams-full-flow-test",
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Full flow Teams test",
					Body:     "Testing end-to-end delivery.",
				},
			}

			err := svc.Deliver(context.Background(), notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(teamsHits.Load()).To(Equal(int32(1)))
		})

		It("IT-NOT-593-003: Config reload replaces Teams delivery services in orchestrator", func() {
			teamsMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer teamsMock.Close()

			writeCredFile("teams-v1", teamsMock.URL)
			writeCredFile("teams-v2", teamsMock.URL+"/v2")
			logger := ctrl.Log.WithName("test")
			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			orch := delivery.NewOrchestrator(sanitization.NewSanitizer(), nil, nil, logger)

			svc1 := delivery.NewTeamsDeliveryService(teamsMock.URL, 0)
			orch.RegisterChannel("teams:v1-alerts", svc1)
			Expect(orch.HasChannel("teams:v1-alerts")).To(BeTrue())

			// Simulate config reload
			orch.UnregisterChannel("teams:v1-alerts")
			Expect(orch.HasChannel("teams:v1-alerts")).To(BeFalse())

			svc2 := delivery.NewTeamsDeliveryService(teamsMock.URL+"/v2", 0)
			orch.RegisterChannel("teams:v2-alerts", svc2)
			Expect(orch.HasChannel("teams:v2-alerts")).To(BeTrue())
		})
	})

	Describe("Teams Workflows Format Integration", func() {

		It("IT-NOT-593-004: Delivered payload uses Workflows format (NOT legacy MessageCard)", func() {
			var capturedBody []byte

			teamsMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				capturedBody, err = readAllBody(r)
				Expect(err).NotTo(HaveOccurred())
				w.WriteHeader(http.StatusOK)
			}))
			defer teamsMock.Close()

			writeCredFile("teams-format-key", teamsMock.URL)
			logger := ctrl.Log.WithName("test")
			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			webhookURL, err := resolver.Resolve("teams-format-key")
			Expect(err).NotTo(HaveOccurred())

			svc := delivery.NewTeamsDeliveryService(webhookURL, 0)

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "teams-format-integration",
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeApproval,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  "Approval required: OOMKill remediation",
					Body:     "AI analysis recommends increasing memory limits.",
					Context: &notificationv1alpha1.NotificationContext{
						Lineage: &notificationv1alpha1.LineageContext{
							RemediationRequest: "oom-recovery-rr",
						},
						Analysis: &notificationv1alpha1.AnalysisContext{
							RootCause: "Container OOMKill due to memory leak",
						},
					},
				},
			}
			err = svc.Deliver(context.Background(), notification)
			Expect(err).NotTo(HaveOccurred())

			var payload map[string]interface{}
			Expect(json.Unmarshal(capturedBody, &payload)).To(Succeed())

			Expect(payload["type"]).To(Equal("message"))
			attachments, ok := payload["attachments"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(attachments).To(HaveLen(1))

			attachment := attachments[0].(map[string]interface{})
			Expect(attachment["contentType"]).To(Equal("application/vnd.microsoft.card.adaptive"))

			content := attachment["content"].(map[string]interface{})
			Expect(content["type"]).To(Equal("AdaptiveCard"))
			Expect(content["version"]).To(Equal("1.0"))

			bodyStr := string(capturedBody)
			Expect(bodyStr).NotTo(ContainSubstring("MessageCard"))
			Expect(bodyStr).NotTo(ContainSubstring("@type"))
			Expect(bodyStr).NotTo(ContainSubstring("@context"))

			Expect(bodyStr).To(ContainSubstring("kubectl kubernaut chat rar/oom-recovery-rr"))
			Expect(bodyStr).To(ContainSubstring("Container OOMKill due to memory leak"))
		})
	})
})
