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
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/credentials/s3"
	kserveTypes "github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

const CaBundleVolumeName = "cabundle-cert"

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
func (r *LLMISVCReconciler) attachModelArtifacts(ctx context.Context, serviceAccount *corev1.ServiceAccount, llmSvc *v1alpha2.LLMInferenceService, podSpec *corev1.PodSpec, config *Config) error {
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

	switch schema + "://" {
	case constants.PvcURIPrefix:
		return r.attachPVCModelArtifact(modelUri, podSpec)

	case constants.OciURIPrefix:
		// Check of OCI is enabled
		if !config.StorageConfig.EnableOciImageSource {
			return errors.New("OCI modelcars is not enabled")
		}

		return r.attachOciModelArtifact(modelUri, podSpec, config.StorageConfig)

	case constants.HfURIPrefix:
		return r.attachHfModelArtifact(ctx, serviceAccount, llmSvc, modelUri, podSpec, config.StorageConfig, config.CredentialConfig)

	case constants.S3URIPrefix:
		return r.attachS3ModelArtifact(ctx, serviceAccount, llmSvc, modelUri, podSpec, config.StorageConfig, config.CredentialConfig)
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
func (r *LLMISVCReconciler) attachOciModelArtifact(modelUri string, podSpec *corev1.PodSpec, storageConfig *kserveTypes.StorageInitializerConfig) error {
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
	pvcName, pvcPath, err := utils.ParsePvcURI(modelUri)
	if err != nil {
		return err
	}

	storageMountParams := utils.StorageMountParams{
		MountPath:  constants.DefaultModelLocalMountPath,
		VolumeName: constants.PvcSourceMountName,
		ReadOnly:   true,
		PVCName:    pvcName,
		SubPath:    pvcPath,
	}

	if err := utils.AddModelMount(storageMountParams, "main", podSpec); err != nil {
		return err
	}

	return nil
}

// attachS3ModelArtifact configures a PodSpec to use a model stored in an S3-compatible object store.
// Model downloading is delegated to vLLM by passing the S3 URI and other required arguments.
//
// Parameters:
//   - ctx: The context for API calls and logging.
//   - serviceAccount: service account associated with the LLMInferenceService.
//   - llmSvc: The LLMInferenceService resource containing the model specification.
//   - modelUri: The URI of the model in the S3-compatible object store.
//   - podSpec: The PodSpec to which the S3 model should be attached.
//   - storageConfig: The storage initializer configuration.
//   - credentialConfig: The credential configuration used for model downloads.
//
// Returns:
//
//	An error if the configuration fails, otherwise nil.
func (r *LLMISVCReconciler) attachS3ModelArtifact(ctx context.Context, serviceAccount *corev1.ServiceAccount, llmSvc *v1alpha2.LLMInferenceService, modelUri string, podSpec *corev1.PodSpec, storageConfig *kserveTypes.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) error {
	if err := r.attachStorageInitializer(modelUri, podSpec, storageConfig); err != nil {
		return err
	}
	if initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName); initContainer != nil {
		// If service account is nil, fetch the default service account
		if serviceAccount == nil {
			serviceAccount = &corev1.ServiceAccount{}
			err := r.Client.Get(ctx, types.NamespacedName{Name: "default", Namespace: llmSvc.Namespace}, serviceAccount)
			if err != nil {
				log.FromContext(ctx).Error(err, "Failed to find default service account", "namespace", llmSvc.Namespace)
				injectCaBundle(llmSvc.Namespace, podSpec, initContainer, storageConfig)
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
		injectCaBundle(llmSvc.Namespace, podSpec, initContainer, storageConfig)
	}

	return nil
}

// attachHfModelArtifact configures a PodSpec to use a model stored in the hugging face hub.
// Model downloading is delegated to vLLM by passing the HF URI and other required arguments.
//
// Parameters:
//   - ctx: The context for API calls and logging.
//   - serviceAccount: service account associated with the LLMInferenceService.
//   - llmSvc: The LLMInferenceService resource containing the model specification.
//   - modelUri: The URI of the model in the S3-compatible object store.
//   - podSpec: The PodSpec to which the S3 model should be attached.
//   - storageConfig: The storage initializer configuration.
//   - credentialConfig: The credential configuration used for model downloads.
//
// Returns:
//
//	An error if the configuration fails, otherwise nil.
func (r *LLMISVCReconciler) attachHfModelArtifact(ctx context.Context, serviceAccount *corev1.ServiceAccount, llmSvc *v1alpha2.LLMInferenceService, modelUri string, podSpec *corev1.PodSpec, storageConfig *kserveTypes.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) error {
	if err := r.attachStorageInitializer(modelUri, podSpec, storageConfig); err != nil {
		return err
	}
	if initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName); initContainer != nil {
		// If service account is nil, fetch the default service account
		if serviceAccount == nil {
			serviceAccount = &corev1.ServiceAccount{}
			err := r.Client.Get(ctx, types.NamespacedName{Name: "default", Namespace: llmSvc.Namespace}, serviceAccount)
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
	}

	return nil
}

// attachStorageInitializer configures a PodSpec to use KServe storage-initializer for
// downloading a model from compatible storage.
//
// Parameters:
//   - modelUri: The URI of the model in compatible object store.
//   - podSpec: The PodSpec to which the storage-initializer container should be attached.
//   - storageConfig: The storage initializer configuration.
//
// Returns:
//
//	An error if the configuration fails, otherwise nil.
func (r *LLMISVCReconciler) attachStorageInitializer(modelUri string, podSpec *corev1.PodSpec, storageConfig *kserveTypes.StorageInitializerConfig) error {
	containerArgs := []string{
		modelUri,
		constants.DefaultModelLocalMountPath,
	}
	storageMountParams := utils.StorageMountParams{
		MountPath:  constants.DefaultModelLocalMountPath,
		VolumeName: constants.StorageInitializerVolumeName,
		ReadOnly:   false,
	}
	initContainer := utils.CreateInitContainerWithConfig(storageConfig, containerArgs)
	podSpec.InitContainers = append(podSpec.InitContainers, *initContainer)

	utils.AddModelMount(storageMountParams, initContainer.Name, podSpec)

	storageMountParams.ReadOnly = true
	utils.AddModelMount(storageMountParams, "main", podSpec)

	return nil
}

// caBundleConfig holds the configuration for CA bundle injection
type caBundleConfig struct {
	configMapName   string
	volumeMountPath string
	envVarExists    bool
	mountPathExists bool
}

// extractCaBundleConfig extracts and processes CA bundle configuration from environment variables in a single pass
func extractCaBundleConfig(initContainer *corev1.Container, storageConfig *kserveTypes.StorageInitializerConfig, namespace string) *caBundleConfig {
	config := &caBundleConfig{
		configMapName:   storageConfig.CaBundleConfigMapName,
		volumeMountPath: storageConfig.CaBundleVolumeMountPath,
	}

	// Set defaults
	if namespace != constants.KServeNamespace {
		config.configMapName = constants.DefaultGlobalCaBundleConfigMapName
	}
	if config.volumeMountPath == "" {
		config.volumeMountPath = constants.DefaultCaBundleVolumeMountPath
	}

	// Single pass through environment variables to extract values and check existence
	for _, envVar := range initContainer.Env {
		switch envVar.Name {
		case s3.AWSCABundleConfigMap:
			config.configMapName = envVar.Value
		case s3.AWSCABundle:
			config.volumeMountPath = filepath.Dir(envVar.Value)
		case constants.CaBundleConfigMapNameEnvVarKey:
			config.envVarExists = true
		case constants.CaBundleVolumeMountPathEnvVarKey:
			config.mountPathExists = true
		}
	}

	return config
}

func injectCaBundle(namespace string, podSpec *corev1.PodSpec, initContainer *corev1.Container, storageConfig *kserveTypes.StorageInitializerConfig) bool { //nolint:unparam
	// Inject CA bundle configMap if caBundleConfigMapName or constants.DefaultGlobalCaBundleConfigMapName annotation is set
	if !needCaBundleMount(storageConfig.CaBundleConfigMapName, initContainer) {
		return false
	}

	config := extractCaBundleConfig(initContainer, storageConfig, namespace)

	// Add CA bundle env vars only if they don't already exist (could be customized by user)
	if !config.envVarExists {
		initContainer.Env = append(initContainer.Env, corev1.EnvVar{
			Name:  constants.CaBundleConfigMapNameEnvVarKey,
			Value: config.configMapName,
		})
	}

	if !config.mountPathExists {
		initContainer.Env = append(initContainer.Env, corev1.EnvVar{
			Name:  constants.CaBundleVolumeMountPathEnvVarKey,
			Value: config.volumeMountPath,
		})
	}

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

	return true
}

func needCaBundleMount(caBundleConfigMapName string, initContainer *corev1.Container) bool {
	result := false
	if caBundleConfigMapName != "" {
		result = true
	}
	for _, envVar := range initContainer.Env {
		if envVar.Name == s3.AWSCABundleConfigMap {
			result = true
			break
		}
	}
	return result
}
