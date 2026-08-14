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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/kernelcachecommon"
)

func TestInjectKernelCacheCaptureSidecar_NoLabel(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewCacheCaptureInjector(fakeClient, nil)

	configMap := &corev1.ConfigMap{
		Data: map[string]string{
			"kernelcache": `{"enabled": true}`,
		},
	}

	err := injector.InjectCacheCaptureSidecar(context.Background(), pod, configMap)
	assert.NoError(t, err)
	assert.Len(t, pod.Spec.Containers, 1, "No sidecar should be added without label")
	assert.Empty(t, pod.Spec.Volumes, "No volumes should be added without label")
}

func TestInjectKernelCacheCaptureSidecar_AlreadyInjected(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				CacheCaptureLabelKey: "my-kcc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
				{Name: CacheCaptureContainerName},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewCacheCaptureInjector(fakeClient, nil)

	configMap := &corev1.ConfigMap{
		Data: map[string]string{
			"kernelcache": `{"enabled": true}`,
		},
	}

	err := injector.InjectCacheCaptureSidecar(context.Background(), pod, configMap)
	assert.NoError(t, err)
	assert.Len(t, pod.Spec.Containers, 2, "Should not inject duplicate sidecar")
}

func TestInjectKernelCacheCaptureSidecar_FeatureDisabled(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				CacheCaptureLabelKey: "my-kcc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewCacheCaptureInjector(fakeClient, nil)

	configMap := &corev1.ConfigMap{
		Data: map[string]string{
			"kernelcache": `{"enabled": false}`,
		},
	}

	err := injector.InjectCacheCaptureSidecar(context.Background(), pod, configMap)
	assert.NoError(t, err)
	assert.Len(t, pod.Spec.Containers, 1, "Should not inject sidecar when feature is disabled")
}

// newTestKCCServer creates a test HTTP server that serves KCC objects at the expected API path
func newTestKCCServer(t *testing.T, kcc *v1alpha1.KernelCacheCapture) (*httptest.Server, kubernetes.Interface) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/apis/serving.kserve.io/v1alpha1/namespaces/" + kcc.Namespace + "/kernelcachecaptures/" + kcc.Name
		if r.URL.Path == expectedPath {
			w.Header().Set("Content-Type", "application/json")
			data, _ := json.Marshal(kcc)
			w.Write(data)
			return
		}
		// Return 404 for unknown paths
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","reason":"NotFound","code":404}`))
	}))

	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("Failed to create clientset: %v", err)
	}
	return server, clientset
}

