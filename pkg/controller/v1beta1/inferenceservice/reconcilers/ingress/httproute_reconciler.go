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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	knapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	HTTPRouteNotReady                 = "HttpRouteNotReady"
	HTTPRouteParentStatusNotAvailable = "ParentStatusNotAvailable"
)

var DefaultTimeout = toGatewayAPIDuration(60)

type RawHTTPRouteReconciler struct {
	client        client.Client
	scheme        *runtime.Scheme
	ingressConfig *v1beta1.IngressConfig
	isvcConfig    *v1beta1.InferenceServicesConfig
}

func NewRawHTTPRouteReconciler(client client.Client, scheme *runtime.Scheme, ingressConfig *v1beta1.IngressConfig,
	isvcConfig *v1beta1.InferenceServicesConfig,
) *RawHTTPRouteReconciler {
	return &RawHTTPRouteReconciler{
		client:        client,
		scheme:        scheme,
		ingressConfig: ingressConfig,
		isvcConfig:    isvcConfig,
	}
}

// toGatewayAPIDuration converts seconds to gatewayapiv1.Duration
func toGatewayAPIDuration(seconds int64) *gatewayapiv1.Duration {
	duration := gatewayapiv1.Duration(fmt.Sprintf("%ds", seconds))
	return &duration
}

func createRawURL(isvc *v1beta1.InferenceService,
	ingressConfig *v1beta1.IngressConfig,
) (*knapis.URL, error) {
	var err error
	url := &knapis.URL{}
	url.Scheme = ingressConfig.UrlScheme
	url.Host, err = GenerateDomainName(isvc.Name, isvc.ObjectMeta, ingressConfig)
	if err != nil {
		return nil, err
	}

	return url, nil
}

func getRawServiceHost(ctx context.Context, isvc *v1beta1.InferenceService, client client.Client) string {
	existingService := &corev1.Service{}
	if isvc.Spec.Transformer != nil {
		transformerName := constants.TransformerServiceName(isvc.Name)

		// Check if existing transformer service name has default suffix
		err := client.Get(ctx, types.NamespacedName{Name: constants.DefaultTransformerServiceName(isvc.Name), Namespace: isvc.Namespace}, existingService)
		if err == nil {
			transformerName = constants.DefaultTransformerServiceName(isvc.Name)
		}
		return network.GetServiceHostname(transformerName, isvc.Namespace)
	}

	predictorName := constants.PredictorServiceName(isvc.Name)

	// Check if existing predictor service name has default suffix
	err := client.Get(ctx, types.NamespacedName{Name: constants.DefaultPredictorServiceName(isvc.Name), Namespace: isvc.Namespace}, existingService)
	if err == nil {
		predictorName = constants.DefaultPredictorServiceName(isvc.Name)
	}
	return network.GetServiceHostname(predictorName, isvc.Namespace)
}

func createHTTPRouteMatch(prefix string) gatewayapiv1.HTTPRouteMatch {
	return gatewayapiv1.HTTPRouteMatch{
		Path: &gatewayapiv1.HTTPPathMatch{
			Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
			Value: ptr.To(prefix),
		},
	}
}

