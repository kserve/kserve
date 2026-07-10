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
	"fmt"
	"strconv"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	wvaDesiredReplicasMetricName = "wva_desired_replicas"
	variantNameLabelKey          = "variant_name"
	acceleratorNameLabelKey      = "inference.optimization/acceleratorName"

	// WVA annotation-based discovery annotations (matching WVA internal/annotations/annotations.go).
	// WVA discovers HPAs/ScaledObjects bearing these annotations and synthesizes in-memory
	// VariantAutoscaling objects from them, eliminating the need for a separate VA CRD.
	wvaManagedAnnotation     = "llm-d.ai/managed"
	wvaModelIDAnnotation     = "llm-d.ai/model-id"
	wvaVariantCostAnnotation = "llm-d.ai/variant-cost"
)

// reconcileScaling manages the autoscaling actuators (HPA or KEDA ScaledObject) for the LLM workload.
// Each actuator carries WVA discovery annotations (llm-d.ai/managed, llm-d.ai/model-id) so that
// WVA discovers it, synthesizes an in-memory VariantAutoscaling, and emits wva_desired_replicas.
// When scaling is removed (or the workload is stopped), it cleans up any existing autoscaling resources.
func (r *LLMISVCReconciler) reconcileScaling(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	logger := log.FromContext(ctx).WithName("reconcileScaling")
	ctx = log.IntoContext(ctx, logger)

	if err := r.reconcileWorkloadScaling(ctx, llmSvc, config, mainWorkloadScalingParams(llmSvc)); err != nil {
		return fmt.Errorf("failed to reconcile main workload scaling: %w", err)
	}

	if err := r.reconcileWorkloadScaling(ctx, llmSvc, config, prefillWorkloadScalingParams(llmSvc)); err != nil {
		return fmt.Errorf("failed to reconcile prefill workload scaling: %w", err)
	}

	return nil
}

// workloadScalingParams captures the per-workload differences between main and prefill
// scaling so that reconcileWorkloadScaling can handle both without duplication.
type workloadScalingParams struct {
	name             string
	scaling          *v1alpha2.ScalingSpec
	scaleTargetRef   autoscalingv2.CrossVersionObjectReference
	legacyVAName     string // used only for cleaning up deprecated VA CRDs during migration
	hpaName          string
	scaledObjectName string
	workloadLabels   map[string]string
	markReady        func()
	markNotReady     func(reason, messageFormat string, messageA ...interface{})
	markUnset        func()
}

func mainWorkloadScalingParams(llmSvc *v1alpha2.LLMInferenceService) workloadScalingParams {
	return workloadScalingParams{
		name:             "main",
		scaling:          llmSvc.Spec.Scaling,
		scaleTargetRef:   mainScaleTargetRef(llmSvc),
		legacyVAName:     legacyMainVAName(llmSvc),
		hpaName:          mainHPAName(llmSvc),
		scaledObjectName: mainScaledObjectName(llmSvc),
		workloadLabels:   llmSvc.Spec.Labels,
		markReady:        llmSvc.MarkScalingReady,
		markNotReady:     llmSvc.MarkScalingNotReady,
		markUnset:        llmSvc.MarkScalingUnset,
	}
}

func prefillWorkloadScalingParams(llmSvc *v1alpha2.LLMInferenceService) workloadScalingParams {
	var scaling *v1alpha2.ScalingSpec
	var labels map[string]string
	if llmSvc.Spec.Prefill != nil {
		scaling = llmSvc.Spec.Prefill.Scaling
		labels = llmSvc.Spec.Prefill.Labels
	}
	return workloadScalingParams{
		name:             "prefill",
		scaling:          scaling,
		scaleTargetRef:   prefillScaleTargetRef(llmSvc),
		legacyVAName:     legacyPrefillVAName(llmSvc),
		hpaName:          prefillHPAName(llmSvc),
		scaledObjectName: prefillScaledObjectName(llmSvc),
		workloadLabels:   labels,
		markReady:        llmSvc.MarkPrefillScalingReady,
		markNotReady:     llmSvc.MarkPrefillScalingNotReady,
		markUnset:        llmSvc.MarkPrefillScalingUnset,
	}
}

