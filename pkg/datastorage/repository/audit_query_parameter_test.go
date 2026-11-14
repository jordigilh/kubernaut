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

package repository

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAuditParameterReadback(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Audit Parameter Readback Suite")
}

var _ = Describe("BR-AUDIT-005: audit query numeric fidelity", func() {
	It("UT-AUD-2575-023: scanQueryRow retains int64 values and separate lexical evidence", func() {
		db, mock, err := sqlmock.New()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(db.Close)
		columns := make([]string, 31)
		values := make([]driver.Value, len(columns))
		for i := range columns {
			columns[i] = fmt.Sprintf("column_%d", i)
		}
		now := time.Now().UTC()
		values[0] = "25e60fbc-3e1e-4145-8d74-3247b9029292"
		values[1], values[2], values[3], values[4] = "1.0", "workflow.test", "workflow", "execute"
		values[5], values[6], values[7] = "rr-2575", now, "success"
		values[15] = []byte(`{"parameters":{"P":{"kind":"integer","integerValue":9007199254740993}},"parameter_evidence":"{\"P\":9007199254740993,\"F\":1e0}"}`)
		values[16] = now
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(columns).AddRow(values...))
		mock.ExpectClose()
		rows, err := db.Query("SELECT test")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(rows.Close)
		Expect(rows.Next()).To(BeTrue())
		event, err := scanQueryRow(rows)
		Expect(err).NotTo(HaveOccurred())
		parameters := event.EventData["parameters"].(map[string]interface{})
		value := parameters["P"].(map[string]interface{})
		Expect(value["integerValue"]).To(Equal(json.Number("9007199254740993")))
		Expect(event.EventData["parameter_evidence"]).To(Equal(`{"P":9007199254740993,"F":1e0}`))
	})
})
