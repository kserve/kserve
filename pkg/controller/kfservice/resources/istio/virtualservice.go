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
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	istiov1alpha3 "knative.dev/pkg/apis/istio/v1alpha3"
	istiov1alpha1 "knative.dev/pkg/apis/istio/common/v1alpha1"
)

type VirtualServiceBuilder struct {
}

func NewVirtualServiceBuilder() *VirtualServiceBuilder {
	return &VirtualServiceBuilder{}
}

func (r *VirtualServiceBuilder) CreateVirtualService(kfsvc *v1alpha2.KFService) *istiov1alpha3.VirtualService {

	httpRoutes = []istiov1alpha3.HTTPRoute{}

	// TODO: get these from config
	ingressGateway = "knative-ingress-gateway.knative-serving"
	destinationServiceName := "istio-ingressgateway.istio-system.svc.cluster.local"

	// destination for the default predict is required
	if predictDefaultSpec, err := getPredictStatusConfigurationSpec(&kfsvc.Spec.Default, &kfsvc.Status.Default); err {
		return nil, err
	}
	predictRouteDestinations := []istiov1alpha3.HTTPRouteDestination{
		createHttpRouteDestination(predictDefaultSpec.Hostname, predictDefaultSpec.Weight, destinationServiceName),
	}

	// extract the virtual service hostname from the predictor hostname
	if serviceHostname, err := extractVirtualServiceHostname(kfsvc.Name, predictDefaultSpec.Hostname); err {
		return nil, err
	}

	// optionally get a destination for canary predict
	if predictCanarySpec, err := getPredictStatusConfigurationSpec(&kfsvc.Spec.Canary, &kfsvc.Status.Canary); !err {
		routeDestination := createHttpRouteDestination(predictCanarySpec.Hostname, predictCanarySpec.Weight, destinationServiceName)
		predictRouteDestinations = append(predictRouteDestinations, routeDestination)
	}

	// prepare the predict route
	predictRoute := istiov1alpha3.HTTPRoute{
		Match: []istiov1alpha3.HTTPMatchRequest{
			URI: &istiov1alpha1.StringMatch{
				Prefix: fmt.Sprint("/v1/models/%s:predict", kfsvc.Name),
			},
		},
		Route: predictRouteDestinations,
	}
	httpRoutes = append(httpRoutes, predictRoute)
	
	// optionally add the explain route
	explainRouteDestinations := []istiov1alpha3.HTTPRouteDestination{}
	if explainDefaultSpec := getExplainStatusConfigurationSpec(&kfsvc.Spec.Default, &kfsvc.Status.Default); explainDefaultSpec != nil {
		routeDefaultDestination := createHttpRouteDestination(explainDefaultSpec.Hostname, explainDefaultSpec.Weight, destinationServiceName)
		explainRouteDestinations = append(explainRouteDestinations, routeDefaultDestination)

		if explainCanarySpec := getExplainStatusConfigurationSpec(&kfsvc.Spec.Canary, &kfsvc.Status.Canary); explainDefaultSpec != nil {
			routeCanaryDestination := createHttpRouteDestination(explainCanarySpec.Hostname, explainCanarySpec.Weight, destinationServiceName)
			explainRouteDestinations = append(explainRouteDestinations, routeCanaryDestination)
		}	
	}

	if len(explainRouteDestinations){
		explainRoute := istiov1alpha3.HTTPRoute{
			Match: []istiov1alpha3.HTTPMatchRequest{
				URI: &istiov1alpha1.StringMatch{
					Prefix: fmt.Sprint("/v1/models/%s:explain", kfsvc.Name),
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
				serviceHostname,
			},
			Gateways: []string{
				ingressGateway,
			},
			HTTP: httpRoutes,
		},
	}

	return &vs, nil
}

func extractVirtualServiceHostname(name, predictorHostName) string, error {
	index := strings.Index(predictorHostName, ".")
	if index == -1 {
		return fmt.Errorf("predictorHostName [%s] should be of the format hostname.suffix", predictorHostName)
	}

	return name + predictorHostName[index:]
}

func getPredictStatusConfigurationSpec(endpointSpec *v1alpha2.EndpointSpec, endpointStatusMap *v1alpha2.EndpointStatusMap) StatusConfigurationSpec, error {
	var statusConfigurationSpec v1alpha2.StatusConfigurationSpec
	if endpointSpec.Predictor == nil {
		return fmt.Errorf("Predictor spec missing"), nil
	}
	if predictorStatus, ok := endpointStatusMap[constants.Predictor]; !ok {
		return fmt.Errorf("Predictor status missing"), nil
	} else if predictorStatus.Hostname == nil {
		return fmt.Errorf("Predictor hostname missing"), nil
	} else {
		statusConfigurationSpec = predictorStatus
	}

	// point to transfromer if we have one
	if endpointSpec.Transformer != nil {
		if transformerStatus, ok := endpointStatusMap[constants.Transformer]; !ok {
			return fmt.Errorf("Trasformer status missing"), nil
		} else if transformerStatus.Hostname == nil {
			return fmt.Errorf("Trasformer hostname missing"), nil
		} else {
			statusConfigurationSpec = transformerStatus
		}
	}

	return statusConfigurationSpec, nil
}

func getExplainStatusConfigurationSpec(endpointSpec *v1alpha2.EndpointSpec, endpointStatusMap *v1alpha2.EndpointStatusMap) StatusConfigurationSpec {
	var statusConfigurationSpec v1alpha2.StatusConfigurationSpec
	if endpointSpec.Explainer == nil {
		return nil
	}
	if explainerStatus, ok := endpointStatusMap[constants.Explain]; !ok {
		return nil
	} else if explainerStatus.Hostname == nil {
		return nil
	} 
	
	return explainerStatus
}

func (r *VirtualServiceBuilder) createHttpRouteDestination(targetHost string, weight int, gatewayService string) istiov1alpha3.HTTPRouteDestination {
	httpRouteDestination := istiov1alpha3.HTTPRouteDestination{
		Weight: weight,
		Headers: &istiov1alpha3.Headers{
			Request: &istiov1alpha3.HeaderOperations{
				Set: map[string] string{
					"Host": targetHost,
				},
			},
		},
		Destination: istiov1alpha3.Destination{
			Host: gatewayService,
		},
	},

}
