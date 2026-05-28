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
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"knative.dev/pkg/apis"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	kserveTypes "github.com/kserve/kserve/pkg/types"
)

func speculatorTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = v1alpha2.AddToScheme(s)
	return s
}

func newTestReconciler(objects ...corev1.ServiceAccount) *LLMISVCReconciler {
	builder := clientfake.NewClientBuilder().WithScheme(speculatorTestScheme())
	for i := range objects {
		builder = builder.WithObjects(&objects[i])
	}
	return &LLMISVCReconciler{
		Client:    builder.Build(),
		Clientset: fake.NewSimpleClientset(),
	}
}

func newTestConfig() *Config {
	return &Config{
		StorageConfig: &kserveTypes.StorageInitializerConfig{
			Image:          "kserve/storage-initializer:latest",
			CpuRequest:     "100m",
			CpuLimit:       "1",
			MemoryRequest:  "100Mi",
			MemoryLimit:    "1Gi",
			CpuModelcar:    "10m",
			MemoryModelcar: "15Mi",
		},
		CredentialConfig: &credentials.CredentialConfig{},
	}
}

// --- Tests for attachSpeculatorModelArtifacts ---

func TestAttachSpeculatorModelArtifacts_NilSpeculator(t *testing.T) {
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model:      v1alpha2.LLMModelSpec{URI: *modelURI},
			Speculator: nil,
		},
	}

	r := newTestReconciler()
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := r.attachSpeculatorModelArtifacts(context.Background(), nil, llmSvc, corev1.PodSpec{}, podSpec, newTestConfig(), "main")
	require.NoError(t, err)

	assert.Empty(t, podSpec.InitContainers, "no init containers should be added when speculator is nil")
	assert.Empty(t, podSpec.Containers[0].Env, "no env vars should be added when speculator is nil")
}

func TestAttachSpeculatorModelArtifacts_NgramNoModel(t *testing.T) {
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
			Speculator: &v1alpha2.SpeculatorSpec{
				Config: map[string]string{
					"method":                 "ngram",
					"num_speculative_tokens": "4",
					"prompt_lookup_max":      "5",
				},
			},
		},
	}

	r := newTestReconciler()
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := r.attachSpeculatorModelArtifacts(context.Background(), nil, llmSvc, corev1.PodSpec{}, podSpec, newTestConfig(), "main")
	require.NoError(t, err)

	assert.Empty(t, podSpec.InitContainers, "ngram should not create init containers")

	var vllmArgs string
	for _, env := range podSpec.Containers[0].Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			vllmArgs = env.Value
		}
	}
	assert.Contains(t, vllmArgs, "--speculative-config", "should inject speculative config args")
	assert.NotContains(t, vllmArgs, "model", "ngram should not have model in speculative config")
}

