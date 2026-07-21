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

package llmisvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	kserveTypes "github.com/kserve/kserve/pkg/types"
)

func testBaseStorageConfig() *kserveTypes.StorageInitializerConfig {
	return &kserveTypes.StorageInitializerConfig{
		Image:         "kserve/storage-initializer:latest",
		CpuRequest:    "100m",
		CpuLimit:      "1",
		MemoryRequest: "256Mi",
		MemoryLimit:   "1Gi",
	}
}

func TestLookupStorageContainerSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	nsHfSC := &v1alpha1.StorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: "hf-storage", Namespace: "team-a"},
		Spec: v1alpha1.StorageContainerSpec{
			Container: corev1.Container{
				Name:  "storage-initializer",
				Image: "custom/storage-initializer:hf",
				Env: []corev1.EnvVar{
					{Name: "HF_TOKEN", Value: "secret-token"},
				},
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{
				{Prefix: "hf://"},
			},
			WorkloadType: v1alpha1.InitContainer,
		},
	}

	clusterS3SC := &v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: "s3-storage"},
		Spec: v1alpha1.StorageContainerSpec{
			Container: corev1.Container{
				Name:  "storage-initializer",
				Image: "custom/storage-initializer:s3",
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{
				{Prefix: "s3://"},
			},
			WorkloadType: v1alpha1.InitContainer,
		},
	}

	clusterHfSC := &v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-hf-storage"},
		Spec: v1alpha1.StorageContainerSpec{
			Container: corev1.Container{
				Name:  "storage-initializer",
				Image: "cluster/storage-initializer:hf",
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{
				{Prefix: "hf://"},
			},
			WorkloadType: v1alpha1.InitContainer,
		},
	}

	disabledNsSC := &v1alpha1.StorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: "disabled-hf", Namespace: "team-a"},
		Disabled:   ptr.To(true),
		Spec: v1alpha1.StorageContainerSpec{
			Container: corev1.Container{
				Name:  "storage-initializer",
				Image: "disabled/init:hf",
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{
				{Prefix: "hf://"},
			},
			WorkloadType: v1alpha1.InitContainer,
		},
	}

	downloadJobSC := &v1alpha1.StorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: "download-job-sc", Namespace: "team-a"},
		Spec: v1alpha1.StorageContainerSpec{
			Container: corev1.Container{
				Name:  "storage-initializer",
				Image: "download-job/init:hf",
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{
				{Prefix: "hf://"},
			},
			WorkloadType: v1alpha1.LocalModelDownloadJob,
		},
	}

	tests := []struct {
		name          string
		namespace     string
		storageUri    string
		objects       []runtime.Object
		expectNil     bool
		expectedImage string
	}{
		{
			name:          "namespace SC found for hf URI",
			namespace:     "team-a",
			storageUri:    "hf://meta-llama/Llama-2-7b",
			objects:       []runtime.Object{nsHfSC, clusterS3SC},
			expectedImage: "custom/storage-initializer:hf",
		},
		{
			name:          "namespace SC takes precedence over cluster SC for same prefix",
			namespace:     "team-a",
			storageUri:    "hf://meta-llama/Llama-2-7b",
			objects:       []runtime.Object{nsHfSC, clusterHfSC},
			expectedImage: "custom/storage-initializer:hf",
		},
		{
			name:          "falls back to cluster SC when no namespace match",
			namespace:     "team-a",
			storageUri:    "s3://bucket/model",
			objects:       []runtime.Object{nsHfSC, clusterS3SC},
			expectedImage: "custom/storage-initializer:s3",
		},
		{
			name:          "falls back to cluster SC when namespace has no SC",
			namespace:     "team-b",
			storageUri:    "hf://meta-llama/Llama-2-7b",
			objects:       []runtime.Object{nsHfSC, clusterHfSC},
			expectedImage: "cluster/storage-initializer:hf",
		},
		{
			name:       "no match returns nil",
			namespace:  "team-a",
			storageUri: "gs://bucket/model",
			objects:    []runtime.Object{nsHfSC, clusterS3SC},
			expectNil:  true,
		},
		{
			name:       "empty namespace skips namespace lookup, uses cluster",
			namespace:  "",
			storageUri: "s3://bucket/model",
			objects:    []runtime.Object{clusterS3SC},
			expectedImage: "custom/storage-initializer:s3",
		},
		{
			name:          "disabled namespace SC is skipped, falls through to cluster",
			namespace:     "team-a",
			storageUri:    "hf://meta-llama/Llama-2-7b",
			objects:       []runtime.Object{disabledNsSC, clusterHfSC},
			expectedImage: "cluster/storage-initializer:hf",
		},
		{
			name:          "non-initContainer workloadType SC is skipped",
			namespace:     "team-a",
			storageUri:    "hf://meta-llama/Llama-2-7b",
			objects:       []runtime.Object{downloadJobSC, clusterHfSC},
			expectedImage: "cluster/storage-initializer:hf",
		},
		{
			name:       "no resources at all returns nil",
			namespace:  "team-a",
			storageUri: "hf://meta-llama/Llama-2-7b",
			objects:    nil,
			expectNil:  true,
		},
		{
			name:      "regex-based cluster SC matches URI",
			namespace: "team-a",
			storageUri: "https://mybucket.blob.core.windows.net/models/llama",
			objects: []runtime.Object{
				&v1alpha1.ClusterStorageContainer{
					ObjectMeta: metav1.ObjectMeta{Name: "azure-blob"},
					Spec: v1alpha1.StorageContainerSpec{
						Container: corev1.Container{
							Name:  "storage-initializer",
							Image: "custom/azure-init:v1",
						},
						SupportedUriFormats: []v1alpha1.SupportedUriFormat{
							{Regex: `https://(.+?)\.blob\.core\.windows\.net/(.+)`},
						},
						WorkloadType: v1alpha1.InitContainer,
					},
				},
			},
			expectedImage: "custom/azure-init:v1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tc.objects {
				builder = builder.WithRuntimeObjects(obj)
			}
			c := builder.Build()

			spec, err := lookupStorageContainerSpec(t.Context(), c, tc.namespace, tc.storageUri)
			require.NoError(t, err)
			if tc.expectNil {
				assert.Nil(t, spec)
			} else {
				require.NotNil(t, spec)
				assert.Equal(t, tc.expectedImage, spec.Container.Image)
			}
		})
	}
}

