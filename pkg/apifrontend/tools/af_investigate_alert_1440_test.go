package tools_test

import (
	"context"
	"errors"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// signalerCall records a single call to SignalInteractive for assertions.
type signalerCall struct {
	taskID   string
	rrName   string
	username string
	groups   []string
}

// recordingSignaler captures AlertISSignaler calls for test assertions.
type recordingSignaler struct {
	mu    sync.Mutex
	calls []signalerCall
	err   error
}

func (r *recordingSignaler) SignalInteractive(_ context.Context, taskID, rrName, username string, groups []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, signalerCall{
		taskID:   taskID,
		rrName:   rrName,
		username: username,
		groups:   groups,
	})
	return r.err
}

func (r *recordingSignaler) Calls() []signalerCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]signalerCall, len(r.calls))
	copy(cp, r.calls)
	return cp
}

var _ = Describe("Fix #1440: IS CRD co-creation in HandleInvestigateAlert", func() {
	baseCfg := func() tools.InvestigateAlertConfig {
		return tools.InvestigateAlertConfig{
			Client:       newTypedFakeClient(),
			ControllerNS: "kubernaut-system",
		}
	}

	validArgs := func() *tools.InvestigateAlertArgs {
		return &tools.InvestigateAlertArgs{
			AlertName:  "KubePodCrashLooping",
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "web",
			Namespace:  "prod",
		}
	}

	Describe("IS CRD co-creation — signaler invocation (UT-AF-1440-001..005)", func() {
		It("UT-AF-1440-001: calls Signaler.SignalInteractive when Signaler is provided and RR is new (SI-4)", func() {
			recorder := &recordingSignaler{}
			cfg := baseCfg()
			cfg.Signaler = recorder

			result, err := tools.HandleInvestigateAlert(context.Background(), cfg, validArgs(), "sre-alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
			Expect(result.AlreadyExists).To(BeFalse())

			calls := recorder.Calls()
			Expect(calls).To(HaveLen(1), "Signaler.SignalInteractive must be called exactly once for new RR")
			Expect(calls[0].rrName).To(Equal(extractRRName(result.RRID)))
		})

		It("UT-AF-1440-002: calls Signaler when RR AlreadyExists (SI-4)", func() {
			recorder := &recordingSignaler{}
			cfg := baseCfg()
			cfg.Signaler = recorder

			// First call creates the RR
			result1, err := tools.HandleInvestigateAlert(context.Background(), cfg, validArgs(), "sre-alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.AlreadyExists).To(BeFalse())

			// Second call hits AlreadyExists
			result2, err := tools.HandleInvestigateAlert(context.Background(), cfg, validArgs(), "sre-alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(result2.AlreadyExists).To(BeTrue())

			calls := recorder.Calls()
			Expect(calls).To(HaveLen(2),
				"Signaler must be called for both new and existing RR (user intent is interactive)")
		})

		It("UT-AF-1440-003: succeeds when Signaler is nil — backward compatibility (SC-24)", func() {
			cfg := baseCfg()
			// cfg.Signaler is nil by default

			result, err := tools.HandleInvestigateAlert(context.Background(), cfg, validArgs(), "sre-alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty(), "must succeed without Signaler for backward compat")
		})

		It("UT-AF-1440-004: succeeds when Signaler returns error — best-effort, non-blocking (SC-24)", func() {
			recorder := &recordingSignaler{err: errors.New("IS CRD creation failed: simulated")}
			cfg := baseCfg()
			cfg.Signaler = recorder

			result, err := tools.HandleInvestigateAlert(context.Background(), cfg, validArgs(), "sre-alice")
			Expect(err).NotTo(HaveOccurred(),
				"HandleInvestigateAlert must NOT fail when Signaler errors (best-effort)")
			Expect(result.RRID).NotTo(BeEmpty())

			Expect(recorder.Calls()).To(HaveLen(1), "Signaler must still be called")
		})

		It("UT-AF-1440-005: Signaler receives correct taskID, username, groups from auth context (AC-3, AU-12)", func() {
			recorder := &recordingSignaler{}
			cfg := baseCfg()
			cfg.Signaler = recorder

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre-alice",
				Groups:   []string{"sre-team", "oncall"},
			})

			result, err := tools.HandleInvestigateAlert(ctx, cfg, validArgs(), "sre-alice")
			Expect(err).NotTo(HaveOccurred())

			calls := recorder.Calls()
			Expect(calls).To(HaveLen(1))
			call := calls[0]
			Expect(call.taskID).To(Equal("a2a-"+extractRRName(result.RRID)),
				"taskID must follow a2a-{RRID} convention")
			Expect(call.username).To(Equal("sre-alice"))
			Expect(call.groups).To(Equal([]string{"sre-team", "oncall"}))
		})
	})
})
