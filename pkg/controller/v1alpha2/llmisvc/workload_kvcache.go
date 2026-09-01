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
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

// Spec class names understood by vLLM's OffloadingConnector. Secondary tiers use
// specNameTieringOffloading, a CPU-only tier uses specNameCPUOffloading.
const (
	specNameCPUOffloading     = "CPUOffloadingSpec"
	specNameTieringOffloading = "TieringOffloadingSpec"
)

// kvTransferArgsEnvVar carries the generated vLLM --kv-transfer-config argument
// to the entrypoint. Declaring it empty is a preset's opt-in: the controller
// fills it when offloading is configured and removes it when it is not. Presets
// that do not declare it render the argument themselves via kvTransferConfig and
// are left untouched.
const kvTransferArgsEnvVar = "KSERVE_KV_TRANSFER_ARGS"

const mainContainerName = "main"

// applyKVCacheCarriers fills the kvTransferArgsEnvVar slot in every pod spec that
// declares one, and changes nothing else.
//
// It runs after template rendering because ReplaceVariables marshals the config
// to JSON, executes the template and unmarshals the result: a value a template
// emits must survive that round trip and then a shell eval, which is the
// escaping kvTransferConfig still carries. Assigning to a typed field afterwards
// leaves only the shell quoting.
func applyKVCacheCarriers(cfg *v1alpha2.LLMInferenceServiceConfig) error {
	for _, t := range kvCacheTargets(cfg) {
		if err := applyCarriersForPodSpec(t.podSpec, t.kv); err != nil {
			return fmt.Errorf("%s: %w", t.name, err)
		}
	}
	return nil
}

// kvCacheTarget is one pod spec and the KV cache configuration that governs it.
type kvCacheTarget struct {
	name    string
	podSpec *corev1.PodSpec
	kv      *v1alpha2.KVCacheOffloadingSpec
}

// kvCacheTargets lists the pod specs a KV cache configuration reaches. Prefill
// carries its own, so the two halves of a disaggregated service are governed
// separately.
func kvCacheTargets(cfg *v1alpha2.LLMInferenceServiceConfig) []kvCacheTarget {
	targets := []kvCacheTarget{
		{"template", cfg.Spec.Template, cfg.Spec.KVCacheOffloading},
		{"worker", cfg.Spec.Worker, cfg.Spec.KVCacheOffloading},
	}
	if cfg.Spec.Prefill != nil {
		targets = append(targets,
			kvCacheTarget{"prefill template", cfg.Spec.Prefill.Template, cfg.Spec.Prefill.KVCacheOffloading},
			kvCacheTarget{"prefill worker", cfg.Spec.Prefill.Worker, cfg.Spec.Prefill.KVCacheOffloading},
		)
	}
	return targets
}

func applyCarriersForPodSpec(podSpec *corev1.PodSpec, kv *v1alpha2.KVCacheOffloadingSpec) error {
	if podSpec == nil {
		return nil
	}
	c := utils.GetContainerWithName(podSpec, mainContainerName)
	if c == nil {
		return nil
	}
	slot := normalizeKVTransferSlot(c)
	if slot < 0 {
		// Preset renders the argument itself; leave it exactly as rendered.
		return nil
	}
	if kv == nil {
		c.Env = slices.Delete(c.Env, slot, slot+1)
		return nil
	}
	transferArgs := &c.Env[slot]
	if transferArgs.ValueFrom != nil {
		// env merges by name, so a valueFrom can land on the slot. Honouring it
		// would leave kvCacheOffloading silently ignored.
		return fmt.Errorf("env %s carries the argument generated from kvCacheOffloading and cannot take a valueFrom; "+
			"to supply the connector configuration yourself, pass --kv-transfer-config via VLLM_ADDITIONAL_ARGS or the container args",
			kvTransferArgsEnvVar)
	}

	jsonStr, err := kvTransferJSON(kv)
	if err != nil {
		return fmt.Errorf("rendering %s: %w", kvTransferArgsEnvVar, err)
	}
	// The preset expands this inside eval, so an unescaped quote would end the
	// argument and run the remainder as commands.
	transferArgs.Value = "--kv-transfer-config " + utils.ShellQuote(jsonStr)
	return nil
}

// normalizeKVTransferSlot collapses the slot to a single entry and returns its
// index, or -1 when the preset does not declare one. Runtimes let the last
// duplicate win, so a second empty slot would discard the filled one. The first
// occurrence is kept because reordering env is a changed pod template.
func normalizeKVTransferSlot(c *corev1.Container) int {
	first := -1
	kept := c.Env[:0]
	for _, env := range c.Env {
		if env.Name == kvTransferArgsEnvVar {
			if first >= 0 {
				continue
			}
			first = len(kept)
		}
		kept = append(kept, env)
	}
	c.Env = kept
	return first
}

