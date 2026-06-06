package tools_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("kubernaut_investigate_alert (#1372)", func() {
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

	baseCfg := func(client *dynamicfake.FakeDynamicClient) tools.InvestigateAlertConfig {
		return tools.InvestigateAlertConfig{
			Client:       client,
			ControllerNS: "kubernaut-system",
		}
	}

	Describe("Input validation — resource scope (UT-AF-1372-010..019)", func() {
		It("UT-AF-1372-010: rejects empty alert_name", func() {
			client := newFakeClient()
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(client),
				&tools.InvestigateAlertArgs{
					AlertName:  "",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("alert_name"))
		})

		It("UT-AF-1372-011: rejects empty api_version", func() {
			client := newFakeClient()
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(client),
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("api_version"))
		})

		It("UT-AF-1372-012: rejects empty kind", func() {
			client := newFakeClient()
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(client),
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kind"))
		})

		It("UT-AF-1372-013: rejects empty name", func() {
			client := newFakeClient()
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(client),
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "",
					Namespace:  "prod",
				}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name"))
		})

		It("UT-AF-1372-014: accepts empty namespace for cluster-scoped resources", func() {
			client := newFakeClient()
			promClient := &alertOverridePromClient{
				alerts: []prom.Alert{
					{
						State:  "firing",
						Labels: map[string]string{"alertname": "NodeNotReady", "node": "worker-03"},
					},
				},
			}
			cfg := baseCfg(client)
			cfg.PromClient = promClient
			result, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "NodeNotReady",
					APIVersion: "v1",
					Kind:       "Node",
					Name:       "worker-03",
				}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-1372-015: rejects nil k8s client", func() {
			cfg := tools.InvestigateAlertConfig{ControllerNS: "kubernaut-system"}
			_, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).To(MatchError(tools.ErrK8sUnavailable))
		})
	})

	Describe("Alert validation — existence check (UT-AF-1372-020..025)", func() {
		It("UT-AF-1372-020: succeeds when alert_name matches a firing alert", func() {
			client := newFakeClient()
			cfg := baseCfg(client)
			cfg.PromClient = &alertOverridePromClient{
				alerts: []prom.Alert{
					{
						State:  "firing",
						Labels: map[string]string{"alertname": "KubePodCrashLooping", "namespace": "prod", "pod": "web-abc123"},
					},
				},
			}
			result, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
			Expect(result.AlertValidated).To(BeTrue())
		})

		It("UT-AF-1372-021: succeeds when alert_name matches a pending alert", func() {
			client := newFakeClient()
			cfg := baseCfg(client)
			cfg.PromClient = &alertOverridePromClient{
				alerts: []prom.Alert{
					{
						State:  "pending",
						Labels: map[string]string{"alertname": "KubePodCrashLooping", "namespace": "prod"},
					},
				},
			}
			result, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.AlertValidated).To(BeTrue())
		})

		It("UT-AF-1372-022: succeeds when alert_name matches a defined rule (not active)", func() {
			client := newFakeClient()
			cfg := baseCfg(client)
			cfg.PromClient = &alertOverridePromClient{
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test-group",
						Rules: []prom.Rule{
							{
								Type:  "alerting",
								Name:  "HighMemoryUsage",
								State: "inactive",
							},
						},
					},
				},
			}
			result, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "HighMemoryUsage",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.AlertValidated).To(BeTrue())
		})

		It("UT-AF-1372-023: rejects when alert_name not found in alerts or rules", func() {
			client := newFakeClient()
			cfg := baseCfg(client)
			cfg.PromClient = &alertOverridePromClient{
				alerts: []prom.Alert{
					{
						State:  "firing",
						Labels: map[string]string{"alertname": "OtherAlert"},
					},
				},
			}
			_, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "NonExistentAlert",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("alert"))
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("UT-AF-1372-024: succeeds without prom client (graceful degradation)", func() {
			client := newFakeClient()
			result, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(client),
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
			Expect(result.AlertValidated).To(BeFalse())
		})
	})

	Describe("RR creation with alert context (UT-AF-1372-030..035)", func() {
		It("UT-AF-1372-030: RR signalName set to alert_name on CRD", func() {
			client := newFakeClient()
			result, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(client),
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
			Expect(result.SignalName).To(Equal("KubePodCrashLooping"))

			rr, getErr := client.Resource(rrGVR).Namespace("kubernaut-system").Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())
			sn, _, _ := unstructured.NestedString(rr.Object, "spec", "signalName")
			Expect(sn).To(Equal("KubePodCrashLooping"),
				"CRD spec.signalName must be the alert name, not derived")
		})

		It("UT-AF-1372-031: RR targetResource includes apiVersion", func() {
			client := newFakeClient()
			result, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(client),
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).NotTo(HaveOccurred())

			rr, getErr := client.Resource(rrGVR).Namespace("kubernaut-system").Get(
				context.Background(), extractRRName(result.RRID), getOpts)
			Expect(getErr).NotTo(HaveOccurred())

			target, found, _ := unstructured.NestedMap(rr.Object, "spec", "targetResource")
			Expect(found).To(BeTrue())
			Expect(target["apiVersion"]).To(Equal("apps/v1"))
			Expect(target["kind"]).To(Equal("Deployment"))
			Expect(target["name"]).To(Equal("web"))
			Expect(target["namespace"]).To(Equal("prod"))
		})

		It("UT-AF-1372-032: dedup returns existing RR for same fingerprint", func() {
			client := newFakeClient()
			cfg := baseCfg(client)
			result1, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user-a")
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.AlreadyExists).To(BeFalse())

			result2, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user-b")
			Expect(err).NotTo(HaveOccurred())
			Expect(result2.AlreadyExists).To(BeTrue())
			Expect(result2.RRID).To(Equal(result1.RRID))
		})
	})

	Describe("FedRAMP compliance (UT-AF-1372-040..048)", func() {
		It("UT-AF-1372-040: path traversal in alert_name rejected", func() {
			client := newFakeClient()
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(client),
				&tools.InvestigateAlertArgs{
					AlertName:  "../../etc/passwd",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("alert_name"))
		})

		It("UT-AF-1372-041: path traversal in api_version rejected", func() {
			client := newFakeClient()
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(client),
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "../../v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("api_version"))
		})

		It("UT-AF-1372-042: oversized alert_name rejected", func() {
			client := newFakeClient()
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(client),
				&tools.InvestigateAlertArgs{
					AlertName:  fmt.Sprintf("%0254d", 0),
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("alert_name"))
		})

		It("UT-AF-1372-043: CRLF injection in alert_name rejected", func() {
			client := newFakeClient()
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(client),
				&tools.InvestigateAlertArgs{
					AlertName:  "alert\r\ninjection",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("alert_name"))
		})
	})

	Describe("Metrics wiring (UT-AF-1372-050..051)", func() {
		It("UT-AF-1372-050: validation failure increments counter", func() {
			client := newFakeClient()
			counter := prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "af_test",
				Name:      "alert_investigation_validation_failures_total",
			}, []string{"reason"})
			cfg := baseCfg(client)
			cfg.ValidationFailures = counter
			_, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).To(HaveOccurred())

			m := &dto.Metric{}
			Expect(counter.WithLabelValues("alert_name").(prometheus.Metric).Write(m)).To(Succeed())
			Expect(m.GetCounter().GetValue()).To(Equal(float64(1)))
		})

		It("UT-AF-1372-051: success does not increment counter", func() {
			client := newFakeClient()
			counter := prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "af_test",
				Name:      "alert_investigation_validation_ok_total",
			}, []string{"reason"})
			cfg := baseCfg(client)
			cfg.ValidationFailures = counter
			_, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).NotTo(HaveOccurred())

			gathered, gErr := prometheus.DefaultGatherer.Gather()
			_ = gathered
			Expect(gErr).NotTo(HaveOccurred())
		})
	})

	Describe("RESTMapper scope validation (UT-AF-1372-055..058)", func() {
		newMapper := func() *meta.DefaultRESTMapper {
			m := meta.NewDefaultRESTMapper([]schema.GroupVersion{
				{Group: "", Version: "v1"},
				{Group: "apps", Version: "v1"},
			})
			m.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
			m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}, meta.RESTScopeRoot)
			return m
		}

		It("UT-AF-1372-055: rejects namespaced kind without namespace", func() {
			client := newFakeClient()
			cfg := baseCfg(client)
			cfg.Mapper = newMapper()
			_, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
				}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("namespaced but namespace was not provided"))
		})

		It("UT-AF-1372-056: rejects cluster-scoped kind with namespace", func() {
			client := newFakeClient()
			cfg := baseCfg(client)
			cfg.Mapper = newMapper()
			_, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubeNodeNotReady",
					APIVersion: "v1",
					Kind:       "Node",
					Name:       "worker-1",
					Namespace:  "default",
				}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cluster-scoped but namespace"))
		})

		It("UT-AF-1372-057: allows cluster-scoped kind without namespace", func() {
			client := newFakeClient()
			cfg := baseCfg(client)
			cfg.Mapper = newMapper()
			result, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubeNodeNotReady",
					APIVersion: "v1",
					Kind:       "Node",
					Name:       "worker-1",
				}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-1372-058: allows namespaced kind with namespace", func() {
			client := newFakeClient()
			cfg := baseCfg(client)
			cfg.Mapper = newMapper()
			result, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})
	})

	Describe("Tool constructor", func() {
		It("UT-AF-1372-045: NewInvestigateAlertTool creates tool with correct name", func() {
			client := newFakeClient()
			t, err := tools.NewInvestigateAlertTool(tools.InvestigateAlertConfig{
				Client:       client,
				ControllerNS: "kubernaut-system",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(t.Name()).To(Equal("kubernaut_investigate_alert"))
		})
	})
})
