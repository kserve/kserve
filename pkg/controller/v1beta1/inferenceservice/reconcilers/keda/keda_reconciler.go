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

package keda

import (
	"context"
	"strconv"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/pkg/errors"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

var log = logf.Log.WithName("KedaReconciler")

type KedaReconciler struct {
	client       client.Client
	scheme       *runtime.Scheme
	ScaledObject *kedav1alpha1.ScaledObject
	componentExt *v1beta1.ComponentExtensionSpec
}

func NewKedaReconciler(client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
) (*KedaReconciler, error) {
	return &KedaReconciler{
		client:       client,
		scheme:       scheme,
		ScaledObject: createKedaScaledObject(componentMeta, componentExt),
		componentExt: componentExt,
	}, nil
}

func getKedaMetrics(componentExt *v1beta1.ComponentExtensionSpec,
) []kedav1alpha1.ScaleTriggers {
	var triggers []kedav1alpha1.ScaleTriggers

	metricType := autoscalingv2.UtilizationMetricType
	targetValue := int(constants.DefaultCPUUtilization)

	// metric configuration from componentExtension.AutoScaling if it is set
	if componentExt.AutoScaling != nil {
		for _, autoScaling := range componentExt.AutoScaling {
			if autoScaling.Type == v1beta1.MetricSourceType(constants.AutoScalerResource) {
				triggerType := string(*autoScaling.Resource.Name)
				metricType = autoscalingv2.MetricTargetType(autoScaling.Resource.Target.Type)
				if metricType == autoscalingv2.UtilizationMetricType {
					targetValue = int(*autoScaling.Resource.Target.AverageUtilization)
				} else if metricType == autoscalingv2.AverageValueMetricType {
					targetValue = int(autoScaling.Resource.Target.AverageValue.AsApproximateFloat64())
				}

				// create a trigger for the resource
				triggers = append(triggers, kedav1alpha1.ScaleTriggers{
					Type:       triggerType,
					Metadata:   map[string]string{"value": strconv.Itoa(targetValue)},
					MetricType: metricType,
				})
			} else if autoScaling.Type == v1beta1.MetricSourceType(constants.AutoScalerExternal) {
				triggerType := string(*autoScaling.External.Metric.Backend)
				serverAddress := autoScaling.External.Metric.ServerAddress
				query := autoScaling.External.Metric.Query
				targetValue = int(autoScaling.External.Target.Value.AsApproximateFloat64())

				// create a trigger for the external metric
				trigger := kedav1alpha1.ScaleTriggers{
					Type: triggerType,
					Metadata: map[string]string{
						"serverAddress": serverAddress,
						"query":         query,
						"threshold":     strconv.Itoa(targetValue),
					},
				}
				if triggerType == string(constants.AutoScalerMetricsPrometheus) {
					if autoScaling.External.Metric.Namespace != "" {
						trigger.Metadata["namespace"] = autoScaling.External.Metric.Namespace
					}
				}

				triggers = append(triggers, trigger)
			}
		}
	} else if componentExt.ScaleMetric != nil {
		triggerType := string(*componentExt.ScaleMetric)
		if componentExt.ScaleMetricType != nil {
			metricType = *componentExt.ScaleMetricType
		}
		if componentExt.ScaleTarget != nil {
			targetValue = int(*componentExt.ScaleTarget)
		}
		triggers = append(triggers, kedav1alpha1.ScaleTriggers{
			Type:       triggerType,
			Metadata:   map[string]string{"value": strconv.Itoa(targetValue)},
			MetricType: metricType,
		})
	}
	return triggers
}

func createKedaScaledObject(componentMeta metav1.ObjectMeta,
	componentExtension *v1beta1.ComponentExtensionSpec,
) *kedav1alpha1.ScaledObject {
	triggers := getKedaMetrics(componentExtension)
	annotations := componentMeta.GetAnnotations()

	MinReplicas := componentExtension.MinReplicas
	MaxReplicas := componentExtension.MaxReplicas

	if MinReplicas == nil {
		MinReplicas = &constants.DefaultMinReplicas
	}

	if MaxReplicas < *MinReplicas {
		MaxReplicas = *MinReplicas
	}

	scaledobject := &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentMeta.Name,
			Namespace:   componentMeta.Namespace,
			Labels:      componentMeta.Labels,
			Annotations: annotations,
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			ScaleTargetRef: &kedav1alpha1.ScaleTarget{
				Name: componentMeta.Name,
			},
			Triggers:        triggers,
			MinReplicaCount: MinReplicas,
			MaxReplicaCount: ptr.To(MaxReplicas),
		},
	}

	return scaledobject
}

func (r *KedaReconciler) Reconcile(ctx context.Context) error {
	desired := r.ScaledObject
	existing := &kedav1alpha1.ScaledObject{}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		log.Info("Updating KEDA ScaledObject", "namespace", desired.Namespace, "name", desired.Name)
		if err := r.client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing); err != nil {
			return err
		}

		return r.client.Update(ctx, existing)
	})
	if err != nil {
		// Create scaledObject if it does not exist
		if apierr.IsNotFound(err) {
			log.Info("Creating KEDA ScaledObject", "namespace", desired.Namespace, "name", desired.Name)
			return r.client.Create(ctx, desired)
		}
		return errors.Wrapf(err, "fails to reconcile KEDA scaledObject")
	}

	return nil
}

func (r *KedaReconciler) SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error {
	return controllerutil.SetControllerReference(owner, r.ScaledObject, scheme)
}
