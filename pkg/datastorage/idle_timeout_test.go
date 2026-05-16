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
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/health"
)

// ========================================
// PHASE 6: HTTP SERVER IDLE TIMEOUT TESTS (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 6 (Performance)
// TDD Phase: BEHAVIORAL — exercises the health.NewHealthServer factory
// and asserts the returned *http.Server has IdleTimeout configured.
//
// Without IdleTimeout, idle keep-alive connections are never reaped,
// which can exhaust file descriptors under sustained probe traffic.
//
// ========================================

var _ = Describe("Phase 6: HTTP Server IdleTimeout (TP-1088-P1)", func() {

	Describe("Health Server (pkg/shared/health)", func() {
		It("UT-DS-1088-P6-030: health server must have IdleTimeout configured", func() {
			noopHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			srv := health.NewHealthServer(":0", noopHandler, noopHandler, false)

			Expect(srv.IdleTimeout).To(Equal(120*time.Second),
				"Health server must set IdleTimeout to prevent file descriptor exhaustion from idle keep-alive connections")
		})
	})
})
