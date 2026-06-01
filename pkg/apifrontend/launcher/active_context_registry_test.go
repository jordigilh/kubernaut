package launcher_test

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("ActiveContextRegistry (BR-SESS-020, BR-SESS-022, BR-SESS-024)", func() {
	var registry *launcher.ActiveContextRegistry

	BeforeEach(func() {
		registry = launcher.NewActiveContextRegistry(2 * time.Hour)
	})

	It("UT-AF-SESS-020-001: Set stores username-to-contextID mapping (SC-7)", func() {
		registry.Set("alice", "ctx-abc-123")

		contextID, ok := registry.Get("alice")
		Expect(ok).To(BeTrue())
		Expect(contextID).To(Equal("ctx-abc-123"))
	})

	It("UT-AF-SESS-020-002: Get returns stored contextID for known user (SC-7)", func() {
		registry.Set("bob", "ctx-def-456")

		contextID, ok := registry.Get("bob")
		Expect(ok).To(BeTrue())
		Expect(contextID).To(Equal("ctx-def-456"))
	})

	It("UT-AF-SESS-020-003: Get returns false for unknown user (SC-7)", func() {
		contextID, ok := registry.Get("unknown-user")
		Expect(ok).To(BeFalse())
		Expect(contextID).To(BeEmpty())
	})

	It("UT-AF-SESS-020-004: Clear removes entry; subsequent Get returns false (AC-2)", func() {
		registry.Set("carol", "ctx-ghi-789")
		registry.Clear("carol")

		contextID, ok := registry.Get("carol")
		Expect(ok).To(BeFalse())
		Expect(contextID).To(BeEmpty())
	})

	It("UT-AF-SESS-020-005: Get returns false for expired entry (AC-2)", func() {
		shortTTL := launcher.NewActiveContextRegistry(1 * time.Millisecond)
		shortTTL.Set("dave", "ctx-expired")

		time.Sleep(5 * time.Millisecond)

		contextID, ok := shortTTL.Get("dave")
		Expect(ok).To(BeFalse())
		Expect(contextID).To(BeEmpty())
	})

	It("UT-AF-SESS-020-006: Set overwrites previous entry for same user (SC-7)", func() {
		registry.Set("eve", "ctx-old")
		registry.Set("eve", "ctx-new")

		contextID, ok := registry.Get("eve")
		Expect(ok).To(BeTrue())
		Expect(contextID).To(Equal("ctx-new"))
	})

	It("UT-AF-SESS-020-007: Concurrent Set/Get/Clear is race-free (SC-7)", func() {
		const goroutines = 50
		var wg sync.WaitGroup
		wg.Add(goroutines * 3)

		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				registry.Set("concurrent-user", "ctx-concurrent")
			}()
			go func() {
				defer wg.Done()
				registry.Get("concurrent-user")
			}()
			go func() {
				defer wg.Done()
				registry.Clear("concurrent-user")
			}()
		}

		wg.Wait()
		// No panic or data race = pass (run with -race flag)
	})
})
