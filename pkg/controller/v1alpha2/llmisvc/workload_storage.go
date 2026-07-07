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
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/cabundleconfigmap"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/credentials/s3"
	"github.com/kserve/kserve/pkg/utils"
)

const CaBundleVolumeName = "cabundle-cert"

// resolveStorageContainerSpec finds a ClusterStorageContainer whose
// supportedUriFormats match modelUri. If explicitName is non-empty the lookup
// is by-name (with eligibility checks); otherwise the first eligible CSC
// matching the URI is returned. Returns (nil, nil) when no CSC is found so
// callers can fall through to defaults.
func resolveStorageContainerSpec(ctx context.Context, c client.Client, modelUri, explicitName string) (*v1alpha1.StorageContainerSpec, error) {
	if explicitName != "" {
		sc := &v1alpha1.ClusterStorageContainer{}
		if err := c.Get(ctx, types.NamespacedName{Name: explicitName}, sc); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("ClusterStorageContainer %q not found", explicitName)
			}
			return nil, fmt.Errorf("failed to fetch ClusterStorageContainer %q: %w", explicitName, err)
		}
		if sc.IsDisabled() {
			return nil, fmt.Errorf("ClusterStorageContainer %q is disabled", explicitName)
		}
		if sc.Spec.WorkloadType != v1alpha1.InitContainer {
			return nil, fmt.Errorf("ClusterStorageContainer %q has workloadType %q; explicit selection requires %q", explicitName, sc.Spec.WorkloadType, v1alpha1.InitContainer)
		}
		supported, err := sc.Spec.IsStorageUriSupported(modelUri)
		if err != nil {
			return nil, fmt.Errorf("ClusterStorageContainer %q URI check failed for %q: %w", explicitName, modelUri, err)
		}
		if !supported {
			return nil, fmt.Errorf("ClusterStorageContainer %q does not support storageUri %q", explicitName, modelUri)
		}
		return &sc.Spec, nil
	}

	list := &v1alpha1.ClusterStorageContainerList{}
	if err := c.List(ctx, list); err != nil {
		return nil, err
	}
	for _, sc := range list.Items {
		if sc.IsDisabled() || sc.Spec.WorkloadType != v1alpha1.InitContainer {
			continue
		}
		ok, err := sc.Spec.IsStorageUriSupported(modelUri)
		if err != nil {
			return nil, fmt.Errorf("error checking ClusterStorageContainer %s: %w", sc.Name, err)
		}
		if ok {
			return &sc.Spec, nil
		}
	}
	return nil, nil
}

// initContainerFromCSC builds the storage-initializer init container from a
// ClusterStorageContainer's container spec, merged over a minimal default that
// carries the supplied args. When csc is nil, only the default image/args are
// used. If the current pod already has a storage-initializer container, its
// image is preserved to avoid unnecessary pod restarts on reconcile.
func initContainerFromCSC(csc *v1alpha1.StorageContainerSpec, containerArgs []string, currentInitContainers []corev1.Container) (*corev1.Container, error) {
	preservedImage := ""
	for _, ic := range currentInitContainers {
		if ic.Name == constants.StorageInitializerContainerName {
			preservedImage = ic.Image
			break
		}
	}
	base := &corev1.Container{
		Name:                     constants.StorageInitializerContainerName,
		Image:                    preservedImage,
		Args:                     containerArgs,
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
	}
	if csc == nil {
		if base.Image == "" {
			base.Image = constants.StorageInitializerContainerImage + ":" + constants.StorageInitializerContainerImageVersion
		}
		return base, nil
	}
	crd := csc.Container.DeepCopy()
	// If the current pod already carries a storage-initializer image, keep
	// it; otherwise let the CSC's image win via strategic merge.
	if preservedImage != "" {
		crd.Image = preservedImage
	}
	if err := utils.MergeContainerSpecs(base, crd); err != nil {
		return nil, err
	}
	// Args on the merged container come from strategic merge (crd.Args replaces
	// base.Args). We always want the caller's containerArgs to win, since they
	// encode the source-uri/dest-path pairs.
	base.Args = containerArgs
	return base, nil
}