func TestAttachSpeculatorModelArtifacts_HfModelCreatesInitContainer(t *testing.T) {
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	speculatorURI, _ := apis.ParseURL("hf://RedHatAI/Qwen3-32B-speculator.eagle3")

	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
	}
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
			Speculator: &v1alpha2.SpeculatorSpec{
				Model: &v1alpha2.LLMModelSpec{URI: *speculatorURI},
				Config: map[string]string{
					"method":                 "eagle3",
					"num_speculative_tokens": "6",
				},
			},
		},
	}

	r := newTestReconciler(sa)
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := r.attachSpeculatorModelArtifacts(context.Background(), nil, llmSvc, corev1.PodSpec{}, podSpec, newTestConfig(), "main")
	require.NoError(t, err)

	// Verify speculator-initializer init container was created
	var speculatorInit *corev1.Container
	for i := range podSpec.InitContainers {
		if podSpec.InitContainers[i].Name == constants.SpeculatorInitializerContainerName {
			speculatorInit = &podSpec.InitContainers[i]
			break
		}
	}
	require.NotNil(t, speculatorInit, "speculator-initializer init container should be created")

	// Verify init container args contain speculatorURI and mount path
	assert.Contains(t, speculatorInit.Args, speculatorURI.String())
	assert.Contains(t, speculatorInit.Args, constants.DefaultSpeculatorLocalMountPath)

	// Verify volume mount on init container (read-write)
	var initMount *corev1.VolumeMount
	for i := range speculatorInit.VolumeMounts {
		if speculatorInit.VolumeMounts[i].Name == constants.SpeculatorVolumeName {
			initMount = &speculatorInit.VolumeMounts[i]
			break
		}
	}
	require.NotNil(t, initMount, "speculator volume should be mounted on init container")
	assert.Equal(t, constants.DefaultSpeculatorLocalMountPath, initMount.MountPath)
	assert.False(t, initMount.ReadOnly, "init container mount should be read-write")

	// Verify volume mount on main container (read-only)
	var mainMount *corev1.VolumeMount
	for i := range podSpec.Containers[0].VolumeMounts {
		if podSpec.Containers[0].VolumeMounts[i].Name == constants.SpeculatorVolumeName {
			mainMount = &podSpec.Containers[0].VolumeMounts[i]
			break
		}
	}
	require.NotNil(t, mainMount, "speculator volume should be mounted on main container")
	assert.Equal(t, constants.DefaultSpeculatorLocalMountPath, mainMount.MountPath)
	assert.True(t, mainMount.ReadOnly, "main container mount should be read-only")

	// Verify VLLM_ADDITIONAL_ARGS with --speculative-config containing model path
	var vllmArgs string
	for _, env := range podSpec.Containers[0].Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			vllmArgs = env.Value
		}
	}
	assert.Contains(t, vllmArgs, "--speculative-config")
	assert.Contains(t, vllmArgs, constants.DefaultSpeculatorLocalMountPath)
}

func TestAttachSpeculatorModelArtifacts_PvcModel(t *testing.T) {
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	speculatorURI, _ := apis.ParseURL("pvc://my-pvc/speculator-model")

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
			Speculator: &v1alpha2.SpeculatorSpec{
				Model: &v1alpha2.LLMModelSpec{URI: *speculatorURI},
				Config: map[string]string{
					"method":                 "draft_model",
					"num_speculative_tokens": "5",
				},
			},
		},
	}

	r := newTestReconciler()
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := r.attachSpeculatorModelArtifacts(context.Background(), nil, llmSvc, corev1.PodSpec{}, podSpec, newTestConfig(), "main")
	require.NoError(t, err)

	// PVC model attachment uses volumes directly, not an init container
	var hasPVCVolume bool
	for _, vol := range podSpec.Volumes {
		if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == "my-pvc" {
			hasPVCVolume = true
			break
		}
	}
	assert.True(t, hasPVCVolume, "PVC volume should be attached for pvc:// speculator URI")

	// Verify speculative config is injected
	var vllmArgs string
	for _, env := range podSpec.Containers[0].Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			vllmArgs = env.Value
		}
	}
	assert.Contains(t, vllmArgs, "--speculative-config")
}

func TestAttachSpeculatorModelArtifacts_InvalidURI(t *testing.T) {
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
			Speculator: &v1alpha2.SpeculatorSpec{
				Model: &v1alpha2.LLMModelSpec{URI: apis.URL{Path: "no-scheme-model"}},
				Config: map[string]string{
					"method": "eagle3",
				},
			},
		},
	}

	r := newTestReconciler()
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := r.attachSpeculatorModelArtifacts(context.Background(), nil, llmSvc, corev1.PodSpec{}, podSpec, newTestConfig(), "main")
	assert.Error(t, err, "should return error for URI without scheme separator")
	assert.Contains(t, err.Error(), "invalid speculator model URI")
}

func TestAttachSpeculatorModelArtifacts_UnsupportedScheme(t *testing.T) {
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	speculatorURI, _ := apis.ParseURL("gs://bucket/speculator-model")

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
			Speculator: &v1alpha2.SpeculatorSpec{
				Model: &v1alpha2.LLMModelSpec{URI: *speculatorURI},
				Config: map[string]string{
					"method": "eagle3",
				},
			},
		},
	}

	r := newTestReconciler()
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := r.attachSpeculatorModelArtifacts(context.Background(), nil, llmSvc, corev1.PodSpec{}, podSpec, newTestConfig(), "main")
	assert.Error(t, err, "should return error for unsupported scheme")
	assert.Contains(t, err.Error(), "unsupported schema in speculator model URI")
}

