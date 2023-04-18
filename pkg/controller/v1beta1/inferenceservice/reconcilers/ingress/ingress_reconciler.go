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

package ingress

import (
	"context"
	"fmt"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	utils "github.com/kserve/kserve/pkg/utils"
	"github.com/pkg/errors"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/kmp"
	"knative.dev/pkg/network"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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
	//Derive the ingress service host from underlying service url
	if isvc.Spec.Transformer != nil {
		if transformerStatus, ok := isvc.Status.Components[v1beta1.TransformerComponent]; !ok {
			return ""
		} else if transformerStatus.URL == nil {
			return ""
		} else {
			if strings.Contains(transformerStatus.URL.Host, "-default") {
				return strings.Replace(transformerStatus.URL.Host, fmt.Sprintf("-%s-default", string(constants.Transformer)), "",
					1)
			} else {
				return strings.Replace(transformerStatus.URL.Host, fmt.Sprintf("-%s", string(constants.Transformer)), "",
					1)
			}
		}
	}

	if predictorStatus, ok := isvc.Status.Components[v1beta1.PredictorComponent]; !ok {
		return ""
	} else if predictorStatus.URL == nil {
		return ""
	} else {
		if strings.Contains(predictorStatus.URL.Host, "-default") {
			return strings.Replace(predictorStatus.URL.Host, fmt.Sprintf("-%s-default", string(constants.Predictor)), "",
				1)
		} else {
			return strings.Replace(predictorStatus.URL.Host, fmt.Sprintf("-%s", string(constants.Predictor)), "",
				1)
		}
	}
}

func getServiceUrl(isvc *v1beta1.InferenceService, config *v1beta1.IngressConfig) string {

	url := getHostBasedServiceUrl(isvc, config)
	if url == "" {
		return ""
	}
	if config.PathTemplate == "" {
		return url
	} else {
		return getPathBasedServiceUrl(isvc, config)
	}

}

func getPathBasedServiceUrl(isvc *v1beta1.InferenceService, config *v1beta1.IngressConfig) string {
	path, err := GenerateUrlPath(isvc.Name, isvc.Namespace, config)
	if err != nil {
		log.Error(err, "Failed to generate URL path from pathTemplate")
		return ""
	}
	url := &apis.URL{}
	url.Scheme = config.UrlScheme
	url.Host = config.IngressDomain
	url.Path = path

	return url.String()
}

func getHostBasedServiceUrl(isvc *v1beta1.InferenceService, config *v1beta1.IngressConfig) string {
	urlScheme := config.UrlScheme
	disableIstioVirtualHost := config.DisableIstioVirtualHost
	if isvc.Status.Components == nil {
		return ""
	}
	//Derive the ingress url from underlying service url
	if isvc.Spec.Transformer != nil {
		if transformerStatus, ok := isvc.Status.Components[v1beta1.TransformerComponent]; !ok {
			return ""
		} else if transformerStatus.URL == nil {
			return ""
		} else {
			url := transformerStatus.URL
			url.Scheme = urlScheme
			urlString := url.String()
			if disableIstioVirtualHost == false {
				if strings.Contains(urlString, "-default") {
					return strings.Replace(urlString, fmt.Sprintf("-%s-default", string(constants.Transformer)), "", 1)
				} else {
					return strings.Replace(urlString, fmt.Sprintf("-%s", string(constants.Transformer)), "", 1)
				}
			}
			return urlString
		}
	}

	if predictorStatus, ok := isvc.Status.Components[v1beta1.PredictorComponent]; !ok {
		return ""
	} else if predictorStatus.URL == nil {
		return ""
	} else {
		url := predictorStatus.URL
		url.Scheme = urlScheme
		urlString := url.String()
		if disableIstioVirtualHost == false {
			if strings.Contains(urlString, "-default") {
				return strings.Replace(urlString, fmt.Sprintf("-%s-default", string(constants.Predictor)), "", 1)
			} else {
				return strings.Replace(urlString, fmt.Sprintf("-%s", string(constants.Predictor)), "", 1)
			}
		}
		return urlString
	}
}