// reconcileWorkloadScaling reconciles the scaling actuator and propagates status for a single
// workload (main or prefill). The actuator (HPA or ScaledObject) carries WVA discovery
// annotations so WVA can synthesize an in-memory VA without a separate CRD.
func (r *LLMISVCReconciler) reconcileWorkloadScaling(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, p workloadScalingParams) error {
	if err := r.cleanupLegacyVA(ctx, llmSvc.GetNamespace(), p.legacyVAName); err != nil {
		return fmt.Errorf("failed to cleanup legacy %s VA: %w", p.name, err)
	}

	if err := r.reconcileActuator(ctx, llmSvc, p.scaling, config, p.scaleTargetRef, p.hpaName, p.scaledObjectName, p.workloadLabels); err != nil {
		return fmt.Errorf("failed to reconcile %s actuator: %w", p.name, err)
	}

	return r.propagateScalingStatus(ctx, llmSvc, p.scaling, p.hpaName, p.scaledObjectName, p.markReady, p.markNotReady, p.markUnset)
}

// reconcileHPA creates or updates an HPA for the workload, or deletes it when not needed.
// The HPA carries WVA discovery annotations and reads wva_desired_replicas via the Kubernetes
// external metrics API, which requires a Prometheus Adapter to be pre-installed in the cluster.
func (r *LLMISVCReconciler) reconcileHPA(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, isStopped bool, scaleTargetRef autoscalingv2.CrossVersionObjectReference, hpaName string, workloadLabels map[string]string) error {
	if scaling == nil || scaling.WVA == nil || isStopped || scaling.WVA.HPA == nil {
		return r.deleteHPAIfExists(ctx, llmSvc, hpaName)
	}

	expected := expectedHPA(llmSvc, scaling, scaleTargetRef, hpaName, workloadLabels)
	return Reconcile(ctx, r, llmSvc, &autoscalingv2.HorizontalPodAutoscaler{}, expected, semanticHPAIsEqual)
}

// deleteHPAIfExists deletes the HPA if it exists.
func (r *LLMISVCReconciler) deleteHPAIfExists(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, hpaName string) error {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hpaName,
			Namespace: llmSvc.GetNamespace(),
		},
	}
	return Delete(ctx, r, llmSvc, hpa)
}

// expectedHPA constructs the desired HPA resource from the LLMISVC scaling spec.
// The HPA carries WVA discovery annotations so WVA synthesizes an in-memory VA from it.
// The metric selector uses hpaName as variant_name because WVA uses the HPA's own name
// as the synthetic VA name when emitting wva_desired_replicas.
//
// The HPA uses an external metric (wva_desired_replicas) with target=1 so that it acts as a
// direct actuator for WVA's decisions rather than an independent scaling algorithm.
func expectedHPA(llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, scaleTargetRef autoscalingv2.CrossVersionObjectReference, hpaName string, workloadLabels map[string]string) *autoscalingv2.HorizontalPodAutoscaler {
	labels := wvaLabels(llmSvc, workloadLabels)
	annotations := wvaAnnotations(llmSvc, scaling)

	minReplicas := ptr.To(ptr.Deref(scaling.MinReplicas, 1))

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:        hpaName,
			Namespace:   llmSvc.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: scaleTargetRef,
			MinReplicas:    minReplicas,
			MaxReplicas:    scaling.MaxReplicas,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ExternalMetricSourceType,
					External: &autoscalingv2.ExternalMetricSource{
						Metric: autoscalingv2.MetricIdentifier{
							Name: wvaDesiredReplicasMetricName,
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									variantNameLabelKey: hpaName,
								},
							},
						},
						Target: autoscalingv2.MetricTarget{
							Type:  autoscalingv2.ValueMetricType,
							Value: resource.NewQuantity(1, resource.DecimalSI),
						},
					},
				},
			},
		},
	}

	if scaling.WVA != nil && scaling.WVA.HPA != nil && scaling.WVA.HPA.Behavior != nil {
		hpa.Spec.Behavior = scaling.WVA.HPA.Behavior
	}

	return hpa
}

