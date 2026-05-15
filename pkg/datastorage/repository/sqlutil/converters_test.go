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

package sqlutil_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/sqlutil"
)

func TestSqlutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sqlutil Suite")
}

var _ = Describe("SQL Null Converters", func() {
	Describe("ToNullString", func() {
		It("should return Valid=false when pointer is nil", func() {
			result := sqlutil.ToNullString(nil)
			Expect(result.Valid).To(BeFalse())
		})

		It("should return Valid=false when string is empty", func() {
			emptyStr := ""
			result := sqlutil.ToNullString(&emptyStr)
			Expect(result.Valid).To(BeFalse())
		})

		It("should return Valid=true with string value when pointer is non-nil", func() {
			testStr := "test value"
			result := sqlutil.ToNullString(&testStr)
			Expect(result.Valid).To(BeTrue())
			Expect(result.String).To(Equal("test value"))
		})
	})

	Describe("ToNullStringValue", func() {
		It("should return Valid=false when string is empty", func() {
			result := sqlutil.ToNullStringValue("")
			Expect(result.Valid).To(BeFalse())
		})

		It("should return Valid=true with string value when non-empty", func() {
			result := sqlutil.ToNullStringValue("test value")
			Expect(result.Valid).To(BeTrue())
			Expect(result.String).To(Equal("test value"))
		})
	})

	Describe("ToNullUUID", func() {
		It("should return Valid=false when UUID pointer is nil", func() {
			result := sqlutil.ToNullUUID(nil)
			Expect(result.Valid).To(BeFalse())
		})

		It("should return Valid=true with UUID string when pointer is non-nil", func() {
			id := uuid.New()
			result := sqlutil.ToNullUUID(&id)
			Expect(result.Valid).To(BeTrue())
			Expect(result.String).To(Equal(id.String()))
		})
	})

	Describe("ToNullTime", func() {
		It("should return Valid=false when time pointer is nil", func() {
			result := sqlutil.ToNullTime(nil)
			Expect(result.Valid).To(BeFalse())
		})

		It("should return Valid=true with time value when pointer is non-nil", func() {
			now := time.Now()
			result := sqlutil.ToNullTime(&now)
			Expect(result.Valid).To(BeTrue())
			Expect(result.Time).To(BeTemporally("==", now))
		})
	})

	Describe("ToNullInt64", func() {
		It("should return Valid=false when int64 pointer is nil", func() {
			result := sqlutil.ToNullInt64(nil)
			Expect(result.Valid).To(BeFalse())
		})

		It("should return Valid=true with int64 value when pointer is non-nil", func() {
			value := int64(1500)
			result := sqlutil.ToNullInt64(&value)
			Expect(result.Valid).To(BeTrue())
			Expect(result.Int64).To(Equal(int64(1500)))
		})

		It("should handle zero value correctly", func() {
			value := int64(0)
			result := sqlutil.ToNullInt64(&value)
			Expect(result.Valid).To(BeTrue())
			Expect(result.Int64).To(Equal(int64(0)))
		})
	})

	Describe("FromNullString", func() {
		It("should return nil when Valid=false", func() {
			nullStr := sql.NullString{Valid: false}
			result := sqlutil.FromNullString(nullStr)
			Expect(result).To(BeNil())
		})

		It("should return string pointer when Valid=true", func() {
			nullStr := sql.NullString{String: "test value", Valid: true}
			result := sqlutil.FromNullString(nullStr)
			Expect(result).ToNot(BeNil())
			Expect(*result).To(Equal("test value"))
		})
	})

	Describe("FromNullTime", func() {
		It("should return nil when Valid=false", func() {
			nullTime := sql.NullTime{Valid: false}
			result := sqlutil.FromNullTime(nullTime)
			Expect(result).To(BeNil())
		})

		It("should return time pointer when Valid=true", func() {
			now := time.Now()
			nullTime := sql.NullTime{Time: now, Valid: true}
			result := sqlutil.FromNullTime(nullTime)
			Expect(result).ToNot(BeNil())
			Expect(*result).To(BeTemporally("==", now))
		})
	})

	Describe("FromNullInt64", func() {
		It("should return nil when Valid=false", func() {
			nullInt := sql.NullInt64{Valid: false}
			result := sqlutil.FromNullInt64(nullInt)
			Expect(result).To(BeNil())
		})

		It("should return int64 pointer when Valid=true", func() {
			nullInt := sql.NullInt64{Int64: 1500, Valid: true}
			result := sqlutil.FromNullInt64(nullInt)
			Expect(result).ToNot(BeNil())
			Expect(*result).To(Equal(int64(1500)))
		})

		It("should handle zero value correctly", func() {
			nullInt := sql.NullInt64{Int64: 0, Valid: true}
			result := sqlutil.FromNullInt64(nullInt)
			Expect(result).ToNot(BeNil())
			Expect(*result).To(Equal(int64(0)))
		})
	})

	Describe("Round-trip conversions", func() {
		It("should preserve string value through ToNull and From conversion", func() {
			original := "test value"
			nullStr := sqlutil.ToNullString(&original)
			result := sqlutil.FromNullString(nullStr)
			Expect(result).ToNot(BeNil())
			Expect(*result).To(Equal(original))
		})

		It("should preserve nil through ToNull and From conversion", func() {
			nullStr := sqlutil.ToNullString(nil)
			result := sqlutil.FromNullString(nullStr)
			Expect(result).To(BeNil())
		})

		It("should preserve UUID through ToNull and From conversion (as string)", func() {
			id := uuid.New()
			nullUUID := sqlutil.ToNullUUID(&id)
			// Note: FromNullString used because UUID is stored as string
			result := sqlutil.FromNullString(nullUUID)
			Expect(result).ToNot(BeNil())
			Expect(*result).To(Equal(id.String()))
		})

		It("should preserve time through ToNull and From conversion", func() {
			now := time.Now()
			nullTime := sqlutil.ToNullTime(&now)
			result := sqlutil.FromNullTime(nullTime)
			Expect(result).ToNot(BeNil())
			Expect(*result).To(BeTemporally("==", now))
		})

		It("should preserve int64 through ToNull and From conversion", func() {
			value := int64(1500)
			nullInt := sqlutil.ToNullInt64(&value)
			result := sqlutil.FromNullInt64(nullInt)
			Expect(result).ToNot(BeNil())
			Expect(*result).To(Equal(value))
		})
	})
})
