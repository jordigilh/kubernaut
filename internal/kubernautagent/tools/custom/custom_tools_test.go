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

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/workflowcatalog"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// toolCtx returns a context with a representative SignalContext attached.
// Required since #779: workflow discovery tools extract signal fields from ctx.
func toolCtx() context.Context {
	return katypes.WithSignalContext(context.Background(), katypes.SignalContext{
		Severity:     "critical",
		ResourceKind: "Deployment",
		Environment:  "production",
		Priority:     "P0",
	})
}

// fakeWorkflowDS captures the args passed to each Catalog method and returns
// canned responses. Satisfies custom.WorkflowCatalog (Issue #1677 Phase 2d:
// replaces the former ogen-client-shaped fake with one matching
// workflowcatalog.Catalog's Go-native signatures).
type fakeWorkflowDS struct {
	listActionsFilters *models.WorkflowDiscoveryFilters
	listActionsOffset  int
	listActionsLimit   int
	listActionsEntries []models.ActionTypeEntry
	listActionsTotal   int
	listActionsErr     error

	listWorkflowsActionType string
	listWorkflowsFilters    *models.WorkflowDiscoveryFilters
	listWorkflowsOffset     int
	listWorkflowsLimit      int
	listWorkflowsEntries    []models.RemediationWorkflow
	listScoredWorkflows     []workflowcatalog.ScoredWorkflow
	listWorkflowsTotal      int
	listWorkflowsErr        error

	getWorkflowID      string
	getWorkflowFilters *models.WorkflowDiscoveryFilters
	getWorkflowResult  *models.RemediationWorkflow
	getWorkflowErr     error
}

func (f *fakeWorkflowDS) ListActions(_ context.Context, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.ActionTypeEntry, int, error) {
	f.listActionsFilters = filters
	f.listActionsOffset = offset
	f.listActionsLimit = limit
	return f.listActionsEntries, f.listActionsTotal, f.listActionsErr
}

func (f *fakeWorkflowDS) ListWorkflowsByActionType(_ context.Context, actionType string, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.RemediationWorkflow, int, error) {
	f.listWorkflowsActionType = actionType
	f.listWorkflowsFilters = filters
	f.listWorkflowsOffset = offset
	f.listWorkflowsLimit = limit
	return f.listWorkflowsEntries, f.listWorkflowsTotal, f.listWorkflowsErr
}

func (f *fakeWorkflowDS) ListScoredWorkflowsByActionType(_ context.Context, actionType string, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]workflowcatalog.ScoredWorkflow, int, error) {
	f.listWorkflowsActionType = actionType
	f.listWorkflowsFilters = filters
	f.listWorkflowsOffset = offset
	f.listWorkflowsLimit = limit
	if f.listScoredWorkflows != nil {
		return f.listScoredWorkflows, f.listWorkflowsTotal, f.listWorkflowsErr
	}
	candidates := make([]workflowcatalog.ScoredWorkflow, 0, len(f.listWorkflowsEntries))
	for _, workflow := range f.listWorkflowsEntries {
		candidates = append(candidates, workflowcatalog.ScoredWorkflow{Workflow: workflow, FinalScore: 0.5})
	}
	return candidates, f.listWorkflowsTotal, f.listWorkflowsErr
}

func (f *fakeWorkflowDS) GetWorkflowWithContextFilters(_ context.Context, workflowID string, filters *models.WorkflowDiscoveryFilters) (*models.RemediationWorkflow, error) {
	f.getWorkflowID = workflowID
	f.getWorkflowFilters = filters
	if f.getWorkflowErr != nil {
		return nil, f.getWorkflowErr
	}
	if f.getWorkflowResult != nil {
		return f.getWorkflowResult, nil
	}
	return &models.RemediationWorkflow{}, nil
}

// newTestTools builds the 3 discovery tools against fake with no audit store
// (audit emission is exercised separately in discovery_audit_test.go).
func newTestTools(fake custom.WorkflowCatalog) []tools.Tool {
	return custom.NewAllTools(fake, nil, logr.Discard())
}

// Guard: validate tool registration order assumed by allTools[0]/[1]/[2] indexing across all test files.
var _ = Describe("NewAllTools registration order guard", func() {
	It("should return tools in the expected positional order", func() {
		allTools := newTestTools(&fakeWorkflowDS{})
		Expect(allTools).To(HaveLen(3))
		Expect(allTools[0].Name()).To(Equal("list_available_actions"))
		Expect(allTools[1].Name()).To(Equal("list_workflows"))
		Expect(allTools[2].Name()).To(Equal("get_workflow"))
	})
})