func TestInjectKernelCacheCaptureSidecar_FullInjection_SharedVolume(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage:    "kind-registry:5000/my-cache:v1",
			CachePreset:    "vllm",
			VolumeStrategy: "shared",
		},
	}

	server, testClientset := newTestKCCServer(t, kcc)
	defer server.Close()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				CacheCaptureLabelKey: "test-kcc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewCacheCaptureInjector(fakeClient, testClientset)

	configMap := &corev1.ConfigMap{
		Data: map[string]string{
			"kernelcache": `{"enabled": true}`,
		},
	}

	err := injector.InjectCacheCaptureSidecar(context.Background(), pod, configMap)
	assert.NoError(t, err)

	// Verify sidecar container injected
	assert.Len(t, pod.Spec.Containers, 2, "Sidecar should be injected")
	sidecar := pod.Spec.Containers[1]
	assert.Equal(t, CacheCaptureContainerName, sidecar.Name)
	assert.Equal(t, kernelcachecommon.DefaultCaptureImage, sidecar.Image)

	// Verify security context
	assert.NotNil(t, sidecar.SecurityContext)
	assert.True(t, *sidecar.SecurityContext.RunAsNonRoot)
	assert.Equal(t, int64(1000), *sidecar.SecurityContext.RunAsUser)
	assert.False(t, *sidecar.SecurityContext.AllowPrivilegeEscalation)

	// Verify shared volume created
	var cacheVolume *corev1.Volume
	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].Name == CacheCaptureVolumeName {
			cacheVolume = &pod.Spec.Volumes[i]
			break
		}
	}
	assert.NotNil(t, cacheVolume, "Shared cache volume should be created")
	assert.NotNil(t, cacheVolume.EmptyDir)

	// Verify main container has cache volume mount
	mainContainer := pod.Spec.Containers[0]
	var mainMount *corev1.VolumeMount
	for i := range mainContainer.VolumeMounts {
		if mainContainer.VolumeMounts[i].Name == CacheCaptureVolumeName {
			mainMount = &mainContainer.VolumeMounts[i]
			break
		}
	}
	assert.NotNil(t, mainMount, "Main container should have cache volume mount")
	assert.Equal(t, "/home/kserve/.cache/vllm", mainMount.MountPath)

	// Verify sidecar has read-only cache volume mount
	var sidecarMount *corev1.VolumeMount
	for i := range sidecar.VolumeMounts {
		if sidecar.VolumeMounts[i].Name == CacheCaptureVolumeName {
			sidecarMount = &sidecar.VolumeMounts[i]
			break
		}
	}
	assert.NotNil(t, sidecarMount, "Sidecar should have cache volume mount")
	assert.Equal(t, "/home/kserve/.cache/vllm", sidecarMount.MountPath)
	assert.True(t, sidecarMount.ReadOnly)

	// Verify annotations
	assert.Equal(t, "/home/kserve/.cache/vllm", pod.Annotations[CacheCaptureAnnotationCachePath])
	assert.Equal(t, "test-kcc", pod.Annotations[CacheCaptureAnnotationKCCName])

	// Verify CACHE_ROOT env var on main container
	var cacheRootEnv *corev1.EnvVar
	for i := range mainContainer.Env {
		if mainContainer.Env[i].Name == "CACHE_ROOT" {
			cacheRootEnv = &mainContainer.Env[i]
			break
		}
	}
	assert.NotNil(t, cacheRootEnv)
	assert.Equal(t, "/home/kserve/.cache/vllm", cacheRootEnv.Value)

	// Verify STORAGE_DRIVER env on sidecar
	var storageDriverEnv *corev1.EnvVar
	for i := range sidecar.Env {
		if sidecar.Env[i].Name == "STORAGE_DRIVER" {
			storageDriverEnv = &sidecar.Env[i]
			break
		}
	}
	assert.NotNil(t, storageDriverEnv)
	assert.Equal(t, "vfs", storageDriverEnv.Value)
}

func TestInjectKernelCacheCaptureSidecar_WithRegistrySecret(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage:    "quay.io/myorg/cache:v1",
			CachePreset:    "vllm",
			VolumeStrategy: "shared",
			RegistrySecretRef: &v1alpha1.SecretKeySelector{
				Name: "quay-push-secret",
				Key:  ".dockerconfigjson",
			},
		},
	}

	server, testClientset := newTestKCCServer(t, kcc)
	defer server.Close()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				CacheCaptureLabelKey: "test-kcc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewCacheCaptureInjector(fakeClient, testClientset)

	configMap := &corev1.ConfigMap{
		Data: map[string]string{
			"kernelcache": `{"enabled": true}`,
		},
	}

	err := injector.InjectCacheCaptureSidecar(context.Background(), pod, configMap)
	assert.NoError(t, err)

	// Verify registry credentials volume
	var credsVolume *corev1.Volume
	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].Name == RegistryCredsVolumeName {
			credsVolume = &pod.Spec.Volumes[i]
			break
		}
	}
	assert.NotNil(t, credsVolume, "Registry credentials volume should be created")
	assert.NotNil(t, credsVolume.Secret)
	assert.Equal(t, "quay-push-secret", credsVolume.Secret.SecretName)

	// Verify sidecar has credentials mount
	sidecar := pod.Spec.Containers[1]
	var credsMount *corev1.VolumeMount
	for i := range sidecar.VolumeMounts {
		if sidecar.VolumeMounts[i].Name == RegistryCredsVolumeName {
			credsMount = &sidecar.VolumeMounts[i]
			break
		}
	}
	assert.NotNil(t, credsMount, "Sidecar should have credentials mount")
	assert.Equal(t, "/var/run/secrets/registry", credsMount.MountPath)
	assert.True(t, credsMount.ReadOnly)
}