// --- Tests for attachSpeculatorStorageInitializer ---

func TestAttachSpeculatorStorageInitializer_HfURI(t *testing.T) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
	}
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
		},
	}

	r := newTestReconciler(sa)
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := r.attachSpeculatorStorageInitializer(
		context.Background(),
		&sa,
		llmSvc,
		"hf://RedHatAI/Qwen3-32B-speculator.eagle3",
		constants.HfURIPrefix,
		corev1.PodSpec{},
		podSpec,
		newTestConfig(),
		"main",
	)
	require.NoError(t, err)

	// Verify init container name
	var initC *corev1.Container
	for i := range podSpec.InitContainers {
		if podSpec.InitContainers[i].Name == constants.SpeculatorInitializerContainerName {
			initC = &podSpec.InitContainers[i]
			break
		}
	}
	require.NotNil(t, initC, "speculator-initializer init container should exist")
	assert.Equal(t, constants.SpeculatorInitializerContainerName, initC.Name)

	// Verify args: [speculatorURI, mountPath]
	require.Len(t, initC.Args, 2)
	assert.Equal(t, "hf://RedHatAI/Qwen3-32B-speculator.eagle3", initC.Args[0])
	assert.Equal(t, constants.DefaultSpeculatorLocalMountPath, initC.Args[1])

	// Verify speculator volume exists
	var speculatorVol *corev1.Volume
	for i := range podSpec.Volumes {
		if podSpec.Volumes[i].Name == constants.SpeculatorVolumeName {
			speculatorVol = &podSpec.Volumes[i]
			break
		}
	}
	require.NotNil(t, speculatorVol, "speculator volume should be created")

	// Verify init container mount (read-write)
	var initMount *corev1.VolumeMount
	for i := range initC.VolumeMounts {
		if initC.VolumeMounts[i].Name == constants.SpeculatorVolumeName {
			initMount = &initC.VolumeMounts[i]
			break
		}
	}
	require.NotNil(t, initMount)
	assert.False(t, initMount.ReadOnly)
	assert.Equal(t, constants.DefaultSpeculatorLocalMountPath, initMount.MountPath)

	// Verify main container mount (read-only)
	var mainMount *corev1.VolumeMount
	for i := range podSpec.Containers[0].VolumeMounts {
		if podSpec.Containers[0].VolumeMounts[i].Name == constants.SpeculatorVolumeName {
			mainMount = &podSpec.Containers[0].VolumeMounts[i]
			break
		}
	}
	require.NotNil(t, mainMount)
	assert.True(t, mainMount.ReadOnly)
	assert.Equal(t, constants.DefaultSpeculatorLocalMountPath, mainMount.MountPath)
}

func TestAttachSpeculatorStorageInitializer_PreservesExistingImage(t *testing.T) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
	}
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
		},
	}

	r := newTestReconciler(sa)
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	existingImage := "custom-registry/storage-init:v2.0"
	curr := corev1.PodSpec{
		InitContainers: []corev1.Container{
			{
				Name:  constants.SpeculatorInitializerContainerName,
				Image: existingImage,
			},
		},
	}

	err := r.attachSpeculatorStorageInitializer(
		context.Background(),
		&sa,
		llmSvc,
		"hf://RedHatAI/Qwen3-32B-speculator.eagle3",
		constants.HfURIPrefix,
		curr,
		podSpec,
		newTestConfig(),
		"main",
	)
	require.NoError(t, err)

	var initC *corev1.Container
	for i := range podSpec.InitContainers {
		if podSpec.InitContainers[i].Name == constants.SpeculatorInitializerContainerName {
			initC = &podSpec.InitContainers[i]
			break
		}
	}
	require.NotNil(t, initC)
	assert.Equal(t, existingImage, initC.Image, "should preserve existing speculator-initializer image")
}

// --- Tests for attachMultiStorageDownloads ---