// reconcileActuator reconciles the scaling actuators (HPA, KEDA ScaledObject) for the workload.
func (r *LLMISVCReconciler) reconcileActuator(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, config *Config, scaleTargetRef autoscalingv2.CrossVersionObjectReference, hpaName, scaledObjectName string, workloadLabels map[string]string) error {
	isStopped := utils.GetForceStopRuntime(llmSvc)

	if err := r.reconcileKEDAScaledObject(ctx, llmSvc, scaling, isStopped, config, scaleTargetRef, scaledObjectName, workloadLabels); err != nil {
		return err
	}

	return r.reconcileHPA(ctx, llmSvc, scaling, isStopped, scaleTargetRef, hpaName, workloadLabels)
}

// propagateScalingStatus determines which actuator is active and propagates its status.
// When no scaling is configured (or the service is stopped), the condition is cleared.
func (r *LLMISVCReconciler) propagateScalingStatus(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, hpaName, scaledObjectName string, ready func(), notReady func(reason, messageFormat string, messageA ...interface{}), unset func()) error {
	isStopped := utils.GetForceStopRuntime(llmSvc)

	if scaling == nil || scaling.WVA == nil || isStopped {
		unset()
		return nil
	}

	if scaling.WVA.HPA != nil {
		expected := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hpaName,
				Namespace: llmSvc.GetNamespace(),
			},
		}
		return r.propagateHPAStatus(ctx, expected, ready, notReady)
	}

	if scaling.WVA.KEDA != nil {
		expected := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scaledObjectName,
				Namespace: llmSvc.GetNamespace(),
			},
		}
		return r.propagateScaledObjectStatus(ctx, expected, ready, notReady)
	}

	unset()
	return nil
}

