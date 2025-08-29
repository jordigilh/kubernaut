//go:build integration
// +build integration

package infrastructure_test

import (
	"database/sql"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/prometheus-alerts-slm/test/integration/shared"
)

var _ = Describe("Database Only Infrastructure", Ordered, func() {
	var (
		config shared.DatabaseTestConfig
		db     *sql.DB
		logger *logrus.Logger
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		logger.Info("Testing database infrastructure only")

		config = shared.LoadDatabaseTestConfig()

		connectionString := "postgres://" + config.Username + ":" + config.Password +
			"@" + config.Host + ":" + config.Port + "/" + config.Database +
			"?sslmode=" + config.SSLMode

		var err error
		db, err = sql.Open("postgres", connectionString)
		Expect(err).ToNot(HaveOccurred())

		err = db.Ping()
		Expect(err).ToNot(HaveOccurred())

		logger.WithField("connection", connectionString).Info("Database connection successful")
	})

	AfterAll(func() {
		if db != nil {
			db.Close()
		}
	})

	It("should connect to PostgreSQL database", func() {
		Expect(db).ToNot(BeNil())

		err := db.Ping()
		Expect(err).ToNot(HaveOccurred())

		logger.Info("Database ping successful")
	})

	It("should have required tables from migrations", func() {
		rows, err := db.Query(`
			SELECT tablename FROM pg_tables
			WHERE schemaname = 'public'
			ORDER BY tablename
		`)
		Expect(err).ToNot(HaveOccurred())
		defer rows.Close()

		var tables []string
		for rows.Next() {
			var tableName string
			err := rows.Scan(&tableName)
			Expect(err).ToNot(HaveOccurred())
			tables = append(tables, tableName)
		}

		// Verify key tables exist
		Expect(tables).To(ContainElement("resource_references"))
		Expect(tables).To(ContainElement("action_histories"))
		Expect(tables).To(ContainElement("resource_action_traces"))
		Expect(tables).To(ContainElement("oscillation_patterns"))

		logger.WithField("tables", tables).Info("Database tables verified")
	})

	It("should have applied all migrations", func() {
		rows, err := db.Query(`
			SELECT version FROM schema_migrations
			ORDER BY version
		`)
		Expect(err).ToNot(HaveOccurred())
		defer rows.Close()

		var versions []string
		for rows.Next() {
			var version string
			err := rows.Scan(&version)
			Expect(err).ToNot(HaveOccurred())
			versions = append(versions, version)
		}

		// Should have migrations 001, 002, 003
		Expect(versions).To(ContainElement("001"))
		Expect(versions).To(ContainElement("002"))
		Expect(versions).To(ContainElement("003"))

		logger.WithField("migrations", versions).Info("Database migrations verified")
	})
})