// storageDownloadPair is one uri→path pair for the storage-initializer (multi-arg entrypoint).
type storageDownloadPair struct {
	uri  string
	path string
}

var tokenizerOnlyDownload = corev1.EnvVar{
	Name:  "STORAGE_ALLOW_PATTERNS",
	Value: `["tokenizer.json", "tokenizer_config.json", "special_tokens_map.json", "vocab.json", "merges.txt", "config.json", "generation_config.json"]`,
}

// stripPriorControllerStorageInitializer removes the storage-initializer init container that would
// duplicate the one the controller is about to add: merged/user templates often already
// define "storage-initializer".
func stripPriorControllerStorageInitializer(podSpec *corev1.PodSpec) {
	if podSpec == nil {
		return
	}
	keptInit := podSpec.InitContainers[:0]
	for _, ic := range podSpec.InitContainers {
		if ic.Name == constants.StorageInitializerContainerName {
			continue
		}
		keptInit = append(keptInit, ic)
	}
	podSpec.InitContainers = keptInit
}

// attachModelArtifacts configures a PodSpec to fetch and use a model from a provided URI in the LLMInferenceService.
// The storage backend (PVC, OCI, Hugging Face, or S3) is determined from the URI schema and the appropriate helper function
// is called to configure the PodSpec. This function will adjust volumes, container arguments, container volume mounts,
// add containers, and do other changes to the PodSpec to ensure the model is fetched properly from storage.
//
// Parameters:
//   - ctx: The context for API calls and logging.
//   - serviceAccount: service account associated with the LLMInferenceService.
//   - llmSvc: The LLMInferenceService resource containing the model specification.
//   - podSpec: The PodSpec to configure with the model artifact.
//   - config: The configuration information for LLMInferenceServices.
//
// Returns:
//
//	An error if the configuration fails, otherwise nil.
func (r *LLMISVCReconciler) attachModelArtifacts(ctx context.Context, serviceAccount *corev1.ServiceAccount, llmSvc *v1alpha2.LLMInferenceService, curr corev1.PodSpec, podSpec *corev1.PodSpec, config *Config, containerName string, modelPath string, attachLoRA bool) error {
	modelUri := llmSvc.Spec.Model.URI.String()
	schema, _, sepFound := strings.Cut(modelUri, "://")

	if !sepFound {
		return fmt.Errorf("invalid model URI: %s", modelUri)
	}

	// Check if storage-initializer is explicitly disabled
	storageInitializerDisabled := llmSvc.Spec.StorageInitializer != nil &&
		llmSvc.Spec.StorageInitializer.Enabled != nil &&
		!*llmSvc.Spec.StorageInitializer.Enabled
	if storageInitializerDisabled {
		// Skip storage-initializer when explicitly disabled
		return nil
	}

	// Rewrite model URI to use cached PVC when local model cache is active
	if _, ok := llmSvc.Labels[constants.LocalModelLabel]; ok {
		sourceUri, ok := llmSvc.Annotations[constants.LocalModelSourceUriAnnotationKey]
		if !ok {
			return fmt.Errorf("LLMInferenceService %s/%s: annotation %s not found", llmSvc.Namespace, llmSvc.Name, constants.LocalModelSourceUriAnnotationKey)
		}
		pvcName, ok := llmSvc.Annotations[constants.LocalModelPVCNameAnnotationKey]
		if !ok {
			return fmt.Errorf("LLMInferenceService %s/%s: annotation %s not found", llmSvc.Namespace, llmSvc.Name, constants.LocalModelPVCNameAnnotationKey)
		}
		subPath, _ := strings.CutPrefix(modelUri, sourceUri)
		if !strings.HasPrefix(subPath, "/") {
			subPath = "/" + subPath
		}
		storageKey := v1alpha1.GetStorageKey(sourceUri)
		modelUri = "pvc://" + pvcName + "/models/" + storageKey + subPath
		schema = "pvc"
	}

	var loraPairs []storageDownloadPair
	if attachLoRA {
		loraPairs = collectLoRADownloadPairs(config.ResolvedLoRAAdapters)
	}

	// Resolve a ClusterStorageContainer for this URI. PVC URIs are mounted
	// directly so they don't need a CSC. An operator can pin a specific CSC via
	// the well-known annotation; otherwise we auto-match by URI prefix.
	var csc *v1alpha1.StorageContainerSpec
	if schema+"://" != constants.PvcURIPrefix {
		explicit := llmSvc.Annotations[constants.StorageContainerNameAnnotationKey]
		found, err := resolveStorageContainerSpec(ctx, r.Client, modelUri, explicit)
		if err != nil {
			return fmt.Errorf("failed to resolve ClusterStorageContainer for %q: %w", modelUri, err)
		}
		csc = found
	}

	// Handle model artifact downloads based on URI scheme
	switch schema + "://" {
	case constants.PvcURIPrefix:
		if err := r.attachPVCModelArtifact(modelUri, podSpec, containerName, modelPath); err != nil {
			return err
		}
		if len(loraPairs) > 0 {
			if err := r.attachMultiStorageDownloads(ctx, serviceAccount, llmSvc, curr, podSpec, csc, config.CredentialConfig, containerName, loraPairs); err != nil {
				return err
			}
		}

	case constants.OciURIPrefix:
		// OCI (modelcar) requires a matching ClusterStorageContainer.
		if csc == nil {
			return errors.New("no ClusterStorageContainer found for oci:// URI; deploy a ClusterStorageContainer whose supportedUriFormats matches oci:// to enable modelcar")
		}
		if err := r.attachOciModelArtifact(modelUri, podSpec, csc, containerName, modelPath); err != nil {
			return err
		}
		if len(loraPairs) > 0 {
			if err := r.attachMultiStorageDownloads(ctx, serviceAccount, llmSvc, curr, podSpec, csc, config.CredentialConfig, containerName, loraPairs); err != nil {
				return err
			}
		}

	case constants.HfURIPrefix:
		if len(loraPairs) == 0 {
			if err := r.attachHfModelArtifact(ctx, serviceAccount, llmSvc, modelUri, curr, podSpec, csc, config.CredentialConfig, containerName, modelPath); err != nil {
				return err
			}
		} else {
			pairs := append([]storageDownloadPair{{uri: modelUri, path: constants.DefaultModelLocalMountPath}}, loraPairs...)
			if err := r.attachMultiStorageDownloads(ctx, serviceAccount, llmSvc, curr, podSpec, csc, config.CredentialConfig, containerName, pairs); err != nil {
				return err
			}
		}

	case constants.S3URIPrefix:
		if len(loraPairs) == 0 {
			if err := r.attachS3ModelArtifact(ctx, serviceAccount, llmSvc, modelUri, curr, podSpec, csc, config.CredentialConfig, containerName, modelPath); err != nil {
				return err
			}
		} else {
			pairs := append([]storageDownloadPair{{uri: modelUri, path: constants.DefaultModelLocalMountPath}}, loraPairs...)
			if err := r.attachMultiStorageDownloads(ctx, serviceAccount, llmSvc, curr, podSpec, csc, config.CredentialConfig, containerName, pairs); err != nil {
				return err
			}
		}

	default:
		return fmt.Errorf("unsupported schema in model URI: %s", modelUri)
	}

	// Attach LoRA adapters (PVC mounts + vLLM flag injection) after model downloads
	if attachLoRA {
		if err := r.attachLoRAAdapters(ctx, llmSvc, podSpec, config.ResolvedLoRAAdapters); err != nil {
			return err
		}
	}

	return nil
}