func TestAttachMultiStorageDownloads_EmptyPairs(t *testing.T) {
	r := newTestReconciler()
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	config := newTestConfig()
	err := r.attachMultiStorageDownloads(
		context.Background(), nil, llmSvc, corev1.PodSpec{}, podSpec,
		config.StorageConfig, config.CredentialConfig, "main", nil,
	)
	require.NoError(t, err)
	assert.Empty(t, podSpec.InitContainers, "no init containers for empty pairs")
}

func TestAttachMultiStorageDownloads_MultiplePairs(t *testing.T) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
	}
	r := newTestReconciler(sa)
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	pairs := []storageDownloadPair{
		{uri: "hf://meta-llama/Llama-2-7b", path: "/mnt/models"},
		{uri: "hf://lora-adapter/adapter-1", path: "/mnt/models/adapters/adapter-1"},
	}

	config := newTestConfig()
	err := r.attachMultiStorageDownloads(
		context.Background(), &sa, llmSvc, corev1.PodSpec{}, podSpec,
		config.StorageConfig, config.CredentialConfig, "main", pairs,
	)
	require.NoError(t, err)

	// Verify init container was created with correct args (pairs interleaved as uri, path, uri, path)
	require.NotEmpty(t, podSpec.InitContainers)
	initC := &podSpec.InitContainers[0]
	assert.Equal(t, constants.StorageInitializerContainerName, initC.Name)
	require.Len(t, initC.Args, 4)
	assert.Equal(t, "hf://meta-llama/Llama-2-7b", initC.Args[0])
	assert.Equal(t, "/mnt/models", initC.Args[1])
	assert.Equal(t, "hf://lora-adapter/adapter-1", initC.Args[2])
	assert.Equal(t, "/mnt/models/adapters/adapter-1", initC.Args[3])

	// Verify common parent path mount on main container
	var mainMount *corev1.VolumeMount
	for i := range podSpec.Containers[0].VolumeMounts {
		if podSpec.Containers[0].VolumeMounts[i].Name == constants.StorageInitializerVolumeName {
			mainMount = &podSpec.Containers[0].VolumeMounts[i]
			break
		}
	}
	require.NotNil(t, mainMount)
	assert.Equal(t, "/mnt/models", mainMount.MountPath, "should use common parent path")
	assert.True(t, mainMount.ReadOnly, "main container mount should be read-only")
}

func TestAttachMultiStorageDownloads_StripsExistingStorageInitializer(t *testing.T) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
	}
	r := newTestReconciler(sa)
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
		InitContainers: []corev1.Container{
			{Name: constants.StorageInitializerContainerName, Image: "old-image"},
			{Name: "other-init", Image: "other-image"},
		},
	}

	pairs := []storageDownloadPair{
		{uri: "hf://meta-llama/Llama-2-7b", path: "/mnt/models"},
	}

	config := newTestConfig()
	err := r.attachMultiStorageDownloads(
		context.Background(), &sa, llmSvc, corev1.PodSpec{}, podSpec,
		config.StorageConfig, config.CredentialConfig, "main", pairs,
	)
	require.NoError(t, err)

	// The old storage-initializer should have been stripped and a new one added
	initNames := make([]string, len(podSpec.InitContainers))
	for i, c := range podSpec.InitContainers {
		initNames[i] = c.Name
	}
	assert.Contains(t, initNames, "other-init", "should preserve non-storage init containers")
	assert.Contains(t, initNames, constants.StorageInitializerContainerName, "should have new storage-initializer")

	// Should be exactly 2: other-init + new storage-initializer
	assert.Len(t, podSpec.InitContainers, 2)
}

// --- Tests for collectLoRADownloadPairs ---