func addIsvcHeaders(name string, namespace string) gatewayapiv1.HTTPRouteFilter {
	return gatewayapiv1.HTTPRouteFilter{
		Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
		RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
			Set: []gatewayapiv1.HTTPHeader{
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
	serviceName, namespace string, port int32, timeout *gatewayapiv1.Duration,
) gatewayapiv1.HTTPRouteRule {
	var backendRefs []gatewayapiv1.HTTPBackendRef
	if serviceName != "" {
		backendRefs = []gatewayapiv1.HTTPBackendRef{
			{
				BackendRef: gatewayapiv1.BackendRef{
					BackendObjectReference: gatewayapiv1.BackendObjectReference{
						Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
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
		Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
			Request: timeout,
		},
	}
}

func createRawPredictorHTTPRoute(ctx context.Context, isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig, isvcConfig *v1beta1.InferenceServicesConfig,
	client client.Client,
) (*gatewayapiv1.HTTPRoute, error) {
	var httpRouteRules []gatewayapiv1.HTTPRouteRule
	var allowedHosts []gatewayapiv1.Hostname

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

	// Check if existing predictor service name has default suffix
	err := client.Get(ctx, types.NamespacedName{Name: constants.DefaultPredictorServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
	if err == nil {
		// Found existing predictor service with default suffix
		predictorName = constants.DefaultPredictorServiceName(isvc.Name)
	}

	// Add isvc name and namespace headers
	filters := []gatewayapiv1.HTTPRouteFilter{addIsvcHeaders(isvc.Name, isvc.Namespace)}

	// Add predictor host rules
	predictorHost, err := GenerateDomainName(predictorName, isvc.ObjectMeta, ingressConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate predictor ingress host: %w", err)
	}
	allowedHosts = append(allowedHosts, gatewayapiv1.Hostname(predictorHost))
	routeMatch := []gatewayapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
	timeout := DefaultTimeout
	if isvc.Spec.Predictor.TimeoutSeconds != nil {
		timeout = toGatewayAPIDuration(*isvc.Spec.Predictor.TimeoutSeconds)
	}
	httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, predictorName, isvc.Namespace, constants.CommonDefaultHttpPort, timeout))

	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceAnnotationDisallowedList, key)
	})
	labels := utils.Filter(isvc.Labels, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceLabelDisallowedList, key)
	})
	gatewaySlice := strings.Split(ingressConfig.KserveIngressGateway, "/")
	httpRoute := gatewayapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        constants.PredictorServiceName(isvc.Name),
			Namespace:   isvc.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: gatewayapiv1.HTTPRouteSpec{
			Hostnames: allowedHosts,
			Rules:     httpRouteRules,
			CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
				ParentRefs: []gatewayapiv1.ParentReference{
					{
						Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
						Kind:      (*gatewayapiv1.Kind)(ptr.To(constants.GatewayKind)),
						Namespace: (*gatewayapiv1.Namespace)(&gatewaySlice[0]),
						Name:      gatewayapiv1.ObjectName(gatewaySlice[1]),
					},
				},
			},
		},
	}
	return &httpRoute, nil
}

func createRawTransformerHTTPRoute(ctx context.Context, isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig, isvcConfig *v1beta1.InferenceServicesConfig,
	client client.Client,
) (*gatewayapiv1.HTTPRoute, error) {
	var httpRouteRules []gatewayapiv1.HTTPRouteRule
	var allowedHosts []gatewayapiv1.Hostname

	if !isvc.Status.IsConditionReady(v1beta1.TransformerReady) {
		isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionFalse,
			Reason: "Transformer ingress not created",
		})
		return nil, nil
	}

	existing := &corev1.Service{}
	transformerName := constants.TransformerServiceName(isvc.Name)

	// Check if existing transformer service name has default suffix
	err := client.Get(ctx, types.NamespacedName{Name: constants.DefaultTransformerServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
	if err == nil {
		// Found existing transformer service with default suffix
		transformerName = constants.DefaultTransformerServiceName(isvc.Name)
	}

	// Add isvc name and namespace headers
	filters := []gatewayapiv1.HTTPRouteFilter{addIsvcHeaders(isvc.Name, isvc.Namespace)}

	transformerHost, err := GenerateDomainName(transformerName, isvc.ObjectMeta, ingressConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate transformer ingress host: %w", err)
	}
	allowedHosts = append(allowedHosts, gatewayapiv1.Hostname(transformerHost))
	routeMatch := []gatewayapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
	timeout := DefaultTimeout
	if isvc.Spec.Transformer.TimeoutSeconds != nil {
		timeout = toGatewayAPIDuration(*isvc.Spec.Transformer.TimeoutSeconds)
	}
	httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, transformerName, isvc.Namespace,
		constants.CommonDefaultHttpPort, timeout))

	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceAnnotationDisallowedList, key)
	})
	labels := utils.Filter(isvc.Labels, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceLabelDisallowedList, key)
	})
	gatewaySlice := strings.Split(ingressConfig.KserveIngressGateway, "/")
	httpRoute := gatewayapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        constants.TransformerServiceName(isvc.Name),
			Namespace:   isvc.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: gatewayapiv1.HTTPRouteSpec{
			Hostnames: allowedHosts,
			Rules:     httpRouteRules,
			CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
				ParentRefs: []gatewayapiv1.ParentReference{
					{
						Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
						Kind:      (*gatewayapiv1.Kind)(ptr.To(constants.GatewayKind)),
						Namespace: (*gatewayapiv1.Namespace)(&gatewaySlice[0]),
						Name:      gatewayapiv1.ObjectName(gatewaySlice[1]),
					},
				},
			},
		},
	}
	return &httpRoute, nil
}

