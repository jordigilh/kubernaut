package controller_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrl "sigs.k8s.io/controller-runtime"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/apifrontend"
)

var _ = Describe("LeaseSyncReconciler", func() {
	var (
		scheme *runtime.Scheme
		ns     = "kubernaut-system"
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
	})

	It("UT-AF-JOIN06-001: syncs lease holder to IS CRD status when Active", func() {
		sess := &v1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{Name: "test-sess", Namespace: ns},
			Spec: v1alpha1.InvestigationSessionSpec{
				RemediationRequestRef: v1alpha1.ObjectRef{Name: "rr1", Namespace: "default"},
				A2ATaskID:             "task-1",
				JoinMode:              v1alpha1.SessionJoinModeStart,
				UserIdentity:          v1alpha1.SessionUser{Username: "sre@kubernaut.ai"},
			},
			Status: v1alpha1.InvestigationSessionStatus{
				Phase: v1alpha1.SessionPhaseActive,
			},
		}

		acquireTime := metav1.NewMicroTime(metav1.Now().Time)
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubernaut-interactive-default-rr1",
				Namespace: ns,
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity: ptr.To("sre@kubernaut.ai"),
				AcquireTime:    &acquireTime,
			},
		}

		cli := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(sess, lease).
			WithStatusSubresource(sess).
			Build()

		r := controller.NewLeaseSyncReconciler(cli, ns, logr.Discard())
		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-sess", Namespace: ns},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))

		var updated v1alpha1.InvestigationSession
		Expect(cli.Get(context.Background(), types.NamespacedName{Name: "test-sess", Namespace: ns}, &updated)).To(Succeed())
		Expect(updated.Status.LeaseHolder).To(Equal("sre@kubernaut.ai"))
		Expect(updated.Status.LeaseAcquiredAt).NotTo(BeNil())
	})

	It("UT-AF-JOIN06-002: skips reconcile for non-Active sessions", func() {
		sess := &v1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{Name: "completed-sess", Namespace: ns},
			Spec: v1alpha1.InvestigationSessionSpec{
				RemediationRequestRef: v1alpha1.ObjectRef{Name: "rr2", Namespace: "default"},
				A2ATaskID:             "task-2",
				JoinMode:              v1alpha1.SessionJoinModeStart,
				UserIdentity:          v1alpha1.SessionUser{Username: "sre@kubernaut.ai"},
			},
			Status: v1alpha1.InvestigationSessionStatus{
				Phase: v1alpha1.SessionPhaseCompleted,
			},
		}

		cli := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(sess).
			WithStatusSubresource(sess).
			Build()

		r := controller.NewLeaseSyncReconciler(cli, ns, logr.Discard())
		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "completed-sess", Namespace: ns},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})

	It("UT-AF-JOIN06-003: no-op when lease does not exist", func() {
		sess := &v1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{Name: "no-lease-sess", Namespace: ns},
			Spec: v1alpha1.InvestigationSessionSpec{
				RemediationRequestRef: v1alpha1.ObjectRef{Name: "rr-missing", Namespace: "default"},
				A2ATaskID:             "task-3",
				JoinMode:              v1alpha1.SessionJoinModeStart,
				UserIdentity:          v1alpha1.SessionUser{Username: "sre@kubernaut.ai"},
			},
			Status: v1alpha1.InvestigationSessionStatus{
				Phase: v1alpha1.SessionPhaseActive,
			},
		}

		cli := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(sess).
			WithStatusSubresource(sess).
			Build()

		r := controller.NewLeaseSyncReconciler(cli, ns, logr.Discard())
		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "no-lease-sess", Namespace: ns},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})

	It("UT-AF-JOIN06-004: no-op when holder unchanged", func() {
		sess := &v1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{Name: "unchanged-sess", Namespace: ns},
			Spec: v1alpha1.InvestigationSessionSpec{
				RemediationRequestRef: v1alpha1.ObjectRef{Name: "rr4", Namespace: "default"},
				A2ATaskID:             "task-4",
				JoinMode:              v1alpha1.SessionJoinModeStart,
				UserIdentity:          v1alpha1.SessionUser{Username: "sre@kubernaut.ai"},
			},
			Status: v1alpha1.InvestigationSessionStatus{
				Phase:       v1alpha1.SessionPhaseActive,
				LeaseHolder: "sre@kubernaut.ai",
			},
		}

		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubernaut-interactive-default-rr4",
				Namespace: ns,
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity: ptr.To("sre@kubernaut.ai"),
			},
		}

		cli := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(sess, lease).
			WithStatusSubresource(sess).
			Build()

		r := controller.NewLeaseSyncReconciler(cli, ns, logr.Discard())
		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "unchanged-sess", Namespace: ns},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})
})
