/*
Copyright 2025 The KServe Authors.

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

package logger

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
)

// logRecord is a flattened tabular representation of LogRequest used by
// CSV and Parquet marshallers. All complex types (URL, bytes, maps) are
// converted to string representations for tabular storage.
type logRecord struct {
	Url              string `parquet:"url"              csv:"url"`
	Bytes            string `parquet:"bytes"            csv:"bytes"`
	ContentType      string `parquet:"contentType"      csv:"contentType"`
	ReqType          string `parquet:"reqType"          csv:"reqType"`
	Id               string `parquet:"id"               csv:"id"`
	SourceUri        string `parquet:"sourceUri"        csv:"sourceUri"`
	InferenceService string `parquet:"inferenceService" csv:"inferenceService"`
	Namespace        string `parquet:"namespace"        csv:"namespace"`
	Component        string `parquet:"component"        csv:"component"`
	Endpoint         string `parquet:"endpoint"         csv:"endpoint"`
	Metadata         string `parquet:"metadata"         csv:"metadata"`
	Annotations      string `parquet:"annotations"      csv:"annotations"`
	CertName         string `parquet:"certName"         csv:"certName"`
	TlsSkipVerify    bool   `parquet:"tlsSkipVerify"    csv:"tlsSkipVerify"`
}

// logRecordColumns returns the CSV header row as a slice of column names
// in the same order as the logRecord struct fields.
func logRecordColumns() []string {
	return []string{
		"url",
		"bytes",
		"contentType",
		"reqType",
		"id",
		"sourceUri",
		"inferenceService",
		"namespace",
		"component",
		"endpoint",
		"metadata",
		"annotations",
		"certName",
		"tlsSkipVerify",
	}
}

// toLogRecord converts a LogRequest to a logRecord with the following rules:
// - Url, SourceUri: url.URL.String(), empty string if nil
// - Bytes: base64 standard encoding, empty string if nil
// - Metadata: json.Marshal(map), empty string if nil
// - Annotations: json.Marshal(map), empty string if nil
// - All other string fields: direct copy
// - TlsSkipVerify: direct copy
func toLogRecord(req LogRequest) logRecord {
	record := logRecord{
		ContentType:      req.ContentType,
		ReqType:          req.ReqType,
		Id:               req.Id,
		InferenceService: req.InferenceService,
		Namespace:        req.Namespace,
		Component:        req.Component,
		Endpoint:         req.Endpoint,
		CertName:         req.CertName,
		TlsSkipVerify:    req.TlsSkipVerify,
	}

	// Convert URL to string
	if req.Url != nil {
		record.Url = req.Url.String()
	}

	// Convert SourceUri to string
	if req.SourceUri != nil {
		record.SourceUri = req.SourceUri.String()
	}

	// Convert Bytes to base64 string
	if req.Bytes != nil {
		record.Bytes = base64.StdEncoding.EncodeToString(*req.Bytes)
	}

	// Convert Metadata to JSON string
	if req.Metadata != nil {
		if jsonBytes, err := json.Marshal(req.Metadata); err == nil {
			record.Metadata = string(jsonBytes)
		}
	}

	// Convert Annotations to JSON string
	if req.Annotations != nil {
		if jsonBytes, err := json.Marshal(req.Annotations); err == nil {
			record.Annotations = string(jsonBytes)
		}
	}

	return record
}

// logRecordToStrings converts a logRecord to a slice of strings for CSV writing.
// The order matches logRecordColumns(). Boolean values are represented as "true" or "false".
func logRecordToStrings(record logRecord) []string {
	return []string{
		record.Url,
		record.Bytes,
		record.ContentType,
		record.ReqType,
		record.Id,
		record.SourceUri,
		record.InferenceService,
		record.Namespace,
		record.Component,
		record.Endpoint,
		record.Metadata,
		record.Annotations,
		record.CertName,
		strconv.FormatBool(record.TlsSkipVerify),
	}
}
