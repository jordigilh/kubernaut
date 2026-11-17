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

package executor_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// UT-WE-2588-011 (BR-WE-015, #2588): AWX's launch endpoint accepts labels
// only as label primary keys, never as names, and labels are keyed by
// (organization, name). CI run 43937713539 proved a string payload is
// rejected with HTTP 400
// `{"labels":["Incorrect type. Expected pk value, received str."]}`.
type recordingAWXServer struct {
	server        *httptest.Server
	launchBody    map[string]any
	createdLabels []map[string]any
	labelSearches int
	launchCalls   int
	// searchResults is the body returned for label searches.
	searchResults string
	// labelStatusCode overrides the label search/create response status.
	labelSearchStatus int
	labelCreateStatus int
	templateStatus    int
	// templateBody overrides the job-template GET body.
	templateBody string
	// createResponse overrides the label-create success body.
	createResponse string
}

func newRecordingAWXServer(t GinkgoTInterface, searchResults string) *recordingAWXServer {
	rec := &recordingAWXServer{searchResults: searchResults}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/labels/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			rec.labelSearches++
			status := rec.labelSearchStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if status == http.StatusOK {
				_, _ = io.WriteString(w, rec.searchResults)
			} else {
				_, _ = io.WriteString(w, `{"detail":"search failed"}`)
			}
		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			created := map[string]any{}
			Expect(json.Unmarshal(body, &created)).To(Succeed())
			rec.createdLabels = append(rec.createdLabels, created)
			status := rec.labelCreateStatus
			if status == 0 {
				status = http.StatusCreated
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if status == http.StatusCreated {
				_, _ = io.WriteString(w, orDefault(rec.createResponse, `{"id":99,"name":"kubernaut-awx-launch-created"}`))
			} else {
				_, _ = io.WriteString(w, `{"detail":"create failed"}`)
			}
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/api/v2/job_templates/9/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/job_templates/9/":
			status := rec.templateStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if status == http.StatusOK {
				_, _ = io.WriteString(w, orDefault(rec.templateBody, `{"id":9,"ask_labels_on_launch":true,"organization":3}`))
			} else {
				_, _ = io.WriteString(w, `{"detail":"template unavailable"}`)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/job_templates/9/launch/":
			rec.launchCalls++
			body, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			rec.launchBody = map[string]any{}
			Expect(json.Unmarshal(body, &rec.launchBody)).To(Succeed())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":77,"status":"pending"}`)
		default:
			http.NotFound(w, r)
		}
	})
	rec.server = httptest.NewServer(mux)
	t.Cleanup(rec.server.Close)
	return rec
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func launchWithLabels(rec *recordingAWXServer, label string) (int, error) {
	client, err := executor.NewAWXHTTPClient(rec.server.URL, "test-token")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return client.LaunchJobTemplateWithLabels(context.Background(), 9, nil, nil, []string{label})
}

