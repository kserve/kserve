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
	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	"knative.dev/pkg/network"
	"testing"
)

func TestCreateVirtualService(t *testing.T) {
	serviceName := "my-model"
	namespace := "test"
	domain := "example.com"
	expectedURL := "http://" + constants.InferenceServiceHostName(serviceName, namespace, domain)
	serviceHostName := constants.InferenceServiceHostName(serviceName, namespace, domain)
	serviceInternalHostName := network.GetServiceHostname(serviceName, namespace)
	predictorHostname := constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceName), namespace, domain)
	transformerHostname := constants.InferenceServiceHostName(constants.DefaultTransformerServiceName(serviceName), namespace, domain)
	explainerHostname := constants.InferenceServiceHostName(constants.DefaultExplainerServiceName(serviceName), namespace, domain)
	canaryPredictorHostname := constants.InferenceServiceHostName(constants.CanaryPredictorServiceName(serviceName), namespace, domain)
	canaryTransformerHostname := constants.InferenceServiceHostName(constants.CanaryTransformerServiceName(serviceName), namespace, domain)
	predictorRouteMatch := []*istiov1alpha3.HTTPMatchRequest{
		{
			Authority: &istiov1alpha3.StringMatch{
				MatchType: &istiov1alpha3.StringMatch_Regex{
					Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, domain)),
				},
			},
			Gateways: []string{constants.KnativeIngressGateway},
		},
		{
			Authority: &istiov1alpha3.StringMatch{
				MatchType: &istiov1alpha3.StringMatch_Regex{
					Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
				},
			},
			Gateways: []string{constants.KnativeLocalGateway},
		},
	}
	cases := []struct {
		name            string
		defaultStatus   v1alpha2.ComponentStatusMap
		canaryStatus    v1alpha2.ComponentStatusMap
		expectedStatus  *v1alpha2.VirtualServiceStatus
		expectedService *v1alpha3.VirtualService
	}{{
		name:            "nil status should not be ready",
		defaultStatus:   nil,
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(PredictorStatusUnknown, PredictorMissingMessage),
		expectedService: nil,
	}, {
		name:            "empty status should not be ready",
		defaultStatus:   nil,
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(PredictorStatusUnknown, PredictorMissingMessage),
		expectedService: nil,
	}, {
		name: "predictor missing host name",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Predictor: v1alpha2.StatusConfigurationSpec{},
		},
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(PredictorHostnameUnknown, PredictorMissingMessage),
		expectedService: nil,
	}, {
		name: "found predictor",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: nil,
		expectedStatus: &v1alpha2.VirtualServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha2.RoutesReady,
					Status: corev1.ConditionTrue,
				}},
			},
			URL: expectedURL,
			Address: &duckv1beta1.Addressable{
				URL: &apis.URL{
					Scheme: "http",
					Path:   constants.PredictPath(serviceName),
					Host:   network.GetServiceHostname(serviceName, namespace),
				},
			},
			DefaultWeight: 100,
		},
		expectedService: &v1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Spec: istiov1alpha3.VirtualService{
				Hosts:    []string{serviceHostName, serviceInternalHostName},
				Gateways: []string{constants.KnativeIngressGateway, constants.KnativeLocalGateway},
				Http: []*istiov1alpha3.HTTPRoute{
					{
						Match: predictorRouteMatch,
						Route: []*istiov1alpha3.HTTPRouteDestination{
							{
								Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
								Weight:      100,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
										"Host": network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), namespace)}},
								},
							},
						},
						Retries: &istiov1alpha3.HTTPRetry{
							Attempts:      0,
							PerTryTimeout: nil,
						},
					},
				},
			},
		},
	}, {
		name: "missing canary predictor",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Predictor: v1alpha2.StatusConfigurationSpec{},
		},
		expectedStatus:  createFailedStatus(PredictorHostnameUnknown, PredictorMissingMessage),
		expectedService: nil,
	}, {
		name: "canary predictor no hostname",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Predictor: v1alpha2.StatusConfigurationSpec{},
		},
		expectedStatus:  createFailedStatus(PredictorHostnameUnknown, PredictorMissingMessage),
		expectedService: nil,
	}, {
		name: "found default and canary predictor",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: canaryPredictorHostname,
			},
		},
		expectedStatus: &v1alpha2.VirtualServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha2.RoutesReady,
					Status: corev1.ConditionTrue,
				}},
			},
			URL: expectedURL,
			Address: &duckv1beta1.Addressable{
				URL: &apis.URL{
					Scheme: "http",
					Path:   constants.PredictPath(serviceName),
					Host:   network.GetServiceHostname(serviceName, namespace),
				},
			},
			DefaultWeight: 80,
			CanaryWeight:  20,
		},
		expectedService: &v1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Spec: istiov1alpha3.VirtualService{
				Hosts:    []string{serviceHostName, serviceInternalHostName},
				Gateways: []string{constants.KnativeIngressGateway, constants.KnativeLocalGateway},
				Http: []*istiov1alpha3.HTTPRoute{
					{
						Match: predictorRouteMatch,
						Route: []*istiov1alpha3.HTTPRouteDestination{
							{
								Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
								Weight:      80,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
										"Host": network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), namespace)},
									},
								},
							},
							{
								Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
								Weight:      20,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
										"Host": network.GetServiceHostname(constants.CanaryPredictorServiceName(serviceName), namespace)},
									},
								},
							},
						},
						Retries: &istiov1alpha3.HTTPRetry{
							Attempts:      0,
							PerTryTimeout: nil,
						},
					},
				},
			},
		},
	}, {
		name: "nil transformer status fails with status unknown",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Transformer: v1alpha2.StatusConfigurationSpec{},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(TransformerHostnameUnknown, TransformerMissingMessage),
		expectedService: nil,
	}, {
		name: "transformer missing host name",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Transformer: v1alpha2.StatusConfigurationSpec{},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(TransformerHostnameUnknown, TransformerMissingMessage),
		expectedService: nil,
	}, {
		name: "default transformer and predictor",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Transformer: v1alpha2.StatusConfigurationSpec{
				Hostname: transformerHostname,
			},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: nil,
		expectedStatus: &v1alpha2.VirtualServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha2.RoutesReady,
					Status: corev1.ConditionTrue,
				}},
			},
			URL: expectedURL,
			Address: &duckv1beta1.Addressable{
				URL: &apis.URL{
					Scheme: "http",
					Path:   constants.PredictPath(serviceName),
					Host:   network.GetServiceHostname(serviceName, namespace),
				},
			},
			DefaultWeight: 100,
		},
		expectedService: &v1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Spec: istiov1alpha3.VirtualService{
				Hosts:    []string{serviceHostName, serviceInternalHostName},
				Gateways: []string{constants.KnativeIngressGateway, constants.KnativeLocalGateway},
				Http: []*istiov1alpha3.HTTPRoute{
					{
						Match: predictorRouteMatch,
						Route: []*istiov1alpha3.HTTPRouteDestination{
							{
								Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
								Weight:      100,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
										"Host": network.GetServiceHostname(constants.DefaultTransformerServiceName(serviceName), namespace),
									}},
								},
							},
						},
						Retries: &istiov1alpha3.HTTPRetry{
							Attempts:      0,
							PerTryTimeout: nil,
						},
					},
				},
			},
		},
	}, {
		name: "missing canary transformer",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Transformer: v1alpha2.StatusConfigurationSpec{
				Hostname: transformerHostname,
			},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Transformer: v1alpha2.StatusConfigurationSpec{},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		expectedStatus:  createFailedStatus(TransformerHostnameUnknown, TransformerMissingMessage),
		expectedService: nil,
	}, {
		name: "canary & default transformer and predictor",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Transformer: v1alpha2.StatusConfigurationSpec{
				Hostname: transformerHostname,
			},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Transformer: v1alpha2.StatusConfigurationSpec{
				Hostname: canaryTransformerHostname,
			},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: canaryPredictorHostname,
			},
		},
		expectedStatus: &v1alpha2.VirtualServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha2.RoutesReady,
					Status: corev1.ConditionTrue,
				}},
			},
			URL: expectedURL,
			Address: &duckv1beta1.Addressable{
				URL: &apis.URL{
					Scheme: "http",
					Path:   constants.PredictPath(serviceName),
					Host:   network.GetServiceHostname(serviceName, namespace),
				},
			},
			DefaultWeight: 80,
			CanaryWeight:  20,
		},
		expectedService: &v1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Spec: istiov1alpha3.VirtualService{
				Hosts:    []string{serviceHostName, serviceInternalHostName},
				Gateways: []string{constants.KnativeIngressGateway, constants.KnativeLocalGateway},
				Http: []*istiov1alpha3.HTTPRoute{
					{
						Match: predictorRouteMatch,
						Route: []*istiov1alpha3.HTTPRouteDestination{
							{
								Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
								Weight:      80,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
										"Host": network.GetServiceHostname(constants.DefaultTransformerServiceName(serviceName), namespace)}},
								},
							},
							{
								Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
								Weight:      20,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
										"Host": network.GetServiceHostname(constants.CanaryTransformerServiceName(serviceName), namespace)}},
								},
							},
						},
						Retries: &istiov1alpha3.HTTPRetry{
							Attempts:      0,
							PerTryTimeout: nil,
						},
					},
				},
			},
		},
	}, {
		name: "nil explainer status fails with status unknown",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Explainer: v1alpha2.StatusConfigurationSpec{},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(ExplainerHostnameUnknown, ExplainerMissingMessage),
		expectedService: nil,
	}, {
		name: "explainer missing host name",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Explainer: v1alpha2.StatusConfigurationSpec{},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(ExplainerHostnameUnknown, ExplainerMissingMessage),
		expectedService: nil,
	}, {
		name: "default explainer and predictor",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Explainer: v1alpha2.StatusConfigurationSpec{
				Hostname: explainerHostname,
			},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: nil,
		expectedStatus: &v1alpha2.VirtualServiceStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   v1alpha2.RoutesReady,
					Status: corev1.ConditionTrue,
				}},
			},
			URL: expectedURL,
			Address: &duckv1beta1.Addressable{
				URL: &apis.URL{
					Scheme: "http",
					Path:   constants.PredictPath(serviceName),
					Host:   network.GetServiceHostname(serviceName, namespace),
				},
			},
			DefaultWeight: 100,
		},
		expectedService: &v1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Spec: istiov1alpha3.VirtualService{
				Hosts:    []string{serviceHostName, serviceInternalHostName},
				Gateways: []string{constants.KnativeIngressGateway, constants.KnativeLocalGateway},
				Http: []*istiov1alpha3.HTTPRoute{
					{
						Match: []*istiov1alpha3.HTTPMatchRequest{
							{
								Uri: &istiov1alpha3.StringMatch{
									MatchType: &istiov1alpha3.StringMatch_Regex{
										Regex: constants.ExplainPrefix(),
									},
								},
								Authority: &istiov1alpha3.StringMatch{
									MatchType: &istiov1alpha3.StringMatch_Regex{
										Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, domain)),
									},
								},
								Gateways: []string{constants.KnativeIngressGateway},
							},
							{
								Uri: &istiov1alpha3.StringMatch{
									MatchType: &istiov1alpha3.StringMatch_Regex{
										Regex: constants.ExplainPrefix(),
									},
								},
								Authority: &istiov1alpha3.StringMatch{
									MatchType: &istiov1alpha3.StringMatch_Regex{
										Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
									},
								},
								Gateways: []string{constants.KnativeLocalGateway},
							},
						},
						Route: []*istiov1alpha3.HTTPRouteDestination{
							{
								Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
								Weight:      100,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
										"Host": network.GetServiceHostname(constants.DefaultExplainerServiceName(serviceName), namespace)},
									},
								},
							},
						},
						Retries: &istiov1alpha3.HTTPRetry{
							Attempts:      0,
							PerTryTimeout: nil,
						},
					},
					{
						Match: predictorRouteMatch,
						Route: []*istiov1alpha3.HTTPRouteDestination{
							{
								Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
								Weight:      100,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
										"Host": network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), namespace)},
									},
								},
							},
						},
						Retries: &istiov1alpha3.HTTPRetry{
							Attempts:      0,
							PerTryTimeout: nil,
						},
					},
				},
			},
		},
	},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testIsvc := &v1alpha2.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
				Spec: v1alpha2.InferenceServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Predictor:   createMockPredictorSpec(&tc.defaultStatus),
						Explainer:   createMockExplainerSpec(tc.defaultStatus),
						Transformer: createMockTransformerSpec(tc.defaultStatus),
					},
				},
				Status: v1alpha2.InferenceServiceStatus{
					Default: tc.defaultStatus,
					Canary:  tc.canaryStatus,
				},
			}

			if tc.canaryStatus != nil {
				canarySpec := &v1alpha2.EndpointSpec{
					Predictor:   createMockPredictorSpec(&tc.canaryStatus),
					Explainer:   createMockExplainerSpec(tc.canaryStatus),
					Transformer: createMockTransformerSpec(tc.canaryStatus),
				}
				testIsvc.Spec.Canary = canarySpec
				testIsvc.Spec.CanaryTrafficPercent = v1alpha2.GetIntReference(20)
			}

			serviceBuilder := VirtualServiceBuilder{
				ingressConfig: &IngressConfig{
					IngressGateway:     constants.KnativeIngressGateway,
					IngressServiceName: "someIngressServiceName",
				},
			}
			actualService, actualStatus := serviceBuilder.CreateVirtualService(testIsvc)

			if diff := cmp.Diff(tc.expectedStatus, actualStatus); diff != "" {
				t.Errorf("Test %q unexpected status (-want +got): %v", tc.name, diff)
			}

			if diff := cmp.Diff(tc.expectedService, actualService); diff != "" {
				t.Errorf("Test %q unexpected service (-want +got): %v", tc.name, diff)
			}
		})
	}
}

