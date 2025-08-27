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
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

// attachModelArtifacts configures a PodSpec to fetch and use a model from a provided URI in the LLMInferenceService.
// The storage backend (PVC, OCI, Hugging Face, or S3) is determined from the URI schema and the appropriate helper function
// is called to configure the PodSpec. This function will adjust volumes, container arguments, container volume mounts,
// add containers, and do other changes to the PodSpec to ensure the model is fetched properly from storage.
func (r *LLMISVCReconciler) attachModelArtifacts(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, podSpec *corev1.PodSpec, storageConfig *types.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) error {
	modelUri := llmSvc.Spec.Model.URI.String()
	schema, _, sepFound := strings.Cut(modelUri, "://")

	if !sepFound {
		return fmt.Errorf("invalid model URI: %s", modelUri)
	}

	switch schema + "://" {
	case constants.PvcURIPrefix:
		return r.attachPVCModelArtifact(modelUri, podSpec)

	case constants.OciURIPrefix:
		// Check of OCI is enabled
		if !storageConfig.EnableOciImageSource {
			return errors.New("OCI modelcars is not enabled")
		}

		return r.attachOciModelArtifact(modelUri, podSpec, storageConfig)

	case constants.HfURIPrefix:
		return r.attachHfModelArtifact(ctx, llmSvc.Namespace, llmSvc.Annotations, modelUri, podSpec, storageConfig, credentialConfig)

	case constants.S3URIPrefix:
		return r.attachS3ModelArtifact(ctx, llmSvc.Namespace, llmSvc.Annotations, modelUri, podSpec, storageConfig, credentialConfig)
	}

	return fmt.Errorf("unsupported schema in model URI: %s", modelUri)
}

// attachOciModelArtifact configures a PodSpec to use a model stored in an OCI registry.
// It updates the "main" container in the PodSpec to use the model from OCI image. The
// required supporting volumes and volume mounts are added to the PodSpec.
//
// Parameters:
//   - modelUri: The URI of the model in the OCI registry.
//   - podSpec: The PodSpec to which the OCI model should be attached.
//   - storageConfig: The storage initializer configuration.
//
// Returns:
//
//	An error if the configuration fails, otherwise nil.
func (r *LLMISVCReconciler) attachOciModelArtifact(modelUri string, podSpec *corev1.PodSpec, storageConfig *types.StorageInitializerConfig) error {
	if err := utils.ConfigureModelcarToContainer(modelUri, podSpec, "main", storageConfig); err != nil {
		return err
	}

	return nil
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
func (r *LLMISVCReconciler) attachPVCModelArtifact(modelUri string, podSpec *corev1.PodSpec) error {
	if err := utils.AddModelPvcMount(modelUri, "main", true, podSpec); err != nil {
		return err
	}

	return nil
}

// attachS3ModelArtifact configures a PodSpec to use a model stored in an S3-compatible object store.
// Model downloading is delegated to vLLM by passing the S3 URI and other required arguments.
//
// Parameters:
//   - ctx: The context for API calls and logging.
//   - podNamespace: The namespace in which the pod and llmisvc exist.
//   - podAnnotations: The annotations present in the pod and llmisvc.
//   - modelUri: The URI of the model in the S3-compatible object store.
//   - podSpec: The PodSpec to which the S3 model should be attached.
//   - storageConfig: The storage initializer configuration.
//   - credentialConfig: The credential configuration used for model downloads.
//
// Returns:
//
//	An error if the configuration fails, otherwise nil.
func (r *LLMISVCReconciler) attachS3ModelArtifact(ctx context.Context, podNamespace string, podAnnotations map[string]string, modelUri string, podSpec *corev1.PodSpec, storageConfig *types.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) error {
	if err := r.attachStorageInitializer(modelUri, podSpec, storageConfig); err != nil {
		return err
	}
	if initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName); initContainer != nil {
		// Check for AWS IAM Role for Service Account or AWS IAM User Credentials
		credentialBuilder := credentials.NewCredentialBuilderFromConfig(r.Client, r.Clientset, *credentialConfig)
		if err := credentialBuilder.CreateSecretVolumeAndEnv(
			ctx,
			podNamespace,
			podAnnotations,
			podSpec.ServiceAccountName,
			initContainer,
			&podSpec.Volumes,
		); err != nil {
			return err
		}
	}

	return nil
}

// attachHfModelArtifact configures a PodSpec to use a model stored in the hugging face hub.
// Model downloading is delegated to vLLM by passing the HF URI and other required arguments.
//
// Parameters:
//   - ctx: The context for API calls and logging.
//   - podNamespace: The namespace in which the pod and llmisvc exist.
//   - podAnnotations: The annotations present in the pod and llmisvc.
//   - modelUri: The URI of the model in hugging face hub.
//   - podSpec: The PodSpec to which the HF model should be attached.
//   - storageConfig: The storage initializer configuration.
//   - credentialConfig: The credential configuration used for model downloads.
//
// Returns:
//
//	An error if the configuration fails, otherwise nil.
func (r *LLMISVCReconciler) attachHfModelArtifact(ctx context.Context, podNamespace string, podAnnotations map[string]string, modelUri string, podSpec *corev1.PodSpec, storageConfig *types.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) error {
	if err := r.attachStorageInitializer(modelUri, podSpec, storageConfig); err != nil {
		return err
	}
	if initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName); initContainer != nil {
		// Check for service account with secret ref
		credentialBuilder := credentials.NewCredentialBuilderFromConfig(r.Client, r.Clientset, *credentialConfig)
		if err := credentialBuilder.CreateSecretVolumeAndEnv(
			ctx,
			podNamespace,
			podAnnotations,
			podSpec.ServiceAccountName,
			initContainer,
			&podSpec.Volumes,
		); err != nil {
			return err
		}
	}

	return nil
}

// attachStorageInitializer configures a PodSpec to use KServe storage-initializer for
// downloading a model from compatible storage.
//
// Parameters:
//   - modelUri: The URI of the model in compatible object store.
//   - podSpec: The PodSpec to which the storage-initializer container should be attached.
//
// Returns:
//
//	An error if the configuration fails, otherwise nil.
func (r *LLMISVCReconciler) attachStorageInitializer(modelUri string, podSpec *corev1.PodSpec, storageConfig *types.StorageInitializerConfig) error {
	utils.AddStorageInitializerContainer(podSpec, "main", modelUri, true, storageConfig)

	return nil
}
