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
	knapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/network"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

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
func toGatewayAPIDuration(seconds int64) *gwapiv1.Duration {
	duration := gwapiv1.Duration(fmt.Sprintf("%ds", seconds))
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

func getRawServiceHost(isvc *v1beta1.InferenceService) string {
	if isvc.Spec.Transformer != nil {
		transformerName := constants.TransformerServiceName(isvc.Name)
		return network.GetServiceHostname(transformerName, isvc.Namespace)
	}
	predictorName := constants.PredictorServiceName(isvc.Name)
	return network.GetServiceHostname(predictorName, isvc.Namespace)
}

func createHTTPRouteMatch(prefix string) gwapiv1.HTTPRouteMatch {
	return gwapiv1.HTTPRouteMatch{
		Path: &gwapiv1.HTTPPathMatch{
			Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
			Value: ptr.To(prefix),
		},
	}
}

func addIsvcHeaders(name string, namespace string) gwapiv1.HTTPRouteFilter {
	return gwapiv1.HTTPRouteFilter{
		Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
		RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
			Set: []gwapiv1.HTTPHeader{
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

func createHTTPRouteRule(routeMatches []gwapiv1.HTTPRouteMatch, filters []gwapiv1.HTTPRouteFilter,
	serviceName, namespace string, port int32, timeout *gwapiv1.Duration,
) gwapiv1.HTTPRouteRule {
	var backendRefs []gwapiv1.HTTPBackendRef
	if serviceName != "" {
		backendRefs = []gwapiv1.HTTPBackendRef{
			{
				BackendRef: gwapiv1.BackendRef{
					BackendObjectReference: gwapiv1.BackendObjectReference{
						Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
						Name:      gwapiv1.ObjectName(serviceName),
						Namespace: (*gwapiv1.Namespace)(&namespace),
						Port:      (*gwapiv1.PortNumber)(&port),
					},
				},
			},
		}
	}
	return gwapiv1.HTTPRouteRule{
		Matches:     routeMatches,
		Filters:     filters,
		BackendRefs: backendRefs,
		Timeouts: &gwapiv1.HTTPRouteTimeouts{
			Request: timeout,
		},
	}
}

func createRawPredictorHTTPRoute(isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig,
	isvcConfig *v1beta1.InferenceServicesConfig,
) (*gwapiv1.HTTPRoute, error) {
	var httpRouteRules []gwapiv1.HTTPRouteRule
	var allowedHosts []gwapiv1.Hostname

	if !isvc.Status.IsConditionReady(v1beta1.PredictorReady) {
		isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionFalse,
			Reason: "Predictor ingress not created",
		})
		return nil, nil
	}
	predictorName := constants.PredictorServiceName(isvc.Name)

	// Add isvc name and namespace headers
	filters := []gwapiv1.HTTPRouteFilter{addIsvcHeaders(isvc.Name, isvc.Namespace)}

	// Add predictor host rules
	predictorHost, err := GenerateDomainName(predictorName, isvc.ObjectMeta, ingressConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate predictor ingress host: %w", err)
	}
	allowedHosts = append(allowedHosts, gwapiv1.Hostname(predictorHost))
	routeMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
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
	httpRoute := gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        constants.PredictorServiceName(isvc.Name),
			Namespace:   isvc.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: gwapiv1.HTTPRouteSpec{
			Hostnames: allowedHosts,
			Rules:     httpRouteRules,
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{
					{
						Group:     (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
						Kind:      (*gwapiv1.Kind)(ptr.To(constants.GatewayKind)),
						Namespace: (*gwapiv1.Namespace)(&gatewaySlice[0]),
						Name:      gwapiv1.ObjectName(gatewaySlice[1]),
					},
				},
			},
		},
	}
	return &httpRoute, nil
}

func createRawTransformerHTTPRoute(isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig,
	isvcConfig *v1beta1.InferenceServicesConfig,
) (*gwapiv1.HTTPRoute, error) {
	var httpRouteRules []gwapiv1.HTTPRouteRule
	var allowedHosts []gwapiv1.Hostname

	if !isvc.Status.IsConditionReady(v1beta1.TransformerReady) {
		isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionFalse,
			Reason: "Transformer ingress not created",
		})
		return nil, nil
	}
	transformerName := constants.TransformerServiceName(isvc.Name)

	// Add isvc name and namespace headers
	filters := []gwapiv1.HTTPRouteFilter{addIsvcHeaders(isvc.Name, isvc.Namespace)}

	transformerHost, err := GenerateDomainName(transformerName, isvc.ObjectMeta, ingressConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate transformer ingress host: %w", err)
	}
	allowedHosts = append(allowedHosts, gwapiv1.Hostname(transformerHost))
	routeMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
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
	httpRoute := gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        constants.TransformerServiceName(isvc.Name),
			Namespace:   isvc.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: gwapiv1.HTTPRouteSpec{
			Hostnames: allowedHosts,
			Rules:     httpRouteRules,
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{
					{
						Group:     (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
						Kind:      (*gwapiv1.Kind)(ptr.To(constants.GatewayKind)),
						Namespace: (*gwapiv1.Namespace)(&gatewaySlice[0]),
						Name:      gwapiv1.ObjectName(gatewaySlice[1]),
					},
				},
			},
		},
	}
	return &httpRoute, nil
}