func TestGetServiceHostname(t *testing.T) {

	testCases := []struct {
		name              string
		expectedHostName  string
		predictorHostName string
	}{
		{
			name:              "using knative domainTemplate: {{.Name}}.{{.Namespace}}.{{.Domain}}",
			expectedHostName:  "kftest.user1.example.com",
			predictorHostName: "kftest-predictor-default.user1.example.com",
		},
		{
			name:              "using knative domainTemplate: {{.Name}}-{{.Namespace}}.{{.Domain}}",
			expectedHostName:  "kftest-user1.example.com",
			predictorHostName: "kftest-predictor-default-user1.example.com",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			testIsvc := createInferenceServiceWithHostname(tt.predictorHostName)
			result, _ := getServiceHostname(testIsvc)
			if diff := cmp.Diff(tt.expectedHostName, result); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
			}
		})
	}
}

func createInferenceServiceWithHostname(hostName string) *v1alpha2.InferenceService {
	return &v1alpha2.InferenceService{
		Status: v1alpha2.InferenceServiceStatus{
			Default: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
				constants.Predictor: v1alpha2.StatusConfigurationSpec{
					Hostname: hostName,
				}},
		},
	}
}

func createMockPredictorSpec(componentStatusMap *v1alpha2.ComponentStatusMap) v1alpha2.PredictorSpec {
	return v1alpha2.PredictorSpec{}
}

func createMockExplainerSpec(componentStatusMap v1alpha2.ComponentStatusMap) *v1alpha2.ExplainerSpec {
	if componentStatusMap == nil {
		return nil
	}

	if _, ok := (*componentStatusMap)[constants.Explainer]; ok {
		return &v1alpha2.ExplainerSpec{}
	}
	return nil
}

func createMockTransformerSpec(componentStatusMap v1alpha2.ComponentStatusMap) *v1alpha2.TransformerSpec {
	if componentStatusMap == nil {
		return nil
	}

	if _, ok := (*componentStatusMap)[constants.Transformer]; ok {
		return &v1alpha2.TransformerSpec{}
	}
	return nil
}