func TestLookupStorageContainerSpecByName(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	nsSC := &v1alpha1.StorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: "my-sc", Namespace: "team-a"},
		Spec: v1alpha1.StorageContainerSpec{
			Container: corev1.Container{
				Name:  "storage-initializer",
				Image: "ns/init:v1",
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "hf://"}},
			WorkloadType:        v1alpha1.InitContainer,
		},
	}

	clusterSC := &v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: "my-sc"},
		Spec: v1alpha1.StorageContainerSpec{
			Container: corev1.Container{
				Name:  "storage-initializer",
				Image: "cluster/init:v1",
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "hf://"}},
			WorkloadType:        v1alpha1.InitContainer,
		},
	}

	tests := []struct {
		name          string
		namespace     string
		scName        string
		storageUri    string
		objects       []runtime.Object
		expectedImage string
		expectErr     bool
	}{
		{
			name:          "namespace SC found by name",
			namespace:     "team-a",
			scName:        "my-sc",
			storageUri:    "hf://model",
			objects:       []runtime.Object{nsSC, clusterSC},
			expectedImage: "ns/init:v1",
		},
		{
			name:          "falls back to cluster SC when not in namespace",
			namespace:     "team-b",
			scName:        "my-sc",
			storageUri:    "hf://model",
			objects:       []runtime.Object{nsSC, clusterSC},
			expectedImage: "cluster/init:v1",
		},
		{
			name:       "not found in either tier",
			namespace:  "team-a",
			scName:     "nonexistent",
			storageUri: "hf://model",
			objects:    []runtime.Object{nsSC},
			expectErr:  true,
		},
		{
			name:       "namespace SC exists but URI not supported",
			namespace:  "team-a",
			scName:     "my-sc",
			storageUri: "s3://bucket/model",
			objects:    []runtime.Object{nsSC},
			expectErr:  true,
		},
		{
			name:      "disabled namespace SC returns error",
			namespace: "team-a",
			scName:    "disabled-sc",
			storageUri: "hf://model",
			objects: []runtime.Object{
				&v1alpha1.StorageContainer{
					ObjectMeta: metav1.ObjectMeta{Name: "disabled-sc", Namespace: "team-a"},
					Disabled:   ptr.To(true),
					Spec: v1alpha1.StorageContainerSpec{
						Container:           corev1.Container{Name: "si", Image: "img"},
						SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "hf://"}},
						WorkloadType:        v1alpha1.InitContainer,
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tc.objects {
				builder = builder.WithRuntimeObjects(obj)
			}
			c := builder.Build()

			spec, err := lookupStorageContainerSpecByName(t.Context(), c, tc.namespace, tc.scName, tc.storageUri)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, spec)
			assert.Equal(t, tc.expectedImage, spec.Container.Image)
		})
	}
}