func TestInjectKernelCacheCaptureSidecar_KCCNotFound(t *testing.T) {
	// Server returns 404 for all paths
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","reason":"NotFound","code":404}`))
	}))
	defer server.Close()

	testClientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	assert.NoError(t, err)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				CacheCaptureLabelKey: "nonexistent-kcc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewCacheCaptureInjector(fakeClient, testClientset)

	configMap := &corev1.ConfigMap{
		Data: map[string]string{
			"kernelcache": `{"enabled": true}`,
		},
	}

	err = injector.InjectCacheCaptureSidecar(context.Background(), pod, configMap)
	assert.NoError(t, err, "KCC not found should not return error (graceful skip)")
	assert.Len(t, pod.Spec.Containers, 1, "No sidecar should be injected when KCC not found")
}

func TestInjectKernelCacheCaptureSidecar_ExplicitCachePath(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage:    "kind-registry:5000/cache:v1",
			CachePath:      "/custom/cache/path",
			VolumeStrategy: "shared",
		},
	}

	server, testClientset := newTestKCCServer(t, kcc)
	defer server.Close()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				CacheCaptureLabelKey: "test-kcc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewCacheCaptureInjector(fakeClient, testClientset)

	configMap := &corev1.ConfigMap{
		Data: map[string]string{
			"kernelcache": `{"enabled": true}`,
		},
	}

	err := injector.InjectCacheCaptureSidecar(context.Background(), pod, configMap)
	assert.NoError(t, err)

	assert.Equal(t, "/custom/cache/path", pod.Annotations[CacheCaptureAnnotationCachePath])

	// Verify main container mount at custom path
	mainContainer := pod.Spec.Containers[0]
	var mainMount *corev1.VolumeMount
	for i := range mainContainer.VolumeMounts {
		if mainContainer.VolumeMounts[i].Name == CacheCaptureVolumeName {
			mainMount = &mainContainer.VolumeMounts[i]
			break
		}
	}
	assert.NotNil(t, mainMount)
	assert.Equal(t, "/custom/cache/path", mainMount.MountPath)
}

func TestInjectKernelCacheCaptureSidecar_WithKernelCacheLabel(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage:    "kind-registry:5000/cache:v1",
			CachePreset:    "vllm",
			VolumeStrategy: "shared",
		},
	}

	server, testClientset := newTestKCCServer(t, kcc)
	defer server.Close()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				CacheCaptureLabelKey:       "test-kcc",
				constants.KernelCacheLabel: "existing-kc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "kernel-cache-vol",
							MountPath: "/home/kserve/.cache/vllm/torch_compile_cache",
							SubPath:   "some/subpath",
							ReadOnly:  true,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "kernel-cache-vol",
					VolumeSource: corev1.VolumeSource{
						Image: &corev1.ImageVolumeSource{
							Reference: "quay.io/test/cache@sha256:abc123",
						},
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewCacheCaptureInjector(fakeClient, testClientset)

	configMap := &corev1.ConfigMap{
		Data: map[string]string{
			"kernelcache": `{"enabled": true}`,
		},
	}

	err := injector.InjectCacheCaptureSidecar(context.Background(), pod, configMap)
	assert.NoError(t, err)

	// Verify sidecar has the KC imageVolume mounted (for pre-seeded cache testing)
	sidecar := pod.Spec.Containers[1]
	assert.True(t, len(sidecar.VolumeMounts) >= 2, "Sidecar should have cache-capture volume + KC imageVolume")
}

func TestInjectKernelCacheCaptureSidecar_ExistingVolumeAtPath(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage:    "kind-registry:5000/cache:v1",
			CachePreset:    "vllm",
			VolumeStrategy: "shared",
		},
	}

	server, testClientset := newTestKCCServer(t, kcc)
	defer server.Close()

	// Pod already has a volume at the vllm cache path
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				CacheCaptureLabelKey: "test-kcc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: constants.InferenceServiceContainerName,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "existing-vol",
							MountPath: "/home/kserve/.cache/vllm",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "existing-vol",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewCacheCaptureInjector(fakeClient, testClientset)

	configMap := &corev1.ConfigMap{
		Data: map[string]string{
			"kernelcache": `{"enabled": true}`,
		},
	}

	err := injector.InjectCacheCaptureSidecar(context.Background(), pod, configMap)
	assert.NoError(t, err)

	// Should not create duplicate volume — only the existing volume should be present
	volumeCount := 0
	for _, vol := range pod.Spec.Volumes {
		if vol.EmptyDir != nil {
			volumeCount++
		}
	}
	assert.Equal(t, 1, volumeCount, "Should not create duplicate emptyDir when volume already exists at cache path")
}

