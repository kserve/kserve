/* Copyright 2025 The KServe Authors. Licensed under the Apache License, Version 2.0. */
package llmisvc

import (
	"context"
	"fmt"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// reconcileScaling manages direct spec.scaling.keda. HPA cleanup is retained so
// resources created by older configurations are removed after an upgrade.
func (r *LLMISVCReconciler) reconcileScaling(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, _ *Config) error {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithName("reconcileScaling"))
	if err := r.reconcileWorkloadScaling(ctx, llmSvc, mainWorkloadScalingParams(llmSvc)); err != nil {
		return fmt.Errorf("failed to reconcile main workload scaling: %w", err)
	}
	if err := r.reconcileWorkloadScaling(ctx, llmSvc, prefillWorkloadScalingParams(llmSvc)); err != nil {
		return fmt.Errorf("failed to reconcile prefill workload scaling: %w", err)
	}
	return nil
}

type workloadScalingParams struct {
	name                      string
	scaling                   *v1alpha2.ScalingSpec
	scaleTargetRef            autoscalingv2.CrossVersionObjectReference
	hpaName, scaledObjectName string
	markReady                 func()
	markNotReady              func(string, string, ...interface{})
	markUnset                 func()
}

func mainWorkloadScalingParams(s *v1alpha2.LLMInferenceService) workloadScalingParams {
	return workloadScalingParams{name: "main", scaling: s.Spec.Scaling, scaleTargetRef: mainScaleTargetRef(s), hpaName: mainHPAName(s), scaledObjectName: mainScaledObjectName(s), markReady: s.MarkScalingReady, markNotReady: s.MarkScalingNotReady, markUnset: s.MarkScalingUnset}
}

func prefillWorkloadScalingParams(s *v1alpha2.LLMInferenceService) workloadScalingParams {
	var sc *v1alpha2.ScalingSpec
	if s.Spec.Prefill != nil {
		sc = s.Spec.Prefill.Scaling
	}
	return workloadScalingParams{name: "prefill", scaling: sc, scaleTargetRef: prefillScaleTargetRef(s), hpaName: prefillHPAName(s), scaledObjectName: prefillScaledObjectName(s), markReady: s.MarkPrefillScalingReady, markNotReady: s.MarkPrefillScalingNotReady, markUnset: s.MarkPrefillScalingUnset}
}

func (r *LLMISVCReconciler) reconcileWorkloadScaling(ctx context.Context, s *v1alpha2.LLMInferenceService, p workloadScalingParams) error {
	if err := r.reconcileKEDAScaledObject(ctx, s, p.scaling, utils.GetForceStopRuntime(s), p.scaleTargetRef, p.scaledObjectName); err != nil {
		return fmt.Errorf("failed to reconcile %s scaled object: %w", p.name, err)
	}
	if err := r.deleteHPAIfExists(ctx, s, p.hpaName); err != nil {
		return fmt.Errorf("failed to clean up %s HPA: %w", p.name, err)
	}
	return r.propagateScalingStatus(ctx, s, p.scaling, p.scaledObjectName, p.markReady, p.markNotReady, p.markUnset)
}

func (r *LLMISVCReconciler) deleteHPAIfExists(ctx context.Context, s *v1alpha2.LLMInferenceService, name string) error {
	return Delete(ctx, r, s, &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.GetNamespace()}})
}

func (r *LLMISVCReconciler) propagateScalingStatus(ctx context.Context, s *v1alpha2.LLMInferenceService, sc *v1alpha2.ScalingSpec, name string, ready func(), notReady func(string, string, ...interface{}), unset func()) error {
	// Keep the WVA API fields readable for upgrade compatibility, but do not
	// recreate or report an operational WVA autoscaler after deprecation.
	if sc != nil && sc.WVA != nil && !utils.GetForceStopRuntime(s) {
		notReady("WVAUnsupported", "WVA autoscaling is no longer supported; autoscaling is disabled. Remove spec.scaling.wva and configure direct KEDA scaling if needed")
		return nil
	}
	if sc == nil || utils.GetForceStopRuntime(s) || sc.KEDA == nil {
		unset()
		return nil
	}
	return r.propagateScaledObjectStatus(ctx, &kedav1alpha1.ScaledObject{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.GetNamespace()}}, ready, notReady)
}

func (r *LLMISVCReconciler) propagateScaledObjectStatus(ctx context.Context, expected *kedav1alpha1.ScaledObject, ready func(), notReady func(string, string, ...interface{})) error {
	curr := &kedav1alpha1.ScaledObject{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(expected), curr); err != nil {
		if apierrors.IsNotFound(err) {
			notReady("ScaledObjectProgressing", "ScaledObject not yet visible in cache")
			return nil
		}
		return fmt.Errorf("failed to get current ScaledObject %s/%s: %w", expected.Namespace, expected.Name, err)
	}
	if curr.Status.Conditions == nil || !curr.Status.Conditions.AreInitialized() {
		notReady("ScaledObjectProgressing", "ScaledObject conditions not yet available")
		return nil
	}
	c := curr.Status.Conditions.GetReadyCondition()
	if c.Status == metav1.ConditionFalse {
		notReady(c.Reason, c.Message)
		return nil
	}
	if c.Status != metav1.ConditionTrue {
		notReady("ScaledObjectProgressing", "ScaledObject is not yet ready")
		return nil
	}
	ready()
	return nil
}