func createRawExplainerHTTPRoute(ctx context.Context, isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig, isvcConfig *v1beta1.InferenceServicesConfig,
	client client.Client,
) (*gatewayapiv1.HTTPRoute, error) {
	var httpRouteRules []gatewayapiv1.HTTPRouteRule
	var allowedHosts []gatewayapiv1.Hostname

	if !isvc.Status.IsConditionReady(v1beta1.ExplainerReady) {
		isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionFalse,
			Reason: "Explainer ingress not created",
		})
		return nil, nil
	}

	existing := &corev1.Service{}
	explainerName := constants.ExplainerServiceName(isvc.Name)

	// Check if existing explainer service name has default suffix
	err := client.Get(ctx, types.NamespacedName{Name: constants.DefaultExplainerServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
	if err == nil {
		// Found existing explainer service with default suffix
		explainerName = constants.DefaultExplainerServiceName(isvc.Name)
	}

	// Add isvc name and namespace headers
	filters := []gatewayapiv1.HTTPRouteFilter{addIsvcHeaders(isvc.Name, isvc.Namespace)}

	explainerHost, err := GenerateDomainName(explainerName, isvc.ObjectMeta, ingressConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate explainer ingress host: %w", err)
	}
	allowedHosts = append(allowedHosts, gatewayapiv1.Hostname(explainerHost))

	// Add explainer host rules
	routeMatch := []gatewayapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
	timeout := DefaultTimeout
	if isvc.Spec.Explainer.TimeoutSeconds != nil {
		timeout = toGatewayAPIDuration(*isvc.Spec.Explainer.TimeoutSeconds)
	}
	httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, explainerName, isvc.Namespace,
		constants.CommonDefaultHttpPort, timeout))

	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceAnnotationDisallowedList, key)
	})
	labels := utils.Filter(isvc.Labels, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceLabelDisallowedList, key)
	})
	gatewaySlice := strings.Split(ingressConfig.KserveIngressGateway, "/")
	httpRoute := gatewayapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        constants.ExplainerServiceName(isvc.Name),
			Namespace:   isvc.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: gatewayapiv1.HTTPRouteSpec{
			Hostnames: allowedHosts,
			Rules:     httpRouteRules,
			CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
				ParentRefs: []gatewayapiv1.ParentReference{
					{
						Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
						Kind:      (*gatewayapiv1.Kind)(ptr.To(constants.GatewayKind)),
						Namespace: (*gatewayapiv1.Namespace)(&gatewaySlice[0]),
						Name:      gatewayapiv1.ObjectName(gatewaySlice[1]),
					},
				},
			},
		},
	}
	return &httpRoute, nil
}

