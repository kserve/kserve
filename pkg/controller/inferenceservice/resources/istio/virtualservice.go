/*
Copyright 2019 kubeflow.org.

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

package istio

import (
	"encoding/json"
	"fmt"
	"github.com/gogo/protobuf/types"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/network"
	"strings"
	"time"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

const (
	IngressConfigKeyName = "ingress"
)

var (
	RetryTimeout = types.DurationProto(time.Duration(10) * time.Minute)
)

// Status Constants
var (
	PredictorSpecMissing       = "PredictorSpecMissing"
	PredictorStatusUnknown     = "PredictorStatusUnknown"
	PredictorHostnameUnknown   = "PredictorHostnameUnknown"
	TransformerSpecMissing     = "TransformerSpecMissing"
	TransformerStatusUnknown   = "TransformerStatusUnknown"
	TransformerHostnameUnknown = "TransformerHostnameUnknown"
	ExplainerSpecMissing       = "ExplainerSpecMissing"
	ExplainerStatusUnknown     = "ExplainerStatusUnknown"
	ExplainerHostnameUnknown   = "ExplainerHostnameUnknown"
)

// Message constants
var (
	PredictorMissingMessage   = "Failed to reconcile predictor"
	TransformerMissingMessage = "Failed to reconcile transformer"
	ExplainerMissingMessage   = "Failed to reconcile explainer"
)

type IngressConfig struct {
	IngressGateway     string `json:"ingressGateway,omitempty"`
	IngressServiceName string `json:"ingressService,omitempty"`
}

type VirtualServiceBuilder struct {
	ingressConfig *IngressConfig
}

func NewVirtualServiceBuilder(config *corev1.ConfigMap) *VirtualServiceBuilder {
	ingressConfig := &IngressConfig{}
	if ingress, ok := config.Data[IngressConfigKeyName]; ok {
		err := json.Unmarshal([]byte(ingress), &ingressConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to parse ingress config json: %v", err))
		}

		if ingressConfig.IngressGateway == "" || ingressConfig.IngressServiceName == "" {
			panic(fmt.Errorf("Invalid ingress config, ingressGateway and ingressService are required."))
		}
	}

	return &VirtualServiceBuilder{ingressConfig: ingressConfig}
}

func createFailedStatus(reason string, message string) *v1alpha2.VirtualServiceStatus {
	return &v1alpha2.VirtualServiceStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{{
				Type:    v1alpha2.RoutesReady,
				Status:  corev1.ConditionFalse,
				Reason:  reason,
				Message: message,
			}},
		},
	}
}

func (r *VirtualServiceBuilder) getPredictRouteDestination(meta metav1.Object, isCanary bool,
	endpointSpec *v1alpha2.EndpointSpec, componentStatusMap *v1alpha2.ComponentStatusMap, weight int32) (*istiov1alpha3.HTTPRouteDestination, *v1alpha2.VirtualServiceStatus) {
	if endpointSpec == nil {
		return nil, nil
	}
	// destination for the predict is required
	predictStatus, reason := getPredictStatusConfigurationSpec(componentStatusMap)
	if predictStatus == nil {
		return nil, createFailedStatus(reason, PredictorMissingMessage)
	}

	predictorHost := constants.DefaultPredictorServiceName(meta.GetName())
	if isCanary {
		predictorHost = constants.CanaryPredictorServiceName(meta.GetName())
	}

	// use transformer instead (if one is configured)
	if endpointSpec.Transformer != nil {
		predictStatus, reason = getTransformerStatusConfigurationSpec(componentStatusMap)
		if predictStatus == nil {
			return nil, createFailedStatus(reason, TransformerMissingMessage)
		}
		predictorHost = constants.DefaultTransformerServiceName(meta.GetName())
		if isCanary {
			predictorHost = constants.CanaryTransformerServiceName(meta.GetName())
		}
	}

	httpRouteDestination := createHTTPRouteDestination(predictorHost, meta.GetNamespace(), weight, r.ingressConfig.IngressServiceName)
	return &httpRouteDestination, nil
}

func (r *VirtualServiceBuilder) getExplainerRouteDestination(meta metav1.Object, isCanary bool,
	endpointSpec *v1alpha2.EndpointSpec, componentStatusMap *v1alpha2.ComponentStatusMap, weight int32) (*istiov1alpha3.HTTPRouteDestination, *v1alpha2.VirtualServiceStatus) {
	if endpointSpec == nil {
		return nil, nil
	}
	predictorHost := constants.DefaultPredictorServiceName(meta.GetName())
	if isCanary {
		predictorHost = constants.CanaryPredictorServiceName(meta.GetName())
	}
	if endpointSpec.Explainer != nil {
		explainSpec, explainerReason := getExplainStatusConfigurationSpec(endpointSpec, componentStatusMap)
		if explainSpec != nil {
			predictorHost = constants.DefaultExplainerServiceName(meta.GetName())
			if isCanary {
				predictorHost = constants.CanaryExplainerServiceName(meta.GetName())
			}
			httpRouteDestination := createHTTPRouteDestination(predictorHost, meta.GetNamespace(), weight, r.ingressConfig.IngressServiceName)
			return &httpRouteDestination, nil
		} else {
			return nil, createFailedStatus(explainerReason, ExplainerMissingMessage)
		}
	}
	return nil, nil
}

func (r *VirtualServiceBuilder) CreateVirtualService(isvc *v1alpha2.InferenceService) (*v1alpha3.VirtualService, *v1alpha2.VirtualServiceStatus) {

	httpRoutes := []*istiov1alpha3.HTTPRoute{}
	predictRouteDestinations := []*istiov1alpha3.HTTPRouteDestination{}
	serviceHostname, _ := getServiceHostname(isvc)

	defaultWeight := 100 - isvc.Spec.CanaryTrafficPercent
	canaryWeight := isvc.Spec.CanaryTrafficPercent

	if defaultPredictRouteDestination, err := r.getPredictRouteDestination(isvc.GetObjectMeta(), false, &isvc.Spec.Default, isvc.Status.Default, int32(defaultWeight)); err != nil {
		return nil, err
	} else {
		predictRouteDestinations = append(predictRouteDestinations, defaultPredictRouteDestination)
	}
	if canaryPredictRouteDestination, err := r.getPredictRouteDestination(isvc.GetObjectMeta(), true, isvc.Spec.Canary, isvc.Status.Canary, int32(canaryWeight)); err != nil {
		return nil, err
	} else {
		if canaryPredictRouteDestination != nil {
			predictRouteDestinations = append(predictRouteDestinations, canaryPredictRouteDestination)
		}
	}
	// prepare the predict route
	predictRoute := &istiov1alpha3.HTTPRoute{
		Match: []*istiov1alpha3.HTTPMatchRequest{
			{
				Uri: &istiov1alpha3.StringMatch{
					MatchType: &istiov1alpha3.StringMatch_Regex{
						Regex: constants.PredictPrefix(),
					},
				},
				Authority: &istiov1alpha3.StringMatch{
					MatchType: &istiov1alpha3.StringMatch_Regex{
						Regex: constants.HostRegExp(serviceHostname),
					},
				},
				Gateways: []string{r.ingressConfig.IngressGateway},
			},
			{
				Uri: &istiov1alpha3.StringMatch{
					MatchType: &istiov1alpha3.StringMatch_Regex{
						Regex: constants.PredictPrefix(),
					},
				},
				Authority: &istiov1alpha3.StringMatch{
					MatchType: &istiov1alpha3.StringMatch_Regex{
						Regex: constants.HostRegExp(network.GetServiceHostname(isvc.Name, isvc.Namespace)),
					},
				},
				Gateways: []string{constants.KnativeLocalGateway},
			},
		},
		Route: predictRouteDestinations,
		Retries: &istiov1alpha3.HTTPRetry{
			Attempts:      0,
			PerTryTimeout: nil,
		},
	}
	httpRoutes = append(httpRoutes, predictRoute)

	// optionally add the explain route
	explainRouteDestinations := []*istiov1alpha3.HTTPRouteDestination{}
	if defaultExplainRouteDestination, err := r.getExplainerRouteDestination(isvc.GetObjectMeta(), false, &isvc.Spec.Default, isvc.Status.Default, int32(defaultWeight)); err != nil {
		return nil, err
	} else {
		if defaultExplainRouteDestination != nil {
			explainRouteDestinations = append(explainRouteDestinations, defaultExplainRouteDestination)
		}
	}
	if canaryExplainRouteDestination, err := r.getExplainerRouteDestination(isvc.GetObjectMeta(), true, isvc.Spec.Canary, isvc.Status.Canary, int32(canaryWeight)); err != nil {
		return nil, err
	} else {
		if canaryExplainRouteDestination != nil {
			explainRouteDestinations = append(explainRouteDestinations, canaryExplainRouteDestination)
		}
	}

	if len(explainRouteDestinations) > 0 {
		explainRoute := &istiov1alpha3.HTTPRoute{
			Match: []*istiov1alpha3.HTTPMatchRequest{
				{
					Uri: &istiov1alpha3.StringMatch{
						MatchType: &istiov1alpha3.StringMatch_Regex{
							Regex: constants.ExplainPrefix(),
						},
					},
					Authority: &istiov1alpha3.StringMatch{
						MatchType: &istiov1alpha3.StringMatch_Regex{
							Regex: constants.HostRegExp(serviceHostname),
						},
					},
					Gateways: []string{r.ingressConfig.IngressGateway},
				},
				{
					Uri: &istiov1alpha3.StringMatch{
						MatchType: &istiov1alpha3.StringMatch_Regex{
							Regex: constants.ExplainPrefix(),
						},
					},
					Authority: &istiov1alpha3.StringMatch{
						MatchType: &istiov1alpha3.StringMatch_Regex{
							Regex: constants.HostRegExp(network.GetServiceHostname(isvc.Name, isvc.Namespace)),
						},
					},
					Gateways: []string{constants.KnativeLocalGateway},
				},
			},
			Route: explainRouteDestinations,
			Retries: &istiov1alpha3.HTTPRetry{
				Attempts:      3,
				PerTryTimeout: RetryTimeout,
			},
		}
		httpRoutes = append(httpRoutes, explainRoute)
	}
	// extract the virtual service hostname from the predictor hostname
	serviceURL := fmt.Sprintf("%s://%s%s", "http", serviceHostname, constants.InferenceServicePrefix(isvc.Name))

	vs := v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        isvc.Name,
			Namespace:   isvc.Namespace,
			Labels:      isvc.Labels,
			Annotations: isvc.Annotations,
		},
		Spec: istiov1alpha3.VirtualService{
			Hosts: []string{
				serviceHostname,
				network.GetServiceHostname(isvc.Name, isvc.Namespace),
			},
			Gateways: []string{
				r.ingressConfig.IngressGateway,
				constants.KnativeLocalGateway,
			},
			Http: httpRoutes,
		},
	}

	status := v1alpha2.VirtualServiceStatus{
		URL: serviceURL,
		Address: &duckv1beta1.Addressable{URL: &apis.URL{
			Scheme: "http",
			Host:   network.GetServiceHostname(isvc.Name, isvc.Namespace),
			Path:   fmt.Sprintf("/v1/models/%s:predict", isvc.Name),
		}},
		CanaryWeight:  canaryWeight,
		DefaultWeight: defaultWeight,
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{{
				Type:   v1alpha2.RoutesReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}

	return &vs, &status
}

func getServiceHostname(isvc *v1alpha2.InferenceService) (string, error) {
	predictorStatus, reason := getPredictStatusConfigurationSpec(isvc.Status.Default)
	if predictorStatus == nil {
		return "", fmt.Errorf("failed to get service hostname: %s", reason)
	}
	return strings.ReplaceAll(predictorStatus.Hostname, fmt.Sprintf("-%s-%s", string(constants.Predictor), constants.InferenceServiceDefault), ""), nil
}

func getPredictStatusConfigurationSpec(componentStatusMap *v1alpha2.ComponentStatusMap) (*v1alpha2.StatusConfigurationSpec, string) {
	if componentStatusMap == nil {
		return nil, PredictorStatusUnknown
	}

	if predictorStatus, ok := (*componentStatusMap)[constants.Predictor]; !ok {
		return nil, PredictorStatusUnknown
	} else if len(predictorStatus.Hostname) == 0 {
		return nil, PredictorHostnameUnknown
	} else {
		return &predictorStatus, ""
	}
}

func getTransformerStatusConfigurationSpec(componentStatusMap *v1alpha2.ComponentStatusMap) (*v1alpha2.StatusConfigurationSpec, string) {
	if componentStatusMap == nil {
		return nil, TransformerStatusUnknown
	}

	if transformerStatus, ok := (*componentStatusMap)[constants.Transformer]; !ok {
		return nil, TransformerStatusUnknown
	} else if len(transformerStatus.Hostname) == 0 {
		return nil, TransformerHostnameUnknown
	} else {
		return &transformerStatus, ""
	}
}

func getExplainStatusConfigurationSpec(endpointSpec *v1alpha2.EndpointSpec, componentStatusMap *v1alpha2.ComponentStatusMap) (*v1alpha2.StatusConfigurationSpec, string) {
	if endpointSpec.Explainer == nil {
		return nil, ExplainerSpecMissing
	}

	if componentStatusMap == nil {
		return nil, ExplainerStatusUnknown
	}

	if explainerStatus, ok := (*componentStatusMap)[constants.Explainer]; !ok {
		return nil, ExplainerStatusUnknown
	} else if len(explainerStatus.Hostname) == 0 {
		return nil, ExplainerHostnameUnknown
	} else {
		return &explainerStatus, ""
	}
}

func createHTTPRouteDestination(targetHost, namespace string, weight int32, gatewayService string) istiov1alpha3.HTTPRouteDestination {
	httpRouteDestination := istiov1alpha3.HTTPRouteDestination{
		Weight: weight,
		Headers: &istiov1alpha3.Headers{
			Request: &istiov1alpha3.Headers_HeaderOperations{
				Set: map[string]string{
					"Host": network.GetServiceHostname(targetHost, namespace),
				},
			},
		},
		Destination: &istiov1alpha3.Destination{
			Host: constants.LocalGatewayHost,
			Port: &istiov1alpha3.PortSelector{
				Number: constants.CommonDefaultHttpPort,
			},
		},
	}

	return httpRouteDestination
}
