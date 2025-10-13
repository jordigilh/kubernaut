/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package datastorage

import (
	"context"
	"fmt"

	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/database/schema"
)

var _ = Describe("BR-STORAGE-008: Idempotent Schema Initialization", func() {
	var (
		testInitializer *schema.Initializer
		testSchema      string
		testCtx         context.Context
	)

	BeforeEach(func() {
		testCtx = context.Background()

		// Create unique test schema (not database)
		testSchema = fmt.Sprintf("test_schema_%d", GinkgoRandomSeed())
		_, err := db.Exec(fmt.Sprintf("CREATE SCHEMA %s", testSchema))
		Expect(err).ToNot(HaveOccurred())

		// Set search path to test schema
		_, err = db.Exec(fmt.Sprintf("SET search_path TO %s", testSchema))
		Expect(err).ToNot(HaveOccurred())

		// Create initializer for test schema
		testInitializer = schema.NewInitializer(db, logger)

		// Initialize schema for first-time tests
		err = testInitializer.Initialize(testCtx)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Reset search path
		_, _ = db.Exec("SET search_path TO public")

		// Drop test schema
		if testSchema != "" {
			_, _ = db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", testSchema))
		}
	})
	Context("when initializing schema for the first time", func() {
		It("should create all required tables successfully", func() {
			By("verifying all tables exist")
			err := testInitializer.Verify(testCtx)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should enable pgvector extension", func() {
			ctx := context.Background()

			By("checking if pgvector extension is enabled")
			var extensionExists bool
			err := db.QueryRowContext(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM pg_extension WHERE extname = 'vector'
				)
			`).Scan(&extensionExists)
			Expect(err).ToNot(HaveOccurred())
			Expect(extensionExists).To(BeTrue(), "pgvector extension should be enabled")
		})

		It("should create remediation_audit table with all columns", func() {
			ctx := context.Background()

			By("querying remediation_audit table structure")
			rows, err := db.QueryContext(ctx, `
				SELECT column_name, data_type
				FROM information_schema.columns
				WHERE table_name = 'remediation_audit'
				ORDER BY ordinal_position
			`)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = rows.Close() }()

			expectedColumns := []string{
				"id", "name", "namespace", "phase", "action_type", "status",
				"start_time", "end_time", "duration", "remediation_request_id",
				"alert_fingerprint", "severity", "environment", "cluster_name",
				"target_resource", "error_message", "metadata", "embedding",
				"created_at", "updated_at",
			}

			var foundColumns []string
			for rows.Next() {
				var colName, dataType string
				err := rows.Scan(&colName, &dataType)
				Expect(err).ToNot(HaveOccurred())
				foundColumns = append(foundColumns, colName)
			}

			Expect(foundColumns).To(ContainElements(expectedColumns))
		})

		It("should create ai_analysis_audit table", func() {
			ctx := context.Background()

			By("verifying ai_analysis_audit table exists")
			var exists bool
			err := db.QueryRowContext(ctx, `
				SELECT EXISTS (
					SELECT FROM information_schema.tables
					WHERE table_name = 'ai_analysis_audit'
				)
			`).Scan(&exists)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("should create workflow_audit table", func() {
			ctx := context.Background()

			By("verifying workflow_audit table exists")
			var exists bool
			err := db.QueryRowContext(ctx, `
				SELECT EXISTS (
					SELECT FROM information_schema.tables
					WHERE table_name = 'workflow_audit'
				)
			`).Scan(&exists)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("should create execution_audit table", func() {
			ctx := context.Background()

			By("verifying execution_audit table exists")
			var exists bool
			err := db.QueryRowContext(ctx, `
				SELECT EXISTS (
					SELECT FROM information_schema.tables
					WHERE table_name = 'execution_audit'
				)
			`).Scan(&exists)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})
	})

	Context("when initializing schema a second time (idempotency)", func() {
		It("should succeed without errors", func() {
			By("initializing schema second time (idempotent)")
			err := testInitializer.Initialize(testCtx)
			Expect(err).ToNot(HaveOccurred(), "second initialization should succeed")

			By("verifying tables still exist and are functional")
			err = testInitializer.Verify(testCtx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("when verifying schema", func() {
		It("should confirm all required indexes exist", func() {
			By("checking for namespace index")
			var indexExists bool
			err := db.QueryRowContext(testCtx, `
				SELECT EXISTS (
					SELECT 1 FROM pg_indexes
					WHERE tablename = 'remediation_audit'
					AND indexname LIKE '%namespace%'
				)
			`).Scan(&indexExists)
			Expect(err).ToNot(HaveOccurred())
			Expect(indexExists).To(BeTrue(), "namespace index should exist")
		})
	})
})