func TestInjectKernelCacheCaptureSidecar_SidecarResources(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage:    "kind-registry:5000/cache:v1",
			CachePreset:    "vllm",
			VolumeStrategy: "shared",
		},
	}

	server, testClientset := newTestKCCServer(t, kcc)
	defer server.Close()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				CacheCaptureLabelKey: "test-kcc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewCacheCaptureInjector(fakeClient, testClientset)

	configMap := &corev1.ConfigMap{
		Data: map[string]string{
			"kernelcache": `{"enabled": true}`,
		},
	}

	err := injector.InjectCacheCaptureSidecar(context.Background(), pod, configMap)
	assert.NoError(t, err)

	sidecar := pod.Spec.Containers[1]

	// Verify resource requests
	memReq := sidecar.Resources.Requests[corev1.ResourceMemory]
	assert.Equal(t, "512Mi", memReq.String())
	cpuReq := sidecar.Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "500m", cpuReq.String())

	// Verify resource limits
	memLim := sidecar.Resources.Limits[corev1.ResourceMemory]
	assert.Equal(t, "2Gi", memLim.String())
	cpuLim := sidecar.Resources.Limits[corev1.ResourceCPU]
	assert.Equal(t, "2", cpuLim.String())
}

func TestInjectKernelCacheCaptureSidecar_CachePresets(t *testing.T) {
	tests := []struct {
		preset       string
		expectedPath string
	}{
		{"vllm", "/home/kserve/.cache/vllm"},
		{"tgi", "/data"},
		{"triton-python", "/opt/tritonserver/backends/python/models/.cache"},
	}

	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			kcc := &v1alpha1.KernelCacheCapture{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kcc",
					Namespace: "default",
				},
				Spec: v1alpha1.KernelCacheCaptureSpec{
					TargetImage:    "registry/cache:v1",
					CachePreset:    tt.preset,
					VolumeStrategy: "shared",
				},
			}

			server, testClientset := newTestKCCServer(t, kcc)
			defer server.Close()

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						CacheCaptureLabelKey: "test-kcc",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: constants.InferenceServiceContainerName},
					},
				},
			}

			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
			injector := NewCacheCaptureInjector(fakeClient, testClientset)

			configMap := &corev1.ConfigMap{
				Data: map[string]string{
					"kernelcache": `{"enabled": true}`,
				},
			}

			err := injector.InjectCacheCaptureSidecar(context.Background(), pod, configMap)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedPath, pod.Annotations[CacheCaptureAnnotationCachePath])
		})
	}
}

func TestInjectKernelCacheCaptureSidecar_CustomCaptureImage(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage:    "kind-registry:5000/cache:v1",
			CachePreset:    "vllm",
			VolumeStrategy: "shared",
		},
	}

	server, testClientset := newTestKCCServer(t, kcc)
	defer server.Close()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				CacheCaptureLabelKey: "test-kcc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewCacheCaptureInjector(fakeClient, testClientset)

	configMap := &corev1.ConfigMap{
		Data: map[string]string{
			"kernelcache": `{"enabled": true, "captureImage": "quay.io/custom/mcv:v2"}`,
		},
	}

	err := injector.InjectCacheCaptureSidecar(context.Background(), pod, configMap)
	assert.NoError(t, err)

	assert.Len(t, pod.Spec.Containers, 2, "Sidecar should be injected")
	sidecar := pod.Spec.Containers[1]
	assert.Equal(t, "quay.io/custom/mcv:v2", sidecar.Image, "Should use custom captureImage from ConfigMap")
}
