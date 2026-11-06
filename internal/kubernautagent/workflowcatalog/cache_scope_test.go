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

package workflowcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
)

var _ = Describe("BR-WORKFLOW-006/007: trusted catalog informer scope", func() {
	It("UT-KA-2577-033 UT-KA-2577-034 excludes foreign workflows and action types", func() {
		requests := make(chan string, 20)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case requests <- r.URL.Path:
			default:
			}
			namespaced := strings.Contains(r.URL.Path, "/namespaces/trusted/")
			if r.URL.Query().Get("watch") == "true" {
				if r.URL.Query().Get("sendInitialEvents") == "true" {
					http.Error(w, "watch-list unsupported by fixture", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				return
			}
			w.Header().Set("Content-Type", "application/json")
			var response string
			switch {
			case strings.HasSuffix(r.URL.Path, "/remediationworkflows"):
				response = `{"apiVersion":"kubernaut.ai/v1alpha1","kind":"RemediationWorkflowList","metadata":{"resourceVersion":"1"},"items":[{"apiVersion":"kubernaut.ai/v1alpha1","kind":"RemediationWorkflow","metadata":{"name":"shared","namespace":"trusted","resourceVersion":"1"},"spec":{"actionType":"TrustedAction"},"status":{"workflowId":"shared-id"}}]}`
				if !namespaced {
					response = `{"apiVersion":"kubernaut.ai/v1alpha1","kind":"RemediationWorkflowList","metadata":{"resourceVersion":"1"},"items":[{"apiVersion":"kubernaut.ai/v1alpha1","kind":"RemediationWorkflow","metadata":{"name":"foreign-only","namespace":"foreign","resourceVersion":"1"},"spec":{"actionType":"ForeignAction"},"status":{"workflowId":"shared-id"}}]}`
				}
			case strings.HasSuffix(r.URL.Path, "/actiontypes"):
				response = `{"apiVersion":"kubernaut.ai/v1alpha1","kind":"ActionTypeList","metadata":{"resourceVersion":"1"},"items":[{"apiVersion":"kubernaut.ai/v1alpha1","kind":"ActionType","metadata":{"name":"shared","namespace":"trusted","resourceVersion":"1"},"spec":{"name":"TrustedAction"}}]}`
				if !namespaced {
					response = `{"apiVersion":"kubernaut.ai/v1alpha1","kind":"ActionTypeList","metadata":{"resourceVersion":"1"},"items":[{"apiVersion":"kubernaut.ai/v1alpha1","kind":"ActionType","metadata":{"name":"shared","namespace":"foreign","resourceVersion":"1"},"spec":{"name":"ForeignAction"}}]}`
				}
			default:
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(response))
		}))
		defer server.Close()
		scheme, err := NewScheme()
		Expect(err).NotTo(HaveOccurred())
		cache, cancel, err := NewInformerCache(&rest.Config{Host: server.URL}, scheme, "trusted", logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		defer cancel()
		workflows, err := cache.ListWorkflows(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(workflows).To(HaveLen(1))
		Expect(workflows[0].Namespace).To(Equal("trusted"))
		workflow, err := cache.GetWorkflowByID(context.Background(), "shared-id")
		Expect(err).NotTo(HaveOccurred())
		Expect(workflow).NotTo(BeNil())
		Expect(workflow.Spec.ActionType).To(Equal("TrustedAction"))
		foreignWorkflow, err := cache.GetWorkflow(context.Background(), "foreign-only")
		Expect(err).NotTo(HaveOccurred())
		Expect(foreignWorkflow).To(BeNil())
		action, err := cache.GetActionType(context.Background(), "TrustedAction")
		Expect(err).NotTo(HaveOccurred())
		Expect(action).NotTo(BeNil())
		Expect(action.Namespace).To(Equal("trusted"))
		foreign, err := cache.GetActionType(context.Background(), "ForeignAction")
		Expect(err).NotTo(HaveOccurred())
		Expect(foreign).To(BeNil())
		Expect(len(requests)).To(BeNumerically(">=", 2))
		for len(requests) > 0 {
			Expect(<-requests).To(ContainSubstring("/namespaces/trusted/"))
		}
	})
})
