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

	"github.com/kserve/kserve/pkg/logger/types"
	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/writer"
)

const LogStoreFormatParquet = "parquet"

type ParquetLogRequest struct {
	Url              *string                           `parquet:"name=url, type=BYTE_ARRAY, convertedtype=UTF8"`
	Bytes            *[]byte                           `parquet:"name=bytes, type=MAP, convertedtype=LIST, valuetype=INT32"`
	ContentType      *string                           `parquet:"name=content_type, type=BYTE_ARRAY, convertedtype=UTF8"`
	ReqType          *string                           `parquet:"name=req_type, type=BYTE_ARRAY, convertedtype=UTF8"`
	Id               *string                           `parquet:"name=id, type=BYTE_ARRAY, convertedtype=UTF8"`
	SourceUri        *string                           `parquet:"name=source_uri, type=BYTE_ARRAY, convertedtype=UTF8"`
	InferenceService *string                           `parquet:"name=inference_service, type=BYTE_ARRAY, convertedtype=UTF8"`
	Namespace        *string                           `parquet:"name=namespace, type=BYTE_ARRAY, convertedtype=UTF8"`
	Component        *string                           `parquet:"name=component, type=BYTE_ARRAY, convertedtype=UTF8"`
	Endpoint         *string                           `parquet:"name=endpoint, type=BYTE_ARRAY, convertedtype=UTF8"`
	Metadata         *map[string]ParquetMetadataValues `parquet:"name=metadata, type=MAP, keytype=BYTE_ARRAY, keyconvertedtype=UTF8, valuetype=LIST, valueconvertedtype=LIST"`
	Annotations      *map[string]string                `parquet:"name=annotations, type=MAP, keytype=BYTE_ARRAY, keyconvertedtype=UTF8, valuetype=BYTE_ARRAY, valueconvertedtype=UTF8"`
	CertName         *string                           `parquet:"name=cert_name, type=BYTE_ARRAY, convertedtype=UTF8"`
	TlsSkipVerify    *bool                             `parquet:"name=tls_skip_verify, type=BOOLEAN"`
}

type ParquetMetadataValues struct {
	Values []string `parquet:"name=metadata_values, type=LIST, valuetype=BYTE_ARRAY, valueconvertedtype=UTF8"`
}

type ParquetLogMetadataValues struct {
	Values []string `parquet:"name=bytes, type=LIST, type=MAP, convertedtype=LIST, valuetype=INT32"`
}

type ParquetMarshaller struct{}

func (p *ParquetMarshaller) Marshal(v []types.LogRequest) ([]byte, error) {
	buffer := bytes.Buffer{}

	pw, err := writer.NewParquetWriterFromWriter(&buffer, new(ParquetLogRequest), 1)
	if err != nil {
		return nil, err
	}
	pw.RowGroupSize = 128 * 1024 * 1024
	pw.CompressionType = parquet.CompressionCodec_SNAPPY

	for _, record := range v {
		parquetRecord := ParquetLogRequest{}
		if record.Url != nil {
			u := record.Url.String()
			parquetRecord.Url = &u
		}
		if record.Bytes != nil {
			parquetRecord.Bytes = record.Bytes
		}
		if record.ContentType != "" {
			parquetRecord.ContentType = &record.ContentType
		}
		if record.ReqType != "" {
			parquetRecord.ReqType = &record.ReqType
		}
		if record.Id != "" {
			parquetRecord.Id = &record.Id
		}
		if record.SourceUri != nil {
			u := record.SourceUri.String()
			parquetRecord.SourceUri = &u
		}
		if record.InferenceService != "" {
			parquetRecord.InferenceService = &record.InferenceService
		}
		if record.Namespace != "" {
			parquetRecord.Namespace = &record.Namespace
		}
		if record.Component != "" {
			parquetRecord.Component = &record.Component
		}
		if record.Endpoint != "" {
			parquetRecord.Endpoint = &record.Endpoint
		}
		if len(record.Metadata) > 0 {
			metadata := make(map[string]ParquetMetadataValues)
			for k, vals := range record.Metadata {
				values := ParquetMetadataValues{
					Values: vals,
				}
				metadata[k] = values
			}
			parquetRecord.Metadata = &metadata
		}
		if len(record.Annotations) > 0 {
			parquetRecord.Annotations = &record.Annotations
		}
		if record.CertName != "" {
			parquetRecord.CertName = &record.CertName
		}
		parquetRecord.TlsSkipVerify = &record.TlsSkipVerify

		if err := pw.Write(&parquetRecord); err != nil {
			return nil, err
		}
	}

	if err := pw.WriteStop(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

var _ Marshaller = &ParquetMarshaller{}
