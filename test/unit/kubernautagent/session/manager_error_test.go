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

package session_test

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

var _ = Describe("UT-KA-948: Session manager logs store.Update errors — BR-AUDIT-005", func() {

	Describe("UT-KA-948-001: Manager logs error when session is cancelled before goroutine completes", func() {
		It("should log when Update fails due to terminal state conflict", func() {
			var mu sync.Mutex
			var logLines []string
			logger := funcr.New(func(prefix, args string) {
				mu.Lock()
				defer mu.Unlock()
				logLines = append(logLines, prefix+" "+args)
			}, funcr.Options{Verbosity: 10})

			store := session.NewStore(5 * time.Minute)
			mgr := session.NewManager(store, logger, audit.NopAuditStore{}, metrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			gate := make(chan struct{})
			done := make(chan struct{})
			id, err := mgr.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				<-gate
				close(done)
				return "result", nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(mgr.CancelInvestigation(id)).To(Succeed())

			close(gate)
			Eventually(done, 2*time.Second).Should(BeClosed())

			Eventually(func() string {
				mu.Lock()
				defer mu.Unlock()
				return strings.Join(logLines, "\n")
			}, 2*time.Second, 5*time.Millisecond).Should(
				ContainSubstring("post-investigation status update rejected"),
				"manager must log store.Update errors instead of silently discarding them",
			)
		})
	})
})
