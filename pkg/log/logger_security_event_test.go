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

package log

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

// Issue #2694 (BR-SECURITY-016, FedRAMP AU-2): the security_event records
// emitted by the shared auth middleware must not be dropped by zap's
// production sampler (100 entries/sec per message+level, then every 100th)
// or suppressed by the host service's configured log level. This suite
// proves NewSecurityEventLogger builds a logger that is neither sampled nor
// level-gated, and passes non-zap hosts (test sinks, controller-runtime
// loggers) through unchanged.

func TestLogSecurityEvent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Log Security Event Suite")
}

// countMessages counts complete JSON lines in path whose "msg" field equals
// msg, so incidental non-JSON output (e.g. zap internal errors) never
// inflates the delivery count.
func countMessages(path, msg string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open log output: %w", err)
	}
	defer func() { _ = f.Close() }()

	want := fmt.Sprintf(`"msg":%q`, msg)
	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), want) {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan log output: %w", err)
	}
	return count, nil
}

// nullSink is a minimal non-zap logr sink (stands in for Ginkgo capture
// sinks and controller-runtime loggers).
type nullSink struct{}

func (nullSink) Init(logr.RuntimeInfo)                    {}
func (nullSink) Enabled(int) bool                         { return false }
func (nullSink) Info(int, string, ...interface{})         {}
func (nullSink) Error(error, string, ...interface{})      {}
func (s nullSink) WithValues(...interface{}) logr.LogSink { return s }
func (s nullSink) WithName(string) logr.LogSink           { return s }

var _ = Describe("NewSecurityEventLogger (Issue #2694, BR-SECURITY-016, FedRAMP AU-2)", func() {
	const burst = 250

	It("UT-LOG-2694-001: returns the host logger unchanged when it is not zap-backed", func() {
		host := logr.New(nullSink{})

		sec := NewSecurityEventLogger(host, Options{ServiceName: "datastorage"})

		Expect(sec.GetSink()).To(BeIdenticalTo(host.GetSink()),
			"non-zap hosts (test sinks, controller-runtime loggers) apply no zap sampling; "+
				"passing them through keeps security-event capture working in wiring tests")
	})

	It("UT-LOG-2694-002: delivers every security_event record under a sampling host configuration", func() {
		// The host is a production logger: zap production config keeps zap's
		// default sampler (100 then every 100th per message+level), which is
		// exactly the configuration that drops bursty grant records today.
		host := NewLoggerWithAtomicLevel(Options{ServiceName: "datastorage"}, zap.NewAtomicLevelAt(zap.InfoLevel))
		sec := NewSecurityEventLogger(host, Options{ServiceName: "datastorage"})
		Expect(GetZapLogger(sec)).NotTo(BeNil(),
			"a zap-backed host must yield a dedicated zap-backed security-event logger")
		// Smoke: the public constructor's own product emits without error
		// (its stderr destination is not assertable portably; the delivery
		// count is proven below through the same builder with an injectable
		// output path).
		sec.Info("security_event", "event_id", "smoke")

		// Delivery proof: build the same logger with the output redirected to
		// a file (the only difference from the public constructor's "stderr")
		// and emit one security_event per authorization decision.
		outFile := filepath.Join(GinkgoT().TempDir(), "security-events.jsonl")
		delivered, err := buildSecurityEventLogger(Options{}, []string{outFile})
		Expect(err).NotTo(HaveOccurred())
		for i := 0; i < burst; i++ {
			delivered.Info("security_event", "event_id", fmt.Sprintf("id-%03d", i))
		}
		zapDelivered := GetZapLogger(delivered)
		Expect(zapDelivered).NotTo(BeNil())
		Expect(zapDelivered.Sync()).To(Succeed())

		count, err := countMessages(outFile, "security_event")
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(burst),
			"AU-2: one security_event record per decision -- sampling must never drop any")

		// Control: the identical burst through a sampled production-style
		// logger is dropped by zap's sampler. The count is asserted as
		// strictly less than the burst rather than an exact number because
		// the sampler's one-second window can reset on a slow runner.
		ctrlFile := filepath.Join(GinkgoT().TempDir(), "control.jsonl")
		ctrlCfg := zap.NewProductionConfig()
		ctrlCfg.OutputPaths = []string{ctrlFile}
		ctrlCfg.ErrorOutputPaths = []string{ctrlFile}
		ctrl, err := ctrlCfg.Build()
		Expect(err).NotTo(HaveOccurred())
		for i := 0; i < burst; i++ {
			ctrl.Info("security_event", zap.String("event_id", fmt.Sprintf("id-%03d", i)))
		}
		Expect(ctrl.Sync()).To(Succeed())
		ctrlCount, err := countMessages(ctrlFile, "security_event")
		Expect(err).NotTo(HaveOccurred())
		Expect(ctrlCount).To(BeNumerically("<", burst),
			"control proves the sampler is real: a sampled logger drops part of the burst")
	})

	It("UT-LOG-2694-003: stays at Info level when the host logger is WARN-gated and on hot-reload to ERROR", func() {
		atomicLevel := zap.NewAtomicLevelAt(zap.WarnLevel)
		host := NewLoggerWithAtomicLevel(Options{ServiceName: "gateway"}, atomicLevel)

		sec := NewSecurityEventLogger(host, Options{ServiceName: "gateway"})
		zapSec := GetZapLogger(sec)
		Expect(zapSec).NotTo(BeNil())
		Expect(zapSec.Level()).To(Equal(zap.InfoLevel),
			"AU-2: a service configured at WARN must still emit Info-level security events")

		// Issue #875 hot-reload: raising the service level must never
		// suppress security events; the dedicated logger is deliberately not
		// tied to the host's AtomicLevel.
		atomicLevel.SetLevel(zap.ErrorLevel)
		Expect(zapSec.Level()).To(Equal(zap.InfoLevel),
			"hot-reloading the host level to ERROR must not gate the security-event logger")
	})
})