func (r *LLMISVCReconciler) reconcileKEDAScaledObject(ctx context.Context, s *v1alpha2.LLMInferenceService, sc *v1alpha2.ScalingSpec, stopped bool, target autoscalingv2.CrossVersionObjectReference, name string) error {
	if sc == nil || stopped || sc.KEDA == nil {
		return r.deleteScaledObjectIfExists(ctx, s, name)
	}
	return Reconcile(ctx, r, s, &kedav1alpha1.ScaledObject{}, expectedDirectScaledObject(s, sc, target, name), semanticScaledObjectIsEqual, PreserveKEDAManagedMetadata())
}

func expectedDirectScaledObject(s *v1alpha2.LLMInferenceService, sc *v1alpha2.ScalingSpec, target autoscalingv2.CrossVersionObjectReference, name string) *kedav1alpha1.ScaledObject {
	k := sc.KEDA
	return &kedav1alpha1.ScaledObject{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.GetNamespace(), Labels: scalingLabels(s), OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(s, v1alpha2.LLMInferenceServiceGVK)}}, Spec: kedav1alpha1.ScaledObjectSpec{ScaleTargetRef: &kedav1alpha1.ScaleTarget{APIVersion: target.APIVersion, Kind: target.Kind, Name: target.Name}, MinReplicaCount: ptr.To(ptr.Deref(sc.MinReplicas, 1)), MaxReplicaCount: &sc.MaxReplicas, PollingInterval: k.PollingInterval, CooldownPeriod: k.CooldownPeriod, IdleReplicaCount: k.IdleReplicaCount, Fallback: k.Fallback, Advanced: k.Advanced, InitialCooldownPeriod: k.InitialCooldownPeriod, Triggers: k.Triggers}}
}

func (r *LLMISVCReconciler) deleteScaledObjectIfExists(ctx context.Context, s *v1alpha2.LLMInferenceService, name string) error {
	return Delete(ctx, r, s, &kedav1alpha1.ScaledObject{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.GetNamespace()}})
}

func semanticScaledObjectIsEqual(expected, curr *kedav1alpha1.ScaledObject) bool {
	return equality.Semantic.DeepEqual(expected.Spec, curr.Spec) && equality.Semantic.DeepEqual(expected.Labels, curr.Labels) && equality.Semantic.DeepEqual(expected.Annotations, curr.Annotations)
}

func PreserveKEDAManagedMetadata() UpdateOption[*kedav1alpha1.ScaledObject] {
	return AfterDryRun(func(expected, _ *kedav1alpha1.ScaledObject, curr *kedav1alpha1.ScaledObject) {
		if v, ok := curr.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation]; ok {
			if expected.Labels == nil {
				expected.Labels = map[string]string{}
			}
			expected.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation] = v
		}
		if len(curr.Finalizers) > 0 {
			expected.Finalizers = curr.Finalizers
		}
	})
}

func scalingLabels(s *v1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{constants.KubernetesComponentLabelKey: constants.LLMComponentWorkload, constants.KubernetesAppNameLabelKey: s.GetName(), constants.KubernetesPartOfLabelKey: constants.LLMInferenceServicePartOfValue}
}

func mainScaleTargetRef(s *v1alpha2.LLMInferenceService) autoscalingv2.CrossVersionObjectReference {
	if s.Spec.Worker != nil {
		return autoscalingv2.CrossVersionObjectReference{APIVersion: lwsapi.GroupVersion.String(), Kind: "LeaderWorkerSet", Name: mainLWSName(s)}
	}
	return autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: mainDeploymentName(s)}
}

func prefillScaleTargetRef(s *v1alpha2.LLMInferenceService) autoscalingv2.CrossVersionObjectReference {
	if s.Spec.Prefill != nil && s.Spec.Prefill.Worker != nil {
		return autoscalingv2.CrossVersionObjectReference{APIVersion: lwsapi.GroupVersion.String(), Kind: "LeaderWorkerSet", Name: prefillLWSName(s)}
	}
	return autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: prefillDeploymentName(s)}
}

func mainHPAName(s *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(s.GetName(), "-kserve-hpa")
}

func prefillHPAName(s *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(s.GetName(), "-kserve-prefill-hpa")
}

func mainScaledObjectName(s *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(s.GetName(), "-kserve-keda")
}

func prefillScaledObjectName(s *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(s.GetName(), "-kserve-prefill-keda")
}
