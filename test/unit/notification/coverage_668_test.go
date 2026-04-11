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

package notification

import (
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification"
	notificationcfg "github.com/jordigilh/kubernaut/pkg/notification/config"
	"github.com/jordigilh/kubernaut/pkg/notification/credentials"
)

// Phase 2 coverage (BR-NOT-069, BR-NOT-104, ADR-030): helpers previously uncovered by unit tests.
var _ = Describe("Notification coverage 668 (BR-NOT-069 BR-NOT-104)", func() {

	Describe("SetReady (BR-NOT-069)", func() {
		It("sets Ready=True with reason and message on the NotificationRequest", func() {
			nr := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "n1", Namespace: "ns", Generation: 2},
			}
			notification.SetReady(nr, true, notification.ReasonReady, "routing finished")
			cond := notification.GetCondition(nr, notification.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Type).To(Equal(notification.ConditionReady))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(notification.ReasonReady))
			Expect(cond.Message).To(Equal("routing finished"))
			Expect(cond.ObservedGeneration).To(Equal(int64(2)))
		})

		It("sets Ready=False when ready flag is false", func() {
			nr := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "n2", Namespace: "ns", Generation: 1},
			}
			notification.SetReady(nr, false, notification.ReasonNotReady, "waiting on channel")
			cond := notification.GetCondition(nr, notification.ConditionReady)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(notification.ReasonNotReady))
			Expect(cond.Message).To(Equal("waiting on channel"))
		})
	})

	Describe("notification config LoadFromFile / LoadFromEnv / Validate (BR-NOT-104 ADR-030)", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "notif-cfg-668-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			_ = os.RemoveAll(tmpDir)
		})

		It("LoadFromFile reads YAML and applyDefaults fills controller delivery and datastorage", func() {
			path := filepath.Join(tmpDir, "config.yaml")
			yaml := `
controller:
  metricsAddr: ":9191"
  healthProbeAddr: ":8181"
  leaderElection: true
  leaderElectionId: "notification.test.local"
delivery:
  console:
    enabled: false
datastorage:
  url: "http://ds-test:9090"
  timeout: 3s
  buffer:
    bufferSize: 200
    batchSize: 20
    flushInterval: 2s
    maxRetries: 1
`
			Expect(os.WriteFile(path, []byte(yaml), 0o600)).To(Succeed())

			cfg, err := notificationcfg.LoadFromFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Controller.MetricsAddr).To(Equal(":9191"))
			Expect(cfg.Controller.HealthProbeAddr).To(Equal(":8181"))
			Expect(cfg.Controller.LeaderElection).To(BeTrue())
			Expect(cfg.Controller.LeaderElectionID).To(Equal("notification.test.local"))
			// applyDefaults re-enables console for local-style configs when YAML disables it.
			Expect(cfg.Delivery.Console.Enabled).To(BeTrue())
			Expect(cfg.DataStorage.URL).To(Equal("http://ds-test:9090"))
			Expect(cfg.Validate()).To(Succeed())
		})

		It("LoadFromFile returns wrapped error when path is missing", func() {
			_, err := notificationcfg.LoadFromFile(filepath.Join(tmpDir, "missing.yaml"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read config file"))
		})

		It("LoadFromEnv is a no-op after BR-NOT-104 and leaves credentials dir default", func() {
			cfg := notificationcfg.DefaultConfig()
			cfg.LoadFromEnv()
			Expect(cfg.Delivery.Credentials.Dir).To(Equal("/etc/notification/credentials/"))
		})

		It("Validate returns error when controller.metricsAddr is cleared", func() {
			cfg := notificationcfg.DefaultConfig()
			cfg.Controller.MetricsAddr = ""
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("metrics_addr")))
		})
	})

	Describe("credentials Resolver Count (BR-NOT-104)", func() {
		It("returns the number of non-hidden credential files loaded", func() {
			dir, err := os.MkdirTemp("", "cred-668-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(dir) }()

			Expect(os.WriteFile(filepath.Join(dir, "slack-primary"), []byte("secret-a\n"), 0o600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, "slack-secondary"), []byte("secret-b"), 0o600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0o600)).To(Succeed())

			r, err := credentials.NewResolver(dir, logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			Expect(r.Count()).To(Equal(2))
		})
	})
})
