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

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	istiov1alpha1 "knative.dev/pkg/apis/istio/common/v1alpha1"
	istiov1alpha3 "knative.dev/pkg/apis/istio/v1alpha3"
)

const (
	IngressConfigKeyName = "ingress"
)

// Status Constants
var (
	PredictorSpecMissing       = "predictorSpecMissing"
	PredictorStatusUnknown     = "predictorStatusUnknown"
	PredictorHostnameUnknown   = "predictorHostnameUnknown"
	TransformerSpecMissing     = "transformerSpecMissing"
	TransformerStatusUnknown   = "transformerStatusUnknown"
	TransformerHostnameUnknown = "transformerHostnameUnknown"
	ExplainerSpecMissing       = "explainerSpecMissing"
	ExplainerStatusUnknown     = "explainerStatusUnknown"
	ExplainerHostnameUnknown   = "explainerHostnameUnknown"
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

func (r *VirtualServiceBuilder) getPredictRouteDestination(
	endpointSpec *v1alpha2.EndpointSpec, endpointStatusMap *v1alpha2.EndpointStatusMap, weight int) (*istiov1alpha3.HTTPRouteDestination, *v1alpha2.VirtualServiceStatus) {
	if endpointSpec == nil {
		return nil, nil
	}
	// destination for the predict is required
	predictSpec, reason := getPredictStatusConfigurationSpec(endpointSpec, endpointStatusMap)
	if predictSpec == nil {
		return nil, createFailedStatus(reason, "Failed to reconcile predictor")
	}

	// use transformer instead (if one is configured)
	if endpointSpec.Transformer != nil {
		predictSpec, reason = getTransformerStatusConfigurationSpec(endpointSpec, endpointStatusMap)
		if predictSpec == nil {
			return nil, createFailedStatus(reason, "Failed to reconcile transformer")
		}
	}

	httpRouteDestination := createHTTPRouteDestination(predictSpec.Hostname, weight, r.ingressConfig.IngressServiceName)
	return &httpRouteDestination, nil
}

func (r *VirtualServiceBuilder) getExplainerRouteDestination(
	endpointSpec *v1alpha2.EndpointSpec, endpointStatusMap *v1alpha2.EndpointStatusMap, weight int) (*istiov1alpha3.HTTPRouteDestination, *v1alpha2.VirtualServiceStatus) {
	if endpointSpec == nil {
		return nil, nil
	}
	if endpointSpec.Explainer != nil {
		explainSpec, explainerReason := getExplainStatusConfigurationSpec(endpointSpec, endpointStatusMap)
		if explainSpec != nil {
			httpRouteDestination := createHTTPRouteDestination(explainSpec.Hostname, weight, r.ingressConfig.IngressServiceName)
			return &httpRouteDestination, nil
		} else {
			return nil, createFailedStatus(explainerReason, "Failed to reconcile default explainer")
		}
	}
	return nil, nil
}

func (r *VirtualServiceBuilder) CreateVirtualService(isvc *v1alpha2.InferenceService) (*istiov1alpha3.VirtualService, *v1alpha2.VirtualServiceStatus) {

	httpRoutes := []istiov1alpha3.HTTPRoute{}
	predictRouteDestinations := []istiov1alpha3.HTTPRouteDestination{}

	defaultWeight := 100 - isvc.Spec.CanaryTrafficPercent
	canaryWeight := isvc.Spec.CanaryTrafficPercent

	if defaultPredictRouteDestination, err := r.getPredictRouteDestination(&isvc.Spec.Default, isvc.Status.Default, defaultWeight); err != nil {
		return nil, err
	} else {
		predictRouteDestinations = append(predictRouteDestinations, *defaultPredictRouteDestination)
	}
	if canaryPredictRouteDestination, err := r.getPredictRouteDestination(isvc.Spec.Canary, isvc.Status.Canary, canaryWeight); err != nil {
		return nil, err
	} else {
		if canaryPredictRouteDestination != nil {
			predictRouteDestinations = append(predictRouteDestinations, *canaryPredictRouteDestination)
		}
	}
	// prepare the predict route
	predictRoute := istiov1alpha3.HTTPRoute{
		Match: []istiov1alpha3.HTTPMatchRequest{
			istiov1alpha3.HTTPMatchRequest{
				URI: &istiov1alpha1.StringMatch{
					Prefix: constants.PredictPrefix(isvc.Name),
				},
			},
		},
		Route: predictRouteDestinations,
	}
	httpRoutes = append(httpRoutes, predictRoute)

	// optionally add the explain route
	explainRouteDestinations := []istiov1alpha3.HTTPRouteDestination{}
	if defaultExplainRouteDestination, err := r.getExplainerRouteDestination(&isvc.Spec.Default, isvc.Status.Default, defaultWeight); err != nil {
		return nil, err
	} else {
		if defaultExplainRouteDestination != nil {
			explainRouteDestinations = append(explainRouteDestinations, *defaultExplainRouteDestination)
		}
	}
	if canaryExplainRouteDestination, err := r.getExplainerRouteDestination(isvc.Spec.Canary, isvc.Status.Canary, canaryWeight); err != nil {
		return nil, err
	} else {
		if canaryExplainRouteDestination != nil {
			explainRouteDestinations = append(explainRouteDestinations, *canaryExplainRouteDestination)
		}
	}

	if len(explainRouteDestinations) > 0 {
		explainRoute := istiov1alpha3.HTTPRoute{
			Match: []istiov1alpha3.HTTPMatchRequest{
				istiov1alpha3.HTTPMatchRequest{
					URI: &istiov1alpha1.StringMatch{
						Prefix: constants.ExplainPrefix(isvc.Name),
					},
				},
			},
			Route: explainRouteDestinations,
		}
		httpRoutes = append(httpRoutes, explainRoute)
	}
	// extract the virtual service hostname from the predictor hostname
	serviceHostname, _ := getServiceHostname(isvc)
	serviceURL := constants.ServiceURL(isvc.Name, serviceHostname)

	vs := istiov1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        isvc.Name,
			Namespace:   isvc.Namespace,
			Labels:      isvc.Labels,
			Annotations: isvc.Annotations,
		},
		Spec: istiov1alpha3.VirtualServiceSpec{
			Hosts: []string{
				serviceHostname,
			},
			Gateways: []string{
				r.ingressConfig.IngressGateway,
			},
			HTTP: httpRoutes,
		},
	}

	status := v1alpha2.VirtualServiceStatus{
		URL:           serviceURL,
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
	predictSpec, reason := getPredictStatusConfigurationSpec(&isvc.Spec.Default, isvc.Status.Default)
	if predictSpec == nil {
		return "", fmt.Errorf("Fail to get service hostname: %s.", reason)
	}
	return constants.VirtualServiceHostname(isvc.Name, predictSpec.Hostname), nil
}

