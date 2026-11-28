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

package pod

import (
	"testing"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

func TestHasHTTPStorageURI(t *testing.T) {
	for _, tt := range []struct {
		name string
		uris []v1beta1.StorageUri
		want bool
	}{
		{name: "empty", want: false},
		{name: "S3 only", uris: []v1beta1.StorageUri{{Uri: "s3://bucket/model"}}, want: false},
		{name: "HTTP among multiple sources", uris: []v1beta1.StorageUri{{Uri: "s3://bucket/model"}, {Uri: "https://models.example/model"}}, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasHTTPStorageURI(tt.uris); got != tt.want {
				t.Errorf("hasHTTPStorageURI() = %t, want %t", got, tt.want)
			}
		})
	}
}
