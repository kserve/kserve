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

// resolveTimeout returns the timeout duration for an HTTPRoute rule.
// If disableTimeout is true, it returns nil so the Timeouts field is omitted entirely.
// This is needed for Gateway implementations (e.g. GKE) that do not support
// the spec.rules.timeouts field.
func resolveTimeout(disableTimeout bool, timeoutSeconds *int64) *gwapiv1.Duration {
	if disableTimeout {
		return nil
	}
	if timeoutSeconds != nil {
		return toGatewayAPIDuration(*timeoutSeconds)
	}
	return DefaultTimeout
}

func createRawURL(isvc *v1beta1.InferenceService,
	ingressConfig *v1beta1.IngressConfig,
) (*knapis.URL, error) {
	url := &knapis.URL{}
	url.Scheme = ingressConfig.UrlScheme

	// With host-based routing disabled, the URL is scheme://ingressDomain/path, not the subdomain URL
	if ingressConfig.PathTemplate != "" &&
		ingressConfig.RawDeployment != nil &&
		ingressConfig.RawDeployment.DisableHostBasedRouting {
		path, err := GenerateUrlPath(isvc.Name, isvc.Namespace, ingressConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to generate URL path for status URL: %w", err)
		}
		url.Host = ingressConfig.IngressDomain
		url.Path = path
		return url, nil
	}

	var err error
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

// detectServiceProtocolPorts queries the Service to extract protocol port information.
// It analyzes the service ports to identify REST and gRPC ports based on appProtocol annotations
// and port names. Only the first port of each type is detected and returned.
// Returns the REST port, gRPC port, and any error encountered.
func detectServiceProtocolPorts(ctx context.Context, client client.Client, serviceName, namespace string) (restPort, grpcPort int32, err error) {
	// Query the Service
	svc := &corev1.Service{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      serviceName,
		Namespace: namespace,
	}, svc)
	if err != nil {
		// Service may not exist yet in early reconciliation paths. In that case, gracefully fall back to default routing.
		if apierr.IsNotFound(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	// Analyze ports to identify REST and gRPC
	for _, port := range svc.Spec.Ports {
		// Check if this is a gRPC port based on appProtocol or port name
		switch {
		case port.AppProtocol != nil && *port.AppProtocol == "kubernetes.io/h2c":
			if grpcPort == 0 {
				grpcPort = port.Port
			}
		case isGrpcPortByName(port.Name):
			if grpcPort == 0 {
				grpcPort = port.Port
			}
		default:
			// HTTP port
			if restPort == 0 {
				restPort = port.Port
			}
		}
	}
	return restPort, grpcPort, nil
}

// isGrpcPortByName checks if a port name indicates gRPC protocol
func isGrpcPortByName(portName string) bool {
	portNameLower := strings.ToLower(portName)
	return strings.Contains(portNameLower, "grpc") || strings.Contains(portNameLower, "h2c")
}

// createGRPCRouteMatches creates HTTPRouteMatch entries that match gRPC requests
// gRPC requests are identified by:
// 1. Path: /inference.GRPCInferenceService/* (gRPC v2 protocol)
// 2. Content-Type: application/grpc* (includes application/grpc+proto, application/grpc+json)
func createGRPCRouteMatches() []gwapiv1.HTTPRouteMatch {
	return []gwapiv1.HTTPRouteMatch{
		{
			Path: &gwapiv1.HTTPPathMatch{
				Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
				Value: ptr.To("^/inference\\.GRPCInferenceService/.*$"),
			},
			Headers: []gwapiv1.HTTPHeaderMatch{
				{
					Type:  ptr.To(gwapiv1.HeaderMatchRegularExpression),
					Name:  gwapiv1.HTTPHeaderName("content-type"),
					Value: "^application/grpc.*",
				},
			},
		},
	}
}

// createHTTPRouteMatches creates HTTPRouteMatch entries that match HTTP/REST requests
// This matches all requests that are NOT gRPC (no content-type restriction for broader compatibility)
func createHTTPRouteMatches(pathPrefix string) []gwapiv1.HTTPRouteMatch {
	return []gwapiv1.HTTPRouteMatch{
		{
			Path: &gwapiv1.HTTPPathMatch{
				Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
				Value: ptr.To(pathPrefix),
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
						Port:      &port,
					},
				},
			},
		}
	}
	rule := gwapiv1.HTTPRouteRule{
		Matches:     routeMatches,
		Filters:     filters,
		BackendRefs: backendRefs,
	}
	if timeout != nil {
		rule.Timeouts = &gwapiv1.HTTPRouteTimeouts{
			Request: timeout,
		}
	}
	return rule
}