func TestCollectLoRADownloadPairs_FiltersSchemes(t *testing.T) {
	adapters := []resolvedLoRAAdapter{
		{scheme: constants.HfURIPrefix, uri: "hf://adapter/a1", mountPath: "/mnt/adapters/a1"},
		{scheme: constants.S3URIPrefix, uri: "s3://bucket/a2", mountPath: "/mnt/adapters/a2"},
		{scheme: constants.PvcURIPrefix, uri: "pvc://pvc-name/a3", mountPath: "/mnt/adapters/a3"},
	}

	pairs := collectLoRADownloadPairs(adapters)

	require.Len(t, pairs, 2, "should only include hf:// and s3:// adapters")
	assert.Equal(t, "hf://adapter/a1", pairs[0].uri)
	assert.Equal(t, "/mnt/adapters/a1", pairs[0].path)
	assert.Equal(t, "s3://bucket/a2", pairs[1].uri)
	assert.Equal(t, "/mnt/adapters/a2", pairs[1].path)
}

func TestCollectLoRADownloadPairs_EmptyAdapters(t *testing.T) {
	pairs := collectLoRADownloadPairs(nil)
	assert.Empty(t, pairs)
}

// --- Tests for stripPriorControllerStorageInitializer ---

func TestStripPriorControllerStorageInitializer(t *testing.T) {
	podSpec := &corev1.PodSpec{
		InitContainers: []corev1.Container{
			{Name: constants.StorageInitializerContainerName},
			{Name: "other-init"},
			{Name: constants.SpeculatorInitializerContainerName},
		},
	}

	stripPriorControllerStorageInitializer(podSpec)

	require.Len(t, podSpec.InitContainers, 2)
	assert.Equal(t, "other-init", podSpec.InitContainers[0].Name)
	assert.Equal(t, constants.SpeculatorInitializerContainerName, podSpec.InitContainers[1].Name)
}

// --- Tests for attachSpeculatorModelArtifacts with OCI URI ---

func TestAttachSpeculatorModelArtifacts_OciModel(t *testing.T) {
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	speculatorURI, _ := apis.ParseURL("oci://registry.io/speculator:latest")

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
			Speculator: &v1alpha2.SpeculatorSpec{
				Model: &v1alpha2.LLMModelSpec{URI: *speculatorURI},
				Config: map[string]string{
					"method":                 "eagle3",
					"num_speculative_tokens": "3",
				},
			},
		},
	}

	r := newTestReconciler()
	config := newTestConfig()
	config.StorageConfig.EnableOciImageSource = true
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := r.attachSpeculatorModelArtifacts(context.Background(), nil, llmSvc, corev1.PodSpec{}, podSpec, config, "main")
	require.NoError(t, err)

	// OCI modelcar adds a volume mount on the main container at the parent directory
	var hasMount bool
	for _, vm := range podSpec.Containers[0].VolumeMounts {
		if vm.MountPath == "/mnt" || vm.MountPath == constants.DefaultSpeculatorLocalMountPath {
			hasMount = true
			break
		}
	}
	assert.True(t, hasMount, "OCI speculator should add volume mount for modelcar")

	// Verify speculative config args are injected
	var vllmArgs string
	for _, env := range podSpec.Containers[0].Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			vllmArgs = env.Value
		}
	}
	assert.Contains(t, vllmArgs, "--speculative-config")
	assert.Contains(t, vllmArgs, constants.DefaultSpeculatorLocalMountPath)
}

func TestAttachSpeculatorModelArtifacts_OciDisabled(t *testing.T) {
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	speculatorURI, _ := apis.ParseURL("oci://registry.io/speculator:latest")

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
			Speculator: &v1alpha2.SpeculatorSpec{
				Model: &v1alpha2.LLMModelSpec{URI: *speculatorURI},
				Config: map[string]string{
					"method": "eagle3",
				},
			},
		},
	}

	r := newTestReconciler()
	config := newTestConfig()
	config.StorageConfig.EnableOciImageSource = false
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := r.attachSpeculatorModelArtifacts(context.Background(), nil, llmSvc, corev1.PodSpec{}, podSpec, config, "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OCI modelcars is not enabled")
}

// --- Tests for attachSpeculatorStorageInitializer with S3 URI ---

