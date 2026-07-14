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
