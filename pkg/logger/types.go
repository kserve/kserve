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

package logger

import (
	"net/url"
)

type LogRequest struct {
	Url              *url.URL            `json:"url,omitempty"`
	Bytes            *[]byte             `json:"bytes,omitempty"`
	ContentType      string              `json:"contentType,omitempty"`
	ReqType          string              `json:"reqType,omitempty"`
	Id               string              `json:"id,omitempty"`
	SourceUri        *url.URL            `json:"sourceUri,omitempty"`
	InferenceService string              `json:"inferenceService,omitempty"`
	Namespace        string              `json:"namespace,omitempty"`
	Component        string              `json:"component,omitempty"`
	Endpoint         string              `json:"endpoint,omitempty"`
	Metadata         map[string][]string `json:"metadata,omitempty"`
	Annotations      map[string]string   `json:"annotations,omitempty"`
	CertName         string              `json:"certName,omitempty"`
	TlsSkipVerify    bool                `json:"tlsSkipVerify,omitempty"`
}
