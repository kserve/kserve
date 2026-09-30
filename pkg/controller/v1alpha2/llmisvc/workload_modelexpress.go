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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	kservevalidation "github.com/kserve/kserve/pkg/validation"
)

// modelArgsEnvVar carries the vLLM model argument. Presets declare it empty and
// expand ${KSERVE_MODEL_ARGS:-/mnt/models}. The controller fills it for native
// hf:// models and removes it otherwise.
const modelArgsEnvVar = "KSERVE_MODEL_ARGS"

const (
	modelExpressLoadFormat = "modelexpress"

	modelExpressTokenVolumeName        = "modelexpress-token"
	modelExpressTokenMountPath         = "/var/run/secrets/modelexpress" // #nosec G101
	modelExpressTokenFile              = "token"
	modelExpressTokenExpirationSeconds = int64(3600)

	modelExpressCacheVolumeName = "modelexpress-cache"
	modelExpressCacheMountPath  = "/mnt/modelexpress-cache"
)

// modelExpressWeightPatterns are the files native s3:// leaves to ModelStreamer.
var modelExpressWeightPatterns = []string{
	"*.safetensors", "*.bin", "*.pt", "*.pth", "*.gguf", "*.h5", "*.msgpack", "*.onnx",
}

// modelExpressServer is where engine pods reach the ModelExpress server.
type modelExpressServer struct {
	// Address is host:port, http://host:port or https://host:port.
	Address string
	// TokenAudience is the audience of the projected ServiceAccount token the
	// client presents. Empty sends no token.
	TokenAudience string
}

// modelExpressConfig is the resolved ModelExpress setup for one reconcile.
type modelExpressConfig struct {
	Mode     kservevalidation.ModelExpressMode
	Server   modelExpressServer
	Revision string
}

func (m *modelExpressConfig) native() bool {
	return m != nil && m.Mode == kservevalidation.ModelExpressModeNative
}

// modelExpressServerFromAnnotations reads the server from the address and
// token-audience annotations.
func modelExpressServerFromAnnotations(llmSvc *v1alpha2.LLMInferenceService) (*modelExpressServer, error) {
	address := strings.TrimSpace(llmSvc.Annotations[constants.ModelExpressAddressAnnotationKey])
	if address == "" {
		return nil, fmt.Errorf("annotation %s is not set", constants.ModelExpressAddressAnnotationKey)
	}
	if err := kservevalidation.ValidateModelExpressAddress(address); err != nil {
		return nil, fmt.Errorf("annotation %s: %w", constants.ModelExpressAddressAnnotationKey, err)
	}
	return &modelExpressServer{
		Address:       address,
		TokenAudience: strings.TrimSpace(llmSvc.Annotations[constants.ModelExpressTokenAudienceAnnotationKey]),
	}, nil
}

// modelExpressRevision returns the revision annotation, or a digest of the model URI.
func modelExpressRevision(llmSvc *v1alpha2.LLMInferenceService) string {
	if rev := strings.TrimSpace(llmSvc.Annotations[constants.ModelExpressRevisionAnnotationKey]); rev != "" {
		return rev
	}
	sum := sha256.Sum256([]byte(llmSvc.Spec.Model.URI.String()))
	return "uri-" + hex.EncodeToString(sum[:8])
}

