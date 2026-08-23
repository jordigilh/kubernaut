package tools_test

import (
	"context"
	"errors"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	k8stypes "k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
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

// alertBackfillCall records a single call to BackfillOwnerReference (#2265).
type alertBackfillCall struct {
	rrNamespace, rrName string
	rrUID               k8stypes.UID
}

// recordingSignaler captures AlertISSignaler calls for test assertions.
type recordingSignaler struct {
	mu            sync.Mutex
	calls         []signalerCall
	backfillCalls []alertBackfillCall
	err           error
	// onSignal, when set, is invoked synchronously from SignalInteractive
	// (before returning) so a test can probe co-temporal state (#2265's
	// ordering proof: is the RR visible yet?).
	onSignal func(ctx context.Context, rrName string)
}

func (r *recordingSignaler) SignalInteractive(ctx context.Context, taskID, rrName, username string, groups []string) error {
	if r.onSignal != nil {
		r.onSignal(ctx, rrName)
	}
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

func (r *recordingSignaler) BackfillOwnerReference(_ context.Context, rrNamespace, rrName string, rrUID k8stypes.UID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backfillCalls = append(r.backfillCalls, alertBackfillCall{rrNamespace: rrNamespace, rrName: rrName, rrUID: rrUID})
}

func (r *recordingSignaler) Calls() []signalerCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]signalerCall, len(r.calls))
	copy(cp, r.calls)
	return cp
}

func (r *recordingSignaler) BackfillCalls() []alertBackfillCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]alertBackfillCall, len(r.backfillCalls))
	copy(cp, r.backfillCalls)
	return cp
}

var _ = Describe("Fix #1440: IS CRD co-creation in HandleInvestigateAlert", func() {
	baseCfg := func() tools.InvestigateAlertConfig {
		return tools.InvestigateAlertConfig{
			Client:       newTypedFakeClient(),
			ControllerNS: "kubernaut-system",
			Triager:      defaultTestTriager("prod", "Deployment", "web"),
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

	// #2265 / DD-AF-013: SignalInteractive must fire from HandleCreateRR's
	// BeforeCreate hook -- strictly before the RR becomes visible to any
	// other component -- instead of the old post-hoc call issued after RR
	// creation had already completed.
	Describe("IS-before-RR ordering (#2265)", func() {
		It("UT-AF-2265-201: SignalInteractive fires with the about-to-be-created RR's name before the RR is visible to any reader", func() {
			tc := newTypedFakeClientWithUIDAssignment()
			cfg := baseCfg()
			cfg.Client = tc

			recorder := &recordingSignaler{}
			var rrVisibleAtSignalTime bool
			recorder.onSignal = func(ctx context.Context, rrName string) {
				var probe remediationv1.RemediationRequest
				err := tc.Get(ctx, crclient.ObjectKey{Namespace: "kubernaut-system", Name: rrName}, &probe)
				rrVisibleAtSignalTime = err == nil
			}
			cfg.Signaler = recorder

			result, err := tools.HandleInvestigateAlert(context.Background(), cfg, validArgs(), "sre-alice")
			Expect(err).NotTo(HaveOccurred())

			calls := recorder.Calls()
			Expect(calls).To(HaveLen(1), "SignalInteractive must fire exactly once for a fresh RR (no duplicate post-hoc call)")
			Expect(calls[0].rrName).To(Equal(extractRRName(result.RRID)))
			Expect(rrVisibleAtSignalTime).To(BeFalse(),
				"#2265: SignalInteractive must fire before the RR becomes visible to any reader")
		})

		It("UT-AF-2265-202: BackfillOwnerReference fires with the persisted RR's namespace/name/UID once the RR is created", func() {
			tc := newTypedFakeClientWithUIDAssignment()
			cfg := baseCfg()
			cfg.Client = tc
			recorder := &recordingSignaler{}
			cfg.Signaler = recorder

			result, err := tools.HandleInvestigateAlert(context.Background(), cfg, validArgs(), "sre-alice")
			Expect(err).NotTo(HaveOccurred())

			backfills := recorder.BackfillCalls()
			Expect(backfills).To(HaveLen(1), "#1300: OwnerReference backfill must fire once the RR exists")
			Expect(backfills[0].rrName).To(Equal(extractRRName(result.RRID)))
			Expect(backfills[0].rrNamespace).To(Equal("kubernaut-system"))
			Expect(backfills[0].rrUID).NotTo(BeEmpty())
		})

		It("UT-AF-2265-203: the dedup (AlreadyExists) branch still signals via the existing post-hoc path, not the hook, without double-signaling the fresh-create call", func() {
			tc := newTypedFakeClientWithUIDAssignment()
			cfg := baseCfg()
			cfg.Client = tc
			recorder := &recordingSignaler{}
			cfg.Signaler = recorder

			first, err := tools.HandleInvestigateAlert(context.Background(), cfg, validArgs(), "sre-alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(first.AlreadyExists).To(BeFalse())

			second, err := tools.HandleInvestigateAlert(context.Background(), cfg, validArgs(), "sre-alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(second.AlreadyExists).To(BeTrue())

			Expect(recorder.Calls()).To(HaveLen(2), "one signal from the fresh-create hook, one from the dedup branch's post-hoc call")
			Expect(recorder.BackfillCalls()).To(HaveLen(1), "backfill only applies to the genuinely-new RR from the first call")
		})

		It("UT-AF-2265-204: a signaler error during BeforeCreate does not abort RR creation (fire-and-forget, unchanged from pre-#2265 semantics)", func() {
			tc := newTypedFakeClientWithUIDAssignment()
			cfg := baseCfg()
			cfg.Client = tc
			recorder := &recordingSignaler{err: errors.New("IS CRD creation failed: simulated")}
			cfg.Signaler = recorder

			result, err := tools.HandleInvestigateAlert(context.Background(), cfg, validArgs(), "sre-alice")
			Expect(err).NotTo(HaveOccurred(), "HandleInvestigateAlert must NOT fail when the signaler errors (best-effort, SC-24)")
			Expect(result.RRID).NotTo(BeEmpty())
			Expect(recorder.Calls()).To(HaveLen(1), "signaler must still be attempted exactly once, not retried via the post-hoc path")
		})
	})
})
