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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials/s3"
	"github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

func (r *LLMInferenceServiceReconciler) reconcileSingleNodeWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *types.StorageInitializerConfig) error {
	log.FromContext(ctx).Info("Reconciling single-node workload")

	if err := r.reconcileSingleNodeMainServiceAccount(ctx, llmSvc, storageConfig); err != nil {
		return fmt.Errorf("failed to reconcile service account: %w", err)
	}

	if err := r.reconcileSingleNodeMainWorkload(ctx, llmSvc, storageConfig); err != nil {
		return fmt.Errorf("failed to reconcile main workload: %w", err)
	}

	if err := r.reconcileSingleNodePrefill(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile prefill workload: %w", err)
	}
	return nil
}

func (r *LLMInferenceServiceReconciler) reconcileSingleNodeMainWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *types.StorageInitializerConfig) error {
	expected, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, storageConfig)
	if err != nil {
		return fmt.Errorf("failed to get expected main deployment: %w", err)
	}
	if llmSvc.Spec.Worker != nil {
		return Delete(ctx, r, llmSvc, expected)
	}
	if err := Reconcile(ctx, r, llmSvc, &appsv1.Deployment{}, expected, semanticDeploymentIsEqual); err != nil {
		return err
	}
	return r.propagateDeploymentStatus(ctx, expected, llmSvc.MarkMainWorkloadReady, llmSvc.MarkMainWorkloadNotReady)
}

func (r *LLMInferenceServiceReconciler) expectedSingleNodeMainDeployment(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *types.StorageInitializerConfig) (*appsv1.Deployment, error) {
	if llmSvc.Spec.Template == nil {
		return nil, errors.New("llmSvc.Spec.Template must not be nil")
	}

	role := "decode"
	if llmSvc.Spec.Prefill == nil {
		role = "both"
	}

	labels := r.singleNodeLabels(llmSvc)
	labels["kserve.io/component"] = "workload"
	labels["llm-d.ai/role"] = role

	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: llmSvc.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
		},
	}

	if llmSvc.Spec.Template != nil {
		d.Spec.Template.Spec = *llmSvc.Spec.Template.DeepCopy()
		if hasRoutingSidecar(d.Spec.Template.Spec) {
			log.FromContext(ctx).Info("Main container has a routing sidecar")

			serviceAccount := r.expectedSingleNodeMainServiceAccount(llmSvc)
			d.Spec.Template.Spec.ServiceAccountName = serviceAccount.GetName()
			s := routingSidecar(&d.Spec.Template.Spec)
			if llmSvc.Spec.Router != nil {
				s.Env = append(s.Env, corev1.EnvVar{
					Name:      "INFERENCE_POOL_NAME",
					Value:     llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc),
					ValueFrom: nil,
				})
			}
		}
	}

	if err := r.attachModelArtifacts(llmSvc, &d.Spec.Template.Spec, storageConfig); err != nil {
		return nil, fmt.Errorf("failed to attach model artifacts to main deployment: %w", err)
	}

	log.FromContext(ctx).V(2).Info("Expected main deployment", "deployment", d)

	return d, nil
}

func (r *LLMInferenceServiceReconciler) reconcileSingleNodePrefill(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	prefill := r.expectedPrefillMainDeployment(ctx, llmSvc)
	if llmSvc.Spec.Prefill == nil {
		if err := Delete(ctx, r, llmSvc, prefill); err != nil {
			return fmt.Errorf("failed to delete prefill main deployment: %w", err)
		}
		return nil
	}
	if err := Reconcile(ctx, r, llmSvc, &appsv1.Deployment{}, prefill, semanticDeploymentIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile prefill deployment %s/%s: %w", prefill.GetNamespace(), prefill.GetName(), err)
	}
	return r.propagateDeploymentStatus(ctx, prefill, llmSvc.MarkPrefillWorkloadReady, llmSvc.MarkPrefillWorkloadNotReady)
}

func (r *LLMInferenceServiceReconciler) expectedPrefillMainDeployment(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) *appsv1.Deployment {
	labels := map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-workload-prefill",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
		"kserve.io/component":         "workload",
		"llm-d.ai/role":               "prefill",
	}

	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-prefill"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: labels,
		},
	}

	if llmSvc.Spec.Prefill != nil {
		d.Spec = appsv1.DeploymentSpec{
			Replicas: llmSvc.Spec.Prefill.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
		}
	}

	if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.Template != nil {
		d.Spec.Template.Spec = *llmSvc.Spec.Prefill.Template.DeepCopy()
	}

	log.FromContext(ctx).V(2).Info("Expected prefill deployment", "deployment", d)

	return d
}