// reconcileModelExpress resolves config.ModelExpress from the merged spec and
// reports it on the ModelExpressReady condition. A native-mode failure is
// returned so the workload is not rendered; a layered-mode failure renders the
// workload without ModelExpress.
func (r *LLMISVCReconciler) reconcileModelExpress(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	config.ModelExpress = nil
	if utils.GetForceStopRuntime(llmSvc) {
		llmSvc.MarkModelExpressUnset()
		return nil
	}

	mode, err := llmSvc.ModelExpressMode()
	if err != nil {
		llmSvc.MarkModelExpressNotReady("InvalidMode", err.Error())
		return err
	}
	if mode == "" {
		llmSvc.MarkModelExpressUnset()
		return nil
	}

	fail := func(reason string, err error) error {
		llmSvc.MarkModelExpressNotReady(reason, err.Error())
		if mode == kservevalidation.ModelExpressModeNative {
			return fmt.Errorf("ModelExpress: %w", err)
		}
		log.FromContext(ctx).Info("Rendering workload without ModelExpress", "reason", reason, "error", err.Error())
		r.Eventf(llmSvc, corev1.EventTypeWarning, "ModelExpress"+reason, "Rendering workload without ModelExpress: %v", err)
		return nil
	}

	loraURIs := make([]string, 0, len(config.ResolvedLoRAAdapters))
	for _, a := range config.ResolvedLoRAAdapters {
		loraURIs = append(loraURIs, a.uri)
	}
	modelURI := llmSvc.Spec.Model.URI.String()
	if err := kservevalidation.ValidateModelExpressSource(mode, modelURI, loraURIs); err != nil {
		return fail("UnsupportedModelSource", err)
	}
	if mode == kservevalidation.ModelExpressModeNative {
		if err := validateNativeModelExpressWorkload(llmSvc); err != nil {
			return fail("UnsupportedWorkload", err)
		}
	}

	server, err := r.resolveModelExpressServer(ctx, llmSvc)
	if err != nil {
		return fail("ServerNotResolved", err)
	}

	config.ModelExpress = &modelExpressConfig{
		Mode:     mode,
		Server:   *server,
		Revision: modelExpressRevision(llmSvc),
	}
	llmSvc.MarkModelExpressReady()
	return nil
}

// validateNativeModelExpressWorkload checks the merged spec for what native
// mode needs beyond the model source.
func validateNativeModelExpressWorkload(llmSvc *v1alpha2.LLMInferenceService) error {
	modelURI := llmSvc.Spec.Model.URI.String()
	if strings.HasPrefix(modelURI, constants.S3URIPrefix) {
		if si := llmSvc.Spec.StorageInitializer; si != nil && si.Enabled != nil && !*si.Enabled {
			return errors.New("an s3:// model downloads its non-weight files with the storage initializer, which spec.storageInitializer.enabled disables")
		}
		return nil
	}
	for name, podSpec := range engineTemplates(llmSvc.Spec) {
		c := utils.GetContainerWithName(podSpec, mainContainerName)
		if c == nil {
			continue
		}
		if !slices.ContainsFunc(c.Env, func(e corev1.EnvVar) bool { return e.Name == modelArgsEnvVar }) {
			return fmt.Errorf("%s: container %q does not declare env %s, so an hf:// model cannot be passed to vLLM; use a preset that declares it",
				name, mainContainerName, modelArgsEnvVar)
		}
	}
	return nil
}

// engineTemplates lists the engine pod templates of a merged spec by role.
func engineTemplates(spec v1alpha2.LLMInferenceServiceSpec) map[string]*corev1.PodSpec {
	templates := map[string]*corev1.PodSpec{}
	add := func(name string, podSpec *corev1.PodSpec) {
		if podSpec != nil {
			templates[name] = podSpec
		}
	}
	add("template", spec.Template)
	add("worker", spec.Worker)
	if spec.Prefill != nil {
		add("prefill template", spec.Prefill.Template)
		add("prefill worker", spec.Prefill.Worker)
	}
	return templates
}