func createRawTopLevelHTTPRoute(ctx context.Context, isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig, isvcConfig *v1beta1.InferenceServicesConfig,
	client client.Client,
) (*gatewayapiv1.HTTPRoute, error) {
	var httpRouteRules []gatewayapiv1.HTTPRouteRule
	var allowedHosts []gatewayapiv1.Hostname

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
	err := client.Get(ctx, types.NamespacedName{Name: constants.DefaultPredictorServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
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
	domainList := []string{ingressConfig.IngressDomain}
	additionalHosts := GetAdditionalHosts(&domainList, topLevelHost, ingressConfig)
	// Add additional hosts to allowed hosts
	if additionalHosts != nil {
		hostMap := make(map[gatewayapiv1.Hostname]bool, len(allowedHosts))
		for _, host := range allowedHosts {
			hostMap[host] = true
		}
		for _, additionalHost := range *additionalHosts {
			gwHost := gatewayapiv1.Hostname(additionalHost)
			if _, found := hostMap[gwHost]; !found {
				allowedHosts = append(allowedHosts, gwHost)
			}
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
		timeout := DefaultTimeout
		if isvc.Spec.Explainer.TimeoutSeconds != nil {
			timeout = toGatewayAPIDuration(*isvc.Spec.Explainer.TimeoutSeconds)
		}
		// Add toplevel host :explain route
		// :explain routes to the explainer when there is only explainer
		explainRouteMatch := []gatewayapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.ExplainPrefix())}
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(explainRouteMatch, filters,
			explainerName, isvc.Namespace, constants.CommonDefaultHttpPort, timeout))
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
		timeout := DefaultTimeout
		if isvc.Spec.Transformer.TimeoutSeconds != nil {
			timeout = toGatewayAPIDuration(*isvc.Spec.Transformer.TimeoutSeconds)
		}
		// :predict routes to the transformer when there are both predictor and transformer
		routeMatch := []gatewayapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, transformerName, isvc.Namespace, constants.CommonDefaultHttpPort, timeout))
	} else {
		// Scenario: When predictor without transformer and with/without explainer present
		timeout := DefaultTimeout
		if isvc.Spec.Predictor.TimeoutSeconds != nil {
			timeout = toGatewayAPIDuration(*isvc.Spec.Predictor.TimeoutSeconds)
		}
		// Add toplevel host rules for predictor which routes all traffic to predictor
		routeMatch := []gatewayapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, predictorName, isvc.Namespace, constants.CommonDefaultHttpPort, timeout))
	}

	// Add path based routing rules
	if ingressConfig.PathTemplate != "" {
		path, err := GenerateUrlPath(isvc.Name, isvc.Namespace, ingressConfig)
		if err != nil {
			log.Error(err, "Failed to generate URL from pathTemplate")
			return nil, fmt.Errorf("failed to generate URL from pathTemplate: %w", err)
		}
		path = strings.TrimSuffix(path, "/") // remove trailing "/" if present
		// Include ingressDomain to the allowed hosts
		allowedHosts = append(allowedHosts, gatewayapiv1.Hostname(ingressConfig.IngressDomain))

		if isvc.Spec.Explainer != nil {
			timeout := DefaultTimeout
			if isvc.Spec.Explainer.TimeoutSeconds != nil {
				timeout = toGatewayAPIDuration(*isvc.Spec.Explainer.TimeoutSeconds)
			}
			// Add path based routing rule for :explain endpoint
			explainerPathRouteMatch := []gatewayapiv1.HTTPRouteMatch{createHTTPRouteMatch(path + constants.PathBasedExplainPrefix())}
			httpRouteRules = append(httpRouteRules, createHTTPRouteRule(explainerPathRouteMatch, filters, explainerName, isvc.Namespace,
				constants.CommonDefaultHttpPort, timeout))
		}
		// Add path based routing rule for :predict endpoint
		if isvc.Spec.Transformer != nil {
			timeout := DefaultTimeout
			if isvc.Spec.Transformer.TimeoutSeconds != nil {
				timeout = toGatewayAPIDuration(*isvc.Spec.Transformer.TimeoutSeconds)
			}
			// :predict routes to the transformer when there are both predictor and transformer
			pathRouteMatch := []gatewayapiv1.HTTPRouteMatch{createHTTPRouteMatch(path + "/")}
			httpRouteRules = append(httpRouteRules, createHTTPRouteRule(pathRouteMatch, filters, transformerName, isvc.Namespace,
				constants.CommonDefaultHttpPort, timeout))
		} else {
			timeout := DefaultTimeout
			if isvc.Spec.Predictor.TimeoutSeconds != nil {
				timeout = toGatewayAPIDuration(*isvc.Spec.Predictor.TimeoutSeconds)
			}
			// :predict routes to the predictor when there is only predictor
			pathRouteMatch := []gatewayapiv1.HTTPRouteMatch{createHTTPRouteMatch(path + "/")}
			httpRouteRules = append(httpRouteRules, createHTTPRouteRule(pathRouteMatch, filters, predictorName, isvc.Namespace,
				constants.CommonDefaultHttpPort, timeout))
		}
	}

	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceAnnotationDisallowedList, key)
	})
	labels := utils.Filter(isvc.Labels, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceLabelDisallowedList, key)
	})
	gatewaySlice := strings.Split(ingressConfig.KserveIngressGateway, "/")
	httpRoute := gatewayapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        isvc.Name,
			Namespace:   isvc.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: gatewayapiv1.HTTPRouteSpec{
			Hostnames: allowedHosts,
			Rules:     httpRouteRules,
			CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
				ParentRefs: []gatewayapiv1.ParentReference{
					{
						Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
						Kind:      (*gatewayapiv1.Kind)(ptr.To(constants.GatewayKind)),
						Namespace: (*gatewayapiv1.Namespace)(&gatewaySlice[0]),
						Name:      gatewayapiv1.ObjectName(gatewaySlice[1]),
					},
				},
			},
		},
	}
	return &httpRoute, nil
}

func semanticHttpRouteEquals(desired, existing *gatewayapiv1.HTTPRoute) bool {
	return equality.Semantic.DeepDerivative(desired.Spec, existing.Spec)
}