var _ = Describe("AWX launch label handling (UT-WE-2588-011)", func() {
	It("launches with the organization-scoped label primary key, not the name", func() {
		rec := newRecordingAWXServer(GinkgoT(), `{"results":[{"id":42,"name":"kubernaut-awx-launch-existing","organization":3}]}`)
		jobID, err := launchWithLabels(rec, "kubernaut-awx-launch-existing")
		Expect(err).NotTo(HaveOccurred())
		Expect(jobID).To(Equal(77))
		Expect(rec.launchCalls).To(Equal(1))
		Expect(rec.launchBody["labels"]).To(Equal([]any{float64(42)}),
			"AWX requires label PKs; string names are rejected with HTTP 400")
	})

	It("does not reuse a same-named label from another organization", func() {
		// AWX labels are keyed by (organization, name): the foreign-org
		// result must be ignored and the label created in the template's
		// own organization instead.
		rec := newRecordingAWXServer(GinkgoT(), `{"results":[{"id":42,"name":"kubernaut-awx-launch-dup","organization":7}]}`)
		jobID, err := launchWithLabels(rec, "kubernaut-awx-launch-dup")
		Expect(err).NotTo(HaveOccurred())
		Expect(jobID).To(Equal(77))
		Expect(rec.createdLabels).To(HaveLen(1))
		Expect(rec.createdLabels[0]["organization"]).To(Equal(float64(3)))
		Expect(rec.launchBody["labels"]).To(Equal([]any{float64(99)}),
			"the cross-organization label PK must never be launched with")
	})

	It("creates a missing launch label in the template organization", func() {
		rec := newRecordingAWXServer(GinkgoT(), `{"results":[]}`)
		jobID, err := launchWithLabels(rec, "kubernaut-awx-launch-created")
		Expect(err).NotTo(HaveOccurred())
		Expect(jobID).To(Equal(77))
		Expect(rec.createdLabels).To(HaveLen(1))
		Expect(rec.createdLabels[0]["name"]).To(Equal("kubernaut-awx-launch-created"))
		Expect(rec.createdLabels[0]["organization"]).To(Equal(float64(3)))
		Expect(rec.launchBody["labels"]).To(Equal([]any{float64(99)}))
	})

	It("classifies a transient label search failure as recovery pending", func() {
		rec := newRecordingAWXServer(GinkgoT(), `{"results":[]}`)
		rec.labelSearchStatus = http.StatusInternalServerError
		_, err := launchWithLabels(rec, "kubernaut-awx-launch-transient")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, executor.ErrAWXLaunchRecoveryPending)).To(BeTrue(),
			"a 5xx label search must retry instead of terminalizing the WorkflowExecution")
		Expect(rec.launchCalls).To(Equal(0), "nothing must be launched while label resolution is failing")
	})

	It("classifies a transient label create failure as recovery pending", func() {
		rec := newRecordingAWXServer(GinkgoT(), `{"results":[]}`)
		rec.labelCreateStatus = http.StatusBadGateway
		_, err := launchWithLabels(rec, "kubernaut-awx-launch-transient")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, executor.ErrAWXLaunchRecoveryPending)).To(BeTrue(),
			"a 5xx label create is ambiguous and must retry; the retried search finds the label")
		Expect(rec.launchCalls).To(Equal(0))
	})

	It("classifies a transient template fetch failure as recovery pending", func() {
		rec := newRecordingAWXServer(GinkgoT(), `{"results":[]}`)
		rec.templateStatus = http.StatusServiceUnavailable
		_, err := launchWithLabels(rec, "kubernaut-awx-launch-transient")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, executor.ErrAWXLaunchRecoveryPending)).To(BeTrue(),
			"a 5xx template fetch must retry instead of terminalizing the WorkflowExecution")
		Expect(rec.labelSearches).To(Equal(0))
		Expect(rec.launchCalls).To(Equal(0))
	})

	It("surfaces a deterministic template fetch client error without the retry sentinel", func() {
		rec := newRecordingAWXServer(GinkgoT(), `{"results":[]}`)
		rec.templateStatus = http.StatusNotFound
		_, err := launchWithLabels(rec, "kubernaut-awx-launch-missing-template")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, executor.ErrAWXLaunchRecoveryPending)).To(BeFalse(),
			"a 4xx template fetch is deterministic and must not trigger recovery retries")
	})

	It("rejects a template without an organization instead of guessing a label scope", func() {
		rec := newRecordingAWXServer(GinkgoT(), `{"results":[]}`)
		rec.templateBody = `{"id":9,"ask_labels_on_launch":true,"organization":0}`
		_, err := launchWithLabels(rec, "kubernaut-awx-launch-unscoped")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no organization"),
			"labels must never be matched or created without their organization scope")
		Expect(rec.launchCalls).To(Equal(0))
	})

	It("surfaces a deterministic label search client error without the retry sentinel", func() {
		rec := newRecordingAWXServer(GinkgoT(), `{"results":[]}`)
		rec.labelSearchStatus = http.StatusForbidden
		_, err := launchWithLabels(rec, "kubernaut-awx-launch-forbidden")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, executor.ErrAWXLaunchRecoveryPending)).To(BeFalse())
	})

	It("surfaces a deterministic label create client error without the retry sentinel", func() {
		rec := newRecordingAWXServer(GinkgoT(), `{"results":[]}`)
		rec.labelCreateStatus = http.StatusBadRequest
		_, err := launchWithLabels(rec, "kubernaut-awx-launch-bad-create")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, executor.ErrAWXLaunchRecoveryPending)).To(BeFalse())
	})

	It("treats a label create response without a valid id as recovery pending", func() {
		rec := newRecordingAWXServer(GinkgoT(), `{"results":[]}`)
		rec.createResponse = `{"id":0}`
		_, err := launchWithLabels(rec, "kubernaut-awx-launch-empty-id")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, executor.ErrAWXLaunchRecoveryPending)).To(BeTrue(),
			"an unusable create response is ambiguous and must retry via search")
	})
})
