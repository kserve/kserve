/*
Copyright 2025 The KServe Authors.

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
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// attachKVCacheSecondaryTiers injects a volume and container mount for each
// secondary KV cache tier defined in the spec. It mirrors attachModelArtifacts
// in that it operates on all pods (leader and workers) in both single-node and
// multi-node deployments.
func attachKVCacheSecondaryTiers(podSpec *corev1.PodSpec, secondary []v1alpha2.SecondaryTierSpec, containerName string) {
	for i, s := range secondary {
		if s.FileSystem == nil {
			continue
		}
		volumeName := fmt.Sprintf("kv-cache-secondary-%d", i)
		mountPath := s.FileSystem.MountPath
		if mountPath == "" {
			mountPath = fmt.Sprintf("/mnt/kv-cache-%d", i)
		}
		attachFileSystemKVCacheTier(podSpec, s.FileSystem, volumeName, mountPath, containerName)
	}
}

// attachFileSystemKVCacheTier adds a single filesystem-backed KV cache volume to podSpec.
func attachFileSystemKVCacheTier(podSpec *corev1.PodSpec, fs *v1alpha2.FileSystemTierSpec, volumeName, mountPath, containerName string) {
	var volumeSource corev1.VolumeSource
	var subPath string

	switch {
	case fs.EmptyDir != nil:
		sizeLimit := fs.EmptyDir.Size.DeepCopy()
		volumeSource = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &sizeLimit},
		}
	case fs.PVC != nil:
		volumeSource = corev1.VolumeSource{
			Ephemeral: &corev1.EphemeralVolumeSource{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes:      fs.PVC.AccessModes,
						StorageClassName: fs.PVC.StorageClassName,
						Resources:        fs.PVC.Resources,
					},
				},
			},
		}
	case fs.Ref != nil:
		volumeSource = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: fs.Ref.Name,
			},
		}
		subPath = fs.Ref.Path
	default:
		return
	}

	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name:         volumeName,
		VolumeSource: volumeSource,
	})
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == containerName {
			podSpec.Containers[i].VolumeMounts = append(podSpec.Containers[i].VolumeMounts,
				corev1.VolumeMount{Name: volumeName, MountPath: mountPath, SubPath: subPath})
			break
		}
	}
}

// kvCacheFileSystemSize returns the storage size in bytes for a filesystem tier,
// or 0 if the size cannot be determined (e.g. for a Ref tier with no declared size).
func kvCacheFileSystemSize(fs *v1alpha2.FileSystemTierSpec) int64 {
	switch {
	case fs.EmptyDir != nil:
		return fs.EmptyDir.Size.Value()
	case fs.PVC != nil:
		if storage, ok := fs.PVC.Resources.Requests[corev1.ResourceStorage]; ok {
			return storage.Value()
		}
	}
	return 0
}