// attachModelExpress configures the main engine container of podSpec for
// ModelExpress. It always normalizes the model argument slot, so it runs for
// every engine pod whether or not ModelExpress is enabled.
func attachModelExpress(llmSvc *v1alpha2.LLMInferenceService, podSpec *corev1.PodSpec, mx *modelExpressConfig) error {
	c := utils.GetContainerWithName(podSpec, mainContainerName)
	if c == nil {
		return nil
	}

	modelArgs := ""
	if mx.native() && strings.HasPrefix(llmSvc.Spec.Model.URI.String(), constants.HfURIPrefix) {
		var err error
		if modelArgs, err = hfModelArgs(llmSvc.Spec.Model.URI.String()); err != nil {
			return err
		}
	}
	if err := fillModelArgsSlot(c, modelArgs); err != nil {
		return err
	}

	if mx == nil {
		return nil
	}

	if !slices.ContainsFunc(c.Args, func(a string) bool {
		return a == "--load-format" || strings.HasPrefix(a, "--load-format=")
	}) {
		c.Args = append(c.Args, "--load-format", modelExpressLoadFormat)
	}

	utils.AddEnvVars(c, []corev1.EnvVar{
		{Name: "MX_SERVER_ADDRESS", Value: mx.Server.Address},
		{Name: "MODEL_EXPRESS_URL", Value: mx.Server.Address},
		{Name: "MX_MODEL_REVISION", Value: mx.Revision},
		fieldRefEnv("POD_NAME", "metadata.name"),
		fieldRefEnv("POD_NAMESPACE", "metadata.namespace"),
		fieldRefEnv("POD_UID", "metadata.uid"),
		fieldRefEnv("MX_WORKER_HOST", "status.podIP"),
	})

	if mx.Server.TokenAudience != "" {
		podSpec.Volumes = utils.AppendVolumeIfNotExists(podSpec.Volumes, corev1.Volume{
			Name: modelExpressTokenVolumeName,
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{{
						ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
							Audience:          mx.Server.TokenAudience,
							ExpirationSeconds: ptr.To(modelExpressTokenExpirationSeconds),
							Path:              modelExpressTokenFile,
						},
					}},
				},
			},
		})
		addVolumeMount(c, corev1.VolumeMount{Name: modelExpressTokenVolumeName, MountPath: modelExpressTokenMountPath, ReadOnly: true})
		utils.AddEnvVars(c, []corev1.EnvVar{
			{Name: "MX_AUTH_TOKEN_PATH", Value: path.Join(modelExpressTokenMountPath, modelExpressTokenFile)},
		})
	}

	if modelArgs != "" {
		attachModelExpressServerCache(podSpec, c)
	}
	return nil
}

// attachModelExpressServerCache has the ModelExpress server supply an hf://
// model: workers fetch files from the server into the Hugging Face cache vLLM
// reads offline.
func attachModelExpressServerCache(podSpec *corev1.PodSpec, c *corev1.Container) {
	cacheDir := ""
	if i := slices.IndexFunc(c.Env, func(e corev1.EnvVar) bool { return e.Name == "HF_HUB_CACHE" }); i >= 0 && c.Env[i].ValueFrom == nil {
		cacheDir = c.Env[i].Value
	}
	if cacheDir == "" {
		cacheDir = modelExpressCacheMountPath
		podSpec.Volumes = utils.AppendVolumeIfNotExists(podSpec.Volumes, corev1.Volume{
			Name:         modelExpressCacheVolumeName,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
		addVolumeMount(c, corev1.VolumeMount{Name: modelExpressCacheVolumeName, MountPath: modelExpressCacheMountPath})
		utils.AddEnvVars(c, []corev1.EnvVar{{Name: "HF_HUB_CACHE", Value: cacheDir}})
	}
	utils.AddEnvVars(c, []corev1.EnvVar{
		{Name: "MODEL_EXPRESS_NO_SHARED_STORAGE", Value: "1"},
		{Name: "MODEL_EXPRESS_CACHE_DIRECTORY", Value: cacheDir},
		{Name: "HF_HUB_OFFLINE", Value: "1"},
	})
}

// attachModelExpressObjectStore hands the weights of an s3:// model to
// ModelStreamer. The storage initializer still downloads every other file to
// modelPath, and its S3 credentials and CA bundle are shared with the engine.
func attachModelExpressObjectStore(podSpec *corev1.PodSpec, containerName string, modelURI string) error {
	initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName)
	if initContainer == nil {
		return errors.New("ModelExpress: storage-initializer init container not found for s3:// model")
	}
	c := utils.GetContainerWithName(podSpec, containerName)
	if c == nil {
		return fmt.Errorf("ModelExpress: container %q not found", containerName)
	}

	patterns := make([]string, len(modelExpressWeightPatterns))
	for i, p := range modelExpressWeightPatterns {
		patterns[i] = `"` + p + `"`
	}
	utils.AddEnvVars(initContainer, []corev1.EnvVar{
		{Name: "STORAGE_IGNORE_PATTERNS", Value: "[" + strings.Join(patterns, ", ") + "]"},
	})

	var s3Env []corev1.EnvVar
	for _, e := range initContainer.Env {
		if strings.HasPrefix(e.Name, "AWS_") || strings.HasPrefix(e.Name, "S3_") {
			s3Env = append(s3Env, e)
		}
	}
	utils.AddEnvVars(c, s3Env)
	utils.AddEnvVars(c, []corev1.EnvVar{{Name: "MX_MODEL_URI", Value: modelURI}})

	if endpoint := envValue(c, "AWS_ENDPOINT_URL"); endpoint != "" {
		virtual := "0"
		if strings.EqualFold(envValue(c, "S3_USER_VIRTUAL_BUCKET"), "true") {
			virtual = "1"
		}
		utils.AddEnvVars(c, []corev1.EnvVar{
			{Name: "RUNAI_STREAMER_S3_USE_VIRTUAL_ADDRESSING", Value: virtual},
			{Name: "AWS_EC2_METADATA_DISABLED", Value: "true"},
		})
	}

	if i := slices.IndexFunc(initContainer.VolumeMounts, func(m corev1.VolumeMount) bool { return m.Name == CaBundleVolumeName }); i >= 0 {
		mount := initContainer.VolumeMounts[i]
		addVolumeMount(c, mount)
		utils.AddEnvVars(c, []corev1.EnvVar{
			{Name: "AWS_CA_BUNDLE", Value: path.Join(mount.MountPath, constants.DefaultCaBundleFileName)},
		})
	}
	return nil
}

// hfModelArgs renders the vLLM model argument for hf://owner/model[:revision].
func hfModelArgs(uri string) (string, error) {
	repo, revision, _ := strings.Cut(strings.TrimPrefix(uri, constants.HfURIPrefix), ":")
	if owner, name, ok := strings.Cut(repo, "/"); !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", fmt.Errorf("invalid Hugging Face URI %q: expected hf://owner/model[:revision]", uri)
	}
	args := utils.ShellQuote(repo)
	if revision != "" {
		args += " --revision " + utils.ShellQuote(revision)
	}
	return args, nil
}

