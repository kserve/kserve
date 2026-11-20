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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	kserveTypes "github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	testCSCImage = "ghcr.io/example/hardened-storage-initializer:v1"
	testCSCName  = "internal-registry"
)

func testClusterStorageContainer(mutate ...func(*v1alpha1.ClusterStorageContainer)) *v1alpha1.ClusterStorageContainer {
	csc := &v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: testCSCName},
		Spec: v1alpha1.StorageContainerSpec{
			Container: corev1.Container{
				Name:  "custom-downloader",
				Image: testCSCImage,
				Args:  []string{"ignored"},
				Env:   []corev1.EnvVar{{Name: "CUSTOM_CA_BUNDLE", Value: "/etc/ssl/internal/ca.pem"}},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("300Mi"),
					},
				},
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "s3://"}, {Prefix: "hf://"}},
			WorkloadType:        v1alpha1.InitContainer,
		},
	}
	for _, m := range mutate {
		m(csc)
	}
	return csc
}

func storageContainerTestReconciler(t *testing.T, objs ...*v1alpha1.ClusterStorageContainer) (*LLMISVCReconciler, *record.FakeRecorder) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, o := range objs {
		builder = builder.WithObjects(o)
	}
	recorder := record.NewFakeRecorder(10)
	return &LLMISVCReconciler{Client: builder.Build(), Clientset: k8sfake.NewSimpleClientset(), EventRecorder: recorder}, recorder
}

func storageContainerTestLLMSvc(storageContainerName *string) *v1alpha2.LLMInferenceService {
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			StorageInitializer: &v1alpha2.StorageInitializerSpec{StorageContainerName: storageContainerName},
		},
	}
}

func storageContainerTestConfig() *kserveTypes.StorageInitializerConfig {
	return &kserveTypes.StorageInitializerConfig{
		Image:         "kserve/storage-initializer:test",
		CpuRequest:    "100m",
		CpuLimit:      "1",
		MemoryRequest: "200Mi",
		MemoryLimit:   "1Gi",
	}
}

func TestAttachStorageInitializerWithStorageContainer(t *testing.T) {
	t.Parallel()

	const modelURI = "s3://models/llama"
	controllerArgs := []string{modelURI, constants.DefaultModelLocalMountPath}

	tests := []struct {
		name          string
		cscs          []*v1alpha1.ClusterStorageContainer
		cscName       *string
		userOverride  *corev1.Container
		wantErr       string
		wantImage     string
		wantEnv       []corev1.EnvVar
		wantCPUReq    string
		wantCPULimit  string
		wantMemReq    string
		wantNoCSCWork bool
	}{
		{
			name:         "named csc applied, controller name and args kept",
			cscs:         []*v1alpha1.ClusterStorageContainer{testClusterStorageContainer()},
			cscName:      ptr.To(testCSCName),
			wantImage:    testCSCImage,
			wantEnv:      []corev1.EnvVar{{Name: "CUSTOM_CA_BUNDLE", Value: "/etc/ssl/internal/ca.pem"}},
			wantCPUReq:   "250m",
			wantCPULimit: "1",
			wantMemReq:   "300Mi",
		},
		{
			name:          "unset field leaves the default initializer",
			cscs:          []*v1alpha1.ClusterStorageContainer{testClusterStorageContainer()},
			cscName:       nil,
			wantImage:     "kserve/storage-initializer:test",
			wantCPUReq:    "100m",
			wantCPULimit:  "1",
			wantMemReq:    "200Mi",
			wantNoCSCWork: true,
		},
		{
			name:          "empty name is treated as unset",
			cscName:       ptr.To(""),
			wantImage:     "kserve/storage-initializer:test",
			wantCPUReq:    "100m",
			wantCPULimit:  "1",
			wantMemReq:    "200Mi",
			wantNoCSCWork: true,
		},
		{
			name:    "not found",
			cscName: ptr.To("missing"),
			wantErr: `ClusterStorageContainer "missing" not found`,
		},
		{
			name: "disabled",
			cscs: []*v1alpha1.ClusterStorageContainer{testClusterStorageContainer(func(c *v1alpha1.ClusterStorageContainer) {
				c.Disabled = ptr.To(true)
			})},
			cscName: ptr.To(testCSCName),
			wantErr: `ClusterStorageContainer "internal-registry" is disabled`,
		},
		{
			name: "wrong workload type",
			cscs: []*v1alpha1.ClusterStorageContainer{testClusterStorageContainer(func(c *v1alpha1.ClusterStorageContainer) {
				c.Spec.WorkloadType = v1alpha1.LocalModelDownloadJob
			})},
			cscName: ptr.To(testCSCName),
			wantErr: `has workloadType "localModelDownloadJob"`,
		},
		{
			name: "unsupported uri",
			cscs: []*v1alpha1.ClusterStorageContainer{testClusterStorageContainer(func(c *v1alpha1.ClusterStorageContainer) {
				c.Spec.SupportedUriFormats = []v1alpha1.SupportedUriFormat{{Prefix: "gs://"}}
			})},
			cscName: ptr.To(testCSCName),
			wantErr: `does not support storageUri "s3://models/llama"`,
		},
		{
			name:    "user override wins over the csc",
			cscs:    []*v1alpha1.ClusterStorageContainer{testClusterStorageContainer()},
			cscName: ptr.To(testCSCName),
			userOverride: &corev1.Container{
				Name:  constants.StorageInitializerContainerName,
				Image: "user/initializer:v2",
				Env:   []corev1.EnvVar{{Name: "CUSTOM_CA_BUNDLE", Value: "/user/ca.pem"}},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
				},
			},
			wantImage:    "user/initializer:v2",
			wantEnv:      []corev1.EnvVar{{Name: "CUSTOM_CA_BUNDLE", Value: "/user/ca.pem"}},
			wantCPUReq:   "500m",
			wantCPULimit: "1",
			wantMemReq:   "300Mi", // from the CSC, not overridden by the user
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, recorder := storageContainerTestReconciler(t, tt.cscs...)
			podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}
			if tt.userOverride != nil {
				podSpec.InitContainers = []corev1.Container{*tt.userOverride}
			}

			err := r.attachStorageInitializer(t.Context(), storageContainerTestLLMSvc(tt.cscName), modelURI, corev1.PodSpec{}, podSpec, storageContainerTestConfig(), "main", constants.DefaultModelLocalMountPath)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName),
					"no storage-initializer must be attached when the ClusterStorageContainer cannot be used")
				requireEvent(t, recorder, "Warning StorageContainerNotResolved")
				return
			}
			require.NoError(t, err)

			initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName)
			require.NotNil(t, initContainer)
			require.Equal(t, constants.StorageInitializerContainerName, initContainer.Name)
			require.Equal(t, controllerArgs, initContainer.Args)
			require.Nil(t, initContainer.Command)
			require.Equal(t, tt.wantImage, initContainer.Image)
			require.Equal(t, tt.wantEnv, initContainer.Env)
			require.Equal(t, tt.wantCPUReq, ptr.To(initContainer.Resources.Requests[corev1.ResourceCPU]).String())
			require.Equal(t, tt.wantCPULimit, ptr.To(initContainer.Resources.Limits[corev1.ResourceCPU]).String())
			require.Equal(t, tt.wantMemReq, ptr.To(initContainer.Resources.Requests[corev1.ResourceMemory]).String())
			require.Len(t, initContainer.VolumeMounts, 1, "the controller's model volume mount must be kept")

			if tt.wantNoCSCWork {
				// Byte-for-byte the container the controller built before storageContainerName existed.
				noField := &corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}
				legacy := &LLMISVCReconciler{}
				require.NoError(t, legacy.attachStorageInitializer(t.Context(), &v1alpha2.LLMInferenceService{}, modelURI, corev1.PodSpec{}, noField, storageContainerTestConfig(), "main", constants.DefaultModelLocalMountPath))
				require.Equal(t, noField.InitContainers, podSpec.InitContainers)
			}
		})
	}
}