// propagateHPAStatus reads the live HPA status and maps its conditions to a ScalingReady
// condition on the LLMInferenceService. AbleToScale=False or ScalingActive=False means the
// metrics pipeline is broken and sets ScalingReady=False.
func (r *LLMISVCReconciler) propagateHPAStatus(ctx context.Context, expected *autoscalingv2.HorizontalPodAutoscaler, ready func(), notReady func(reason, messageFormat string, messageA ...interface{})) error {
	curr := &autoscalingv2.HorizontalPodAutoscaler{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(expected), curr); err != nil {
		if apierrors.IsNotFound(err) {
			notReady("HPAProgressing", "HPA not yet visible in cache")
			return nil
		}
		return fmt.Errorf("failed to get current HPA %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}

	foundAny := false

	for _, cond := range curr.Status.Conditions {
		switch cond.Type {
		case autoscalingv2.AbleToScale, autoscalingv2.ScalingActive:
			foundAny = true
			if cond.Status == corev1.ConditionFalse {
				notReady(cond.Reason, cond.Message)
				return nil
			}
		}
	}

	if !foundAny {
		notReady("HPAProgressing", "HPA conditions not yet available")
		return nil
	}

	ready()
	return nil
}

// propagateScaledObjectStatus reads the live KEDA ScaledObject status and maps its conditions
// to a ScalingReady condition on the LLMInferenceService. Ready=False means a trigger/config
// issue and sets ScalingReady=False.
func (r *LLMISVCReconciler) propagateScaledObjectStatus(ctx context.Context, expected *kedav1alpha1.ScaledObject, ready func(), notReady func(reason, messageFormat string, messageA ...interface{})) error {
	curr := &kedav1alpha1.ScaledObject{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(expected), curr); err != nil {
		if apierrors.IsNotFound(err) {
			notReady("ScaledObjectProgressing", "ScaledObject not yet visible in cache")
			return nil
		}
		return fmt.Errorf("failed to get current ScaledObject %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}

	conditions := curr.Status.Conditions
	if conditions == nil || !conditions.AreInitialized() {
		notReady("ScaledObjectProgressing", "ScaledObject conditions not yet available")
		return nil
	}

	readyCond := conditions.GetReadyCondition()
	if readyCond.Status == metav1.ConditionFalse {
		notReady(readyCond.Reason, readyCond.Message)
		return nil
	}

	if readyCond.Status != metav1.ConditionTrue {
		notReady("ScaledObjectProgressing", "ScaledObject is not yet ready")
		return nil
	}

	ready()
	return nil
}

// validateAutoscalingConfig checks that the WVAAutoscalingConfig is valid for use with KEDA.
// It returns an error if the config is nil, if prometheus.url is missing, or if the auth
// fields are only partially configured (both prometheus.authModes and prometheus.triggerAuthName
// must be set together or both left empty).
func validateAutoscalingConfig(cfg *WVAAutoscalingConfig) error {
	if cfg == nil || cfg.Prometheus.URL == "" {
		return fmt.Errorf("%s.prometheus.url is required in inferenceservice-config when using KEDA", autoscalingConfigName)
	}
	if (cfg.Prometheus.TriggerAuthName == "") != (cfg.Prometheus.AuthModes == "") {
		return fmt.Errorf("%s.prometheus.authModes and %s.prometheus.triggerAuthName must both be set or both be empty", autoscalingConfigName, autoscalingConfigName)
	}
	return nil
}

// reconcileKEDAScaledObject creates or updates a KEDA ScaledObject, or deletes it when not needed.
// The ScaledObject carries WVA discovery annotations for annotation-based VA synthesis.
func (r *LLMISVCReconciler) reconcileKEDAScaledObject(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, isStopped bool, config *Config, scaleTargetRef autoscalingv2.CrossVersionObjectReference, scaledObjectName string, workloadLabels map[string]string) error {
	if scaling == nil || scaling.WVA == nil || isStopped || scaling.WVA.KEDA == nil {
		return r.deleteScaledObjectIfExists(ctx, llmSvc, scaledObjectName)
	}

	if err := validateAutoscalingConfig(config.WVAAutoscalingConfig); err != nil {
		return err
	}

	expected := expectedScaledObject(llmSvc, scaling, config, scaleTargetRef, scaledObjectName, workloadLabels)
	return Reconcile(ctx, r, llmSvc, &kedav1alpha1.ScaledObject{}, expected, semanticScaledObjectIsEqual,
		PreserveKEDAManagedMetadata(),
	)
}

// deleteScaledObjectIfExists deletes the ScaledObject if it exists.
func (r *LLMISVCReconciler) deleteScaledObjectIfExists(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, scaledObjectName string) error {
	so := &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scaledObjectName,
			Namespace: llmSvc.GetNamespace(),
		},
	}
	return Delete(ctx, r, llmSvc, so)
}

// expectedScaledObject constructs the desired KEDA ScaledObject from the LLMISVC scaling spec.
// The ScaledObject carries WVA discovery annotations for annotation-based VA synthesis.
// The Prometheus query uses scaledObjectName as variant_name because WVA uses the ScaledObject's
// own name as the synthetic VA name when emitting wva_desired_replicas.
func expectedScaledObject(llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, config *Config, scaleTargetRef autoscalingv2.CrossVersionObjectReference, scaledObjectName string, workloadLabels map[string]string) *kedav1alpha1.ScaledObject {
	labels := wvaLabels(llmSvc, workloadLabels)
	annotations := wvaAnnotations(llmSvc, scaling)

	keda := scaling.WVA.KEDA
	minReplicas := ptr.To(ptr.Deref(scaling.MinReplicas, 1))

	// exported_namespace is used instead of namespace because Prometheus renames the namespace
	// label emitted by WVA to exported_namespace during scraping.
	query := fmt.Sprintf(`wva_desired_replicas{variant_name="%s",exported_namespace="%s"}`, scaledObjectName, llmSvc.GetNamespace())

	so := &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:        scaledObjectName,
			Namespace:   llmSvc.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			ScaleTargetRef: &kedav1alpha1.ScaleTarget{
				APIVersion: scaleTargetRef.APIVersion,
				Kind:       scaleTargetRef.Kind,
				Name:       scaleTargetRef.Name,
			},
			MinReplicaCount:       minReplicas,
			MaxReplicaCount:       &scaling.MaxReplicas,
			PollingInterval:       keda.PollingInterval,
			CooldownPeriod:        keda.CooldownPeriod,
			IdleReplicaCount:      keda.IdleReplicaCount,
			Fallback:              keda.Fallback,
			Advanced:              keda.Advanced,
			InitialCooldownPeriod: keda.InitialCooldownPeriod,
			Triggers: []kedav1alpha1.ScaleTriggers{
				prometheusTrigger(config.WVAAutoscalingConfig, query),
			},
		},
	}

	return so
}

