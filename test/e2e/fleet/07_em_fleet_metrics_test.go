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

package fleet

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-FLEET-019: EM cluster-scoped Prometheus/AlertManager queries prevent
// fleet cluster collision (BR-EM-002, BR-EM-003, BR-FLEET-054, DD-EM-005
// v1.3, Issue #2274)
//
// Authority: Issue #54 (fleet E2E lane), Issue #2274, ADR-068, ADR-EM-001,
// DD-EM-005 v1.3 addendum
// FedRAMP: AU-3 (content of audit records -- cluster provenance in metrics
// must not be corrupted by a same-named resource on a different fleet
// cluster)
//
// SUPERSEDES the old E2E-FLEET-008 smoke checks (raw HTTP GET against
// Prometheus/AlertManager /api/v1/targets and /api/v2/status), which only
// proved the two services were *reachable* -- never that EM's own PromQL/
// AlertManager queries were cluster-scoped. That gap is exactly why Issue
// #2274 escaped this suite for as long as it did: this fleet Kind cluster
// runs a SINGLE shared Prometheus/AlertManager instance (no real Thanos
// federation across the primary + remote Kind clusters, see
// test/infrastructure/prometheus_alertmanager_e2e.go), so a namespace/
// alertname collision between "this remediation's fleet cluster" and
// "some other fleet cluster" was never exercised by anything that actually
// asked EM to score an EffectivenessAssessment.
//
// Both tests below simulate that collision directly against the real,
// already-deployed Prometheus/AlertManager (InjectMetrics/InjectAlerts,
// test/infrastructure/prometheus_alertmanager_e2e.go) by writing two series/
// alerts that share every label EM's query selects on EXCEPT `cluster`, then
// asserting the real EM controller -- reconciling a real EA CRD end-to-end,
// no mocks -- computes a score consistent with ONLY the matching-cluster
// data. Pre-#2274-fix, both scenarios below fail: PromQL's sum() blends
// both clusters' series (metrics case), and AlertManager returns both
// clusters' alerts for the shared alertname so the still-firing collision
// alert masks the real resolution (alerts case).
var _ = Describe("E2E-FLEET-019 [AU-3]: EM cluster-scoped assessment prevents fleet cluster collision (BR-EM-002, BR-EM-003, BR-FLEET-054, DD-EM-005 v1.3, Issue #2274)", Label("fleet"), func() {
	const (
		prometheusURL   = "http://localhost:9190"
		alertManagerURL = "http://localhost:9193"

		// matchingClusterID is the fleet cluster identity the EA under test
		// claims to belong to (ea.Spec.ClusterID). It doesn't need to be one
		// of the suite's real MCP-registered identities (remote-cluster/
		// prod-east/prod-west) -- EM's Prometheus/AlertManager cluster
		// matcher is a plain Thanos external-label lookup, independent of
		// MCP cluster registration.
		matchingClusterID = "remote-cluster"
		// collisionClusterID simulates a second, unrelated fleet cluster
		// that happens to report data for a same-named resource/alertname.
		collisionClusterID = "collision-cluster"
	)

	// fleetEAName / fleetEACorrelation generate parallel-safe unique names so
	// concurrent Ginkgo processes (and repeated local runs) never collide.
	uniqueSuffix := func(prefix string) string {
		return fmt.Sprintf("%s-p%d-%d", prefix, GinkgoParallelProcess(), time.Now().UnixNano()%100000)
	}

	// seedWorkflowAuditEvents satisfies EM's no_execution guard and #573 G4
	// full-scope gate (ADR-EM-001 Section 5): without both audit events, EM
	// stops at a reduced assessment scope and never queries Prometheus/
	// AlertManager at all, which would make either test below a false
	// positive (mirrors test/e2e/effectivenessmonitor/helpers_test.go's
	// seedWorkflowStartedEvent/seedWorkflowCompletedEvent, adapted to this
	// suite's dataStorageClient).
	seedWorkflowAuditEvents := func(correlationID string) {
		GinkgoHelper()
		started := &ogenclient.AuditEventRequest{
			Version:        "1.0",
			EventType:      "workflowexecution.execution.started",
			EventTimestamp: time.Now().UTC(),
			EventCategory:  ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
			EventAction:    "started",
			EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
			CorrelationID:  correlationID,
			EventData: ogenclient.NewAuditEventRequestEventDataWorkflowexecutionExecutionStartedAuditEventRequestEventData(
				ogenclient.WorkflowExecutionAuditPayload{
					EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionExecutionStarted,
					WorkflowID:      "e2e-fleet-2274-workflow",
					WorkflowVersion: "v1.0.0",
					TargetResource:  "Deployment/collision-target",
					Phase:           ogenclient.WorkflowExecutionAuditPayloadPhaseRunning,
					ContainerImage:  "registry.io/test/workflow:latest",
					ExecutionName:   fmt.Sprintf("wfe-%s", correlationID),
				},
			),
		}
		_, err := dataStorageClient.CreateAuditEvent(ctx, started)
		Expect(err).ToNot(HaveOccurred(), "failed to seed execution.started event for %s", correlationID)

		completed := &ogenclient.AuditEventRequest{
			Version:        "1.0",
			EventType:      "workflowexecution.workflow.completed",
			EventTimestamp: time.Now().UTC(),
			EventCategory:  ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
			EventAction:    "completed",
			EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
			CorrelationID:  correlationID,
			EventData: ogenclient.NewAuditEventRequestEventDataWorkflowexecutionWorkflowCompletedAuditEventRequestEventData(
				ogenclient.WorkflowExecutionAuditPayload{
					EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowCompleted,
					WorkflowID:      "e2e-fleet-2274-workflow",
					WorkflowVersion: "v1.0.0",
					TargetResource:  "Deployment/collision-target",
					Phase:           ogenclient.WorkflowExecutionAuditPayloadPhaseCompleted,
					ContainerImage:  "registry.io/test/workflow:latest",
					ExecutionName:   fmt.Sprintf("wfe-%s", correlationID),
				},
			),
		}
		_, err = dataStorageClient.CreateAuditEvent(ctx, completed)
		Expect(err).ToNot(HaveOccurred(), "failed to seed workflow.completed event for %s", correlationID)
	}

	// createFleetEA creates an EA CRD in kubernaut-system (ADR-057) with
	// Spec.ClusterID set, mirroring a real fleet remediation whose signal
	// originated on a remote cluster.
	createFleetEA := func(name, correlationID, clusterID, targetNamespace string) {
		GinkgoHelper()
		target := eav1.TargetResource{
			Kind:      "Deployment",
			Name:      "collision-target",
			Namespace: targetNamespace,
		}
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace, // kubernaut-system (package const)
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           correlationID,
				ClusterID:               clusterID,
				RemediationRequestPhase: "Completed",
				SignalTarget:            target,
				RemediationTarget:       target,
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 5 * time.Second},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed(), "failed to create EA %s/%s", namespace, name)
	}

	waitForFleetEAPhase := func(name, expectedPhase string) *eav1.EffectivenessAssessment {
		GinkgoHelper()
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() string {
			if err := apiReader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, ea); err != nil {
				return ""
			}
			return ea.Status.Phase
		}, timeout, interval).Should(Equal(expectedPhase),
			"EA %s/%s did not reach phase %s", namespace, name, expectedPhase)
		return ea
	}

	// ========================================================================
	// E2E-FLEET-019a: Metrics collision (BR-EM-003)
	// ========================================================================
	It("E2E-FLEET-019a: cluster-scoped PromQL prevents blending memory readings across colliding fleet clusters", func() {
		targetNS := uniqueSuffix("em-2274-fleet-ns")
		correlationID := uniqueSuffix("corr-2274-metrics")
		eaName := uniqueSuffix("ea-2274-metrics")
		now := time.Now()

		// Same namespace, same pod name -- as if a Deployment named
		// "collision-target" independently exists on TWO fleet clusters.
		// EM's namespace-scoped PromQL selector only filters on `namespace`
		// (no pod matcher, see buildMetricQuerySpecs), so without the
		// cluster= matcher these two series are indistinguishable to
		// sum(container_memory_working_set_bytes{namespace="..."}).
		matchingLabels := map[string]string{
			"namespace":                    targetNS,
			"pod":                          "collision-target-abc",
			"container":                    "workload",
			infrastructure.ClusterLabelKey: matchingClusterID,
		}
		collisionLabels := map[string]string{
			"namespace":                    targetNS,
			"pod":                          "collision-target-xyz",
			"container":                    "workload",
			infrastructure.ClusterLabelKey: collisionClusterID,
		}

		By("Injecting a genuine memory IMPROVEMENT for the matching (remote-cluster) fleet cluster")
		matchingSeries := []infrastructure.TestMetric{
			{Name: "container_memory_working_set_bytes", Labels: matchingLabels, Value: 500_000_000, Timestamp: now.Add(-20 * time.Second)},
			{Name: "container_memory_working_set_bytes", Labels: matchingLabels, Value: 200_000_000, Timestamp: now},
		}
		Expect(infrastructure.InjectMetrics(ctx, prometheusURL, matchingSeries)).To(Succeed(),
			"failed to inject matching-cluster metric series")

		By("Injecting an unrelated memory REGRESSION for the colliding fleet cluster (same namespace label)")
		collisionSeries := []infrastructure.TestMetric{
			{Name: "container_memory_working_set_bytes", Labels: collisionLabels, Value: 100_000_000, Timestamp: now.Add(-20 * time.Second)},
			{Name: "container_memory_working_set_bytes", Labels: collisionLabels, Value: 800_000_000, Timestamp: now},
		}
		Expect(infrastructure.InjectMetrics(ctx, prometheusURL, collisionSeries)).To(Succeed(),
			"failed to inject collision-cluster metric series")

		// Unscoped sum() would see: early = 500M+100M = 600M, late = 200M+800M = 1000M
		// -- a net INCREASE (regression), masking the matching cluster's real
		// improvement. Only a cluster="remote-cluster" matcher isolates the
		// true 500M -> 200M improvement.
		By("Seeding workflow audit events (no_execution guard + #573 G4 full scope)")
		seedWorkflowAuditEvents(correlationID)

		By("Creating a fleet-scoped EA (ClusterID=remote-cluster) targeting the colliding resource")
		createFleetEA(eaName, correlationID, matchingClusterID, targetNS)

		By("Waiting for EM to complete the assessment")
		ea := waitForFleetEAPhase(eaName, eav1.PhaseCompleted)

		By("Verifying MetricsScore reflects ONLY the matching cluster's genuine improvement")
		Expect(ea.Status.Components.MetricsAssessed).To(BeTrue(), "metrics component should be assessed")
		Expect(ea.Status.Components.MetricsScore).ToNot(BeNil(), "metrics score should be set")
		Expect(*ea.Status.Components.MetricsScore).To(BeNumerically(">", 0.0),
			"Issue #2274 regression: MetricsScore must reflect the matching fleet cluster's improvement "+
				"(500M->200M), not a blend with the colliding cluster's regression (100M->800M) which "+
				"would net a REGRESSION and drive the score to 0")
	})

	// ========================================================================
	// E2E-FLEET-019b: Alert resolution collision (BR-EM-002)
	// ========================================================================
	It("E2E-FLEET-019b: cluster-scoped AlertManager query prevents a colliding cluster's firing alert from masking resolution", func() {
		correlationID := uniqueSuffix("corr-2274-alert")
		eaName := uniqueSuffix("ea-2274-alert")
		targetNS := uniqueSuffix("em-2274-fleet-alert-ns")

		// EM queries AlertManager using alertname=<correlationID> (reconciler.go
		// assessAlert). Both alerts below share that alertname -- as if the
		// same correlation ID/alert definition coincidentally also exists on
		// a second fleet cluster, unresolved, at the exact moment EM assesses
		// the FIRST cluster's (already-resolved) remediation.
		By("Injecting a RESOLVED alert for the matching (remote-cluster) fleet cluster")
		resolvedAlert := []infrastructure.TestAlert{
			{
				Name: correlationID,
				Labels: map[string]string{
					"namespace":                    targetNS,
					infrastructure.ClusterLabelKey: matchingClusterID,
					"severity":                     "warning",
				},
				Annotations: map[string]string{"summary": "High memory usage (test, resolved on remote-cluster)"},
				Status:      "resolved",
				StartsAt:    time.Now().Add(-10 * time.Minute),
				EndsAt:      time.Now().Add(-1 * time.Minute),
			},
		}
		Expect(infrastructure.InjectAlerts(alertManagerURL, resolvedAlert)).To(Succeed(),
			"failed to inject resolved matching-cluster alert")

		By("Injecting a still-FIRING alert with the SAME alertname for the colliding fleet cluster")
		firingAlert := []infrastructure.TestAlert{
			{
				Name: correlationID,
				Labels: map[string]string{
					"namespace":                    targetNS,
					infrastructure.ClusterLabelKey: collisionClusterID,
					"severity":                     "critical",
				},
				Annotations: map[string]string{"summary": "High memory usage (test, still firing on collision-cluster)"},
				Status:      "firing",
				StartsAt:    time.Now(),
				EndsAt:      time.Now().Add(10 * time.Minute), // keep firing beyond resolve_timeout (1m)
			},
		}
		Expect(infrastructure.InjectAlerts(alertManagerURL, firingAlert)).To(Succeed(),
			"failed to inject firing collision-cluster alert")

		By("Seeding workflow audit events (no_execution guard + #573 G4 full scope)")
		seedWorkflowAuditEvents(correlationID)

		By("Creating a fleet-scoped EA (ClusterID=remote-cluster) for the resolved remediation")
		createFleetEA(eaName, correlationID, matchingClusterID, targetNS)

		By("Waiting for EM to complete the assessment")
		ea := waitForFleetEAPhase(eaName, eav1.PhaseCompleted)

		By("Verifying AlertScore reflects ONLY the matching cluster's resolution (1.0), not the colliding firing alert")
		Expect(ea.Status.Components.AlertAssessed).To(BeTrue(), "alert component should be assessed")
		Expect(ea.Status.Components.AlertScore).ToNot(BeNil(), "alert score should be set")
		Expect(*ea.Status.Components.AlertScore).To(Equal(1.0),
			"Issue #2274 regression: AlertScore must be 1.0 (resolved on the matching fleet cluster), "+
				"not masked to 0.0 by the colliding cluster's same-alertname alert that is still firing")
	})
})
