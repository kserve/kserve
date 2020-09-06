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
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/kmp"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"strings"
)

var (
	log = logf.Log.WithName("IngressReconciler")
)

type IngressReconciler struct {
	client        client.Client
	scheme        *runtime.Scheme
	ingressConfig *v1beta1.IngressConfig
}

func NewIngressReconciler(client client.Client, scheme *runtime.Scheme, ingressConfig *v1beta1.IngressConfig) *IngressReconciler {
	return &IngressReconciler{
		client:        client,
		scheme:        scheme,
		ingressConfig: ingressConfig,
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

func (ir *IngressReconciler) createHTTPRouteDestination(targetHost, namespace string, gatewayService string) *istiov1alpha3.HTTPRouteDestination {
	httpRouteDestination := &istiov1alpha3.HTTPRouteDestination{
		Headers: &istiov1alpha3.Headers{
			Request: &istiov1alpha3.Headers_HeaderOperations{
				Set: map[string]string{
					"Host": network.GetServiceHostname(targetHost, namespace),
				},
			},
		},
		Destination: &istiov1alpha3.Destination{
			Host: gatewayService,
			Port: &istiov1alpha3.PortSelector{
				Number: constants.CommonDefaultHttpPort,
			},
		},
	}
	return httpRouteDestination
}

func (ir *IngressReconciler) createHTTPMatchRequest(prefix, targetHost, internalHost string, isInternal bool) []*istiov1alpha3.HTTPMatchRequest {
	matchRequests := []*istiov1alpha3.HTTPMatchRequest{
		{
			Uri: &istiov1alpha3.StringMatch{
				MatchType: &istiov1alpha3.StringMatch_Regex{
					Regex: prefix,
				},
			},
			Authority: &istiov1alpha3.StringMatch{
				MatchType: &istiov1alpha3.StringMatch_Regex{
					Regex: constants.HostRegExp(internalHost),
				},
			},
			Gateways: []string{constants.KnativeLocalGateway},
		},
	}
	if !isInternal {
		matchRequests = append(matchRequests,
			&istiov1alpha3.HTTPMatchRequest{
				Authority: &istiov1alpha3.StringMatch{
					MatchType: &istiov1alpha3.StringMatch_Regex{
						Regex: constants.HostRegExp(targetHost),
					},
				},
				Gateways: []string{ir.ingressConfig.IngressGateway},
			})
	}
	return matchRequests
}

func (ir *IngressReconciler) Reconcile(isvc *v1beta1.InferenceService) error {
	if !isvc.Status.IsConditionReady(v1beta1.PredictorReady) {
		isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionFalse,
			Reason: "Predictor ingress not created",
		})
		return nil
	}
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
		if !isvc.Status.IsConditionReady(v1beta1.TransformerReady) {
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionFalse,
				Reason: "Transformer ingress not created",
			})
			return nil
		}
	}
	isInternal := false
	if val, ok := isvc.Labels["serving.knative.dev/visibility"]; ok && val == "ClusterLocal" {
		isInternal = true
	}
	httpRoutes := []*istiov1alpha3.HTTPRoute{}
	if isvc.Spec.Explainer != nil {
		if !isvc.Status.IsConditionReady(v1beta1.ExplainerReady) {
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionFalse,
				Reason: "Explainer ingress not created",
			})
			return nil
		}
		explainerRouter := istiov1alpha3.HTTPRoute{
			Match: ir.createHTTPMatchRequest(constants.ExplainPrefix(), constants.HostRegExp(serviceHost),
				network.GetServiceHostname(isvc.Name, isvc.Namespace), isInternal),
			Route: []*istiov1alpha3.HTTPRouteDestination{
				ir.createHTTPRouteDestination(constants.ExplainerServiceName(isvc.Name), isvc.Namespace, constants.LocalGatewayHost),
			},
		}
		httpRoutes = append(httpRoutes, &explainerRouter)
	}
	httpRoutes = append(httpRoutes, &istiov1alpha3.HTTPRoute{
		Match: ir.createHTTPMatchRequest("*", constants.HostRegExp(serviceHost),
			network.GetServiceHostname(isvc.Name, isvc.Namespace), isInternal),
		Route: []*istiov1alpha3.HTTPRouteDestination{
			ir.createHTTPRouteDestination(backend, isvc.Namespace, constants.LocalGatewayHost),
		},
	})

	desiredIngress := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      isvc.Name,
			Namespace: isvc.Namespace,
		},
		Spec: istiov1alpha3.VirtualService{
			Hosts: []string{
				serviceHost,
				network.GetServiceHostname(isvc.Name, isvc.Namespace),
			},
			Gateways: []string{
				ir.ingressConfig.IngressGateway,
				constants.KnativeLocalGateway,
			},
			Http: httpRoutes,
		},
	}
	if err := controllerutil.SetControllerReference(isvc, desiredIngress, ir.scheme); err != nil {
		return err
	}

	existing := &v1alpha3.VirtualService{}
	err := ir.client.Get(context.TODO(), types.NamespacedName{Name: desiredIngress.Name, Namespace: desiredIngress.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Ingress for isvc", "namespace", desiredIngress.Namespace, "name", desiredIngress.Name)
			err = ir.client.Create(context.TODO(), desiredIngress)
		}
	} else {
		if !equality.Semantic.DeepEqual(desiredIngress.Spec, existing.Spec) {
			existing.Spec = desiredIngress.Spec
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
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionTrue,
			})
			return nil
		} else {
			return err
		}
	}
}
