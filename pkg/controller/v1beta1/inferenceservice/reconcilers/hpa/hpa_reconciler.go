/*
Copyright 2021 The KServe Authors.
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

package hpa

import (
	"context"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

var log = logf.Log.WithName("HPAReconciler")

// HPAReconciler is the struct of Raw K8S Object
type HPAReconciler struct {
	client       client.Client
	scheme       *runtime.Scheme
	HPA          *autoscalingv2.HorizontalPodAutoscaler
	componentExt *v1beta1.ComponentExtensionSpec
}

func NewHPAReconciler(client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
) (*HPAReconciler, error) {
	hpa, err := createHPA(componentMeta, componentExt)
	if err != nil {
		return nil, err
	}
	return &HPAReconciler{
		client:       client,
		scheme:       scheme,
		HPA:          hpa,
		componentExt: componentExt,
	}, nil
}

func getHPAMetrics(metadata metav1.ObjectMeta, componentExt *v1beta1.ComponentExtensionSpec) ([]autoscalingv2.MetricSpec, error) {
	var metrics []autoscalingv2.MetricSpec
	var utilization int32
	var err error
	annotations := metadata.Annotations
	resourceName := corev1.ResourceCPU

	if value, ok := annotations[constants.TargetUtilizationPercentage]; ok {
		utilization, err = utils.StringToInt32(value)
		if err != nil {
			return metrics, err
		}
	} else {
		utilization = constants.DefaultCPUUtilization
	}

	if componentExt.ScaleTarget != nil {
		utilization = *componentExt.ScaleTarget
	}

	if componentExt.ScaleMetric != nil {
		resourceName = corev1.ResourceName(*componentExt.ScaleMetric)
	}

	metricTarget := autoscalingv2.MetricTarget{
		Type:               "Utilization",
		AverageUtilization: &utilization,
	}

	ms := autoscalingv2.MetricSpec{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name:   resourceName,
			Target: metricTarget,
		},
	}
	metrics = append(metrics, ms)
	return metrics, nil
}

func createHPA(componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	var minReplicas int32
	if componentExt.MinReplicas == nil || (*componentExt.MinReplicas) < constants.DefaultMinReplicas {
		minReplicas = constants.DefaultMinReplicas
	} else {
		minReplicas = *componentExt.MinReplicas
	}

	maxReplicas := componentExt.MaxReplicas
	if maxReplicas < minReplicas {
		maxReplicas = minReplicas
	}
	metrics, err := getHPAMetrics(componentMeta, componentExt)
	if err != nil {
		return nil, err
	}
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: componentMeta,
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       componentMeta.Name,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxReplicas,
			Metrics:     metrics,
			Behavior:    &autoscalingv2.HorizontalPodAutoscalerBehavior{},
		},
	}
	return hpa, nil
}

// checkHPAExist checks if the hpa exists?
func (r *HPAReconciler) checkHPAExist(ctx context.Context, client client.Client) (constants.CheckResultType, *autoscalingv2.HorizontalPodAutoscaler, error) {
	// get hpa
	existingHPA := &autoscalingv2.HorizontalPodAutoscaler{}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: r.HPA.ObjectMeta.Namespace,
		Name:      r.HPA.ObjectMeta.Name,
	}, existingHPA)
	if err != nil {
		if apierr.IsNotFound(err) {
			if shouldCreateHPA(r.HPA) {
				return constants.CheckResultCreate, nil, nil
			} else {
				return constants.CheckResultSkipped, nil, nil
			}
		}
		return constants.CheckResultUnknown, nil, err
	}

	// existed, check equivalent
	if semanticHPAEquals(r.HPA, existingHPA) {
		return constants.CheckResultExisted, existingHPA, nil
	}
	if shouldDeleteHPA(r.HPA) {
		return constants.CheckResultDelete, existingHPA, nil
	}
	return constants.CheckResultUpdate, existingHPA, nil
}

func semanticHPAEquals(desired, existing *autoscalingv2.HorizontalPodAutoscaler) bool {
	desiredAutoscalerClass, hasDesiredAutoscalerClass := desired.Annotations[constants.AutoscalerClass]
	existingAutoscalerClass, hasExistingAutoscalerClass := existing.Annotations[constants.AutoscalerClass]
	var autoscalerClassChanged bool
	if hasDesiredAutoscalerClass && hasExistingAutoscalerClass {
		autoscalerClassChanged = desiredAutoscalerClass != existingAutoscalerClass
	} else if hasDesiredAutoscalerClass || hasExistingAutoscalerClass {
		autoscalerClassChanged = true
	}
	return equality.Semantic.DeepEqual(desired.Spec, existing.Spec) && !autoscalerClassChanged
}

func shouldDeleteHPA(desired *autoscalingv2.HorizontalPodAutoscaler) bool {
	desiredAutoscalerClass, hasDesiredAutoscalerClass := desired.Annotations[constants.AutoscalerClass]
	return hasDesiredAutoscalerClass && constants.AutoscalerClassType(desiredAutoscalerClass) == constants.AutoscalerClassExternal
}

func shouldCreateHPA(desired *autoscalingv2.HorizontalPodAutoscaler) bool {
	desiredAutoscalerClass, hasDesiredAutoscalerClass := desired.Annotations[constants.AutoscalerClass]
	return !hasDesiredAutoscalerClass || (hasDesiredAutoscalerClass && constants.AutoscalerClassType(desiredAutoscalerClass) == constants.AutoscalerClassHPA)
}

// Reconcile ...
func (r *HPAReconciler) Reconcile(ctx context.Context) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	// reconcile HorizontalPodAutoscaler
	checkResult, existingHPA, err := r.checkHPAExist(ctx, r.client)
	log.Info("HorizontalPodAutoscaler reconcile", "checkResult", checkResult, "err", err)
	if err != nil {
		return err
	}

	var opErr error
	switch checkResult {
	case constants.CheckResultCreate:
		opErr = r.client.Create(ctx, r.HPA)
	case constants.CheckResultUpdate:
		opErr = r.client.Update(ctx, r.HPA)
	case constants.CheckResultDelete:
		opErr = r.client.Delete(ctx, r.HPA)
	default:
		return nil
	}

	if opErr != nil {
		return opErr
	}

	return nil
}

func (r *HPAReconciler) SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error {
	return controllerutil.SetControllerReference(owner, r.HPA, scheme)
}
