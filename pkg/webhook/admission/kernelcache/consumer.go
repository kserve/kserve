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

package kernelcache

import (
	"context"
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	kernelcacheutil "github.com/kserve/kserve/pkg/kernelcache"
)

const (
	kernelCacheSourceVolumeName = "kernel-cache-source"
	kernelCacheSourceMountPath  = "/var/run/gkm/kernel-cache"
	kernelCacheLinkerName       = "kernel-cache-linker"

	kernelCacheLinkerScript = `set -eu
echo "kernel-cache-linker: starting cache materialization"
while [ "$#" -gt 0 ]; do
    source="$1"
    target="$2"
    shift 2
    echo "kernel-cache-linker: linking $source to $target"
    if [ ! -d "$source" ]; then
        echo "kernel-cache-linker: source directory does not exist: $source" >&2
        exit 1
    fi
    mkdir -p "$target"
    cp -sr "$source"/. "$target"/
    echo "kernel-cache-linker: completed $target"
done
echo "kernel-cache-linker: cache materialization completed"`
)

func (m *PodMutator) injectKernelCacheArtifact(
	ctx context.Context,
	pod *corev1.Pod,
	cfg *v1beta1.KernelCacheConfig,
) (bool, error) {
	selection, found, err := m.findKernelCacheSelection(ctx, pod)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	mutatedPod := pod.DeepCopy()
	if err := injectKernelCacheMount(mutatedPod, selection.cache, cfg); err != nil {
		return false, err
	}
	*pod = *mutatedPod
	return true, nil
}

func (m *PodMutator) findKernelCacheSelection(ctx context.Context, pod *corev1.Pod) (*kernelCacheSelection, bool, error) {
	reader := m.Reader
	if reader == nil {
		reader = m.Client
	}

	workload, err := workloadRefForPod(pod)
	if err != nil {
		return nil, false, err
	}
	modelURI, known, err := resolveWorkloadModelSource(ctx, reader, pod, workload)
	if err != nil {
		return nil, false, err
	}
	if !known {
		return nil, false, nil
	}
	requested, err := identityForPod(pod, modelURI)
	if err != nil {
		return nil, false, err
	}
	if requested.Footprints.WorkloadFootprint == "" && requested.Footprints.CompatibilityFootprint == "" {
		return nil, false, nil
	}
	nodes := &v1alpha1.KernelCacheNodeList{}
	if err := reader.List(ctx, nodes); err != nil {
		return nil, false, err
	}
	candidates := readyKernelCacheCandidates(nodes, pod.Namespace)
	if len(candidates) == 0 {
		return nil, false, nil
	}

	if selection, found, err := selectKernelCacheCandidate(ctx, reader, pod, requested, candidates); err != nil {
		return nil, false, err
	} else if found {
		logger.Info("Matched KernelCache", "kernelCache", client.ObjectKeyFromObject(selection.cache), "matchType", selection.matchType)
		return selection, true, nil
	}
	return nil, false, nil
}

// selectKernelCacheCandidate prefers workload matches and then compatibility matches.
func selectKernelCacheCandidate(
	ctx context.Context,
	reader client.Reader,
	pod *corev1.Pod,
	requested v1alpha1.KernelCacheIdentity,
	candidates []kernelCacheNodeCandidate,
) (*kernelCacheSelection, bool, error) {
	for _, candidate := range candidates {
		if !candidateMatchesWorkload(requested, candidate) {
			continue
		}

		cache, found, err := loadKernelCacheCandidate(ctx, reader, pod, candidate)
		if err != nil {
			return nil, false, err
		}
		if found {
			return &kernelCacheSelection{cache: cache, matchType: kernelCacheMatchWorkload}, true, nil
		}
	}
	for _, candidate := range candidates {
		if !candidateMatchesCompatibility(requested, candidate) {
			continue
		}

		cache, found, err := loadKernelCacheCandidate(ctx, reader, pod, candidate)
		if err != nil {
			return nil, false, err
		}
		if found {
			return &kernelCacheSelection{cache: cache, matchType: kernelCacheMatchCompatibility}, true, nil
		}
	}
	return nil, false, nil
}