// modelArgsSlotError reports a spec that writes to the model argument slot the
// controller owns.
type modelArgsSlotError struct {
	container string
}

func (e *modelArgsSlotError) Error() string {
	return fmt.Sprintf("container %q sets env %s, which carries the model argument the controller generates; "+
		"set spec.model.uri instead", e.container, modelArgsEnvVar)
}

// fillModelArgsSlot sets the slot to value, or removes it when value is empty.
// Containers that do not declare the slot are left untouched.
func fillModelArgsSlot(c *corev1.Container, value string) error {
	first := -1
	for i, e := range c.Env {
		if e.Name != modelArgsEnvVar {
			continue
		}
		if e.ValueFrom != nil || e.Value != "" {
			return &modelArgsSlotError{container: c.Name}
		}
		if first < 0 {
			first = i
		}
	}
	if first < 0 {
		return nil
	}
	c.Env = slices.DeleteFunc(c.Env, func(e corev1.EnvVar) bool { return e.Name == modelArgsEnvVar })
	if value != "" {
		c.Env = slices.Insert(c.Env, first, corev1.EnvVar{Name: modelArgsEnvVar, Value: value})
	}
	return nil
}

func fieldRefEnv(name, fieldPath string) corev1.EnvVar {
	return corev1.EnvVar{Name: name, ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: fieldPath}}}
}

func envValue(c *corev1.Container, name string) string {
	for _, e := range c.Env {
		if e.Name == name && e.ValueFrom == nil {
			return e.Value
		}
	}
	return ""
}

func addVolumeMount(c *corev1.Container, mount corev1.VolumeMount) {
	if slices.ContainsFunc(c.VolumeMounts, func(m corev1.VolumeMount) bool { return m.Name == mount.Name && m.MountPath == mount.MountPath }) {
		return
	}
	c.VolumeMounts = append(c.VolumeMounts, mount)
}