func TestAttachSpeculatorStorageInitializer_S3URI(t *testing.T) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
	}
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
		},
	}

	r := newTestReconciler(sa)
	config := newTestConfig()
	config.StorageConfig.CaBundleConfigMapName = "test-ca-bundle"
	config.StorageConfig.CaBundleVolumeMountPath = "/etc/ssl/custom"

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := r.attachSpeculatorStorageInitializer(
		context.Background(),
		&sa,
		llmSvc,
		"s3://bucket/speculator-model",
		constants.S3URIPrefix,
		corev1.PodSpec{},
		podSpec,
		config,
		"main",
	)
	require.NoError(t, err)

	// Verify init container was created
	var initC *corev1.Container
	for i := range podSpec.InitContainers {
		if podSpec.InitContainers[i].Name == constants.SpeculatorInitializerContainerName {
			initC = &podSpec.InitContainers[i]
			break
		}
	}
	require.NotNil(t, initC, "speculator-initializer should be created for S3")
	assert.Equal(t, "s3://bucket/speculator-model", initC.Args[0])
}

// --- Tests for attachSpeculatorStorageInitializer with nil service account ---

func TestAttachSpeculatorStorageInitializer_NilServiceAccountFallback(t *testing.T) {
	// No service account registered in fake client - r.Get will fail
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
		},
	}

	r := newTestReconciler() // no service account
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	// Should not error - graceful degradation when SA not found
	err := r.attachSpeculatorStorageInitializer(
		context.Background(),
		nil, // nil SA forces r.Get lookup
		llmSvc,
		"hf://RedHatAI/Qwen3-32B-speculator.eagle3",
		constants.HfURIPrefix,
		corev1.PodSpec{},
		podSpec,
		newTestConfig(),
		"main",
	)
	require.NoError(t, err, "should gracefully handle missing service account")

	// Init container should still be created (just without credentials)
	var initC *corev1.Container
	for i := range podSpec.InitContainers {
		if podSpec.InitContainers[i].Name == constants.SpeculatorInitializerContainerName {
			initC = &podSpec.InitContainers[i]
			break
		}
	}
	require.NotNil(t, initC, "init container should still be created even without SA")
}

// --- Tests for injectSpeculativeDecodingArgs edge cases ---

func TestInjectSpeculativeDecodingArgs_EmptyConfig(t *testing.T) {
	speculator := &v1alpha2.SpeculatorSpec{
		Config: map[string]string{},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	var vllmArgs string
	for _, env := range podSpec.Containers[0].Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			vllmArgs = env.Value
		}
	}
	require.NotEmpty(t, vllmArgs, "should still inject --speculative-config even with empty config")
	assert.Contains(t, vllmArgs, "--speculative-config")

	// Verify the JSON is valid (just an empty object)
	jsonStr := extractSpecConfigJSON(t, vllmArgs)
	var specConfig map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &specConfig)
	require.NoError(t, err)
	assert.Empty(t, specConfig, "empty config should produce empty JSON object")
}

func TestInjectSpeculativeDecodingArgs_NilConfig(t *testing.T) {
	speculator := &v1alpha2.SpeculatorSpec{
		Config: nil,
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	var vllmArgs string
	for _, env := range podSpec.Containers[0].Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			vllmArgs = env.Value
		}
	}
	assert.Contains(t, vllmArgs, "--speculative-config")
}

