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

package readiness_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
)

func TestFleetReadiness(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet Readiness Suite")
}

// fakeProber is a test double implementing readiness.Prober whose result is
// controlled by an atomically-swapped error pointer, so tests can flip it
// concurrently with the Gate's background probe loop without racing.
type fakeProber struct {
	err   atomic.Pointer[error]
	calls atomic.Int32
}

func (f *fakeProber) Probe(_ context.Context) error {
	f.calls.Add(1)
	if e := f.err.Load(); e != nil {
		return *e
	}
	return nil
}

func (f *fakeProber) setErr(err error) {
	if err == nil {
		f.err.Store(nil)
		return
	}
	f.err.Store(&err)
}

var _ = Describe("Gate", func() {
	logger := logr.Discard()

	It("UT-FLEET-READY-001: probes synchronously before Start returns, so Ready() is correct immediately", func() {
		p := &fakeProber{}
		gate := readiness.NewGate(time.Hour, logger, p)

		gate.Start(context.Background())
		defer gate.Stop()

		Expect(p.calls.Load()).To(Equal(int32(1)))
		Expect(gate.Ready()).To(BeTrue())
	})

	It("UT-FLEET-READY-002: Ready() reflects the last probe result", func() {
		p := &fakeProber{}
		p.setErr(errors.New("boom"))
		gate := readiness.NewGate(time.Hour, logger, p)

		gate.Start(context.Background())
		defer gate.Stop()

		Expect(gate.Ready()).To(BeFalse())
	})

	It("UT-FLEET-READY-003: ticker fires and re-probes at the configured interval", func() {
		p := &fakeProber{}
		gate := readiness.NewGate(20*time.Millisecond, logger, p)

		gate.Start(context.Background())
		defer gate.Stop()

		Eventually(func() int32 { return p.calls.Load() }, "500ms", "10ms").Should(BeNumerically(">=", 3))
	})

	It("UT-FLEET-READY-004: Ready() transitions back to true once the dependency recovers", func() {
		p := &fakeProber{}
		p.setErr(errors.New("boom"))
		gate := readiness.NewGate(20*time.Millisecond, logger, p)

		gate.Start(context.Background())
		defer gate.Stop()

		Expect(gate.Ready()).To(BeFalse())

		p.setErr(nil)

		Eventually(gate.Ready, "500ms", "10ms").Should(BeTrue())
	})

	It("UT-FLEET-READY-005: Stop halts the periodic probe loop", func() {
		p := &fakeProber{}
		gate := readiness.NewGate(10*time.Millisecond, logger, p)

		gate.Start(context.Background())
		Eventually(func() int32 { return p.calls.Load() }, "200ms", "5ms").Should(BeNumerically(">=", 2))

		gate.Stop()
		countAtStop := p.calls.Load()

		time.Sleep(100 * time.Millisecond)
		Expect(p.calls.Load()).To(Equal(countAtStop), "no further probes after Stop")
	})

	It("UT-FLEET-READY-006: context cancellation also halts the probe loop", func() {
		p := &fakeProber{}
		ctx, cancel := context.WithCancel(context.Background())
		gate := readiness.NewGate(10*time.Millisecond, logger, p)

		gate.Start(ctx)
		Eventually(func() int32 { return p.calls.Load() }, "200ms", "5ms").Should(BeNumerically(">=", 2))

		cancel()
		time.Sleep(50 * time.Millisecond)
		countAfterCancel := p.calls.Load()

		time.Sleep(100 * time.Millisecond)
		Expect(p.calls.Load()).To(Equal(countAfterCancel), "no further probes after context cancellation")
	})

	It("UT-FLEET-READY-007: Check adapts Ready() to the healthz.Checker func(*http.Request) error shape", func() {
		okProber := &fakeProber{}
		okGate := readiness.NewGate(time.Hour, logger, okProber)
		okGate.Start(context.Background())
		defer okGate.Stop()
		Expect(okGate.Check(&http.Request{})).To(Succeed())

		badProber := &fakeProber{}
		badProber.setErr(errors.New("mcp gateway unreachable"))
		badGate := readiness.NewGate(time.Hour, logger, badProber)
		badGate.Start(context.Background())
		defer badGate.Stop()

		err := badGate.Check(&http.Request{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("mcp gateway unreachable"))
	})

	It("UT-FLEET-READY-008: multiple probers are all probed; first failure marks NotReady", func() {
		p1 := &fakeProber{}
		p2 := &fakeProber{}
		p2.setErr(errors.New("second prober down"))
		gate := readiness.NewGate(time.Hour, logger, p1, p2)

		gate.Start(context.Background())
		defer gate.Stop()

		Expect(gate.Ready()).To(BeFalse())
		Expect(p1.calls.Load()).To(BeNumerically(">=", 1))
	})

	It("UT-FLEET-READY-009: Stop is safe to call multiple times and before Start", func() {
		p := &fakeProber{}
		gate := readiness.NewGate(time.Hour, logger, p)
		Expect(func() { gate.Stop() }).ToNot(Panic())
		Expect(func() { gate.Stop() }).ToNot(Panic())
	})

	It("UT-FLEET-READY-010: zero interval falls back to DefaultInterval instead of a busy-loop", func() {
		p := &fakeProber{}
		gate := readiness.NewGate(0, logger, p)

		gate.Start(context.Background())
		defer gate.Stop()

		// Only the synchronous initial probe should run within a short window.
		Consistently(func() int32 { return p.calls.Load() }, "100ms", "10ms").Should(Equal(int32(1)))
	})
})
