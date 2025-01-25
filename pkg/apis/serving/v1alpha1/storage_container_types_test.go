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

package v1alpha1

import (
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestStorageContainerSpec_IsStorageUriSupported(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	customSpec := StorageContainerSpec{
		Container: corev1.Container{
			Image: "kserve/custom:latest",
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("200Mi"),
				},
			},
		},
		SupportedUriFormats: []SupportedUriFormat{{Prefix: "custom://"}},
	}
	s3AzureSpec := StorageContainerSpec{
		Container: corev1.Container{
			Image: "kserve/storage-initializer:latest",
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("200Mi"),
				},
			},
		},
		SupportedUriFormats: []SupportedUriFormat{{Prefix: "s3://"}, {Regex: "https://(.+?).blob.core.windows.net/(.+)"}},
	}
	cases := []struct {
		name       string
		spec       StorageContainerSpec
		storageUri string
		supported  bool
	}{
		{
			name:       "custom spec supports custom protocol",
			spec:       customSpec,
			storageUri: "custom://abc.com/model.pt",
			supported:  true,
		},
		{
			name:       "s3Azure spec supports Azure",
			spec:       s3AzureSpec,
			storageUri: "https://account.blob.core.windows.net/myblob",
			supported:  true,
		},
		{
			name:       "custom spec does not support Azure",
			spec:       s3AzureSpec,
			storageUri: "https://account.blob.core.windoes.net/myblob",
			supported:  false,
		},
		{
			name:       "s3Azure spec supports s3",
			spec:       s3AzureSpec,
			storageUri: "s3://mybucket/mykey",
			supported:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			supported, err := tc.spec.IsStorageUriSupported(tc.storageUri)
			g.Expect(err).ToNot(gomega.HaveOccurred())
			g.Expect(supported).To(gomega.Equal(tc.supported))
		})
	}
}
