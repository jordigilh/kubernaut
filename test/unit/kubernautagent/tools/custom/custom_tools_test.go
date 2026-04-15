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

package custom_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
)

// fakeWorkflowDS captures the params passed to each DS method and returns
// canned responses. Satisfies custom.WorkflowDiscoveryClient.
type fakeWorkflowDS struct {
	listActionsParams   ogenclient.ListAvailableActionsParams
	listActionsResponse *ogenclient.ActionTypeListResponse

	listWorkflowsParams   ogenclient.ListWorkflowsByActionTypeParams
	listWorkflowsResponse *ogenclient.WorkflowDiscoveryResponse

}

func (f *fakeWorkflowDS) ListAvailableActions(_ context.Context, params ogenclient.ListAvailableActionsParams) (ogenclient.ListAvailableActionsRes, error) {
	f.listActionsParams = params
	return f.listActionsResponse, nil
}

func (f *fakeWorkflowDS) ListWorkflowsByActionType(_ context.Context, params ogenclient.ListWorkflowsByActionTypeParams) (ogenclient.ListWorkflowsByActionTypeRes, error) {
	f.listWorkflowsParams = params
	return f.listWorkflowsResponse, nil
}

func (f *fakeWorkflowDS) GetWorkflowByID(_ context.Context, _ ogenclient.GetWorkflowByIDParams) (ogenclient.GetWorkflowByIDRes, error) {
	return &ogenclient.RemediationWorkflow{}, nil
}

