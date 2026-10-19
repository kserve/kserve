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

package v1beta1

import "testing"

func TestValidateStorageURISpecRejectsInternalHTTP(t *testing.T) {
	storageURI := StorageUri{Uri: "http://169.254.169.254/latest/meta-data", MountPath: "/mnt/models"}
	if err := validateStorageURISpec(&storageURI); err == nil {
		t.Fatal("expected metadata storage URI to be rejected")
	}
}

func TestValidateMultipleStorageURIsRejectsLegacyInternalHTTP(t *testing.T) {
	storageURI := "https://kubernetes.default.svc/api"
	isvc := &InferenceService{Spec: InferenceServiceSpec{Predictor: PredictorSpec{
		Model: &ModelSpec{PredictorExtensionSpec: PredictorExtensionSpec{StorageURI: &storageURI}},
	}}}
	if err := validateMultipleStorageURIs(isvc); err == nil {
		t.Fatal("expected Kubernetes API storage URI to be rejected")
	}
}
