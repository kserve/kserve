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
	"bytes"
	"context"
	"testing"

	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/memory"
	"github.com/apache/arrow/go/v17/parquet"
	"github.com/apache/arrow/go/v17/parquet/pqarrow"
	"github.com/kserve/kserve/pkg/logger/types"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

func TestParquetMarshalling(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	marshaller := ParquetMarshaller{}

	logRequests := []types.LogRequest{
		{
			Id:               "0123",
			Namespace:        "ns",
			InferenceService: "inference",
			Component:        "predictor",
			ReqType:          "REQUEST",
		},
		{
			Id:               "1234",
			Namespace:        "ns",
			InferenceService: "inference",
			Component:        "predictor",
			ReqType:          "REQUEST",
		},
	}

	marshalledBytes, err := marshaller.Marshal(logRequests)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	assert.NotEmpty(t, marshalledBytes, "marshalled byte array is empty")

	bytesReader := bytes.NewReader(marshalledBytes)
	alloc := memory.DefaultAllocator
	props := parquet.NewReaderProperties(alloc)
	arrProps := pqarrow.ArrowReadProperties{}
	tbl, err := pqarrow.ReadTable(context.Background(), bytesReader, props, arrProps, alloc)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	defer tbl.Release()

	// 3. Create a TableReader to iterate over the table as records
	// We can use this instead of the file-based RecordReader
	rr := array.NewTableReader(tbl, -1)
	defer rr.Release()

	items := make([]types.LogRequest, 0, len(logRequests))
	schema := rr.Schema() // Get schema to find column indices by name
	indices := schema.FieldIndices("id")
	g.Expect(indices).To(gomega.HaveLen(1), "Expected one index for field 'id'")
	idIdx := indices[0]

	totalRows := int64(0)
	for rr.Next() {
		rec := rr.Record()
		// We don't need to release rec, as it's owned by the TableReader
		totalRows += rec.NumRows()

		idCol := rec.Column(idIdx).(*array.String)
		g.Expect(idCol).ToNot(gomega.BeNil())

		for i := 0; i < int(rec.NumRows()); i++ {
			val := idCol.Value(i)
			g.Expect(val).To(gomega.Equal(logRequests[i].Id))
		}
	}

	// Verify the number of rows
	g.Expect(totalRows).To(gomega.Equal(int64(2))) // Use totalRows from iteration
	g.Expect(len(items)).To(gomega.Equal(2))

}