func getPredictStatusConfigurationSpec(endpointSpec *v1alpha2.EndpointSpec, endpointStatusMap *v1alpha2.EndpointStatusMap) (*v1alpha2.StatusConfigurationSpec, string) {
	if endpointSpec == nil {
		return nil, PredictorSpecMissing
	}

	if predictorStatus, ok := (*endpointStatusMap)[constants.Predictor]; !ok {
		return nil, PredictorStatusUnknown
	} else if len(predictorStatus.Hostname) == 0 {
		return nil, PredictorHostnameUnknown
	} else {
		return predictorStatus, ""
	}
}

func getTransformerStatusConfigurationSpec(endpointSpec *v1alpha2.EndpointSpec, endpointStatusMap *v1alpha2.EndpointStatusMap) (*v1alpha2.StatusConfigurationSpec, string) {
	if endpointSpec.Transformer == nil {
		return nil, TransformerSpecMissing
	}

	if transformerStatus, ok := (*endpointStatusMap)[constants.Transformer]; !ok {
		return nil, TransformerStatusUnknown
	} else if len(transformerStatus.Hostname) == 0 {
		return nil, TransformerHostnameUnknown
	} else {
		return transformerStatus, ""
	}
}
func getExplainStatusConfigurationSpec(endpointSpec *v1alpha2.EndpointSpec, endpointStatusMap *v1alpha2.EndpointStatusMap) (*v1alpha2.StatusConfigurationSpec, string) {
	if endpointSpec.Explainer == nil {
		return nil, ExplainerSpecMissing
	}

	explainerStatus, ok := (*endpointStatusMap)[constants.Explainer]
	if !ok {
		return nil, ExplainerStatusUnknown
	} else if len(explainerStatus.Hostname) == 0 {
		return nil, ExplainerHostnameUnknown
	}

	return explainerStatus, ""
}

func createHTTPRouteDestination(targetHost string, weight int, gatewayService string) istiov1alpha3.HTTPRouteDestination {
	httpRouteDestination := istiov1alpha3.HTTPRouteDestination{
		Weight: weight,
		Headers: &istiov1alpha3.Headers{
			Request: &istiov1alpha3.HeaderOperations{
				Set: map[string]string{
					"Host": targetHost,
				},
			},
		},
		Destination: istiov1alpha3.Destination{
			Host: gatewayService,
		},
	}

	return httpRouteDestination
}
