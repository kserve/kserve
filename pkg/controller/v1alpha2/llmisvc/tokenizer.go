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
	"path"
	"slices"

	"github.com/coreos/go-semver/semver"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	tokenizerDeploymentSuffix = "-kserve-tokenizer"  //nolint:gosec // not a credential
	tokenizerServiceSuffix    = "-tokenizer-service" //nolint:gosec // not a credential
	tokenizerServicePort      = 8000
)

var tokenizerVersionGate = *semver.New("0.9.0")

func tokenizerDeploymentName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), tokenizerDeploymentSuffix)
}

func tokenizerServiceName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), tokenizerServiceSuffix)
}

// TokenizerLabels returns the standard label set for tokenizer resources.
func TokenizerLabels(llmSvc *v1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentTokenizer,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
	}
}

// schedulerVersionAtLeast returns true when the scheduler template's
// app.kubernetes.io/version annotation is >= the given threshold.
func schedulerVersionAtLeast(spec v1alpha2.LLMInferenceServiceSpec, threshold semver.Version) bool {
	if spec.Router == nil || spec.Router.Scheduler == nil || spec.Router.Scheduler.Annotations == nil {
		return false
	}
	raw, ok := spec.Router.Scheduler.Annotations["app.kubernetes.io/version"]
	if !ok || raw == "" {
		return false
	}
	v, err := semver.NewVersion(raw)
	if err != nil {
		return false
	}
	return v.Compare(threshold) >= 0
}

// shouldDeployStandaloneTokenizer returns true when the tokenizer container in
// the scheduler template should be extracted into a separate Deployment rather
// than kept as a sidecar. This is the case when:
//  1. A scheduler with a pod template is configured (not an external pool ref)
//  2. The template contains a container named "tokenizer"
//  3. The scheduler version annotation is >= tokenizerVersionGate
func shouldDeployStandaloneTokenizer(spec v1alpha2.LLMInferenceServiceSpec) bool {
	if !isUsingTokenizerSidecar(spec) {
		return false
	}
	return schedulerVersionAtLeast(spec, tokenizerVersionGate)
}

// extractTokenizerContainer returns a deep copy of the tokenizer container and
// its associated volumes from the scheduler template. Returns nil if not found.
func extractTokenizerContainer(spec v1alpha2.LLMInferenceServiceSpec) (*corev1.Container, []corev1.Volume) {
	if spec.Router == nil || spec.Router.Scheduler == nil || spec.Router.Scheduler.Template == nil {
		return nil, nil
	}
	tmpl := spec.Router.Scheduler.Template

	idx := slices.IndexFunc(tmpl.Containers, func(c corev1.Container) bool {
		return c.Name == tokenizerContainerName
	})
	if idx < 0 {
		return nil, nil
	}

	container := tmpl.Containers[idx].DeepCopy()

	// Collect volumes referenced by the tokenizer container's volume mounts.
	mountNames := make(map[string]bool, len(container.VolumeMounts))
	for _, vm := range container.VolumeMounts {
		mountNames[vm.Name] = true
	}
	var volumes []corev1.Volume
	for _, v := range tmpl.Volumes {
		if mountNames[v.Name] {
			volumes = append(volumes, *v.DeepCopy())
		}
	}

	return container, volumes
}

// reconcileTokenizer creates or deletes the standalone tokenizer Deployment and
// its associated ClusterIP Service. It is called from reconcileScheduler.
func (r *LLMISVCReconciler) reconcileTokenizer(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	if err := r.reconcileTokenizerDeployment(ctx, llmSvc); err != nil {
		return err
	}
	return r.reconcileTokenizerService(ctx, llmSvc)
}

func (r *LLMISVCReconciler) reconcileTokenizerDeployment(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	expected, err := r.expectedTokenizerDeployment(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to build expected tokenizer deployment: %w", err)
	}

	if isStopped := utils.GetForceStopRuntime(llmSvc); isStopped || !shouldDeployStandaloneTokenizer(llmSvc.Spec) {
		return Delete(ctx, r, llmSvc, expected)
	}

	if err := Reconcile(ctx, r, llmSvc, &appsv1.Deployment{}, expected, semanticDeploymentIsEqual, PreserveDeploymentReplicas()); err != nil {
		return fmt.Errorf("failed to reconcile tokenizer deployment %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}
	return nil
}

func (r *LLMISVCReconciler) reconcileTokenizerService(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	expected := r.expectedTokenizerService(llmSvc)

	if isStopped := utils.GetForceStopRuntime(llmSvc); isStopped || !shouldDeployStandaloneTokenizer(llmSvc.Spec) {
		return Delete(ctx, r, llmSvc, expected)
	}

	if err := Reconcile(ctx, r, llmSvc, &corev1.Service{}, expected, semanticServiceIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile tokenizer service %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}
	return nil
}

func (r *LLMISVCReconciler) expectedTokenizerDeployment(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*appsv1.Deployment, error) {
	labels := TokenizerLabels(llmSvc)

	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tokenizerDeploymentName(llmSvc),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
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

	if !shouldDeployStandaloneTokenizer(llmSvc.Spec) {
		return d, nil
	}

	container, volumes := extractTokenizerContainer(llmSvc.Spec)
	if container == nil {
		return d, nil
	}

	d.Spec.Template.Spec.Containers = []corev1.Container{*container}
	d.Spec.Template.Spec.Volumes = volumes

	// Reuse the scheduler SA which has credentials propagated from the main workload SA.
	sa, _, saErr := r.expectedSchedulerServiceAccount(ctx, llmSvc)
	if saErr != nil {
		return d, fmt.Errorf("failed to get scheduler service account for tokenizer: %w", saErr)
	}
	d.Spec.Template.Spec.ServiceAccountName = sa.GetName()

	// Attach model artifacts so the tokenizer can download the model tokenizer files.
	config, err := r.loadConfig(ctx)
	if err != nil {
		return d, fmt.Errorf("failed to load config for tokenizer deployment: %w", err)
	}

	curr := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(d), curr); err != nil && !apierrors.IsNotFound(err) {
		return d, fmt.Errorf("failed to get current tokenizer deployment %s/%s: %w", d.GetNamespace(), d.GetName(), err)
	}

	modelPath := path.Join(constants.DefaultModelLocalMountPath, "base")

	if err := r.attachModelArtifacts(ctx, sa, llmSvc, curr.Spec.Template.Spec,
		&d.Spec.Template.Spec, config, tokenizerContainerName, modelPath, false); err != nil {
		return d, fmt.Errorf("failed to attach model artifacts to tokenizer deployment: %w", err)
	}

	log.FromContext(ctx).V(2).Info("Expected tokenizer deployment", "deployment", d)

	return d, nil
}

func (r *LLMISVCReconciler) expectedTokenizerService(llmSvc *v1alpha2.LLMInferenceService) *corev1.Service {
	labels := TokenizerLabels(llmSvc)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tokenizerServiceName(llmSvc),
			Namespace: llmSvc.GetNamespace(),
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       tokenizerServicePort,
					TargetPort: intstr.FromInt32(tokenizerServicePort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// tokenizerEndpointURL returns the in-cluster URL for the tokenizer Service.
func tokenizerEndpointURL(llmSvc *v1alpha2.LLMInferenceService) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		tokenizerServiceName(llmSvc),
		llmSvc.GetNamespace(),
		tokenizerServicePort,
	)
}