// createHTTPRouteMatchWithType creates an HTTPRouteMatch with a configurable path match type.
func createHTTPRouteMatchWithType(matchValue string, matchType gwapiv1.PathMatchType) gwapiv1.HTTPRouteMatch {
	return gwapiv1.HTTPRouteMatch{
		Path: &gwapiv1.HTTPPathMatch{
			Type:  ptr.To(matchType),
			Value: ptr.To(matchValue),
		},
	}
}

// createHTTPRouteRuleWithTimeouts is like createHTTPRouteRule but also sets the optional backendRequest timeout.
func createHTTPRouteRuleWithTimeouts(routeMatches []gwapiv1.HTTPRouteMatch, filters []gwapiv1.HTTPRouteFilter, serviceName, namespace string, port int32, requestTimeout, backendTimeout *gwapiv1.Duration) gwapiv1.HTTPRouteRule { //nolint:unparam
	rule := createHTTPRouteRule(routeMatches, filters, serviceName, namespace, port, requestTimeout)
	if backendTimeout != nil {
		if rule.Timeouts == nil {
			rule.Timeouts = &gwapiv1.HTTPRouteTimeouts{}
		}
		rule.Timeouts.BackendRequest = backendTimeout
	}
	return rule
}

// rawSectionName returns a SectionName pointer when gatewayListenerName is set, or nil to attach to all listeners.
func rawSectionName(cfg *v1beta1.RawDeploymentIngressConfig) *gwapiv1.SectionName {
	if cfg == nil || cfg.GatewayListenerName == "" {
		return nil
	}
	s := gwapiv1.SectionName(cfg.GatewayListenerName)
	return &s
}

// applyRewriteFilter prepends an optional URLRewrite filter to a filter slice.
func applyRewriteFilter(filters []gwapiv1.HTTPRouteFilter, rewriteFilter *gwapiv1.HTTPRouteFilter) []gwapiv1.HTTPRouteFilter {
	if rewriteFilter == nil {
		return filters
	}
	result := make([]gwapiv1.HTTPRouteFilter, 0, len(filters)+1)
	result = append(result, *rewriteFilter)
	result = append(result, filters...)
	return result
}

// mergeRouteLabels merges rawDeployment.routeLabels into the provided label map (in-place), taking precedence over ISVC labels.
func mergeRouteLabels(labels map[string]string, ingressConfig *v1beta1.IngressConfig) {
	if ingressConfig.RawDeployment == nil {
		return
	}
	for k, v := range ingressConfig.RawDeployment.RouteLabels {
		labels[k] = v
	}
}

// resolveRawRequestTimeout returns the effective request timeout, preferring rawDeployment.requestTimeout when set.
func resolveRawRequestTimeout(disableTimeout bool, componentTimeout *int64, rawCfg *v1beta1.RawDeploymentIngressConfig) *gwapiv1.Duration {
	timeout := resolveTimeout(disableTimeout, componentTimeout)
	if rawCfg != nil && rawCfg.RequestTimeout != "" {
		d := gwapiv1.Duration(rawCfg.RequestTimeout)
		return &d
	}
	return timeout
}

