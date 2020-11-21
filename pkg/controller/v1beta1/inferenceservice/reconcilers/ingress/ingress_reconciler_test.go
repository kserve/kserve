/*
Copyright 2020 kubeflow.org.

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
"github.com/google/go-cmp/cmp"
"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
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
		defaultStatus   *map[constants.InferenceServiceComponent]v1beta1.ComponentStatusSpec
		expectedService *v1alpha3.VirtualService
	}{{
		name:            "nil status should not be ready",
		defaultStatus:   nil,
		expectedService: nil,
	}, {
		name:            "empty status should not be ready",
		defaultStatus:   nil,
		expectedService: nil,
	}, {
		name: "predictor missing host name",
		defaultStatus: &map[constants.InferenceServiceComponent]v1beta1.ComponentStatusSpec{
			constants.Predictor: v1beta1.ComponentStatusSpec{},
		},
		expectedService: nil,
	}, {
		name: "found predictor",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
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
	},{
		name: "nil transformer status fails with status unknown",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Transformer: v1alpha2.StatusConfigurationSpec{},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		expectedService: nil,
	}, {
		name: "transformer missing host name",
		defaultStatus: &map[constants.InferenceServiceComponent]v1alpha2.StatusConfigurationSpec{
			constants.Transformer: v1alpha2.StatusConfigurationSpec{},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
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
		expectedService: nil,
	}, {
		name: "explainer missing host name",
		defaultStatus: &map[constants.InferenceServiceComponent]v1beta1.ComponentStatusSpec{
			constants.Explainer: v1alpha2.StatusConfigurationSpec{},
			constants.Predictor: v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
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
			}
		},
	},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testIsvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
						Predictor:   createMockPredictorSpec(&tc.defaultStatus),
						Explainer:   createMockExplainerSpec(tc.defaultStatus),
						Transformer: createMockTransformerSpec(tc.defaultStatus),
				},
				Status: v1beta1.InferenceServiceStatus{
					Components: tc.defaultStatus,
				},
			}

			ingressConfig := &v1beta1.IngressConfig{
				IngressGateway:     constants.KnativeIngressGateway,
				IngressServiceName: "someIngressServiceName",
			}

			reconciler := NewIngressReconciler(r.Client, r.Scheme, ingressConfig)
			reconciler.Reconcile(testIsvc)
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