// isHTTPRouteReady checks if the HTTPRoute is ready. If not, returns the reason and message.
func isHTTPRouteReady(httpRouteStatus gatewayapiv1.HTTPRouteStatus) (bool, *string, *string) {
	if len(httpRouteStatus.Parents) == 0 {
		return false, ptr.To(HTTPRouteParentStatusNotAvailable), ptr.To(HTTPRouteNotReady)
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

func (r *RawHTTPRouteReconciler) reconcilePredictorHTTPRoute(ctx context.Context, isvc *v1beta1.InferenceService) error {
	desired, err := createRawPredictorHTTPRoute(ctx, isvc, r.ingressConfig, r.isvcConfig, r.client)
	if err != nil {
		return err
	}

	// reconcile httpRoute
	httpRouteName := constants.PredictorServiceName(isvc.Name)
	existingHttpRoute := &gatewayapiv1.HTTPRoute{}
	getExistingErr := r.client.Get(ctx, types.NamespacedName{
		Namespace: isvc.Namespace,
		Name:      httpRouteName,
	}, existingHttpRoute)
	httpRouteIsNotFound := apierr.IsNotFound(getExistingErr)
	if getExistingErr != nil && !httpRouteIsNotFound {
		return fmt.Errorf("failed to get existing http route: %w", getExistingErr)
	}

	// ISVC is stopped, delete the httproute if it exists, otherwise, do nothing
	forceStopRuntime := utils.GetForceStopRuntime(isvc)
	if (getExistingErr != nil && httpRouteIsNotFound) && forceStopRuntime {
		return nil
	}
	if forceStopRuntime {
		if ctrl := metav1.GetControllerOf(existingHttpRoute); ctrl != nil && ctrl.UID == isvc.UID {
			log.Info("The InferenceService is marked as stopped — deleting its associated http route", "name", httpRouteName)
			if err := r.client.Delete(ctx, existingHttpRoute); err != nil {
				return fmt.Errorf("failed to delete HTTPRoute %s/%s: %w", desired.Namespace, desired.Name, err)
			}
		}
		return nil
	}

	// Create or update the httproute to match the desired state
	if desired == nil {
		return nil
	}

	if err := controllerutil.SetControllerReference(isvc, desired, r.scheme); err != nil {
		log.Error(err, "Failed to set controller reference for predictor HttpRoute", "name", httpRouteName)
		return err
	}

	if getExistingErr != nil && httpRouteIsNotFound {
		log.Info("Creating Predictor HttpRoute resource", "name", httpRouteName)
		if err := r.client.Create(ctx, desired); err != nil {
			log.Error(err, "Failed to create predictor HttpRoute", "name", desired.Name)
			return err
		}
		return nil
	}

	// Set ResourceVersion which is required for update operation.
	desired.ResourceVersion = existingHttpRoute.ResourceVersion
	if !semanticHttpRouteEquals(desired, existingHttpRoute) {
		if err = r.client.Update(ctx, desired); err != nil {
			log.Error(err, "Failed to update predictor HttpRoute", "name", isvc.Name)
			return err
		}
	}
	return nil
}

func (r *RawHTTPRouteReconciler) reconcileTransformerHTTPRoute(ctx context.Context, isvc *v1beta1.InferenceService) error {
	desired, err := createRawTransformerHTTPRoute(ctx, isvc, r.ingressConfig, r.isvcConfig, r.client)
	if err != nil {
		return err
	}
	if desired == nil {
		return nil
	}
	if err := controllerutil.SetControllerReference(isvc, desired, r.scheme); err != nil {
		log.Error(err, "Failed to set controller reference for transformer HttpRoute", "name", desired.Name)
	}
	existing := &gatewayapiv1.HTTPRoute{}
	err = r.client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: isvc.Namespace}, existing)
	if err != nil {
		if apierr.IsNotFound(err) {
			log.Info("Creating transformer HttpRoute resource", "name", desired.Name)
			if err := r.client.Create(ctx, desired); err != nil {
				log.Error(err, "Failed to create transformer HttpRoute", "name", desired.Name)
				return err
			}
		} else {
			return err
		}
	} else {
		// Set ResourceVersion which is required for update operation.
		desired.ResourceVersion = existing.ResourceVersion
		// Do a dry-run update to avoid diffs generated by default values.
		// This will populate our local httpRoute with any default values that are present on the remote version.
		if err := r.client.Update(ctx, desired, client.DryRunAll); err != nil {
			log.Error(err, "Failed to perform dry-run update for transformer HttpRoute", "name", desired.Name)
			return err
		}
		if !semanticHttpRouteEquals(desired, existing) {
			if err := r.client.Update(ctx, desired); err != nil {
				log.Error(err, "Failed to update transformer HttpRoute", "name", desired.Name)
			}
		}
	}
	return nil
}