func createRawExplainerHTTPRoute(isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig,
	isvcConfig *v1beta1.InferenceServicesConfig,
) (*gwapiv1.HTTPRoute, error) {
	var httpRouteRules []gwapiv1.HTTPRouteRule
	var allowedHosts []gwapiv1.Hostname

	if !isvc.Status.IsConditionReady(v1beta1.ExplainerReady) {
		isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionFalse,
			Reason: "Explainer ingress not created",
		})
		return nil, nil
	}
	explainerName := constants.ExplainerServiceName(isvc.Name)

	// Add isvc name and namespace headers
	filters := []gwapiv1.HTTPRouteFilter{addIsvcHeaders(isvc.Name, isvc.Namespace)}

	explainerHost, err := GenerateDomainName(explainerName, isvc.ObjectMeta, ingressConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate explainer ingress host: %w", err)
	}
	allowedHosts = append(allowedHosts, gwapiv1.Hostname(explainerHost))

	// Add explainer host rules
	routeMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
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
	httpRoute := gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        constants.ExplainerServiceName(isvc.Name),
			Namespace:   isvc.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: gwapiv1.HTTPRouteSpec{
			Hostnames: allowedHosts,
			Rules:     httpRouteRules,
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{
					{
						Group:     (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
						Kind:      (*gwapiv1.Kind)(ptr.To(constants.GatewayKind)),
						Namespace: (*gwapiv1.Namespace)(&gatewaySlice[0]),
						Name:      gwapiv1.ObjectName(gatewaySlice[1]),
					},
				},
			},
		},
	}
	return &httpRoute, nil
}

