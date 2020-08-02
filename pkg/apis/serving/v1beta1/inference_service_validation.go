/*
Copyright 2020 kubeflow.org.

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

package v1beta1

import "sigs.k8s.io/controller-runtime/pkg/client"

// Known error messages
const (
	MinReplicasShouldBeLessThanMaxError = "MinReplicas cannot be greater than MaxReplicas."
	MinReplicasLowerBoundExceededError  = "MinReplicas cannot be less than 0."
	MaxReplicasLowerBoundExceededError  = "MaxReplicas cannot be less than 0."
	ParallelismLowerBoundExceededError  = "Parallelism cannot be less than 0."
	UnsupportedStorageURIFormatError    = "storageUri, must be one of: [%s] or match https://{}.blob.core.windows.net/{}/{} or be an absolute or relative local path. StorageUri [%s] is not supported."
	InvalidLoggerType                   = "Invalid logger type"
)

var (
	SupportedStorageURIPrefixList = []string{"gs://", "s3://", "pvc://", "file://"}
	AzureBlobURIRegEx             = "https://(.+?).blob.core.windows.net/(.+)"
)

// Validate the resource
func (i *InferenceService) Validate(client client.Client) {
	//i.GetPredictor().Validate()
}
