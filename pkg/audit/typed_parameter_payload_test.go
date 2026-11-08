package audit_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("typed DataStorage audit parameter union", func() {
	It("UT-AUD-2575-001: rejects a value whose discriminator and field disagree", func() {
		var value ogenclient.WorkflowParameterValue

		err := json.Unmarshal([]byte(`{"kind":"string","integerValue":9007199254740993}`), &value)

		Expect(err).To(HaveOccurred(), "the generated DataStorage boundary must reject mixed union fields")
	})

	It("UT-AUD-2575-002: accepts a matching string discriminator and value", func() {
		var value ogenclient.WorkflowParameterValue

		err := json.Unmarshal([]byte(`{"kind":"string","stringValue":"healthy"}`), &value)

		Expect(err).NotTo(HaveOccurred())
		Expect(value.Validate()).To(Succeed())
	})

	It("UT-AUD-2575-003: preserves int64 precision and primitive arrays in the generated union", func() {
		values, _, err := sharedtypes.DecodeTypedParameterMap([]byte(`{"large":9007199254740993,"items":["x",0,false]}`))
		Expect(err).NotTo(HaveOccurred())

		converted, err := sharedaudit.ToOgenParameterMap(values)
		Expect(err).NotTo(HaveOccurred())

		large, ok := converted["large"].GetWorkflowParameterInteger()
		Expect(ok).To(BeTrue())
		Expect(large.IntegerValue).To(Equal(int64(9007199254740993)))

		items, ok := converted["items"].GetWorkflowParameterArray()
		Expect(ok).To(BeTrue())
		Expect(items.ArrayValue).To(HaveLen(3))
		item, ok := items.ArrayValue[0].GetWorkflowParameterString()
		Expect(ok).To(BeTrue())
		Expect(item.StringValue).To(Equal("x"))
	})
})
