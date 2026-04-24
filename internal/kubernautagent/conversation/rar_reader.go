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

package conversation

import (
	"context"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// RARReader reads RemediationApprovalRequest context for conversation sessions.
type RARReader interface {
	GetRARContext(ctx context.Context, namespace, name string) (*RARContext, error)
}

// RARContext holds the investigation context extracted from a RAR CRD.
type RARContext struct {
	InvestigationSummary string
	Reason               string
	Confidence           float64
}

var rarGVR = schema.GroupVersionResource{
	Group:    "kubernaut.ai",
	Version:  "v1alpha1",
	Resource: "remediationapprovalrequests",
}

// DynamicRARReader reads RAR CRDs using the Kubernetes dynamic client.
type DynamicRARReader struct {
	client dynamic.Interface
	logger *slog.Logger
}

// NewDynamicRARReader creates a RARReader backed by the dynamic client.
func NewDynamicRARReader(client dynamic.Interface, logger *slog.Logger) *DynamicRARReader {
	return &DynamicRARReader{client: client, logger: logger}
}

// GetRARContext fetches the RAR and extracts investigation context fields.
func (r *DynamicRARReader) GetRARContext(ctx context.Context, namespace, name string) (*RARContext, error) {
	obj, err := r.client.Resource(rarGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting RAR %s/%s: %w", namespace, name, err)
	}

	summary, _, _ := unstructured.NestedString(obj.Object, "spec", "investigationSummary")
	reason, _, _ := unstructured.NestedString(obj.Object, "spec", "reason")
	confidence, _, _ := unstructured.NestedFloat64(obj.Object, "spec", "confidence")

	return &RARContext{
		InvestigationSummary: summary,
		Reason:               reason,
		Confidence:           confidence,
	}, nil
}