func (r *IngressReconciler) reconcileExternalService(isvc *v1beta1.InferenceService, config *v1beta1.IngressConfig) error {
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      isvc.Name,
			Namespace: isvc.Namespace,
		},
		Spec: corev1.ServiceSpec{
			ExternalName:    config.LocalGatewayServiceName,
			Type:            corev1.ServiceTypeExternalName,
			SessionAffinity: corev1.ServiceAffinityNone,
		},
	}
	if err := controllerutil.SetControllerReference(isvc, desired, r.scheme); err != nil {
		return err
	}

	// Create service if does not exist
	existing := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if apierr.IsNotFound(err) {
			log.Info("Creating external name service", "namespace", desired.Namespace, "name", desired.Name)
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
		return errors.Wrapf(err, "failed to diff external name service")
	}
	log.Info("Reconciling external service diff (-desired, +observed):", "diff", diff)
	log.Info("Updating external service", "namespace", existing.Namespace, "name", existing.Name)
	existing.Spec = desired.Spec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.ObjectMeta.Annotations = desired.ObjectMeta.Annotations
	err = r.client.Update(context.TODO(), existing)
	if err != nil {
		return errors.Wrapf(err, "fails to update external name service")
	}

	return nil
}

func createHTTPRouteDestination(gatewayService string) *istiov1alpha3.HTTPRouteDestination {
	httpRouteDestination := &istiov1alpha3.HTTPRouteDestination{
		Destination: &istiov1alpha3.Destination{
			Host: gatewayService,
			Port: &istiov1alpha3.PortSelector{
				Number: constants.CommonDefaultHttpPort,
			},
		},
		Weight: 100,
	}
	return httpRouteDestination
}

func createHTTPMatchRequest(prefix, targetHost, internalHost string, isInternal bool, config *v1beta1.IngressConfig) []*istiov1alpha3.HTTPMatchRequest {
	var uri *istiov1alpha3.StringMatch
	if prefix != "" {
		uri = &istiov1alpha3.StringMatch{
			MatchType: &istiov1alpha3.StringMatch_Regex{
				Regex: prefix,
			},
		}
	}
	matchRequests := []*istiov1alpha3.HTTPMatchRequest{
		{
			Uri: uri,
			Authority: &istiov1alpha3.StringMatch{
				MatchType: &istiov1alpha3.StringMatch_Regex{
					Regex: constants.HostRegExp(internalHost),
				},
			},
			Gateways: []string{config.LocalGateway},
		},
	}
	if !isInternal {
		matchRequests = append(matchRequests,
			&istiov1alpha3.HTTPMatchRequest{
				Uri: uri,
				Authority: &istiov1alpha3.StringMatch{
					MatchType: &istiov1alpha3.StringMatch_Regex{
						Regex: constants.HostRegExp(targetHost),
					},
				},
				Gateways: []string{config.IngressGateway},
			})
	}
	return matchRequests
}

