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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/kernelcachecommon"
)

const (
	// CacheCaptureLabelKey is the label key that references a KernelCacheCapture name
	CacheCaptureLabelKey = "serving.kserve.io/cache-capture"

	// CacheCaptureAnnotationCachePath stores the resolved cache path for the controller
	CacheCaptureAnnotationCachePath = "serving.kserve.io/cache-path"

	// CacheCaptureAnnotationKCCName stores the KCC name for the controller
	CacheCaptureAnnotationKCCName = "serving.kserve.io/kcc-name"

	// CacheCaptureContainerName is the name of the MCV sidecar container
	CacheCaptureContainerName = "cache-capture"

	// CacheCaptureVolumeN is the name of the shared cache volume
	CacheCaptureVolumeName = "cache-capture-volume"

	// RegistryCredsVolumeName is the name of the registry credentials volume
	RegistryCredsVolumeName = "registry-creds"

)

// CacheCaptureInjector injects MCV sidecar for cache capture
type CacheCaptureInjector struct {
	client    client.Client
	clientset kubernetes.Interface
}

// NewCacheCaptureInjector creates a new CacheCaptureInjector
func NewCacheCaptureInjector(client client.Client, clientset kubernetes.Interface) *CacheCaptureInjector {
	return &CacheCaptureInjector{
		client:    client,
		clientset: clientset,
	}
}

// InjectCacheCaptureSidecar injects the MCV sidecar if the pod has the cache-capture label.
// The configMap parameter is the already-fetched inferenceservice-config ConfigMap.
func (cci *CacheCaptureInjector) InjectCacheCaptureSidecar(ctx context.Context, pod *corev1.Pod, configMap *corev1.ConfigMap) error {
	// Check if pod has cache-capture label
	kccName, ok := pod.Labels[CacheCaptureLabelKey]
	if !ok {
		// No label, nothing to inject
		return nil
	}

	// Check if sidecar is already injected (avoid duplicate injection on webhook reinvocation)
	for _, container := range pod.Spec.Containers {
		if container.Name == CacheCaptureContainerName {
			log.Info("Cache capture sidecar already injected, skipping",
				"pod", pod.Name,
				"namespace", pod.Namespace)
			return nil
		}
	}

	log.Info("Detected cache-capture label on pod",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"kcc", kccName)

	// Parse KernelCache config
	kernelCacheConfig, err := v1beta1.NewKernelCacheConfig(configMap)
	if err != nil {
		log.Error(err, "Failed to parse kernelcache config", "pod", pod.Name)
		return nil
	}

	if !kernelCacheConfig.Enabled {
		log.Info("KernelCache feature is disabled - skipping cache capture sidecar injection. "+
			"To use KernelCacheCapture, enable the feature in the inferenceservice-config ConfigMap (kernelcache.enabled=true)",
			"pod", pod.Name,
			"kcc", kccName)
		return nil
	}

	captureImage := kernelCacheConfig.CaptureImage
	if captureImage == "" {
		captureImage = kernelcachecommon.DefaultCaptureImage
	}

	// Fetch the KernelCacheCapture CR using a direct API call (not the cached client,
	// which would require cluster-scoped list/watch RBAC for KCC)
	kcc := &v1alpha1.KernelCacheCapture{}
	result, err := cci.clientset.Discovery().RESTClient().Get().
		AbsPath("/apis/serving.kserve.io/v1alpha1/namespaces/" + pod.Namespace + "/kernelcachecaptures/" + kccName).
		DoRaw(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("KernelCacheCapture not found - skipping cache capture sidecar injection. "+
				"The InferenceService validation should have prevented this. "+
				"If the label was added manually to the pod, it will not work (labels must be on the InferenceService at creation time)",
				"pod", pod.Name,
				"kcc", kccName,
				"namespace", pod.Namespace)
		} else {
			log.Error(err, "Failed to get KernelCacheCapture - skipping sidecar injection",
				"pod", pod.Name,
				"kcc", kccName,
				"namespace", pod.Namespace)
		}
		return nil
	}
	if err := json.Unmarshal(result, kcc); err != nil {
		log.Error(err, "Failed to unmarshal KernelCacheCapture",
			"pod", pod.Name,
			"kcc", kccName)
		return nil
	}

	// Resolve cache path from KCC
	cachePath := ResolveCachePath(kcc.Spec.CachePreset, kcc.Spec.CachePath)
	if cachePath == "" {
		return fmt.Errorf("failed to resolve cache path from KCC %s: no preset or explicit path specified", kccName)
	}

	log.Info("Resolved cache path",
		"kcc", kccName,
		"preset", kcc.Spec.CachePreset,
		"explicitPath", kcc.Spec.CachePath,
		"resolvedPath", cachePath)

	// Add annotations for controller
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[CacheCaptureAnnotationCachePath] = cachePath
	pod.Annotations[CacheCaptureAnnotationKCCName] = kccName

	// Inject shared volume if volumeStrategy=shared
	if kcc.Spec.VolumeStrategy == "shared" {
		cci.injectSharedVolume(pod, cachePath)
	}

	// Inject MCV sidecar
	cci.injectMCVSidecar(pod, kcc, cachePath, captureImage)

	log.Info("Successfully injected cache capture sidecar",
		"pod", pod.Name,
		"kcc", kccName,
		"cachePath", cachePath,
		"volumeStrategy", kcc.Spec.VolumeStrategy)

	return nil
}