func createRawPredictorHTTPRoute(ctx context.Context, client client.Client, isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig,
	isvcConfig *v1beta1.InferenceServicesConfig,
) (*gwapiv1.HTTPRoute, error) {
	httpRouteRules := make([]gwapiv1.HTTPRouteRule, 0, 2)
	allowedHosts := make([]gwapiv1.Hostname, 0, 1)

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
	timeout := resolveTimeout(ingressConfig.DisableHTTPRouteTimeout, isvc.Spec.Predictor.TimeoutSeconds)

	// Detect dual-protocol configuration
	restPort, grpcPort, err := detectServiceProtocolPorts(ctx, client, predictorName, isvc.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to detect protocol ports for predictor service: %w", err)
	}

	if grpcPort != 0 && restPort != 0 {
		// Generate separate rules for gRPC and HTTP with header-based matching
		// gRPC rule FIRST (more specific - matches Content-Type: application/grpc* and gRPC path)
		grpcMatches := createGRPCRouteMatches()
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(grpcMatches, filters, predictorName, isvc.Namespace, grpcPort, timeout))

		// HTTP rule SECOND (fallback - matches all other requests)
		httpMatches := createHTTPRouteMatches(constants.FallbackPrefix())
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(httpMatches, filters, predictorName, isvc.Namespace, restPort, timeout))
	} else {
		// Fall back to old default behavior
		routeMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, predictorName, isvc.Namespace, constants.CommonDefaultHttpPort, timeout))
	}

	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceAnnotationDisallowedList, key)
	})
	labels := utils.Filter(isvc.Labels, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceLabelDisallowedList, key)
	})
	mergeRouteLabels(labels, ingressConfig)
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
						Group:       (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
						Kind:        (*gwapiv1.Kind)(ptr.To(constants.GatewayKind)),
						Namespace:   (*gwapiv1.Namespace)(&gatewaySlice[0]),
						Name:        gwapiv1.ObjectName(gatewaySlice[1]),
						SectionName: rawSectionName(ingressConfig.RawDeployment),
					},
				},
			},
		},
	}
	return &httpRoute, nil
}

func createRawTransformerHTTPRoute(ctx context.Context, client client.Client, isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig,
	isvcConfig *v1beta1.InferenceServicesConfig,
) (*gwapiv1.HTTPRoute, error) {
	httpRouteRules := make([]gwapiv1.HTTPRouteRule, 0, 2)
	allowedHosts := make([]gwapiv1.Hostname, 0, 1)

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
	timeout := resolveTimeout(ingressConfig.DisableHTTPRouteTimeout, isvc.Spec.Transformer.TimeoutSeconds)
	// Detect dual-protocol configuration
	restPort, grpcPort, err := detectServiceProtocolPorts(ctx, client, transformerName, isvc.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to detect protocol ports for transformer service: %w", err)
	}

	if grpcPort != 0 && restPort != 0 {
		// Generate separate rules for gRPC and HTTP with header-based matching
		// gRPC rule FIRST (more specific - matches Content-Type: application/grpc* and gRPC path)
		grpcMatches := createGRPCRouteMatches()
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(grpcMatches, filters, transformerName, isvc.Namespace, grpcPort, timeout))

		// HTTP rule SECOND (fallback - matches all other requests)
		httpMatches := createHTTPRouteMatches(constants.FallbackPrefix())
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(httpMatches, filters, transformerName, isvc.Namespace, restPort, timeout))
	} else {
		// Fall back to old default behavior
		routeMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, transformerName, isvc.Namespace, constants.CommonDefaultHttpPort, timeout))
	}

	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceAnnotationDisallowedList, key)
	})
	labels := utils.Filter(isvc.Labels, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceLabelDisallowedList, key)
	})
	mergeRouteLabels(labels, ingressConfig)
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
						Group:       (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
						Kind:        (*gwapiv1.Kind)(ptr.To(constants.GatewayKind)),
						Namespace:   (*gwapiv1.Namespace)(&gatewaySlice[0]),
						Name:        gwapiv1.ObjectName(gatewaySlice[1]),
						SectionName: rawSectionName(ingressConfig.RawDeployment),
					},
				},
			},
		},
	}
	return &httpRoute, nil
}