func loadKernelCacheCandidate(
	ctx context.Context,
	reader client.Reader,
	pod *corev1.Pod,
	candidate kernelCacheNodeCandidate,
) (*v1alpha1.KernelCache, bool, error) {
	cache := &v1alpha1.KernelCache{}
	key := client.ObjectKey{Namespace: candidate.ref.Namespace, Name: candidate.ref.Name}
	if err := reader.Get(ctx, key, cache); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !candidateMatchesKernelCache(candidate, cache) || !cacheCanMountToPod(cache, pod) {
		return nil, false, nil
	}
	return cache, true, nil
}

func injectKernelCacheMount(pod *corev1.Pod, kernelCache *v1alpha1.KernelCache, cfg *v1beta1.KernelCacheConfig) error {
	if len(kernelCache.Spec.Artifact.CachePaths) == 0 {
		return fmt.Errorf("KernelCache %s/%s has no cache paths", kernelCache.Namespace, kernelCache.Name)
	}
	if kernelCache.Spec.Artifact.ImageReference == "" {
		return fmt.Errorf("KernelCache %s/%s has no artifact image reference", kernelCache.Namespace, kernelCache.Name)
	}

	resolvedCachePaths := make([]v1alpha1.KernelCachePath, len(kernelCache.Spec.Artifact.CachePaths))
	containerIndexes := make(map[string]int, len(kernelCache.Spec.Artifact.CachePaths))
	for index, cachePath := range kernelCache.Spec.Artifact.CachePaths {
		containerName, err := kernelcacheutil.ResolveRuntimeContainerName(pod.Spec.Containers, cachePath.ContainerName)
		if err != nil {
			return err
		}
		cachePath.ContainerName = containerName
		containerIndex := findContainerIndex(pod.Spec.Containers, cachePath.ContainerName)
		if containerIndex < 0 {
			return fmt.Errorf("cache container %q was not found", cachePath.ContainerName)
		}
		containerPath, err := kernelcacheutil.ResolveContainerPath(&pod.Spec.Containers[containerIndex], cachePath.ContainerPath)
		if err != nil {
			return err
		}
		cachePath.ContainerPath = containerPath
		ociPath, err := kernelcacheutil.ResolveOCIPath(cachePath.OCIPath)
		if err != nil {
			return err
		}
		cachePath.OCIPath = ociPath
		resolvedCachePaths[index] = cachePath
		if err := validateCachePath(cachePath); err != nil {
			return err
		}
		containerIndexes[cachePath.ContainerName] = containerIndex
	}

	mountType := kernelCache.Spec.MountType
	if mountType == "" {
		mountType = v1alpha1.KernelCacheMountType(cfg.DefaultMountType)
	}
	if mountType == "" {
		mountType = v1alpha1.KernelCacheMountTypeOCI
	}
	if mountType != v1alpha1.KernelCacheMountTypeOCI {
		return fmt.Errorf("unsupported KernelCache mount type %q", mountType)
	}

	sourceVolume, err := kernelCacheSourceVolume(kernelCache, mountType)
	if err != nil {
		return err
	}
	if err := addKernelCacheVolume(&pod.Spec, sourceVolume); err != nil {
		return err
	}

	linkerArgs := []string{kernelCacheLinkerScript, kernelCacheLinkerName}
	linker := corev1.Container{
		Name:            kernelCacheLinkerName,
		Image:           kernelCacheLinkerImage(cfg),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-c"},
	}
	linker.VolumeMounts = append(linker.VolumeMounts, corev1.VolumeMount{
		Name:      kernelCacheSourceVolumeName,
		MountPath: kernelCacheSourceMountPath,
		ReadOnly:  true,
	})

	for index, cachePath := range resolvedCachePaths {
		container := &pod.Spec.Containers[containerIndexes[cachePath.ContainerName]]
		targetMount, err := ensureKernelCacheTargetMount(&pod.Spec, container, index, cachePath)
		if err != nil {
			return err
		}
		if err := addKernelCacheVolumeMount(&linker, targetMount, false); err != nil {
			return err
		}
		linkerArgs = append(linkerArgs,
			path.Join(kernelCacheSourceMountPath, path.Clean(cachePath.OCIPath)),
			cachePath.ContainerPath,
		)
		if err := addKernelCacheVolumeMount(container, corev1.VolumeMount{
			Name:      kernelCacheSourceVolumeName,
			MountPath: kernelCacheSourceMountPath,
			ReadOnly:  true,
		}, true); err != nil {
			return err
		}
	}
	linker.Args = linkerArgs

	if findContainerIndex(pod.Spec.InitContainers, kernelCacheLinkerName) < 0 {
		pod.Spec.InitContainers = append([]corev1.Container{linker}, pod.Spec.InitContainers...)
	}
	if err := recordKernelCacheUsage(pod, kernelCache); err != nil {
		return err
	}
	return nil
}

