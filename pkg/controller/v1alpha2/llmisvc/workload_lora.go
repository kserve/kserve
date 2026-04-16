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
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	// defaultMaxLoRARank is passed to vLLM --max-lora-rank when LoRA adapters are reconciled from the CR.
	defaultMaxLoRARank = 64
	// loraAdaptersMountRoot must not be under constants.DefaultModelLocalMountPath: the base model
	// is mounted there read-only, and nested volume mounts require mkdir on the parent (fails on RO).
	loraAdaptersMountRoot = "/mnt/lora"
	// loraAdapterDocsURL is linked in user-facing error messages for better UX.
	loraAdapterDocsURL = "https://github.com/kserve/kserve/blob/master/docs/samples/llmisvc/lora-adapters/README.md"
)

// loraPathInvalidCharsRe matches characters that are invalid in filesystem paths.
// Replaces anything that is not alphanumeric, dash, underscore, or dot.
var loraPathInvalidCharsRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// loraVolumeNameInvalidCharsRe matches characters that are invalid in Kubernetes volume names
// (which must be DNS labels: lowercase alphanumeric and hyphens only).
var loraVolumeNameInvalidCharsRe = regexp.MustCompile(`[^a-z0-9-]`)

// resolvedLoRAAdapter is one adapter after URI validation (hf/s3 downloads are handled in attachModelArtifacts).
type resolvedLoRAAdapter struct {
	name      string
	mountPath string
	uri       string
	scheme    string
}

// enumerateLoRAAdapters validates spec.model.lora.adapters and returns mount paths and schemes.
// Returns adapters in the same order as spec.model.lora.adapters.
func enumerateLoRAAdapters(spec v1alpha2.LLMInferenceServiceSpec) ([]resolvedLoRAAdapter, error) {
	if spec.Model.LoRA == nil || len(spec.Model.LoRA.Adapters) == 0 {
		return nil, nil
	}

	storageInitializerDisabled := spec.StorageInitializer != nil &&
		spec.StorageInitializer.Enabled != nil &&
		!*spec.StorageInitializer.Enabled

	// Sort by name for stable ordering: ensures the same spec always produces the same
	// pod spec regardless of the order adapters appear in spec.model.lora.adapters.
	adapters := slices.SortedFunc(slices.Values(spec.Model.LoRA.Adapters), func(a, b v1alpha2.LLMModelSpec) int {
		return strings.Compare(ptr.Deref(a.Name, ""), ptr.Deref(b.Name, ""))
	})
	out := make([]resolvedLoRAAdapter, 0, len(adapters))

	for _, adapter := range adapters {
		adapterName := ptr.Deref(adapter.Name, "")

		uri := adapter.URI.String()
		schema, _, sepFound := strings.Cut(uri, "://")
		if !sepFound {
			return nil, fmt.Errorf("LoRA adapter %q: invalid URI %q", adapterName, uri)
		}
		scheme := schema + "://"
		mountPath := filepath.Join(loraAdaptersMountRoot, sanitizeLoRAPathSegment(adapterName))

		switch scheme {
		case constants.HfURIPrefix, constants.S3URIPrefix:
			if storageInitializerDisabled {
				return nil, fmt.Errorf("LoRA adapter %q: hf:// and s3:// require the storage initializer — set storageInitializer.enabled to true (see %s)", adapterName, loraAdapterDocsURL)
			}
		case constants.PvcURIPrefix:
			if storageInitializerDisabled {
				return nil, fmt.Errorf("LoRA adapter %q: pvc:// requires a mounted volume — do not set storageInitializer.enabled to false (see %s)", adapterName, loraAdapterDocsURL)
			}
		case constants.OciURIPrefix:
			// oci:// is intentionally not supported for LoRA adapters. OCI models run as sidecar
			// containers ("modelcars") with shared process namespaces, but only one modelcar per pod
			// is currently supported. Workaround: package the adapter in a PVC and use pvc://.
			return nil, fmt.Errorf("LoRA adapter %q: oci:// is not supported for LoRA adapters; use hf://, s3://, or pvc:// instead (see %s)", adapterName, loraAdapterDocsURL)
		default:
			return nil, fmt.Errorf("LoRA adapter %q: unsupported URI scheme %q; supported schemes are hf://, s3://, pvc:// (see %s)", adapterName, scheme, loraAdapterDocsURL)
		}

		out = append(out, resolvedLoRAAdapter{
			name:      adapterName,
			mountPath: mountPath,
			uri:       uri,
			scheme:    scheme,
		})
	}
	return out, nil
}

// collectLoRADownloadPairs filters pre-resolved adapters to hf:// and s3:// uri/path pairs
// for a single storage-initializer run.
func collectLoRADownloadPairs(adapters []resolvedLoRAAdapter) []storageDownloadPair {
	var pairs []storageDownloadPair
	for _, a := range adapters {
		if a.scheme == constants.HfURIPrefix || a.scheme == constants.S3URIPrefix {
			pairs = append(pairs, storageDownloadPair{uri: a.uri, path: a.mountPath})
		}
	}
	return pairs
}