func (r *RawHTTPRouteReconciler) reconcileExplainerHTTPRoute(ctx context.Context, isvc *v1beta1.InferenceService) error {
	desired, err := createRawExplainerHTTPRoute(ctx, isvc, r.ingressConfig, r.isvcConfig, r.client)
	if err != nil {
		return err
	}
	if desired == nil {
		return nil
	}
	if err := controllerutil.SetControllerReference(isvc, desired, r.scheme); err != nil {
		log.Error(err, "Failed to set controller reference for explainer HttpRoute", "name", desired.Name)
	}
	existing := &gatewayapiv1.HTTPRoute{}
	err = r.client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: isvc.Namespace}, existing)
	if err != nil {
		if apierr.IsNotFound(err) {
			log.Info("Creating explainer HttpRoute resource", "name", desired.Name)
			if err := r.client.Create(ctx, desired); err != nil {
				log.Error(err, "Failed to create explainer HttpRoute", "name", desired.Name)
				return err
			}
		} else {
			return err
		}
	} else {
		// Set ResourceVersion which is required for update operation.
		desired.ResourceVersion = existing.ResourceVersion
		// Do a dry-run update to avoid diffs generated by default values.
		// This will populate our local httpRoute with any default values that are present on the remote version.
		if err := r.client.Update(ctx, desired, client.DryRunAll); err != nil {
			log.Error(err, "Failed to perform dry-run update for explainer HttpRoute", "name", desired.Name)
			return err
		}
		if !semanticHttpRouteEquals(desired, existing) {
			if err := r.client.Update(ctx, desired); err != nil {
				log.Error(err, "Failed to update explainer HttpRoute", "name", desired.Name)
			}
		}
	}
	return nil
}