// attachOciModelArtifact configures a PodSpec to use a model stored in an OCI
// registry. The modelcar sidecar and its init container inherit resources,
// security context, env, and any extra fields from the ClusterStorageContainer's
// container spec.
func (r *LLMISVCReconciler) attachOciModelArtifact(modelUri string, podSpec *corev1.PodSpec, csc *v1alpha1.StorageContainerSpec, containerName string, modelPath string) error {
	var cscContainer *corev1.Container
	if csc != nil {
		cscContainer = &csc.Container
	}
	return utils.ConfigureModelcarToContainerFromCSC(modelUri, podSpec, containerName, modelPath, cscContainer, 0)
}

// attachPVCModelArtifact mounts a model artifact from a PersistentVolumeClaim (PVC) to the specified PodSpec.
// It adds the PVC as a volume and mounts it to the `main` container. The mount path is added to the arguments of the
// `main` container, assuming the model server expects a positional argument indicating the location of the model (which is the case of vLLM)
//
// Parameters:
//   - modelUri: The URI of the model, expected to have a PVC prefix.
//   - podSpec: The PodSpec to which the PVC volume and mount should be attached.
//
// Returns:
//
//	An error if attaching the PVC model artifact fails, otherwise nil.
//
// TODO: For now, this supports only direct mount. Copying from PVC would come later (if it makes sense at all).
func (r *LLMISVCReconciler) attachPVCModelArtifact(modelUri string, podSpec *corev1.PodSpec, containerName string, modelPath string) error {
	pvcName, pvcPath, err := utils.ParsePvcURI(modelUri)
	if err != nil {
		return err
	}

	storageMountParams := utils.StorageMountParams{
		MountPath:  modelPath,
		VolumeName: constants.PvcSourceMountName,
		ReadOnly:   true,
		PVCName:    pvcName,
		SubPath:    pvcPath,
	}

	if err := utils.AddModelMount(storageMountParams, containerName, podSpec); err != nil {
		return err
	}

	return nil
}

