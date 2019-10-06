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
	"strings"

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
			panic(fmt.Errorf("Unable to unmarshall ingress json string due to %v ", err))
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

func (r *VirtualServiceBuilder) CreateVirtualService(kfsvc *v1alpha2.KFService) (*istiov1alpha3.VirtualService, *v1alpha2.VirtualServiceStatus) {

	httpRoutes := []istiov1alpha3.HTTPRoute{}

	// destination for the default predict is required
	predictDefaultSpec, defaultPredictReason := getPredictStatusConfigurationSpec(&kfsvc.Spec.Default, kfsvc.Status.Default)
	if predictDefaultSpec == nil {
		return nil, createFailedStatus(defaultPredictReason, "Failed to reconcile default predictor")
	}

	// extract the virtual service hostname from the predictor hostname
	serviceHostname := extractVirtualServiceHostname(kfsvc.Name, predictDefaultSpec.Hostname)
	serviceURL := fmt.Sprintf("http://%s/v1/models/%s", serviceHostname, kfsvc.Name)

	// add the default route
	defaultWeight := 100 - kfsvc.Spec.CanaryTrafficPercent
	canaryWeight := kfsvc.Spec.CanaryTrafficPercent
	predictRouteDestinations := []istiov1alpha3.HTTPRouteDestination{
		createHTTPRouteDestination(predictDefaultSpec.Hostname, defaultWeight, r.ingressConfig.IngressServiceName),
	}

	// optionally get a destination for canary predict
	if kfsvc.Spec.Canary != nil {
		predictCanarySpec, canaryPredictReason := getPredictStatusConfigurationSpec(kfsvc.Spec.Canary, kfsvc.Status.Canary)
		if predictCanarySpec != nil {
			canaryRouteDestination := createHTTPRouteDestination(predictCanarySpec.Hostname, canaryWeight, r.ingressConfig.IngressServiceName)
			predictRouteDestinations = append(predictRouteDestinations, canaryRouteDestination)
		} else {
			return nil, createFailedStatus(canaryPredictReason, "Failed to reconcile canary predictor")
		}
	}

	// prepare the predict route
	predictRoute := istiov1alpha3.HTTPRoute{
		Match: []istiov1alpha3.HTTPMatchRequest{
			istiov1alpha3.HTTPMatchRequest{
				URI: &istiov1alpha1.StringMatch{
					Prefix: fmt.Sprintf("/v1/models/%s:predict", kfsvc.Name),
				},
			},
		},
		Route: predictRouteDestinations,
	}
	httpRoutes = append(httpRoutes, predictRoute)

	// optionally add the explain route
	explainRouteDestinations := []istiov1alpha3.HTTPRouteDestination{}
	if kfsvc.Spec.Default.Explainer != nil {
		explainDefaultSpec, defaultExplainerReason := getExplainStatusConfigurationSpec(&kfsvc.Spec.Default, kfsvc.Status.Default)
		if explainDefaultSpec != nil {
			routeDefaultDestination := createHTTPRouteDestination(explainDefaultSpec.Hostname, defaultWeight, r.ingressConfig.IngressServiceName)
			explainRouteDestinations = append(explainRouteDestinations, routeDefaultDestination)

			explainCanarySpec, canaryExplainerReason := getExplainStatusConfigurationSpec(kfsvc.Spec.Canary, kfsvc.Status.Canary)
			if explainCanarySpec != nil {
				routeCanaryDestination := createHTTPRouteDestination(explainCanarySpec.Hostname, canaryWeight, r.ingressConfig.IngressServiceName)
				explainRouteDestinations = append(explainRouteDestinations, routeCanaryDestination)
			} else {
				return nil, createFailedStatus(canaryExplainerReason, "Failed to reconcile canary explainer")
			}
		} else {
			return nil, createFailedStatus(defaultExplainerReason, "Failed to reconcile default explainer")
		}

		explainRoute := istiov1alpha3.HTTPRoute{
			Match: []istiov1alpha3.HTTPMatchRequest{
				istiov1alpha3.HTTPMatchRequest{
					URI: &istiov1alpha1.StringMatch{
						Prefix: fmt.Sprintf("/v1/models/%s:explain", kfsvc.Name),
					},
				},
			},
			Route: predictRouteDestinations,
		}
		httpRoutes = append(httpRoutes, explainRoute)
	}

	vs := istiov1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        constants.VirtualServiceName(kfsvc.Name),
			Namespace:   kfsvc.Namespace,
			Labels:      kfsvc.Labels,
			Annotations: kfsvc.Annotations,
		},
		Spec: istiov1alpha3.VirtualServiceSpec{
			Hosts: []string{
				"*",
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

func extractVirtualServiceHostname(name string, predictorHostName string) string {
	index := strings.Index(predictorHostName, ".")
	return name + predictorHostName[index:]
}

func getPredictStatusConfigurationSpec(endpointSpec *v1alpha2.EndpointSpec, endpointStatusMap *v1alpha2.EndpointStatusMap) (*v1alpha2.StatusConfigurationSpec, string) {
	var statusConfigurationSpec *v1alpha2.StatusConfigurationSpec
	if endpointSpec == nil {
		return nil, PredictorSpecMissing
	}
	if predictorStatus, ok := (*endpointStatusMap)[constants.Predictor]; !ok {
		return nil, PredictorStatusUnknown
	} else if len(predictorStatus.Hostname) == 0 {
		return nil, PredictorHostnameUnknown
	} else {
		statusConfigurationSpec = predictorStatus
	}

	// point to transfromer if we have one
	if endpointSpec.Transformer != nil {
		if transformerStatus, ok := (*endpointStatusMap)[constants.Transformer]; !ok {
			return nil, TransformerStatusUnknown
		} else if len(transformerStatus.Hostname) == 0 {
			return nil, TransformerHostnameUnknown
		} else {
			statusConfigurationSpec = transformerStatus
		}
	}

	return statusConfigurationSpec, ExplainerStatusUnknown
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