func createRawTopLevelHTTPRoute(isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig,
	isvcConfig *v1beta1.InferenceServicesConfig,
) (*gwapiv1.HTTPRoute, error) {
	var httpRouteRules []gwapiv1.HTTPRouteRule
	var allowedHosts []gwapiv1.Hostname

	if !isvc.Status.IsConditionReady(v1beta1.PredictorReady) {
		isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionFalse,
			Reason: "Predictor ingress not created",
		})
		return nil, nil
	}
	predictorName := constants.PredictorServiceName(isvc.Name)
	transformerName := constants.TransformerServiceName(isvc.Name)
	explainerName := constants.ExplainerServiceName(isvc.Name)

	topLevelHost, err := GenerateDomainName(isvc.Name, isvc.ObjectMeta, ingressConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate top level ingress host: %w", err)
	}
	allowedHosts = append(allowedHosts, gwapiv1.Hostname(topLevelHost))
	domainList := []string{ingressConfig.IngressDomain}
	additionalHosts := GetAdditionalHosts(&domainList, topLevelHost, ingressConfig)
	// Add additional hosts to allowed hosts
	if additionalHosts != nil {
		hostMap := make(map[gwapiv1.Hostname]bool, len(allowedHosts))
		for _, host := range allowedHosts {
			hostMap[host] = true
		}
		for _, additionalHost := range *additionalHosts {
			gwHost := gwapiv1.Hostname(additionalHost)
			if _, found := hostMap[gwHost]; !found {
				allowedHosts = append(allowedHosts, gwHost)
			}
		}
	}
	// Add isvc name and namespace headers
	filters := []gwapiv1.HTTPRouteFilter{addIsvcHeaders(isvc.Name, isvc.Namespace)}

	if isvc.Spec.Explainer != nil {
		// Scenario: When explainer present
		if !isvc.Status.IsConditionReady(v1beta1.ExplainerReady) {
			isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
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
		explainRouteMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.ExplainPrefix())}
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(explainRouteMatch, filters,
			explainerName, isvc.Namespace, constants.CommonDefaultHttpPort, timeout))
	}
	if isvc.Spec.Transformer != nil {
		// Scenario: When predictor with transformer and with/without explainer present
		if !isvc.Status.IsConditionReady(v1beta1.TransformerReady) {
			isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
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
		routeMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, transformerName, isvc.Namespace, constants.CommonDefaultHttpPort, timeout))
	} else {
		// Scenario: When predictor without transformer and with/without explainer present
		timeout := DefaultTimeout
		if isvc.Spec.Predictor.TimeoutSeconds != nil {
			timeout = toGatewayAPIDuration(*isvc.Spec.Predictor.TimeoutSeconds)
		}
		// Add toplevel host rules for predictor which routes all traffic to predictor
		routeMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
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
		allowedHosts = append(allowedHosts, gwapiv1.Hostname(ingressConfig.IngressDomain))

		if isvc.Spec.Explainer != nil {
			timeout := DefaultTimeout
			if isvc.Spec.Explainer.TimeoutSeconds != nil {
				timeout = toGatewayAPIDuration(*isvc.Spec.Explainer.TimeoutSeconds)
			}
			// Add path based routing rule for :explain endpoint
			explainerPathRouteMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(path + constants.PathBasedExplainPrefix())}
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
			pathRouteMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(path + "/")}
			httpRouteRules = append(httpRouteRules, createHTTPRouteRule(pathRouteMatch, filters, transformerName, isvc.Namespace,
				constants.CommonDefaultHttpPort, timeout))
		} else {
			timeout := DefaultTimeout
			if isvc.Spec.Predictor.TimeoutSeconds != nil {
				timeout = toGatewayAPIDuration(*isvc.Spec.Predictor.TimeoutSeconds)
			}
			// :predict routes to the predictor when there is only predictor
			pathRouteMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(path + "/")}
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
	httpRoute := gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        isvc.Name,
			Namespace:   isvc.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: gwapiv1.HTTPRouteSpec{
			Hostnames: allowedHosts,
			Rules:     httpRouteRules,
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{
					{
						Group:     (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
						Kind:      (*gwapiv1.Kind)(ptr.To(constants.GatewayKind)),
						Namespace: (*gwapiv1.Namespace)(&gatewaySlice[0]),
						Name:      gwapiv1.ObjectName(gatewaySlice[1]),
					},
				},
			},
		},
	}
	return &httpRoute, nil
}

func semanticHttpRouteEquals(desired, existing *gwapiv1.HTTPRoute) bool {
	return equality.Semantic.DeepDerivative(desired.Spec, existing.Spec) &&
		equality.Semantic.DeepDerivative(desired.Labels, existing.Labels) &&
		equality.Semantic.DeepDerivative(desired.Annotations, existing.Annotations)
}

// isHTTPRouteReady checks if the HTTPRoute is ready. If not, returns the reason and message.
func isHTTPRouteReady(httpRouteStatus gwapiv1.HTTPRouteStatus) (bool, *string, *string) {
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
	desired, err := createRawPredictorHTTPRoute(isvc, r.ingressConfig, r.isvcConfig)
	if err != nil {
		return err
	}

	// reconcile httpRoute
	httpRouteName := constants.PredictorServiceName(isvc.Name)
	existingHttpRoute := &gwapiv1.HTTPRoute{}
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
			log.Error(err, "Failed to update predictor HttpRoute", "name", desired.Name)
			return err
		}
	}
	return nil
}

