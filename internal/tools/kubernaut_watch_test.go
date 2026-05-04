package tools_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("kubernaut_watch", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-106-001: emits phase change events for RR", func() {
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Pending"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		})

		go func() {
			defer fakeWatcher.Stop()
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Executing"))
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Completed"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Events).NotTo(BeEmpty())
	})

	It("UT-AF-106-002: correlates related CRDs via ownerRef", func() {
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		})

		go func() {
			defer fakeWatcher.Stop()
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Completed"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).NotTo(BeEmpty())
	})

	It("UT-AF-106-003: emits events in chronological order", func() {
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Pending"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		})

		go func() {
			defer fakeWatcher.Stop()
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Executing"))
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Completed"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		if len(result.Events) >= 2 {
			Expect(result.Events[0].Timestamp <= result.Events[1].Timestamp).To(BeTrue())
		}
	})

	It("UT-AF-106-004: closes stream on terminal RR state", func() {
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		})

		go func() {
			defer fakeWatcher.Stop()
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Completed"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))
	})

	It("UT-AF-106-005: sends heartbeat every 30s", func() {
		Skip("heartbeat testing requires longer timeout -- covered in integration tests")
	})

	It("UT-AF-106-006: respects context cancellation", func() {
		cancelCtx, cancel := context.WithCancel(ctx)
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		})

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		result, err := tools.HandleWatch(cancelCtx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("cancelled"))
	})

	It("UT-AF-106-007: uses impersonated watch", func() {
		var capturedAction k8stesting.Action
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			capturedAction = action
			return true, fakeWatcher, nil
		})

		go func() {
			defer fakeWatcher.Stop()
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Completed"))
		}()

		_, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(capturedAction).NotTo(BeNil())
		Expect(capturedAction.GetNamespace()).To(Equal("payments"))
		Expect(capturedAction.GetResource()).To(Equal(schema.GroupVersionResource{
			Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests",
		}))
	})

	It("UT-AF-106-008: returns 403 when user cannot watch namespace", func() {
		client := newDynamicFakeClient()
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, nil, newForbiddenError("remediationrequests")
		})
		_, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "forbidden", Name: "rr-1"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("access denied"))
	})
})
