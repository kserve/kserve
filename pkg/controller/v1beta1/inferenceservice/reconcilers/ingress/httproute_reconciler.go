/*
Copyright 2024 The KServe Authors.

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
	"slices"
	"strings"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	knapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type RawHTTPRouteReconciler struct {
	client        client.Client
	scheme        *runtime.Scheme
	ingressConfig *v1beta1.IngressConfig
}

func NewRawHTTPRouteReconciler(client client.Client, scheme *runtime.Scheme, ingressConfig *v1beta1.IngressConfig) *RawHTTPRouteReconciler {
	return &RawHTTPRouteReconciler{
		client:        client,
		scheme:        scheme,
		ingressConfig: ingressConfig,
	}
}

func createRawURL(isvc *v1beta1.InferenceService,
	ingressConfig *v1beta1.IngressConfig) (*knapis.URL, error) {
	var err error
	url := &knapis.URL{}
	url.Scheme = ingressConfig.UrlScheme
	url.Host, err = GenerateDomainName(isvc.Name, isvc.ObjectMeta, ingressConfig)
	if err != nil {
		return nil, err
	}

	return url, nil
}

func getRawServiceHost(isvc *v1beta1.InferenceService, client client.Client) string {
	existingService := &corev1.Service{}
	if isvc.Spec.Transformer != nil {
		transformerName := constants.TransformerServiceName(isvc.Name)

		// Check if existing transformer service name has default suffix
		err := client.Get(context.TODO(), types.NamespacedName{Name: constants.DefaultTransformerServiceName(isvc.Name), Namespace: isvc.Namespace}, existingService)
		if err == nil {
			transformerName = constants.DefaultTransformerServiceName(isvc.Name)
		}
		return network.GetServiceHostname(transformerName, isvc.Namespace)
	}

	predictorName := constants.PredictorServiceName(isvc.Name)

	// Check if existing predictor service name has default suffix
	err := client.Get(context.TODO(), types.NamespacedName{Name: constants.DefaultPredictorServiceName(isvc.Name), Namespace: isvc.Namespace}, existingService)
	if err == nil {
		predictorName = constants.DefaultPredictorServiceName(isvc.Name)
	}
	return network.GetServiceHostname(predictorName, isvc.Namespace)
}

func createHTTPRouteMatch(prefix string, targetHosts, internalHosts []string, additionalHosts *[]string,
	isInternal bool) []gatewayapiv1.HTTPRouteMatch {
	var pathMatch *gatewayapiv1.HTTPPathMatch
	var httpRouteMatches []gatewayapiv1.HTTPRouteMatch
	if prefix != "" {
		pathMatch = &gatewayapiv1.HTTPPathMatch{
			Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
			Value: utils.ToPointer(prefix),
		}
	}
	for _, internalHost := range internalHosts {
		httpRouteMatches = append(httpRouteMatches, gatewayapiv1.HTTPRouteMatch{
			Path: pathMatch,
			Headers: []gatewayapiv1.HTTPHeaderMatch{
				{
					Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
					Name:  gatewayapiv1.HTTPHeaderName(constants.HostHeader),
					Value: constants.HostRegExp(internalHost),
				},
			},
		})
	}

	if !isInternal {
		// We only create the HTTPRouteMatch for the targetHosts and the additional hosts, when the ingress is not internal.
		for _, targetHost := range targetHosts {
			hostMatch := gatewayapiv1.HTTPHeaderMatch{
				Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
				Name:  gatewayapiv1.HTTPHeaderName(constants.HostHeader),
				Value: constants.HostRegExp(targetHost),
			}
			httpRouteMatches = append(httpRouteMatches, gatewayapiv1.HTTPRouteMatch{
				Path:    pathMatch,
				Headers: []gatewayapiv1.HTTPHeaderMatch{hostMatch},
			})
		}

		if additionalHosts != nil && len(*additionalHosts) > 0 {
			for _, host := range *additionalHosts {
				hostMatch := gatewayapiv1.HTTPHeaderMatch{
					Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
					Name:  gatewayapiv1.HTTPHeaderName(constants.HostHeader),
					Value: constants.HostRegExp(host),
				}
				if !slices.ContainsFunc(httpRouteMatches, func(routeMatch gatewayapiv1.HTTPRouteMatch) bool {
					return equality.Semantic.DeepEqual(routeMatch.Headers[0], hostMatch)
				}) {
					httpRouteMatches = append(httpRouteMatches, gatewayapiv1.HTTPRouteMatch{
						Path:    pathMatch,
						Headers: []gatewayapiv1.HTTPHeaderMatch{hostMatch},
					})
				}
			}
		}
	}
	return httpRouteMatches
}

func createHTTPRouteHostModifier(host string) gatewayapiv1.HTTPRouteFilter {
	return gatewayapiv1.HTTPRouteFilter{
		Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
		RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
			Set: []gatewayapiv1.HTTPHeader{
				{
					Name:  constants.HostHeader,
					Value: host,
				},
			},
		},
	}
}

func addIsvcHeaders(name string, namespace string) gatewayapiv1.HTTPRouteFilter {
	return gatewayapiv1.HTTPRouteFilter{
		Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
		RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
			Add: []gatewayapiv1.HTTPHeader{
				{
					Name:  constants.IsvcNameHeader,
					Value: name,
				},
				{
					Name:  constants.IsvcNamespaceHeader,
					Value: namespace,
				},
			},
		},
	}
}

func createHTTPRouteRule(routeMatches []gatewayapiv1.HTTPRouteMatch, filters []gatewayapiv1.HTTPRouteFilter,
	serviceName, namespace string, port int32) gatewayapiv1.HTTPRouteRule {
	var backendRefs []gatewayapiv1.HTTPBackendRef
	if serviceName != "" {
		backendRefs = []gatewayapiv1.HTTPBackendRef{
			{
				BackendRef: gatewayapiv1.BackendRef{
					BackendObjectReference: gatewayapiv1.BackendObjectReference{
						Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
						Name:      gatewayapiv1.ObjectName(serviceName),
						Namespace: (*gatewayapiv1.Namespace)(&namespace),
						Port:      (*gatewayapiv1.PortNumber)(&port),
					},
				},
			},
		}
	}
	return gatewayapiv1.HTTPRouteRule{
		Matches:     routeMatches,
		Filters:     filters,
		BackendRefs: backendRefs,
	}
}

func createRawHTTPRoute(isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig,
	client client.Client) (*gatewayapiv1.HTTPRoute, error) {
	var httpRouteRules []gatewayapiv1.HTTPRouteRule
	var allowedHosts []gatewayapiv1.Hostname
	additionalHosts := ingressConfig.AdditionalIngressDomains

	if !isvc.Status.IsConditionReady(v1beta1.PredictorReady) {
		isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionFalse,
			Reason: "Predictor ingress not created",
		})
		return nil, nil
	}

	existing := &corev1.Service{}
	predictorName := constants.PredictorServiceName(isvc.Name)
	transformerName := constants.TransformerServiceName(isvc.Name)
	explainerName := constants.ExplainerServiceName(isvc.Name)
	// Check if existing predictor service name has default suffix
	err := client.Get(context.TODO(), types.NamespacedName{Name: constants.DefaultPredictorServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
	if err == nil {
		// Found existing predictor service with default suffix
		predictorName = constants.DefaultPredictorServiceName(isvc.Name)
		transformerName = constants.DefaultTransformerServiceName(isvc.Name)
		explainerName = constants.DefaultExplainerServiceName(isvc.Name)

	}
	topLevelHost, err := GenerateDomainName(isvc.Name, isvc.ObjectMeta, ingressConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate top level ingress host: %w", err)
	}
	allowedHosts = append(allowedHosts, gatewayapiv1.Hostname(topLevelHost))
	// Add additional hosts to allowed hosts
	if additionalHosts != nil {
		for _, host := range *additionalHosts {
			allowedHosts = append(allowedHosts, gatewayapiv1.Hostname(host))
		}
	}
	// Add isvc name and namespace headers
	filters := []gatewayapiv1.HTTPRouteFilter{addIsvcHeaders(isvc.Name, isvc.Namespace)}

	if isvc.Spec.Explainer != nil {
		// Scenario: When explainer present
		if !isvc.Status.IsConditionReady(v1beta1.ExplainerReady) {
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionFalse,
				Reason: "Explainer ingress not created",
			})
			return nil, nil
		}
		explainerHost, err := GenerateDomainName(explainerName, isvc.ObjectMeta, ingressConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to generate explainer ingress host: %w", err)
		}
		allowedHosts = append(allowedHosts, gatewayapiv1.Hostname(explainerHost))

		// Add explainer host rules
		explainerRouteMatch := createHTTPRouteMatch(constants.FallbackPrefix(), []string{explainerHost}, nil, nil, false)
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(explainerRouteMatch, filters, explainerName, isvc.Namespace,
			constants.CommonDefaultHttpPort))

		// Add toplevel host :explain route
		// :explain routes to the explainer when there is only explainer
		explainRouteMatch := createHTTPRouteMatch(constants.ExplainPrefix(), []string{topLevelHost}, nil, additionalHosts, false)
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(explainRouteMatch, filters,
			explainerName, isvc.Namespace, constants.CommonDefaultHttpPort))
	}
	if isvc.Spec.Transformer != nil {
		// Scenario: When predictor with transformer and with/without explainer present
		if !isvc.Status.IsConditionReady(v1beta1.TransformerReady) {
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionFalse,
				Reason: "Transformer ingress not created",
			})
			return nil, nil
		}
		transformerHost, err := GenerateDomainName(transformerName, isvc.ObjectMeta, ingressConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to generate transformer ingress host: %w", err)
		}
		allowedHosts = append(allowedHosts, gatewayapiv1.Hostname(transformerHost))

		// :predict routes to the transformer when there are both predictor and transformer
		targetHosts := []string{topLevelHost, transformerHost}
		routeMatch := createHTTPRouteMatch(constants.FallbackPrefix(), targetHosts, nil, additionalHosts, false)
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, transformerName, isvc.Namespace, constants.CommonDefaultHttpPort))
	} else {
		// Scenario: When predictor without transformer and with/without explainer present
		if !isvc.Status.IsConditionReady(v1beta1.PredictorReady) {
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionFalse,
				Reason: "Predictor ingress not created",
			})
			return nil, nil
		}
		// Add toplevel host rules for predictor which routes all traffic to predictor
		routeMatch := createHTTPRouteMatch(constants.FallbackPrefix(), []string{topLevelHost}, nil, additionalHosts, false)
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, predictorName, isvc.Namespace, constants.CommonDefaultHttpPort))
	}
	// Add predictor host rules
	predictorHost, err := GenerateDomainName(predictorName, isvc.ObjectMeta, ingressConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate predictor ingress host: %w", err)
	}
	allowedHosts = append(allowedHosts, gatewayapiv1.Hostname(predictorHost))
	routeMatch := createHTTPRouteMatch(constants.FallbackPrefix(), []string{predictorHost}, nil, nil, false)
	httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, predictorName, isvc.Namespace, int32(constants.CommonDefaultHttpPort)))

	// Add path based routing rules
	if ingressConfig.PathTemplate != "" {
		path, err := GenerateUrlPath(isvc.Name, isvc.Namespace, ingressConfig)
		if err != nil {
			log.Error(err, "Failed to generate URL from pathTemplate")
			return nil, fmt.Errorf("failed to generate URL from pathTemplate: %w", err)
		}
		url := &apis.URL{}
		url.Path = strings.TrimSuffix(path, "/") // remove trailing "/" if present
		url.Host = ingressConfig.IngressDomain

		if isvc.Spec.Explainer != nil {
			// Add path based routing rule for :explain endpoint
			explainerPathRouteMatch := createHTTPRouteMatch(url.Path+constants.PathBasedExplainPrefix(), []string{url.Host}, nil, nil, false)
			httpRouteRules = append(httpRouteRules, createHTTPRouteRule(explainerPathRouteMatch, filters, explainerName, isvc.Namespace,
				constants.CommonDefaultHttpPort))
		}
		// Add path based routing rule for :predict endpoint
		if isvc.Spec.Transformer != nil {
			// :predict routes to the transformer when there are both predictor and transformer
			pathRouteMatch := createHTTPRouteMatch(url.Path+"/", []string{url.Host}, nil, nil, false)
			httpRouteRules = append(httpRouteRules, createHTTPRouteRule(pathRouteMatch, filters, transformerName, isvc.Namespace,
				int32(constants.CommonDefaultHttpPort)))
		} else {
			// :predict routes to the predictor when there is only predictor
			pathRouteMatch := createHTTPRouteMatch(url.Path+"/", []string{url.Host}, nil, nil, false)
			httpRouteRules = append(httpRouteRules, createHTTPRouteRule(pathRouteMatch, filters, predictorName, isvc.Namespace,
				int32(constants.CommonDefaultHttpPort)))
		}
		// Include ingressDomain to the allowed hosts
		allowedHosts = append(allowedHosts, gatewayapiv1.Hostname(url.Host))
	}

	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
	})
	gatewaySlice := strings.Split(ingressConfig.KserveIngressGateway, "/")
	httpRoute := gatewayapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        isvc.Name,
			Namespace:   isvc.Namespace,
			Annotations: annotations,
		},
		Spec: gatewayapiv1.HTTPRouteSpec{
			Hostnames: allowedHosts,
			Rules:     httpRouteRules,
			CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
				ParentRefs: []gatewayapiv1.ParentReference{
					{
						Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
						Kind:      (*gatewayapiv1.Kind)(utils.ToPointer(constants.GatewayKind)),
						Namespace: (*gatewayapiv1.Namespace)(&gatewaySlice[0]),
						Name:      gatewayapiv1.ObjectName(gatewaySlice[1]),
						Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
					},
				},
			},
		},
	}
	return &httpRoute, nil
}

func semanticHttpRouteEquals(desired, existing *gatewayapiv1.HTTPRoute) bool {
	return equality.Semantic.DeepEqual(desired.Spec, existing.Spec)
}

// isHTTPRouteReady checks if the HTTPRoute is ready. If not, returns the reason and message.
func isHTTPRouteReady(httpRouteStatus gatewayapiv1.HTTPRouteStatus) (bool, *string, *string) {
	if len(httpRouteStatus.Parents) == 0 {
		return false, utils.ToPointer("HttpRoute is not ready"), utils.ToPointer("Parent status is not available")
	}
	for _, parent := range httpRouteStatus.Parents {
		for _, condition := range parent.Conditions {
			if condition.Status == metav1.ConditionFalse {
				return false, &condition.Reason, &condition.Message
			}
		}
	}
	return true, nil, nil
}

// ReconcileHTTPRoute reconciles the HTTPRoute resource
func (r *RawHTTPRouteReconciler) Reconcile(ctx context.Context, isvc *v1beta1.InferenceService) error {
	var err error
	isInternal := false
	// disable ingress creation if service is labelled with cluster local or kserve domain is cluster local
	if val, ok := isvc.Labels[constants.NetworkVisibility]; ok && val == constants.ClusterLocalVisibility {
		isInternal = true
	}
	if r.ingressConfig.IngressDomain == constants.ClusterLocalDomain {
		isInternal = true
	}
	if !isInternal && !r.ingressConfig.DisableIngressCreation {
		httpRoute, err := createRawHTTPRoute(isvc, r.ingressConfig, r.client)
		if httpRoute == nil {
			return nil
		}
		if err != nil {
			return err
		}
		if err := controllerutil.SetControllerReference(isvc, httpRoute, r.scheme); err != nil {
			log.Error(err, "Failed to set controller reference for HttpRoute", "name", isvc.Name)
			return err
		}
		// reconcile httpRoute
		existingHttpRoute := &gatewayapiv1.HTTPRoute{}
		err = r.client.Get(context.TODO(), types.NamespacedName{
			Namespace: isvc.Namespace,
			Name:      isvc.Name,
		}, existingHttpRoute)
		if err != nil {
			if apierr.IsNotFound(err) {
				err = r.client.Create(context.TODO(), httpRoute)
				log.Info("Creating HttpRoute resource", "name", isvc.Name, "err", err)
			} else {
				return err
			}
		} else {
			// Set ResourceVersion which is required for update operation.
			httpRoute.ResourceVersion = existingHttpRoute.ResourceVersion
			// Do a dry-run update to avoid diffs generated by default values.
			// This will populate our local httpRoute with any default values that are present on the remote version.
			if err := r.client.Update(ctx, httpRoute, client.DryRunAll); err != nil {
				log.Error(err, "Failed to perform dry-run update on httpRoute", "name", httpRoute.Name)
				return err
			}
			if !semanticHttpRouteEquals(httpRoute, existingHttpRoute) {
				log.Info("Updating HttpRoute", "name", isvc.Name)
				if err = r.client.Update(ctx, httpRoute); err != nil {
					log.Error(err, "Failed to update HttpRoute", "name", isvc.Name)
					return err
				}
			}
		}
		if ready, reason, message := isHTTPRouteReady(httpRoute.Status); ready {
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionTrue,
			})
		} else {
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:    v1beta1.IngressReady,
				Status:  corev1.ConditionFalse,
				Reason:  *reason,
				Message: *message,
			})
		}
	} else {
		// Ingress creation is disabled. We set it to true as the isvc condition depends on it.
		isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionTrue,
		})
	}
	isvc.Status.URL, err = createRawURL(isvc, r.ingressConfig)
	if err != nil {
		return err
	}
	isvc.Status.Address = &duckv1.Addressable{
		URL: &apis.URL{
			Host:   getRawServiceHost(isvc, r.client),
			Scheme: r.ingressConfig.UrlScheme,
			Path:   "",
		},
	}
	return nil
}