// attachLoRAAdapters reconciles spec.model.lora.adapters into vLLM CLI flags appended to the main
// container's Args. hf:// and s3:// adapters are downloaded in attachModelArtifacts via one
// storage-initializer; pvc:// adapters are mounted here.
func (r *LLMISVCReconciler) attachLoRAAdapters(
	ctx context.Context,
	llmSvc *v1alpha2.LLMInferenceService,
	podSpec *corev1.PodSpec,
	adapters []resolvedLoRAAdapter,
) error {
	const containerName = "main"

	if len(adapters) == 0 {
		return nil
	}

	var loraModules []string
	for _, a := range adapters {
		switch a.scheme {
		case constants.PvcURIPrefix:
			volName := "lora-pvc-" + sanitizeLoRAVolumeName(a.name)
			if err := attachLoraPVCAdapter(a.uri, podSpec, containerName, a.mountPath, volName); err != nil {
				return fmt.Errorf("LoRA adapter %q: %w", a.name, err)
			}
		case constants.HfURIPrefix, constants.S3URIPrefix:
			// Downloaded alongside the base model in attachModelArtifacts.
		default:
			return fmt.Errorf("LoRA adapter %q: internal error, unhandled scheme %q", a.name, a.scheme)
		}
		loraModules = append(loraModules, fmt.Sprintf("%s=%s", a.name, a.mountPath))
	}

	mainIdx := -1
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == containerName {
			mainIdx = i
			break
		}
	}
	if mainIdx < 0 {
		return fmt.Errorf("no container %q in pod spec for LoRA injection", containerName)
	}
	main := &podSpec.Containers[mainIdx]

	if hasValueFromLoRAConfig(main) {
		log.FromContext(ctx).Info("VLLM_ADDITIONAL_ARGS is set via valueFrom; cannot inspect value at reconcile time — "+
			"injecting --lora-modules as usual; if the referenced value already contains --lora-modules, duplicate flags may result",
			"llmService", llmSvc.Name, "namespace", llmSvc.Namespace)
	}

	if userSuppliedLoRAConfig(main) {
		log.FromContext(ctx).Info("Skipping controller LoRA injection: workload already sets --lora-modules",
			"llmService", llmSvc.Name, "namespace", llmSvc.Namespace)
		return nil
	}

	loraSpec := llmSvc.Spec.Model.LoRA

	maxRank := int(defaultMaxLoRARank)
	if loraSpec != nil && loraSpec.MaxRank != nil {
		maxRank = int(*loraSpec.MaxRank)
	}

	maxAdapters := len(loraModules)
	if loraSpec != nil && loraSpec.MaxAdapters != nil {
		maxAdapters = int(*loraSpec.MaxAdapters)
	}

	maxCpuAdapters := len(loraModules)
	if loraSpec != nil && loraSpec.MaxCpuAdapters != nil {
		maxCpuAdapters = int(*loraSpec.MaxCpuAdapters)
	}

	appendLoRAVLLMWorkloadArgs(main, loraModules, maxRank, maxAdapters, maxCpuAdapters)

	return nil
}

func userSuppliedLoRAConfig(c *corev1.Container) bool {
	for _, e := range c.Env {
		if e.Name == "VLLM_ADDITIONAL_ARGS" && strings.Contains(e.Value, "--lora-modules") {
			return true
		}
	}
	joined := strings.Join(c.Command, " ") + " " + strings.Join(c.Args, " ")
	return strings.Contains(joined, "--lora-modules")
}

// hasValueFromLoRAConfig reports whether VLLM_ADDITIONAL_ARGS is set via valueFrom
// (ConfigMap/Secret reference), meaning the controller cannot inspect the value at reconcile time.
func hasValueFromLoRAConfig(c *corev1.Container) bool {
	for _, e := range c.Env {
		if e.Name == "VLLM_ADDITIONAL_ARGS" && e.ValueFrom != nil {
			return true
		}
	}
	return false
}

// appendLoRAVLLMWorkloadArgs appends vLLM LoRA flags to main.Args so the LLMInferenceServiceConfig
// entrypoint can pass them to `vllm serve` (eval "... $@") after the trailing `--` argv separator.
func appendLoRAVLLMWorkloadArgs(main *corev1.Container, loraModules []string, maxRank, maxAdapters, maxCpuAdapters int) {
	argv := make([]string, 0, 5+len(loraModules))
	argv = append(argv,
		"--enable-lora",
		fmt.Sprintf("--max-lora-rank=%d", maxRank),
		fmt.Sprintf("--max-loras=%d", maxAdapters),
		fmt.Sprintf("--max-cpu-loras=%d", maxCpuAdapters),
		"--lora-modules",
	)
	argv = append(argv, loraModules...)
	main.Args = append(main.Args, argv...)
}

func sanitizeLoRAPathSegment(s string) string {
	out := loraPathInvalidCharsRe.ReplaceAllString(s, "-")
	// "." and ".." are valid after sanitization but dangerous as path components:
	// filepath.Join("/mnt/lora", "..") resolves to "/mnt" (path traversal).
	if out == "" || out == "." || out == ".." {
		return "adapter"
	}
	return out
}

// sanitizeLoRAVolumeName produces a valid Kubernetes volume name (DNS label) from an adapter name.
// Volume names must be lowercase alphanumeric and hyphens, max 63 chars. The "lora-pvc-" prefix
// (9 chars) is added by the caller, so we cap the output at 54 chars.
func sanitizeLoRAVolumeName(s string) string {
	out := loraVolumeNameInvalidCharsRe.ReplaceAllString(strings.ToLower(s), "-")
	out = strings.Trim(out, "-")
	if len(out) > 54 {
		out = strings.TrimRight(out[:54], "-")
	}
	if out == "" {
		return "adapter"
	}
	return out
}

func attachLoraPVCAdapter(modelURI string, podSpec *corev1.PodSpec, workloadContainerName, mountPath, volumeName string) error {
	pvcName, pvcPath, err := utils.ParsePvcURI(modelURI)
	if err != nil {
		return err
	}
	storageMountParams := utils.StorageMountParams{
		MountPath:  mountPath,
		VolumeName: volumeName,
		ReadOnly:   true,
		PVCName:    pvcName,
		SubPath:    pvcPath,
	}
	return utils.AddModelMount(storageMountParams, workloadContainerName, podSpec)
}