var _ = Describe("UT-KA-688: Conditional pagination stripping", func() {

	Describe("UT-KA-688-001: StripPaginationIfComplete removes pagination when all results fit in one page", func() {
		It("should strip pagination when hasMore is false", func() {
			input := `{"actionTypes":[{"actionType":"RestartDeployment"},{"actionType":"ScaleReplicas"}],"pagination":{"totalCount":2,"offset":0,"limit":10,"hasMore":false}}`
			result := custom.StripPaginationIfComplete(json.RawMessage(input))

			var parsed map[string]interface{}
			Expect(json.Unmarshal(result, &parsed)).To(Succeed())
			Expect(parsed).NotTo(HaveKey("pagination"))
			Expect(parsed).To(HaveKey("actionTypes"))
		})

		It("should preserve pagination when hasMore is true", func() {
			input := `{"actionTypes":[{"actionType":"RestartDeployment"}],"pagination":{"totalCount":16,"offset":0,"limit":10,"hasMore":true}}`
			result := custom.StripPaginationIfComplete(json.RawMessage(input))

			var parsed map[string]interface{}
			Expect(json.Unmarshal(result, &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("pagination"))
			Expect(parsed).To(HaveKey("actionTypes"))
		})
	})

	Describe("UT-KA-688-002: StripPaginationIfComplete works for workflow discovery responses", func() {
		It("should strip pagination from workflow list when complete", func() {
			input := `{"actionType":"RestartDeployment","workflows":[{"workflowId":"abc-123"}],"pagination":{"totalCount":1,"offset":0,"limit":10,"hasMore":false}}`
			result := custom.StripPaginationIfComplete(json.RawMessage(input))

			var parsed map[string]interface{}
			Expect(json.Unmarshal(result, &parsed)).To(Succeed())
			Expect(parsed).NotTo(HaveKey("pagination"))
			Expect(parsed).To(HaveKey("workflows"))
			Expect(parsed).To(HaveKey("actionType"))
		})
	})

	Describe("UT-KA-688-003: StripPaginationIfComplete is safe for edge cases", func() {
		It("should return input unchanged when pagination field is absent", func() {
			input := `{"actionTypes":[{"actionType":"RestartDeployment"}]}`
			result := custom.StripPaginationIfComplete(json.RawMessage(input))

			var parsed map[string]interface{}
			Expect(json.Unmarshal(result, &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("actionTypes"))
		})

		It("should return input unchanged for invalid JSON", func() {
			input := `not json`
			result := custom.StripPaginationIfComplete(json.RawMessage(input))
			Expect(string(result)).To(Equal(input))
		})
	})
})

// --- Group A: Cursor Encoding/Decoding (UT-KA-688-100 through UT-KA-688-104) ---

var _ = Describe("UT-KA-688: Cursor encoding/decoding", func() {

	Describe("UT-KA-688-100: EncodeCursor round-trips correctly", func() {
		It("should produce a token that DecodeCursor restores to the original values", func() {
			token := custom.EncodeCursor(10, 10)
			Expect(token).NotTo(BeEmpty())

			offset, limit := custom.DecodeCursor(token)
			Expect(offset).To(Equal(10))
			Expect(limit).To(Equal(10))
		})

		It("should round-trip with offset=0 and limit=10", func() {
			token := custom.EncodeCursor(0, 10)
			offset, limit := custom.DecodeCursor(token)
			Expect(offset).To(Equal(0))
			Expect(limit).To(Equal(10))
		})

		It("should round-trip with large valid offset", func() {
			token := custom.EncodeCursor(90, 10)
			offset, limit := custom.DecodeCursor(token)
			Expect(offset).To(Equal(90))
			Expect(limit).To(Equal(10))
		})
	})

	Describe("UT-KA-688-101: DecodeCursor handles invalid base64", func() {
		It("should return safe defaults for garbage input", func() {
			offset, limit := custom.DecodeCursor("not-valid-base64!!!")
			Expect(offset).To(Equal(0))
			Expect(limit).To(Equal(10))
		})

		It("should return safe defaults for empty string", func() {
			offset, limit := custom.DecodeCursor("")
			Expect(offset).To(Equal(0))
			Expect(limit).To(Equal(10))
		})
	})

	Describe("UT-KA-688-102: DecodeCursor handles valid base64 but non-JSON content", func() {
		It("should return safe defaults when base64 decodes to non-JSON", func() {
			token := base64.RawURLEncoding.EncodeToString([]byte("not json at all"))
			offset, limit := custom.DecodeCursor(token)
			Expect(offset).To(Equal(0))
			Expect(limit).To(Equal(10))
		})
	})

	Describe("UT-KA-688-103: DecodeCursor clamps tampered values", func() {
		DescribeTable("should clamp invalid values to safe defaults",
			func(inputJSON string, expectedOffset, expectedLimit int) {
				token := base64.RawURLEncoding.EncodeToString([]byte(inputJSON))
				offset, limit := custom.DecodeCursor(token)
				Expect(offset).To(Equal(expectedOffset))
				Expect(limit).To(Equal(expectedLimit))
			},
			Entry("negative offset → 0", `{"o":-5,"l":10}`, 0, 10),
			Entry("huge limit → 100", `{"o":0,"l":500}`, 0, 100),
			Entry("zero limit → default 10", `{"o":0,"l":0}`, 0, 10),
			Entry("negative limit → default 10", `{"o":0,"l":-1}`, 0, 10),
			Entry("missing offset key → 0", `{"l":10}`, 0, 10),
			Entry("missing limit key → default 10", `{"o":20}`, 20, 10),
		)
	})

	Describe("UT-KA-688-104: EncodeCursor produces base64-URL safe output", func() {
		It("should not contain padding or URL-unsafe characters", func() {
			token := custom.EncodeCursor(10, 10)
			Expect(token).NotTo(ContainSubstring("="))
			Expect(token).NotTo(ContainSubstring("+"))
			Expect(token).NotTo(ContainSubstring("/"))
			Expect(strings.TrimSpace(token)).To(Equal(token))
		})
	})
})

// --- Group B: TransformPagination (UT-KA-688-110 through UT-KA-688-115) ---

var _ = Describe("UT-KA-688: TransformPagination", func() {

	Describe("UT-KA-688-110: First page with more results (offset=0, hasMore=true)", func() {
		It("should include hasNext and nextCursor, but not hasPrevious or previousCursor", func() {
			input := `{"actionTypes":[{"actionType":"RestartDeployment"}],"pagination":{"totalCount":16,"offset":0,"limit":10,"hasMore":true}}`
			result := custom.TransformPagination(json.RawMessage(input))

			var parsed map[string]json.RawMessage
			Expect(json.Unmarshal(result, &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("actionTypes"))
			Expect(parsed).To(HaveKey("pagination"))

			var pag map[string]interface{}
			Expect(json.Unmarshal(parsed["pagination"], &pag)).To(Succeed())
			Expect(pag["hasNext"]).To(BeTrue())
			Expect(pag).To(HaveKey("nextCursor"))
			Expect(pag).NotTo(HaveKey("hasPrevious"))
			Expect(pag).NotTo(HaveKey("previousCursor"))
			Expect(pag).NotTo(HaveKey("totalCount"))
			Expect(pag).NotTo(HaveKey("offset"))
			Expect(pag).NotTo(HaveKey("limit"))
		})
	})

	Describe("UT-KA-688-111: Middle page (offset=10, hasMore=true)", func() {
		It("should include both hasNext/nextCursor and hasPrevious/previousCursor", func() {
			input := `{"actionTypes":[{"actionType":"IncreaseCPU"}],"pagination":{"totalCount":30,"offset":10,"limit":10,"hasMore":true}}`
			result := custom.TransformPagination(json.RawMessage(input))

			var parsed map[string]json.RawMessage
			Expect(json.Unmarshal(result, &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("pagination"))

			var pag map[string]interface{}
			Expect(json.Unmarshal(parsed["pagination"], &pag)).To(Succeed())
			Expect(pag["hasNext"]).To(BeTrue())
			Expect(pag).To(HaveKey("nextCursor"))
			Expect(pag["hasPrevious"]).To(BeTrue())
			Expect(pag).To(HaveKey("previousCursor"))

			nextOffset, nextLimit := custom.DecodeCursor(pag["nextCursor"].(string))
			Expect(nextOffset).To(Equal(20))
			Expect(nextLimit).To(Equal(10))

			prevOffset, prevLimit := custom.DecodeCursor(pag["previousCursor"].(string))
			Expect(prevOffset).To(Equal(0))
			Expect(prevLimit).To(Equal(10))
		})
	})

	Describe("UT-KA-688-112: Last page (offset=20, hasMore=false)", func() {
		It("should include hasPrevious/previousCursor but not hasNext/nextCursor", func() {
			input := `{"actionTypes":[{"actionType":"Rollback"}],"pagination":{"totalCount":25,"offset":20,"limit":10,"hasMore":false}}`
			result := custom.TransformPagination(json.RawMessage(input))

			var parsed map[string]json.RawMessage
			Expect(json.Unmarshal(result, &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("pagination"))

			var pag map[string]interface{}
			Expect(json.Unmarshal(parsed["pagination"], &pag)).To(Succeed())
			Expect(pag).NotTo(HaveKey("hasNext"))
			Expect(pag).NotTo(HaveKey("nextCursor"))
			Expect(pag["hasPrevious"]).To(BeTrue())
			Expect(pag).To(HaveKey("previousCursor"))

			prevOffset, prevLimit := custom.DecodeCursor(pag["previousCursor"].(string))
			Expect(prevOffset).To(Equal(10))
			Expect(prevLimit).To(Equal(10))
		})
	})

	Describe("UT-KA-688-113: Single page (offset=0, hasMore=false)", func() {
		It("should strip pagination entirely", func() {
			input := `{"actionTypes":[{"actionType":"RestartDeployment"}],"pagination":{"totalCount":3,"offset":0,"limit":10,"hasMore":false}}`
			result := custom.TransformPagination(json.RawMessage(input))

			var parsed map[string]interface{}
			Expect(json.Unmarshal(result, &parsed)).To(Succeed())
			Expect(parsed).NotTo(HaveKey("pagination"))
			Expect(parsed).To(HaveKey("actionTypes"))
		})
	})

	Describe("UT-KA-688-114: TransformPagination never exposes totalCount", func() {
		DescribeTable("totalCount must be absent in all page positions",
			func(input string) {
				result := custom.TransformPagination(json.RawMessage(input))

				var parsed map[string]json.RawMessage
				Expect(json.Unmarshal(result, &parsed)).To(Succeed())
				if pagRaw, ok := parsed["pagination"]; ok {
					var pag map[string]interface{}
					Expect(json.Unmarshal(pagRaw, &pag)).To(Succeed())
					Expect(pag).NotTo(HaveKey("totalCount"))
				}
			},
			Entry("first page", `{"actionTypes":[],"pagination":{"totalCount":16,"offset":0,"limit":10,"hasMore":true}}`),
			Entry("middle page", `{"actionTypes":[],"pagination":{"totalCount":30,"offset":10,"limit":10,"hasMore":true}}`),
			Entry("last page", `{"actionTypes":[],"pagination":{"totalCount":25,"offset":20,"limit":10,"hasMore":false}}`),
			Entry("single page", `{"actionTypes":[],"pagination":{"totalCount":3,"offset":0,"limit":10,"hasMore":false}}`),
		)
	})

	Describe("UT-KA-688-115: TransformPagination preserves non-pagination fields", func() {
		It("should preserve actionTypes for action list response", func() {
			input := `{"actionTypes":[{"actionType":"ScaleReplicas"}],"pagination":{"totalCount":16,"offset":0,"limit":10,"hasMore":true}}`
			result := custom.TransformPagination(json.RawMessage(input))

			var parsed map[string]interface{}
			Expect(json.Unmarshal(result, &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("actionTypes"))
		})

		It("should preserve workflows and actionType for workflow response", func() {
			input := `{"actionType":"ScaleReplicas","workflows":[{"workflowId":"abc-123"}],"pagination":{"totalCount":16,"offset":0,"limit":10,"hasMore":true}}`
			result := custom.TransformPagination(json.RawMessage(input))

			var parsed map[string]interface{}
			Expect(json.Unmarshal(result, &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("workflows"))
			Expect(parsed).To(HaveKey("actionType"))
		})

		It("should return input unchanged for invalid JSON", func() {
			input := `not json`
			result := custom.TransformPagination(json.RawMessage(input))
			Expect(string(result)).To(Equal(input))
		})

		It("should return input unchanged when no pagination field exists", func() {
			input := `{"actionTypes":[{"actionType":"RestartDeployment"}]}`
			result := custom.TransformPagination(json.RawMessage(input))

			var parsed map[string]interface{}
			Expect(json.Unmarshal(result, &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("actionTypes"))
			Expect(parsed).NotTo(HaveKey("pagination"))
		})
	})
})

// --- Group C: Schema Validation (UT-KA-688-201 through UT-KA-688-202) ---

var _ = Describe("Kubernaut Agent Custom Tool Schemas — #433", func() {

	Describe("UT-KA-433-170: list_available_actions has valid JSON schema", func() {
		It("should return a non-nil parameter schema", func() {
			schema := custom.ListAvailableActionsSchema()
			Expect(schema).NotTo(BeNil())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal("object"))
		})
	})

	Describe("UT-KA-433-171: list_workflows has valid JSON schema with required action_type", func() {
		It("should require action_type parameter", func() {
			schema := custom.ListWorkflowsSchema()
			Expect(schema).NotTo(BeNil())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal("object"))

			required, ok := parsed["required"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(required).To(ContainElement("action_type"))
		})

		It("should not expose offset/limit (DD-WORKFLOW-016 v1.4: cursor replaces raw offset)", func() {
			schema := custom.ListWorkflowsSchema()

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())

			props, ok := parsed["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(props).NotTo(HaveKey("offset"))
			Expect(props).NotTo(HaveKey("limit"))
		})
	})

	Describe("UT-KA-433-172: get_workflow has valid JSON schema with required workflow_id", func() {
		It("should require workflow_id parameter", func() {
			schema := custom.GetWorkflowSchema()
			Expect(schema).NotTo(BeNil())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal("object"))

			required, ok := parsed["required"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(required).To(ContainElement("workflow_id"))
		})
	})

	Describe("UT-KA-433-173: All existing custom tools return non-nil Parameters()", func() {
		It("should have non-nil schemas for all 3 DataStorage tools", func() {
			Expect(custom.ListAvailableActionsSchema()).NotTo(BeNil())
			Expect(custom.ListWorkflowsSchema()).NotTo(BeNil())
			Expect(custom.GetWorkflowSchema()).NotTo(BeNil())
		})
	})

	Describe("UT-KA-688-201: list_workflows schema includes cursor pagination properties", func() {
		It("should have page property with enum [next, previous]", func() {
			schema := custom.ListWorkflowsSchema()

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())

			props, ok := parsed["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			pageProp, ok := props["page"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "page property must exist in list_workflows schema")
			Expect(pageProp["type"]).To(Equal("string"))

			enumVals, ok := pageProp["enum"].([]interface{})
			Expect(ok).To(BeTrue(), "page must have enum constraint")
			Expect(enumVals).To(ConsistOf("next", "previous"))
		})

		It("should have cursor property as string type", func() {
			schema := custom.ListWorkflowsSchema()

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())

			props := parsed["properties"].(map[string]interface{})
			cursorProp, ok := props["cursor"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "cursor property must exist in list_workflows schema")
			Expect(cursorProp["type"]).To(Equal("string"))
		})
	})

	Describe("UT-KA-688-202: list_available_actions schema includes cursor pagination properties", func() {
		It("should have page property with enum [next, previous]", func() {
			schema := custom.ListAvailableActionsSchema()

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())

			props, ok := parsed["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			pageProp, ok := props["page"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "page property must exist in list_available_actions schema")
			Expect(pageProp["type"]).To(Equal("string"))

			enumVals, ok := pageProp["enum"].([]interface{})
			Expect(ok).To(BeTrue(), "page must have enum constraint")
			Expect(enumVals).To(ConsistOf("next", "previous"))
		})

		It("should have cursor property as string type", func() {
			schema := custom.ListAvailableActionsSchema()

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())

			props := parsed["properties"].(map[string]interface{})
			cursorProp, ok := props["cursor"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "cursor property must exist in list_available_actions schema")
			Expect(cursorProp["type"]).To(Equal("string"))
		})

		It("should not expose offset/limit", func() {
			schema := custom.ListAvailableActionsSchema()

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())

			props := parsed["properties"].(map[string]interface{})
			Expect(props).NotTo(HaveKey("offset"))
			Expect(props).NotTo(HaveKey("limit"))
		})
	})
})

// --- Group D: Execute Wiring (UT-KA-688-301 through UT-KA-688-305) ---

var _ = Describe("UT-KA-688: Execute wiring with cursor pagination", func() {

	var fake *fakeWorkflowDS

	singlePagePagination := ogenclient.PaginationMetadata{
		TotalCount: 2, Offset: 0, Limit: 10, HasMore: false,
	}
	multiPagePagination := ogenclient.PaginationMetadata{
		TotalCount: 16, Offset: 0, Limit: 10, HasMore: true,
	}

	BeforeEach(func() {
		fake = &fakeWorkflowDS{
			listActionsResponse: &ogenclient.ActionTypeListResponse{
				ActionTypes: []ogenclient.ActionTypeEntry{
					{
						ActionType:    "ScaleReplicas",
						Description:   ogenclient.StructuredDescription{What: "test", WhenToUse: "test"},
						WorkflowCount: 1,
					},
				},
				Pagination: singlePagePagination,
			},
			listWorkflowsResponse: &ogenclient.WorkflowDiscoveryResponse{
				ActionType: "ScaleReplicas",
				Workflows: []ogenclient.WorkflowDiscoveryEntry{
					{
						WorkflowId:   uuid.New(),
						WorkflowName: "scale-conservative-v1",
						Name:         "Scale Conservative",
						Description:  ogenclient.StructuredDescription{What: "test", WhenToUse: "test"},
					},
				},
				Pagination: singlePagePagination,
			},
		}
	})

	Describe("UT-KA-688-301: list_workflows Execute with no pagination args", func() {
		It("should call DS with unset Offset/Limit and strip pagination from single-page response", func() {
			allTools := custom.NewAllTools(fake)
			var listWorkflows = allTools[1]

			result, err := listWorkflows.Execute(context.Background(),
				json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())

			_, ok := fake.listWorkflowsParams.Offset.Get()
			Expect(ok).To(BeFalse(), "Offset should be unset when no cursor provided")
			_, ok = fake.listWorkflowsParams.Limit.Get()
			Expect(ok).To(BeFalse(), "Limit should be unset when no cursor provided")

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed).NotTo(HaveKey("pagination"), "single-page response should have pagination stripped")
			Expect(parsed).To(HaveKey("workflows"))
		})
	})

	Describe("UT-KA-688-302: list_workflows Execute with page=next and cursor", func() {
		It("should decode cursor and pass offset/limit to DS", func() {
			fake.listWorkflowsResponse.Pagination = multiPagePagination
			fake.listWorkflowsResponse.Pagination.Offset = 10
			fake.listWorkflowsResponse.Pagination.HasMore = true

			cursor := custom.EncodeCursor(10, 10)
			allTools := custom.NewAllTools(fake)
			var listWorkflows = allTools[1]

			args := fmt.Sprintf(`{"action_type":"ScaleReplicas","page":"next","cursor":"%s"}`, cursor)
			result, err := listWorkflows.Execute(context.Background(), json.RawMessage(args))
			Expect(err).NotTo(HaveOccurred())

			gotOffset, ok := fake.listWorkflowsParams.Offset.Get()
			Expect(ok).To(BeTrue(), "Offset should be set when cursor provided")
			Expect(gotOffset).To(Equal(10))

			gotLimit, ok := fake.listWorkflowsParams.Limit.Get()
			Expect(ok).To(BeTrue(), "Limit should be set when cursor provided")
			Expect(gotLimit).To(Equal(10))

			var parsed map[string]json.RawMessage
			Expect(json.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("pagination"), "multi-page response should have pagination")

			var pag map[string]interface{}
			Expect(json.Unmarshal(parsed["pagination"], &pag)).To(Succeed())
			Expect(pag["hasNext"]).To(BeTrue())
			Expect(pag).To(HaveKey("nextCursor"))
			Expect(pag["hasPrevious"]).To(BeTrue())
			Expect(pag).To(HaveKey("previousCursor"))
		})
	})

	Describe("UT-KA-688-303: list_available_actions Execute with no pagination args", func() {
		It("should call DS with unset Offset/Limit and strip pagination from single-page response", func() {
			allTools := custom.NewAllTools(fake)
			var listActions = allTools[0]

			result, err := listActions.Execute(context.Background(),
				json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			_, ok := fake.listActionsParams.Offset.Get()
			Expect(ok).To(BeFalse(), "Offset should be unset when no cursor provided")
			_, ok = fake.listActionsParams.Limit.Get()
			Expect(ok).To(BeFalse(), "Limit should be unset when no cursor provided")

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed).NotTo(HaveKey("pagination"), "single-page response should have pagination stripped")
			Expect(parsed).To(HaveKey("actionTypes"))
		})
	})

	Describe("UT-KA-688-304: list_available_actions Execute with page=next and cursor", func() {
		It("should decode cursor and pass offset/limit to DS", func() {
			fake.listActionsResponse.Pagination = multiPagePagination

			cursor := custom.EncodeCursor(10, 10)
			allTools := custom.NewAllTools(fake)
			var listActions = allTools[0]

			args := fmt.Sprintf(`{"page":"next","cursor":"%s"}`, cursor)
			result, err := listActions.Execute(context.Background(), json.RawMessage(args))
			Expect(err).NotTo(HaveOccurred())

			gotOffset, ok := fake.listActionsParams.Offset.Get()
			Expect(ok).To(BeTrue(), "Offset should be set when cursor provided")
			Expect(gotOffset).To(Equal(10))

			gotLimit, ok := fake.listActionsParams.Limit.Get()
			Expect(ok).To(BeTrue(), "Limit should be set when cursor provided")
			Expect(gotLimit).To(Equal(10))

			var parsed map[string]json.RawMessage
			Expect(json.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("pagination"))
		})
	})

	Describe("UT-KA-688-305: Execute with invalid cursor falls back gracefully", func() {
		It("should not error and should call DS with default offset/limit", func() {
			allTools := custom.NewAllTools(fake)
			var listWorkflows = allTools[1]

			result, err := listWorkflows.Execute(context.Background(),
				json.RawMessage(`{"action_type":"ScaleReplicas","page":"next","cursor":"garbage"}`))
			Expect(err).NotTo(HaveOccurred())

			gotOffset, ok := fake.listWorkflowsParams.Offset.Get()
			Expect(ok).To(BeTrue(), "Offset should be set (decoded from invalid cursor with fallback)")
			Expect(gotOffset).To(Equal(0))

			gotLimit, ok := fake.listWorkflowsParams.Limit.Get()
			Expect(ok).To(BeTrue(), "Limit should be set (decoded from invalid cursor with fallback)")
			Expect(gotLimit).To(Equal(10))

			Expect(result).NotTo(BeEmpty())
		})
	})
})