// injectSharedVolume injects the shared emptyDir volume and mounts it to the main container
func (cci *CacheCaptureInjector) injectSharedVolume(pod *corev1.Pod, cachePath string) {
	// Check if there's already a volume mounted exactly at cachePath
	var volumeExistsAtPath bool
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == constants.InferenceServiceContainerName {
			for _, vm := range pod.Spec.Containers[i].VolumeMounts {
				if vm.MountPath == cachePath {
					volumeExistsAtPath = true
					log.Info("Found existing volume mounted at cache path",
						"volumeName", vm.Name,
						"mountPath", vm.MountPath)
					break
				}
			}
			break
		}
	}

	if volumeExistsAtPath {
		// Volume already exists at exact path (e.g., from previous webhook)
		// Don't create duplicate
		log.Info("Skipping emptyDir creation - volume already mounted at cachePath")
		return
	}

	// Create shared emptyDir at cachePath
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: CacheCaptureVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	// Mount to kserve-container (the main predictor container)
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == constants.InferenceServiceContainerName {
			pod.Spec.Containers[i].VolumeMounts = append(pod.Spec.Containers[i].VolumeMounts, corev1.VolumeMount{
				Name:      CacheCaptureVolumeName,
				MountPath: cachePath,
			})

			// Set cache root environment variable (framework-specific)
			// For vLLM: VLLM_CACHE_ROOT
			// For now, just add a generic env var
			pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, corev1.EnvVar{
				Name:  "CACHE_ROOT",
				Value: cachePath,
			})

			log.Info("Mounted shared volume to main container",
				"container", pod.Spec.Containers[i].Name,
				"mountPath", cachePath)
			break
		}
	}
}

