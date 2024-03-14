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
	"strconv"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
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
	componentExt *v1beta1.ComponentExtensionSpec) *HPAReconciler {
	return &HPAReconciler{
		client:       client,
		scheme:       scheme,
		HPA:          createHPA(componentMeta, componentExt),
		componentExt: componentExt,
	}
}

func getHPAMetrics(metadata metav1.ObjectMeta, componentExt *v1beta1.ComponentExtensionSpec) []autoscalingv2.MetricSpec {
	var metrics []autoscalingv2.MetricSpec
	var utilization int32
	annotations := metadata.Annotations
	resourceName := corev1.ResourceCPU

	if value, ok := annotations[constants.TargetUtilizationPercentage]; ok {
		utilizationInt, _ := strconv.Atoi(value)
		utilization = int32(utilizationInt) // #nosec G109
	} else {
		utilization = constants.DefaultCPUUtilization
	}

	if componentExt.ScaleTarget != nil {
		utilization = int32(*componentExt.ScaleTarget)
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
	return metrics
}

func createHPA(componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec) *autoscalingv2.HorizontalPodAutoscaler {
	var minReplicas int32
	if componentExt.MinReplicas == nil || (*componentExt.MinReplicas) < constants.DefaultMinReplicas {
		minReplicas = int32(constants.DefaultMinReplicas)
	} else {
		minReplicas = int32(*componentExt.MinReplicas)
	}

	maxReplicas := int32(componentExt.MaxReplicas)
	if maxReplicas < minReplicas {
		maxReplicas = minReplicas
	}
	metrics := getHPAMetrics(componentMeta, componentExt)
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
	return hpa
}

// checkHPAExist checks if the hpa exists?
func (r *HPAReconciler) checkHPAExist(client client.Client) (constants.CheckResultType, *autoscalingv2.HorizontalPodAutoscaler, error) {
	// get hpa
	existingHPA := &autoscalingv2.HorizontalPodAutoscaler{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.HPA.ObjectMeta.Namespace,
		Name:      r.HPA.ObjectMeta.Name,
	}, existingHPA)
	if err != nil {
		if apierr.IsNotFound(err) {
			return constants.CheckResultCreate, nil, nil
		}
		return constants.CheckResultUnknown, nil, err
	}

	// existed, check equivalent
	if semanticHPAEquals(r.HPA, existingHPA) {
		return constants.CheckResultExisted, existingHPA, nil
	}
	return constants.CheckResultUpdate, existingHPA, nil
}

func semanticHPAEquals(desired, existing *autoscalingv2.HorizontalPodAutoscaler) bool {
	return equality.Semantic.DeepEqual(desired.Spec, existing.Spec)
}

// Reconcile ...
func (r *HPAReconciler) Reconcile() (*autoscalingv2.HorizontalPodAutoscaler, error) {
	// reconcile Service
	checkResult, existingHPA, err := r.checkHPAExist(r.client)
	log.Info("service reconcile", "checkResult", checkResult, "err", err)
	if err != nil {
		return nil, err
	}

	if checkResult == constants.CheckResultCreate {
		err = r.client.Create(context.TODO(), r.HPA)
		if err != nil {
			return nil, err
		} else {
			return r.HPA, nil
		}
	} else if checkResult == constants.CheckResultUpdate { // CheckResultUpdate
		err = r.client.Update(context.TODO(), r.HPA)
		if err != nil {
			return nil, err
		} else {
			return r.HPA, nil
		}
	} else {
		return existingHPA, nil
	}
}

func (r *HPAReconciler) SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error {
	return controllerutil.SetControllerReference(owner, r.HPA, scheme)
}
