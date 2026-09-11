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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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
			spec:       customSpec,
			storageUri: "https://account.blob.core.windows.net/myblob",
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

func TestClusterStorageContainer_EligibleInitContainerForURI(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	base := ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: "custom"},
		Spec: StorageContainerSpec{
			WorkloadType:        InitContainer,
			SupportedUriFormats: []SupportedUriFormat{{Prefix: "custom://"}},
		},
	}

	t.Run("eligible", func(t *testing.T) {
		g.Expect(base.EligibleInitContainerForURI("custom://models/llama")).To(gomega.Succeed())
	})
	t.Run("disabled", func(t *testing.T) {
		sc := base.DeepCopy()
		sc.Disabled = ptr.To(true)
		err := sc.EligibleInitContainerForURI("custom://models/llama")
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("is disabled"))
	})
	t.Run("wrong workload type", func(t *testing.T) {
		sc := base.DeepCopy()
		sc.Spec.WorkloadType = LocalModelDownloadJob
		err := sc.EligibleInitContainerForURI("custom://models/llama")
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("workloadType"))
	})
	t.Run("unsupported uri", func(t *testing.T) {
		err := base.EligibleInitContainerForURI("s3://bucket/model")
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("does not support storageUri"))
	})
}
