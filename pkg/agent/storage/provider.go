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

package storage

import "github.com/kserve/kserve/pkg/types"

type Provider interface {
	DownloadModel(modelDir string, modelName string, storageUri string) error
	UploadObject(bucket string, key string, object []byte) error
}

type Protocol = types.StorageProtocol

const (
	S3    = types.S3StorageProtocol
	GCS   = types.GCSStorageProtocol
	AZURE = types.AzureStorageProtocol
	HTTPS = types.HTTPSStorageProtocol
	HTTP  = types.HTTPStorageProtocol
)

var SupportedProtocols = types.SupportedStorageProtocols
