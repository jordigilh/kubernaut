package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"

	adksession "google.golang.org/adk/session"
	"k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func isITScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = isv1alpha1.AddToScheme(s)
	return s
}

func newISITClient(objs ...crclient.Object) crclient.Client {
	return fake.NewClientBuilder().
		WithScheme(isITScheme()).
		WithStatusSubresource(&isv1alpha1.InvestigationSession{}).
		WithIndex(&isv1alpha1.InvestigationSession{}, session.FieldIndexRRName,
			func(obj crclient.Object) []string {
				is := obj.(*isv1alpha1.InvestigationSession)
				if is.Spec.RemediationRequestRef.Name == "" {
					return nil
				}
				return []string{is.Spec.RemediationRequestRef.Name}
			}).
		WithObjects(objs...).
		Build()
}

// productionSignalerAdapter adapts CRDSessionService.CreateInvestigationSession to AlertISSignaler.
// This represents the production wiring that will live in agent/root.go.
type productionSignalerAdapter struct {
	svc       *session.CRDSessionService
	namespace string
}

func (a *productionSignalerAdapter) SignalInteractive(ctx context.Context, taskID, rrName, username string, groups []string) error {
	_, err := a.svc.CreateInvestigationSession(ctx, session.CreateISConfig{
		RRNamespace: a.namespace,
		RRName:      rrName,
		TaskID:      taskID,
		Username:    username,
		Groups:      groups,
		JoinMode:    isv1alpha1.SessionJoinModeStart,
	})
	return err
}

var _ = Describe("Fix #1440 Integration: IS CRD co-creation wiring", func() {

	Describe("IT-AF-1440-001: kubernaut_investigate_alert ADK tool creates IS CRD via production signaler wiring (SI-4, AU-12)", func() {
		It("should create IS CRD when HandleInvestigateAlert is called with production signaler", func() {
			isClient := newISITClient()
			svc := session.NewCRDSessionService(adksession.InMemoryService(), isClient, isITScheme(), "kubernaut-system")
			signaler := &productionSignalerAdapter{svc: svc, namespace: "kubernaut-system"}

			cfg := tools.InvestigateAlertConfig{
				Client:       newTypedFakeClient(),
				ControllerNS: "kubernaut-system",
				Signaler:     signaler,
			}

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre-alice",
				Groups:   []string{"sre-team"},
			})

			result, err := tools.HandleInvestigateAlert(ctx, cfg, &tools.InvestigateAlertArgs{
				AlertName:  "KubePodCrashLooping",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "web",
				Namespace:  "prod",
			}, "sre-alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())

			rrName := extractRRName(result.RRID)
			expectedISName := "is-" + rrName
			var is isv1alpha1.InvestigationSession
			Expect(isClient.Get(ctx, crclient.ObjectKey{
				Namespace: "kubernaut-system",
				Name:      expectedISName,
			}, &is)).To(Succeed(), "SI-4: IS CRD must be created by production signaler wiring")

			Expect(is.Spec.UserIdentity.Username).To(Equal("sre-alice"),
				"AC-3: IS CRD must carry user identity for RBAC")
			Expect(is.Spec.UserIdentity.Groups).To(Equal([]string{"sre-team"}))
			Expect(is.Spec.RemediationRequestRef.Name).To(Equal(rrName))
			Expect(is.Spec.JoinMode).To(Equal(isv1alpha1.SessionJoinModeStart))
		})
	})

	Describe("IT-AF-1440-002: IS CRD created by investigate_alert is findable by is_checker.HasActiveSession (SI-4)", func() {
		It("should be detectable by AA's K8sInvestigationSessionChecker", func() {
			isClient := newISITClient()
			svc := session.NewCRDSessionService(adksession.InMemoryService(), isClient, isITScheme(), "kubernaut-system")
			signaler := &productionSignalerAdapter{svc: svc, namespace: "kubernaut-system"}

			cfg := tools.InvestigateAlertConfig{
				Client:       newTypedFakeClient(),
				ControllerNS: "kubernaut-system",
				Signaler:     signaler,
			}

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre-bob",
				Groups:   []string{"sre-team", "oncall"},
			})

			result, err := tools.HandleInvestigateAlert(ctx, cfg, &tools.InvestigateAlertArgs{
				AlertName:  "HighMemoryUsage",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "api-server",
				Namespace:  "production",
			}, "sre-bob")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())

			rrName := extractRRName(result.RRID)

			checker := handlers.NewK8sInvestigationSessionChecker(isClient, "kubernaut-system")
			hasActive, checkErr := checker.HasActiveSession(ctx, rrName)
			Expect(checkErr).NotTo(HaveOccurred())
			Expect(hasActive).To(BeTrue(),
				"SI-4: IS CRD created by investigate_alert must be findable by AA's is_checker")
		})
	})
})
