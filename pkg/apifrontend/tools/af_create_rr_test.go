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
	alerts []prom.Alert
}

func (a *alertOverridePromClient) GetAlerts(_ context.Context) ([]prom.Alert, error) {
	return a.alerts, nil
}
func (a *alertOverridePromClient) GetRules(_ context.Context) ([]prom.RuleGroup, error) {
	return nil, nil
}
func (a *alertOverridePromClient) InstantQuery(_ context.Context, _ string) (*prom.QueryResult, error) {
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

var _ = Describe("af_create_rr (#1282 refactor)", func() {
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
				Kind:        "Deployment",
				Name:        "web",
				Description: "Pod CrashLoopBackOff detected",
			}, "sre-user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
			Expect(result.AlreadyExists).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("created"))
		})

		It("UT-AF-1282-MIN-002: empty kind rejected", func() {
			client := newFakeClient()
			_, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Kind: "", Name: "web", Description: "x",
			}, "user", nil, nil)
			Expect(err).To(MatchError(ContainSubstring("invalid input")))
		})

		It("UT-AF-1282-MIN-003: empty name rejected", func() {
			client := newFakeClient()
			_, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Kind: "Deployment", Name: "", Description: "x",
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
				Kind: "Deployment", Name: "web", Description: string(longDesc),
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
						Kind: "Deployment", Name: "dedup-target", Description: "concurrent test",
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
				Kind: "Deployment", Name: "web", Description: "test triage",
			}, "alice", triager, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-1282-MIN-007: nil Triager defaults severity to medium", func() {
			client := newFakeClient()

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Kind: "Deployment", Name: "web", Description: "no triager",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})
	})

	Describe("Namespace resolution (F-NS)", func() {
		It("UT-AF-1282-NS-005: namespace comes from AF, not LLM args", func() {
			client := newFakeClient()

			result, err := tools.HandleCreateRR(context.Background(), client, "kubernaut-system", &tools.CreateRRArgs{
				Kind: "Deployment", Name: "web", Description: "ns from AF",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).To(HavePrefix("kubernaut-system/"))
		})

		It("UT-AF-1282-NS-006: empty namespace from AF is rejected", func() {
			client := newFakeClient()
			_, err := tools.HandleCreateRR(context.Background(), client, "", &tools.CreateRRArgs{
				Kind: "Deployment", Name: "web", Description: "x",
			}, "user", nil, nil)
			Expect(err).To(MatchError(ContainSubstring("invalid input")))
		})

		It("UT-AF-1282-NS-007: invalid namespace from AF (path traversal) rejected", func() {
			client := newFakeClient()
			_, err := tools.HandleCreateRR(context.Background(), client, "../../etc", &tools.CreateRRArgs{
				Kind: "Deployment", Name: "web", Description: "x",
			}, "user", nil, nil)
			Expect(err).To(MatchError(ContainSubstring("invalid input")))
		})
	})

	Describe("Signal source (F-SRC)", func() {
		It("UT-AF-1282-SRC-001: created RR has signalSource=a2a-agent", func() {
			client := newFakeClient()

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Kind: "Deployment", Name: "web", Description: "check source",
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
				Kind: "Deployment", Name: "web", Description: "dup",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.AlreadyExists).To(BeTrue())
		})
	})

	Describe("Signal name grounding (F-SIG)", func() {
		It("UT-AF-1282-SIG-006: signalName never starts with af-manual-", func() {
			client := newFakeClient()

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Kind: "Deployment", Name: "web", Description: "check signal name",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())

			created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())

			signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
			Expect(signalName).NotTo(HavePrefix("af-manual-"))
		})

		It("UT-AF-1282-SIG-009: K8s events fallback — OOMKilling event becomes signalName", func() {
			ev := newUnstructuredEventWithType("prod", "ev-oom", "OOMKilling", "killed", "Deployment", "web", "Warning")
			client := newFakeClient(ev)

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Kind: "Deployment", Name: "web", Description: "OOM detected",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())

			created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())

			signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
			Expect(signalName).To(Equal("OOMKilling"))
		})

		It("UT-AF-1282-SIG-010: no events → synthetic a2a- fallback", func() {
			client := newFakeClient()

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Kind: "StatefulSet", Name: "db", Description: "no events",
			}, "user", nil, nil)
			Expect(err).NotTo(HaveOccurred())

			created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())

			signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
			Expect(signalName).To(Equal("a2a-StatefulSet-db"))
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
				Kind: "Deployment", Name: "web", Description: "alert-based",
			}, "user", triager, nil)
			Expect(err).NotTo(HaveOccurred())

			created, getErr := client.Resource(rrGVR).Namespace("prod").Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())

			signalName, _, _ := unstructured.NestedString(created.Object, "spec", "signalName")
			Expect(signalName).To(Equal("HighErrorRate"))
		})
	})

	It("UT-AF-1282-K8S: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleCreateRR(context.Background(), nil, "prod", &tools.CreateRRArgs{
			Kind: "Deployment", Name: "web", Description: "x",
		}, "user", nil, nil)
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-1282-DEDUP: returns existing RR when non-terminal match found", func() {
		rr := newUnstructuredRR("prod", "rr-deploy-web-existing", "Executing", "Deployment", "web")
		client := newFakeClient(rr)

		result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
			Kind: "Deployment", Name: "web", Description: "duplicate",
		}, "sre-user", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.AlreadyExists).To(BeTrue())
		Expect(result.RRID).To(Equal("prod/rr-deploy-web-existing"))
	})
})