// attachModelArtifacts configures a PodSpec to fetch and use a model froma provided URI in the LLMInferenceService.
// The storage backend (PVC, OCI, Hugging Face, or S3) is determined from the URI schema and the appropriate helper function
// is called to configure the PodSpec. This function will adjust volumes, container arguments, container volume mounts,
// add containers, and do other changes to the PodSpec to ensure the model is fetched properly from storage.
//
// Parameters:
//   - llmSvc: The LLMInferenceService resource containing the model specification.
//   - podSpec: The PodSpec to configure with the model artifact.
//   - storageConfig: The storage initializer configuration.
//
// Returns:
//
//	An error if the configuration fails, otherwise nil.
func (r *LLMInferenceServiceReconciler) attachModelArtifacts(llmSvc *v1alpha1.LLMInferenceService, podSpec *corev1.PodSpec, storageConfig *types.StorageInitializerConfig) error {
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
		return r.attachStorageInitializer(modelUri, podSpec, storageConfig)

	case constants.S3URIPrefix:
		return r.attachS3ModelArtifact(modelUri, podSpec, storageConfig)
	}

	return fmt.Errorf("unsupported schema in model URI: %s", modelUri)
}

// attachOciModelArtifact configures a PodSpec to use a model stored in an OCI registry.
// It updates the "main" container in the PodSpec to use the model from OCI image. The
// required supporting volumes and volume mounts are added to the PodSpec.
//
// Parameters:
//   - ctx: The context for API calls and logging.
//   - modelUri: The URI of the model in the OCI registry.
//   - podSpec: The PodSpec to which the OCI model should be attached.
//   - storageConfig: The storage initializer configuration.
//
// Returns:
//
//	An error if the configuration fails, otherwise nil.
func (r *LLMInferenceServiceReconciler) attachOciModelArtifact(modelUri string, podSpec *corev1.PodSpec, storageConfig *types.StorageInitializerConfig) error {
	if err := utils.ConfigureModelcarToContainer(modelUri, podSpec, "main", storageConfig); err != nil {
		return err
	}

	if mainContainer := utils.GetContainerWithName(podSpec, "main"); mainContainer != nil {
		mainContainer.Command = append(mainContainer.Command, constants.DefaultModelLocalMountPath)
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
func (r *LLMInferenceServiceReconciler) attachPVCModelArtifact(modelUri string, podSpec *corev1.PodSpec) error {
	if err := utils.AddModelPvcMount(modelUri, "main", true, podSpec); err != nil {
		return err
	}
	if mainContainer := utils.GetContainerWithName(podSpec, "main"); mainContainer != nil {
		mainContainer.Command = append(mainContainer.Command, constants.DefaultModelLocalMountPath)
	}

	return nil
}

// attachS3ModelArtifact configures a PodSpec to use a model stored in an S3-compatible object store.
// Model downloading is delegated to vLLM by passing the S3 URI and other required arguments.
//
// Parameters:
//   - modelUri: The URI of the model in the S3-compatible object store.
//   - podSpec: The PodSpec to which the S3 model should be attached.
//   - storageConfig: The storage initializer configuration.
//
// Returns:
//
//	An error if the configuration fails, otherwise nil.
func (r *LLMInferenceServiceReconciler) attachS3ModelArtifact(modelUri string, podSpec *corev1.PodSpec, storageConfig *types.StorageInitializerConfig) error {
	if err := r.attachStorageInitializer(modelUri, podSpec, storageConfig); err != nil {
		return err
	}
	if initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName); initContainer != nil {
		initContainer.Env = append(initContainer.Env, corev1.EnvVar{
			Name:  s3.AWSAnonymousCredential,
			Value: "true",
		})
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
func (r *LLMInferenceServiceReconciler) attachStorageInitializer(modelUri string, podSpec *corev1.PodSpec, storageConfig *types.StorageInitializerConfig) error {
	utils.AddStorageInitializerContainer(podSpec, "main", modelUri, true, storageConfig)
	if mainContainer := utils.GetContainerWithName(podSpec, "main"); mainContainer != nil {
		mainContainer.Command = append(mainContainer.Command, constants.DefaultModelLocalMountPath)
	}

	return nil
}

func (r *LLMInferenceServiceReconciler) propagateDeploymentStatus(ctx context.Context, expected *appsv1.Deployment, ready func(), notReady func(reason, messageFormat string, messageA ...interface{})) error {
	logger := log.FromContext(ctx)

	curr := &appsv1.Deployment{}
	err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return r.Client.Get(ctx, client.ObjectKeyFromObject(expected), curr)
	})
	if err != nil {
		return fmt.Errorf("failed to get current deployment %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}
	for _, cond := range curr.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable {
			if cond.Status == corev1.ConditionTrue {
				ready()
			} else {
				notReady(cond.Reason, cond.Message)
			}
			return nil
		}
	}
	logger.Info("Deployment processed")
	notReady(string(appsv1.DeploymentProgressing), "")
	return nil
}

func semanticDeploymentIsEqual(expected *appsv1.Deployment, curr *appsv1.Deployment) bool {
	return equality.Semantic.DeepDerivative(expected.Spec, curr.Spec) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}

func (r *LLMInferenceServiceReconciler) reconcileSingleNodeMainServiceAccount(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *types.StorageInitializerConfig) error {
	expectedDeployment, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, storageConfig)
	if err != nil {
		return fmt.Errorf("failed to get expected main deployment: %w", err)
	}

	serviceAccount := r.expectedSingleNodeMainServiceAccount(llmSvc)
	if !hasRoutingSidecar(expectedDeployment.Spec.Template.Spec) {
		return Delete(ctx, r, llmSvc, serviceAccount)
	}

	if err := Reconcile(ctx, r, llmSvc, &corev1.ServiceAccount{}, serviceAccount, semanticServiceAccountIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile single node service account %s/%s: %w", serviceAccount.GetNamespace(), serviceAccount.GetName(), err)
	}

	if err := r.reconcileSingleNodeMainRole(ctx, llmSvc, storageConfig); err != nil {
		return err
	}

	return r.reconcileSingleNodeMainRoleBinding(ctx, llmSvc, serviceAccount, storageConfig)
}

func (r *LLMInferenceServiceReconciler) reconcileSingleNodeMainRole(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *types.StorageInitializerConfig) error {
	expectedDeployment, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, storageConfig)
	if err != nil {
		return fmt.Errorf("failed to get expected main deployment: %w", err)
	}

	role := r.expectedSingleNodeRole(llmSvc)
	if !hasRoutingSidecar(expectedDeployment.Spec.Template.Spec) {
		return Delete(ctx, r, llmSvc, role)
	}

	if err := Reconcile(ctx, r, llmSvc, &rbacv1.Role{}, role, semanticRoleIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile single node role %s/%s: %w", role.GetNamespace(), role.GetName(), err)
	}

	return nil
}

