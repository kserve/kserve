/*
Copyright 2023 The KServe Authors.

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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func (r *LLMISVCReconciler) reconcileMultiNodeWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("multi-node-workload")
	ctx = log.IntoContext(ctx, logger)

	if err := r.reconcileMultiNodeMainWorkload(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile multi-node main workload: %w", err)
	}
	if err := r.reconcileMultiNodePrefillWorkload(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile multi-node prefill workload: %w", err)
	}
	return nil
}

func (r *LLMISVCReconciler) reconcileMultiNodeMainWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	expected := r.expectedMainMultiNodeLWS(ctx, llmSvc)
	if llmSvc.Spec.Worker == nil {
		if err := Delete(ctx, r, llmSvc, expected); err != nil {
			return err
		}
		return nil
	}
	if err := Reconcile(ctx, r, llmSvc, &lwsapi.LeaderWorkerSet{}, expected, semanticLWSIsEqual); err != nil {
		return err
	}
	return r.propagateLeaderWorkerSetStatus(ctx, expected, llmSvc.MarkMainWorkloadReady, llmSvc.MarkMainWorkloadNotReady)
}

func (r *LLMISVCReconciler) reconcileMultiNodePrefillWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	expected := r.expectedPrefillMultiNodeLWS(ctx, llmSvc)
	if llmSvc.Spec.Prefill == nil || llmSvc.Spec.Prefill.Worker == nil {
		if err := Delete(ctx, r, llmSvc, expected); err != nil {
			return err
		}
		return nil
	}
	if err := Reconcile(ctx, r, llmSvc, &lwsapi.LeaderWorkerSet{}, expected, semanticLWSIsEqual); err != nil {
		return err
	}
	return r.propagateLeaderWorkerSetStatus(ctx, expected, llmSvc.MarkPrefillWorkloadReady, llmSvc.MarkPrefillWorkloadNotReady)
}

func (r *LLMISVCReconciler) propagateLeaderWorkerSetStatus(ctx context.Context, expected *lwsapi.LeaderWorkerSet, ready func(), notReady func(reason string, messageFormat string, messageA ...interface{})) error {
	logger := log.FromContext(ctx)

	curr := &lwsapi.LeaderWorkerSet{}
	err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return r.Client.Get(ctx, client.ObjectKeyFromObject(expected), curr)
	})
	if err != nil {
		return fmt.Errorf("failed to get current leaderworkerset %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}
	for _, cond := range curr.Status.Conditions {
		if cond.Type == string(lwsapi.LeaderWorkerSetAvailable) {
			if cond.Status == metav1.ConditionTrue {
				ready()
			} else {
				notReady(cond.Reason, cond.Message)
			}
			return nil
		}
	}
	logger.Info("LeaderWorkerSet processed")
	notReady(string(lwsapi.LeaderWorkerSetProgressing), "")
	return nil
}

func (r *LLMISVCReconciler) expectedMainMultiNodeLWS(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) *lwsapi.LeaderWorkerSet {
	workerLabels := map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-workload-worker",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
	}
	leaderLabels := map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-workload-leader",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
	}

	expected := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: workerLabels,
		},
		Spec: lwsapi.LeaderWorkerSetSpec{
			Replicas: llmSvc.Spec.Replicas,
			LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
				Size: llmSvc.Spec.Parallelism.GetSize(),
				WorkerTemplate: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: workerLabels,
					},
				},
				RestartPolicy: lwsapi.RecreateGroupOnPodRestart,
			},
			RolloutStrategy: lwsapi.RolloutStrategy{
				Type: lwsapi.RollingUpdateStrategyType,
			},
			StartupPolicy: lwsapi.LeaderReadyStartupPolicy,
		},
	}

	if llmSvc.Spec.Template != nil {
		expected.Spec.LeaderWorkerTemplate.LeaderTemplate = &corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: leaderLabels,
			},
			Spec: *llmSvc.Spec.Template.DeepCopy(),
		}
	}
	if llmSvc.Spec.Worker != nil {
		expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec = *llmSvc.Spec.Worker.DeepCopy()
	}

	log.FromContext(ctx).V(2).Info("Expected main LWS", "leaderworkerset", expected)

	return expected
}

func (r *LLMISVCReconciler) expectedPrefillMultiNodeLWS(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) *lwsapi.LeaderWorkerSet {
	workerLabels := map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-workload-worker-prefill",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
	}
	leaderLabels := map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-workload-leader-prefill",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
	}

	expected := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-prefill"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: workerLabels,
		},
		Spec: lwsapi.LeaderWorkerSetSpec{
			LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
				WorkerTemplate: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: workerLabels,
					},
				},
				RestartPolicy: lwsapi.RecreateGroupOnPodRestart,
			},
			RolloutStrategy: lwsapi.RolloutStrategy{
				Type: lwsapi.RollingUpdateStrategyType,
			},
			StartupPolicy: lwsapi.LeaderReadyStartupPolicy,
		},
	}

	if llmSvc.Spec.Prefill != nil {
		expected.Spec.Replicas = llmSvc.Spec.Prefill.Replicas
		expected.Spec.LeaderWorkerTemplate.Size = llmSvc.Spec.Prefill.Parallelism.GetSize()

		if llmSvc.Spec.Prefill.Template != nil {
			expected.Spec.LeaderWorkerTemplate.LeaderTemplate = &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: leaderLabels,
				},
				Spec: *llmSvc.Spec.Prefill.Template.DeepCopy(),
			}
		}
		if llmSvc.Spec.Prefill.Worker != nil {
			expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec = *llmSvc.Spec.Worker.DeepCopy()
		}
	}

	log.FromContext(ctx).V(2).Info("Expected main LWS", "leaderworkerset", expected)

	return expected
}

func semanticLWSIsEqual(expected *lwsapi.LeaderWorkerSet, curr *lwsapi.LeaderWorkerSet) bool {
	return equality.Semantic.DeepDerivative(expected.Spec, curr.Spec) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}
