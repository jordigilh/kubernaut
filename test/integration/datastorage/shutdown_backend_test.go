package datastorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/partition"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"
)

// Test-only ownership: no production Redis configuration or replay semantics change.
type shutdownBackend struct {
	name               string
	connStr            string
	redisAddr          string
	verifyDB           *sql.DB
	verifyRedis        *redis.Client
	databaseCreated    bool
	containerAttempted bool
	stopServer         func(context.Context) error
	cleanupOnce        sync.Once
	cleanupErr         error
}

func newShutdownBackend(ctx context.Context, redisImage string) (owned *shutdownBackend, retErr error) {
	backend := &shutdownBackend{name: "ds_shutdown_" + strings.ReplaceAll(uuid.NewString(), "-", "")}
	defer func() {
		if retErr != nil {
			owned = backend // Keep ownership identity available for failure-path assertions.
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			retErr = errors.Join(retErr, backend.cleanup(cleanupCtx))
		}
	}()
	if _, err := db.ExecContext(ctx, "CREATE DATABASE "+pgx.Identifier{backend.name}.Sanitize()); err != nil {
		return nil, fmt.Errorf("create database for %s: %w", backend.name, err)
	}
	backend.databaseCreated = true
	pgHost := os.Getenv("POSTGRES_HOST")
	if pgHost == "" {
		pgHost = localhost
	}
	pgPort := os.Getenv("POSTGRES_PORT")
	if pgPort == "" {
		pgPort = "15433"
	}
	backend.connStr = fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=%s sslmode=disable", pgHost, pgPort, backend.name)
	var err error
	backend.verifyDB, err = sql.Open("pgx", backend.connStr)
	if err != nil {
		return nil, fmt.Errorf("open owned database %s: %w", backend.name, err)
	}
	backend.verifyDB.SetMaxOpenConns(2)
	if err := infrastructure.RunGooseMigrations(ctx, backend.verifyDB, "../../../migrations", GinkgoWriter); err != nil {
		return nil, fmt.Errorf("migrate owned database %s: %w", backend.name, err)
	}
	if err := partition.EnsureMonthlyPartitions(ctx, backend.verifyDB, time.Now().UTC(), partition.DefaultLookaheadMonths, partition.AllTables()); err != nil {
		return nil, fmt.Errorf("partition owned database %s: %w", backend.name, err)
	}
	// Podman chooses the host port atomically; never reserve/release a guessed port.
	// Own this unique name before launch: an error/cancellation can arrive after
	// Podman creates or starts the container, before the CLI reports success.
	backend.containerAttempted = true
	output, err := exec.CommandContext(ctx, "podman", "run", "-d", "--pull=never", "--name", backend.name,
		"-p", "127.0.0.1::6379", redisImage).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("start owned redis %s: %w: %s", backend.name, err, strings.TrimSpace(string(output)))
	}
	output, err = exec.CommandContext(ctx, "podman", "port", backend.name, "6379/tcp").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("resolve owned redis port %s: %w: %s", backend.name, err, strings.TrimSpace(string(output)))
	}
	backend.redisAddr = strings.TrimSpace(string(output))
	if _, _, err := net.SplitHostPort(backend.redisAddr); err != nil {
		return nil, fmt.Errorf("invalid owned redis endpoint %s: %w", backend.name, err)
	}
	backend.verifyRedis = redis.NewClient(&redis.Options{Addr: backend.redisAddr, DialTimeout: time.Second})
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := backend.verifyRedis.Ping(ctx).Err(); err == nil {
			return backend, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for owned redis %s: %w", backend.name, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (b *shutdownBackend) cleanup(ctx context.Context) error {
	b.cleanupOnce.Do(func() {
		var errs []error
		if b.stopServer != nil {
			if err := b.stopServer(ctx); err != nil {
				errs = append(errs, fmt.Errorf("stop owned server %s: %w", b.name, err))
			}
		}
		if b.verifyRedis != nil {
			if err := b.verifyRedis.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close owned redis client %s: %w", b.name, err))
			}
		}
		if b.verifyDB != nil {
			if err := b.verifyDB.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close owned database client %s: %w", b.name, err))
			}
		}
		if b.containerAttempted {
			if output, err := exec.CommandContext(ctx, "podman", "rm", "--force", "--ignore", b.name).CombinedOutput(); err != nil {
				errs = append(errs, fmt.Errorf("remove owned redis %s: %w: %s", b.name, err, strings.TrimSpace(string(output))))
			}
		}
		if b.databaseCreated {
			if _, err := db.ExecContext(ctx, "DROP DATABASE "+pgx.Identifier{b.name}.Sanitize()+" WITH (FORCE)"); err != nil {
				errs = append(errs, fmt.Errorf("drop owned database %s: %w", b.name, err))
			}
		}
		b.cleanupErr = errors.Join(errs...)
		if b.cleanupErr != nil {
			logger.Error(b.cleanupErr, "shutdown fixture cleanup failed", "owner", b.name)
		}
	})
	return b.cleanupErr
}

