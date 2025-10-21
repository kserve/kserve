package marshaller

import (
	"encoding/json"
	"testing"

	"github.com/kserve/kserve/pkg/logger/types"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

func TestJsonMarshalling(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	marshaller := JSONMarshaller{}

	bytes, err := marshaller.Marshal([]types.LogRequest{
		{
			Id:               "0123",
			Namespace:        "ns",
			InferenceService: "inference",
			Component:        "predictor",
			ReqType:          CEInferenceRequest,
		},
		{
			Id:               "1234",
			Namespace:        "ns",
			InferenceService: "inference",
			Component:        "predictor",
			ReqType:          CEInferenceRequest,
		},
	})
	g.Expect(err).ToNot(gomega.HaveOccurred())
	assert.Greater(t, len(bytes), 0, "marshalled byte array is empty")
	result := make([]types.LogRequest, 0)
	err = json.Unmarshal(bytes, &result)
	if err != nil {
		assert.Fail(t, err.Error())
	}
	g.Expect(len(result)).To(gomega.Equal(2))
	g.Expect(result[0].Id).To(gomega.Equal("0123"))
	g.Expect(result[1].Id).To(gomega.Equal("1234"))
}