func (r *LLMInferenceServiceReconciler) reconcileSingleNodeMainRoleBinding(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, sa *corev1.ServiceAccount, storageConfig *types.StorageInitializerConfig) error {
	expectedDeployment, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, storageConfig)
	if err != nil {
		return fmt.Errorf("failed to get expected main deployment: %w", err)
	}

	roleBinding := r.expectedSingleNodeRoleBinding(llmSvc, sa)
	if !hasRoutingSidecar(expectedDeployment.Spec.Template.Spec) {
		return Delete(ctx, r, llmSvc, roleBinding)
	}

	if err := Reconcile(ctx, r, llmSvc, &rbacv1.RoleBinding{}, roleBinding, semanticRoleBindingIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile single node rolebinding %s/%s: %w", roleBinding.GetNamespace(), roleBinding.GetName(), err)
	}

	return nil
}

func (r *LLMInferenceServiceReconciler) expectedSingleNodeMainServiceAccount(llmSvc *v1alpha1.LLMInferenceService) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: r.singleNodeLabels(llmSvc),
		},
	}
}

func (r *LLMInferenceServiceReconciler) expectedSingleNodeRole(llmSvc *v1alpha1.LLMInferenceService) *rbacv1.Role {
	ro := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-role"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: r.singleNodeLabels(llmSvc),
		},
	}
	ro.Rules = append(ro.Rules, sidecarSSRFProtectionRules...)
	return ro
}

func (r *LLMInferenceServiceReconciler) expectedSingleNodeRoleBinding(llmSvc *v1alpha1.LLMInferenceService, sa *corev1.ServiceAccount) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-rb"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: r.singleNodeLabels(llmSvc),
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      sa.GetName(),
			Namespace: sa.GetNamespace(),
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     kmeta.ChildName(llmSvc.GetName(), "-kserve-role"),
		},
	}
}

func (r *LLMInferenceServiceReconciler) singleNodeLabels(llmSvc *v1alpha1.LLMInferenceService) map[string]string {
	return map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-workload",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
	}
}
