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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	tokenizerDeploymentContainerName = "main"
	tokenizerServicePort             = 8000
	tokenizerPortName                = "render-http"
)

func tokenizerDeploymentName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-tokenizer")
}

func tokenizerServiceName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-tokenizer")
}

func tokenizerServiceURL(llmSvc *v1alpha2.LLMInferenceService, enableTLS bool) string {
	scheme := "http"
	if enableTLS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s.%s.svc.%s:%d",
		scheme,
		tokenizerServiceName(llmSvc),
		llmSvc.GetNamespace(),
		network.GetClusterDomainName(),
		tokenizerServicePort,
	)
}

// TokenizerLabels returns labels for tokenizer resources, distinct from SchedulerLabels.
func TokenizerLabels(llmSvc *v1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentTokenizer,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
	}
}

// isTokenizerEnabled returns true if the standalone tokenizer should be deployed.
// The tokenizer is inferred from plugin presence in the scheduler config:
//   - token-producer plugin detected (primary trigger — directly calls the tokenizer)
//   - legacy precise-prefix-cache-scorer plugin detected (migration path — will be decomposed)
//   - spec.router.scheduler.tokenizer is explicitly set (operational override)
func isTokenizerEnabled(spec v1alpha2.LLMInferenceServiceSpec) bool {
	if spec.Router == nil || spec.Router.Scheduler == nil {
		return false
	}
	if spec.Router.Scheduler.Tokenizer != nil {
		return true
	}
	return hasTokenProducerPlugin(spec) || hasPrecisePrefixCachePlugin(spec)
}

// hasTokenProducerPlugin checks if the scheduler config contains the token-producer
// plugin, which requires a standalone tokenizer deployment to serve tokenization
// requests over HTTP (vLLM render endpoint).
func hasTokenProducerPlugin(spec v1alpha2.LLMInferenceServiceSpec) bool {
	if spec.Router == nil || spec.Router.Scheduler == nil || spec.Router.Scheduler.Config == nil || spec.Router.Scheduler.Config.Inline == nil {
		return false
	}
	u := unstructured.Unstructured{}
	if err := yaml.Unmarshal(spec.Router.Scheduler.Config.Inline.Raw, &u.Object); err != nil {
		return false
	}
	return hasPluginType(u.Object, tokenProducerPlugin)
}

// hasPrecisePrefixCachePlugin checks if the scheduler config contains the legacy
// precise-prefix-cache-scorer plugin (migration trigger).
func hasPrecisePrefixCachePlugin(spec v1alpha2.LLMInferenceServiceSpec) bool {
	if spec.Router == nil || spec.Router.Scheduler == nil || spec.Router.Scheduler.Config == nil || spec.Router.Scheduler.Config.Inline == nil {
		return false
	}
	u := unstructured.Unstructured{}
	if err := yaml.Unmarshal(spec.Router.Scheduler.Config.Inline.Raw, &u.Object); err != nil {
		return false
	}
	return hasPluginType(u.Object, precisePrefixCacheScorerPlugin)
}

// shouldDeleteTokenizer returns true when tokenizer resources should be cleaned up.
func shouldDeleteTokenizer(llmSvc *v1alpha2.LLMInferenceService) bool {
	return utils.GetForceStopRuntime(llmSvc) ||
		llmSvc.Spec.Router == nil ||
		llmSvc.Spec.Router.Scheduler == nil ||
		llmSvc.Spec.Router.Scheduler.Pool.HasRef() ||
		!isTokenizerEnabled(llmSvc.Spec)
}

func (r *LLMISVCReconciler) reconcileTokenizerDeployment(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	expected, err := r.expectedTokenizerDeployment(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to build expected tokenizer deployment: %w", err)
	}

	if shouldDeleteTokenizer(llmSvc) {
		if utils.GetForceStopRuntime(llmSvc) {
			llmSvc.MarkTokenizerNotReady("Stopped", "Service is stopped")
		} else {
			llmSvc.MarkTokenizerUnset()
		}
		return Delete(ctx, r, llmSvc, expected)
	}

	if err := Reconcile(ctx, r, llmSvc, &appsv1.Deployment{}, expected, semanticDeploymentIsEqual, PreserveDeploymentReplicas()); err != nil {
		return fmt.Errorf("failed to reconcile tokenizer deployment %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}

	return r.propagateComponentDeploymentStatus(ctx, expected, llmSvc.MarkTokenizerReady, llmSvc.MarkTokenizerNotReady)
}

func (r *LLMISVCReconciler) reconcileTokenizerService(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	expected := r.expectedTokenizerService(llmSvc)
	if shouldDeleteTokenizer(llmSvc) {
		return Delete(ctx, r, llmSvc, expected)
	}

	return Reconcile(ctx, r, llmSvc, &corev1.Service{}, expected, semanticServiceIsEqual)
}

func (r *LLMISVCReconciler) expectedTokenizerDeployment(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*appsv1.Deployment, error) {
	labels := TokenizerLabels(llmSvc)
	replicas := int32(1)

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
			Replicas: &replicas,
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

	// Build the pod spec from the merged tokenizer template
	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Router.Scheduler.Tokenizer != nil &&
		llmSvc.Spec.Router.Scheduler.Tokenizer.Template != nil {
		d.Spec.Template.Spec = *llmSvc.Spec.Router.Scheduler.Tokenizer.Template.DeepCopy()
	}

	// Attach model artifacts so the tokenizer has access to tokenizer files
	var existingServiceAccount *corev1.ServiceAccount
	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil &&
		llmSvc.Spec.Router.Scheduler.Template != nil && llmSvc.Spec.Router.Scheduler.Template.ServiceAccountName != "" {
		existingServiceAccount = &corev1.ServiceAccount{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      llmSvc.Spec.Router.Scheduler.Template.ServiceAccountName,
			Namespace: llmSvc.Namespace,
		}, existingServiceAccount)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return d, fmt.Errorf("failed to fetch scheduler service account %s/%s: %w",
					llmSvc.Namespace, llmSvc.Spec.Router.Scheduler.Template.ServiceAccountName, err)
			}
			existingServiceAccount = nil
		}
	}

	curr := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(d), curr); err != nil && !apierrors.IsNotFound(err) {
		return d, fmt.Errorf("failed to get current tokenizer deployment %s/%s: %w", d.GetNamespace(), d.GetName(), err)
	}

	config, err := r.loadConfig(ctx)
	if err != nil {
		return d, fmt.Errorf("failed to load config for tokenizer deployment: %w", err)
	}

	modelPath := path.Join(constants.DefaultModelLocalMountPath, "base")
	if err := r.attachModelArtifacts(ctx, existingServiceAccount, llmSvc, curr.Spec.Template.Spec, &d.Spec.Template.Spec, config, tokenizerDeploymentContainerName, modelPath, constants.LLMISVCSchedulerAttachesLoRA, false); err != nil {
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
					Name:       tokenizerPortName,
					Port:       tokenizerServicePort,
					TargetPort: intstr.FromString(tokenizerPortName),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}
