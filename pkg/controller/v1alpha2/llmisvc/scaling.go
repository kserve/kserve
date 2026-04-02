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
	wvav1alpha1 "github.com/llm-d/llm-d-workload-variant-autoscaler/api/v1alpha1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	wvaDesiredReplicasMetricName = "wva_desired_replicas"
	variantNameLabelKey          = "variant_name"
	acceleratorNameLabelKey      = "inference.optimization/acceleratorName"
)

// reconcileScaling manages the autoscaling resources (VariantAutoscaling + HPA or KEDA ScaledObject)
// for the LLM workload. When scaling is configured, it creates a VariantAutoscaling CR for WVA to
// compute desired replicas and an actuator (HPA or KEDA ScaledObject) to enforce them.
// When scaling is removed (or the workload is stopped), it cleans up any existing autoscaling resources.
//
// A missing scaling CRD (NoMatchError) is treated as a hard failure: the LLMInferenceService is
// misconfigured and reconciliation of remaining resources is blocked until the CRD is installed.
func (r *LLMISVCReconciler) reconcileScaling(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	logger := log.FromContext(ctx).WithName("reconcileScaling")
	ctx = log.IntoContext(ctx, logger)

	if err := r.reconcileMainWorkloadScaling(ctx, llmSvc, config); err != nil {
		return fmt.Errorf("failed to reconcile main workload scaling: %w", err)
	}

	if err := r.reconcilePrefillWorkloadScaling(ctx, llmSvc, config); err != nil {
		return fmt.Errorf("failed to reconcile prefill workload scaling: %w", err)
	}

	return nil
}

// reconcileMainWorkloadScaling handles scaling for the main (decode) workload.
func (r *LLMISVCReconciler) reconcileMainWorkloadScaling(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	deploymentName := mainDeploymentName(llmSvc)
	vaName := mainVAName(llmSvc)
	scaling := llmSvc.Spec.Scaling

	if err := r.reconcileVA(ctx, llmSvc, scaling, deploymentName, vaName, llmSvc.Spec.Labels); err != nil {
		return fmt.Errorf("failed to reconcile main VA: %w", err)
	}

	if err := r.reconcileActuator(ctx, llmSvc, scaling, config, deploymentName, vaName, mainHPAName(llmSvc), mainScaledObjectName(llmSvc)); err != nil {
		return fmt.Errorf("failed to reconcile main actuator: %w", err)
	}

	return nil
}

// reconcilePrefillWorkloadScaling handles scaling for the prefill workload in disaggregated deployments.
func (r *LLMISVCReconciler) reconcilePrefillWorkloadScaling(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	var scaling *v1alpha2.ScalingSpec
	if llmSvc.Spec.Prefill != nil {
		scaling = llmSvc.Spec.Prefill.Scaling
	}

	deploymentName := prefillDeploymentName(llmSvc)
	vaName := prefillVAName(llmSvc)

	var prefillLabels map[string]string
	if llmSvc.Spec.Prefill != nil {
		prefillLabels = llmSvc.Spec.Prefill.Labels
	}

	if err := r.reconcileVA(ctx, llmSvc, scaling, deploymentName, vaName, prefillLabels); err != nil {
		return fmt.Errorf("failed to reconcile prefill VA: %w", err)
	}

	if err := r.reconcileActuator(ctx, llmSvc, scaling, config, deploymentName, vaName, prefillHPAName(llmSvc), prefillScaledObjectName(llmSvc)); err != nil {
		return fmt.Errorf("failed to reconcile prefill actuator: %w", err)
	}

	return nil
}