// attachS3ModelArtifact configures a PodSpec to use a model stored in an
// S3-compatible object store. The storage-initializer init container is built
// from the resolved ClusterStorageContainer.
func (r *LLMISVCReconciler) attachS3ModelArtifact(ctx context.Context, serviceAccount *corev1.ServiceAccount, llmSvc *v1alpha2.LLMInferenceService, modelUri string, curr corev1.PodSpec, podSpec *corev1.PodSpec, csc *v1alpha1.StorageContainerSpec, credentialConfig *credentials.CredentialConfig, containerName string, modelPath string) error {
	if err := r.attachStorageInitializer(modelUri, curr, podSpec, csc, llmSvc.Spec.Model.Confidential, containerName, modelPath); err != nil {
		return err
	}
	if initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName); initContainer != nil {
		// If service account is nil, fetch the default service account
		if serviceAccount == nil {
			serviceAccount = &corev1.ServiceAccount{}
			err := r.Get(ctx, types.NamespacedName{Name: constants.LLMISVCDefaultServiceAccountName, Namespace: llmSvc.Namespace}, serviceAccount)
			if err != nil {
				log.FromContext(ctx).Error(err, "Failed to find default service account", "namespace", llmSvc.Namespace)
				if err := r.materializeCaBundle(ctx, llmSvc.Namespace, podSpec, initContainer); err != nil {
					return err
				}
				return nil
			}
		}
		// Check for AWS IAM Role for Service Account or AWS IAM User Credentials
		credentialBuilder := credentials.NewCredentialBuilderFromConfig(r.Client, r.Clientset, *credentialConfig)
		if err := credentialBuilder.CreateSecretVolumeAndEnvFromServiceAccount(
			ctx,
			serviceAccount,
			llmSvc.Annotations,
			initContainer,
			&podSpec.Volumes,
		); err != nil {
			return err
		}
		if err := r.materializeCaBundle(ctx, llmSvc.Namespace, podSpec, initContainer); err != nil {
			return err
		}

		if containerName == tokenizerContainerName {
			utils.AddEnvVars(initContainer, []corev1.EnvVar{*tokenizerOnlyDownload.DeepCopy()})
		}
	}

	return nil
}

