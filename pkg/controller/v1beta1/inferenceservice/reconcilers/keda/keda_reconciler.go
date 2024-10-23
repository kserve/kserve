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

package keda

import (
	"context"
	"strconv"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("KedaReconciler")

type KedaReconciler struct {
	client       client.Client
	scheme       *runtime.Scheme
	ScaledObject *kedav1alpha1.ScaledObject
	componentExt *v1beta1.ComponentExtensionSpec
}

func NewKedaReconciler(client client.Client,
	clientset kubernetes.Interface,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec) *KedaReconciler {
	return &KedaReconciler{
		client:       client,
		scheme:       scheme,
		ScaledObject: createKedaScaledObject(clientset, componentMeta, componentExt),
		componentExt: componentExt,
	}
}

func getKedaMetrics(metricConfig *v1beta1.MetricsConfig, metadata metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec) []kedav1alpha1.ScaleTriggers {
	var triggers []kedav1alpha1.ScaleTriggers
	var serverAddress string

	// Default values
	triggerType := string(corev1.ResourceCPU)
	metricType := autoscalingv2.UtilizationMetricType
	scaleTarget := int(constants.DefaultCPUUtilization)

	// Get metric configuration from metricConfig (configmap)
	if metricConfig != nil && metricConfig.MetricBackend != "" {
		triggerType = metricConfig.MetricBackend
		serverAddress = metricConfig.ServerAddress
	}
	// override metric configuration from componentExtension if it is set
	if componentExt.ScaleMetric != nil {
		triggerType = string(*componentExt.ScaleMetric)
	}
	if componentExt.ScaleMetricType != nil {
		metricType = *componentExt.ScaleMetricType
	}
	if componentExt.ScaleTarget != nil {
		scaleTarget = *componentExt.ScaleTarget
	}
	// override metric configuration from componentExtension.ScalerSpec if it is set
	if componentExt.ScalerSpec != nil {
		if componentExt.ScalerSpec.ScaleMetric != nil {
			triggerType = string(*componentExt.ScalerSpec.ScaleMetric)
		}
		if componentExt.ScalerSpec.ServerAddress != "" {
			serverAddress = componentExt.ScalerSpec.ServerAddress
		}
		if componentExt.ScalerSpec.ScaleMetricType != nil {
			scaleTarget = *componentExt.ScalerSpec.ScaleTarget
		}
		if componentExt.ScalerSpec.ScaleMetricType != nil {
			metricType = *componentExt.ScalerSpec.ScaleMetricType
		}
	}

	trigger := kedav1alpha1.ScaleTriggers{
		Type:     triggerType,
		Metadata: map[string]string{},
	}

	// set trigger metadata for prometheus and graphite triggers
	if triggerType == "prometheus" || triggerType == "graphite" {
		if serverAddress != "" {
			trigger.Metadata["serverAddress"] = serverAddress
		}
		if componentExt.ScalerSpec != nil {
			if componentExt.ScalerSpec.MetricQuery != "" {
				trigger.Metadata["query"] = componentExt.ScalerSpec.MetricQuery
			}
			if componentExt.ScalerSpec.QueryParameters != "" {
				trigger.Metadata["queryParameters"] = componentExt.ScalerSpec.QueryParameters
			}
			if componentExt.ScalerSpec.ServerAddress != "" {
				trigger.Metadata["threshold"] = strconv.Itoa((scaleTarget))
			}
		}
	} else {
		// set trigger metadata for other triggerTypes
		trigger.Metadata["value"] = strconv.Itoa((scaleTarget))
		trigger.MetricType = metricType
	}

	// set queryTime for graphite trigger (if set)
	if triggerType == "graphite" {
		if componentExt.ScalerSpec.QueryTime != "" {
			trigger.Metadata["queryTime"] = componentExt.ScalerSpec.QueryTime
		}
	}

	triggers = append(triggers, trigger)
	return triggers
}

func createKedaScaledObject(clientset kubernetes.Interface, componentMeta metav1.ObjectMeta,
	componentExtension *v1beta1.ComponentExtensionSpec) *kedav1alpha1.ScaledObject {
	metricConfig, err := v1beta1.NewMetricsConfig(clientset)
	if err != nil {
		return nil
	}

	triggers := getKedaMetrics(metricConfig, componentMeta, componentExtension)
	annotations := componentMeta.GetAnnotations()

	MinReplicas := componentExtension.MinReplicas
	MaxReplicas := componentExtension.MaxReplicas
	if componentExtension.ScalerSpec != nil {
		MinReplicas = componentExtension.ScalerSpec.MinReplicas
		MaxReplicas = componentExtension.ScalerSpec.MaxReplicas
	}

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
			MinReplicaCount: proto.Int32(int32(*MinReplicas)),
			MaxReplicaCount: proto.Int32(int32(MaxReplicas)),
		},
	}

	return scaledobject
}

func (r *KedaReconciler) Reconcile() error {
	desired := r.ScaledObject
	existing := &kedav1alpha1.ScaledObject{}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		log.Info("Updating KEDA ScaledObject", "namespace", desired.Namespace, "name", desired.Name)
		if err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing); err != nil {
			return err
		}

		return r.client.Update(context.TODO(), existing)
	})
	if err != nil {
		// Create scaledObject if it does not exist
		if apierr.IsNotFound(err) {
			log.Info("Creating KEDA ScaledObject", "namespace", desired.Namespace, "name", desired.Name)
			return r.client.Create(context.TODO(), desired)
		}
		return errors.Wrapf(err, "fails to reconcile KEDA scaledObject")
	}

	return nil
}
func (r *KedaReconciler) SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error {
	return controllerutil.SetControllerReference(owner, r.ScaledObject, scheme)
}