// prometheusTrigger builds the KEDA ScaleTriggers entry for the Prometheus scaler.
// Authentication is optional: when Prometheus.AuthModes and Prometheus.TriggerAuthName are
// set in the config, the trigger will carry authModes metadata and an AuthenticationRef
// pointing to the pre-existing TriggerAuthentication or ClusterTriggerAuthentication CR.
func prometheusTrigger(cfg *WVAAutoscalingConfig, query string) kedav1alpha1.ScaleTriggers {
	prom := &cfg.Prometheus
	trigger := kedav1alpha1.ScaleTriggers{
		Type: "prometheus",
		Name: "wva-desired-replicas",
		Metadata: map[string]string{
			"serverAddress": prom.URL,
			"query":         query,
			"threshold":     "1",
			"unsafeSsl":     strconv.FormatBool(prom.TLSInsecureSkipVerify),
		},
	}

	if prom.AuthModes != "" {
		trigger.Metadata["authModes"] = prom.AuthModes
	}

	if prom.TriggerAuthName != "" {
		trigger.AuthenticationRef = &kedav1alpha1.AuthenticationRef{
			Name: prom.TriggerAuthName,
			Kind: prom.TriggerAuthKind,
		}
	}

	return trigger
}

// cleanupLegacyVA deletes a deprecated VariantAutoscaling CR if it exists.
// Uses unstructured delete to avoid importing WVA API types.
// Caches the CRD-absent state so that clusters where the CRD was never
// installed skip the API call after the first reconcile.
// This is a temporary migration step; remove after one release cycle.
func (r *LLMISVCReconciler) cleanupLegacyVA(ctx context.Context, namespace, vaName string) error {
	if r.legacyVACRDAbsent.Load() {
		return nil
	}
	logger := log.FromContext(ctx).WithName("cleanupLegacyVA")
	va := &unstructured.Unstructured{}
	va.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "llmd.ai",
		Version: "v1alpha1",
		Kind:    "VariantAutoscaling",
	})
	va.SetName(vaName)
	va.SetNamespace(namespace)
	err := r.Delete(ctx, va)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if apimeta.IsNoMatchError(err) {
		r.legacyVACRDAbsent.Store(true)
		return nil
	}
	if err == nil {
		logger.Info("Deleted legacy VariantAutoscaling CR; WVA >= v0.8.0 is required for annotation-based discovery",
			"namespace", namespace, "name", vaName)
	}
	return err
}

