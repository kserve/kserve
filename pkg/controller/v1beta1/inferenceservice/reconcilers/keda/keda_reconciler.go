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

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("KedaReconciler")

var managedKsvcAnnotations = map[string]bool{
	constants.RollOutDurationAnnotationKey: true,
	// Required for the integration of Openshift Serverless with Openshift Service Mesh
	constants.KnativeOpenshiftEnablePassthroughKey: true,
}

type KedaReconciler struct {
	client       client.Client
	scheme       *runtime.Scheme
	ScaledObject *kedav1alpha1.ScaledObject
}

func NewKedaReconciler(client client.Client,
	componentMeta metav1.ObjectMeta,
	scheme *runtime.Scheme,
	podSpec *corev1.PodSpec,
	componentStatus v1beta1.ComponentStatusSpec) *KedaReconciler {
	return &KedaReconciler{
		client:       client,
		scheme:       scheme,
		ScaledObject: createKedaScaledObject(componentMeta, podSpec, componentStatus),
	}
}

func createKedaScaledObject(componentMeta metav1.ObjectMeta,
	podSpec *corev1.PodSpec,
	componentStatus v1beta1.ComponentStatusSpec) *kedav1alpha1.ScaledObject {
	annotations := componentMeta.GetAnnotations()

	// ksvc metadata.annotations
	// rollout-duration must be put under metadata.annotations
	ksvcAnnotations := make(map[string]string)
	for ksvcAnnotationKey := range managedKsvcAnnotations {
		if value, ok := annotations[ksvcAnnotationKey]; ok {
			ksvcAnnotations[ksvcAnnotationKey] = value
			delete(annotations, ksvcAnnotationKey)
		}
	}

	scaledobject := &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentMeta.Name,
			Namespace:   componentMeta.Namespace,
			Labels:      componentMeta.Labels,
			Annotations: ksvcAnnotations,
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			ScaleTargetRef: &kedav1alpha1.ScaleTarget{
				Name: componentMeta.Name,
			},
			Triggers: []kedav1alpha1.ScaleTriggers{
				{
					Type:             "cpu",
					UseCachedMetrics: false,
					Metadata:         map[string]string{"type": "Utilization", "value": "60"}},
			},
			MinReplicaCount: proto.Int32(1),
		},
		Status: kedav1alpha1.ScaledObjectStatus{},
	}
	return scaledobject
}

func (r *KedaReconciler) Reconcile() (*kedav1alpha1.ScaledObjectStatus, error) {
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
		// Create service if it does not exist
		if apierr.IsNotFound(err) {
			log.Info("Creating KEDA ScaledObject", "namespace", desired.Namespace, "name", desired.Name)
			return &desired.Status, r.client.Create(context.TODO(), desired)
		}
		return &existing.Status, errors.Wrapf(err, "fails to reconcile knative service")
	}

	return &existing.Status, nil
}
