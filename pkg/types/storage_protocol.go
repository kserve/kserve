/*
Copyright 2026 The KServe Authors.

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

package types

// StorageProtocol is a storage URI prefix the model agent can download from.
// It lives here rather than in pkg/agent/storage so that pkg/apis can validate
// TrainedModel storage URIs without importing the cloud storage SDKs.
type StorageProtocol string

const (
	S3StorageProtocol    StorageProtocol = "s3://"
	GCSStorageProtocol   StorageProtocol = "gs://"
	AzureStorageProtocol StorageProtocol = "abfs://"
	// PVC   StorageProtocol = "pvc://"
	// File  StorageProtocol = "file://"
	HTTPSStorageProtocol StorageProtocol = "https://"
	HTTPStorageProtocol  StorageProtocol = "http://"
)

var SupportedStorageProtocols = []StorageProtocol{S3StorageProtocol, GCSStorageProtocol, HTTPSStorageProtocol, HTTPStorageProtocol}

func GetAllStorageProtocols() (protocols []string) {
	for _, protocol := range SupportedStorageProtocols {
		protocols = append(protocols, string(protocol))
	}
	return protocols
}
