package severity_test

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	prom "github.com/jordigilh/kubernaut-apifrontend/internal/prometheus"
	"github.com/jordigilh/kubernaut-apifrontend/internal/severity"
)

var _ = Describe("RulesCache", func() {

	It("UT-AF-T-039: second call within TTL returns cached response", func() {
		cache := severity.NewRulesCache(5)
		groups := []prom.RuleGroup{
			{Name: "g1", Rules: []prom.Rule{{Name: "r1"}}},
		}
		cache.Set(groups)

		cached := cache.Get()
		Expect(cached).To(HaveLen(1))
		Expect(cached[0].Name).To(Equal("g1"))
	})

	It("UT-AF-T-040: call after TTL expiry returns nil", func() {
		cache := severity.NewRulesCache(1)
		groups := []prom.RuleGroup{
			{Name: "g1", Rules: []prom.Rule{{Name: "r1"}}},
		}
		cache.Set(groups)

		time.Sleep(1100 * time.Millisecond)

		cached := cache.Get()
		Expect(cached).To(BeNil())
	})

	It("UT-AF-T-041: 10 goroutines read/write concurrently under -race", func() {
		cache := severity.NewRulesCache(5)
		groups := []prom.RuleGroup{
			{Name: "g1", Rules: []prom.Rule{{Name: "r1"}}},
		}

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(2)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				cache.Set(groups)
			}()
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				_ = cache.Get()
			}()
		}
		wg.Wait()
	})

	It("UT-AF-T-042: 50 set/get cycles do not grow unbounded", func() {
		cache := severity.NewRulesCache(5)
		for i := 0; i < 50; i++ {
			groups := []prom.RuleGroup{
				{Name: "g1", Rules: []prom.Rule{{Name: "r1"}}},
			}
			cache.Set(groups)
			_ = cache.Get()
		}
		Expect(cache.Len()).To(BeNumerically("<=", 1))
	})
})
