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
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
)

// resolvedLoRAAdapter is one adapter after URI validation (hf/s3 downloads are handled in attachModelArtifacts).
type resolvedLoRAAdapter struct {
	name      string
	mountPath string
	uri       string
	scheme    string
}

// enumerateLoRAAdapters validates spec.model.lora.adapters and returns mount paths and schemes in order.
func enumerateLoRAAdapters(llmSvc *v1alpha2.LLMInferenceService) ([]resolvedLoRAAdapter, error) {
	if llmSvc.Spec.Model.LoRA == nil || len(llmSvc.Spec.Model.LoRA.Adapters) == 0 {
		return nil, nil
	}

	storageInitializerDisabled := llmSvc.Spec.StorageInitializer != nil &&
		llmSvc.Spec.StorageInitializer.Enabled != nil &&
		!*llmSvc.Spec.StorageInitializer.Enabled

	baseModelName := ptr.Deref(llmSvc.Spec.Model.Name, llmSvc.Name)
	adapters := llmSvc.Spec.Model.LoRA.Adapters
	out := make([]resolvedLoRAAdapter, 0, len(adapters))

	for i, adapter := range adapters {
		if adapter.Name == nil || *adapter.Name == "" {
			return nil, fmt.Errorf("LoRA adapter[%d]: name is required when lora.adapters is set", i)
		}
		adapterName := *adapter.Name
		if adapterName == baseModelName {
			return nil, fmt.Errorf("LoRA adapter name %q must differ from base model name %q", adapterName, baseModelName)
		}

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
				return nil, errors.New("LoRA adapter with hf:// or s3:// URI requires storage initializer (set storageInitializer.enabled to true)")
			}
		case constants.PvcURIPrefix:
			if storageInitializerDisabled {
				return nil, errors.New("LoRA adapter with pvc:// URI requires a mounted volume (do not set storageInitializer.enabled to false)")
			}
		case constants.OciURIPrefix:
			return nil, fmt.Errorf("LoRA adapter %q: oci:// adapter URIs are not supported yet; use hf://, s3://, or pvc://", adapterName)
		default:
			return nil, fmt.Errorf("LoRA adapter %q: unsupported URI scheme in %q (supported: hf://, s3://, pvc://)", adapterName, uri)
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

// collectLoRADownloadPairs returns hf:// and s3:// adapter uri/path pairs for a single storage-initializer run.
func collectLoRADownloadPairs(llmSvc *v1alpha2.LLMInferenceService) ([]storageDownloadPair, error) {
	adapters, err := enumerateLoRAAdapters(llmSvc)
	if err != nil || len(adapters) == 0 {
		return nil, err
	}
	var pairs []storageDownloadPair
	for _, a := range adapters {
		if a.scheme == constants.HfURIPrefix || a.scheme == constants.S3URIPrefix {
			pairs = append(pairs, storageDownloadPair{uri: a.uri, path: a.mountPath})
		}
	}
	return pairs, nil
}

// attachLoRAAdapters reconciles spec.model.lora.adapters into vLLM CLI flags appended to the main
// container's Args. hf:// and s3:// adapters are downloaded in attachModelArtifacts via one
// storage-initializer; pvc:// adapters are mounted here.
func (r *LLMISVCReconciler) attachLoRAAdapters(
	ctx context.Context,
	_ *corev1.ServiceAccount,
	llmSvc *v1alpha2.LLMInferenceService,
	_ corev1.PodSpec,
	podSpec *corev1.PodSpec,
	_ *Config,
) error {
	const containerName = "main"

	adapters, err := enumerateLoRAAdapters(llmSvc)
	if err != nil {
		return err
	}
	if len(adapters) == 0 {
		return nil
	}

	var loraModules []string
	for i, a := range adapters {
		switch a.scheme {
		case constants.PvcURIPrefix:
			volName := fmt.Sprintf("lora-pvc-%d", i)
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

	if userSuppliedLoRAConfig(main) {
		log.FromContext(ctx).Info("Skipping controller LoRA injection: workload already sets --lora-modules",
			"llmService", llmSvc.Name, "namespace", llmSvc.Namespace)
		return nil
	}

	appendLoRAVLLMWorkloadArgs(main, len(loraModules), loraModules)

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

// appendLoRAVLLMWorkloadArgs appends vLLM LoRA flags to main.Args so the LLMInferenceServiceConfig
// entrypoint can pass them to `vllm serve` (eval "... $@") after the trailing `--` argv separator.
func appendLoRAVLLMWorkloadArgs(main *corev1.Container, n int, loraModules []string) {
	argv := make([]string, 0, 5+len(loraModules))
	argv = append(argv,
		"--enable-lora",
		fmt.Sprintf("--max-lora-rank=%d", defaultMaxLoRARank),
		fmt.Sprintf("--max-loras=%d", n),
		fmt.Sprintf("--max-cpu-loras=%d", n),
		"--lora-modules",
	)
	argv = append(argv, loraModules...)
	main.Args = append(main.Args, argv...)
}

func sanitizeLoRAPathSegment(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
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