func createIngress(isvc *v1beta1.InferenceService, useDefault bool, config *v1beta1.IngressConfig) *v1alpha3.VirtualService {
	serviceHost := getServiceHost(isvc)
	if serviceHost == "" {
		return nil
	}

	if !isvc.Status.IsConditionReady(v1beta1.PredictorReady) {
		isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionFalse,
			Reason: "Predictor ingress not created",
		})
		return nil
	}
	backend := constants.PredictorServiceName(isvc.Name)
	if useDefault {
		backend = constants.DefaultPredictorServiceName(isvc.Name)
	}

	if isvc.Spec.Transformer != nil {
		backend = constants.TransformerServiceName(isvc.Name)
		if useDefault {
			backend = constants.DefaultTransformerServiceName(isvc.Name)
		}
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
	//if service is labelled with cluster local or knative domain is configured as internal
	if val, ok := isvc.Labels[constants.VisibilityLabel]; ok && val == constants.ClusterLocalVisibility {
		isInternal = true
	}
	serviceInternalHostName := network.GetServiceHostname(isvc.Name, isvc.Namespace)
	if serviceHost == serviceInternalHostName {
		isInternal = true
	}
	httpRoutes := []*istiov1alpha3.HTTPRoute{}
	// Build explain route
	expBackend := constants.ExplainerServiceName(isvc.Name)
	if useDefault {
		expBackend = constants.DefaultExplainerServiceName(isvc.Name)
	}
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
			Match: createHTTPMatchRequest(constants.ExplainPrefix(), serviceHost,
				network.GetServiceHostname(isvc.Name, isvc.Namespace), isInternal, config),
			Route: []*istiov1alpha3.HTTPRouteDestination{
				createHTTPRouteDestination(config.LocalGatewayServiceName),
			},
			Headers: &istiov1alpha3.Headers{
				Request: &istiov1alpha3.Headers_HeaderOperations{
					Set: map[string]string{
						"Host": network.GetServiceHostname(expBackend, isvc.Namespace),
					},
				},
			},
		}
		httpRoutes = append(httpRoutes, &explainerRouter)
	}
	// Add predict route
	httpRoutes = append(httpRoutes, &istiov1alpha3.HTTPRoute{
		Match: createHTTPMatchRequest("", serviceHost,
			network.GetServiceHostname(isvc.Name, isvc.Namespace), isInternal, config),
		Route: []*istiov1alpha3.HTTPRouteDestination{
			createHTTPRouteDestination(config.LocalGatewayServiceName),
		},
		Headers: &istiov1alpha3.Headers{
			Request: &istiov1alpha3.Headers_HeaderOperations{
				Set: map[string]string{
					"Host": network.GetServiceHostname(backend, isvc.Namespace),
				},
			},
		},
	})
	hosts := []string{
		network.GetServiceHostname(isvc.Name, isvc.Namespace),
	}
	gateways := []string{
		config.LocalGateway,
	}
	if !isInternal {
		hosts = append(hosts, serviceHost)
		gateways = append(gateways, config.IngressGateway)
	}
	if config.PathTemplate != "" {
		path, err := GenerateUrlPath(isvc.Name, isvc.Namespace, config)
		if err != nil {
			log.Error(err, "Failed to generate URL from pathTemplate")
			return nil
		}
		url := &apis.URL{}
		url.Path = strings.TrimSuffix(path, "/") // remove trailing "/" if present
		url.Host = config.IngressDomain
		// In this case, we have a path-based URL so we add a path-based rule
		httpRoutes = append(httpRoutes, &istiov1alpha3.HTTPRoute{
			Match: []*istiov1alpha3.HTTPMatchRequest{
				{
					Uri: &istiov1alpha3.StringMatch{
						MatchType: &istiov1alpha3.StringMatch_Prefix{
							Prefix: url.Path + "/",
						},
					},
					Authority: &istiov1alpha3.StringMatch{
						MatchType: &istiov1alpha3.StringMatch_Regex{
							Regex: constants.HostRegExp(url.Host),
						},
					},
					Gateways: []string{config.IngressGateway},
				},
				{
					Uri: &istiov1alpha3.StringMatch{
						MatchType: &istiov1alpha3.StringMatch_Exact{
							Exact: url.Path,
						},
					},
					Authority: &istiov1alpha3.StringMatch{
						MatchType: &istiov1alpha3.StringMatch_Regex{
							Regex: constants.HostRegExp(url.Host),
						},
					},
					Gateways: []string{config.IngressGateway},
				},
			},
			Rewrite: &istiov1alpha3.HTTPRewrite{
				Uri: "/",
			},
			Route: []*istiov1alpha3.HTTPRouteDestination{
				createHTTPRouteDestination(config.LocalGatewayServiceName),
			},
			Headers: &istiov1alpha3.Headers{
				Request: &istiov1alpha3.Headers_HeaderOperations{
					Set: map[string]string{
						"Host": network.GetServiceHostname(backend, isvc.Namespace),
					},
				},
			},
		})
		// Include ingressDomain to the domains (both internal and external) derived by KNative
		hosts = append(hosts, url.Host)
	}

	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
	})
	desiredIngress := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        isvc.Name,
			Namespace:   isvc.Namespace,
			Annotations: annotations,
			Labels:      isvc.Labels,
		},
		Spec: istiov1alpha3.VirtualService{
			Hosts:    hosts,
			Gateways: gateways,
			Http:     httpRoutes,
		},
	}
	return desiredIngress
}