var _ = Describe("list_available_actions ranked evidence — #2478", func() {
	It("UT-KA-2478-004: renders advisory rank and detected-label evidence without exposing raw score", func() {
		fake := &fakeWorkflowDS{
			listActionsEntries: []models.ActionTypeEntry{
				{
					ActionType:    "HelmRollback",
					WorkflowCount: 1,
					Rank:          1,
					Preferred:     true,
					MatchedDetectedLabels: &models.DetectedLabels{
						HelmManaged: true,
					},
					PreferenceReason: "detected-label match for helmManaged=true",
					BestMatchScore:   0.502,
				},
				{
					ActionType:       "PatchConfiguration",
					WorkflowCount:    3,
					Rank:             2,
					BestMatchScore:   0.5,
					PreferenceReason: "highest-scoring matching workflow",
				},
			},
			listActionsTotal: 2,
		}

		result, err := newTestTools(fake)[0].Execute(toolCtx(), json.RawMessage(`{}`))
		Expect(err).NotTo(HaveOccurred())

		var response struct {
			ActionTypes []map[string]interface{} `json:"actionTypes"`
		}
		Expect(json.Unmarshal([]byte(result), &response)).To(Succeed())
		Expect(response.ActionTypes).To(HaveLen(2))
		Expect(response.ActionTypes[0]).To(HaveKeyWithValue("actionType", "HelmRollback"))
		Expect(response.ActionTypes[0]).To(HaveKeyWithValue("rank", BeNumerically("==", 1)))
		Expect(response.ActionTypes[0]).To(HaveKeyWithValue("preferred", true))
		Expect(response.ActionTypes[0]).To(HaveKey("matchedDetectedLabels"))
		Expect(response.ActionTypes[0]).To(HaveKeyWithValue("preferenceReason", "detected-label match for helmManaged=true"))
		Expect(response.ActionTypes[0]).NotTo(HaveKey("bestMatchScore"),
			"numeric ranking scores remain audit-only per DD-WORKFLOW-016")
		Expect(response.ActionTypes[0]).NotTo(HaveKey("bestWorkflowID"),
			"best workflow identity remains audit-only per DD-WORKFLOW-016")
		Expect(response.ActionTypes[1]).To(HaveKeyWithValue("actionType", "PatchConfiguration"),
			"generic action families must remain available")
		Expect(response.ActionTypes[1]).To(HaveKeyWithValue("preferred", false))
		Expect(response.ActionTypes[1]).To(HaveKeyWithValue("matchedDetectedLabels", BeNil()))
		Expect(response.ActionTypes[1]).To(HaveKeyWithValue("preferenceReason", "highest-scoring matching workflow"))
	})
})

// UT-KA-688-001/002/003 (stripPaginationIfComplete) were removed as dead-code
// coverage (#1677 dead-code sweep, follow-up): stripPaginationIfComplete
// itself was deleted -- superseded by TransformPagination (DD-WORKFLOW-016
// v1.4, see UT-KA-688-110 through 115 below), which is a strict superset and
// is the only pagination-shaping function called from production code.

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

