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

package tools_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// #2289: SignalInteractive (IS CRD creation) previously "failed open" on any
// error other than a session_active conflict -- a single transient K8s API
// hiccup silently downgraded an interactively-requested investigation to
// unconsented autonomous remediation. This suite proves the fix: bounded
// exponential-backoff retries (pkg/shared/backoff, DD-SHARED-001) for
// transient failures, and a hard fail-closed (the tool call errors out, no
// RR/investigation proceeds) once the retry budget is exhausted.
var _ = Describe("SignalInteractive bounded retry + fail-closed (#2289)", func() {

	Describe("signalInteractiveWithRetry", func() {
		It("UT-AF-2289-001: succeeds on the first attempt without retrying", func() {
			recorder := &recordingISSignaler{}
			name, err := tools.SignalInteractiveWithRetry(context.Background(), recorder, tools.SignalInteractiveRequest{
				RRNamespace: "kubernaut-system", RRName: "rr-1", TaskID: "task-1", Username: "user", JoinMode: "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("is-rr-1"))
			Expect(recorder.signalCalls).To(HaveLen(1))
		})

		It("UT-AF-2289-002: retries transient failures and succeeds once the signaler recovers", func() {
			recorder := &recordingISSignaler{
				signalErrFn: func(callNum int) error {
					if callNum < tools.SignalInteractiveMaxAttempts {
						return fmt.Errorf("transient k8s api error")
					}
					return nil
				},
			}
			name, err := tools.SignalInteractiveWithRetry(context.Background(), recorder, tools.SignalInteractiveRequest{
				RRNamespace: "kubernaut-system", RRName: "rr-2", TaskID: "task-2", Username: "user", JoinMode: "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("is-rr-2"))
			Expect(recorder.signalCalls).To(HaveLen(tools.SignalInteractiveMaxAttempts),
				"must retry until the last configured attempt before succeeding")
		})

		It("UT-AF-2289-003: exhausts all attempts and returns the last error when the signaler never recovers", func() {
			persistentErr := errors.New("persistent k8s api error")
			recorder := &recordingISSignaler{
				signalErrFn: func(int) error { return persistentErr },
			}
			_, err := tools.SignalInteractiveWithRetry(context.Background(), recorder, tools.SignalInteractiveRequest{
				RRNamespace: "kubernaut-system", RRName: "rr-3", TaskID: "task-3", Username: "user", JoinMode: "start",
			})
			Expect(err).To(MatchError(persistentErr))
			Expect(recorder.signalCalls).To(HaveLen(tools.SignalInteractiveMaxAttempts),
				"must attempt exactly SignalInteractiveMaxAttempts times before giving up")
		})

		It("UT-AF-2289-004: does not retry a session_active conflict -- single-driver enforcement is a legitimate rejection, not a transient failure", func() {
			sessionActiveErr := errors.New("session_active: another driver already attached")
			recorder := &recordingISSignaler{
				signalErrFn: func(int) error { return sessionActiveErr },
			}
			_, err := tools.SignalInteractiveWithRetry(context.Background(), recorder, tools.SignalInteractiveRequest{
				RRNamespace: "kubernaut-system", RRName: "rr-4", TaskID: "task-4", Username: "user", JoinMode: "start",
			})
			Expect(err).To(MatchError(sessionActiveErr))
			Expect(recorder.signalCalls).To(HaveLen(1), "session_active must fail fast, never retried")
		})
	})

	Describe("existing-rr_id (takeover) path fails closed after retries exhaust (#2289)", func() {
		It("UT-AF-2289-010: kubernaut_investigate returns an error and never starts KA when SignalInteractive persistently fails", func() {
			tc := newTypedClientForInvestigate()
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					Fail("KA investigation must never start when the interactive signal could not be established (fail closed)")
					return nil, nil
				},
			}
			persistentErr := errors.New("etcd unavailable")
			recorder := &recordingISSignaler{signalErrFn: func(int) error { return persistentErr }}
			auditRec := &auditRecorder{}

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{Username: "sre@kubernaut.ai", Groups: []string{"sre"}})
			_, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, &tools.InvestigateConfig{
					MCPClient: mockMCP,
					Client:    tc,
					Namespace: "kubernaut-system",
					Signaler:  recorder,
					Auditor:   auditRec,
				}, tools.InvestigateMCPArgs{RRID: "rr-2289-fail"},
				false, "",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to establish interactive session"))
			Expect(recorder.signalCalls).To(HaveLen(tools.SignalInteractiveMaxAttempts))

			Expect(auditRec.events).To(HaveLen(1))
			Expect(auditRec.events[0].Type).To(Equal(audit.EventInteractiveSignalFailed))
			Expect(auditRec.events[0].Detail["rr_id"]).To(Equal("rr-2289-fail"))
		})
	})

	Describe("new-RR path fails closed before RR creation (#2289)", func() {
		It("UT-AF-2289-020: kubernaut_investigate returns an error and creates no RemediationRequest when SignalInteractive persistently fails", func() {
			tc := newTypedClientForInvestigateWithUIDAssignment()
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					Fail("KA investigation must never start when the interactive signal could not be established (fail closed)")
					return nil, nil
				},
			}
			persistentErr := errors.New("etcd unavailable")
			recorder := &recordingISSignaler{signalErrFn: func(int) error { return persistentErr }}
			auditRec := &auditRecorder{}

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{Username: "sre@kubernaut.ai", Groups: []string{"sre"}})
			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, &tools.InvestigateConfig{
					MCPClient: mockMCP,
					Client:    tc,
					Namespace: "kubernaut-system",
					Signaler:  recorder,
					Triager:   defaultTestTriager("prod", "Deployment", "web-2289"),
					Auditor:   auditRec,
				}, tools.InvestigateMCPArgs{
					APIVersion: "apps/v1", Namespace: "prod", Kind: "Deployment", Name: "web-2289",
				},
				false, "sre-user",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to establish interactive session"))
			Expect(result.RRID).To(BeEmpty())

			var rrs remediationv1.RemediationRequestList
			Expect(tc.List(context.Background(), &rrs, crclient.InNamespace("kubernaut-system"))).To(Succeed())
			Expect(rrs.Items).To(BeEmpty(), "#2289: an unconsented autonomous RR must never be created when the interactive signal fails closed")

			Expect(auditRec.events).To(HaveLen(1))
			Expect(auditRec.events[0].Type).To(Equal(audit.EventInteractiveSignalFailed))
		})
	})
})
