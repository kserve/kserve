/*
Copyright 2021 The KServe Authors.

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
	"fmt"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/memory"
	"github.com/apache/arrow/go/v17/parquet"
	"github.com/apache/arrow/go/v17/parquet/compress"
	"github.com/apache/arrow/go/v17/parquet/pqarrow"

	"github.com/kserve/kserve/pkg/logger/types"
)

const LogStoreFormatParquet = "parquet"

type ParquetMarshaller struct{}

// buildArrowSchema defines the Arrow schema that matches ParquetLogRequest.
func (p *ParquetMarshaller) buildArrowSchema() *arrow.Schema {
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "url", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "bytes", Type: arrow.BinaryTypes.Binary, Nullable: true},
		{Name: "content_type", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "req_type", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "id", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "source_uri", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "inference_service", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "namespace", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "component", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "endpoint", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "metadata", Type: arrow.MapOf(arrow.BinaryTypes.String, arrow.ListOf(arrow.BinaryTypes.String)), Nullable: true},
		{Name: "annotations", Type: arrow.MapOf(arrow.BinaryTypes.String, arrow.BinaryTypes.String), Nullable: true},
		{Name: "cert_name", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "tls_skip_verify", Type: arrow.FixedWidthTypes.Boolean, Nullable: true},
	}, nil)

	return schema
}

func (p *ParquetMarshaller) Marshal(v []types.LogRequest) ([]byte, error) {
	schema := p.buildArrowSchema()
	pool := memory.NewGoAllocator()
	builder := array.NewRecordBuilder(pool, schema)
	defer builder.Release()
	urlB := builder.Field(0).(*array.StringBuilder)
	bytesB := builder.Field(1).(*array.BinaryBuilder)
	contentTypeB := builder.Field(2).(*array.StringBuilder)
	reqTypeB := builder.Field(3).(*array.StringBuilder)
	idB := builder.Field(4).(*array.StringBuilder)
	sourceUriB := builder.Field(5).(*array.StringBuilder)
	inferenceServiceB := builder.Field(6).(*array.StringBuilder)
	namespaceB := builder.Field(7).(*array.StringBuilder)
	componentB := builder.Field(8).(*array.StringBuilder)
	endpointB := builder.Field(9).(*array.StringBuilder)
	metadataB := builder.Field(10).(*array.MapBuilder)
	annotationsB := builder.Field(11).(*array.MapBuilder)
	certNameB := builder.Field(12).(*array.StringBuilder)
	tlsSkipVerifyB := builder.Field(13).(*array.BooleanBuilder)
	for i := range v {
		req := &v[i]
		if req.Url != nil {
			urlB.Append(req.Url.String())
		} else {
			urlB.AppendNull()
		}
		contentTypeB.Append(req.ContentType)
		reqTypeB.Append(req.ReqType)
		idB.Append(req.Id)
		if req.SourceUri != nil {
			sourceUriB.Append(req.SourceUri.String())
		} else {
			sourceUriB.AppendNull()
		}
		inferenceServiceB.Append(req.InferenceService)
		namespaceB.Append(req.Namespace)
		componentB.Append(req.Component)
		endpointB.Append(req.Endpoint)
		certNameB.Append(req.CertName)

		if req.Bytes != nil {
			bytesB.Append(*req.Bytes)
		} else {
			bytesB.AppendNull()
		}

		if req.Metadata != nil {
			metadataB.Append(true)
			keyBuilder := metadataB.KeyBuilder().(*array.StringBuilder)
			listBuilder := metadataB.ItemBuilder().(*array.ListBuilder)
			valueBuilder := listBuilder.ValueBuilder().(*array.StringBuilder)

			for k, values := range req.Metadata {
				keyBuilder.Append(k)
				if values != nil {
					listBuilder.Append(true)
					for _, val := range values {
						valueBuilder.Append(val)
					}
				} else {
					listBuilder.AppendNull()
				}
			}
		} else {
			metadataB.AppendNull()
		}

		if req.Annotations != nil {
			annotationsB.Append(true)
			keyBuilder := annotationsB.KeyBuilder().(*array.StringBuilder)
			valueBuilder := annotationsB.ItemBuilder().(*array.StringBuilder)
			for k, vStr := range req.Annotations {
				keyBuilder.Append(k)
				valueBuilder.Append(vStr)
			}
		} else {
			annotationsB.AppendNull()
		}

		tlsSkipVerifyB.Append(req.TlsSkipVerify)
	}

	rec := builder.NewRecord()
	defer rec.Release()

	buf := new(bytes.Buffer)
	props := parquet.NewWriterProperties(parquet.WithCompression(compress.Codecs.Snappy))
	arrProps := pqarrow.NewArrowWriterProperties()

	fw, err := pqarrow.NewFileWriter(schema, buf, props, arrProps)
	if err != nil {
		return nil, fmt.Errorf("failed to create parquet writer: %w", err)
	}

	if err := fw.Write(rec); err != nil {
		return nil, fmt.Errorf("failed to write record to parquet: %w", err)
	}
	err = fw.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close parquet writer: %w", err)
	}

	return buf.Bytes(), nil
}

var _ Marshaller = &ParquetMarshaller{}
