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
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/utils"
)

// attachSpeculatorModelArtifacts configures a PodSpec for speculative decoding. When
// spec.speculator is set, it optionally downloads the speculator/draft model (for methods
// that require one) and injects the --speculative-config vLLM argument.
// This is called only for decode workloads (single-node main, multi-node leader/worker);
// prefill pods do not receive speculator configuration.
func (r *LLMISVCReconciler) attachSpeculatorModelArtifacts(ctx context.Context, serviceAccount *corev1.ServiceAccount, llmSvc *v1alpha2.LLMInferenceService, curr corev1.PodSpec, podSpec *corev1.PodSpec, config *Config, containerName string) error { //nolint:unparam
	speculator := llmSvc.Spec.Speculator
	if speculator == nil {
		return nil
	}

	if speculator.Model == nil {
		return injectSpeculativeDecodingArgs(speculator, podSpec, containerName)
	}

	storageInitializerDisabled := llmSvc.Spec.StorageInitializer != nil &&
		llmSvc.Spec.StorageInitializer.Enabled != nil &&
		!*llmSvc.Spec.StorageInitializer.Enabled

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
		// ociIndex=1 for speculator (main model uses 0, LoRA doesn't support OCI)
		if err := utils.ConfigureModelcarToContainer(speculatorUri, podSpec, containerName, constants.DefaultSpeculatorLocalMountPath, config.StorageConfig, 1); err != nil {
			return err
		}

	case constants.HfURIPrefix, constants.S3URIPrefix:
		if storageInitializerDisabled {
			return fmt.Errorf("speculator model %q: hf:// and s3:// require the storage initializer — set storageInitializer.enabled to true", speculatorUri)
		}
		if err := r.attachSpeculatorStorageInitializer(ctx, serviceAccount, llmSvc, speculatorUri, schema+"://", curr, podSpec, config, containerName); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unsupported scheme %q in speculator model URI %q; supported: pvc://, oci://, hf://, s3://", schema+"://", speculatorUri)
	}

	return injectSpeculativeDecodingArgs(speculator, podSpec, containerName)
}

// stripPriorSpeculatorInitializer removes any existing speculator-initializer init container
// from the pod spec to avoid duplicates when merged templates (via baseRefs) already define one.
func stripPriorSpeculatorInitializer(podSpec *corev1.PodSpec) {
	if podSpec == nil {
		return
	}
	kept := make([]corev1.Container, 0, len(podSpec.InitContainers))
	for _, ic := range podSpec.InitContainers {
		if ic.Name == constants.SpeculatorInitializerContainerName {
			continue
		}
		kept = append(kept, ic)
	}
	podSpec.InitContainers = kept
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

// attachSpeculatorStorageInitializer creates a second storage-initializer init container
// (named "speculator-initializer") to download the speculator model from HF or S3.
func (r *LLMISVCReconciler) attachSpeculatorStorageInitializer(ctx context.Context, serviceAccount *corev1.ServiceAccount, llmSvc *v1alpha2.LLMInferenceService, speculatorUri string, uriPrefix string, curr corev1.PodSpec, podSpec *corev1.PodSpec, config *Config, containerName string) error {
	stripPriorSpeculatorInitializer(podSpec)

	containerArgs := []string{
		speculatorUri,
		constants.DefaultSpeculatorLocalMountPath,
	}

	// Preserve the existing speculator-initializer image from the current deployment
	copied := *config.StorageConfig
	for _, initContainer := range curr.InitContainers {
		if initContainer.Name == constants.SpeculatorInitializerContainerName {
			copied.Image = initContainer.Image
		}
	}

	initContainer := utils.CreateInitContainerWithConfig(&copied, containerArgs)
	initContainer.Name = constants.SpeculatorInitializerContainerName
	podSpec.InitContainers = append(podSpec.InitContainers, *initContainer)

	// Mount the speculator volume RW on the init container
	if err := utils.AddModelMount(utils.StorageMountParams{
		MountPath:  constants.DefaultSpeculatorLocalMountPath,
		VolumeName: constants.SpeculatorVolumeName,
		ReadOnly:   false,
	}, initContainer.Name, podSpec); err != nil {
		return err
	}

	// Mount the speculator volume RO on the main container
	if err := utils.AddModelMount(utils.StorageMountParams{
		MountPath:  constants.DefaultSpeculatorLocalMountPath,
		VolumeName: constants.SpeculatorVolumeName,
		ReadOnly:   true,
	}, containerName, podSpec); err != nil {
		return err
	}

	initPtr := utils.GetInitContainerWithName(podSpec, constants.SpeculatorInitializerContainerName)
	if initPtr == nil {
		return errors.New("speculator-initializer init container not found after creation")
	}

	if serviceAccount == nil {
		serviceAccount = &corev1.ServiceAccount{}
		err := r.Get(ctx, types.NamespacedName{Name: constants.LLMISVCDefaultServiceAccountName, Namespace: llmSvc.Namespace}, serviceAccount)
		if err != nil {
			log.FromContext(ctx).Error(err, "Failed to find default service account", "namespace", llmSvc.Namespace)
			injectCaBundle(llmSvc.Namespace, podSpec, initPtr, config.StorageConfig)
			return nil
		}
	}

	credentialBuilder := credentials.NewCredentialBuilderFromConfig(r.Client, r.Clientset, *config.CredentialConfig)
	if err := credentialBuilder.CreateSecretVolumeAndEnvFromServiceAccount(
		ctx,
		serviceAccount,
		llmSvc.Annotations,
		initPtr,
		&podSpec.Volumes,
	); err != nil {
		return err
	}

	if uriPrefix == constants.HfURIPrefix {
		currentInit := utils.GetInitContainerWithName(&curr, constants.SpeculatorInitializerContainerName)
		if currentInit == nil || slices.ContainsFunc(currentInit.Env, func(e corev1.EnvVar) bool {
			return strings.HasPrefix(e.Name, "HF_")
		}) {
			utils.AddDefaultHuggingFaceEnvVars(initPtr)
		}
	}

	injectCaBundle(llmSvc.Namespace, podSpec, initPtr, config.StorageConfig)

	return nil
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
