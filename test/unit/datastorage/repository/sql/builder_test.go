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

package sql

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	prodsql "github.com/jordigilh/kubernaut/pkg/datastorage/repository/sql"
)

func TestSQLBuilder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SQL Query Builder Suite")
}

var _ = Describe("SQL Query Builder", func() {
	Describe("Simple SELECT", func() {
		It("should build basic SELECT * FROM table", func() {
			query, args := prodsql.NewBuilder().
				Select("*").
				From("users").
				Build()

			Expect(query).To(Equal("SELECT * FROM users"))
			Expect(args).To(BeEmpty())
		})

		It("should build SELECT with specific columns", func() {
			query, args := prodsql.NewBuilder().
				Select("id, name, email").
				From("users").
				Build()

			Expect(query).To(Equal("SELECT id, name, email FROM users"))
			Expect(args).To(BeEmpty())
		})

		It("should default to SELECT * when no columns specified", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				Build()

			Expect(query).To(Equal("SELECT * FROM users"))
			Expect(args).To(BeEmpty())
		})
	})

	Describe("WHERE clauses", func() {
		It("should build single WHERE condition", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				Where("email = ?", "test@example.com").
				Build()

			Expect(query).To(Equal("SELECT * FROM users WHERE email = $1"))
			Expect(args).To(Equal([]interface{}{"test@example.com"}))
		})

		It("should build multiple WHERE conditions with AND", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				Where("status = ?", "active").
				Where("age > ?", 18).
				Build()

			Expect(query).To(Equal("SELECT * FROM users WHERE status = $1 AND age > $2"))
			Expect(args).To(Equal([]interface{}{"active", 18}))
		})

		It("should skip empty WHERE conditions", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				Where("", "").
				Where("status = ?", "active").
				Build()

			Expect(query).To(Equal("SELECT * FROM users WHERE status = $1"))
			Expect(args).To(Equal([]interface{}{"active"}))
		})

		It("should handle WhereRaw for custom conditions", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				WhereRaw("(status = $1 OR status = $2)", "active", "pending").
				Build()

			Expect(query).To(Equal("SELECT * FROM users WHERE (status = $1 OR status = $2)"))
			Expect(args).To(Equal([]interface{}{"active", "pending"}))
		})

		It("should handle multiple WHERE and WhereRaw together", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				Where("email = ?", "test@example.com").
				WhereRaw("(status = $2 OR status = $3)", "active", "pending").
				Build()

			Expect(query).To(Equal("SELECT * FROM users WHERE email = $1 AND (status = $2 OR status = $3)"))
			Expect(args).To(Equal([]interface{}{"test@example.com", "active", "pending"}))
		})
	})

	Describe("ORDER BY", func() {
		It("should build ORDER BY ASC", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				OrderBy("name", prodsql.ASC).
				Build()

			Expect(query).To(Equal("SELECT * FROM users ORDER BY name ASC"))
			Expect(args).To(BeEmpty())
		})

		It("should build ORDER BY DESC", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				OrderBy("created_at", prodsql.DESC).
				Build()

			Expect(query).To(Equal("SELECT * FROM users ORDER BY created_at DESC"))
			Expect(args).To(BeEmpty())
		})

		It("should build multiple ORDER BY clauses", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				OrderBy("status", prodsql.ASC).
				OrderBy("created_at", prodsql.DESC).
				Build()

			Expect(query).To(Equal("SELECT * FROM users ORDER BY status ASC, created_at DESC"))
			Expect(args).To(BeEmpty())
		})
	})

	Describe("LIMIT and OFFSET", func() {
		It("should build LIMIT", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				Limit(10).
				Build()

			Expect(query).To(Equal("SELECT * FROM users LIMIT $1"))
			Expect(args).To(Equal([]interface{}{10}))
		})

		It("should build OFFSET", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				Offset(20).
				Build()

			Expect(query).To(Equal("SELECT * FROM users OFFSET $1"))
			Expect(args).To(Equal([]interface{}{20}))
		})

		It("should build LIMIT and OFFSET together", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				Limit(10).
				Offset(20).
				Build()

			Expect(query).To(Equal("SELECT * FROM users LIMIT $1 OFFSET $2"))
			Expect(args).To(Equal([]interface{}{10, 20}))
		})

		It("should build with WHERE, ORDER BY, LIMIT, OFFSET", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				Where("status = ?", "active").
				OrderBy("created_at", prodsql.DESC).
				Limit(10).
				Offset(20).
				Build()

			Expect(query).To(Equal("SELECT * FROM users WHERE status = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3"))
			Expect(args).To(Equal([]interface{}{"active", 10, 20}))
		})
	})

	Describe("Complex queries", func() {
		It("should build complete query with all clauses", func() {
			query, args := prodsql.NewBuilder().
				Select("id, name, email").
				From("users").
				Where("status = ?", "active").
				Where("age > ?", 18).
				OrderBy("name", prodsql.ASC).
				OrderBy("created_at", prodsql.DESC).
				Limit(50).
				Offset(100).
				Build()

			Expect(query).To(ContainSubstring("SELECT id, name, email FROM users"))
			Expect(query).To(ContainSubstring("WHERE status = $1 AND age"))
			Expect(query).To(ContainSubstring("ORDER BY name ASC, created_at DESC"))
			Expect(query).To(ContainSubstring("LIMIT $"))
			Expect(query).To(ContainSubstring("OFFSET $"))
			Expect(args).To(HaveLen(4))
			Expect(args[0]).To(Equal("active"))
			Expect(args[1]).To(Equal(18))
			Expect(args[2]).To(Equal(50))
			Expect(args[3]).To(Equal(100))
		})

		It("should handle JSON operators in WHERE clause", func() {
			query, args := prodsql.NewBuilder().
				From("workflows").
				Where("labels->>'component' = ?", "api").
				Where("labels->>'severity' = ?", "critical").
				Build()

			Expect(query).To(Equal("SELECT * FROM workflows WHERE labels->>'component' = $1 AND labels->>'severity' = $2"))
			Expect(args).To(Equal([]interface{}{"api", "critical"}))
		})
	})

	Describe("BuildCount", func() {
		It("should build COUNT(*) query with same WHERE clauses", func() {
			builder := prodsql.NewBuilder().
				From("users").
				Where("status = ?", "active").
				Where("age > ?", 18)

			query, args := builder.BuildCount()

			Expect(query).To(Equal("SELECT COUNT(*) FROM users WHERE status = $1 AND age > $2"))
			Expect(args).To(Equal([]interface{}{"active", 18}))
		})

		It("should not include ORDER BY in count query", func() {
			builder := prodsql.NewBuilder().
				From("users").
				Where("status = ?", "active").
				OrderBy("created_at", prodsql.DESC)

			query, args := builder.BuildCount()

			Expect(query).To(Equal("SELECT COUNT(*) FROM users WHERE status = $1"))
			Expect(query).ToNot(ContainSubstring("ORDER BY"))
			Expect(args).To(Equal([]interface{}{"active"}))
		})

		It("should not include LIMIT/OFFSET in count query", func() {
			builder := prodsql.NewBuilder().
				From("users").
				Where("status = ?", "active").
				Limit(10).
				Offset(20)

			query, args := builder.BuildCount()

			Expect(query).To(Equal("SELECT COUNT(*) FROM users WHERE status = $1"))
			Expect(query).ToNot(ContainSubstring("LIMIT"))
			Expect(query).ToNot(ContainSubstring("OFFSET"))
			Expect(args).To(Equal([]interface{}{"active"}))
		})
	})

	Describe("Helper methods", func() {
		It("should track current arg index", func() {
			builder := prodsql.NewBuilder().
				Where("status = ?", "active").
				Where("age > ?", 18)

			// After 2 parameters, index should be at 3
			Expect(builder.CurrentArgIndex()).To(Equal(3))
		})

		It("should return current args", func() {
			builder := prodsql.NewBuilder().
				Where("status = ?", "active").
				Where("age > ?", 18)

			args := builder.Args()
			Expect(args).To(Equal([]interface{}{"active", 18}))
		})
	})

	Describe("Edge cases", func() {
		It("should handle query with no conditions", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				Build()

			Expect(query).To(Equal("SELECT * FROM users"))
			Expect(args).To(BeEmpty())
		})

		It("should handle query with only ORDER BY", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				OrderBy("created_at", prodsql.DESC).
				Build()

			Expect(query).To(Equal("SELECT * FROM users ORDER BY created_at DESC"))
			Expect(args).To(BeEmpty())
		})

		It("should handle query with only LIMIT", func() {
			query, args := prodsql.NewBuilder().
				From("users").
				Limit(10).
				Build()

			Expect(query).To(Equal("SELECT * FROM users LIMIT $1"))
			Expect(args).To(Equal([]interface{}{10}))
		})
	})
})
