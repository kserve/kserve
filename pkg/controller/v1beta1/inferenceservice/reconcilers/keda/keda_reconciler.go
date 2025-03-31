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
	"fmt"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"strconv"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	configMap *corev1.ConfigMap,
) (*KedaReconciler, error) {
	scaledObject, err := createKedaScaledObject(componentMeta, componentExt, configMap)
	if err != nil {
		return nil, err
	}
	return &KedaReconciler{
		client:       client,
		scheme:       scheme,
		ScaledObject: scaledObject,
		componentExt: componentExt,
	}, nil
}

func getKedaMetrics(componentExt *v1beta1.ComponentExtensionSpec,
	minReplicas int32, maxReplicas int32, configMap *corev1.ConfigMap,
) ([]kedav1alpha1.ScaleTriggers, error) {
	var triggers []kedav1alpha1.ScaleTriggers

	// metric configuration from componentExtension.AutoScaling if it is set
	if componentExt.AutoScaling != nil {
		metrics := componentExt.AutoScaling.Metrics
		for _, metric := range metrics {
			switch metric.Type {
			case v1beta1.ResourceMetricSourceType:
				triggerType := string(metric.Resource.Name)
				metricType := metric.Resource.Target.Type
				targetValue := "0"
				if metricType == v1beta1.UtilizationMetricType {
					averageUtil := metric.Resource.Target.AverageUtilization
					if metric.Resource.Name == v1beta1.ResourceMetricCPU {
						if metric.Resource.Target.AverageUtilization == nil {
							averageUtil = &constants.DefaultCPUUtilization
						}
					}
					if metric.Resource.Target.AverageUtilization != nil {
						targetValue = fmt.Sprintf("%d", averageUtil)
					}
				} else if metricType == v1beta1.AverageValueMetricType && metric.Resource.Target.AverageValue != nil {
					targetValue = metric.Resource.Target.AverageValue.String()
				} else if metricType == v1beta1.ValueMetricType && metric.Resource.Target.Value != nil {
					targetValue = fmt.Sprintf("%f", metric.Resource.Target.Value.AsApproximateFloat64())
				}
				triggers = append(triggers, kedav1alpha1.ScaleTriggers{
					Type:       triggerType,
					Metadata:   map[string]string{"value": targetValue},
					MetricType: autoscalingv2.MetricTargetType(metricType),
				})
			case v1beta1.ExternalMetricSourceType:
				triggerType := string(metric.External.Metric.Backend)
				serverAddress := metric.External.Metric.ServerAddress
				query := metric.External.Metric.Query

				trigger := kedav1alpha1.ScaleTriggers{
					Type: triggerType,
					Metadata: map[string]string{
						"serverAddress": serverAddress,
						"query":         query,
						"threshold":     fmt.Sprintf("%f", metric.External.Target.Value.AsApproximateFloat64()),
					},
				}
				if triggerType == string(constants.AutoScalerMetricsSourcePrometheus) && metric.External.Metric.Namespace != "" {
					trigger.Metadata["namespace"] = metric.External.Metric.Namespace
				}
				triggers = append(triggers, trigger)
			case v1beta1.PodMetricSourceType:
				otelConfig, err := v1beta1.NewOtelCollectorConfig(configMap)
				if err != nil {
					return nil, err
				}
				MetricScalerEndpoint := otelConfig.MetricScalerEndpoint
				if metric.PodMetric.Metric.ServerAddress != "" {
					MetricScalerEndpoint = metric.PodMetric.Metric.ServerAddress
				}

				triggerType := string(metric.PodMetric.Metric.Backend)
				query := metric.PodMetric.Metric.Query
				targetValue := metric.PodMetric.Target.Value.AsApproximateFloat64()

				trigger := kedav1alpha1.ScaleTriggers{
					Metadata: map[string]string{},
				}

				if triggerType == string(constants.AutoScalerMetricsSourceOpenTelemetry) {
					trigger.Type = "external"
					trigger.Metadata = map[string]string{
						"clampMin":      strconv.Itoa(int(minReplicas)),
						"clampMax":      strconv.Itoa(int(maxReplicas)),
						"metricQuery":   query,
						"targetValue":   fmt.Sprintf("%f", targetValue),
						"scalerAddress": MetricScalerEndpoint,
					}
					if metric.PodMetric.Metric.OperationOverTime != "" {
						trigger.Metadata["operationOverTime"] = metric.PodMetric.Metric.OperationOverTime
					}
				}

				triggers = append(triggers, trigger)
			}
		}
	}
	return triggers, nil
}

func createKedaScaledObject(componentMeta metav1.ObjectMeta,
	componentExtension *v1beta1.ComponentExtensionSpec,
	configMap *corev1.ConfigMap,
) (*kedav1alpha1.ScaledObject, error) {
	annotations := componentMeta.GetAnnotations()

	MinReplicas := componentExtension.MinReplicas
	MaxReplicas := componentExtension.MaxReplicas

	if MinReplicas == nil {
		MinReplicas = &constants.DefaultMinReplicas
	}

	if MaxReplicas < *MinReplicas {
		MaxReplicas = *MinReplicas
	}
	triggers, err := getKedaMetrics(componentExtension, *MinReplicas, MaxReplicas, configMap)
	if err != nil {
		return nil, err
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

	return scaledobject, nil
}

func semanticScaledObjectEquals(desired, existing *kedav1alpha1.ScaledObject) bool {
	return equality.Semantic.DeepEqual(desired.Spec, existing.Spec)
}

func (r *KedaReconciler) Reconcile(ctx context.Context) error {
	desired := r.ScaledObject
	existing := &kedav1alpha1.ScaledObject{}
	err := r.client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if apierr.IsNotFound(err) {
			log.Info("Creating KEDA ScaledObject resource", "name", desired.Name)
			if err := r.client.Create(ctx, desired); err != nil {
				log.Error(err, "Failed to create KEDA ScaledObject", "name", desired.Name)
				return err
			}
		} else {
			return err
		}
	} else {
		// Set ResourceVersion which is required for update operation.
		desired.ResourceVersion = existing.ResourceVersion
		// Do a dry-run update to avoid diffs generated by default values.
		// This will populate our local ScaledObject with any default values that are present on the remote version.
		if err := r.client.Update(ctx, desired, client.DryRunAll); err != nil {
			log.Error(err, "Failed to perform dry-run update for KEDA ScaledObject", "name", desired.Name)
			return err
		}
		if !semanticScaledObjectEquals(desired, existing) {
			log.Info("Updating KEDA ScaledObject resource", "name", desired.Name)
			if err := r.client.Update(ctx, desired); err != nil {
				log.Error(err, "Failed to update KEDA ScaledObject", "name", desired.Name)
			}
		}
	}
	return nil
}

func (r *KedaReconciler) SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error {
	return controllerutil.SetControllerReference(owner, r.ScaledObject, scheme)
}
