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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	istiov1alpha1 "knative.dev/pkg/apis/istio/common/v1alpha1"
	istiov1alpha3 "knative.dev/pkg/apis/istio/v1alpha3"
	"testing"
)

func TestCreateVirtualService(t *testing.T) {
	expectedURL := "http://my-model.myDomain/v1/models/my-model"
	predictorHostname := "myPredictor.myDomain"
	transformerHostname := "myTransfomer.myDomain"
	explainerHostname := "myExplainer.myDomain"
	canaryPredictorHostname := "myCanaryPredictor.myDomain"
	canaryTransformerHostname := "myCanaryTransformer.myDomain"
	cases := []struct {
		name            string
		defaultStatus   *v1alpha2.EndpointStatusMap
		canaryStatus    *v1alpha2.EndpointStatusMap
		expectedStatus  *v1alpha2.VirtualServiceStatus
		expectedService *istiov1alpha3.VirtualService
	}{{
		name:            "nil status should not be ready",
		defaultStatus:   nil,
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(PredictorStatusUnknown, PredictorMissingMessage),
		expectedService: nil,
	}, {
		name:            "empty status should not be ready",
		defaultStatus:   &v1alpha2.EndpointStatusMap{},
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(PredictorStatusUnknown, PredictorMissingMessage),
		expectedService: nil,
	}, {
		name: "predictor missing host name",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{},
		},
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(PredictorHostnameUnknown, PredictorMissingMessage),
		expectedService: nil,
	}, {
		name: "found predictor",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
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
			URL:           expectedURL,
			DefaultWeight: 100,
		},
		expectedService: &istiov1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{Name: "my-model"},
			Spec: istiov1alpha3.VirtualServiceSpec{
				Hosts:    []string{"my-model.myDomain"},
				Gateways: []string{"someIngressGateway"},
				HTTP: []istiov1alpha3.HTTPRoute{
					{
						Match: []istiov1alpha3.HTTPMatchRequest{{URI: &istiov1alpha1.StringMatch{Prefix: "/v1/models/my-model:predict"}}},
						Route: []istiov1alpha3.HTTPRouteDestination{
							{
								Destination: istiov1alpha3.Destination{Host: "someIngressServiceName"},
								Weight:      100,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.HeaderOperations{Set: map[string]string{"Host": predictorHostname}},
								},
							},
						},
					},
				},
			},
		},
	}, {
		name: "missing canary predictor",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: &v1alpha2.EndpointStatusMap{
			constants.Predictor: nil,
		},
		expectedStatus:  createFailedStatus(PredictorStatusUnknown, PredictorMissingMessage),
		expectedService: nil,
	}, {
		name: "canary predictor no hostname",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: &v1alpha2.EndpointStatusMap{
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{},
		},
		expectedStatus:  createFailedStatus(PredictorHostnameUnknown, PredictorMissingMessage),
		expectedService: nil,
	}, {
		name: "found default and canary predictor",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: &v1alpha2.EndpointStatusMap{
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
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
			URL:           expectedURL,
			DefaultWeight: 80,
			CanaryWeight:  20,
		},
		expectedService: &istiov1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{Name: "my-model"},
			Spec: istiov1alpha3.VirtualServiceSpec{
				Hosts:    []string{"my-model.myDomain"},
				Gateways: []string{"someIngressGateway"},
				HTTP: []istiov1alpha3.HTTPRoute{
					{
						Match: []istiov1alpha3.HTTPMatchRequest{{URI: &istiov1alpha1.StringMatch{Prefix: "/v1/models/my-model:predict"}}},
						Route: []istiov1alpha3.HTTPRouteDestination{
							{
								Destination: istiov1alpha3.Destination{Host: "someIngressServiceName"},
								Weight:      80,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.HeaderOperations{Set: map[string]string{"Host": predictorHostname}},
								},
							},
							{
								Destination: istiov1alpha3.Destination{Host: "someIngressServiceName"},
								Weight:      20,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.HeaderOperations{Set: map[string]string{"Host": canaryPredictorHostname}},
								},
							},
						},
					},
				},
			},
		},
	}, {
		name: "nil transformer status fails with status unknown",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Transformer: nil,
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(TransformerStatusUnknown, TransformerMissingMessage),
		expectedService: nil,
	}, {
		name: "transformer missing host name",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Transformer: &v1alpha2.StatusConfigurationSpec{},
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(TransformerHostnameUnknown, TransformerMissingMessage),
		expectedService: nil,
	}, {
		name: "default transformer and predictor",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Transformer: &v1alpha2.StatusConfigurationSpec{
				Hostname: transformerHostname,
			},
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
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
			URL:           expectedURL,
			DefaultWeight: 100,
		},
		expectedService: &istiov1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{Name: "my-model"},
			Spec: istiov1alpha3.VirtualServiceSpec{
				Hosts:    []string{"my-model.myDomain"},
				Gateways: []string{"someIngressGateway"},
				HTTP: []istiov1alpha3.HTTPRoute{
					{
						Match: []istiov1alpha3.HTTPMatchRequest{{URI: &istiov1alpha1.StringMatch{Prefix: "/v1/models/my-model:predict"}}},
						Route: []istiov1alpha3.HTTPRouteDestination{
							{
								Destination: istiov1alpha3.Destination{Host: "someIngressServiceName"},
								Weight:      100,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.HeaderOperations{Set: map[string]string{"Host": transformerHostname}},
								},
							},
						},
					},
				},
			},
		},
	}, {
		name: "missing canary transformer",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Transformer: &v1alpha2.StatusConfigurationSpec{
				Hostname: transformerHostname,
			},
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: &v1alpha2.EndpointStatusMap{
			constants.Transformer: &v1alpha2.StatusConfigurationSpec{},
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		expectedStatus:  createFailedStatus(TransformerHostnameUnknown, TransformerMissingMessage),
		expectedService: nil,
	}, {
		name: "canary & default transformer and predictor",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Transformer: &v1alpha2.StatusConfigurationSpec{
				Hostname: transformerHostname,
			},
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus: &v1alpha2.EndpointStatusMap{
			constants.Transformer: &v1alpha2.StatusConfigurationSpec{
				Hostname: canaryTransformerHostname,
			},
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
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
			URL:           expectedURL,
			DefaultWeight: 80,
			CanaryWeight:  20,
		},
		expectedService: &istiov1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{Name: "my-model"},
			Spec: istiov1alpha3.VirtualServiceSpec{
				Hosts:    []string{"my-model.myDomain"},
				Gateways: []string{"someIngressGateway"},
				HTTP: []istiov1alpha3.HTTPRoute{
					{
						Match: []istiov1alpha3.HTTPMatchRequest{{URI: &istiov1alpha1.StringMatch{Prefix: "/v1/models/my-model:predict"}}},
						Route: []istiov1alpha3.HTTPRouteDestination{
							{
								Destination: istiov1alpha3.Destination{Host: "someIngressServiceName"},
								Weight:      80,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.HeaderOperations{Set: map[string]string{"Host": transformerHostname}},
								},
							},
							{
								Destination: istiov1alpha3.Destination{Host: "someIngressServiceName"},
								Weight:      20,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.HeaderOperations{Set: map[string]string{"Host": canaryTransformerHostname}},
								},
							},
						},
					},
				},
			},
		},
	}, {
		name: "nil explainer status fails with status unknown",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Explainer: nil,
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(ExplainerStatusUnknown, ExplainerMissingMessage),
		expectedService: nil,
	}, {
		name: "explainer missing host name",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Explainer: &v1alpha2.StatusConfigurationSpec{},
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
				Hostname: predictorHostname,
			},
		},
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(ExplainerHostnameUnknown, ExplainerMissingMessage),
		expectedService: nil,
	}, {
		name: "default explainer and predictor",
		defaultStatus: &v1alpha2.EndpointStatusMap{
			constants.Explainer: &v1alpha2.StatusConfigurationSpec{
				Hostname: explainerHostname,
			},
			constants.Predictor: &v1alpha2.StatusConfigurationSpec{
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
			URL:           expectedURL,
			DefaultWeight: 100,
		},
		expectedService: &istiov1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{Name: "my-model"},
			Spec: istiov1alpha3.VirtualServiceSpec{
				Hosts:    []string{"my-model.myDomain"},
				Gateways: []string{"someIngressGateway"},
				HTTP: []istiov1alpha3.HTTPRoute{
					{
						Match: []istiov1alpha3.HTTPMatchRequest{{URI: &istiov1alpha1.StringMatch{Prefix: "/v1/models/my-model:predict"}}},
						Route: []istiov1alpha3.HTTPRouteDestination{
							{
								Destination: istiov1alpha3.Destination{Host: "someIngressServiceName"},
								Weight:      100,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.HeaderOperations{Set: map[string]string{"Host": predictorHostname}},
								},
							},
						},
					},
					{
						Match: []istiov1alpha3.HTTPMatchRequest{{URI: &istiov1alpha1.StringMatch{Prefix: "/v1/models/my-model:explain"}}},
						Route: []istiov1alpha3.HTTPRouteDestination{
							{
								Destination: istiov1alpha3.Destination{Host: "someIngressServiceName"},
								Weight:      100,
								Headers: &istiov1alpha3.Headers{
									Request: &istiov1alpha3.HeaderOperations{Set: map[string]string{"Host": explainerHostname}},
								},
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
			testIsvc := &v1alpha2.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-model",
				},
				Spec: v1alpha2.InferenceServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Predictor:   createMockPredictorSpec(tc.defaultStatus),
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
					Predictor:   createMockPredictorSpec(tc.canaryStatus),
					Explainer:   createMockExplainerSpec(tc.canaryStatus),
					Transformer: createMockTransformerSpec(tc.canaryStatus),
				}
				testIsvc.Spec.Canary = canarySpec
				testIsvc.Spec.CanaryTrafficPercent = 20
			}

			serviceBuilder := VirtualServiceBuilder{
				ingressConfig: &IngressConfig{
					IngressGateway:     "someIngressGateway",
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

func createMockPredictorSpec(endpointStatusMap *v1alpha2.EndpointStatusMap) v1alpha2.PredictorSpec {
	return v1alpha2.PredictorSpec{}
}
func createMockExplainerSpec(endpointStatusMap *v1alpha2.EndpointStatusMap) *v1alpha2.ExplainerSpec {
	if endpointStatusMap == nil {
		return nil
	}

	if _, ok := (*endpointStatusMap)[constants.Explainer]; ok {
		return &v1alpha2.ExplainerSpec{}
	}
	return nil
}
func createMockTransformerSpec(endpointStatusMap *v1alpha2.EndpointStatusMap) *v1alpha2.TransformerSpec {
	if endpointStatusMap == nil {
		return nil
	}

	if _, ok := (*endpointStatusMap)[constants.Transformer]; ok {
		return &v1alpha2.TransformerSpec{}
	}
	return nil
}
