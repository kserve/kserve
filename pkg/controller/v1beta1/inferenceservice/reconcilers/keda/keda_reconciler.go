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
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
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

func createKedaScaledObject(clientset kubernetes.Interface, componentMeta metav1.ObjectMeta,
	componentExtension *v1beta1.ComponentExtensionSpec) *kedav1alpha1.ScaledObject {
	metricConfig, err := v1beta1.NewMetricsConfig(clientset)
	if err != nil {
		return nil
	}

	// metrics configs from configmap (which can be overridden by ScalerSpec)
	triggerType := metricConfig.MetricBackend
	serverAddress := metricConfig.ServerAddress

	annotations := componentMeta.GetAnnotations()

	ScaleMetric := componentExtension.ScaleMetric
	MinReplicas := componentExtension.MinReplicas
	MaxReplicas := componentExtension.MaxReplicas
	ScaleMetricType := componentExtension.ScaleMetricType
	ScaleTarget := componentExtension.ScaleTarget
	if componentExtension.ScalerSpec != nil {
		ScaleMetric = componentExtension.ScalerSpec.ScaleMetric
		MinReplicas = componentExtension.ScalerSpec.MinReplicas
		MaxReplicas = componentExtension.ScalerSpec.MaxReplicas
		ScaleMetricType = componentExtension.ScalerSpec.ScaleMetricType
		ScaleTarget = componentExtension.ScalerSpec.ScaleTarget
	}

	// overridding the trigger type if ScaleMetric is set in he ScalerSpec
	if ScaleMetric != nil && *ScaleMetric != "" {
		triggerType = string(*ScaleMetric)
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
			Triggers: []kedav1alpha1.ScaleTriggers{
				{
					Type:     triggerType,
					Metadata: map[string]string{},
				},
			},
			MinReplicaCount: proto.Int32(int32(*MinReplicas)),
			MaxReplicaCount: proto.Int32(int32(MaxReplicas)),
		},
	}
	if ScaleMetricType != nil {
		scaledobject.Spec.Triggers[0].MetricType = *ScaleMetricType
	}

	// set queryTime for graphite trigger
	if triggerType == "graphite" {
		if componentExtension.ScalerSpec.QueryTime != "" {
			scaledobject.Spec.Triggers[0].Metadata["queryTime"] = componentExtension.ScalerSpec.QueryTime
		}
	}

	// set trigger metadata for prometheus and graphite triggers
	if triggerType == "prometheus" || triggerType == "graphite" {
		if componentExtension.ScalerSpec.ServerAddress != "" {
			serverAddress = componentExtension.ScalerSpec.ServerAddress
		}
		if componentExtension.ScalerSpec.ServerAddress != "" {
			scaledobject.Spec.Triggers[0].Metadata["serverAddress"] = serverAddress
		}
		if componentExtension.ScalerSpec.MetricQuery != "" {
			scaledobject.Spec.Triggers[0].Metadata["query"] = componentExtension.ScalerSpec.MetricQuery
		}
		if componentExtension.ScalerSpec.QueryParameters != "" {
			scaledobject.Spec.Triggers[0].Metadata["queryParameters"] = componentExtension.ScalerSpec.QueryParameters
		}
		if componentExtension.ScalerSpec.ServerAddress != "" {
			scaledobject.Spec.Triggers[0].Metadata["threshold"] = strconv.Itoa((*ScaleTarget))
		}
	} else {
		// set trigger metadata other ScaleMetric
		scaledobject.Spec.Triggers[0].Metadata["value"] = strconv.Itoa((*ScaleTarget))
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