// attachHfModelArtifact configures a PodSpec to fetch a model from the Hugging
// Face hub. The storage-initializer init container is built from the resolved
// ClusterStorageContainer.
func (r *LLMISVCReconciler) attachHfModelArtifact(ctx context.Context, serviceAccount *corev1.ServiceAccount, llmSvc *v1alpha2.LLMInferenceService, modelUri string, curr corev1.PodSpec, podSpec *corev1.PodSpec, csc *v1alpha1.StorageContainerSpec, credentialConfig *credentials.CredentialConfig, containerName string, modelPath string) error {
	if err := r.attachStorageInitializer(modelUri, curr, podSpec, csc, llmSvc.Spec.Model.Confidential, containerName, modelPath); err != nil {
		return err
	}
	if initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName); initContainer != nil {
		// If service account is nil, fetch the default service account
		if serviceAccount == nil {
			serviceAccount = &corev1.ServiceAccount{}
			err := r.Get(ctx, types.NamespacedName{Name: constants.LLMISVCDefaultServiceAccountName, Namespace: llmSvc.Namespace}, serviceAccount)
			if err != nil {
				log.FromContext(ctx).Error(err, "Failed to find default service account", "namespace", llmSvc.Namespace)
				return nil
			}
		}
		// Check for service account with secret ref
		credentialBuilder := credentials.NewCredentialBuilderFromConfig(r.Client, r.Clientset, *credentialConfig)
		if err := credentialBuilder.CreateSecretVolumeAndEnvFromServiceAccount(
			ctx,
			serviceAccount,
			llmSvc.Annotations,
			initContainer,
			&podSpec.Volumes,
		); err != nil {
			return err
		}

		currentInitContainer := utils.GetInitContainerWithName(&curr, constants.StorageInitializerContainerName)

		// Add HF default env vars when the init container is new or already has HF_ env vars
		// from a previous reconciliation. Skip only when upgrading from an older operator version
		// that didn't set these vars, to avoid unnecessary pod restarts.
		if currentInitContainer == nil || slices.ContainsFunc(currentInitContainer.Env, func(e corev1.EnvVar) bool {
			return strings.HasPrefix(e.Name, "HF_")
		}) {
			utils.AddDefaultHuggingFaceEnvVars(initContainer)
		}

		if containerName == tokenizerContainerName {
			utils.AddEnvVars(initContainer, []corev1.EnvVar{*tokenizerOnlyDownload.DeepCopy()})
		}
	}

	return nil
}

// attachStorageInitializer configures a PodSpec to use the KServe storage
// initializer for downloading a model from a compatible storage backend. The
// init container's image, resources, env vars, and other pod-spec fields are
// sourced from the provided ClusterStorageContainer (CSC-wins semantics; see
// initContainerFromCSC).
func (r *LLMISVCReconciler) attachStorageInitializer(modelUri string, curr corev1.PodSpec, podSpec *corev1.PodSpec, csc *v1alpha1.StorageContainerSpec, confidential *v1alpha2.ConfidentialSpec, containerName string, modelPath string) error {
	stripPriorControllerStorageInitializer(podSpec)

	containerArgs := []string{
		modelUri,
		constants.DefaultModelLocalMountPath,
	}
	storageMountParams := utils.StorageMountParams{
		MountPath:  constants.DefaultModelLocalMountPath,
		VolumeName: constants.StorageInitializerVolumeName,
		ReadOnly:   false,
	}

	initContainer, err := initContainerFromCSC(csc, containerArgs, curr.InitContainers)
	if err != nil {
		return err
	}

	// Inject confidential env vars before appending to the pod spec
	if confidential != nil && confidential.Enabled {
		resourceId := ""
		if confidential.ResourceId != nil {
			resourceId = *confidential.ResourceId
		}
		utils.ApplyConfidentialContainerConfig(initContainer, resourceId)
	}

	podSpec.InitContainers = append(podSpec.InitContainers, *initContainer)

	if err := utils.AddModelMount(storageMountParams, initContainer.Name, podSpec); err != nil {
		return err
	}

	storageMountParams.ReadOnly = true
	storageMountParams.MountPath = modelPath
	if err := utils.AddModelMount(storageMountParams, containerName, podSpec); err != nil {
		return err
	}

	return nil
}