func TestAttachMultiStorageDownloadsWithStorageContainer(t *testing.T) {
	t.Parallel()

	pairs := []storageDownloadPair{
		{uri: "s3://models/llama", path: constants.DefaultModelLocalMountPath},
		{uri: "hf://org/adapter-a", path: "/mnt/models/lora/a"},
		{uri: "s3://adapters/b", path: "/mnt/models/lora/b"},
	}

	multiModel := func(v *bool) func(*v1alpha1.ClusterStorageContainer) {
		return func(c *v1alpha1.ClusterStorageContainer) { c.Spec.SupportsMultiModelDownload = v }
	}

	tests := []struct {
		name    string
		csc     *v1alpha1.ClusterStorageContainer
		pairs   []storageDownloadPair
		wantErr string
	}{
		{
			name: "csc supporting every uri and multi-model download is applied",
			csc:  testClusterStorageContainer(multiModel(ptr.To(true))),
		},
		{
			name: "csc missing an adapter scheme is rejected",
			csc: testClusterStorageContainer(multiModel(ptr.To(true)), func(c *v1alpha1.ClusterStorageContainer) {
				c.Spec.SupportedUriFormats = []v1alpha1.SupportedUriFormat{{Prefix: "s3://"}}
			}),
			wantErr: `does not support storageUri "hf://org/adapter-a"`,
		},
		{
			name:    "csc not declaring multi-model download is rejected for several downloads",
			csc:     testClusterStorageContainer(),
			wantErr: "does not support multi-model download",
		},
		{
			name:    "csc declaring single-model download is rejected for several downloads",
			csc:     testClusterStorageContainer(multiModel(ptr.To(false))),
			wantErr: "does not support multi-model download",
		},
		{
			name:  "single-model csc is applied when there is only one download",
			csc:   testClusterStorageContainer(multiModel(ptr.To(false))),
			pairs: pairs[1:2],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, recorder := storageContainerTestReconciler(t, tt.csc)
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}}
			podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}

			downloads := pairs
			if tt.pairs != nil {
				downloads = tt.pairs
			}
			err := r.attachMultiStorageDownloads(t.Context(), sa, storageContainerTestLLMSvc(ptr.To(testCSCName)), corev1.PodSpec{}, podSpec, storageContainerTestConfig(), &credentials.CredentialConfig{}, "main", downloads)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName))
				requireEvent(t, recorder, "Warning StorageContainerNotResolved")
				return
			}
			require.NoError(t, err)
			initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName)
			require.NotNil(t, initContainer)
			require.Equal(t, testCSCImage, initContainer.Image)
			wantArgs := make([]string, 0, len(downloads)*2)
			for _, d := range downloads {
				wantArgs = append(wantArgs, d.uri, d.path)
			}
			require.Equal(t, wantArgs, initContainer.Args)
			require.Contains(t, initContainer.Env, corev1.EnvVar{Name: "CUSTOM_CA_BUNDLE", Value: "/etc/ssl/internal/ca.pem"})
		})
	}
}

func requireEvent(t *testing.T, recorder *record.FakeRecorder, prefix string) {
	t.Helper()
	select {
	case e := <-recorder.Events:
		require.True(t, strings.HasPrefix(e, prefix), "event %q does not start with %q", e, prefix)
	default:
		t.Fatalf("expected an event starting with %q, got none", prefix)
	}
}