func createRawExplainerHTTPRoute(ctx context.Context, client client.Client, isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig,
	isvcConfig *v1beta1.InferenceServicesConfig,
) (*gwapiv1.HTTPRoute, error) {
	httpRouteRules := make([]gwapiv1.HTTPRouteRule, 0, 2)
	allowedHosts := make([]gwapiv1.Hostname, 0, 1)

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
	timeout := resolveTimeout(ingressConfig.DisableHTTPRouteTimeout, isvc.Spec.Explainer.TimeoutSeconds)

	// Detect dual-protocol configuration
	restPort, grpcPort, err := detectServiceProtocolPorts(ctx, client, explainerName, isvc.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to detect protocol ports for explainer service: %w", err)
	}

	if grpcPort != 0 && restPort != 0 {
		// Generate separate rules for gRPC and HTTP with header-based matching
		// gRPC rule FIRST (more specific - matches Content-Type: application/grpc* and gRPC path)
		grpcMatches := createGRPCRouteMatches()
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(grpcMatches, filters, explainerName, isvc.Namespace, grpcPort, timeout))

		// HTTP rule SECOND (fallback - matches all other requests)
		httpMatches := createHTTPRouteMatches(constants.FallbackPrefix())
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(httpMatches, filters, explainerName, isvc.Namespace, restPort, timeout))
	} else {
		// Fall back to old default behavior
		routeMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
		httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, explainerName, isvc.Namespace, constants.CommonDefaultHttpPort, timeout))
	}

	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceAnnotationDisallowedList, key)
	})
	labels := utils.Filter(isvc.Labels, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceLabelDisallowedList, key)
	})
	mergeRouteLabels(labels, ingressConfig)
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
						Group:       (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
						Kind:        (*gwapiv1.Kind)(ptr.To(constants.GatewayKind)),
						Namespace:   (*gwapiv1.Namespace)(&gatewaySlice[0]),
						Name:        gwapiv1.ObjectName(gatewaySlice[1]),
						SectionName: rawSectionName(ingressConfig.RawDeployment),
					},
				},
			},
		},
	}
	return &httpRoute, nil
}