// reconcileHPA creates or updates an HPA for the workload, or deletes it when not needed.
// The HPA reads wva_desired_replicas via the Kubernetes external metrics API, which requires
// a Prometheus Adapter to be pre-installed in the cluster. The controller cannot validate
// whether the Prometheus Adapter is present or correctly configured — if it is missing,
// the HPA will silently enter an Unknown state and stop scaling.
func (r *LLMISVCReconciler) reconcileHPA(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, isStopped bool, deploymentName, vaName, hpaName string) error {
	if scaling == nil || scaling.WVA == nil || isStopped || scaling.WVA.HPA == nil {
		return r.deleteHPAIfExists(ctx, llmSvc, hpaName)
	}

	expected := expectedHPA(llmSvc, scaling, deploymentName, vaName, hpaName)
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
// vaName is used as the metric selector label because WVA emits wva_desired_replicas keyed by VA name.
//
// The HPA uses an external metric (wva_desired_replicas) with target=1 so that it acts as a
// direct actuator for WVA's decisions rather than an independent scaling algorithm.
// WVA computes and publishes the desired replica count; HPA reads it and enforces it on the Deployment.
func expectedHPA(llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, deploymentName, vaName, hpaName string) *autoscalingv2.HorizontalPodAutoscaler {
	labels := scalingLabels(llmSvc)

	minReplicas := ptr.To(int32(1))
	if scaling.MinReplicas != nil {
		minReplicas = scaling.MinReplicas
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hpaName,
			Namespace: llmSvc.GetNamespace(),
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deploymentName,
			},
			MinReplicas: minReplicas,
			MaxReplicas: scaling.MaxReplicas,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ExternalMetricSourceType,
					External: &autoscalingv2.ExternalMetricSource{
						Metric: autoscalingv2.MetricIdentifier{
							Name: wvaDesiredReplicasMetricName,
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									variantNameLabelKey: vaName,
								},
							},
						},
						// Target is set to 1 with ValueMetricType so HPA computes:
						//   desired_replicas = ceil(wva_desired_replicas / 1) = wva_desired_replicas
						// This makes HPA a pass-through: it blindly follows the absolute replica count
						// emitted by WVA, rather than doing its own scaling arithmetic.
						// WVA is the sole source of scaling decisions; HPA is purely the enforcement layer.
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
func (r *LLMISVCReconciler) reconcileActuator(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, config *Config, deploymentName, vaName, hpaName, scaledObjectName string) error {
	isStopped := utils.GetForceStopRuntime(llmSvc)

	if err := r.reconcileKEDAScaledObject(ctx, llmSvc, scaling, isStopped, config, deploymentName, vaName, scaledObjectName); err != nil {
		return err
	}

	return r.reconcileHPA(ctx, llmSvc, scaling, isStopped, deploymentName, vaName, hpaName)
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
func (r *LLMISVCReconciler) reconcileKEDAScaledObject(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, isStopped bool, config *Config, deploymentName, vaName, scaledObjectName string) error {
	if scaling == nil || scaling.WVA == nil || isStopped || scaling.WVA.KEDA == nil {
		return r.deleteScaledObjectIfExists(ctx, llmSvc, scaledObjectName)
	}

	if err := validateAutoscalingConfig(config.WVAAutoscalingConfig); err != nil {
		return err
	}

	expected := expectedScaledObject(llmSvc, scaling, config, deploymentName, vaName, scaledObjectName)
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
// The Prometheus server address and TLS settings come from the controller config (inferenceservice-config ConfigMap).
func expectedScaledObject(llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, config *Config, deploymentName, vaName, scaledObjectName string) *kedav1alpha1.ScaledObject {
	labels := scalingLabels(llmSvc)
	keda := scaling.WVA.KEDA

	minReplicas := ptr.To(int32(1))
	if scaling.MinReplicas != nil {
		minReplicas = scaling.MinReplicas
	}

	// variant_name matches the VariantAutoscaling CR name, which WVA uses as a label when emitting wva_desired_replicas.
	// exported_namespace is used instead of namespace because Prometheus renames the namespace label emitted by WVA
	// to exported_namespace during scraping — this happens because namespace is a Prometheus-reserved label that
	// Prometheus itself sets to the scrape target's namespace (the WVA controller namespace). WVA's original
	// namespace label (the workload namespace) is therefore preserved under the exported_namespace name.
	// The VariantAutoscaling CR always lives in the same namespace as the LLMInferenceService, so llmSvc.GetNamespace()
	// is the correct value to filter on.
	query := fmt.Sprintf(`wva_desired_replicas{variant_name="%s",exported_namespace="%s"}`, vaName, llmSvc.GetNamespace())

	so := &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scaledObjectName,
			Namespace: llmSvc.GetNamespace(),
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			ScaleTargetRef: &kedav1alpha1.ScaleTarget{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deploymentName,
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

// reconcileVA creates, updates, or deletes a VariantAutoscaling CR based on the scaling configuration.
// The VA tells the WVA controller to compute wva_desired_replicas for this deployment.
// workloadLabels are the labels from the WorkloadSpec for the specific workload (decode or prefill).
func (r *LLMISVCReconciler) reconcileVA(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, deploymentName, vaName string, workloadLabels map[string]string) error {
	isStopped := utils.GetForceStopRuntime(llmSvc)

	if scaling == nil || scaling.WVA == nil || isStopped {
		return r.deleteVAIfExists(ctx, llmSvc, vaName)
	}

	expected := expectedVA(llmSvc, scaling, deploymentName, vaName, workloadLabels)
	return Reconcile(ctx, r, llmSvc, &wvav1alpha1.VariantAutoscaling{}, expected, semanticVAIsEqual)
}

// deleteVAIfExists deletes the VariantAutoscaling if it exists.
func (r *LLMISVCReconciler) deleteVAIfExists(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, vaName string) error {
	va := &wvav1alpha1.VariantAutoscaling{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vaName,
			Namespace: llmSvc.GetNamespace(),
		},
	}
	return Delete(ctx, r, llmSvc, va)
}

// expectedVA constructs the desired VariantAutoscaling resource from the LLMISVC spec.
// workloadLabels are the labels from the WorkloadSpec for the specific workload (decode or prefill).
// If the workload labels contain inference.optimization/acceleratorName, that value is propagated
// to the VA label. Otherwise the label is set to "unknown".
func expectedVA(llmSvc *v1alpha2.LLMInferenceService, scaling *v1alpha2.ScalingSpec, deploymentName, vaName string, workloadLabels map[string]string) *wvav1alpha1.VariantAutoscaling {
	labels := scalingLabels(llmSvc)
	accelerator := "unknown"
	if val, ok := workloadLabels[acceleratorNameLabelKey]; ok && val != "" {
		accelerator = val
	}
	labels[acceleratorNameLabelKey] = accelerator

	modelID := llmSvc.Spec.Model.URI.String()
	if llmSvc.Spec.Model.Name != nil {
		modelID = *llmSvc.Spec.Model.Name
	}

	va := &wvav1alpha1.VariantAutoscaling{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vaName,
			Namespace: llmSvc.GetNamespace(),
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: wvav1alpha1.VariantAutoscalingSpec{
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deploymentName,
			},
			ModelID:     modelID,
			VariantCost: scaling.WVA.VariantCost,
		},
	}

	return va
}

func semanticVAIsEqual(expected, curr *wvav1alpha1.VariantAutoscaling) bool {
	return equality.Semantic.DeepEqual(expected.Spec, curr.Spec) &&
		equality.Semantic.DeepEqual(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepEqual(expected.Annotations, curr.Annotations)
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

func mainHPAName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-hpa")
}

func prefillHPAName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-prefill-hpa")
}

func mainVAName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-va")
}

func prefillVAName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-prefill-va")
}

func mainScaledObjectName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-keda")
}

func prefillScaledObjectName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-prefill-keda")
}
