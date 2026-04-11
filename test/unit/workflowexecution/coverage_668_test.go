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

package workflowexecution

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	weclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
	weconfig "github.com/jordigilh/kubernaut/pkg/workflowexecution/config"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/metrics"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
)

// Phase 2 coverage: AWX HTTP client, metrics wiring, phase helpers, config load, DS querier factory (BR-WE-008, BR-WE-015, BR-WE-005).
var _ = Describe("WorkflowExecution coverage 668 (BR-WE-008 BR-WE-015)", func() {

	Describe("AWXHTTPClient (BR-WE-015)", func() {
		It("CancelJob accepts HTTP 202 from AWX cancel endpoint", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.URL.Path).To(Equal("/api/v2/jobs/77/cancel/"))
				w.WriteHeader(http.StatusAccepted)
			}))
			defer srv.Close()

			c := executor.NewAWXHTTPClient(srv.URL, "test-token", false)
			Expect(c.CancelJob(context.Background(), 77)).To(Succeed())
		})

		It("FindJobTemplateByName decodes first template id from list response", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodGet))
				Expect(r.URL.Path).To(Equal("/api/v2/job_templates/"))
				Expect(r.URL.Query().Get("name")).To(Equal("restart-api"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"count":1,"results":[{"id":501}]}`)
			}))
			defer srv.Close()

			c := executor.NewAWXHTTPClient(srv.URL, "tok", false)
			id, err := c.FindJobTemplateByName(context.Background(), "restart-api")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(501))
		})

		It("FindCredentialTypeByKind returns id from credential_types search", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v2/credential_types/"))
				Expect(r.URL.Query().Get("kind")).To(Equal("kubernetes_bearer_token"))
				Expect(r.URL.Query().Get("managed")).To(Equal("true"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"count":1,"results":[{"id":88}]}`)
			}))
			defer srv.Close()

			c := executor.NewAWXHTTPClient(srv.URL, "tok", false)
			id, err := c.FindCredentialTypeByKind(context.Background(), "kubernetes_bearer_token", true)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(88))
		})

		It("GetJobTemplateCredentials collects credential ids from results array", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v2/job_templates/3/credentials/"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"results":[{"id":10},{"id":20}]}`)
			}))
			defer srv.Close()

			c := executor.NewAWXHTTPClient(srv.URL, "tok", false)
			ids, err := c.GetJobTemplateCredentials(context.Background(), 3)
			Expect(err).NotTo(HaveOccurred())
			Expect(ids).To(Equal([]int{10, 20}))
		})
	})

	Describe("metrics.NewMetrics (BR-WE-008)", func() {
		It("registers counters so Completed label can be incremented", func() {
			m := metrics.NewMetrics()
			m.RecordWorkflowCompletion(12.5)

			var metric dto.Metric
			Expect(m.ExecutionTotal.WithLabelValues(metrics.LabelOutcomeCompleted).(prometheus.Metric).Write(&metric)).To(Succeed())
			Expect(metric.GetCounter().GetValue()).To(BeNumerically(">=", 1))
		})
	})

	Describe("phase helpers (BR-WE-003 state machine)", func() {
		It("IsTerminal is true only for Completed and Failed", func() {
			Expect(phase.IsTerminal(phase.Completed)).To(BeTrue())
			Expect(phase.IsTerminal(phase.Failed)).To(BeTrue())
			Expect(phase.IsTerminal(phase.Pending)).To(BeFalse())
			Expect(phase.IsTerminal(phase.Running)).To(BeFalse())
		})

		It("Validate accepts known phases and rejects unknown", func() {
			Expect(phase.Validate(phase.Running)).To(Succeed())
			Expect(phase.Validate(phase.Phase("Unknown"))).To(MatchError(ContainSubstring("invalid phase")))
		})
	})

	Describe("weconfig.LoadFromFile (BR-WE-005)", func() {
		It("returns default config when path is empty", func() {
			cfg, err := weconfig.LoadFromFile("")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Execution.Namespace).To(Equal("kubernaut-workflows"))
			Expect(cfg.Validate()).To(Succeed())
		})

		It("overrides execution namespace when YAML file is present", func() {
			dir, err := os.MkdirTemp("", "we-cfg-668-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(dir) }()

			p := filepath.Join(dir, "config.yaml")
			yaml := `
execution:
  namespace: custom-ns
  cooldownPeriod: 2m
controller:
  metricsAddr: ":9090"
  healthProbeAddr: ":8081"
  leaderElectionId: "workflowexecution.kubernaut.ai"
datastorage:
  url: "http://data-storage:8080"
  timeout: 10s
  buffer:
    bufferSize: 100
    batchSize: 10
    flushInterval: 1s
    maxRetries: 3
`
			Expect(os.WriteFile(p, []byte(yaml), 0o600)).To(Succeed())
			cfg, err := weconfig.LoadFromFile(p)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Execution.Namespace).To(Equal("custom-ns"))
			Expect(cfg.Validate()).To(Succeed())
		})
	})

	Describe("NewOgenWorkflowQuerierFromConfig (BR-WE-005)", func() {
		It("returns error when base URL is empty", func() {
			_, err := weclient.NewOgenWorkflowQuerierFromConfig("", time.Second)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot be empty"))
		})

		It("builds a querier against a reachable base URL", func() {
			srv := httptest.NewServer(http.NotFoundHandler())
			defer srv.Close()

			q, err := weclient.NewOgenWorkflowQuerierFromConfig(srv.URL, 500*time.Millisecond)
			Expect(err).NotTo(HaveOccurred())
			_, _, err = q.GetWorkflowExecutionEngine(context.Background(), "not-a-uuid")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid workflow ID"))
		})
	})
})
