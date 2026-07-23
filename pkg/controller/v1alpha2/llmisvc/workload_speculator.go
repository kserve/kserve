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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// attachSpeculatorModelArtifacts configures a PodSpec for speculative decoding. When
// spec.speculator is set, it handles PVC/OCI mounts for speculator models and injects
// the --speculative-config vLLM argument.
//
// For hf:// and s3:// speculator models, the download is handled by attachModelArtifacts
// via the shared storage-initializer init container; this function only validates the
// storage-initializer requirement for those schemes.
//
// This is called only for decode workloads (single-node main, multi-node leader/worker);
// prefill pods do not receive speculator configuration.
func (r *LLMISVCReconciler) attachSpeculatorModelArtifacts(_ context.Context, _ *corev1.ServiceAccount, llmSvc *v1alpha2.LLMInferenceService, _ corev1.PodSpec, podSpec *corev1.PodSpec, config *Config, containerName string) error { //nolint:unparam
	speculator := llmSvc.Spec.Speculator
	if speculator == nil {
		return nil
	}

	if speculator.Model == nil {
		return injectSpeculativeDecodingArgs(speculator, podSpec, containerName)
	}

	speculatorUri := speculator.Model.URI.String()
	schema, _, sepFound := strings.Cut(speculatorUri, "://")
	if !sepFound {
		return fmt.Errorf("invalid speculator model URI: %s", speculatorUri)
	}

	switch schema + "://" {
	case constants.PvcURIPrefix:
		if err := r.attachSpeculatorPVCModelArtifact(speculatorUri, podSpec, containerName); err != nil {
			return err
		}

	case constants.OciURIPrefix:
		if !config.StorageConfig.EnableOciImageSource {
			return fmt.Errorf("speculator model %q uses oci:// but OCI modelcars is not enabled in the cluster configuration", speculatorUri)
		}
		if err := utils.ConfigureModelcarToContainer(speculatorUri, podSpec, containerName, constants.DefaultSpeculatorLocalMountPath, config.StorageConfig, 1); err != nil {
			return err
		}

	case constants.HfURIPrefix, constants.S3URIPrefix:
		storageInitializerDisabled := llmSvc.Spec.StorageInitializer != nil &&
			llmSvc.Spec.StorageInitializer.Enabled != nil &&
			!*llmSvc.Spec.StorageInitializer.Enabled
		if storageInitializerDisabled {
			return fmt.Errorf("speculator model %q: hf:// and s3:// require the storage initializer — set storageInitializer.enabled to true", speculatorUri)
		}

	default:
		return fmt.Errorf("unsupported scheme %q in speculator model URI %q; supported: pvc://, oci://, hf://, s3://", schema+"://", speculatorUri)
	}

	return injectSpeculativeDecodingArgs(speculator, podSpec, containerName)
}

// attachSpeculatorPVCModelArtifact mounts a speculator model from a PVC using a dedicated
// volume name to avoid colliding with the main model's PVC volume.
func (r *LLMISVCReconciler) attachSpeculatorPVCModelArtifact(modelUri string, podSpec *corev1.PodSpec, containerName string) error {
	pvcName, pvcPath, err := utils.ParsePvcURI(modelUri)
	if err != nil {
		return err
	}

	return utils.AddModelMount(utils.StorageMountParams{
		MountPath:  constants.DefaultSpeculatorLocalMountPath,
		VolumeName: constants.SpeculatorVolumeName,
		ReadOnly:   true,
		PVCName:    pvcName,
		SubPath:    pvcPath,
	}, containerName, podSpec)
}

// injectSpeculativeDecodingArgs builds the --speculative-config JSON from the speculator's
// config map and appends it to VLLM_ADDITIONAL_ARGS on the named container.
func injectSpeculativeDecodingArgs(speculator *v1alpha2.SpeculatorSpec, podSpec *corev1.PodSpec, containerName string) error {
	if len(speculator.Config) == 0 && speculator.Model == nil {
		return nil
	}

	specConfig := map[string]interface{}{}
	for k, v := range speculator.Config {
		specConfig[k] = inferJSONType(v)
	}

	if speculator.Model != nil {
		specConfig["model"] = constants.DefaultSpeculatorLocalMountPath
	}

	specConfigJSON, err := json.Marshal(specConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal speculative decoding config: %w", err)
	}

	escaped := strings.ReplaceAll(string(specConfigJSON), "'", "'\\''")
	specArg := fmt.Sprintf("--speculative-config '%s'", escaped)

	return appendToVLLMAdditionalArgs(podSpec, containerName, specArg)
}

// appendToVLLMAdditionalArgs appends arg to the VLLM_ADDITIONAL_ARGS env var on the named
// container. If the env var already exists, the arg is appended with a space separator.
// When --speculative-config is already present, the CR-defined config takes priority and
// the existing flag is stripped before injection. Returns an error when the env var uses
// valueFrom, since we cannot safely merge with an external source.
func appendToVLLMAdditionalArgs(podSpec *corev1.PodSpec, containerName string, arg string) error {
	container := getContainerByName(podSpec, containerName)
	if container == nil {
		return fmt.Errorf("container %q not found in pod spec", containerName)
	}

	for i, env := range container.Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			if env.ValueFrom != nil {
				return fmt.Errorf("VLLM_ADDITIONAL_ARGS on container %q uses valueFrom; cannot append speculative decoding args", containerName)
			}
			cleaned := stripSpeculativeConfigFlag(env.Value)
			container.Env[i].Value = strings.TrimSpace(cleaned + " " + arg)
			return nil
		}
	}

	container.Env = append(container.Env, corev1.EnvVar{
		Name:  "VLLM_ADDITIONAL_ARGS",
		Value: arg,
	})

	return nil
}

// stripSpeculativeConfigFlag removes any existing --speculative-config '...' or
// --speculative-config "..." from a VLLM_ADDITIONAL_ARGS value so the CR-defined
// config can replace it. Handles backslash-escaped quotes inside double-quoted values.
func stripSpeculativeConfigFlag(value string) string {
	const flag = "--speculative-config"
	idx := strings.Index(value, flag)
	if idx == -1 {
		return value
	}

	before := value[:idx]
	after := value[idx+len(flag):]

	after = strings.TrimLeft(after, " ")
	if len(after) > 0 && (after[0] == '\'' || after[0] == '"') {
		quote := after[0]
		end := findClosingQuote(after[1:], quote)
		if end != -1 {
			after = after[end+2:]
		} else {
			after = ""
		}
	}

	return strings.Join(strings.Fields(strings.TrimSpace(before+" "+after)), " ")
}

// findClosingQuote returns the index of the unescaped closing quote in s,
// skipping backslash-escaped characters.
func findClosingQuote(s string, quote byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == quote {
			return i
		}
	}
	return -1
}

// inferJSONType attempts to parse a string value as a native JSON type (integer,
// float, or boolean) so that the marshalled JSON uses the correct type for vLLM's
// --speculative-config schema. Falls back to the original string if no conversion applies.
func inferJSONType(v string) interface{} {
	if i, err := strconv.ParseInt(v, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	return v
}

func getContainerByName(podSpec *corev1.PodSpec, name string) *corev1.Container {
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == name {
			return &podSpec.Containers[i]
		}
	}
	return nil
}