func recordKernelCacheUsage(pod *corev1.Pod, kernelCache *v1alpha1.KernelCache) error {
	usageRef := kernelCache.Namespace + "/" + kernelCache.Name
	if existing := pod.Annotations[constants.KernelCacheUsageAnnotationKey]; existing != "" && existing != usageRef {
		return fmt.Errorf("Pod already records KernelCache %q", existing)
	}
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[constants.KernelCacheUsageAnnotationKey] = usageRef
	return nil
}

func kernelCacheSourceVolume(kernelCache *v1alpha1.KernelCache, mountType v1alpha1.KernelCacheMountType) (corev1.Volume, error) {
	volume := corev1.Volume{Name: kernelCacheSourceVolumeName}
	if mountType == v1alpha1.KernelCacheMountTypeOCI {
		volume.Image = &corev1.ImageVolumeSource{
			Reference:  kernelCache.Spec.Artifact.ImageReference,
			PullPolicy: corev1.PullIfNotPresent,
		}
		return volume, nil
	}
	return corev1.Volume{}, fmt.Errorf("unsupported KernelCache mount type %q", mountType)
}

func kernelCacheLinkerImage(cfg *v1beta1.KernelCacheConfig) string {
	if cfg.PrefetchImage != "" {
		return cfg.PrefetchImage
	}
	return v1beta1.DefaultKernelCachePrefetchImage
}

func addKernelCacheVolume(podSpec *corev1.PodSpec, volume corev1.Volume) error {
	index := slices.IndexFunc(podSpec.Volumes, func(existing corev1.Volume) bool { return existing.Name == volume.Name })
	if index < 0 {
		podSpec.Volumes = append(podSpec.Volumes, volume)
		return nil
	}
	if !reflect.DeepEqual(podSpec.Volumes[index], volume) {
		return fmt.Errorf("conflicting volume %q", volume.Name)
	}
	return nil
}

func ensureKernelCacheTargetMount(
	podSpec *corev1.PodSpec,
	container *corev1.Container,
	index int,
	cachePath v1alpha1.KernelCachePath,
) (corev1.VolumeMount, error) {
	for _, existing := range container.VolumeMounts {
		if existing.MountPath == cachePath.ContainerPath {
			if existing.ReadOnly {
				return corev1.VolumeMount{}, fmt.Errorf("cache path %q is read-only", cachePath.ContainerPath)
			}
			return existing, nil
		}
		if volumeMountPathsOverlap(cachePath.ContainerPath, existing.MountPath) {
			return corev1.VolumeMount{}, fmt.Errorf("cache path %q overlaps mount %q", cachePath.ContainerPath, existing.MountPath)
		}
	}

	volume := corev1.Volume{
		Name: fmt.Sprintf("kernel-cache-runtime-%d", index),
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	if err := addKernelCacheVolume(podSpec, volume); err != nil {
		return corev1.VolumeMount{}, err
	}
	mount := corev1.VolumeMount{Name: volume.Name, MountPath: cachePath.ContainerPath}
	container.VolumeMounts = append(container.VolumeMounts, mount)
	return mount, nil
}

func addKernelCacheVolumeMount(container *corev1.Container, mount corev1.VolumeMount, readOnly bool) error {
	for _, existing := range container.VolumeMounts {
		if existing.MountPath == mount.MountPath {
			if existing.Name != mount.Name || (readOnly && !existing.ReadOnly) {
				return fmt.Errorf("conflicting mount at %q", mount.MountPath)
			}
			return nil
		}
		if volumeMountPathsOverlap(mount.MountPath, existing.MountPath) {
			return fmt.Errorf("mount path %q overlaps mount %q", mount.MountPath, existing.MountPath)
		}
	}
	if readOnly {
		mount.ReadOnly = true
	}
	container.VolumeMounts = append(container.VolumeMounts, mount)
	return nil
}

func volumeMountPathsOverlap(first, second string) bool {
	first = strings.TrimSuffix(path.Clean(first), "/")
	second = strings.TrimSuffix(path.Clean(second), "/")
	return first == second || strings.HasPrefix(first, second+"/") || strings.HasPrefix(second, first+"/")
}
