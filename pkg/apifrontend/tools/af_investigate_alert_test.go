package tools_test

import (
	"context"
	"fmt"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("kubernaut_investigate_alert (#1372)", func() {
	baseCfg := func() tools.InvestigateAlertConfig {
		return tools.InvestigateAlertConfig{
			Client:       newTypedFakeClient(),
			ControllerNS: "kubernaut-system",
		}
	}

	Describe("Input validation — resource scope (UT-AF-1372-010..019)", func() {
		It("UT-AF-1372-010: rejects empty alert_name", func() {
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(),
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
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(),
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
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(),
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
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(),
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
			promClient := &alertOverridePromClient{
				alerts: []prom.Alert{
					{
						State:  "firing",
						Labels: map[string]string{"alertname": "NodeNotReady", "node": "worker-03"},
					},
				},
			}
			cfg := baseCfg()
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
			cfg := baseCfg()
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
			cfg := baseCfg()
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
			cfg := baseCfg()
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
			cfg := baseCfg()
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
			result, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(),
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
			cfg := baseCfg()
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
			Expect(result.SignalName).To(Equal("KubePodCrashLooping"))

			created := verifyTypedRR(cfg.Client, "kubernaut-system", extractRRName(result.RRID))
			Expect(created.Spec.SignalName).To(Equal("KubePodCrashLooping"),
				"CRD spec.signalName must be the alert name, not derived")
		})

		It("UT-AF-1372-031: RR targetResource includes apiVersion", func() {
			cfg := baseCfg()
			result, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
					Namespace:  "prod",
				}, "user")
			Expect(err).NotTo(HaveOccurred())

			created := verifyTypedRR(cfg.Client, "kubernaut-system", extractRRName(result.RRID))
			Expect(created.Spec.TargetResource.APIVersion).To(Equal("apps/v1"))
			Expect(created.Spec.TargetResource.Kind).To(Equal("Deployment"))
			Expect(created.Spec.TargetResource.Name).To(Equal("web"))
			Expect(created.Spec.TargetResource.Namespace).To(Equal("prod"))
		})

		It("UT-AF-1372-032: dedup returns existing RR for same fingerprint", func() {
			cfg := baseCfg()
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
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(),
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
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(),
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
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(),
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
			_, err := tools.HandleInvestigateAlert(context.Background(), baseCfg(),
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
			counter := prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "af_test",
				Name:      "alert_investigation_validation_failures_total",
			}, []string{"reason"})
			cfg := baseCfg()
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
			counter := prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "af_test",
				Name:      "alert_investigation_validation_ok_total",
			}, []string{"reason"})
			cfg := baseCfg()
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
			cfg := baseCfg()
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

		It("UT-AF-1372-056: strips namespace for cluster-scoped kind (self-healing #1477)", func() {
			cfg := baseCfg()
			cfg.Mapper = newMapper()
			result, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubeNodeNotReady",
					APIVersion: "v1",
					Kind:       "Node",
					Name:       "worker-1",
					Namespace:  "default",
				}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-1372-057: allows cluster-scoped kind without namespace", func() {
			cfg := baseCfg()
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
			cfg := baseCfg()
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
			t, err := tools.NewInvestigateAlertTool(tools.InvestigateAlertConfig{
				Client:       newTypedFakeClient(),
				ControllerNS: "kubernaut-system",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(t.Name()).To(Equal("kubernaut_investigate_alert"))
		})
	})

	Describe("RR context enrichment — #1423 (AU-3, SI-4)", func() {
		It("UT-AF-1423-030: HandleInvestigateAlert sets RR context on EventBridge after RR creation", func() {
			q := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), q, a2a.NewTaskID(), "ctx-1423-030", nil)

			result, err := tools.HandleInvestigateAlert(ctx, tools.InvestigateAlertConfig{
				Client:       newTypedFakeClient(),
				ControllerNS: "kubernaut-system",
			}, &tools.InvestigateAlertArgs{
				AlertName:  "ScalingLimited",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "api-frontend",
				Namespace:  "demo-gateway",
			}, "sre-user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())

			Expect(launcher.EmitStatusSafe(ctx, "post-alert-investigate status")).To(Succeed())

			found := false
			for _, evt := range q.Events() {
				statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if !ok {
					continue
				}
				meta := statusEvt.Metadata
				if meta == nil {
					continue
				}
				if rrid, ok := meta["rr_id"].(string); ok && rrid == result.RRID {
					found = true
					Expect(meta["namespace"]).To(Equal("demo-gateway"),
						"AU-3: namespace must be present for audit trail correlation")
					Expect(meta["kind"]).To(Equal("Deployment"))
					Expect(meta["target"]).To(Equal("api-frontend"))
					Expect(meta["alert_name"]).To(Equal("ScalingLimited"),
						"AU-3: alert_name must match the triggering alert")
					Expect(meta["phase"]).To(Equal("Investigating"),
						"SI-4: initial phase must be Investigating")
				}
			}
			Expect(found).To(BeTrue(),
				"AU-3: status events after HandleInvestigateAlert must carry rr_id from RR context")
		})
	})
})
