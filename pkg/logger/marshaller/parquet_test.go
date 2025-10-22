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
	"os"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"

	"github.com/kserve/kserve/pkg/logger/types"
)

func TestParquetMarshalling(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	marshaller := ParquetMarshaller{}
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
	assert.NotEmpty(t, bytes, "marshalled byte array is empty")
	f, err := os.CreateTemp(t.TempDir(), "test.parquet")
	g.Expect(err).ToNot(gomega.HaveOccurred())
	_, err = f.Write(bytes)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	fr, err := local.NewLocalFileReader(f.Name())
	g.Expect(err).ToNot(gomega.HaveOccurred())

	parquetReader, err := reader.NewParquetReader(fr, new(ParquetLogRequest), 1)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(parquetReader).ToNot(gomega.BeNil())
	rows := parquetReader.GetNumRows()
	g.Expect(rows).To(gomega.Equal(int64(2)))
	items := make([]ParquetLogRequest, 2)
	err = parquetReader.Read(&items)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(*items[0].Id).To(gomega.Equal("0123"))
	g.Expect(*items[1].Id).To(gomega.Equal("1234"))
	parquetReader.ReadStop()
	fr.Close()
}