func TestAttachStorageInitializerWithStorageContainer(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	tests := []struct {
		name          string
		namespace     string
		modelUri      string
		objects       []runtime.Object
		expectedImage string
		expectedEnv   []string
	}{
		{
			name:          "no StorageContainer — uses ConfigMap defaults",
			namespace:     "team-a",
			modelUri:      "hf://meta-llama/Llama-2-7b",
			objects:       nil,
			expectedImage: "kserve/storage-initializer:latest",
		},
		{
			name:      "namespace StorageContainer overrides image and adds env",
			namespace: "team-a",
			modelUri:  "hf://meta-llama/Llama-2-7b",
			objects: []runtime.Object{
				&v1alpha1.StorageContainer{
					ObjectMeta: metav1.ObjectMeta{Name: "hf-sc", Namespace: "team-a"},
					Spec: v1alpha1.StorageContainerSpec{
						Container: corev1.Container{
							Name:  "storage-initializer",
							Image: "custom/si:hf-v2",
							Env: []corev1.EnvVar{
								{Name: "HF_TOKEN", Value: "tok-123"},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
						},
						SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "hf://"}},
						WorkloadType:        v1alpha1.InitContainer,
					},
				},
			},
			expectedImage: "custom/si:hf-v2",
			expectedEnv:   []string{"HF_TOKEN"},
		},
		{
			name:      "ClusterStorageContainer used when no namespace SC",
			namespace: "team-a",
			modelUri:  "s3://bucket/model",
			objects: []runtime.Object{
				&v1alpha1.ClusterStorageContainer{
					ObjectMeta: metav1.ObjectMeta{Name: "s3-sc"},
					Spec: v1alpha1.StorageContainerSpec{
						Container: corev1.Container{
							Name:  "storage-initializer",
							Image: "cluster/si:s3",
						},
						SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "s3://"}},
						WorkloadType:        v1alpha1.InitContainer,
					},
				},
			},
			expectedImage: "cluster/si:s3",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tc.objects {
				builder = builder.WithRuntimeObjects(obj)
			}
			r := &LLMISVCReconciler{
				Client: builder.Build(),
			}

			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "kserve-container"},
				},
			}

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-llmisvc", Namespace: tc.namespace},
			}

			err := r.attachStorageInitializer(
				t.Context(),
				llmSvc,
				tc.modelUri,
				corev1.PodSpec{},
				podSpec,
				testBaseStorageConfig(),
				"kserve-container",
				"/mnt/models",
			)
			require.NoError(t, err)

			var initContainer *corev1.Container
			for i := range podSpec.InitContainers {
				if podSpec.InitContainers[i].Name == constants.StorageInitializerContainerName {
					initContainer = &podSpec.InitContainers[i]
					break
				}
			}
			require.NotNil(t, initContainer, "storage-initializer init container should exist")

			assert.Equal(t, tc.expectedImage, initContainer.Image)

			for _, envName := range tc.expectedEnv {
				found := false
				for _, env := range initContainer.Env {
					if env.Name == envName {
						found = true
						break
					}
				}
				assert.True(t, found, "expected env var %q not found", envName)
			}
		})
	}
}