func (r *RawHTTPRouteReconciler) reconcileTopLevelHTTPRoute(ctx context.Context, isvc *v1beta1.InferenceService) error {
	desired, err := createRawTopLevelHTTPRoute(ctx, isvc, r.ingressConfig, r.isvcConfig, r.client)
	if err != nil {
		return err
	}

	// reconcile httpRoute
	existingHttpRoute := &gatewayapiv1.HTTPRoute{}
	getExistingErr := r.client.Get(ctx, types.NamespacedName{
		Namespace: isvc.Namespace,
		Name:      isvc.Name,
	}, existingHttpRoute)
	httpRouteIsNotFound := apierr.IsNotFound(getExistingErr)
	if getExistingErr != nil && !httpRouteIsNotFound {
		return fmt.Errorf("failed to get existing http route: %w", getExistingErr)
	}

	// ISVC is stopped, delete the httproute if it exists, otherwise, do nothing
	forceStopRuntime := utils.GetForceStopRuntime(isvc)
	if (getExistingErr != nil && apierr.IsNotFound(getExistingErr)) && forceStopRuntime {
		return nil
	}
	if forceStopRuntime {
		if ctrl := metav1.GetControllerOf(existingHttpRoute); ctrl != nil && ctrl.UID == isvc.UID {
			log.Info("The InferenceService is marked as stopped — deleting its associated top level http route", "name", isvc.Name)
			if err := r.client.Delete(ctx, existingHttpRoute); err != nil {
				return fmt.Errorf("failed to delete HTTPRoute %s/%s: %w", desired.Namespace, desired.Name, err)
			}
		}
		return nil
	}

	// Create or update the httproute to match the desired state
	if desired == nil {
		return nil
	}

	if err := controllerutil.SetControllerReference(isvc, desired, r.scheme); err != nil {
		log.Error(err, "Failed to set controller reference for top level HttpRoute", "name", isvc.Name)
		return fmt.Errorf("failed to set controller reference for top level HttpRoute: %w", err)
	}

	if getExistingErr != nil && httpRouteIsNotFound {
		log.Info("Creating top level HttpRoute resource", "name", isvc.Name)
		if err := r.client.Create(ctx, desired); err != nil {
			log.Error(err, "Failed to create top level HttpRoute", "name", desired.Name)
			return fmt.Errorf("failed to create top level HttpRoute: %w", err)
		}
		return nil
	}

	// Set ResourceVersion which is required for update operation.
	desired.ResourceVersion = existingHttpRoute.ResourceVersion
	if !semanticHttpRouteEquals(desired, existingHttpRoute) {
		if err = r.client.Update(ctx, desired); err != nil {
			log.Error(err, "Failed to update toplevel HttpRoute", "name", isvc.Name)
			return fmt.Errorf("failed to update toplevel HttpRoute: %w", err)
		}
	}
	return nil
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
		if err := r.reconcilePredictorHTTPRoute(ctx, isvc); err != nil {
			return err
		}
		if isvc.Spec.Transformer != nil {
			if err := r.reconcileTransformerHTTPRoute(ctx, isvc); err != nil {
				return err
			}
		}
		if isvc.Spec.Explainer != nil {
			if err := r.reconcileExplainerHTTPRoute(ctx, isvc); err != nil {
				return err
			}
		}
		if err := r.reconcileTopLevelHTTPRoute(ctx, isvc); err != nil {
			return err
		}

		if utils.GetForceStopRuntime(isvc) {
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionFalse,
				Reason: v1beta1.StoppedISVCReason,
			})

			return nil
		}

		// Check Predictor HTTPRoute status
		httpRoute := &gatewayapiv1.HTTPRoute{}
		if err := r.client.Get(ctx, types.NamespacedName{
			Name:      constants.PredictorServiceName(isvc.Name),
			Namespace: isvc.Namespace,
		}, httpRoute); err != nil {
			return err
		}
		if ready, reason, message := isHTTPRouteReady(httpRoute.Status); !ready {
			log.Info("Predictor HTTPRoute not ready", "reason", *reason, "message", *message)
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:    v1beta1.IngressReady,
				Status:  corev1.ConditionFalse,
				Reason:  *reason,
				Message: fmt.Sprintf("%s %s", "Predictor", *message),
			})
			return nil
		}
		// Check Transformer HTTPRoute stauts
		if isvc.Spec.Transformer != nil {
			httpRoute = &gatewayapiv1.HTTPRoute{}
			if err := r.client.Get(ctx, types.NamespacedName{
				Name:      constants.TransformerServiceName(isvc.Name),
				Namespace: isvc.Namespace,
			}, httpRoute); err != nil {
				return err
			}
			if ready, reason, message := isHTTPRouteReady(httpRoute.Status); !ready {
				isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
					Type:    v1beta1.IngressReady,
					Status:  corev1.ConditionFalse,
					Reason:  *reason,
					Message: fmt.Sprintf("%s %s", "Transformer", *message),
				})
				return nil
			}
		}
		// Check Explainer HTTPRoute stauts
		if isvc.Spec.Explainer != nil {
			httpRoute = &gatewayapiv1.HTTPRoute{}
			if err := r.client.Get(ctx, types.NamespacedName{
				Name:      constants.ExplainerServiceName(isvc.Name),
				Namespace: isvc.Namespace,
			}, httpRoute); err != nil {
				return err
			}
			if ready, reason, message := isHTTPRouteReady(httpRoute.Status); !ready {
				isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
					Type:    v1beta1.IngressReady,
					Status:  corev1.ConditionFalse,
					Reason:  *reason,
					Message: fmt.Sprintf("%s %s", "Explainer", *message),
				})
				return nil
			}
		}
		// Check Top level HTTPRoute status
		httpRoute = &gatewayapiv1.HTTPRoute{}
		if err := r.client.Get(ctx, types.NamespacedName{
			Name:      isvc.Name,
			Namespace: isvc.Namespace,
		}, httpRoute); err != nil {
			return err
		}
		if ready, reason, message := isHTTPRouteReady(httpRoute.Status); !ready {
			log.Info("Top level HTTPRoute not ready", "reason", *reason, "message", *message)
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:    v1beta1.IngressReady,
				Status:  corev1.ConditionFalse,
				Reason:  *reason,
				Message: fmt.Sprintf("%s %s", "TopLevel", *message),
			})
			return nil
		}
		// If we are here, then all the HTTPRoutes are ready, Mark ingress as ready
		isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionTrue,
		})
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
			Host:   getRawServiceHost(ctx, isvc, r.client),
			Scheme: r.ingressConfig.UrlScheme,
			Path:   "",
		},
	}
	return nil
}
