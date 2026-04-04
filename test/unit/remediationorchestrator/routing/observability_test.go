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

// Issue #616: RO observability tests for CheckIneffectiveRemediationChain.
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// TP-616-v1.1: Validates dsClient nil logging and post-query entry count logging.
package routing

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// testLogEntry captures a single log message with its verbosity level.
type testLogEntry struct {
	Level   int
	Message string
}

// testLogStore is a shared, mutex-protected store of log entries.
// All child sinks derived via WithValues/WithName share the same store.
type testLogStore struct {
	mu      sync.Mutex
	entries []testLogEntry
}

func (st *testLogStore) append(entry testLogEntry) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.entries = append(st.entries, entry)
}

func (st *testLogStore) getEntries() []testLogEntry {
	st.mu.Lock()
	defer st.mu.Unlock()
	cp := make([]testLogEntry, len(st.entries))
	copy(cp, st.entries)
	return cp
}

// testLogSink captures log messages for assertion. Implements logr.LogSink.
type testLogSink struct {
	store  *testLogStore
	name   string
	values []interface{}
}

func (s *testLogSink) Init(logr.RuntimeInfo)  {}
func (s *testLogSink) Enabled(level int) bool { return true }
func (s *testLogSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	merged := make([]interface{}, len(s.values), len(s.values)+len(keysAndValues))
	copy(merged, s.values)
	merged = append(merged, keysAndValues...)
	return &testLogSink{store: s.store, name: s.name, values: merged}
}
func (s *testLogSink) WithName(name string) logr.LogSink {
	return &testLogSink{store: s.store, name: name, values: s.values}
}
func (s *testLogSink) Info(level int, msg string, keysAndValues ...interface{}) {
	s.store.append(testLogEntry{Level: level, Message: msg})
}
func (s *testLogSink) Error(err error, msg string, keysAndValues ...interface{}) {
	s.store.append(testLogEntry{Level: -1, Message: msg})
}

var _ = Describe("Issue #616: CheckIneffectiveRemediationChain Observability", Label("unit", "issue-616"), func() {

	var (
		ctx      context.Context
		logStore *testLogStore
	)

	target := routing.TargetResource{
		Kind:      "Deployment",
		Name:      "nginx",
		Namespace: "default",
	}

	makeRR := func() *remediationv1.RemediationRequest {
		return &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-616-obs",
				Namespace: "default",
				UID:       types.UID("rr-616-obs-uid"),
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "fp-616-obs",
			},
		}
	}

	setupEngineWithQuerier := func(querier routing.RemediationHistoryQuerier) *routing.RoutingEngine {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		config := routing.Config{
			ConsecutiveFailureThreshold:   3,
			ConsecutiveFailureCooldown:    3600,
			RecentlyRemediatedCooldown:    300,
			ExponentialBackoffBase:        60,
			ExponentialBackoffMax:         600,
			ExponentialBackoffMaxExponent: 4,
			IneffectiveChainThreshold:     3,
			RecurrenceCountThreshold:      5,
			IneffectiveTimeWindow:         4 * time.Hour,
			ForwardChainThreshold:         2,
			ForwardChainWindow:            1 * time.Hour,
		}

		if querier != nil {
			return routing.NewRoutingEngine(fakeClient, fakeClient, "default", config, &mocks.AlwaysManagedScopeChecker{}, querier)
		}
		return routing.NewRoutingEngine(fakeClient, fakeClient, "default", config, &mocks.AlwaysManagedScopeChecker{})
	}

	BeforeEach(func() {
		logStore = &testLogStore{}
		sink := &testLogSink{store: logStore}
		logger := logr.New(sink)
		ctx = log.IntoContext(context.Background(), logger)
	})

	It("UT-RO-616-001: should log V(1) Info when dsClient is nil", func() {
		engine := setupEngineWithQuerier(nil)

		result := engine.CheckIneffectiveRemediationChain(ctx, makeRR(), target, "sha256:pre", "RestartPod")
		Expect(result).To(BeNil())

		entries := logStore.getEntries()
		var found bool
		for _, e := range entries {
			if e.Level == 1 && e.Message == "dsClient is nil, skipping ineffective chain detection" {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "Expected V(1) log message about dsClient being nil; got entries: %+v", entries)
	})

	It("UT-RO-616-002: should log V(1) Info with entry count after successful DS query", func() {
		querier := &mockHistoryQuerier{
			entries: []ogenclient.RemediationHistoryEntry{
				{RemediationUID: "rr-1", CompletedAt: time.Now()},
				{RemediationUID: "rr-2", CompletedAt: time.Now()},
			},
		}
		engine := setupEngineWithQuerier(querier)

		_ = engine.CheckIneffectiveRemediationChain(ctx, makeRR(), target, "sha256:pre", "RestartPod")

		entries := logStore.getEntries()
		var found bool
		for _, e := range entries {
			if e.Level == 1 && e.Message == "Ineffective chain detection: DS query completed" {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "Expected V(1) log message with entry count after DS query; got entries: %+v", entries)
	})
})