// attachMultiStorageDownloads adds one storage-initializer init container with
// multiple src_uri dest_path pairs (see storage-initializer entrypoint) and
// mounts the shared emptyDir at the common parent of all destination paths.
// The init container is built from the provided ClusterStorageContainer, which
// must have SupportsMultiModelDownload=true when non-nil.
func (r *LLMISVCReconciler) attachMultiStorageDownloads(
	ctx context.Context,
	serviceAccount *corev1.ServiceAccount,
	llmSvc *v1alpha2.LLMInferenceService,
	curr corev1.PodSpec,
	podSpec *corev1.PodSpec,
	csc *v1alpha1.StorageContainerSpec,
	credentialConfig *credentials.CredentialConfig,
	containerName string,
	pairs []storageDownloadPair,
) error {
	if len(pairs) == 0 {
		return nil
	}
	if csc != nil && (csc.SupportsMultiModelDownload == nil || !*csc.SupportsMultiModelDownload) {
		return fmt.Errorf("ClusterStorageContainer %q does not support multi-model download; enable supportsMultiModelDownload or use a compatible ClusterStorageContainer", csc.Container.Name)
	}
	stripPriorControllerStorageInitializer(podSpec)

	paths := make([]string, len(pairs))
	for i, p := range pairs {
		paths[i] = p.path
	}
	parent := utils.FindCommonParentPath(paths)
	if parent == "" {
		parent = "/"
	}

	args := make([]string, 0, len(pairs)*2)
	for _, p := range pairs {
		args = append(args, p.uri, p.path)
	}

	initC, err := initContainerFromCSC(csc, args, curr.InitContainers)
	if err != nil {
		return err
	}
	podSpec.InitContainers = append(podSpec.InitContainers, *initC)
	iname := initC.Name

	if err := utils.AddModelMount(utils.StorageMountParams{
		MountPath:  parent,
		VolumeName: constants.StorageInitializerVolumeName,
		ReadOnly:   false,
	}, iname, podSpec); err != nil {
		return err
	}
	if err := utils.AddModelMount(utils.StorageMountParams{
		MountPath:  parent,
		VolumeName: constants.StorageInitializerVolumeName,
		ReadOnly:   true,
	}, containerName, podSpec); err != nil {
		return err
	}

	initPtr := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName)
	if initPtr == nil {
		return errors.New("storage-initializer init container not found after attachMultiStorageDownloads")
	}

	if serviceAccount == nil {
		serviceAccount = &corev1.ServiceAccount{}
		err := r.Get(ctx, types.NamespacedName{Name: constants.LLMISVCDefaultServiceAccountName, Namespace: llmSvc.Namespace}, serviceAccount)
		if err != nil {
			log.FromContext(ctx).Error(err, "Failed to find default service account", "namespace", llmSvc.Namespace)
			if err := r.materializeCaBundle(ctx, llmSvc.Namespace, podSpec, initPtr); err != nil {
				return err
			}
			return nil
		}
	}

	credentialBuilder := credentials.NewCredentialBuilderFromConfig(r.Client, r.Clientset, *credentialConfig)
	if err := credentialBuilder.CreateSecretVolumeAndEnvFromServiceAccount(
		ctx,
		serviceAccount,
		llmSvc.Annotations,
		initPtr,
		&podSpec.Volumes,
	); err != nil {
		return err
	}

	needHF := slices.ContainsFunc(pairs, func(p storageDownloadPair) bool {
		return strings.HasPrefix(p.uri, constants.HfURIPrefix)
	})
	currentInit := utils.GetInitContainerWithName(&curr, constants.StorageInitializerContainerName)
	if needHF && (currentInit == nil || slices.ContainsFunc(currentInit.Env, func(e corev1.EnvVar) bool {
		return strings.HasPrefix(e.Name, "HF_")
	})) {
		utils.AddDefaultHuggingFaceEnvVars(initPtr)
	}

	if containerName == tokenizerContainerName {
		utils.AddEnvVars(initPtr, []corev1.EnvVar{*tokenizerOnlyDownload.DeepCopy()})
	}

	if err := r.materializeCaBundle(ctx, llmSvc.Namespace, podSpec, initPtr); err != nil {
		return err
	}
	return nil
}

