/*
Copyright 2023 The KServe Authors.

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