func semanticHPAIsEqual(expected, curr *autoscalingv2.HorizontalPodAutoscaler) bool {
	return equality.Semantic.DeepEqual(expected.Spec, curr.Spec) &&
		equality.Semantic.DeepEqual(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepEqual(expected.Annotations, curr.Annotations)
}

func semanticScaledObjectIsEqual(expected, curr *kedav1alpha1.ScaledObject) bool {
	return equality.Semantic.DeepEqual(expected.Spec, curr.Spec) &&
		equality.Semantic.DeepEqual(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepEqual(expected.Annotations, curr.Annotations)
}

// PreserveKEDAManagedMetadata returns an AfterDryRun hook that copies KEDA-managed
// metadata from the live object into expected before the real update is issued.
//
// It preserves two things:
//  1. The KEDA-injected label (scaledobject.keda.sh/name) — so that DeepEqual does
//     not trigger spurious updates when KEDA adds this label after creation.
//  2. Finalizers — because the Update call is a full PUT (not a patch), which would
//     otherwise wipe any finalizers that KEDA added (e.g. finalizer.keda.sh) whenever
//     a spec/label change triggers an update. Copying finalizers from curr ensures they
//     are preserved in the PUT body.
func PreserveKEDAManagedMetadata() UpdateOption[*kedav1alpha1.ScaledObject] {
	return AfterDryRun(func(expected, _ /* expectedGiven */, curr *kedav1alpha1.ScaledObject) {
		if v, ok := curr.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation]; ok {
			if expected.Labels == nil {
				expected.Labels = make(map[string]string)
			}
			expected.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation] = v
		}
		if len(curr.Finalizers) > 0 {
			expected.Finalizers = curr.Finalizers
		}
	})
}

func scalingLabels(llmSvc *v1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentWorkload,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
	}
}

// wvaLabels builds the label set for an HPA or ScaledObject, including the
// standard scaling labels and the accelerator name from workload labels.
func wvaLabels(llmSvc *v1alpha2.LLMInferenceService, workloadLabels map[string]string) map[string]string {
	labels := scalingLabels(llmSvc)
	accelerator := "unknown"
	if val, ok := workloadLabels[acceleratorNameLabelKey]; ok && val != "" {
		accelerator = val
	}
	labels[acceleratorNameLabelKey] = accelerator
	return labels
}

// wvaAnnotations builds the WVA discovery annotations for an HPA or ScaledObject.
func wvaAnnotations(llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec) map[string]string {
	modelID := llmSvc.Spec.Model.URI.String()
	if llmSvc.Spec.Model.Name != nil {
		modelID = *llmSvc.Spec.Model.Name
	}
	annotations := map[string]string{
		wvaManagedAnnotation: "true",
		wvaModelIDAnnotation: modelID,
	}
	if scaling.WVA.VariantCost != "" {
		annotations[wvaVariantCostAnnotation] = scaling.WVA.VariantCost
	}
	return annotations
}

func mainScaleTargetRef(llmSvc *v1alpha2.LLMInferenceService) autoscalingv2.CrossVersionObjectReference {
	if llmSvc.Spec.Worker != nil {
		return autoscalingv2.CrossVersionObjectReference{
			APIVersion: lwsapi.GroupVersion.String(),
			Kind:       "LeaderWorkerSet",
			Name:       mainLWSName(llmSvc),
		}
	}
	return autoscalingv2.CrossVersionObjectReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       mainDeploymentName(llmSvc),
	}
}

func prefillScaleTargetRef(llmSvc *v1alpha2.LLMInferenceService) autoscalingv2.CrossVersionObjectReference {
	if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.Worker != nil {
		return autoscalingv2.CrossVersionObjectReference{
			APIVersion: lwsapi.GroupVersion.String(),
			Kind:       "LeaderWorkerSet",
			Name:       prefillLWSName(llmSvc),
		}
	}
	return autoscalingv2.CrossVersionObjectReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       prefillDeploymentName(llmSvc),
	}
}

func mainHPAName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-hpa")
}

func prefillHPAName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-prefill-hpa")
}

// legacyMainVAName and legacyPrefillVAName produce the names of deprecated
// VariantAutoscaling CRs that may still exist from before the annotation-based migration.
// Used only by cleanupLegacyVA; remove after one release cycle.
func legacyMainVAName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-va")
}

func legacyPrefillVAName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-prefill-va")
}

func mainScaledObjectName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-keda")
}

func prefillScaledObjectName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-prefill-keda")
}
