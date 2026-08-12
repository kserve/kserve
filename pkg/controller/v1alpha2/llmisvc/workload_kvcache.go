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
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

// sharedMemoryMountPath is where the base preset mounts the tmpfs shared-memory
// volume that vLLM uses for both NCCL and CPU KV cache offload buffer.
const sharedMemoryMountPath = "/dev/shm"

// cpuKVCacheHeadroomPercent adds 20% headroom on top of the user-requested CPU
// KV cache size so the NCCL + offloading buffers do not exhaust /dev/shm and
// crash vLLM at startup.
const cpuKVCacheHeadroomPercent = 120

func attachKVCacheOffloading(podSpec *corev1.PodSpec, kv *v1alpha2.KVCacheOffloadingSpec) {
	const containerName = "main"
	attachKVCachePrimaryTier(podSpec, kv.CPU, containerName)
	attachKVCacheSecondaryTiers(podSpec, kv.Secondary, containerName)
}

func attachKVCachePrimaryTier(podSpec *corev1.PodSpec, cpu resource.Quantity, containerName string) {
	c := utils.GetContainerWithName(podSpec, containerName)
	if c == nil {
		return
	}

	required := *resource.NewQuantity(cpu.Value()*cpuKVCacheHeadroomPercent/100, cpu.Format)
	bumpSharedMemoryVolume(podSpec, c, required)
	bumpResourcesMemory(c, required)
}

func bumpSharedMemoryVolume(podSpec *corev1.PodSpec, c *corev1.Container, size resource.Quantity) {
	var name string
	for _, m := range c.VolumeMounts {
		if m.MountPath == sharedMemoryMountPath {
			name = m.Name
			break
		}
	}
	if name == "" {
		return
	}
	for i := range podSpec.Volumes {
		v := &podSpec.Volumes[i]
		if v.Name != name {
			continue
		}
		if v.EmptyDir != nil && v.EmptyDir.Medium == corev1.StorageMediumMemory && v.EmptyDir.SizeLimit != nil {
			sizeLimit := v.EmptyDir.SizeLimit.DeepCopy()
			sizeLimit.Add(size)
			v.EmptyDir.SizeLimit = &sizeLimit
		}
		return
	}
}

func bumpResourcesMemory(c *corev1.Container, size resource.Quantity) {
	if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
		req.Add(size)
		c.Resources.Requests[corev1.ResourceMemory] = req
	}
	if lim, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
		lim.Add(size)
		c.Resources.Limits[corev1.ResourceMemory] = lim
	}
}

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
		mountPath := fmt.Sprintf("/mnt/kv-cache-%d", i)
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
	case fs.PVC != nil && fs.PVC.Spec != nil:
		volumeSource = corev1.VolumeSource{
			Ephemeral: &corev1.EphemeralVolumeSource{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
					Spec: *fs.PVC.Spec,
				},
			},
		}
	case fs.PVC != nil && fs.PVC.Ref != nil:
		volumeSource = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: fs.PVC.Ref.Name,
			},
		}
		subPath = fs.PVC.Ref.Path
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
			if fs.EmptyDir != nil {
				// Request ephemeral-storage equal to the emptyDir size so the scheduler
				// accounts for the disk space and avoids placing the pod on a node with
				// insufficient local storage.
				if podSpec.Containers[i].Resources.Requests == nil {
					podSpec.Containers[i].Resources.Requests = corev1.ResourceList{}
				}
				existing := podSpec.Containers[i].Resources.Requests[corev1.ResourceEphemeralStorage]
				existing.Add(fs.EmptyDir.Size)
				podSpec.Containers[i].Resources.Requests[corev1.ResourceEphemeralStorage] = existing
			}
			break
		}
	}
}