// --- Group C: Schema Validation (UT-KA-433-170 through UT-KA-688-202) ---

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
			Expect(ok).To(BeTrue(), `expected "required" in list_workflows schema to be a []interface{}`)
			Expect(required).To(ContainElement("action_type"))
		})

		It("should not expose offset/limit (DD-WORKFLOW-016 v1.4: cursor replaces raw offset)", func() {
			schema := custom.ListWorkflowsSchema()

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())

			props, ok := parsed["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue(), `expected "properties" in list_workflows schema to be a map[string]interface{}`)
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
			Expect(ok).To(BeTrue(), `expected "required" in get_workflow schema to be a []interface{}`)
			Expect(required).To(ContainElement("workflow_id"))
		})
	})

	Describe("UT-KA-433-173: All existing custom tools return non-nil Parameters()", func() {
		It("should have non-nil schemas for all 3 workflow discovery tools", func() {
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
			Expect(ok).To(BeTrue(), `expected "properties" in list_workflows schema to be a map[string]interface{}`)

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
			Expect(ok).To(BeTrue(), `expected "properties" in list_available_actions schema to be a map[string]interface{}`)

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

	BeforeEach(func() {
		fake = &fakeWorkflowDS{
			listActionsEntries: []models.ActionTypeEntry{
				{ActionType: "ScaleReplicas", Description: models.ActionTypeDescription{What: "test", WhenToUse: "test"}, WorkflowCount: 1},
			},
			listActionsTotal: 2,
			listWorkflowsEntries: []models.RemediationWorkflow{
				{WorkflowID: uuid.New().String(), WorkflowName: "scale-conservative-v1", Name: "Scale Conservative", Description: models.StructuredDescription{What: "test", WhenToUse: "test"}},
			},
			listWorkflowsTotal: 2,
		}
	})

	Describe("UT-KA-688-301: list_workflows Execute with no pagination args", func() {
		It("should call the catalog with default offset/limit and strip pagination from single-page response", func() {
			fake.listWorkflowsTotal = 1
			allTools := newTestTools(fake)
			listWorkflows := allTools[1]

			result, err := listWorkflows.Execute(toolCtx(),
				json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listWorkflowsOffset).To(Equal(0), "Offset should default to 0 when no cursor provided")
			Expect(fake.listWorkflowsLimit).To(Equal(10), "Limit should default to 10 when no cursor provided")

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed).NotTo(HaveKey("pagination"), "single-page response should have pagination stripped")
			Expect(parsed).To(HaveKey("workflows"))
		})
	})

	Describe("UT-KA-688-302: list_workflows Execute with page=next and cursor", func() {
		It("should decode cursor and pass offset/limit to the catalog", func() {
			fake.listWorkflowsTotal = 30

			cursor := custom.EncodeCursor(10, 10)
			allTools := newTestTools(fake)
			listWorkflows := allTools[1]

			args := fmt.Sprintf(`{"action_type":"ScaleReplicas","page":"next","cursor":"%s"}`, cursor)
			result, err := listWorkflows.Execute(toolCtx(), json.RawMessage(args))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listWorkflowsOffset).To(Equal(10), "Offset should be decoded from cursor")
			Expect(fake.listWorkflowsLimit).To(Equal(10), "Limit should be decoded from cursor")

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
		It("should call the catalog with default offset/limit and strip pagination from single-page response", func() {
			fake.listActionsTotal = 1
			allTools := newTestTools(fake)
			listActions := allTools[0]

			result, err := listActions.Execute(toolCtx(),
				json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listActionsOffset).To(Equal(0), "Offset should default to 0 when no cursor provided")
			Expect(fake.listActionsLimit).To(Equal(10), "Limit should default to 10 when no cursor provided")

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed).NotTo(HaveKey("pagination"), "single-page response should have pagination stripped")
			Expect(parsed).To(HaveKey("actionTypes"))
		})
	})

	Describe("UT-KA-688-304: list_available_actions Execute with page=next and cursor", func() {
		It("should decode cursor and pass offset/limit to the catalog", func() {
			fake.listActionsTotal = 16

			cursor := custom.EncodeCursor(10, 10)
			allTools := newTestTools(fake)
			listActions := allTools[0]

			args := fmt.Sprintf(`{"page":"next","cursor":"%s"}`, cursor)
			result, err := listActions.Execute(toolCtx(), json.RawMessage(args))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listActionsOffset).To(Equal(10), "Offset should be decoded from cursor")
			Expect(fake.listActionsLimit).To(Equal(10), "Limit should be decoded from cursor")

			var parsed map[string]json.RawMessage
			Expect(json.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("pagination"))
		})
	})

	Describe("UT-KA-688-305: Execute with invalid cursor falls back gracefully", func() {
		It("should not error and should call the catalog with default offset/limit", func() {
			allTools := newTestTools(fake)
			listWorkflows := allTools[1]

			result, err := listWorkflows.Execute(toolCtx(),
				json.RawMessage(`{"action_type":"ScaleReplicas","page":"next","cursor":"garbage"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listWorkflowsOffset).To(Equal(0), "Offset should default to 0 (decoded from invalid cursor with fallback)")
			Expect(fake.listWorkflowsLimit).To(Equal(10), "Limit should default to 10 (decoded from invalid cursor with fallback)")

			Expect(result).NotTo(BeEmpty())
		})
	})

	Describe("UT-KA-2466: LLM-facing workflow projection", func() {
		It("UT-KA-2466-001: list_workflows includes candidate identity and description but no execution metadata", func() {
			bundle := "registry.example/workflows/oom@sha256:1234"
			schemaImage := "registry.example/workflows/oom-schema:v1"
			serviceAccount := "workflow-runner"
			fake := &fakeWorkflowDS{
				listWorkflowsTotal: 1,
				listWorkflowsEntries: []models.RemediationWorkflow{{
					WorkflowID:         uuid.NewString(),
					WorkflowName:       "oom-increase-memory-v1",
					Name:               "OOM Increase Memory",
					Version:            "1.0.0",
					Description:        models.StructuredDescription{What: "Increase a pod memory limit", WhenToUse: "OOMKilled due to insufficient memory"},
					ExecutionEngine:    models.ExecutionEngine("tekton"),
					ExecutionBundle:    &bundle,
					SchemaImage:        &schemaImage,
					ServiceAccountName: &serviceAccount,
				}},
			}

			result, err := newTestTools(fake)[1].Execute(toolCtx(), json.RawMessage(`{"action_type":"IncreaseMemoryLimits"}`))
			Expect(err).NotTo(HaveOccurred())

			var response struct {
				Workflows []map[string]json.RawMessage `json:"workflows"`
			}
			Expect(json.Unmarshal([]byte(result), &response)).To(Succeed())
			Expect(response.Workflows).To(HaveLen(1))
			candidate := response.Workflows[0]
			Expect(candidate).To(HaveKey("workflowId"))
			Expect(candidate).To(HaveKey("workflowName"))
			Expect(candidate).To(HaveKey("name"))
			Expect(candidate).To(HaveKey("version"))
			Expect(candidate).To(HaveKey("description"))
			Expect(candidate).NotTo(HaveKey("schemaImage"))
			Expect(candidate).NotTo(HaveKey("executionBundle"))
			Expect(candidate).NotTo(HaveKey("executionEngine"))
			Expect(candidate).NotTo(HaveKey("serviceAccountName"))
			Expect(result).NotTo(ContainSubstring(bundle))
			Expect(result).NotTo(ContainSubstring(schemaImage))
			Expect(result).NotTo(ContainSubstring(serviceAccount))
		})

		It("UT-KA-2466-002: get_workflow exposes only the description and operational parameter schema", func() {
			workflowID := uuid.NewString()
			bundle := "registry.example/workflows/oom@sha256:5678"
			schemaImage := "registry.example/workflows/oom-schema:v1"
			serviceAccount := "workflow-runner"
			engineConfig := json.RawMessage(`{"playbookPath":"private-playbook.yml"}`)
			content := `{"metadata":{"annotations":{"test.kubernaut.ai/audit-sentinel":"synthetic-sensitive-audit-sentinel-2459"}},"spec":{"execution":{"engine":"ansible","bundle":"registry.example/workflows/oom@sha256:5678"},"dependencies":{"secrets":[{"name":"git-creds"}]},"serviceAccountName":"workflow-runner"}}`
			parameters := json.RawMessage(`{"schema":{"parameters":[
				{"name":"TARGET_RESOURCE_NAME","type":"string","required":true,"description":"KA injects this"},
				{"name":"TARGET_RESOURCE_KIND","type":"string","required":true,"description":"KA injects this"},
				{"name":"TARGET_RESOURCE_NAMESPACE","type":"string","required":true,"description":"KA injects this"},
				{"name":"TARGET_RESOURCE_API_VERSION","type":"string","required":false,"description":"KA injects this"},
				{"name":"MEMORY_LIMIT_BASELINE","type":"string","required":true,"description":"Current pod memory limit"},
				{"name":"MEMORY_LIMIT_NEW","type":"string","required":true,"description":"New pod memory limit","enum":["512Mi","1Gi"],"pattern":"^[0-9]+(Mi|Gi)$","default":"512Mi","dependsOn":["TARGET_RESOURCE_KIND","MEMORY_LIMIT_BASELINE"]},
				{"name":"RESTART_AFTER_UPDATE","type":"boolean","required":false,"description":"Restart the workload after updating memory","dependsOn":["TARGET_RESOURCE_NAME"]}
			]}}`)
			fake := &fakeWorkflowDS{getWorkflowResult: &models.RemediationWorkflow{
				WorkflowID:         workflowID,
				WorkflowName:       "oom-increase-memory-v1",
				Name:               "OOM Increase Memory",
				Version:            "1.0.0",
				Description:        models.StructuredDescription{What: "Increase a pod memory limit", WhenToUse: "OOMKilled due to insufficient memory"},
				Content:            content,
				ContentHash:        "sensitive-content-hash",
				ActionType:         "IncreaseMemoryLimits",
				Parameters:         &parameters,
				ExecutionEngine:    models.ExecutionEngine("tekton"),
				SchemaImage:        &schemaImage,
				ExecutionBundle:    &bundle,
				EngineConfig:       &engineConfig,
				ServiceAccountName: &serviceAccount,
			}}

			result, err := newTestTools(fake)[2].Execute(toolCtx(), json.RawMessage(fmt.Sprintf(`{"workflow_id":%q}`, workflowID)))
			Expect(err).NotTo(HaveOccurred())

			var response map[string]json.RawMessage
			Expect(json.Unmarshal([]byte(result), &response)).To(Succeed())
			Expect(response).To(HaveKey("description"))
			Expect(response).To(HaveKey("parameters"))
			Expect(response).To(HaveLen(2), "get_workflow should return only the selection description and permitted parameter schema")
			Expect(result).NotTo(ContainSubstring("TARGET_RESOURCE_NAME"))
			Expect(result).NotTo(ContainSubstring("TARGET_RESOURCE_KIND"))
			Expect(result).NotTo(ContainSubstring("TARGET_RESOURCE_NAMESPACE"))
			Expect(result).NotTo(ContainSubstring("TARGET_RESOURCE_API_VERSION"))
			Expect(result).NotTo(ContainSubstring("synthetic-sensitive-audit-sentinel-2459"),
				"raw CRD content outside the allowed projection must stay hidden")
			Expect(result).NotTo(ContainSubstring("executionEngine"))
			Expect(result).NotTo(ContainSubstring("executionBundle"))
			Expect(result).NotTo(ContainSubstring("serviceAccountName"))
			Expect(result).NotTo(ContainSubstring("dependencies"))
			Expect(result).NotTo(ContainSubstring("contentHash"))
			Expect(result).NotTo(ContainSubstring("private-playbook.yml"))
			Expect(result).NotTo(ContainSubstring("git-creds"))

			var parameterResponse struct {
				Schema struct {
					Parameters []map[string]json.RawMessage `json:"parameters"`
				} `json:"schema"`
			}
			Expect(json.Unmarshal(response["parameters"], &parameterResponse)).To(Succeed())
			Expect(parameterResponse.Schema.Parameters).To(HaveLen(3))
			Expect(parameterResponse.Schema.Parameters[0]).To(HaveKeyWithValue("name", json.RawMessage(`"MEMORY_LIMIT_BASELINE"`)))
			Expect(parameterResponse.Schema.Parameters[1]).To(HaveKeyWithValue("name", json.RawMessage(`"MEMORY_LIMIT_NEW"`)))
			Expect(parameterResponse.Schema.Parameters[1]).To(HaveKey("type"))
			Expect(parameterResponse.Schema.Parameters[1]).To(HaveKey("required"))
			Expect(parameterResponse.Schema.Parameters[1]).To(HaveKey("description"))
			Expect(parameterResponse.Schema.Parameters[1]).To(HaveKey("enum"))
			Expect(parameterResponse.Schema.Parameters[1]).To(HaveKey("pattern"))
			Expect(parameterResponse.Schema.Parameters[1]).To(HaveKey("default"))
			Expect(parameterResponse.Schema.Parameters[1]).To(HaveKey("dependsOn"))
			var dependsOn []string
			Expect(json.Unmarshal(parameterResponse.Schema.Parameters[1]["dependsOn"], &dependsOn)).To(Succeed())
			Expect(dependsOn).To(Equal([]string{"MEMORY_LIMIT_BASELINE"}))
			Expect(parameterResponse.Schema.Parameters[2]).To(HaveKeyWithValue("name", json.RawMessage(`"RESTART_AFTER_UPDATE"`)))
			Expect(parameterResponse.Schema.Parameters[2]).NotTo(HaveKey("dependsOn"),
				"a reference to a hidden KA-managed target parameter must not leak through an operational definition")
			Expect(*fake.getWorkflowResult.Parameters).To(Equal(parameters), "LLM projection must not mutate the full Catalog record")
		})

		It("UT-KA-2466-004: fails closed when the stored parameter schema is malformed", func() {
			workflowID := uuid.NewString()
			invalidSchemas := []json.RawMessage{
				json.RawMessage(`{"schema":`),
				json.RawMessage(`{"schema":{}}`),
				json.RawMessage(`{"schema":{"parameters":[{"type":"string"}]}}`),
				json.RawMessage(`{"schema":{"parameters":[{"name":"MEMORY_LIMIT_NEW","dependsOn":"TARGET_RESOURCE_NAME"}]}}`),
			}
			for _, invalidSchema := range invalidSchemas {
				fake := &fakeWorkflowDS{getWorkflowResult: &models.RemediationWorkflow{
					WorkflowID: workflowID,
					Parameters: &invalidSchema,
				}}
				result, err := newTestTools(fake)[2].Execute(toolCtx(), json.RawMessage(fmt.Sprintf(`{"workflow_id":%q}`, workflowID)))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("projecting workflow parameters for LLM"))
				Expect(result).To(BeEmpty(), "malformed schema content must never be returned as a fallback")
			}
		})
	})
})
