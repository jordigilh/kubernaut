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

package client

import (
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEffectivenessMonitorClientUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Effectiveness Monitor Client Unit Test Suite")
}

// Characterization tests for parsePromResponse (BR-EM-003).
// These tests pin down current behavior before refactoring to reduce
// cognitive complexity, so they must fail if that behavior changes.
var _ = Describe("parsePromResponse", func() {
	It("returns an error when the response body cannot be read", func() {
		_, err := parsePromResponse(&errorReader{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("reading Prometheus response body"))
	})

	It("returns an error when the response body is not valid JSON", func() {
		_, err := parsePromResponse(strings.NewReader("not json"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("parsing Prometheus response"))
	})

	It("returns an error when the API status is not success", func() {
		body := `{"status":"error","error":"query timed out"}`
		_, err := parsePromResponse(strings.NewReader(body))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("query timed out"))
	})

	It("parses a vector result with a numeric string value", func() {
		body := `{
			"status":"success",
			"data":{
				"resultType":"vector",
				"result":[
					{"metric":{"pod":"foo-1"},"value":[1700000000,"1.5"]}
				]
			}
		}`
		result, err := parsePromResponse(strings.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Samples).To(HaveLen(1))
		Expect(result.Samples[0].Metric).To(HaveKeyWithValue("pod", "foo-1"))
		Expect(result.Samples[0].Value).To(BeNumerically("==", 1.5))
		Expect(result.Samples[0].Timestamp.Unix()).To(BeNumerically("==", 1700000000))
	})

	It("parses a vector result with a numeric (non-string) value", func() {
		body := `{
			"status":"success",
			"data":{
				"resultType":"vector",
				"result":[
					{"metric":{"pod":"foo-2"},"value":[1700000000,2.25]}
				]
			}
		}`
		result, err := parsePromResponse(strings.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Samples).To(HaveLen(1))
		Expect(result.Samples[0].Value).To(BeNumerically("==", 2.25))
	})

	It("skips vector entries that fail to unmarshal instead of failing the whole response", func() {
		body := `{
			"status":"success",
			"data":{
				"resultType":"vector",
				"result":[
					"not-an-object",
					{"metric":{"pod":"foo-3"},"value":[1700000000,"3.0"]}
				]
			}
		}`
		result, err := parsePromResponse(strings.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Samples).To(HaveLen(1))
		Expect(result.Samples[0].Metric).To(HaveKeyWithValue("pod", "foo-3"))
	})

	It("skips vector entries whose value cannot be parsed as a float", func() {
		body := `{
			"status":"success",
			"data":{
				"resultType":"vector",
				"result":[
					{"metric":{"pod":"bad"},"value":[1700000000,"not-a-number"]},
					{"metric":{"pod":"good"},"value":[1700000000,"4.0"]}
				]
			}
		}`
		result, err := parsePromResponse(strings.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Samples).To(HaveLen(1))
		Expect(result.Samples[0].Metric).To(HaveKeyWithValue("pod", "good"))
	})

	It("parses a matrix result and returns all data points across the series", func() {
		body := `{
			"status":"success",
			"data":{
				"resultType":"matrix",
				"result":[
					{"metric":{"pod":"bar-1"},"values":[[1700000000,"1.0"],[1700000060,"2.0"]]}
				]
			}
		}`
		result, err := parsePromResponse(strings.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Samples).To(HaveLen(2))
		Expect(result.Samples[0].Value).To(BeNumerically("==", 1.0))
		Expect(result.Samples[1].Value).To(BeNumerically("==", 2.0))
	})

	It("skips matrix value pairs that fail to parse", func() {
		body := `{
			"status":"success",
			"data":{
				"resultType":"matrix",
				"result":[
					{"metric":{"pod":"bar-2"},"values":[[1700000000,"bad"],[1700000060,"5.0"]]}
				]
			}
		}`
		result, err := parsePromResponse(strings.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Samples).To(HaveLen(1))
		Expect(result.Samples[0].Value).To(BeNumerically("==", 5.0))
	})

	It("returns an empty sample set for an unrecognized result type", func() {
		body := `{
			"status":"success",
			"data":{
				"resultType":"scalar",
				"result":[]
			}
		}`
		result, err := parsePromResponse(strings.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Samples).To(BeEmpty())
	})
})

// errorReader implements io.Reader and always returns an error, used to
// characterize the read-failure branch of parsePromResponse.
type errorReader struct{}

func (e *errorReader) Read(_ []byte) (int, error) {
	return 0, errAlwaysFails
}

var errAlwaysFails = &readErr{}

type readErr struct{}

func (r *readErr) Error() string { return "simulated read failure" }