func (ir *IngressReconciler) Reconcile(isvc *v1beta1.InferenceService) error {
	serviceHost := getServiceHost(isvc)
	serviceUrl := getServiceUrl(isvc, ir.ingressConfig)
	disableIstioVirtualHost := ir.ingressConfig.DisableIstioVirtualHost
	if serviceHost == "" || serviceUrl == "" {
		return nil
	}
	// When Istio virtual host is disabled, we return the underlying component url.
	// When Istio virtual host is enabled. we return the url using inference service virtual host name and redirect to the corresponding transformer, predictor or explainer url.
	if disableIstioVirtualHost == false {
		defaultNameExisting := &knservingv1.Service{}
		useDefault := false
		err := ir.client.Get(context.TODO(), types.NamespacedName{Name: constants.DefaultPredictorServiceName(isvc.Name), Namespace: isvc.Namespace}, defaultNameExisting)
		if err == nil {
			useDefault = true
		}
		desiredIngress := createIngress(isvc, useDefault, ir.ingressConfig)
		if desiredIngress == nil {
			return nil
		}

		//Create external service which points to local gateway
		if err := ir.reconcileExternalService(isvc, ir.ingressConfig); err != nil {
			return errors.Wrapf(err, "fails to reconcile external name service")
		}

		if err := controllerutil.SetControllerReference(isvc, desiredIngress, ir.scheme); err != nil {
			return errors.Wrapf(err, "fails to set owner reference for ingress")
		}

		existing := &v1alpha3.VirtualService{}
		err = ir.client.Get(context.TODO(), types.NamespacedName{Name: desiredIngress.Name, Namespace: desiredIngress.Namespace}, existing)
		if err != nil {
			if apierr.IsNotFound(err) {
				log.Info("Creating Ingress for isvc", "namespace", desiredIngress.Namespace, "name", desiredIngress.Name)
				err = ir.client.Create(context.TODO(), desiredIngress)
			}
		} else {
			if !routeSemanticEquals(desiredIngress, existing) {
				existing.Spec = desiredIngress.Spec
				existing.Annotations = desiredIngress.Annotations
				existing.Labels = desiredIngress.Labels
				log.Info("Update Ingress for isvc", "namespace", desiredIngress.Namespace, "name", desiredIngress.Name)
				err = ir.client.Update(context.TODO(), existing)
			}
		}
		if err != nil {
			return errors.Wrapf(err, "fails to create or update ingress")
		}
	}

	if url, err := apis.ParseURL(serviceUrl); err == nil {
		isvc.Status.URL = url
		path := ""
		modelName := isvcutils.GetModelName(isvc)
		if isvc.Spec.Transformer != nil {
			// As of now transformer only supports protocol V1
			path = constants.PredictPath(modelName, constants.ProtocolV1)
		} else if !isvcutils.IsMMSPredictor(&isvc.Spec.Predictor) {

			protocol := isvc.Spec.Predictor.GetImplementation().GetProtocol()

			if protocol == constants.ProtocolV1 {
				path = constants.PredictPath(modelName, constants.ProtocolV1)
			} else if protocol == constants.ProtocolV2 {
				path = constants.PredictPath(modelName, constants.ProtocolV2)
			}

		}
		hostPrefix := getHostPrefix(isvc, disableIstioVirtualHost)
		isvc.Status.Address = &duckv1.Addressable{
			URL: &apis.URL{
				Host:   network.GetServiceHostname(hostPrefix, isvc.Namespace),
				Scheme: "http",
				Path:   path,
			},
		}
		isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionTrue,
		})
		return nil
	} else {
		return errors.Wrapf(err, "fails to parse service url")
	}
}

func routeSemanticEquals(desired, existing *v1alpha3.VirtualService) bool {
	return equality.Semantic.DeepEqual(desired.Spec, existing.Spec) &&
		equality.Semantic.DeepEqual(desired.ObjectMeta.Labels, existing.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desired.ObjectMeta.Annotations, existing.ObjectMeta.Annotations)
}

func getHostPrefix(isvc *v1beta1.InferenceService, disableIstioVirtualHost bool) string {
	if disableIstioVirtualHost == true {
		if isvc.Spec.Transformer != nil {
			return constants.DefaultTransformerServiceName(isvc.Name)
		}
		return constants.DefaultPredictorServiceName(isvc.Name)
	}
	return isvc.Name
}
