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

package ingress

import (
	"context"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/kmp"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	//?"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"strings"
)

var log = logf.Log.WithName("IngressReconciler")

type IngressReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewIngressReconciler(client client.Client, scheme *runtime.Scheme) *IngressReconciler {
	return &IngressReconciler{
		client: client,
		scheme: scheme,
	}
}

func getServiceHost(isvc *v1beta1.InferenceService) string {
	if isvc.Status.Components == nil {
		return ""
	}

	if isvc.Spec.Transformer != nil {
		if transformerStatus, ok := isvc.Status.Components[v1beta1.TransformerComponent]; !ok {
			return ""
		} else if transformerStatus.URL == nil {
			return ""
		} else {
			return strings.ReplaceAll(transformerStatus.URL.Host, fmt.Sprintf("-%s", string(constants.Transformer)), "")
		}
	}

	if predictorStatus, ok := isvc.Status.Components[v1beta1.PredictorComponent]; !ok {
		return ""
	} else if predictorStatus.URL == nil {
		return ""
	} else {
		return strings.ReplaceAll(predictorStatus.URL.Host, fmt.Sprintf("-%s", string(constants.Predictor)), "")
	}
}

func getServiceUrl(isvc *v1beta1.InferenceService) string {
	if isvc.Status.Components == nil {
		return ""
	}

	if predictorStatus, ok := isvc.Status.Components[v1beta1.PredictorComponent]; !ok {
		return ""
	} else if predictorStatus.URL == nil {
		return ""
	} else {
		return strings.ReplaceAll(predictorStatus.URL.String(), fmt.Sprintf("-%s", string(constants.Predictor)), "")
	}
}

func (r *IngressReconciler) reconcileExternalService(isvc *v1beta1.InferenceService) error {
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      isvc.Name,
			Namespace: isvc.Namespace,
		},
		Spec: corev1.ServiceSpec{
			ExternalName: constants.LocalGatewayHost,
			Type:         corev1.ServiceTypeExternalName,
		},
	}
	if err := controllerutil.SetControllerReference(isvc, desired, r.scheme); err != nil {
		return err
	}

	// Create service if does not exist
	existing := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating External Service", "namespace", desired.Namespace, "name", desired.Name)
			err = r.client.Create(context.TODO(), desired)
		}
		return err
	}

	// Return if no differences to reconcile.
	if equality.Semantic.DeepEqual(desired, existing) {
		return nil
	}

	// Reconcile differences and update
	diff, err := kmp.SafeDiff(desired.Spec, existing.Spec)
	if err != nil {
		return fmt.Errorf("failed to diff virtual service: %v", err)
	}
	log.Info("Reconciling external service diff (-desired, +observed):", "diff", diff)
	log.Info("Updating external service", "namespace", existing.Namespace, "name", existing.Name)
	existing.Spec = desired.Spec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.ObjectMeta.Annotations = desired.ObjectMeta.Annotations
	err = r.client.Update(context.TODO(), existing)
	if err != nil {
		return err
	}

	return nil
}

func (ir *IngressReconciler) Reconcile(isvc *v1beta1.InferenceService) error {
	//Create external service which points to local gateway
	if err := ir.reconcileExternalService(isvc); err != nil {
		return err
	}
	//Create ingress
	serviceHost := getServiceHost(isvc)
	serviceUrl := getServiceUrl(isvc)
	if serviceHost == "" || serviceUrl == "" {
		return nil
	}
	backend := constants.PredictorServiceName(isvc.Name)
	if isvc.Spec.Transformer != nil {
		backend = constants.TransformerServiceName(isvc.Name)
	}
	desiredIngress := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      isvc.Name,
			Namespace: isvc.Namespace,
		},
		Spec: networking.IngressSpec{
			Backend: &networking.IngressBackend{
				ServiceName: backend,
				ServicePort: intstr.FromInt(80),
			},
			Rules: []networking.IngressRule{
				{
					Host: serviceHost,
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path: constants.ExplainPath(isvc.Name),
									Backend: networking.IngressBackend{
										ServiceName: constants.ExplainerServiceName(isvc.Name),
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(isvc, desiredIngress, ir.scheme); err != nil {
		return err
	}

	existing := &networking.Ingress{}
	err := ir.client.Get(context.TODO(), types.NamespacedName{Name: desiredIngress.Name, Namespace: desiredIngress.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Ingress for isvc", "namespace", desiredIngress.Namespace, "name", desiredIngress.Name)
			err = ir.client.Create(context.TODO(), desiredIngress)
		}
	} else {
		if !equality.Semantic.DeepEqual(desiredIngress.Spec, existing.Spec) {
			err = ir.client.Update(context.TODO(), existing)
		}
	}
	if err != nil {
		return err
	} else {
		if url, err := apis.ParseURL(serviceUrl); err == nil {
			isvc.Status.URL = url
			isvc.Status.Address = &duckv1.Addressable{
				URL: &apis.URL{
					Host:   network.GetServiceHostname(isvc.Name, isvc.Namespace),
					Scheme: "http",
				},
			}
			return nil
		} else {
			return err
		}
	}
}