func (r *RawHTTPRouteReconciler) reconcileTransformerHTTPRoute(ctx context.Context, isvc *v1beta1.InferenceService) error {
	desired, err := createRawTransformerHTTPRoute(isvc, r.ingressConfig, r.isvcConfig)
	if err != nil {
		return err
	}

	// reconcile httpRoute
	httpRouteName := constants.TransformerServiceName(isvc.Name)
	existingHttpRoute := &gwapiv1.HTTPRoute{}
	getExistingErr := r.client.Get(ctx, types.NamespacedName{
		Name: httpRouteName, Namespace: isvc.Namespace,
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
		log.Error(err, "Failed to set controller reference for transformer HttpRoute", "name", desired.Name)
		return err
	}

	if getExistingErr != nil && httpRouteIsNotFound {
		log.Info("Creating transformer HttpRoute resource", "name", desired.Name)
		if err := r.client.Create(ctx, desired); err != nil {
			log.Error(err, "Failed to create transformer HttpRoute", "name", desired.Name)
			return err
		}
		return nil
	}
	// Set ResourceVersion which is required for update operation.
	desired.ResourceVersion = existingHttpRoute.ResourceVersion
	if !semanticHttpRouteEquals(desired, existingHttpRoute) {
		if err := r.client.Update(ctx, desired); err != nil {
			log.Error(err, "Failed to update transformer HttpRoute", "name", desired.Name)
			return err
		}
	}
	return nil
}

func (r *RawHTTPRouteReconciler) reconcileExplainerHTTPRoute(ctx context.Context, isvc *v1beta1.InferenceService) error {
	desired, err := createRawExplainerHTTPRoute(isvc, r.ingressConfig, r.isvcConfig)
	if err != nil {
		return err
	}

	// reconcile httproute
	httpRouteName := constants.ExplainerServiceName(isvc.Name)
	existingHttpRoute := &gatewayapiv1.HTTPRoute{}
	getExistingErr := r.client.Get(ctx, types.NamespacedName{
		Name: httpRouteName, Namespace: isvc.Namespace,
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
				return fmt.Errorf("failed to delete HTTPRoute %s/%s: %w", isvc.Namespace, httpRouteName, err)
			}
		}
		return nil
	}

	// Create or update the httproute to match the desired state
	if desired == nil {
		return nil
	}
	if err := controllerutil.SetControllerReference(isvc, desired, r.scheme); err != nil {
		log.Error(err, "Failed to set controller reference for explainer HttpRoute", "name", desired.Name)
	}

	if getExistingErr != nil && httpRouteIsNotFound {
		log.Info("Creating explainer HttpRoute resource", "name", desired.Name)
		if err := r.client.Create(ctx, desired); err != nil {
			log.Error(err, "Failed to create explainer HttpRoute", "name", desired.Name)
			return err
		}
		return nil
	}

	// Set ResourceVersion which is required for update operation.
	desired.ResourceVersion = existingHttpRoute.ResourceVersion
	if !semanticHttpRouteEquals(desired, existingHttpRoute) {
		if err := r.client.Update(ctx, desired); err != nil {
			log.Error(err, "Failed to update explainer HttpRoute", "name", desired.Name)
			return err
		}
	}
	return nil
}

