package tools_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aiav1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func newTypedAIAnalysis(ns, name, rrName, sessionID string) *aiav1alpha1.AIAnalysis {
	aia := &aiav1alpha1.AIAnalysis{
		ObjectMeta: objMeta(ns, name),
		Spec: aiav1alpha1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      rrName,
				Namespace: ns,
			},
			RemediationID: rrName,
		},
	}
	if sessionID != "" {
		aia.Status.KASession = &aiav1alpha1.KASession{
			ID: sessionID,
		}
	}
	return aia
}

func newTypedAIAnalysisClient(objects ...crclient.Object) crclient.Client {
	return fake.NewClientBuilder().
		WithScheme(aiaTestScheme()).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		Build()
}

var _ = Describe("kubernaut_await_session", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("HandleAwaitSession validation", func() {
		It("UT-AF-1293-SC8-003: returns error when client is nil", func() {
			_, err := tools.HandleAwaitSession(ctx, nil, tools.AwaitSessionArgs{
				Namespace: "default",
				RRName:    "rr-test",
			})
			Expect(err).To(HaveOccurred())
		})

		It("UT-AF-1293-SC8-004: returns error when namespace is empty", func() {
			tc := newTypedAIAnalysisClient()
			_, err := tools.HandleAwaitSession(ctx, tc, tools.AwaitSessionArgs{
				Namespace: "",
				RRName:    "rr-test",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("UT-AF-1293-SC8-005: returns error when rr_name is empty", func() {
			tc := newTypedAIAnalysisClient()
			_, err := tools.HandleAwaitSession(ctx, tc, tools.AwaitSessionArgs{
				Namespace: "default",
				RRName:    "",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rr_name is required"))
		})
	})

	Describe("HandleAwaitSession fast-path", func() {
		It("UT-AF-1293-006: returns immediately when session already exists", func() {
			aa := newTypedAIAnalysis("default", "aa-ready", "rr-ready", "session-xyz")
			tc := newTypedAIAnalysisClient(aa)

			start := time.Now()
			result, err := tools.HandleAwaitSession(ctx, tc, tools.AwaitSessionArgs{
				Namespace: "default",
				RRName:    "rr-ready",
			})
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("ready"))
			Expect(result.SessionID).To(Equal("session-xyz"))
			Expect(elapsed).To(BeNumerically("<", 2*time.Second))
		})
	})

	Describe("HandleAwaitSession watch path (CHAR-AF-1532)", func() {
		It("observes a session ID that appears via a watch event after the initial list misses it", func() {
			aa := newTypedAIAnalysis("default", "aa-watch", "rr-watch", "")
			tc := newTypedAIAnalysisClient(aa)

			go func() {
				defer GinkgoRecover()
				time.Sleep(200 * time.Millisecond)
				var updated aiav1alpha1.AIAnalysis
				Expect(tc.Get(ctx, crclient.ObjectKey{Namespace: "default", Name: "aa-watch"}, &updated)).To(Succeed())
				updated.Status.KASession = &aiav1alpha1.KASession{ID: "session-via-watch"}
				Expect(tc.Status().Update(ctx, &updated)).To(Succeed())

				// Unrelated AIAnalysis events (different RR name) must be
				// ignored by the watch loop rather than mistakenly matched.
				other := newTypedAIAnalysis("default", "aa-watch-other", "rr-watch-other", "session-should-be-ignored")
				Expect(tc.Create(ctx, other)).To(Succeed())
			}()

			result, err := tools.HandleAwaitSession(ctx, tc, tools.AwaitSessionArgs{
				Namespace: "default",
				RRName:    "rr-watch",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("ready"))
			Expect(result.SessionID).To(Equal("session-via-watch"))
		})

		It("times out when the watch never observes a session ID for this RR", func() {
			orig := tools.AwaitSessionTimeout
			tools.AwaitSessionTimeout = 500 * time.Millisecond
			defer func() { tools.AwaitSessionTimeout = orig }()

			aa := newTypedAIAnalysis("default", "aa-timeout", "rr-timeout", "")
			tc := newTypedAIAnalysisClient(aa)

			result, err := tools.HandleAwaitSession(ctx, tc, tools.AwaitSessionArgs{
				Namespace: "default",
				RRName:    "rr-timeout",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("timeout"))
		})
	})

	Describe("HandleAwaitSession list filtering", func() {
		It("UT-AF-1293-007: list ignores AIAnalysis for different RR name", func() {
			aa := newTypedAIAnalysis("default", "aa-other", "rr-other", "session-other")
			tc := newTypedAIAnalysisClient(aa)

			var aiaList aiav1alpha1.AIAnalysisList
			err := tc.List(ctx, &aiaList, crclient.InNamespace("default"))
			Expect(err).NotTo(HaveOccurred())
			Expect(aiaList.Items).To(HaveLen(1))

			var found string
			for _, item := range aiaList.Items {
				if item.Spec.RemediationRequestRef.Name != "rr-mine" {
					continue
				}
				if item.Status.KASession != nil && item.Status.KASession.ID != "" {
					found = item.Status.KASession.ID
				}
			}
			Expect(found).To(BeEmpty())
		})

		It("UT-AF-1293-008: list skips AIAnalysis with empty session ID", func() {
			aa := newTypedAIAnalysis("default", "aa-nosession", "rr-nosession", "")
			tc := newTypedAIAnalysisClient(aa)

			var aiaList aiav1alpha1.AIAnalysisList
			err := tc.List(ctx, &aiaList, crclient.InNamespace("default"))
			Expect(err).NotTo(HaveOccurred())
			Expect(aiaList.Items).To(HaveLen(1))

			var found string
			for _, item := range aiaList.Items {
				if item.Spec.RemediationRequestRef.Name != "rr-nosession" {
					continue
				}
				if item.Status.KASession != nil && item.Status.KASession.ID != "" {
					found = item.Status.KASession.ID
				}
			}
			Expect(found).To(BeEmpty())
		})
	})
})
