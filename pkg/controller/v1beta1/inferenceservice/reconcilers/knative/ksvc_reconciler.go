/*
Copyright 2020 kubeflow.org.
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

package knative

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmp"
	"knative.dev/serving/pkg/apis/autoscaling"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("KsvcReconciler")

type KsvcReconciler struct {
	client          client.Client
	scheme          *runtime.Scheme
	Service         *knservingv1.Service
	componentExt    *v1beta1.ComponentExtensionSpec
	componentStatus v1beta1.ComponentStatusSpec
}

func NewKsvcReconciler(client client.Client, scheme *runtime.Scheme, componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec, podSpec *corev1.PodSpec,
	componentStatus v1beta1.ComponentStatusSpec) *KsvcReconciler {
	return &KsvcReconciler{
		client:          client,
		scheme:          scheme,
		Service:         createKnativeService(componentMeta, componentExt, podSpec, componentStatus),
		componentExt:    componentExt,
		componentStatus: componentStatus,
	}
}

func createKnativeService(componentMeta metav1.ObjectMeta,
	componentExtension *v1beta1.ComponentExtensionSpec, podSpec *corev1.PodSpec,
	componentStatus v1beta1.ComponentStatusSpec) *knservingv1.Service {
	annotations := componentMeta.GetAnnotations()

	if componentExtension.MinReplicas == nil {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(constants.DefaultMinReplicas)
	} else {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(*componentExtension.MinReplicas)
	}

	if componentExtension.MaxReplicas != 0 {
		annotations[autoscaling.MaxScaleAnnotationKey] = fmt.Sprint(componentExtension.MaxReplicas)
	}

	// User can pass down scaling class annotation to overwrite the default scaling KPA
	if _, ok := annotations[autoscaling.ClassAnnotationKey]; !ok {
		annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
	}
	trafficTargets := []knservingv1.TrafficTarget{}
	if componentExtension.CanaryTrafficPercent != nil && componentStatus.PreviousReadyRevision != "" {
		//canary rollout
		trafficTargets = append(trafficTargets,
			knservingv1.TrafficTarget{
				Tag:            "latest",
				LatestRevision: proto.Bool(true),
				Percent:        proto.Int64(*componentExtension.CanaryTrafficPercent),
			})
		remainingTraffic := 100 - *componentExtension.CanaryTrafficPercent
		trafficTargets = append(trafficTargets,
			knservingv1.TrafficTarget{
				Tag:            "prev",
				RevisionName:   componentStatus.PreviousReadyRevision,
				LatestRevision: proto.Bool(false),
				Percent:        proto.Int64(remainingTraffic),
			})
	} else {
		//blue green rollout
		trafficTargets = append(trafficTargets,
			knservingv1.TrafficTarget{
				Tag:            "latest",
				LatestRevision: proto.Bool(true),
				Percent:        proto.Int64(100),
			})
	}

	return &knservingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentMeta.Name,
			Namespace: componentMeta.Namespace,
			Labels:    componentMeta.Labels,
		},
		Spec: knservingv1.ServiceSpec{
			ConfigurationSpec: knservingv1.ConfigurationSpec{
				Template: knservingv1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      componentMeta.Labels,
						Annotations: annotations,
					},
					Spec: knservingv1.RevisionSpec{
						TimeoutSeconds:       componentExtension.TimeoutSeconds,
						ContainerConcurrency: componentExtension.ContainerConcurrency,
						PodSpec:              *podSpec,
					},
				},
			},
			RouteSpec: knservingv1.RouteSpec{
				Traffic: trafficTargets,
			},
		},
	}
}

func (r *KsvcReconciler) finalizeService(serviceName, namespace string) error {
	existing := &knservingv1.Service{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: serviceName, Namespace: namespace}, existing); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		log.Info("Deleting Knative Service", "namespace", namespace, "name", serviceName)
		if err := r.client.Delete(context.TODO(), existing, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

func (r *KsvcReconciler) Reconcile() (*knservingv1.ServiceStatus, error) {
	// Create service if does not exist
	desired := r.Service
	existing := &knservingv1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Knative Service", "namespace", desired.Namespace, "name", desired.Name)
			return &desired.Status, r.client.Create(context.TODO(), desired)
		}
		return nil, err
	}
	// Return if no differences to reconcile.
	if semanticEquals(desired, existing) {
		return &existing.Status, nil
	}

	// Reconcile differences and update
	diff, err := kmp.SafeDiff(desired.Spec.ConfigurationSpec, existing.Spec.ConfigurationSpec)
	if err != nil {
		return &existing.Status, fmt.Errorf("failed to diff Knative Service: %v", err)
	}
	log.Info("Reconciling Knative Service diff (-desired, +observed):", "diff", diff)
	log.Info("Updating Knative Service", "namespace", desired.Namespace, "name", desired.Name)
	existing.Spec.ConfigurationSpec = desired.Spec.ConfigurationSpec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels

	if r.componentExt.CanaryTrafficPercent != nil && r.componentStatus.LatestReadyRevision != "" &&
		r.componentStatus.LatestReadyRevision != existing.Status.LatestReadyRevisionName {
		log.Info("Updating Knative Service traffic target", "namespace", desired.Namespace, "name", desired.Name)
		trafficTargets := []knservingv1.TrafficTarget{}
		trafficTargets = append(trafficTargets,
			knservingv1.TrafficTarget{
				Tag:            "latest",
				LatestRevision: proto.Bool(true),
				Percent:        r.componentExt.CanaryTrafficPercent,
			})
		remainingTraffic := 100 - *r.componentExt.CanaryTrafficPercent
		trafficTargets = append(trafficTargets,
			knservingv1.TrafficTarget{
				Tag:            "prev",
				RevisionName:   r.componentStatus.LatestReadyRevision,
				LatestRevision: proto.Bool(false),
				Percent:        proto.Int64(remainingTraffic),
			})
		existing.Spec.Traffic = trafficTargets
	}
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.client.Update(context.TODO(), existing)
	})
	if err != nil {
		return &existing.Status, err
	}
	return &existing.Status, nil
}

func semanticEquals(desiredService, service *knservingv1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec.ConfigurationSpec, service.Spec.ConfigurationSpec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels)
}
