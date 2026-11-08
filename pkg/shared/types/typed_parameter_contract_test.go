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

package types_test

import (
	"encoding/json"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("BR-WORKFLOW-004: typed parameter transport", func() {
	It("UT-CRD-2575-001: all persisted selection and approval surfaces use the same typed parameter map", func() {
		mapType := reflect.TypeOf(sharedtypes.TypedParameterMap{})
		selected, ok := reflect.TypeOf(agentsessionv1.AgentSessionResult{}).FieldByName("SelectedWorkflow")
		Expect(ok).To(BeTrue())
		Expect(selected.Type.Kind()).To(Equal(reflect.Ptr))
		agentParameters, ok := selected.Type.Elem().FieldByName("Parameters")
		Expect(ok).To(BeTrue(), "AgentSession selection must have typed parameters")
		Expect(agentParameters.Type).To(Equal(mapType))
		for _, surface := range []reflect.Type{reflect.TypeOf(aianalysisv1.SelectedWorkflow{}), reflect.TypeOf(remediationv1.RemediationApprovalRequestSpec{}), reflect.TypeOf(remediationv1.WorkflowOverride{}), reflect.TypeOf(workflowexecutionv1.WorkflowExecutionSpec{})} {
			field, found := surface.FieldByName("Parameters")
			Expect(found).To(BeTrue(), surface.String())
			Expect(field.Type).To(Equal(mapType), surface.String())
		}
	})
	It("UT-TYP-2575-001: preserves zero values, array order, and numeric evidence", func() {
		values, evidence, err := sharedtypes.DecodeTypedParameterMap([]byte(`{"empty":"","false":false,"zero":0,"array":["x",0,false],"emptyArray":[],"float":1e0,"large":9007199254740993}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(evidence).To(ContainSubstring(`1e0`))
		Expect(evidence).To(ContainSubstring(`9007199254740993`))
		Expect(*values["empty"].StringValue).To(BeEmpty())
		Expect(*values["false"].BooleanValue).To(BeFalse())
		Expect(*values["zero"].IntegerValue).To(BeZero())
		Expect(values["emptyArray"].ArrayValue).To(BeEmpty())
		Expect(values["array"].ArrayValue).To(HaveLen(3))
		Expect(values["array"].ArrayValue[0].Kind).To(Equal("string"))
		Expect(values["array"].ArrayValue[1].Kind).To(Equal("integer"))
		Expect(values["array"].ArrayValue[2].Kind).To(Equal("boolean"))
	})

	It("UT-TYP-2575-002: rejects unsupported and unsafe ordinary JSON values", func() {
		for _, raw := range []string{`{"P":null}`, `{"P":{}}`, `{"P":[[]]}`, `{"P":[{}]}`, `{"P":9223372036854775808}`, `{"P":1e9999}`, `{"P":1e-9999}`} {
			_, _, err := sharedtypes.DecodeTypedParameterMap([]byte(raw))
			Expect(err).To(HaveOccurred(), raw)
		}
	})

	It("UT-TYP-2575-003: rejects malformed persisted unions on ingress and egress", func() {
		for _, raw := range []string{`{"P":null}`, `{"P":{"kind":"string","stringValue":"ok","unknown":true}}`, `{"P":{"kind":"string","stringValue":"ok","integerValue":1}}`, `{"P":{"kind":"array","arrayValue":[{"kind":"string","stringValue":"x","arrayValue":[]}]}}`} {
			var values sharedtypes.TypedParameterMap
			Expect(json.Unmarshal([]byte(raw), &values)).NotTo(Succeed(), raw)
		}
		text, number := "x", int64(1)
		_, err := json.Marshal(sharedtypes.TypedParameterMap{"P": {Kind: "string", StringValue: &text, IntegerValue: &number}})
		Expect(err).To(HaveOccurred())
		mixed := sharedtypes.TypedParameterValue{Kind: "string", StringValue: &text, IntegerValue: &number}
		_, err = sharedtypes.TypedParameterValueToJSONValue(mixed)
		Expect(err).To(HaveOccurred())
		_, err = sharedtypes.TypedParameterValueToEngineValue(mixed)
		Expect(err).To(HaveOccurred())
	})
})