func createRawTopLevelHTTPRoute(ctx context.Context, client client.Client, isvc *v1beta1.InferenceService, ingressConfig *v1beta1.IngressConfig,
	isvcConfig *v1beta1.InferenceServicesConfig,
) (*gwapiv1.HTTPRoute, error) {
	httpRouteRules := make([]gwapiv1.HTTPRouteRule, 0, 2)
	allowedHosts := make([]gwapiv1.Hostname, 0, 1)

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
	// When disableHostBasedRouting is set, omit hostnames so the gateway listener's own filter applies.
	// topLevelHost is still used below for status URL generation.
	disableHostRouting := ingressConfig.RawDeployment != nil && ingressConfig.RawDeployment.DisableHostBasedRouting && ingressConfig.PathTemplate != ""
	if !disableHostRouting {
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
	}
	// Add isvc name and namespace headers
	filters := []gwapiv1.HTTPRouteFilter{addIsvcHeaders(isvc.Name, isvc.Namespace)}

	// Host-based catch-all rules are only generated when host-based routing is active.
	if !disableHostRouting {
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
			timeout := resolveTimeout(ingressConfig.DisableHTTPRouteTimeout, isvc.Spec.Explainer.TimeoutSeconds)
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
			timeout := resolveTimeout(ingressConfig.DisableHTTPRouteTimeout, isvc.Spec.Transformer.TimeoutSeconds)
			// :predict routes to the transformer when there are both predictor and transformer

			// Detect dual-protocol for transformer
			restPort, grpcPort, err := detectServiceProtocolPorts(ctx, client, transformerName, isvc.Namespace)
			if err != nil {
				return nil, fmt.Errorf("failed to detect protocol ports for transformer service: %w", err)
			}

			if grpcPort != 0 && restPort != 0 {
				// Generate separate rules for gRPC and HTTP with header-based matching
				// gRPC rule FIRST (more specific - matches Content-Type: application/grpc* and gRPC path)
				grpcMatches := createGRPCRouteMatches()
				httpRouteRules = append(httpRouteRules, createHTTPRouteRule(grpcMatches, filters, transformerName, isvc.Namespace, grpcPort, timeout))

				// HTTP rule SECOND (fallback - matches all other requests)
				httpMatches := createHTTPRouteMatches(constants.FallbackPrefix())
				httpRouteRules = append(httpRouteRules, createHTTPRouteRule(httpMatches, filters, transformerName, isvc.Namespace, restPort, timeout))
			} else {
				// Fall back to single-port default behavior
				routeMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
				httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, transformerName, isvc.Namespace, constants.CommonDefaultHttpPort, timeout))
			}
		} else {
			// Scenario: When predictor without transformer and with/without explainer present
			timeout := resolveTimeout(ingressConfig.DisableHTTPRouteTimeout, isvc.Spec.Predictor.TimeoutSeconds)
			// Add toplevel host rules for predictor which routes all traffic to predictor

			// Detect dual-protocol for predictor
			restPort, grpcPort, err := detectServiceProtocolPorts(ctx, client, predictorName, isvc.Namespace)
			if err != nil {
				return nil, fmt.Errorf("failed to detect protocol ports for predictor service: %w", err)
			}

			if grpcPort != 0 && restPort != 0 {
				// Generate separate rules for gRPC and HTTP with header-based matching
				// gRPC rule FIRST (more specific - matches Content-Type: application/grpc* and gRPC path)
				grpcMatches := createGRPCRouteMatches()
				httpRouteRules = append(httpRouteRules, createHTTPRouteRule(grpcMatches, filters, predictorName, isvc.Namespace, grpcPort, timeout))

				// HTTP rule SECOND (fallback - matches all other requests)
				httpMatches := createHTTPRouteMatches(constants.FallbackPrefix())
				httpRouteRules = append(httpRouteRules, createHTTPRouteRule(httpMatches, filters, predictorName, isvc.Namespace, restPort, timeout))
			} else {
				// Fall back to single-port default behavior
				routeMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatch(constants.FallbackPrefix())}
				httpRouteRules = append(httpRouteRules, createHTTPRouteRule(routeMatch, filters, predictorName, isvc.Namespace, constants.CommonDefaultHttpPort, timeout))
			}
		}
	}

	// Add path based routing rules
	if ingressConfig.PathTemplate != "" {
		rawCfg := ingressConfig.RawDeployment // may be nil when absent

		path, err := GenerateUrlPath(isvc.Name, isvc.Namespace, ingressConfig)
		if err != nil {
			log.Error(err, "Failed to generate URL from pathTemplate")
			return nil, fmt.Errorf("failed to generate URL from pathTemplate: %w", err)
		}
		path = strings.TrimSuffix(path, "/") // remove trailing "/" if present

		// Include ingressDomain to the allowed hosts unless host-based routing is disabled
		if rawCfg == nil || !rawCfg.DisableHostBasedRouting {
			allowedHosts = append(allowedHosts, gwapiv1.Hostname(ingressConfig.IngressDomain))
		}

		// Resolve path match type (defaults to RegularExpression to preserve current behaviour).
		pathMatchType := gwapiv1.PathMatchRegularExpression
		if rawCfg != nil && rawCfg.PathMatchType == string(gwapiv1.PathMatchPathPrefix) {
			pathMatchType = gwapiv1.PathMatchPathPrefix
		}

		// Resolve optional URL rewrite filter
		var rewriteFilter *gwapiv1.HTTPRouteFilter
		if rawCfg != nil && rawCfg.PathRewriteTarget != "" {
			rewriteTarget := rawCfg.PathRewriteTarget
			rewriteFilter = &gwapiv1.HTTPRouteFilter{
				Type: gwapiv1.HTTPRouteFilterURLRewrite,
				URLRewrite: &gwapiv1.HTTPURLRewriteFilter{
					Path: &gwapiv1.HTTPPathModifier{
						Type:               gwapiv1.PrefixMatchHTTPPathModifier,
						ReplacePrefixMatch: &rewriteTarget,
					},
				},
			}
		}

		// Resolve optional timeout overrides from rawDeployment config
		var rawBackendTimeout *gwapiv1.Duration
		if rawCfg != nil && rawCfg.BackendRequestTimeout != "" {
			d := gwapiv1.Duration(rawCfg.BackendRequestTimeout)
			rawBackendTimeout = &d
		}

		if isvc.Spec.Explainer != nil {
			// Gate on explainer readiness: when disableHostRouting is true the check above is skipped.
			if !isvc.Status.IsConditionReady(v1beta1.ExplainerReady) {
				isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
					Type:   v1beta1.IngressReady,
					Status: corev1.ConditionFalse,
					Reason: "Explainer ingress not created",
				})
				return nil, nil
			}
			timeout := resolveRawRequestTimeout(ingressConfig.DisableHTTPRouteTimeout, isvc.Spec.Explainer.TimeoutSeconds, rawCfg)
			// Add path based routing rule for :explain endpoint.
			// PathBasedExplainPrefix() returns a regex fragment, so the match type must
			// always be RegularExpression regardless of the configured pathMatchType.
			explainerPathRouteMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatchWithType(path+constants.PathBasedExplainPrefix(), gwapiv1.PathMatchRegularExpression)}
			httpRouteRules = append(httpRouteRules, createHTTPRouteRuleWithTimeouts(explainerPathRouteMatch, applyRewriteFilter(filters, rewriteFilter), explainerName, isvc.Namespace,
				constants.CommonDefaultHttpPort, timeout, rawBackendTimeout))
		}
		// Add path based routing rule for :predict endpoint
		if isvc.Spec.Transformer != nil {
			// Gate on transformer readiness: when disableHostRouting is true the check above is skipped.
			if !isvc.Status.IsConditionReady(v1beta1.TransformerReady) {
				isvc.Status.SetCondition(v1beta1.IngressReady, &knapis.Condition{
					Type:   v1beta1.IngressReady,
					Status: corev1.ConditionFalse,
					Reason: "Transformer ingress not created",
				})
				return nil, nil
			}
			timeout := resolveRawRequestTimeout(ingressConfig.DisableHTTPRouteTimeout, isvc.Spec.Transformer.TimeoutSeconds, rawCfg)
			// :predict routes to the transformer when there are both predictor and transformer
			pathRouteMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatchWithType(path+"/", pathMatchType)}
			httpRouteRules = append(httpRouteRules, createHTTPRouteRuleWithTimeouts(pathRouteMatch, applyRewriteFilter(filters, rewriteFilter), transformerName, isvc.Namespace,
				constants.CommonDefaultHttpPort, timeout, rawBackendTimeout))
		} else {
			timeout := resolveRawRequestTimeout(ingressConfig.DisableHTTPRouteTimeout, isvc.Spec.Predictor.TimeoutSeconds, rawCfg)
			// :predict routes to the predictor when there is only predictor
			pathRouteMatch := []gwapiv1.HTTPRouteMatch{createHTTPRouteMatchWithType(path+"/", pathMatchType)}
			httpRouteRules = append(httpRouteRules, createHTTPRouteRuleWithTimeouts(pathRouteMatch, applyRewriteFilter(filters, rewriteFilter), predictorName, isvc.Namespace,
				constants.CommonDefaultHttpPort, timeout, rawBackendTimeout))
		}
	}

	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceAnnotationDisallowedList, key)
	})
	labels := utils.Filter(isvc.Labels, func(key string) bool {
		return !utils.Includes(isvcConfig.ServiceLabelDisallowedList, key)
	})
	mergeRouteLabels(labels, ingressConfig)
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
						Group:       (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
						Kind:        (*gwapiv1.Kind)(ptr.To(constants.GatewayKind)),
						Namespace:   (*gwapiv1.Namespace)(&gatewaySlice[0]),
						Name:        gwapiv1.ObjectName(gatewaySlice[1]),
						SectionName: rawSectionName(ingressConfig.RawDeployment),
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
	desired, err := createRawPredictorHTTPRoute(ctx, r.client, isvc, r.ingressConfig, r.isvcConfig)
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
	desired, err := createRawTransformerHTTPRoute(ctx, r.client, isvc, r.ingressConfig, r.isvcConfig)
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
	desired, err := createRawExplainerHTTPRoute(ctx, r.client, isvc, r.ingressConfig, r.isvcConfig)
	if err != nil {
		return err
	}

	// reconcile httproute
	httpRouteName := constants.ExplainerServiceName(isvc.Name)
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
	desired, err := createRawTopLevelHTTPRoute(ctx, r.client, isvc, r.ingressConfig, r.isvcConfig)
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
	isvc.Status.Address, err = createAddress(ctx, r.client, isvc)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
