/*
Copyright 2024 The KServe Authors.

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

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/kernelcachecommon"
	"github.com/kserve/kserve/pkg/credentials"
)

// TestInjectKernelCache_NoLabel verifies that InjectKernelCache is a no-op when no KC label is present
func TestInjectKernelCache_NoLabel(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pod",
			Namespace:   "default",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(c, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: c,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err, "InjectKernelCache should not error when no KC label is present")

	// Verify no volumes were added
	assert.Empty(t, pod.Spec.Volumes, "No volumes should be added when KC label is missing")

	// Verify no volume mounts were added
	assert.Empty(t, pod.Spec.Containers[0].VolumeMounts, "No volume mounts should be added when KC label is missing")
}

// TestInjectKernelCache_MissingPVCAnnotation verifies auto-derivation when KC label exists but PVC annotation is missing
func TestInjectKernelCache_MissingPVCAnnotation(t *testing.T) {
	// Create a KernelCache CR that the webhook can fetch
	resolvedDigest := "sha256:ce6edaa98a86702092994febc24f0dd58900ec978d2cdb6e3711279ddb66f237"
	imageSpec := "quay.io/test/cache:latest"
	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cache",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationCacheHash:         "abc123",
				v1alpha1.AnnotationCacheMountSubpath: "torch_compile_cache/abc123",
				v1alpha1.AnnotationCacheRootEnv:      "VLLM_CACHE_ROOT=/home/kserve/.cache/vllm",
				v1alpha1.AnnotationResolvedDigest:    resolvedDigest,
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image: imageSpec,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
			Annotations: map[string]string{
				// Missing KernelCachePVCNameAnnotationKey - should be auto-derived
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	// Create a fake client WITH the KC object and scheme
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err, "InjectKernelCache should auto-derive PVC name when annotation is missing")

	// Verify PVC annotation was auto-added
	assert.Equal(t, "test-cache", pod.Annotations[constants.KernelCachePVCNameAnnotationKey],
		"PVC annotation should be auto-derived from KC label (serving PVC = KC name)")

	// Verify volume was created with auto-derived PVC name
	assert.Len(t, pod.Spec.Volumes, 1, "One volume should be added")
	assert.Equal(t, "kernel-cache", pod.Spec.Volumes[0].Name)
	assert.NotNil(t, pod.Spec.Volumes[0].PersistentVolumeClaim)
	assert.Equal(t, "test-cache", pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName)
}

// TestInjectKernelCache_WithoutCRD verifies graceful fallback when KC CRDs are not installed.
// This simulates the scenario where KServe is deployed without KernelCache feature enabled.
// The test uses a fake client that doesn't have KC objects, so Get() will fail with "no matches for kind".
func TestInjectKernelCache_WithoutCRD(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
			Annotations: map[string]string{
				constants.KernelCachePVCNameAnnotationKey: "test-cache-pvc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	// Create a fake client WITHOUT any KC objects
	// This simulates the CRD not being installed
	fakeClient := fake.NewClientBuilder().Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	// Should not error - should fall back to legacy mounting
	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err, "InjectKernelCache should gracefully handle missing CRD by falling back to legacy mode")

	// Verify legacy mount was created at /mnt/kernel-cache
	assert.Len(t, pod.Spec.Volumes, 1, "One volume should be added (legacy fallback)")
	assert.Equal(t, "kernel-cache", pod.Spec.Volumes[0].Name)
	assert.NotNil(t, pod.Spec.Volumes[0].PersistentVolumeClaim)
	assert.Equal(t, "test-cache-pvc", pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName)

	// Verify legacy mount path
	assert.Len(t, pod.Spec.Containers[0].VolumeMounts, 1, "One volume mount should be added (legacy fallback)")
	assert.Equal(t, "kernel-cache", pod.Spec.Containers[0].VolumeMounts[0].Name)
	assert.Equal(t, "/mnt/kernel-cache", pod.Spec.Containers[0].VolumeMounts[0].MountPath, "Should use legacy mount path when CRD is missing")
	assert.True(t, pod.Spec.Containers[0].VolumeMounts[0].ReadOnly)

	// Verify NO environment variable is set in legacy mode
	assert.Empty(t, pod.Spec.Containers[0].Env, "No env vars should be set in legacy fallback mode")
}

// TestInjectKernelCache_WithCRD_FrameworkAgnostic verifies framework-agnostic mounting when KC CRD exists
func TestInjectKernelCache_WithCRD_FrameworkAgnostic(t *testing.T) {
	// Create a KernelCache CR with mounting metadata annotations
	resolvedDigest := "sha256:ce6edaa98a86702092994febc24f0dd58900ec978d2cdb6e3711279ddb66f237"
	imageSpec := "quay.io/test/cache:latest"
	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cache",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationCacheHash:         "abc123",
				v1alpha1.AnnotationCacheMountSubpath: "torch_compile_cache/abc123",
				v1alpha1.AnnotationCacheRootEnv:      "VLLM_CACHE_ROOT=/home/kserve/.cache/vllm",
				v1alpha1.AnnotationResolvedDigest:    resolvedDigest,
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image: imageSpec,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
			Annotations: map[string]string{
				constants.KernelCachePVCNameAnnotationKey: "test-cache-pvc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	// Create a fake client WITH the KC object
	// Need to register the v1alpha1 scheme for KernelCache CRD
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err, "InjectKernelCache should succeed with framework-agnostic mounting")

	// Verify volume was created
	assert.Len(t, pod.Spec.Volumes, 1, "One volume should be added")
	assert.Equal(t, "kernel-cache", pod.Spec.Volumes[0].Name)
	assert.NotNil(t, pod.Spec.Volumes[0].PersistentVolumeClaim)
	assert.Equal(t, "test-cache-pvc", pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName)

	// Verify framework-agnostic mount path
	assert.Len(t, pod.Spec.Containers[0].VolumeMounts, 1, "One volume mount should be added")
	assert.Equal(t, "kernel-cache", pod.Spec.Containers[0].VolumeMounts[0].Name)
	assert.Equal(t, "/home/kserve/.cache/vllm/torch_compile_cache/abc123", pod.Spec.Containers[0].VolumeMounts[0].MountPath,
		"Should use framework-specific mount path from KC annotations")
	assert.True(t, pod.Spec.Containers[0].VolumeMounts[0].ReadOnly)

	// Verify SubPath is set to skip KC extraction job nesting
	// IMPORTANT: Must use the SAME calculation as extraction job!
	// Extraction job uses GetKernelCacheStorageKey(imageWithDigest) where imageWithDigest is computed
	// via ReplaceUrlTag() which REMOVES the tag and replaces with @digest
	imageWithDigest := kernelcachecommon.ReplaceUrlTag(imageSpec, resolvedDigest) // quay.io/test/cache@sha256:... (tag removed!)
	expectedStorageKey := v1alpha1.GetKernelCacheStorageKey(imageWithDigest)
	expectedSubPath := "kernel-cache/" + expectedStorageKey + "/torch_compile_cache/abc123"
	assert.Equal(t, expectedSubPath, pod.Spec.Containers[0].VolumeMounts[0].SubPath,
		"Should set SubPath to skip kernel-cache/<storageKey>/ nesting from extraction job")

	// Verify framework-specific environment variable
	assert.Len(t, pod.Spec.Containers[0].Env, 1, "One env var should be set")
	assert.Equal(t, "VLLM_CACHE_ROOT", pod.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "/home/kserve/.cache/vllm", pod.Spec.Containers[0].Env[0].Value)
}

// TestInjectKernelCache_LegacyFallback verifies fallback to legacy mounting when KC exists but lacks mounting metadata
func TestInjectKernelCache_LegacyFallback(t *testing.T) {
	// Create a KernelCache CR WITHOUT mounting metadata annotations (older cache images)
	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-cache",
			Namespace:   "default",
			Annotations: map[string]string{
				// No mounting metadata annotations
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image: "quay.io/test/cache:old",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
			Annotations: map[string]string{
				constants.KernelCachePVCNameAnnotationKey: "test-cache-pvc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	// Create a fake client WITH the KC object but no mounting metadata
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err, "InjectKernelCache should fall back to legacy mode for old cache images")

	// Verify legacy mount path
	assert.Len(t, pod.Spec.Volumes, 1, "One volume should be added")
	assert.Len(t, pod.Spec.Containers[0].VolumeMounts, 1, "One volume mount should be added")
	assert.Equal(t, "/mnt/kernel-cache", pod.Spec.Containers[0].VolumeMounts[0].MountPath,
		"Should fall back to legacy mount path when mounting metadata is missing")

	// Verify NO environment variable is set in legacy mode
	assert.Empty(t, pod.Spec.Containers[0].Env, "No env vars should be set in legacy mode")
}

// TestInjectKernelCache_Idempotent verifies that calling InjectKernelCache multiple times is safe
func TestInjectKernelCache_Idempotent(t *testing.T) {
	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cache",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationCacheHash:         "abc123",
				v1alpha1.AnnotationCacheMountSubpath: "torch_compile_cache/abc123",
				v1alpha1.AnnotationCacheRootEnv:      "VLLM_CACHE_ROOT=/home/kserve/.cache/vllm",
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image: "quay.io/test/cache:latest",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
			Annotations: map[string]string{
				constants.KernelCachePVCNameAnnotationKey: "test-cache-pvc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	// First injection
	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err, "First InjectKernelCache should succeed")

	volumesAfterFirst := len(pod.Spec.Volumes)
	mountsAfterFirst := len(pod.Spec.Containers[0].VolumeMounts)
	envsAfterFirst := len(pod.Spec.Containers[0].Env)

	// Second injection (webhook may be called multiple times)
	err = injector.InjectKernelCache(pod)
	assert.NoError(t, err, "Second InjectKernelCache should succeed")

	// Verify no duplicates were added
	assert.Equal(t, volumesAfterFirst, len(pod.Spec.Volumes), "No duplicate volumes should be added")
	assert.Equal(t, mountsAfterFirst, len(pod.Spec.Containers[0].VolumeMounts), "No duplicate mounts should be added")
	assert.Equal(t, envsAfterFirst, len(pod.Spec.Containers[0].Env), "No duplicate env vars should be added")
}

// TestInjectKernelCache_CacheMissWritability verifies the mount configuration allows vLLM to rebuild caches on cache miss.
// This is a DESIGN verification test - actual filesystem writability requires integration testing.
func TestInjectKernelCache_CacheMissWritability(t *testing.T) {
	// REQUIREMENT: vLLM must be able to rebuild caches on cache miss, even though KernelCache PVC is read-only.
	//
	// DESIGN: Mount KernelCache PVC at a SPECIFIC SUBDIRECTORY under VLLM_CACHE_ROOT, not at the root itself.
	// This allows:
	// 1. Precompiled caches from PVC are readable (RO mount)
	// 2. Parent directories are container filesystem (RW by default)
	// 3. vLLM can write new caches to SIBLING directories of the mount
	//
	// Example filesystem after mount:
	//   /home/kserve/.cache/vllm/              ← Container FS (RW) - VLLM_CACHE_ROOT points here
	//     torch_compile_cache/                 ← Container FS (RW)
	//       torch_aot_compile/                 ← PVC mount (RO) - precompiled caches
	//         hashA/rank_0_0/model
	//         hashB/rank_0_0/model
	//       newHash/                           ← Container FS (RW) - vLLM writes here on cache miss!
	//
	// This test verifies the CONFIGURATION is correct. Actual writability requires integration tests.

	resolvedDigest := "sha256:abc123def456"
	imageSpec := "quay.io/test/vllm-cache:v1"

	// Use parent directory mounting (exposes multiple hashes, supports cache rebuilds)
	cacheMountSubpath := "torch_compile_cache/torch_aot_compile"

	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-hash-cache",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationCacheHash:         "hashA,hashB,hashC", // Multi-hash image
				v1alpha1.AnnotationCacheMountSubpath: cacheMountSubpath,
				v1alpha1.AnnotationCacheRootEnv:      "VLLM_CACHE_ROOT=/home/kserve/.cache/vllm",
				v1alpha1.AnnotationResolvedDigest:    resolvedDigest,
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image: imageSpec,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "multi-hash-cache",
			},
			Annotations: map[string]string{
				constants.KernelCachePVCNameAnnotationKey: "kernel-cache-pvc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err)

	// CRITICAL ASSERTIONS for cache-miss writability:

	// 1. Mount is at SPECIFIC SUBDIRECTORY, not at VLLM_CACHE_ROOT itself
	expectedMountPath := "/home/kserve/.cache/vllm/torch_compile_cache/torch_aot_compile"
	actualMountPath := pod.Spec.Containers[0].VolumeMounts[0].MountPath
	assert.Equal(t, expectedMountPath, actualMountPath,
		"Mount must be at specific subdirectory, not at VLLM_CACHE_ROOT")

	// 2. VLLM_CACHE_ROOT points to PARENT of mount, not the mount itself
	var vllmCacheRoot string
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == "VLLM_CACHE_ROOT" {
			vllmCacheRoot = env.Value
			break
		}
	}
	assert.Equal(t, "/home/kserve/.cache/vllm", vllmCacheRoot,
		"VLLM_CACHE_ROOT must point to parent directory")

	// 3. Verify mount path is UNDER VLLM_CACHE_ROOT (not equal or outside)
	assert.True(t, len(actualMountPath) > len(vllmCacheRoot),
		"Mount path must be under VLLM_CACHE_ROOT (subdirectory)")
	assert.True(t, actualMountPath[:len(vllmCacheRoot)] == vllmCacheRoot,
		"Mount path must start with VLLM_CACHE_ROOT")

	// 4. Mount is ReadOnly (precompiled caches)
	assert.True(t, pod.Spec.Containers[0].VolumeMounts[0].ReadOnly,
		"KernelCache mount must be ReadOnly")

	// 5. No conflicting RW mounts on parent directories
	// (This test has only one mount, so no conflicts)
	assert.Len(t, pod.Spec.Containers[0].VolumeMounts, 1,
		"Should have exactly one mount (no conflicting mounts on parents)")

	// CONCLUSION:
	// ✅ Precompiled caches (hashA, hashB, hashC) readable from RO mount
	// ✅ Parent directories (/home/kserve/.cache/vllm/torch_compile_cache/) are container FS (RW)
	// ✅ vLLM can write new caches to siblings: /home/kserve/.cache/vllm/torch_compile_cache/newHash/
	//
	// This design supports cache-miss rebuilds WITHOUT requiring emptyDir or additional RW mounts.
	// The container's root filesystem provides writability for sibling directories.
}

// TestInjectImageVolumeMount_BasicSuccess verifies basic image volume mounting with full metadata
func TestInjectImageVolumeMount_BasicSuccess(t *testing.T) {
	resolvedDigest := "sha256:abc123def456"
	imageSpec := "quay.io/test/vllm-cache:v1"

	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cache",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationCacheHash:         "hashA",
				v1alpha1.AnnotationCacheMountSubpath: "torch_compile_cache/torch_aot_compile",
				v1alpha1.AnnotationCacheRootEnv:      "VLLM_CACHE_ROOT=/home/kserve/.cache/vllm",
				v1alpha1.AnnotationResolvedDigest:    resolvedDigest,
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image:     imageSpec,
			MountType: v1alpha1.KernelCacheMountTypeImageVolume,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err, "InjectKernelCache should succeed with image volume")

	// Verify image volume was created with correct reference (digest)
	assert.Len(t, pod.Spec.Volumes, 1, "One image volume should be added")
	volume := pod.Spec.Volumes[0]
	assert.Equal(t, "kernel-cache", volume.Name)
	assert.NotNil(t, volume.Image, "Volume should be an image volume")

	expectedImageRef := "quay.io/test/vllm-cache@sha256:abc123def456"
	assert.Equal(t, expectedImageRef, volume.Image.Reference,
		"Image reference should use resolved digest")
	assert.Equal(t, corev1.PullIfNotPresent, volume.Image.PullPolicy,
		"Default pull policy should be IfNotPresent")

	// Verify volume mount with correct subPath
	assert.Len(t, pod.Spec.Containers[0].VolumeMounts, 1, "One volume mount should be added")
	mount := pod.Spec.Containers[0].VolumeMounts[0]
	assert.Equal(t, "kernel-cache", mount.Name)
	assert.Equal(t, "/home/kserve/.cache/vllm/torch_compile_cache/torch_aot_compile/hashA",
		mount.MountPath, "Mount path should be computed from labels")
	assert.Equal(t, "io.vllm.cache/torch_compile_cache/torch_aot_compile/hashA",
		mount.SubPath, "SubPath should be computed from OCI image labels")
	assert.True(t, mount.ReadOnly, "Image volumes must be read-only")

	// Verify environment variable from labels
	assert.Len(t, pod.Spec.Containers[0].Env, 1, "One env var should be set")
	env := pod.Spec.Containers[0].Env[0]
	assert.Equal(t, "VLLM_CACHE_ROOT", env.Name)
	assert.Equal(t, "/home/kserve/.cache/vllm", env.Value)
}

// TestInjectImageVolumeMount_MissingDigest verifies fallback when digest is missing
func TestInjectImageVolumeMount_MissingDigest(t *testing.T) {
	imageSpec := "quay.io/test/vllm-cache:v1"

	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cache",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationCacheHash:         "hashA",
				v1alpha1.AnnotationCacheMountSubpath: "torch_compile_cache/torch_aot_compile",
				v1alpha1.AnnotationCacheRootEnv:      "VLLM_CACHE_ROOT=/home/kserve/.cache/vllm",
				// Missing AnnotationResolvedDigest
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image:     imageSpec,
			MountType: v1alpha1.KernelCacheMountTypeImageVolume,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err, "Should succeed even without digest")

	// Verify original image is used when digest is missing
	volume := pod.Spec.Volumes[0]
	assert.Equal(t, imageSpec, volume.Image.Reference,
		"Should use original image reference when digest is missing")
}

// TestInjectImageVolumeMount_MissingLabels_LegacyFallback verifies fallback when OCI labels are missing
func TestInjectImageVolumeMount_MissingLabels_LegacyFallback(t *testing.T) {
	imageSpec := "quay.io/test/old-cache:v1"

	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-cache",
			Namespace:   "default",
			Annotations: map[string]string{
				// Missing all OCI label annotations
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image:     imageSpec,
			MountType: v1alpha1.KernelCacheMountTypeImageVolume,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err, "Should fall back gracefully when labels are missing")

	// Verify legacy mount behavior
	mount := pod.Spec.Containers[0].VolumeMounts[0]
	assert.Equal(t, "/mnt/kernel-cache", mount.MountPath,
		"Should fall back to legacy mount path when labels are missing")
	assert.Empty(t, mount.SubPath, "No subPath in legacy mode")

	// No environment variable in legacy mode
	assert.Empty(t, pod.Spec.Containers[0].Env,
		"No env vars should be set in legacy mode")
}

// TestInjectImageVolumeMount_CustomPullPolicy verifies custom pull policy is respected
func TestInjectImageVolumeMount_CustomPullPolicy(t *testing.T) {
	imageSpec := "quay.io/test/cache:latest"

	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cache",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationCacheHash:         "hashA",
				v1alpha1.AnnotationCacheMountSubpath: "cache_dir",
				v1alpha1.AnnotationCacheRootEnv:      "MY_CACHE=/opt/cache",
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image:           imageSpec,
			MountType:       v1alpha1.KernelCacheMountTypeImageVolume,
			ImagePullPolicy: corev1.PullAlways,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err)

	// Verify custom pull policy
	volume := pod.Spec.Volumes[0]
	assert.Equal(t, corev1.PullAlways, volume.Image.PullPolicy,
		"Custom pull policy should be respected")
}

// TestInjectImageVolumeMount_UserProvidedMountPath verifies user-provided MountPath overrides computed path
func TestInjectImageVolumeMount_UserProvidedMountPath(t *testing.T) {
	imageSpec := "quay.io/test/cache:v1"
	userMountPath := "/my/custom/mount"

	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cache",
			Namespace: "default",
			Annotations: map[string]string{
				// Even with labels present, user MountPath takes precedence
				v1alpha1.AnnotationCacheHash:         "hashA",
				v1alpha1.AnnotationCacheMountSubpath: "computed_path",
				v1alpha1.AnnotationCacheRootEnv:      "MY_VAR=/some/path",
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image:     imageSpec,
			MountType: v1alpha1.KernelCacheMountTypeImageVolume,
			MountPath: userMountPath,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err)

	// Verify user-provided mountPath is used
	mount := pod.Spec.Containers[0].VolumeMounts[0]
	assert.Equal(t, userMountPath, mount.MountPath,
		"User-provided MountPath should override computed path")
	// SubPath is still auto-computed from labels
	assert.Equal(t, "io.vllm.cache/computed_path/hashA", mount.SubPath,
		"SubPath should still be auto-computed from OCI labels")
}

// TestInjectImageVolumeMount_InvalidCacheRootEnv verifies error on invalid env format
func TestInjectImageVolumeMount_InvalidCacheRootEnv(t *testing.T) {
	imageSpec := "quay.io/test/cache:v1"

	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cache",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationCacheHash:         "hashA",
				v1alpha1.AnnotationCacheMountSubpath: "cache_dir",
				v1alpha1.AnnotationCacheRootEnv:      "INVALID_FORMAT", // Missing "=" separator
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image:     imageSpec,
			MountType: v1alpha1.KernelCacheMountTypeImageVolume,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.Error(t, err, "Should error on invalid cache-root-env format")
	assert.Contains(t, err.Error(), "invalid cache-root-env format")
}

// TestMountTypeBranchLogic_ImageVolume verifies mountType=imageVolume selects image volume injection
func TestMountTypeBranchLogic_ImageVolume(t *testing.T) {
	imageSpec := "quay.io/test/cache:v1"

	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cache",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationCacheHash:         "hashA",
				v1alpha1.AnnotationCacheMountSubpath: "cache_dir",
				v1alpha1.AnnotationCacheRootEnv:      "MY_CACHE=/opt/cache",
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image:     imageSpec,
			MountType: v1alpha1.KernelCacheMountTypeImageVolume,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err)

	// Verify image volume was created (not PVC)
	volume := pod.Spec.Volumes[0]
	assert.NotNil(t, volume.Image, "Should use image volume when mountType=imageVolume")
	assert.Nil(t, volume.PersistentVolumeClaim, "Should not use PVC when mountType=imageVolume")
}

// TestMountTypeBranchLogic_PVC verifies mountType=pvc selects PVC injection
func TestMountTypeBranchLogic_PVC(t *testing.T) {
	imageSpec := "quay.io/test/cache:v1"

	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cache",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationCacheHash:         "hashA",
				v1alpha1.AnnotationCacheMountSubpath: "cache_dir",
				v1alpha1.AnnotationCacheRootEnv:      "MY_CACHE=/opt/cache",
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image:     imageSpec,
			MountType: v1alpha1.KernelCacheMountTypePVC,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err)

	// Verify PVC was created (not image volume)
	volume := pod.Spec.Volumes[0]
	assert.NotNil(t, volume.PersistentVolumeClaim, "Should use PVC when mountType=pvc")
	assert.Nil(t, volume.Image, "Should not use image volume when mountType=pvc")
}

// TestMountTypeBranchLogic_EmptyDefaultsToPVC verifies empty mountType defaults to PVC
func TestMountTypeBranchLogic_EmptyDefaultsToPVC(t *testing.T) {
	imageSpec := "quay.io/test/cache:v1"

	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cache",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationCacheHash:         "hashA",
				v1alpha1.AnnotationCacheMountSubpath: "cache_dir",
				v1alpha1.AnnotationCacheRootEnv:      "MY_CACHE=/opt/cache",
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image: imageSpec,
			// MountType not set - should default to PVC
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err)

	// Verify PVC is used by default
	volume := pod.Spec.Volumes[0]
	assert.NotNil(t, volume.PersistentVolumeClaim, "Should default to PVC when mountType is empty")
	assert.Nil(t, volume.Image, "Should not use image volume when mountType is empty")
}

// TestInjectImageVolumeMount_PartialMetadata verifies graceful handling of partial metadata
func TestInjectImageVolumeMount_PartialMetadata(t *testing.T) {
	tests := []struct {
		name              string
		cacheHash         string
		cacheMountSubpath string
		cacheRootEnv      string
		expectedMountPath string
		expectedSubPath   string
		expectEnvVar      bool
		shouldError       bool
	}{
		{
			name:              "only hash present - legacy fallback",
			cacheHash:         "hashA",
			cacheMountSubpath: "",
			cacheRootEnv:      "",
			expectedMountPath: "/mnt/kernel-cache",
			expectedSubPath:   "",
			expectEnvVar:      false,
			shouldError:       false,
		},
		{
			name:              "only subpath present - legacy fallback",
			cacheHash:         "",
			cacheMountSubpath: "cache_dir",
			cacheRootEnv:      "",
			expectedMountPath: "/mnt/kernel-cache",
			expectedSubPath:   "",
			expectEnvVar:      false,
			shouldError:       false,
		},
		{
			name:              "only rootEnv present - legacy fallback",
			cacheHash:         "",
			cacheMountSubpath: "",
			cacheRootEnv:      "MY_VAR=/path",
			expectedMountPath: "/mnt/kernel-cache",
			expectedSubPath:   "",
			expectEnvVar:      false,
			shouldError:       false,
		},
		{
			name:              "hash and subpath but no rootEnv - legacy fallback",
			cacheHash:         "hashA",
			cacheMountSubpath: "cache_dir",
			cacheRootEnv:      "",
			expectedMountPath: "/mnt/kernel-cache",
			expectedSubPath:   "",
			expectEnvVar:      false,
			shouldError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageSpec := "quay.io/test/cache:v1"

			annotations := map[string]string{}
			if tt.cacheHash != "" {
				annotations[v1alpha1.AnnotationCacheHash] = tt.cacheHash
			}
			if tt.cacheMountSubpath != "" {
				annotations[v1alpha1.AnnotationCacheMountSubpath] = tt.cacheMountSubpath
			}
			if tt.cacheRootEnv != "" {
				annotations[v1alpha1.AnnotationCacheRootEnv] = tt.cacheRootEnv
			}

			kcCR := &v1alpha1.KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-cache",
					Namespace:   "default",
					Annotations: annotations,
				},
				Spec: v1alpha1.KernelCacheSpec{
					Image:     imageSpec,
					MountType: v1alpha1.KernelCacheMountTypeImageVolume,
				},
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						constants.KernelCacheLabel: "test-cache",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			}

			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			_ = v1alpha1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

			injector := &StorageInitializerInjector{
				credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
					Data: map[string]string{},
				}),
				config: storageInitializerConfig,
				client: fakeClient,
			}

			err := injector.InjectKernelCache(pod)

			if tt.shouldError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			mount := pod.Spec.Containers[0].VolumeMounts[0]
			assert.Equal(t, tt.expectedMountPath, mount.MountPath)
			assert.Equal(t, tt.expectedSubPath, mount.SubPath)

			if tt.expectEnvVar {
				assert.NotEmpty(t, pod.Spec.Containers[0].Env)
			} else {
				assert.Empty(t, pod.Spec.Containers[0].Env)
			}
		})
	}
}

// TestInjectImageVolumeMount_NoKserveContainer verifies graceful handling when kserve-container is missing
func TestInjectImageVolumeMount_NoKserveContainer(t *testing.T) {
	imageSpec := "quay.io/test/cache:v1"

	kcCR := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cache",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationCacheHash:         "hashA",
				v1alpha1.AnnotationCacheMountSubpath: "cache_dir",
				v1alpha1.AnnotationCacheRootEnv:      "MY_VAR=/path",
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image:     imageSpec,
			MountType: v1alpha1.KernelCacheMountTypeImageVolume,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "test-cache",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "some-other-container", // Not kserve-container
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcCR).Build()

	injector := &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(fakeClient, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config: storageInitializerConfig,
		client: fakeClient,
	}

	err := injector.InjectKernelCache(pod)
	assert.NoError(t, err, "Should not error when kserve-container is missing")

	// Volume should still be added
	assert.Len(t, pod.Spec.Volumes, 1, "Volume should be added even if container is missing")

	// No mounts should be added to the wrong container
	assert.Empty(t, pod.Spec.Containers[0].VolumeMounts,
		"No mounts should be added to non-kserve containers")
}