// materializeCaBundle mounts the CA bundle ConfigMap on the init container
// (via injectCaBundle) and, when the mount points at a copy of a source
// ConfigMap in the kserve namespace, invokes the CA bundle reconciler to
// ensure that copy exists in the user namespace before the pod is scheduled.
func (r *LLMISVCReconciler) materializeCaBundle(ctx context.Context, namespace string, podSpec *corev1.PodSpec, initContainer *corev1.Container) error {
	sourceConfigMapName := injectCaBundle(namespace, podSpec, initContainer)
	if sourceConfigMapName == "" {
		return nil
	}
	reconciler := cabundleconfigmap.NewCaBundleConfigMapReconciler(r.Client, r.Clientset)
	if err := reconciler.ReconcileForSource(ctx, namespace, sourceConfigMapName); err != nil {
		return fmt.Errorf("failed to reconcile CA bundle ConfigMap %q into namespace %q: %w", sourceConfigMapName, namespace, err)
	}
	return nil
}

// caBundleConfig holds the configuration for CA bundle injection derived from
// the storage-initializer init container's environment.
type caBundleConfig struct {
	// configMapName is the ConfigMap the init container should mount. In user
	// namespaces this is always the local copy (global-ca-bundle) that the CA
	// bundle reconciler produces. In the kserve system namespace it's the
	// source name directly.
	configMapName string
	// sourceConfigMapName is the ConfigMap in the kserve system namespace that
	// the CA bundle reconciler should copy into the user namespace. Empty if
	// no CA bundle reconciliation is needed (e.g. when the caller runs in the
	// kserve namespace already).
	sourceConfigMapName string
	volumeMountPath     string
}

// extractCaBundleConfig reads CA bundle configuration exclusively from the
// init container's environment variables. Values may come from:
//
//   - CSC-set CA_BUNDLE_CONFIGMAP_NAME: names a ConfigMap in the kserve
//     system namespace; the CA bundle reconciler copies it into the user
//     namespace as constants.DefaultGlobalCaBundleConfigMapName before the
//     init container mounts it.
//
//   - Credential-builder-set AWS_CA_BUNDLE_CONFIG_MAP / AWS_CA_BUNDLE: names
//     a ConfigMap already present in the *same* namespace as the pod (via an
//     S3 secret annotation). No cross-namespace copy is needed — the init
//     container mounts that ConfigMap directly.
//
// When neither is set the returned config has an empty configMapName and the
// caller should skip CA bundle injection.
func extractCaBundleConfig(initContainer *corev1.Container, namespace string) *caBundleConfig {
	config := &caBundleConfig{}
	var (
		crossNsSource     string // from CSC-set CA_BUNDLE_CONFIGMAP_NAME
		crossNsMountPath  string // from CSC-set CA_BUNDLE_VOLUME_MOUNT_POINT
		sameNsSource      string // from credential-builder-set AWS_CA_BUNDLE_CONFIG_MAP
		sameNsMountPath   string // from credential-builder-set AWS_CA_BUNDLE
	)

	for _, envVar := range initContainer.Env {
		switch envVar.Name {
		case constants.CaBundleConfigMapNameEnvVarKey:
			crossNsSource = envVar.Value
		case constants.CaBundleVolumeMountPathEnvVarKey:
			crossNsMountPath = envVar.Value
		case s3.AWSCABundleConfigMap:
			sameNsSource = envVar.Value
		case s3.AWSCABundle:
			sameNsMountPath = filepath.Dir(envVar.Value)
		}
	}

	switch {
	case sameNsSource != "":
		// Credential-builder path — same-namespace ConfigMap, mount directly.
		// Its own mount-path env pins the mount location; CSC-declared mount
		// paths are ignored on this branch.
		config.configMapName = sameNsSource
		config.volumeMountPath = sameNsMountPath
	case crossNsSource != "":
		// CSC path — cross-namespace source needs copying by the CA bundle
		// reconciler. In the kserve namespace itself no copy is needed.
		if namespace == constants.KServeNamespace {
			config.configMapName = crossNsSource
		} else {
			config.sourceConfigMapName = crossNsSource
			config.configMapName = constants.DefaultGlobalCaBundleConfigMapName
		}
		config.volumeMountPath = crossNsMountPath
	}

	if config.volumeMountPath == "" {
		config.volumeMountPath = constants.DefaultCaBundleVolumeMountPath
	}

	return config
}

