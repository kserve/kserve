/*
Copyright 2021 The KServe Authors.

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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/network"
)

func TestCreateVirtualService(t *testing.T) {
	serviceName := "my-model"
	namespace := "test"
	annotations := map[string]string{"test": "test"}
	isvcAnnotations := map[string]string{"test": "test", "kubectl.kubernetes.io/last-applied-configuration": "test"}
	labels := map[string]string{"test": "test"}
	domain := "example.com"
	serviceHostName := constants.InferenceServiceHostName(serviceName, namespace, domain)
	serviceInternalHostName := network.GetServiceHostname(serviceName, namespace)
	predictorHostname := constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceName), namespace, domain)
	transformerHostname := constants.InferenceServiceHostName(constants.DefaultTransformerServiceName(serviceName), namespace, domain)
	explainerHostname := constants.InferenceServiceHostName(constants.DefaultExplainerServiceName(serviceName), namespace, domain)
	predictorRouteMatch := []*istiov1alpha3.HTTPMatchRequest{
		{
			Authority: &istiov1alpha3.StringMatch{
				MatchType: &istiov1alpha3.StringMatch_Regex{
					Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
				},
			},
			Gateways: []string{constants.KnativeLocalGateway},
		},
		{
			Authority: &istiov1alpha3.StringMatch{
				MatchType: &istiov1alpha3.StringMatch_Regex{
					Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, domain)),
				},
			},
			Gateways: []string{constants.KnativeIngressGateway},
		},
	}
	cases := []struct {
		name            string
		componentStatus *v1beta1.InferenceServiceStatus
		expectedService *v1alpha3.VirtualService
	}{{
		name:            "nil status should not be ready",
		componentStatus: nil,
		expectedService: nil,
	}, {
		name: "predictor missing url",
		componentStatus: &v1beta1.InferenceServiceStatus{
			Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
				v1beta1.PredictorComponent: {},
			},
		},
		expectedService: nil,
	}, {
		name: "found predictor status",
		componentStatus: &v1beta1.InferenceServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{
						Type:   v1beta1.PredictorReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
			Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
				v1beta1.PredictorComponent: {
					URL: &apis.URL{
						Scheme: "http",
						Host:   predictorHostname,
					},
					Address: &duckv1.Addressable{
						URL: &apis.URL{
							Scheme: "http",
							Host:   network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), namespace),
						},
					},
				},
			},
		},
		expectedService: &v1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
			Spec: istiov1alpha3.VirtualService{
				Hosts:    []string{serviceInternalHostName, serviceHostName},
				Gateways: []string{constants.KnativeLocalGateway, constants.KnativeIngressGateway},
				Http: []*istiov1alpha3.HTTPRoute{
					{
						Match: predictorRouteMatch,
						Route: []*istiov1alpha3.HTTPRouteDestination{
							{
								Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
								Weight:      100,
							},
						},
						Headers: &istiov1alpha3.Headers{
							Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
								"Host": network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), namespace)}},
						},
					},
				},
			},
		},
	}, {
		name: "local cluster predictor",
		componentStatus: &v1beta1.InferenceServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{
						Type:   v1beta1.PredictorReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
			Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
				v1beta1.PredictorComponent: {
					URL: &apis.URL{
						Scheme: "http",
						Host:   network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), namespace),
					},
					Address: &duckv1.Addressable{
						URL: &apis.URL{
							Scheme: "http",
							Host:   network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), namespace),
						},
					},
				},
			},
		},
		expectedService: &v1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
			Spec: istiov1alpha3.VirtualService{
				Hosts:    []string{serviceInternalHostName},
				Gateways: []string{constants.KnativeLocalGateway},
				Http: []*istiov1alpha3.HTTPRoute{
					{
						Match: []*istiov1alpha3.HTTPMatchRequest{
							{
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
							},
						},
						Headers: &istiov1alpha3.Headers{
							Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
								"Host": network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), namespace)}},
						},
					},
				},
			},
		},
	},
		{
			name: "nil transformer status fails with status unknown",
			componentStatus: &v1beta1.InferenceServiceStatus{
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.TransformerComponent: {},
					v1beta1.PredictorComponent: {
						URL: &apis.URL{
							Scheme: "http",
							Host:   predictorHostname,
						},
						Address: &duckv1.Addressable{
							URL: &apis.URL{
								Scheme: "http",
								Host:   network.GetServiceHostname(serviceName, namespace),
							},
						},
					},
				},
			},
			expectedService: nil,
		}, {
			name: "found transformer and predictor status",
			componentStatus: &v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.PredictorReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   v1beta1.TransformerReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.TransformerComponent: {
						URL: &apis.URL{
							Scheme: "http",
							Host:   transformerHostname,
						},
						Address: &duckv1.Addressable{
							URL: &apis.URL{
								Scheme: "http",
								Host:   network.GetServiceHostname(constants.DefaultTransformerServiceName(serviceName), namespace),
							},
						},
					},
					v1beta1.PredictorComponent: {
						URL: &apis.URL{
							Scheme: "http",
							Host:   predictorHostname,
						},
						Address: &duckv1.Addressable{
							URL: &apis.URL{
								Scheme: "http",
								Host:   network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), namespace),
							},
						},
					},
				},
			},
			expectedService: &v1alpha3.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1alpha3.VirtualService{
					Hosts:    []string{serviceInternalHostName, serviceHostName},
					Gateways: []string{constants.KnativeLocalGateway, constants.KnativeIngressGateway},
					Http: []*istiov1alpha3.HTTPRoute{
						{
							Match: predictorRouteMatch,
							Route: []*istiov1alpha3.HTTPRouteDestination{
								{
									Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1alpha3.Headers{
								Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
									"Host": network.GetServiceHostname(constants.DefaultTransformerServiceName(serviceName), namespace),
								}},
							},
						},
					},
				},
			},
		}, {
			name: "found transformer and predictor status",
			componentStatus: &v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.PredictorReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   v1beta1.TransformerReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.TransformerComponent: {
						URL: &apis.URL{
							Scheme: "http",
							Host:   transformerHostname,
						},
						Address: &duckv1.Addressable{
							URL: &apis.URL{
								Scheme: "http",
								Host:   network.GetServiceHostname(constants.DefaultTransformerServiceName(serviceName), namespace),
							},
						},
					},
					v1beta1.PredictorComponent: {
						URL: &apis.URL{
							Scheme: "http",
							Host:   predictorHostname,
						},
						Address: &duckv1.Addressable{
							URL: &apis.URL{
								Scheme: "http",
								Host:   network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), namespace),
							},
						},
					},
				},
			},
			expectedService: &v1alpha3.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1alpha3.VirtualService{
					Hosts:    []string{serviceInternalHostName, serviceHostName},
					Gateways: []string{constants.KnativeLocalGateway, constants.KnativeIngressGateway},
					Http: []*istiov1alpha3.HTTPRoute{
						{
							Match: predictorRouteMatch,
							Route: []*istiov1alpha3.HTTPRouteDestination{
								{
									Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1alpha3.Headers{
								Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
									"Host": network.GetServiceHostname(constants.DefaultTransformerServiceName(serviceName), namespace),
								}},
							},
						},
					},
				},
			},
		}, {
			name: "nil explainer status fails with status unknown",
			componentStatus: &v1beta1.InferenceServiceStatus{
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.ExplainerComponent: {},
					v1beta1.PredictorComponent: {
						URL: &apis.URL{
							Scheme: "http",
							Host:   predictorHostname,
						},
						Address: &duckv1.Addressable{
							URL: &apis.URL{
								Scheme: "http",
								Host:   network.GetServiceHostname(serviceName, namespace),
							},
						},
					},
				},
			},
			expectedService: nil,
		}, {
			name: "found explainer and predictor status",
			componentStatus: &v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.PredictorReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   v1beta1.ExplainerReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.ExplainerComponent: {
						URL: &apis.URL{
							Scheme: "http",
							Host:   explainerHostname,
						},
						Address: &duckv1.Addressable{
							URL: &apis.URL{
								Scheme: "http",
								Host:   network.GetServiceHostname(constants.DefaultExplainerServiceName(serviceName), namespace),
							},
						},
					},
					v1beta1.PredictorComponent: {
						URL: &apis.URL{
							Scheme: "http",
							Host:   predictorHostname,
						},
						Address: &duckv1.Addressable{
							URL: &apis.URL{
								Scheme: "http",
								Host:   network.GetServiceHostname(serviceName, namespace),
							},
						},
					},
				},
			},
			expectedService: &v1alpha3.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1alpha3.VirtualService{
					Hosts:    []string{serviceInternalHostName, serviceHostName},
					Gateways: []string{constants.KnativeLocalGateway, constants.KnativeIngressGateway},
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
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
										},
									},
									Gateways: []string{constants.KnativeLocalGateway},
								},
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
							},
							Route: []*istiov1alpha3.HTTPRouteDestination{
								{
									Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1alpha3.Headers{
								Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
									"Host": network.GetServiceHostname(constants.DefaultExplainerServiceName(serviceName), namespace)},
								},
							},
						},
						{
							Match: predictorRouteMatch,
							Route: []*istiov1alpha3.HTTPRouteDestination{
								{
									Destination: &istiov1alpha3.Destination{Host: constants.LocalGatewayHost, Port: &istiov1alpha3.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1alpha3.Headers{
								Request: &istiov1alpha3.Headers_HeaderOperations{Set: map[string]string{
									"Host": network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), namespace)},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testIsvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
				},
			}
			if tc.componentStatus != nil {
				testIsvc.Status = *tc.componentStatus
			}
			if _, ok := testIsvc.Status.Components[v1beta1.TransformerComponent]; ok {
				testIsvc.Spec.Transformer = &v1beta1.TransformerSpec{}
			}
			if _, ok := testIsvc.Status.Components[v1beta1.ExplainerComponent]; ok {
				testIsvc.Spec.Explainer = &v1beta1.ExplainerSpec{}
			}
			ingressConfig := &v1beta1.IngressConfig{
				IngressGateway:          constants.KnativeIngressGateway,
				IngressServiceName:      "someIngressServiceName",
				LocalGateway:            constants.KnativeLocalGateway,
				LocalGatewayServiceName: "knative-local-gateway.istio-system.svc.cluster.local",
			}

			actualService := createIngress(testIsvc, ingressConfig)
			if diff := cmp.Diff(tc.expectedService, actualService); diff != "" {
				t.Errorf("Test %q unexpected status (-want +got): %v", tc.name, diff)
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
			result := getServiceHost(testIsvc)
			if diff := cmp.Diff(tt.expectedHostName, result); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
			}
		})
	}
}

func createInferenceServiceWithHostname(hostName string) *v1beta1.InferenceService {
	return &v1beta1.InferenceService{
		Status: v1beta1.InferenceServiceStatus{
			Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
				v1beta1.PredictorComponent: {
					URL: &apis.URL{
						Scheme: "http",
						Host:   hostName,
					},
				},
			},
		},
	}
}
