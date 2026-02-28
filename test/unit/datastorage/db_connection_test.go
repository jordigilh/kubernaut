/*
Copyright 2026 Jordi Gil.

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
	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// ========================================
// POSTGRESQL CONNECTION CONFIGURATION TESTS
// 📋 Bug: #200 - pgx prepared statement cache invalidated by schema migrations
//
// Root Cause: pgx defaults to QueryExecModeCacheStatement which caches
// prepared statements. When Helm upgrade migrations alter the schema while
// DataStorage is running, cached plans become invalid (SQLSTATE 0A000).
//
// Fix: Use QueryExecModeDescribeExec which describes each query (getting
// parameter OIDs for correct JSONB/complex type encoding) but does NOT
// cache the result, preventing stale plan errors.
//
// Note: QueryExecModeExec was tried first but fails to encode complex
// types like DetectedLabels (JSONB) because it skips the describe step
// and doesn't know the target PostgreSQL OID.
//
// 📋 TDD RED Phase: These tests MUST fail before the fix is applied.
// ========================================

var _ = Describe("PostgreSQL Connection Configuration (#200)", func() {

	Describe("NewPgxConnConfig", func() {

		Context("query execution mode", func() {
			It("UT-DS-200-001: should use QueryExecModeDescribeExec to prevent stale prepared statement caches", func() {
				connConfig, err := server.NewPgxConnConfig("host=localhost dbname=test user=test")
				Expect(err).NotTo(HaveOccurred())
				Expect(connConfig.DefaultQueryExecMode).To(Equal(pgx.QueryExecModeDescribeExec),
					"DefaultQueryExecMode must be QueryExecModeDescribeExec to prevent stale prepared "+
						"statement caches after schema migrations during Helm upgrades (#200). "+
						"DescribeExec gets parameter OIDs (needed for JSONB types) without caching.")
			})

			It("UT-DS-200-002: should NOT use QueryExecModeCacheStatement (the unsafe default)", func() {
				connConfig, err := server.NewPgxConnConfig("host=localhost dbname=test user=test")
				Expect(err).NotTo(HaveOccurred())
				Expect(connConfig.DefaultQueryExecMode).NotTo(Equal(pgx.QueryExecModeCacheStatement),
					"QueryExecModeCacheStatement causes 'cached plan must not change result type' "+
						"errors when schema changes occur while connections are open (#200)")
			})
		})

		Context("connection string parsing", func() {
			It("UT-DS-200-003: should parse a valid connection string", func() {
				connConfig, err := server.NewPgxConnConfig("host=localhost port=5432 dbname=kubernaut user=slm_user sslmode=disable")
				Expect(err).NotTo(HaveOccurred())
				Expect(connConfig.Host).To(Equal("localhost"))
				Expect(connConfig.Port).To(Equal(uint16(5432)))
				Expect(connConfig.Database).To(Equal("kubernaut"))
				Expect(connConfig.User).To(Equal("slm_user"))
			})

			It("UT-DS-200-004: should return error for invalid connection string", func() {
				_, err := server.NewPgxConnConfig("not://a valid connection string %%%")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse PostgreSQL connection string"))
			})
		})
	})
})