func TestInjectSpeculativeDecodingArgs_ContainerNotFound(t *testing.T) {
	speculator := &v1alpha2.SpeculatorSpec{
		Config: map[string]string{
			"method":                 "ngram",
			"num_speculative_tokens": "4",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	// Target a container that doesn't exist
	err := injectSpeculativeDecodingArgs(speculator, podSpec, "nonexistent")
	require.NoError(t, err, "should not error when container not found")

	// Verify nothing was injected into the existing container
	assert.Empty(t, podSpec.Containers[0].Env, "should not inject into wrong container")
}

func TestInjectSpeculativeDecodingArgs_EmptyExistingVLLMArgs(t *testing.T) {
	speculatorURI, _ := apis.ParseURL("hf://RedHatAI/speculator")
	speculator := &v1alpha2.SpeculatorSpec{
		Model: &v1alpha2.LLMModelSpec{URI: *speculatorURI},
		Config: map[string]string{
			"method":                 "eagle3",
			"num_speculative_tokens": "3",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name: "main",
				Env: []corev1.EnvVar{
					{Name: "VLLM_ADDITIONAL_ARGS", Value: ""},
				},
			},
		},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	vllmArgs := podSpec.Containers[0].Env[0].Value
	// Should not have leading space
	assert.Equal(t, "--speculative-config", vllmArgs[:len("--speculative-config")])
}

// --- DeepCopy mutation isolation test ---

func TestSpeculatorSpec_DeepCopy_MutationIsolation(t *testing.T) {
	speculatorURI, _ := apis.ParseURL("hf://RedHatAI/Qwen3-32B-speculator.eagle3")
	original := &v1alpha2.SpeculatorSpec{
		Model: &v1alpha2.LLMModelSpec{URI: *speculatorURI},
		Config: map[string]string{
			"method":                 "eagle3",
			"num_speculative_tokens": "3",
		},
	}

	copied := original.DeepCopy()

	// Mutate the copy
	copied.Config["method"] = "draft_model"
	copied.Config["new_key"] = "new_value"
	newURI, _ := apis.ParseURL("hf://different/model")
	copied.Model.URI = *newURI

	// Verify original is unaffected
	assert.Equal(t, "eagle3", original.Config["method"], "original config should not be mutated")
	_, hasNewKey := original.Config["new_key"]
	assert.False(t, hasNewKey, "original config should not have new keys from copy")
	assert.Equal(t, "RedHatAI", original.Model.URI.Host, "original model URI host should not be mutated")
	assert.Contains(t, original.Model.URI.String(), "Qwen3-32B-speculator.eagle3", "original URI path should not be mutated")
}

func TestSpeculatorSpec_DeepCopy_NilModel(t *testing.T) {
	original := &v1alpha2.SpeculatorSpec{
		Config: map[string]string{
			"method":                 "ngram",
			"num_speculative_tokens": "4",
		},
	}

	copied := original.DeepCopy()

	assert.Nil(t, copied.Model, "nil model should remain nil in deep copy")
	assert.Equal(t, "ngram", copied.Config["method"])

	// Mutate copied config
	copied.Config["method"] = "mtp"
	assert.Equal(t, "ngram", original.Config["method"], "mutation isolation should hold")
}

func TestSpeculatorSpec_DeepCopy_NilConfig(t *testing.T) {
	speculatorURI, _ := apis.ParseURL("hf://RedHatAI/Qwen3-32B-speculator.eagle3")
	original := &v1alpha2.SpeculatorSpec{
		Model:  &v1alpha2.LLMModelSpec{URI: *speculatorURI},
		Config: nil,
	}

	copied := original.DeepCopy()

	assert.Nil(t, copied.Config, "nil config should remain nil in deep copy")
	assert.NotNil(t, copied.Model)
}

func TestSpeculatorSpec_DeepCopy_Nil(t *testing.T) {
	var original *v1alpha2.SpeculatorSpec
	copied := original.DeepCopy()
	assert.Nil(t, copied)
}

// --- Tests for attachMultiStorageDownloads with tokenizer container ---

func TestAttachMultiStorageDownloads_TokenizerContainer(t *testing.T) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
	}
	r := newTestReconciler(sa)
	modelURI, _ := apis.ParseURL("hf://meta-llama/Llama-2-7b")
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURI},
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: tokenizerContainerName}},
	}

	pairs := []storageDownloadPair{
		{uri: "hf://meta-llama/Llama-2-7b", path: "/mnt/models"},
	}

	config := newTestConfig()
	err := r.attachMultiStorageDownloads(
		context.Background(), &sa, llmSvc, corev1.PodSpec{}, podSpec,
		config.StorageConfig, config.CredentialConfig, tokenizerContainerName, pairs,
	)
	require.NoError(t, err)

	// Verify STORAGE_ALLOW_PATTERNS is injected for tokenizer container
	initC := &podSpec.InitContainers[0]
	var hasAllowPatterns bool
	for _, env := range initC.Env {
		if env.Name == "STORAGE_ALLOW_PATTERNS" {
			hasAllowPatterns = true
			break
		}
	}
	assert.True(t, hasAllowPatterns, "tokenizer container should have STORAGE_ALLOW_PATTERNS on init container")
}