// injectCaBundle mounts the CA bundle ConfigMap onto the init container when
// the container's environment indicates that a CA bundle is expected. The
// ConfigMap name and mount path are sourced from the init container's env
// vars (populated by the ClusterStorageContainer or the AWS credential
// builder), not from any global operator config. Returns the name of the
// source ConfigMap in the kserve namespace that the CA bundle reconciler must
// copy into the user namespace before the pod can mount it; empty when no
// cross-namespace copy is needed (pod runs in kserve namespace, or no CA
// bundle is expected).
func injectCaBundle(namespace string, podSpec *corev1.PodSpec, initContainer *corev1.Container) string {
	if !needCaBundleMount(initContainer) {
		return ""
	}

	config := extractCaBundleConfig(initContainer, namespace)
	if config.configMapName == "" {
		return ""
	}

	// Overwrite existing CA bundle env vars: the CSC-set value may reference a
	// source ConfigMap in the kserve namespace, but the pod always mounts the
	// user-namespace copy under constants.DefaultGlobalCaBundleConfigMapName.
	utils.AddOrReplaceEnv(initContainer, constants.CaBundleConfigMapNameEnvVarKey, config.configMapName)
	utils.AddOrReplaceEnv(initContainer, constants.CaBundleVolumeMountPathEnvVarKey, config.volumeMountPath)

	caBundleVolume := corev1.Volume{
		Name: CaBundleVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: config.configMapName,
				},
			},
		},
	}

	caBundleVolumeMount := corev1.VolumeMount{
		Name:      CaBundleVolumeName,
		MountPath: config.volumeMountPath,
		ReadOnly:  true,
	}

	podSpec.Volumes = append(podSpec.Volumes, caBundleVolume)
	initContainer.VolumeMounts = append(initContainer.VolumeMounts, caBundleVolumeMount)

	return config.sourceConfigMapName
}

// needCaBundleMount reports whether the init container's environment indicates
// that a CA bundle should be mounted. Any of CA_BUNDLE_CONFIGMAP_NAME,
// CA_BUNDLE_VOLUME_MOUNT_POINT, or the AWS credential builder's
// AWS_CA_BUNDLE_CONFIG_MAP env var triggers the mount.
func needCaBundleMount(initContainer *corev1.Container) bool {
	for _, envVar := range initContainer.Env {
		switch envVar.Name {
		case constants.CaBundleConfigMapNameEnvVarKey,
			constants.CaBundleVolumeMountPathEnvVarKey,
			s3.AWSCABundleConfigMap:
			if envVar.Value != "" {
				return true
			}
		}
	}
	return false
}
