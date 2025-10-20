package contextapi

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/client"
)

var _ = Describe("PostgresClient", func() {
	var (
		logger *zap.Logger
		ctx    context.Context
	)

	BeforeEach(func() {
		var err error
		logger, err = zap.NewDevelopment()
		Expect(err).ToNot(HaveOccurred())
		ctx = context.Background()
	})

	AfterEach(func() {
		if logger != nil {
			logger.Sync()
		}
	})

	Describe("NewPostgresClient", func() {
		Context("with valid connection string", func() {
			It("should create a new PostgreSQL client", func() {
				// BR-CONTEXT-001: Historical Context Query - requires database connection
				connStr := "host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable"
				pgClient, err := client.NewPostgresClient(connStr, logger)
				Expect(err).ToNot(HaveOccurred())
				Expect(pgClient).ToNot(BeNil())

				// Cleanup
				err = pgClient.Close()
				Expect(err).ToNot(HaveOccurred())
			})

			It("should configure connection pool settings", func() {
				// BR-CONTEXT-011: Schema Alignment - connection pool must be configured
				connStr := "host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable"
				pgClient, err := client.NewPostgresClient(connStr, logger)
				Expect(err).ToNot(HaveOccurred())
				Expect(pgClient).ToNot(BeNil())

				// Verify connection pool is working by testing health check
				err = pgClient.HealthCheck(ctx)
				Expect(err).ToNot(HaveOccurred())

				// Cleanup
				err = pgClient.Close()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with invalid connection string", func() {
			It("should return an error for invalid host", func() {
				// BR-CONTEXT-008: REST API - must handle connection errors gracefully
				connStr := "host=invalid-host port=5432 user=postgres password=postgres dbname=postgres sslmode=disable connect_timeout=1"
				pgClient, err := client.NewPostgresClient(connStr, logger)
				Expect(err).To(HaveOccurred())
				Expect(pgClient).To(BeNil())
			})

			It("should return an error for invalid port", func() {
				// BR-CONTEXT-008: REST API - must handle connection errors gracefully
				connStr := "host=localhost port=9999 user=postgres password=postgres dbname=postgres sslmode=disable connect_timeout=1"
				pgClient, err := client.NewPostgresClient(connStr, logger)
				Expect(err).To(HaveOccurred())
				Expect(pgClient).To(BeNil())
			})

			It("should return an error for invalid credentials", func() {
				// BR-CONTEXT-008: REST API - must handle authentication errors
				connStr := "host=localhost port=5432 user=invalid password=invalid dbname=postgres sslmode=disable connect_timeout=1"
				pgClient, err := client.NewPostgresClient(connStr, logger)
				Expect(err).To(HaveOccurred())
				Expect(pgClient).To(BeNil())
			})
		})
	})

	Describe("HealthCheck", func() {
		var pgClient *client.PostgresClient

		BeforeEach(func() {
			connStr := "host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable"
			var err error
			pgClient, err = client.NewPostgresClient(connStr, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if pgClient != nil {
				pgClient.Close()
			}
		})

		Context("with active connection", func() {
			It("should return no error", func() {
				// BR-CONTEXT-012: Multi-Client Support - health check must validate connectivity
				err := pgClient.HealthCheck(ctx)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle context timeout", func() {
				// BR-CONTEXT-008: REST API - must handle timeouts gracefully
				timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
				defer cancel()

				time.Sleep(10 * time.Millisecond) // Ensure timeout occurs

				err := pgClient.HealthCheck(timeoutCtx)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Close", func() {
		It("should close the database connection", func() {
			// BR-CONTEXT-011: Schema Alignment - connections must be properly closed
			connStr := "host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable"
			pgClient, err := client.NewPostgresClient(connStr, logger)
			Expect(err).ToNot(HaveOccurred())

			err = pgClient.Close()
			Expect(err).ToNot(HaveOccurred())

			// Attempting health check after close should fail
			err = pgClient.HealthCheck(ctx)
			Expect(err).To(HaveOccurred())
		})
	})
})
