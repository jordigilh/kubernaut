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

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

// Issue #2694 (BR-SECURITY-016, FedRAMP AU-2): KubernautAgent's
// newAuthMiddleware must route the shared auth middleware's security_event
// records onto the dedicated unsampled, non-level-gated logger derived from
// the host logger.
//
// The host logger is a real zap core at INFO level writing to an in-memory
// buffer. The level matters for discrimination: if the wiring were missing,
// emitSecurityEvent's fallback would write the Info security_event record
// into this host buffer and the spec would fail. With the wiring in place
// the record goes to the dedicated logger instead. The K8s API host is
// never dialed: the request is rejected at the missing Authorization
// header, before any TokenReview/SAR call, so a clientset pointing at an
// unreachable address suffices. The dedicated logger's WARN-gate/sampler
// guarantees are proven in pkg/log (UT-LOG-2694-002/003) and
// pkg/shared/auth (UT-AUTH-2694-003/004).

var _ = Describe("KA auth security_event dedicated logger wiring [Issue #2694]", func() {
	It("IT-KA-2694-001 [BR-SECURITY-016, AU-2]: newAuthMiddleware routes security events off the host logger", func() {
		hostBuf := &syncBufferKA{}
		hostLogger := zapr.NewLogger(zap.New(zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), hostBuf, zapcore.InfoLevel)))

		// clientset construction does not dial; the unauthenticated request
		// below is rejected before any Kubernetes API call.
		clientset, err := kubernetes.NewForConfig(&rest.Config{Host: "http://127.0.0.1:1"})
		Expect(err).NotTo(HaveOccurred())

		mw, cleanup := newAuthMiddleware(&k8sInfra{clientset: clientset}, kaconfig.InteractiveConfig{}, hostLogger)
		Expect(mw).NotTo(BeNil())
		Expect(cleanup).To(BeNil(), "JWT Pattern B is disabled in this configuration")

		recorder := httptest.NewRecorder()
		mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/tools", nil))
		Expect(recorder.Code).To(Equal(http.StatusUnauthorized))

		var securityEvents int
		for _, line := range strings.Split(strings.TrimSpace(hostBuf.String()), "\n") {
			if line == "" {
				continue
			}
			var entry map[string]any
			Expect(json.Unmarshal([]byte(line), &entry)).To(Succeed())
			if entry["msg"] == "security_event" {
				securityEvents++
			}
		}
		Expect(securityEvents).To(Equal(0),
			"the security_event record must leave the host logger's level-gated path: "+
				"newAuthMiddleware wires the dedicated unsampled security-event logger")
	})
})

// syncBufferKA is a concurrency-safe zapcore.WriteSyncer.
type syncBufferKA struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBufferKA) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBufferKA) Sync() error { return nil }

func (b *syncBufferKA) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
