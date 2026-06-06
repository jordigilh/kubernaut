package tools_test

import (
	"context"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

type noopPromClient struct{}

func (n *noopPromClient) GetAlerts(_ context.Context) ([]prom.Alert, error) {
	return nil, nil
}
func (n *noopPromClient) GetRules(_ context.Context) ([]prom.RuleGroup, error) {
	return nil, nil
}
func (n *noopPromClient) InstantQuery(_ context.Context, _ string) (*prom.QueryResult, error) {
	return &prom.QueryResult{}, nil
}

type alertOverridePromClient struct {
	alerts      []prom.Alert
	ruleGroups  []prom.RuleGroup
	queryResult *prom.QueryResult
}

func (a *alertOverridePromClient) GetAlerts(_ context.Context) ([]prom.Alert, error) {
	return a.alerts, nil
}
func (a *alertOverridePromClient) GetRules(_ context.Context) ([]prom.RuleGroup, error) {
	return a.ruleGroups, nil
}
func (a *alertOverridePromClient) InstantQuery(_ context.Context, _ string) (*prom.QueryResult, error) {
	if a.queryResult != nil {
		return a.queryResult, nil
	}
	return &prom.QueryResult{}, nil
}

var getOpts = metav1.GetOptions{}

func extractRRName(rrid string) string {
	parts := strings.SplitN(rrid, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return rrid
}

var _ = Describe("HandleCreateRR (#1282 refactor)", func() {
	rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}
	eventsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

	newFakeClient := func(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
		scheme := runtime.NewScheme()
		return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				rrGVR:     "RemediationRequestList",
				eventsGVR: "EventList",
			},
			objects...)
	}

	Describe("CreateRRArgs minimization (F-MIN)", func() {
		It("UT-AF-1282-MIN-001: creates RR with only Kind, Name, Description", func() {
			client := newFakeClient()

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace:   "prod",
				Kind:        "Deployment",
				Name:        "web",
				Description: "Pod CrashLoopBackOff detected",
				APIVersion:  "apps/v1",
			}, "sre-user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
			Expect(result.AlreadyExists).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("created"))
		})

		It("UT-AF-1282-MIN-002: empty kind rejected", func() {
			client := newFakeClient()
			_, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "", Name: "web", Description: "x", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).To(MatchError(ContainSubstring("invalid input")))
		})

		It("UT-AF-1282-MIN-003: empty name rejected", func() {
			client := newFakeClient()
			_, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "", Description: "x", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).To(MatchError(ContainSubstring("invalid input")))
		})

		It("UT-AF-1282-MIN-004: long description truncated not rejected", func() {
			client := newFakeClient()
			longDesc := make([]byte, 4096)
			for i := range longDesc {
				longDesc[i] = 'a'
			}

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: string(longDesc), APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-1282-MIN-005: concurrent calls with same fingerprint are deduplicated", func() {
			client := newFakeClient()

			var wg sync.WaitGroup
			results := make([]tools.CreateRRResult, 5)
			errs := make([]error, 5)

			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					results[idx], errs[idx] = tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
						Namespace: "prod", Kind: "Deployment", Name: "dedup-target", Description: "concurrent test", APIVersion: "apps/v1",
					}, "user", nil, nil)
				}(i)
			}
			wg.Wait()

			for _, err := range errs {
				Expect(err).NotTo(HaveOccurred())
			}

			firstRRID := results[0].RRID
			for _, r := range results[1:] {
				Expect(r.RRID).To(Equal(firstRRID))
			}
		})

		It("UT-AF-1282-MIN-006: severity resolved by Triager when available", func() {
			client := newFakeClient()
			noopLLM := severity.NewNoopLLMTriager(logr.Discard())
			cfg := severity.DefaultConfig()
			triager := severity.NewTriager(&noopPromClient{}, noopLLM, cfg, logr.Discard())

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "test triage", APIVersion: "apps/v1",
			}, "alice", triager, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-1282-MIN-007: nil Triager defaults severity to medium", func() {
			client := newFakeClient()

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "no triager", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})
	})

	Describe("Namespace resolution (F-NS)", func() {
		It("UT-AF-1282-NS-005: namespace comes from AF, not LLM args", func() {
			client := newFakeClient()

			result, err := tools.HandleCreateRR(context.Background(), client, "kubernaut-system", &tools.CreateRRArgs{
				Namespace: "kubernaut-system", Kind: "Deployment", Name: "web", Description: "ns from AF", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).To(HavePrefix("rr-"))
		})

		It("UT-AF-1282-NS-006: empty namespace from AF is rejected", func() {
			client := newFakeClient()
			_, err := tools.HandleCreateRR(context.Background(), client, "", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "x", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).To(MatchError(ContainSubstring("invalid input")))
		})

		It("UT-AF-1282-NS-007: invalid namespace from AF (path traversal) rejected", func() {
			client := newFakeClient()
			_, err := tools.HandleCreateRR(context.Background(), client, "../../etc", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "x", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).To(MatchError(ContainSubstring("invalid input")))
		})
	})

	Describe("Signal source (F-SRC)", func() {
		It("UT-AF-1282-SRC-001: created RR has signalSource=a2a-agent", func() {
			client := newFakeClient()

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "check source", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())

			created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())

			source, _, _ := unstructured.NestedString(created.Object, "spec", "signalSource")
			Expect(source).To(Equal("a2a-agent"))
		})

		It("UT-AF-1282-SRC-002: dedup does not create new RR (signalSource not applicable)", func() {
			rr := newUnstructuredRR("prod", "rr-deploy-web-existing", "Executing", "Deployment", "web")
			client := newFakeClient(rr)

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "dup", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.AlreadyExists).To(BeTrue())
		})
	})

	Describe("Signal name grounding (F-SIG)", func() {
		It("UT-AF-1282-SIG-006: signalName falls back to unknown when no events exist", func() {
			client := newFakeClient()

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "check signal name", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())

			created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())

			signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
			Expect(signalName).To(Equal("unknown"))
		})

		It("UT-AF-1282-SIG-009: K8s events fallback — OOMKilling event becomes signalName", func() {
			ev := newUnstructuredEventWithType("prod", "ev-oom", "OOMKilling", "killed", "Deployment", "web", "Warning")
			client := newFakeClient(ev)

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "OOM detected", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())

			created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())

			signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
			Expect(signalName).To(Equal("OOMKilling"))
		})

		It("UT-AF-1282-SIG-010: no events and no triager → unknown fallback", func() {
			client := newFakeClient()

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "StatefulSet", Name: "db", Description: "no events", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())

			created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())

			signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
			Expect(signalName).To(Equal("unknown"))
		})

		It("UT-AF-1282-SIG-011: triager AlertName takes precedence over K8s events", func() {
			ev := newUnstructuredEventWithType("prod", "ev-bo", "BackOff", "crash", "Deployment", "web", "Warning")
			client := newFakeClient(ev)
			noopLLM := severity.NewNoopLLMTriager(logr.Discard())
			cfg := severity.DefaultConfig()

			mockProm := &alertOverridePromClient{
				alerts: []prom.Alert{
					{State: "firing", Labels: map[string]string{
						"alertname": "HighErrorRate",
						"namespace": "prod",
						"kind":      "Deployment",
						"name":      "web",
						"severity":  "critical",
					}},
				},
			}
			triager := severity.NewTriager(mockProm, noopLLM, cfg, logr.Discard())

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "alert-based", APIVersion: "apps/v1",
			}, "user", triager, nil)
			Expect(err).NotTo(HaveOccurred())

			created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())

			signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
			Expect(signalName).To(Equal("HighErrorRate"))
		})
	})

	It("UT-AF-1282-SIG-012: triager RuleName used when AlertName is empty", func() {
		ev := newUnstructuredEventWithType("prod", "ev-bo", "BackOff", "crash", "Deployment", "api", "Warning")
		client := newFakeClient(ev)
		noopLLM := severity.NewNoopLLMTriager(logr.Discard())
		cfg := severity.DefaultConfig()

		mockProm := &alertOverridePromClient{
			alerts: []prom.Alert{},
			ruleGroups: []prom.RuleGroup{
				{Name: "test-rules", Rules: []prom.Rule{
					{Name: "HighMemoryUsage", State: "inactive",
						Query:  `container_memory_usage_bytes{namespace="prod"}`,
						Labels: map[string]string{"severity": "warning"}, Type: "alerting"},
				}},
			},
			queryResult: &prom.QueryResult{
				Samples: []prom.Sample{
					{Value: 85, Metric: map[string]string{"namespace": "prod"}},
				},
			},
		}
		triager := severity.NewTriager(mockProm, noopLLM, cfg, logr.Discard())

		result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "api", Description: "rule-based", APIVersion: "apps/v1",
		}, "user", triager, nil)
		Expect(err).NotTo(HaveOccurred())

		created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
			context.Background(), extractRRName(result.RRID), getOpts)
		Expect(getErr).NotTo(HaveOccurred())

		signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
		Expect(signalName).To(Equal("HighMemoryUsage"),
			"RuleName should take precedence over K8s events when AlertName is empty")
	})

	It("UT-AF-1282-SIG-013: Pod BackOff cascades when Deployment only has lifecycle events", func() {
		deployEv := newUnstructuredEventWithType("prod", "ev-deploy", "ScalingReplicaSet", "Scaled up", "Deployment", "web", "Normal")
		podEv := newUnstructuredEventWithType("prod", "ev-pod-bo", "BackOff", "Back-off restarting", "Pod", "web-abc123-xyz", "Warning")
		client := newFakeClient(deployEv, podEv)

		result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web", Description: "pod crash", APIVersion: "apps/v1",
		}, "user", nil, nil)
		Expect(err).NotTo(HaveOccurred())

		created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
			context.Background(), extractRRName(result.RRID), getOpts)
		Expect(getErr).NotTo(HaveOccurred())

		signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
		Expect(signalName).To(Equal("BackOff"),
			"Pod-level BackOff should be found when Deployment only has Normal lifecycle events")
	})

	It("UT-AF-1282-SIG-014: Pod cascade skipped when target Kind is already Pod", func() {
		podEv := newUnstructuredEventWithType("prod", "ev-pod", "Pulled", "image pulled", "Pod", "worker-1", "Normal")
		client := newFakeClient(podEv)

		result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
			Namespace: "prod", Kind: "Pod", Name: "worker-1", Description: "pod check", APIVersion: "apps/v1",
		}, "user", nil, nil)
		Expect(err).NotTo(HaveOccurred())

		created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
			context.Background(), extractRRName(result.RRID), getOpts)
		Expect(getErr).NotTo(HaveOccurred())

		signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
		Expect(signalName).To(Equal("unknown"),
			"Pod target with only lifecycle events should fall to unknown, not re-query Pod")
	})

	It("UT-AF-1282-SIG-015: Pod cascade filters unrelated pods by name prefix", func() {
		deployEv := newUnstructuredEventWithType("prod", "ev-deploy", "ScalingReplicaSet", "Scaled up", "Deployment", "web", "Normal")
		unrelatedPodEv := newUnstructuredEventWithType("prod", "ev-other", "OOMKilling", "killed", "Pod", "database-abc123", "Warning")
		client := newFakeClient(deployEv, unrelatedPodEv)

		result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web", Description: "check filter", APIVersion: "apps/v1",
		}, "user", nil, nil)
		Expect(err).NotTo(HaveOccurred())

		created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
			context.Background(), extractRRName(result.RRID), getOpts)
		Expect(getErr).NotTo(HaveOccurred())

		signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
		Expect(signalName).To(Equal("unknown"),
			"OOMKilling on unrelated pod 'database-abc123' should not match Deployment 'web'")
	})

	It("UT-AF-1282-NS-008: triage matches firing alert when signal source is in different namespace", func() {
		client := newFakeClient()
		noopLLM := severity.NewNoopLLMTriager(logr.Discard())
		cfg := severity.DefaultConfig()

		mockProm := &alertOverridePromClient{
			alerts: []prom.Alert{
				{State: "firing", Labels: map[string]string{
					"alertname": "HighCPU",
					"namespace": "production",
					"kind":      "Deployment",
					"name":      "web-server",
					"severity":  "critical",
				}},
			},
		}
		triager := severity.NewTriager(mockProm, noopLLM, cfg, logr.Discard())

		result, err := tools.HandleCreateRR(context.Background(), client, "kubernaut-system", &tools.CreateRRArgs{
			Namespace: "production", Kind: "Deployment", Name: "web-server", Description: "cross-ns triage", APIVersion: "apps/v1",
		}, "user", triager, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Severity).To(Equal("critical"),
			"triager should match alert by kind+name even when AF namespace differs from signal source namespace")
		Expect(result.SeveritySource).To(Equal("firing_alert"))
	})

	Describe("ADR-057 namespace split (F-NS-SPLIT)", func() {
		It("UT-AF-1292-NS-001: cross-namespace — CRD in controllerNS, targetResource in workloadNS (BR-PLATFORM-057)", func() {
			client := newFakeClient()
			controllerNS := "kubernaut-system"
			workloadNS := "production"

			result, err := tools.HandleCreateRR(context.Background(), client, controllerNS, &tools.CreateRRArgs{
				Namespace:   workloadNS,
				Kind:        "Deployment",
				Name:        "web",
				Description: "cross-namespace RR creation",
				APIVersion:  "apps/v1",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).To(HavePrefix("rr-"))

			created, getErr := client.Resource(rrGVR).Namespace(controllerNS).Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())

			metaNS := created.GetNamespace()
			Expect(metaNS).To(Equal(controllerNS), "CRD metadata.namespace must be controllerNS")

			targetNS, _, _ := unstructured.NestedString(created.Object, "spec", "targetResource", "namespace")
			Expect(targetNS).To(Equal(workloadNS),
				"spec.targetResource.namespace must be workloadNS, not controllerNS")
		})

		It("UT-AF-1292-NS-002: dedup fingerprint uses workload NS (BR-SAFETY-001)", func() {
			controllerNS := "kubernaut-system"
			workloadNS := "production"
			fp := testFingerprint(workloadNS, "Deployment", "web")
			existingRR := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kubernaut.ai/v1alpha1",
					"kind":       "RemediationRequest",
					"metadata": map[string]interface{}{
						"name":      "rr-deploy-web-existing",
						"namespace": controllerNS,
					},
					"spec": map[string]interface{}{
						"signalFingerprint": fp,
						"targetResource": map[string]interface{}{
							"kind":      "Deployment",
							"name":      "web",
							"namespace": workloadNS,
						},
					},
					"status": map[string]interface{}{
						"overallPhase": "Executing",
					},
				},
			}
			client := newFakeClient(existingRR)

			result, err := tools.HandleCreateRR(context.Background(), client, controllerNS, &tools.CreateRRArgs{
				Namespace:   workloadNS,
				Kind:        "Deployment",
				Name:        "web",
				Description: "should dedup on workload NS fingerprint",
				APIVersion:  "apps/v1",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.AlreadyExists).To(BeTrue(),
				"fingerprint(production/Deployment/web) should match the pre-seeded RR")
		})

		It("UT-AF-1292-NS-003: deriveSignalName queries events in workloadNS (BR-AI-056)", func() {
			controllerNS := "kubernaut-system"
			workloadNS := "production"
			ev := newUnstructuredEventWithType(workloadNS, "ev-oom", "OOMKilling", "killed", "Deployment", "web", "Warning")
			client := newFakeClient(ev)

			result, err := tools.HandleCreateRR(context.Background(), client, controllerNS, &tools.CreateRRArgs{
				Namespace:   workloadNS,
				Kind:        "Deployment",
				Name:        "web",
				Description: "events in workload NS",
				APIVersion:  "apps/v1",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())

			created, getErr := client.Resource(rrGVR).Namespace(controllerNS).Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())

			signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
			Expect(signalName).To(Equal("OOMKilling"),
				"deriveSignalName must query events in workloadNS, not controllerNS")
		})

		It("UT-AF-1292-NS-004: empty workload namespace rejected (BR-SAFETY-002)", func() {
			client := newFakeClient()

			_, err := tools.HandleCreateRR(context.Background(), client, "kubernaut-system", &tools.CreateRRArgs{
				Namespace:   "",
				Kind:        "Deployment",
				Name:        "web",
				Description: "empty workload NS",
				APIVersion:  "apps/v1",
			}, "user", nil, nil)
			Expect(err).To(MatchError(ContainSubstring("invalid input")),
				"empty workload namespace must be rejected")
		})

		It("UT-AF-1292-NS-005: triage labels use workloadNS for rule matching (BR-AI-056)", func() {
			controllerNS := "kubernaut-system"
			workloadNS := "production"
			client := newFakeClient()
			noopLLM := severity.NewNoopLLMTriager(logr.Discard())
			cfg := severity.DefaultConfig()

			mockProm := &alertOverridePromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{Name: "test-rules", Rules: []prom.Rule{
						{Name: "HighMemoryUsage", State: "inactive",
							Query:  `container_memory_usage_bytes{namespace="production"}`,
							Labels: map[string]string{"severity": "warning"}, Type: "alerting"},
					}},
				},
				queryResult: &prom.QueryResult{
					Samples: []prom.Sample{
						{Value: 85, Metric: map[string]string{"namespace": workloadNS}},
					},
				},
			}
			triager := severity.NewTriager(mockProm, noopLLM, cfg, logr.Discard())

			result, err := tools.HandleCreateRR(context.Background(), client, controllerNS, &tools.CreateRRArgs{
				Namespace:   workloadNS,
				Kind:        "Deployment",
				Name:        "web",
				Description: "triage with workload NS labels",
				APIVersion:  "apps/v1",
			}, "user", triager, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SeveritySource).To(Equal("rule_evaluation"),
				"Tier 2 must match because TriageInput.Labels['namespace'] = workloadNS matches the rule query")
			Expect(result.Severity).To(Equal("warning"),
				"severity from the matched rule, not LLM fallback medium")
		})
	})

	It("UT-AF-1282-K8S: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleCreateRR(context.Background(), nil, "prod", &tools.CreateRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web", Description: "x", APIVersion: "apps/v1",
		}, "user", nil, nil)
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-1282-DEDUP: returns existing RR when non-terminal match found", func() {
		rr := newUnstructuredRR("prod", "rr-deploy-web-existing", "Executing", "Deployment", "web")
		client := newFakeClient(rr)

		result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web", Description: "duplicate", APIVersion: "apps/v1",
		}, "sre-user", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.AlreadyExists).To(BeTrue())
		Expect(result.RRID).To(Equal("rr-deploy-web-existing"))
	})

	Describe("APIVersion and ClusterScoped (#1372)", func() {
		It("UT-AF-1372-060: RR created with targetResource.apiVersion populated", func() {
			client := newFakeClient()
			result, err := tools.HandleCreateRR(context.Background(), client, "kubernaut-system", &tools.CreateRRArgs{
				Namespace:  "prod",
				Kind:       "Deployment",
				Name:       "web",
				APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())

			rr, getErr := client.Resource(rrGVR).Namespace("kubernaut-system").Get(context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())
			target, found, _ := unstructured.NestedMap(rr.Object, "spec", "targetResource")
			Expect(found).To(BeTrue())
			Expect(target["apiVersion"]).To(Equal("apps/v1"))
		})

		It("UT-AF-1372-061: cluster-scoped RR (Node) with empty namespace creates successfully", func() {
			client := newFakeClient()
			result, err := tools.HandleCreateRR(context.Background(), client, "kubernaut-system", &tools.CreateRRArgs{
				Kind:          "Node",
				Name:          "worker-03",
				APIVersion:    "v1",
				ClusterScoped: true,
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-1372-062: namespaced RR with empty namespace rejects", func() {
			client := newFakeClient()
			_, err := tools.HandleCreateRR(context.Background(), client, "kubernaut-system", &tools.CreateRRArgs{
				Kind:          "Deployment",
				Name:          "web",
				APIVersion:    "apps/v1",
				ClusterScoped: false,
			}, "user", nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("namespace"))
		})
	})
})