// kvTransferJSON builds the --kv-transfer-config payload. The cpu byte count is
// whatever the spec says: rejecting one here would turn a spec the API accepted
// into a terminal reconcile failure. Admission refuses a bad cpu.
func kvTransferJSON(kv *v1alpha2.KVCacheOffloadingSpec) (string, error) {
	extraConfig := map[string]any{
		"spec_name":        specNameCPUOffloading,
		"cpu_bytes_to_use": kv.CPU.Value(),
	}
	if kv.EvictionPolicy != "" {
		extraConfig["eviction_policy"] = kv.EvictionPolicy
	}
	var secondaryTiers []map[string]any
	for i, s := range kv.Secondary {
		if s.FileSystem == nil {
			continue
		}
		entry := map[string]any{
			"type":     "fs",
			"root_dir": fmt.Sprintf("/mnt/kv-cache-%d", i),
		}
		secondaryTiers = append(secondaryTiers, entry)
	}
	if len(secondaryTiers) > 0 {
		extraConfig["spec_name"] = specNameTieringOffloading
		extraConfig["secondary_tiers"] = secondaryTiers
	}
	kvConfig := map[string]any{
		"kv_connector":              "OffloadingConnector",
		"kv_role":                   "kv_both",
		"kv_connector_extra_config": extraConfig,
	}
	b, err := json.Marshal(kvConfig)
	if err != nil {
		return "", fmt.Errorf("marshalling KV transfer config: %w", err)
	}
	return string(b), nil
}

// sharedMemoryMountPath is the mount the presets back with a memory-medium
// emptyDir, used for NCCL and for the KV cache offload file.
const sharedMemoryMountPath = "/dev/shm"

// shmHeadroomPercentAnnotation is the percentage of kvCacheOffloading.cpu added
// to the preset's dshm sizeLimit, which otherwise covers NCCL only. Absent means
// no sizing.
//
// Held on the preset, not in this package, so changing it means a new preset
// object. Installs that set LLM_INFERENCE_SERVICE_CONFIG_PREFIX version the
// preset names, so pinned services keep resolving to the old object; without it
// presets are replaced in place.
const shmHeadroomPercentAnnotation = presetAnnotationPrefix + "kv-cache-shm-headroom-percent"

// parseShmHeadroomPercent parses the annotation value, reporting false when it
// is not a positive integer. Callers log and skip sizing.
func parseShmHeadroomPercent(raw string) (int64, bool) {
	percent, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || percent <= 0 {
		return 0, false
	}
	return percent, true
}

// applyKVCacheShmSizing raises the dshm sizeLimit on every pod spec with a tier
// configured. resources.limits.memory is not adjusted: the tier counts against
// it, but the required value also depends on the model's footprint, which is not
// in the spec.
//
// Sizing covers one engine. A pod running parallelism.dataLocal engines maps one
// file each, so it may need a larger sizeLimit set on the spec; the tier is
// added on top of that value.
func applyKVCacheShmSizing(cfg *v1alpha2.LLMInferenceServiceConfig, percent int64) {
	for _, t := range kvCacheTargets(cfg) {
		if t.podSpec == nil || t.kv == nil {
			continue
		}
		growSharedMemory(t.podSpec, t.kv.CPU, percent)
	}
}

// growSharedMemory adds the scaled tier size to the sizeLimit of the volume
// mounted at sharedMemoryMountPath in the main container.
func growSharedMemory(podSpec *corev1.PodSpec, cpu resource.Quantity, percent int64) {
	c := utils.GetContainerWithName(podSpec, mainContainerName)
	if c == nil {
		return
	}
	mount := slices.IndexFunc(c.VolumeMounts, func(m corev1.VolumeMount) bool {
		return m.MountPath == sharedMemoryMountPath
	})
	if mount < 0 {
		return
	}
	name := c.VolumeMounts[mount].Name
	vol := slices.IndexFunc(podSpec.Volumes, func(v corev1.Volume) bool { return v.Name == name })
	if vol < 0 {
		return
	}

	v := &podSpec.Volumes[vol]
	// No size limit already accommodates any tier, and a disk-backed volume is not
	// what the connector maps into.
	if v.EmptyDir == nil || v.EmptyDir.Medium != corev1.StorageMediumMemory || v.EmptyDir.SizeLimit == nil {
		return
	}
	sizeLimit := v.EmptyDir.SizeLimit.DeepCopy()
	sizeLimit.Add(scaleQuantity(cpu, percent))
	v.EmptyDir.SizeLimit = &sizeLimit
}

// scaleQuantity returns q scaled by percent, falling back to q for quantities
// large enough that scaling would overflow into a negative size.
func scaleQuantity(q resource.Quantity, percent int64) resource.Quantity {
	v := q.Value()
	if v > math.MaxInt64/percent {
		return q
	}
	return *resource.NewQuantity(v*percent/100, q.Format)
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
