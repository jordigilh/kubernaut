package auth_test

import (
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// BR-SECURITY-1505 (GAP-08, kubernaut#1505): distributed jti replay cache
// closes the HA gap of the in-memory ReplayCache — replay state must be
// shared across all APIFrontend replicas via Valkey/Redis.
var _ = Describe("ValkeyReplayCache", func() {
	var (
		mr     *miniredis.Miniredis
		client *redis.Client
		rc     *auth.ValkeyReplayCache
	)

	BeforeEach(func() {
		var err error
		mr, err = miniredis.Run()
		Expect(err).NotTo(HaveOccurred())
		client = redis.NewClient(&redis.Options{Addr: mr.Addr()})
		rc = auth.NewValkeyReplayCache(client, 1*time.Minute, logr.Discard())
	})

	AfterEach(func() {
		rc.Stop()
		_ = client.Close()
		mr.Close()
	})

	It("returns false (not seen) for a new jti", func() {
		Expect(rc.Seen("jti-abc-123")).To(BeFalse())
	})

	It("returns true (replay detected) when the same jti is seen twice", func() {
		Expect(rc.Seen("jti-abc-123")).To(BeFalse())
		Expect(rc.Seen("jti-abc-123")).To(BeTrue())
	})

	It("shares replay state across two independent client instances (simulating two replicas)", func() {
		// A second APIFrontend replica would construct its own redis.Client
		// pointed at the same Valkey instance; simulate that here.
		client2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer func() { _ = client2.Close() }()
		rc2 := auth.NewValkeyReplayCache(client2, 1*time.Minute, logr.Discard())
		defer rc2.Stop()

		Expect(rc.Seen("jti-shared")).To(BeFalse(), "replica 1 observes the token first")
		Expect(rc2.Seen("jti-shared")).To(BeTrue(), "replica 2 must detect the replay via the shared store")
	})

	It("always reports empty jti as not-seen without touching the store", func() {
		Expect(rc.Seen("")).To(BeFalse())
		Expect(rc.Seen("")).To(BeFalse())
	})

	It("reports MissingJTI true only for empty jti", func() {
		Expect(rc.MissingJTI("")).To(BeTrue())
		Expect(rc.MissingJTI("abc-123")).To(BeFalse())
	})

	It("expires entries after the configured TTL", func() {
		shortTTL := auth.NewValkeyReplayCache(client, 50*time.Millisecond, logr.Discard())
		defer shortTTL.Stop()

		Expect(shortTTL.Seen("jti-expiring")).To(BeFalse())
		mr.FastForward(100 * time.Millisecond)
		Expect(shortTTL.Seen("jti-expiring")).To(BeFalse(), "entry should have expired and be treated as new")
	})

	It("fails open (reports not-seen) when Valkey is unreachable, rather than blocking the request", func() {
		mr.Close() // simulate an outage
		Expect(rc.Seen("jti-during-outage")).To(BeFalse())
	})
})