// injectMCVSidecar injects the MCV sidecar container
func (cci *CacheCaptureInjector) injectMCVSidecar(pod *corev1.Pod, kcc *v1alpha1.KernelCacheCapture, cachePath, captureImage string) {
	sidecar := corev1.Container{
		Name:            CacheCaptureContainerName,
		Image:           captureImage,
		ImagePullPolicy: corev1.PullAlways,
		Command:         []string{"/bin/sh", "-c"},
		Args: []string{`
# Add user to /etc/passwd (K8s doesn't auto-add like podman does)
if ! grep -q "^mcv:" /etc/passwd; then
  echo "mcv:x:1000:0:MCV User:/var/lib/containers:/bin/bash" >> /etc/passwd
fi

echo "=== MCV Cache Capture Sidecar Ready ==="
echo "KCC: ` + kcc.Name + `"
echo "Target Image: ` + kcc.Spec.TargetImage + `"
echo "Cache Path: ` + cachePath + `"
echo "Volume Strategy: ` + kcc.Spec.VolumeStrategy + `"
echo ""
echo "Waiting for trigger..."
while true; do sleep 3600; done
`},
		Env: []corev1.EnvVar{
			{
				Name:  "STORAGE_DRIVER",
				Value: "vfs", // Use vfs driver (no FUSE/privilege needed)
			},
			{
				Name:  "DETECTED_CACHE_PATH",
				Value: cachePath,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("512Mi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("2Gi"),
				corev1.ResourceCPU:    resource.MustParse("2000m"),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             &[]bool{true}[0],
			RunAsUser:                &[]int64{1000}[0],
			AllowPrivilegeEscalation: &[]bool{false}[0],
			// ReadOnlyRootFilesystem NOT set - buildah needs writable filesystem
			// Security comes from RunAsUser (non-root) and VFS storage driver
		},
	}

	// Add volume mounts
	if kcc.Spec.VolumeStrategy == "shared" {
		// Mount the shared emptyDir volume
		sidecar.VolumeMounts = append(sidecar.VolumeMounts, corev1.VolumeMount{
			Name:      CacheCaptureVolumeName,
			MountPath: cachePath,
			ReadOnly:  true,
		})

		// Check if pod has a KernelCache label - if so, mount the KC imageVolume too
		// This is needed for testing with pre-seeded cache (GPU-less testing)
		log.Info("Checking for KernelCache label",
			"hasLabels", pod.Labels != nil,
			"labelCount", len(pod.Labels),
			"kcLabelKey", constants.KernelCacheLabel)
		if kcLabel, hasKC := pod.Labels[constants.KernelCacheLabel]; hasKC {
			log.Info("Pod has KernelCache label, will mount KC imageVolume in sidecar",
				"kernelcache", kcLabel)

			// Find the KC imageVolume mount from pod.Spec.Volumes
			// Look for volumes with name containing "kernel-cache"
			for _, vol := range pod.Spec.Volumes {
				if strings.Contains(vol.Name, "kernel-cache") {
					// Find the corresponding mount in the predictor to get the mountPath and subPath
					for i := range pod.Spec.Containers {
						if pod.Spec.Containers[i].Name == constants.InferenceServiceContainerName {
							for _, vm := range pod.Spec.Containers[i].VolumeMounts {
								if vm.Name == vol.Name {
									// Mount the same KC volume in sidecar at the same path
									sidecar.VolumeMounts = append(sidecar.VolumeMounts, corev1.VolumeMount{
										Name:      vm.Name,
										MountPath: vm.MountPath,
										SubPath:   vm.SubPath,
										ReadOnly:  true,
									})
									log.Info("Mounted KernelCache imageVolume to sidecar",
										"volumeName", vm.Name,
										"mountPath", vm.MountPath,
										"subPath", vm.SubPath)
									break
								}
							}
							break
						}
					}
					break
				}
			}
		}

		log.Info("Mounted cache volumes to sidecar", "cachePath", cachePath)
	}

	// Registry credentials (optional - only for authenticated registries)
	if kcc.Spec.RegistrySecretRef != nil {
		sidecar.VolumeMounts = append(sidecar.VolumeMounts, corev1.VolumeMount{
			Name:      RegistryCredsVolumeName,
			MountPath: "/var/run/secrets/registry",
			ReadOnly:  true,
		})

		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: RegistryCredsVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  kcc.Spec.RegistrySecretRef.Name,
					DefaultMode: &[]int32{0o400}[0],
				},
			},
		})

		log.Info("Mounted registry credentials",
			"pod", pod.Name,
			"secret", kcc.Spec.RegistrySecretRef.Name)
	}

	// Add sidecar to pod
	pod.Spec.Containers = append(pod.Spec.Containers, sidecar)

	log.Info("Injected MCV sidecar container",
		"pod", pod.Name,
		"image", captureImage,
		"volumeStrategy", kcc.Spec.VolumeStrategy)
}

