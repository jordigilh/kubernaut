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
		Expect(rc.Seen("jti-abc-123", "source-a")).To(BeFalse())
	})

	// #1999 (BR-SECURITY-1505): replay detection is bound to the source that
	// first presented the jti — same source reusing its own token is
	// legitimate; a different source presenting the same jti is a replay.
	It("returns false (not a replay) when the same jti is reused from the same source", func() {
		Expect(rc.Seen("jti-abc-123", "source-a")).To(BeFalse())
		Expect(rc.Seen("jti-abc-123", "source-a")).To(BeFalse())
	})

	It("returns true (replay detected) when the same jti is presented from a different source", func() {
		Expect(rc.Seen("jti-abc-123", "source-a")).To(BeFalse())
		Expect(rc.Seen("jti-abc-123", "source-b")).To(BeTrue())
	})

	It("shares replay state across two independent client instances (simulating two replicas): same source reused via a different replica is still legitimate", func() {
		// A second APIFrontend replica would construct its own redis.Client
		// pointed at the same Valkey instance; simulate that here. A
		// load-balanced client hitting replica 2 with its own token is the
		// exact scenario GAP-08 must NOT reject.
		client2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer func() { _ = client2.Close() }()
		rc2 := auth.NewValkeyReplayCache(client2, 1*time.Minute, logr.Discard())
		defer rc2.Stop()

		Expect(rc.Seen("jti-shared", "client-x")).To(BeFalse(), "replica 1 observes the token first")
		Expect(rc2.Seen("jti-shared", "client-x")).To(BeFalse(), "replica 2 must see the shared state and recognize the same source as legitimate reuse, not a replay")
	})

	It("shares replay state across two independent client instances (simulating two replicas): a different source via a different replica is a replay", func() {
		client2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer func() { _ = client2.Close() }()
		rc2 := auth.NewValkeyReplayCache(client2, 1*time.Minute, logr.Discard())
		defer rc2.Stop()

		Expect(rc.Seen("jti-shared-attack", "client-x")).To(BeFalse(), "replica 1 observes the legitimate client first")
		Expect(rc2.Seen("jti-shared-attack", "attacker-y")).To(BeTrue(), "replica 2 must detect the cross-source replay via the shared store")
	})

	It("always reports empty jti as not-seen without touching the store", func() {
		Expect(rc.Seen("", "source-a")).To(BeFalse())
		Expect(rc.Seen("", "source-a")).To(BeFalse())
	})

	It("reports MissingJTI true only for empty jti", func() {
		Expect(rc.MissingJTI("")).To(BeTrue())
		Expect(rc.MissingJTI("abc-123")).To(BeFalse())
	})

	It("expires entries after the configured TTL", func() {
		shortTTL := auth.NewValkeyReplayCache(client, 50*time.Millisecond, logr.Discard())
		defer shortTTL.Stop()

		Expect(shortTTL.Seen("jti-expiring", "source-a")).To(BeFalse())
		mr.FastForward(100 * time.Millisecond)
		Expect(shortTTL.Seen("jti-expiring", "source-a")).To(BeFalse(), "entry should have expired and be treated as new")
	})

	It("fails open (reports not-seen) when Valkey is unreachable, rather than blocking the request", func() {
		mr.Close() // simulate an outage
		Expect(rc.Seen("jti-during-outage", "source-a")).To(BeFalse())
	})
})