var _ = Describe("BR-STORAGE-028 / BR-AUDIT-005: owned shutdown backends", func() {
	It("IT-DS-2499-002: overlapping real drains persist only their complete owned notification sets", func() {
		_, _, a, aBackend := createTestServerWithBackends()
		_, _, b, bBackend := createTestServerWithBackends()
		aIDs := enqueueOwnedNotifications(a, 3)
		bIDs := enqueueOwnedNotifications(b, 5)
		Expect(notificationIDs(aBackend)).To(BeEmpty())
		Expect(notificationIDs(bBackend)).To(BeEmpty())
		Expect(aBackend.name).ToNot(Equal(bBackend.name))
		Expect(aBackend.redisAddr).ToNot(Equal(bBackend.redisAddr))

		start := make(chan struct{})
		results := make(chan error, 2)
		drainCtx, drainCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer drainCancel()
		for _, fixture := range []struct {
			server  *server.Server
			backend *shutdownBackend
		}{{a, aBackend}, {b, bBackend}} {
			go func() {
				<-start
				_, err := fixture.server.GetDLQClient().DrainWithTimeout(drainCtx,
					repository.NewNotificationAuditRepository(fixture.backend.verifyDB, logger),
					repository.NewAuditEventsRepository(fixture.backend.verifyDB, logger))
				results <- err
			}()
		}
		close(start)
		for i := 0; i < 2; i++ {
			select {
			case err := <-results:
				Expect(err).ToNot(HaveOccurred())
			case <-drainCtx.Done():
				Fail("owned drains did not finish within their original budget")
			}
		}
		Expect(notificationIDs(aBackend)).To(ConsistOf(aIDs))
		Expect(notificationIDs(bBackend)).To(ConsistOf(bIDs))
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		Expect(a.Shutdown(shutdownCtx)).To(Succeed())
		Expect(b.Shutdown(shutdownCtx)).To(Succeed())
		for _, backend := range []*shutdownBackend{aBackend, bBackend} {
			depth, err := backend.verifyRedis.XLen(ctx, "audit:dlq:notifications").Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(depth).To(BeZero())
		}
	})

	It("IT-DS-2499-004: cleanup and failed provisioning remove only owned resources", func() {
		_, _, _, a := createTestServerWithBackends()
		_, _, _, b := createTestServerWithBackends()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cleanupCancel()
		Expect(a.cleanup(cleanupCtx)).To(Succeed())
		Expect(a.verifyDB.PingContext(cleanupCtx)).ToNot(Succeed())
		var exists bool
		Expect(db.QueryRowContext(cleanupCtx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", a.name).Scan(&exists)).To(Succeed())
		Expect(exists).To(BeFalse())
		Expect(exec.CommandContext(cleanupCtx, "podman", "container", "exists", a.name).Run()).ToNot(Succeed())

		failed, err := newShutdownBackend(cleanupCtx, "localhost/kubernaut/missing-shutdown-fixture:"+uuid.NewString())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("start owned redis"))
		Expect(db.QueryRowContext(cleanupCtx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", failed.name).Scan(&exists)).To(Succeed())
		Expect(exists).To(BeFalse(), "failed provisioning must not leak a migrated database")
		Expect(b.verifyDB.PingContext(cleanupCtx)).To(Succeed())
		Expect(b.verifyRedis.Ping(cleanupCtx).Err()).To(Succeed())
		Expect(db.PingContext(cleanupCtx)).To(Succeed(), "suite database must survive")
		Expect(redisClient.Ping(cleanupCtx).Err()).To(Succeed(), "suite redis must survive")
	})

	It("IT-DS-2499-004: a launcher failure after Redis starts still retires only the attempted owner", func() {
		_, _, _, survivor := createTestServerWithBackends()
		originalPodman, err := exec.LookPath("podman")
		Expect(err).NotTo(HaveOccurred())
		dir := GinkgoT().TempDir()
		// Fault injection is confined to the external launcher: Redis really starts,
		// but the CLI reports a failure before ownership receives a success result.
		script := fmt.Sprintf("#!/bin/bash\nif [[ \"$1\" == run ]]; then\n  %q \"$@\"\n  result=$?\n  (( result == 0 )) && exit 17\n  exit \"$result\"\nfi\nexec %q \"$@\"\n", originalPodman, originalPodman)
		Expect(os.WriteFile(filepath.Join(dir, "podman"), []byte(script), 0755)).To(Succeed())
		GinkgoT().Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		provisionCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		failed, err := newShutdownBackend(provisionCtx, "quay.io/jordigilh/redis:7-alpine")
		Expect(failed).NotTo(BeNil())
		// RED must not leak the real fault-injection container into later specs.
		DeferCleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			output, cleanupErr := exec.CommandContext(cleanupCtx, originalPodman, "rm", "--force", "--ignore", failed.name).CombinedOutput()
			Expect(cleanupErr).NotTo(HaveOccurred(), "owned fault fixture cleanup: %s", output)
		})
		Expect(err).To(MatchError(ContainSubstring("start owned redis")))
		Expect(exec.CommandContext(provisionCtx, originalPodman, "container", "exists", failed.name).Run()).NotTo(Succeed(), "the failed launcher must not orphan its owned container")
		var exists bool
		Expect(db.QueryRowContext(provisionCtx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", failed.name).Scan(&exists)).To(Succeed())
		Expect(exists).To(BeFalse())
		Expect(survivor.verifyRedis.Ping(provisionCtx).Err()).To(Succeed())
		Expect(survivor.verifyDB.PingContext(provisionCtx)).To(Succeed())
		Expect(redisClient.Ping(provisionCtx).Err()).To(Succeed())
		Expect(db.PingContext(provisionCtx)).To(Succeed())
	})
})

func enqueueOwnedNotifications(srv *server.Server, count int) []string {
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id := generateTestID()
		ids = append(ids, id)
		Expect(srv.GetDLQClient().EnqueueNotificationAudit(ctx, &models.NotificationAudit{
			RemediationID: generateTestID(), NotificationID: id,
			Recipient: "owned@example.com", Channel: "slack", Status: "sent",
			MessageSummary: "owned notification", SentAt: time.Now(),
		}, fmt.Errorf("test-owned external database failure"))).To(Succeed())
	}
	Eventually(func(g Gomega) {
		depth, err := srv.GetDLQClient().GetDLQDepth(ctx, "notifications")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(depth).To(Equal(int64(count)))
	}, 2*time.Second, 20*time.Millisecond).Should(Succeed())
	return ids
}

func notificationIDs(backend *shutdownBackend) []string {
	rows, err := backend.verifyDB.QueryContext(ctx, "SELECT notification_id FROM notification_audit")
	Expect(err).ToNot(HaveOccurred())
	defer func() { Expect(rows.Close()).To(Succeed()) }()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		Expect(rows.Scan(&id)).To(Succeed())
		ids = append(ids, id)
	}
	Expect(rows.Err()).ToNot(HaveOccurred())
	return ids
}
