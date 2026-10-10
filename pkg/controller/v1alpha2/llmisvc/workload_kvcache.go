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
//
// What kvTransferJSON emits into this slot is the same kind of contract
// templateFuncs carries, for the same reason: presets are pinned per service but
// the controller binary is not, so a pinned preset is re-filled by whatever
// version is installed. Changing the payload rewrites the pod spec of untouched
// workloads and restarts them. Ship a different shape under a new slot name
// rather than by changing what this one emits.
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
		if err := applyCarriersForPodSpec(t.name, t.podSpec, t.kv); err != nil {
			return err
		}
	}
	return nil
}

// kvTransferSlotError reports a spec that writes to the slot the controller owns.
// Typed so reconcileBaseRefs can classify it the way it classifies the other
// errors a user has to edit their spec to clear; raising it terminal here would
// also fire for the watch mapping handlers, which combine configs for services
// nothing is reconciling.
type kvTransferSlotError struct {
	workload  string
	container string
}

func (e *kvTransferSlotError) Error() string {
	return fmt.Sprintf("%s: container %q sets env %s, which carries the argument generated from kvCacheOffloading; "+
		"to supply the connector configuration yourself, pass --kv-transfer-config via VLLM_ADDITIONAL_ARGS or the container args",
		e.workload, e.container, kvTransferArgsEnvVar)
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

func applyCarriersForPodSpec(workload string, podSpec *corev1.PodSpec, kv *v1alpha2.KVCacheOffloadingSpec) error {
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
	transferArgs := &c.Env[slot]
	// env merges by name, so anything the spec declares lands on the preset's
	// empty slot. Filling over a value would leave it silently ignored and
	// removing one would make it vanish with nothing to say where it went, so
	// both are reported rather than resolved. Presets declare the slot empty, and
	// the merged config is rebuilt from them each reconcile, so a value here is
	// never one this function wrote.
	if transferArgs.ValueFrom != nil || transferArgs.Value != "" {
		return &kvTransferSlotError{workload: workload, container: c.Name}
	}
	if kv == nil {
		c.Env = slices.Delete(c.Env, slot, slot+1)
		return nil
	}

	jsonStr, err := kvTransferJSON(kv)
	if err != nil {
		return fmt.Errorf("%s: rendering %s: %w", workload, kvTransferArgsEnvVar, err)
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

// shmPercentOfCPUAnnotation is the percentage of kvCacheOffloading.cpu added to
// the preset's dshm sizeLimit, which otherwise covers NCCL only. 120 means the
// tier plus a fifth again; absent means no sizing.
//
// Held on the preset, not in this package, so changing it means a new preset
// object. Installs that set LLM_INFERENCE_SERVICE_CONFIG_PREFIX version the
// preset names, so pinned services keep resolving to the old object; without it
// presets are replaced in place.
const shmPercentOfCPUAnnotation = presetAnnotationPrefix + "kv-cache-shm-percent-of-cpu"

// maxShmPercentOfCPU bounds the annotation. Nothing legitimate asks for more
// than a hundred times the tier, and rejecting the rest keeps scaleQuantity's
// overflow fallback out of reach of any value a preset can carry.
const maxShmPercentOfCPU = 10000

// parseShmPercentOfCPU parses the annotation value, reporting false when it is
// not a positive integer within maxShmPercentOfCPU. Callers log and skip sizing.
func parseShmPercentOfCPU(raw string) (int64, bool) {
	percent, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || percent <= 0 || percent > maxShmPercentOfCPU {
		return 0, false
	}
	return percent, true
}

// kvCacheShmPolicy records who declared a volume's sizeLimit. A preset's
// percentage applies only to its own declarations; a user-declared limit is
// preserved even when its value equals the preset's default.
type kvCacheShmPolicy struct {
	percentOfCPU int64
	userDeclared bool
	// declaredBy names the config that last wrote the size, so a skipped workload
	// can be traced back to it. A user who customised a preset by copying it does
	// not think of the inherited size as one they declared.
	declaredBy string
}

// kvCacheShmSizing indexes sizeLimit declarations by workload and volume name.
// Volumes merge by name, while their mounts may arrive from another config.
type kvCacheShmSizing map[string]map[string]kvCacheShmPolicy

// record follows config merge order, so the last declaration of a sizeLimit
// owns it. Inspecting volumes directly also captures partial user overrides
// that inherit their mount and storage medium from a preset.
func (s kvCacheShmSizing) record(cfg *v1alpha2.LLMInferenceServiceConfig, policy kvCacheShmPolicy) {
	for _, t := range kvCacheTargets(cfg) {
		if t.podSpec == nil {
			continue
		}
		for _, v := range t.podSpec.Volumes {
			if v.EmptyDir == nil || v.EmptyDir.SizeLimit == nil {
				continue
			}
			if s[t.name] == nil {
				s[t.name] = map[string]kvCacheShmPolicy{}
			}
			s[t.name][v.Name] = policy
		}
	}
}

// applyKVCacheShmSizing grows only preset-owned shared-memory limits whose
// declaring preset opted in. Container resources are left alone: the required
// memory also depends on the model's footprint. Sizing covers one engine;
// a user can declare a larger limit for pods running multiple local engines.
func applyKVCacheShmSizing(cfg *v1alpha2.LLMInferenceServiceConfig, sizing kvCacheShmSizing) []string {
	var declared []string
	for _, t := range kvCacheTargets(cfg) {
		if t.kv == nil {
			continue
		}
		v := sharedMemoryVolume(t.podSpec)
		if v == nil || v.EmptyDir.SizeLimit == nil {
			continue
		}
		policy := sizing[t.name][v.Name]
		if policy.userDeclared {
			declared = append(declared, fmt.Sprintf("%s (declared by %s)", t.name, policy.declaredBy))
			continue
		}
		if policy.percentOfCPU > 0 {
			sizeLimit := v.EmptyDir.SizeLimit.DeepCopy()
			sizeLimit.Add(scaleQuantity(t.kv.CPU, policy.percentOfCPU))
			v.EmptyDir.SizeLimit = &sizeLimit
		}
	}
	return declared
}

// sharedMemorySizeLimit returns the sizeLimit of the memory-medium volume mounted
// at sharedMemoryMountPath in the main container, or nil when there is no such
// volume or it carries no limit.
func sharedMemorySizeLimit(podSpec *corev1.PodSpec) *resource.Quantity {
	v := sharedMemoryVolume(podSpec)
	if v == nil {
		return nil
	}
	return v.EmptyDir.SizeLimit
}

// sharedMemoryVolume resolves the main container's sharedMemoryMountPath mount to
// its volume, returning nil unless that volume is a memory-medium
// emptyDir. A disk-backed volume is not what the connector maps into.
func sharedMemoryVolume(podSpec *corev1.PodSpec) *corev1.Volume {
	if podSpec == nil {
		return nil
	}
	c := utils.GetContainerWithName(podSpec, mainContainerName)
	if c == nil {
		return nil
	}
	mount := slices.IndexFunc(c.VolumeMounts, func(m corev1.VolumeMount) bool {
		return m.MountPath == sharedMemoryMountPath
	})
	if mount < 0 {
		return nil
	}
	name := c.VolumeMounts[mount].Name
	vol := slices.IndexFunc(podSpec.Volumes, func(v corev1.Volume) bool { return v.Name == name })
	if vol < 0 {
		return nil
	}
	v := &podSpec.Volumes[vol]
	if v.EmptyDir == nil || v.EmptyDir.Medium != corev1.StorageMediumMemory {
		return nil
	}
	return v
}

// scaleQuantity returns q scaled by percent, falling back to q for quantities
// large enough that scaling would overflow into a negative size.
func scaleQuantity(q resource.Quantity, percent int64) resource.Quantity {
	v := q.Value()
	if percent <= 0 {
		return *resource.NewQuantity(0, q.Format)
	}
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