func (r *RawHTTPRouteReconciler) reconcileTopLevelHTTPRoute(ctx context.Context, isvc *v1beta1.InferenceService) error {
	desired, err := createRawTopLevelHTTPRoute(isvc, r.ingressConfig, r.isvcConfig)
	if err != nil {
		return err
	}

	// reconcile httpRoute
	existingHttpRoute := &gwapiv1.HTTPRoute{}
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

// reconcileHTTPRouteStatus checks the readiness status of HTTPRoutes associated with all components
// of an InferenceService. It iterates through Predictor, Transformer (if defined), Explainer (if defined),
// and the top level httproute.
//
// For each component, it retrieves the corresponding HTTPRoute and checks if it's ready.
// If any HTTPRoute is missing or not ready, it updates the InferenceService status with a
// condition indicating why ingress is not ready and requeues the request.
// If all HTTPRoutes are ready, it marks the InferenceService's IngressReady condition as true.
func (r *RawHTTPRouteReconciler) reconcileHTTPRouteStatus(ctx context.Context, isvc *v1beta1.InferenceService) (ctrl.Result, error) {
	// Check HTTPRoute statuses for all components
	type httpRouteCheck struct {
		name      string
		component string
	}

	checks := []httpRouteCheck{
		{
			name:      constants.PredictorServiceName(isvc.Name),
			component: "Predictor",
		},
	}

	if isvc.Spec.Transformer != nil {
		checks = append(checks, httpRouteCheck{
			name:      constants.TransformerServiceName(isvc.Name),
			component: "Transformer",
		})
	}
	if isvc.Spec.Explainer != nil {
		checks = append(checks, httpRouteCheck{
			name:      constants.ExplainerServiceName(isvc.Name),
			component: "Explainer",
		})
	}
	checks = append(checks, httpRouteCheck{
		name:      isvc.Name,
		component: "InferenceService",
	})

	for _, check := range checks {
		httpRoute := &gwapiv1.HTTPRoute{}
		if err := r.client.Get(ctx, types.NamespacedName{
			Name:      check.name,
			Namespace: isvc.Namespace,
		}, httpRoute); err != nil {
			if apierr.IsNotFound(err) {
				// HTTPRoute not found means the component deployment is not ready yet, so we set the IngressReady condition to False
				// and requeue the request with backoff period.
				isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
					Type:    v1beta1.IngressReady,
					Status:  corev1.ConditionFalse,
					Reason:  check.component + " Deployment NotReady",
					Message: check.component + " HTTPRoute not created",
				})
				return ctrl.Result{Requeue: true}, nil
			}
			// Return any other errors
			return ctrl.Result{}, err
		}
		// Check if the HTTPRoute is ready
		// If not, set the IngressReady condition to False and requeue the request with backoff period.
		if ready, reason, message := isHTTPRouteReady(httpRoute.Status); !ready {
			log.Info(check.component+" HTTPRoute not ready", "reason", *reason, "message", *message)
			isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
				Type:    v1beta1.IngressReady,
				Status:  corev1.ConditionFalse,
				Reason:  *reason,
				Message: fmt.Sprintf("%s %s", check.component, *message),
			})
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// If we are here, then all the HTTPRoutes are ready, Mark ingress as ready
	isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
		Type:   v1beta1.IngressReady,
		Status: corev1.ConditionTrue,
	})
	return ctrl.Result{}, nil
}

// ReconcileHTTPRoute reconciles the HTTPRoute resource
func (r *RawHTTPRouteReconciler) Reconcile(ctx context.Context, isvc *v1beta1.InferenceService) (ctrl.Result, error) {
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
			return ctrl.Result{}, err
		}
		if isvc.Spec.Transformer != nil {
			if err := r.reconcileTransformerHTTPRoute(ctx, isvc); err != nil {
				return ctrl.Result{}, err
			}
		}
		if isvc.Spec.Explainer != nil {
			if err := r.reconcileExplainerHTTPRoute(ctx, isvc); err != nil {
				return ctrl.Result{}, err
			}
		}
		if err := r.reconcileTopLevelHTTPRoute(ctx, isvc); err != nil {
			return ctrl.Result{}, err
		}

		if utils.GetForceStopRuntime(isvc) {
			isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionFalse,
				Reason: v1beta1.StoppedISVCReason,
			})

			return ctrl.Result{}, nil
		}

		// Check HTTPRoute statuses for all components
		if result, err := r.reconcileHTTPRouteStatus(ctx, isvc); err != nil || result.Requeue {
			return result, err
		}
	} else {
		// Ingress creation is disabled. We set it to true as the isvc condition depends on it.
		isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionTrue,
		})
	}
	if isvc.Status.URL, err = createRawURL(isvc, r.ingressConfig); err != nil {
		return ctrl.Result{}, err
	}
	isvc.Status.Address = &duckv1.Addressable{
		URL: &knapis.URL{
			Host:   getRawServiceHost(isvc),
			Scheme: r.ingressConfig.UrlScheme,
			Path:   "",
		},
	}
	return ctrl.Result{}, nil
}
