package notification

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jordigilh/kubernaut/pkg/notification/credentials"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

var _ = Describe("Credential Resolver Integration (BR-NOT-104)", func() {

	var (
		tmpDir   string
		resolver *credentials.Resolver
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "it-cred-resolver-*")
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

	Describe("Credential Hot-Reload via FileWatcher", func() {

		It("IT-NOT-104-001: fsnotify detects credential file change and updates cache automatically", func() {
			writeCredFile("slack-cred", "https://hooks.slack.com/original")
			logger := ctrl.Log.WithName("test")

			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			watchCtx, watchCancel := context.WithCancel(context.Background())
			defer watchCancel()

			err = resolver.StartWatching(watchCtx)
			Expect(err).NotTo(HaveOccurred())

			val, err := resolver.Resolve("slack-cred")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("https://hooks.slack.com/original"))

			writeCredFile("slack-cred", "https://hooks.slack.com/updated")

			Eventually(func() string {
				v, _ := resolver.Resolve("slack-cred")
				return v
			}, 5*time.Second, 200*time.Millisecond).Should(Equal("https://hooks.slack.com/updated"))
		})
	})

	Describe("Per-Receiver Slack Service Creation", func() {

		It("IT-NOT-104-002: Routing config with credential_ref creates per-receiver Slack service registered in orchestrator", func() {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer mockServer.Close()

			writeCredFile("slack-sre-cred", mockServer.URL)
			logger := ctrl.Log.WithName("test")
			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			configYAML := `
route:
  receiver: sre-critical
receivers:
  - name: sre-critical
    slackConfigs:
      - channel: '#sre-critical'
        credentialRef: slack-sre-cred
`
			config, err := routing.ParseConfig([]byte(configYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(config.ValidateCredentialRefs()).To(Succeed())

			orch := delivery.NewOrchestrator(
				sanitization.NewSanitizer(), nil, nil, logger,
			)

			for _, recv := range config.Receivers {
				for _, sc := range recv.SlackConfigs {
					webhookURL, err := resolver.Resolve(sc.CredentialRef)
					Expect(err).NotTo(HaveOccurred())
					Expect(webhookURL).To(Equal(mockServer.URL))

					svc := delivery.NewSlackDeliveryService(webhookURL)
					Expect(svc).NotTo(BeNil())

					channelKey := fmt.Sprintf("slack:%s", recv.Name)
					orch.RegisterChannel(channelKey, svc)
				}
			}

			Expect(orch.HasChannel("slack:sre-critical")).To(BeTrue(),
				"orchestrator should have per-receiver Slack channel registered")
		})
	})

	Describe("Multi-Receiver Isolation", func() {

		It("IT-NOT-104-003: Multiple receivers deliver to distinct mock HTTP endpoints", func() {
			var sreHits, opsHits atomic.Int32

			sreMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sreHits.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer sreMock.Close()

			opsMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				opsHits.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer opsMock.Close()

			writeCredFile("slack-sre", sreMock.URL)
			writeCredFile("slack-ops", opsMock.URL)
			logger := ctrl.Log.WithName("test")
			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			configYAML := `
route:
  receiver: sre-critical
  routes:
    - match:
        severity: info
      receiver: ops-general
receivers:
  - name: sre-critical
    slackConfigs:
      - channel: '#sre-critical'
        credentialRef: slack-sre
  - name: ops-general
    slackConfigs:
      - channel: '#ops-general'
        credentialRef: slack-ops
`
			config, err := routing.ParseConfig([]byte(configYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(config.ValidateCredentialRefs()).To(Succeed())

			orch := delivery.NewOrchestrator(
				sanitization.NewSanitizer(), nil, nil, logger,
			)

			for _, recv := range config.Receivers {
				for _, sc := range recv.SlackConfigs {
					url, err := resolver.Resolve(sc.CredentialRef)
					Expect(err).NotTo(HaveOccurred())

					channelKey := fmt.Sprintf("slack:%s", recv.Name)
					svc := delivery.NewSlackDeliveryService(url)
					orch.RegisterChannel(channelKey, svc)
				}
			}

			Expect(orch.HasChannel("slack:sre-critical")).To(BeTrue())
			Expect(orch.HasChannel("slack:ops-general")).To(BeTrue())

			Expect(sreHits.Load()).To(Equal(int32(0)), "no hits before delivery")
			Expect(opsHits.Load()).To(Equal(int32(0)), "no hits before delivery")

			sreURL, err := resolver.Resolve("slack-sre")
			Expect(err).NotTo(HaveOccurred())
			Expect(sreURL).To(Equal(sreMock.URL))

			opsURL, err := resolver.Resolve("slack-ops")
			Expect(err).NotTo(HaveOccurred())
			Expect(opsURL).To(Equal(opsMock.URL))
			Expect(sreURL).NotTo(Equal(opsURL), "receivers must have distinct webhook URLs")
		})
	})

	Describe("Unresolvable Credential Ref Preserves Config", func() {

		It("IT-NOT-104-004: Config with unresolvable credential_ref is rejected, previous config preserved", func() {
			writeCredFile("slack-valid", "https://hooks.slack.com/valid")
			logger := ctrl.Log.WithName("test")
			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			router := routing.NewRouter(logger)
			validConfigYAML := `
route:
  receiver: valid-receiver
receivers:
  - name: valid-receiver
    consoleConfigs:
      - enabled: true
`
			err = router.LoadConfig([]byte(validConfigYAML))
			Expect(err).NotTo(HaveOccurred())

			newConfigYAML := `
route:
  receiver: slack-receiver
receivers:
  - name: slack-receiver
    slackConfigs:
      - channel: '#alerts'
        credentialRef: nonexistent-cred
`
			newConfig, err := routing.ParseConfig([]byte(newConfigYAML))
			Expect(err).NotTo(HaveOccurred())

			refs := []string{}
			for _, recv := range newConfig.Receivers {
				for _, sc := range recv.SlackConfigs {
					refs = append(refs, sc.CredentialRef)
				}
			}

			err = resolver.ValidateRefs(refs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent-cred"))

			currentConfig := router.GetConfig()
			Expect(currentConfig.Receivers[0].Name).To(Equal("valid-receiver"))
		})
	})

	Describe("Credential Rotation", func() {

		It("IT-NOT-104-005: Credential rotation updates resolved value via fsnotify", func() {
			writeCredFile("slack-cred", "https://hooks.slack.com/old-url")
			logger := ctrl.Log.WithName("test")
			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			watchCtx, watchCancel := context.WithCancel(context.Background())
			defer watchCancel()

			err = resolver.StartWatching(watchCtx)
			Expect(err).NotTo(HaveOccurred())

			val, err := resolver.Resolve("slack-cred")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("https://hooks.slack.com/old-url"))

			writeCredFile("slack-cred", "https://hooks.slack.com/rotated-url")

			Eventually(func() string {
				v, _ := resolver.Resolve("slack-cred")
				return v
			}, 5*time.Second, 200*time.Millisecond).Should(Equal("https://hooks.slack.com/rotated-url"))
		})
	})

	Describe("Mixed Channel Receiver", func() {

		It("IT-NOT-104-006: Receiver with Slack and Console produces qualified and unqualified channels", func() {
			writeCredFile("slack-mixed", "https://hooks.slack.com/mixed")
			logger := ctrl.Log.WithName("test")
			var err error
			resolver, err = credentials.NewResolver(tmpDir, logger)
			Expect(err).NotTo(HaveOccurred())

			configYAML := `
route:
  receiver: mixed-receiver
receivers:
  - name: mixed-receiver
    slackConfigs:
      - channel: '#alerts'
        credentialRef: slack-mixed
    consoleConfigs:
      - enabled: true
`
			config, err := routing.ParseConfig([]byte(configYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(config.ValidateCredentialRefs()).To(Succeed())

			recv := config.GetReceiver("mixed-receiver")
			Expect(recv).NotTo(BeNil())

			channels := recv.QualifiedChannels()
			Expect(channels).To(ContainElement("slack:mixed-receiver"))
			Expect(channels).To(ContainElement("console"))
			Expect(channels).To(HaveLen(2))
		})
	})

	Describe("Empty Credentials Directory", func() {

		It("IT-NOT-104-007: Empty credentials directory rejects all credential_refs", func() {
			emptyDir, err := os.MkdirTemp("", "it-empty-cred-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(os.RemoveAll(emptyDir)).To(Succeed()) }()

			logger := ctrl.Log.WithName("test")
			emptyResolver, err := credentials.NewResolver(emptyDir, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(emptyResolver.Count()).To(Equal(0))

			err = emptyResolver.ValidateRefs([]string{"any-ref"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("any-ref"))

			err = emptyResolver.Close()
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
