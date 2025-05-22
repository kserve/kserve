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
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"
	gomegaTypes "github.com/onsi/gomega/types"
	"google.golang.org/protobuf/testing/protocmp"
	istiov1beta1 "istio.io/api/networking/v1beta1"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestCreateVirtualService(t *testing.T) {
	serviceName := "my-model"
	namespace := "test"
	annotations := map[string]string{"test": "test"}
	isvcAnnotations := map[string]string{"test": "test", "kubectl.kubernetes.io/last-applied-configuration": "test"}
	labels := map[string]string{"test": "test"}
	knativeLocalGatewayService := "someIngressServiceName"
	domain := "example.com"
	additionalDomain := "my-additional-domain.com"
	additionalSecondDomain := "my-second-additional-domain.com"
	serviceHostName := constants.InferenceServiceHostName(serviceName, namespace, domain)
	serviceInternalHostName := network.GetServiceHostname(serviceName, namespace)
	predictorHostname := constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceName), namespace, domain)
	transformerHostname := constants.InferenceServiceHostName(constants.DefaultTransformerServiceName(serviceName), namespace, domain)
	explainerHostname := constants.InferenceServiceHostName(constants.DefaultExplainerServiceName(serviceName), namespace, domain)
	predictorRouteMatch := []*istiov1beta1.HTTPMatchRequest{
		{
			Authority: &istiov1beta1.StringMatch{
				MatchType: &istiov1beta1.StringMatch_Regex{
					Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
				},
			},
			Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
		},
		{
			Authority: &istiov1beta1.StringMatch{
				MatchType: &istiov1beta1.StringMatch_Regex{
					Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, domain)),
				},
			},
			Gateways: []string{constants.KnativeIngressGateway},
		},
	}
	defaultInferenceServiceConfig := &v1beta1.InferenceServicesConfig{
		Explainers:                      v1beta1.ExplainersConfig{},
		ServiceAnnotationDisallowedList: constants.ServiceAnnotationDisallowedList,
		ServiceLabelDisallowedList:      constants.RevisionTemplateLabelDisallowedList,
	}
	cases := []struct {
		name            string
		isvc            *v1beta1.InferenceService
		ingressConfig   *v1beta1.IngressConfig
		domainList      *[]string
		useDefault      bool
		componentStatus *v1beta1.InferenceServiceStatus
		expectedService *istioclientv1beta1.VirtualService
	}{
		{
			name:            "nil status should not be ready",
			componentStatus: nil,
			expectedService: nil,
		},
		{
			name: "predictor missing url",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: false,
			componentStatus: &v1beta1.InferenceServiceStatus{
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {},
				},
			},
			expectedService: nil,
		},
		{
			name: "found predictor status",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: false,
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
								Host:   network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
							},
						},
					},
				},
			},
			expectedService: &istioclientv1beta1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1beta1.VirtualService{
					Hosts:    []string{serviceInternalHostName, serviceHostName},
					Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway, constants.KnativeIngressGateway},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: predictorRouteMatch,
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{Set: map[string]string{
									"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
									"KServe-Isvc-Name":      serviceName,
									"KServe-Isvc-Namespace": namespace,
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "local cluster predictor",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: false,
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
							Host:   network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
						},
						Address: &duckv1.Addressable{
							URL: &apis.URL{
								Scheme: "http",
								Host:   network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
							},
						},
					},
				},
			},
			expectedService: &istioclientv1beta1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1beta1.VirtualService{
					Hosts:    []string{serviceInternalHostName},
					Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
										},
									},
									Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
								},
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{Set: map[string]string{
									"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
									"KServe-Isvc-Name":      serviceName,
									"KServe-Isvc-Namespace": namespace,
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "nil transformer status fails with status unknown",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: false,
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
		},
		{
			name: "found transformer and predictor status",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: false,
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
								Host:   network.GetServiceHostname(constants.TransformerServiceName(serviceName), namespace),
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
								Host:   network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
							},
						},
					},
				},
			},
			expectedService: &istioclientv1beta1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1beta1.VirtualService{
					Hosts:    []string{serviceInternalHostName, serviceHostName},
					Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway, constants.KnativeIngressGateway},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: predictorRouteMatch,
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{Set: map[string]string{
									"Host":                  network.GetServiceHostname(constants.TransformerServiceName(serviceName), namespace),
									"KServe-Isvc-Name":      serviceName,
									"KServe-Isvc-Namespace": namespace,
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "found transformer and predictor status",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: false,
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
								Host:   network.GetServiceHostname(constants.TransformerServiceName(serviceName), namespace),
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
			expectedService: &istioclientv1beta1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1beta1.VirtualService{
					Hosts:    []string{serviceInternalHostName, serviceHostName},
					Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway, constants.KnativeIngressGateway},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: predictorRouteMatch,
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{Set: map[string]string{
									"Host":                  network.GetServiceHostname(constants.TransformerServiceName(serviceName), namespace),
									"KServe-Isvc-Name":      serviceName,
									"KServe-Isvc-Namespace": namespace,
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "nil explainer status fails with status unknown",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: false,
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
		},
		{
			name: "found explainer and predictor status",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: false,
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
								Host:   network.GetServiceHostname(constants.ExplainerServiceName(serviceName), namespace),
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
			expectedService: &istioclientv1beta1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1beta1.VirtualService{
					Hosts:    []string{serviceInternalHostName, serviceHostName},
					Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway, constants.KnativeIngressGateway},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.ExplainPrefix(),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
										},
									},
									Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
								},
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.ExplainPrefix(),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, domain)),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{
									Set: map[string]string{
										"Host":                  network.GetServiceHostname(constants.ExplainerServiceName(serviceName), namespace),
										"KServe-Isvc-Name":      serviceName,
										"KServe-Isvc-Namespace": namespace,
									},
								},
							},
						},
						{
							Match: predictorRouteMatch,
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{
									Set: map[string]string{
										"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
										"KServe-Isvc-Name":      serviceName,
										"KServe-Isvc-Namespace": namespace,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "found predictor status with path template",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
				UrlScheme:                  "http",
				IngressDomain:              "my-domain.com",
				PathTemplate:               "/serving/{{ .Namespace }}/{{ .Name }}",
				DisableIstioVirtualHost:    false,
			},
			useDefault: false,
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
								Host:   network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
							},
						},
					},
				},
			},
			expectedService: &istioclientv1beta1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1beta1.VirtualService{
					Hosts:    []string{serviceInternalHostName, serviceHostName, "my-domain.com"},
					Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway, constants.KnativeIngressGateway},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
										},
									},
									Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
								},
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, domain)),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{Set: map[string]string{
									"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
									"KServe-Isvc-Name":      serviceName,
									"KServe-Isvc-Namespace": namespace,
								}},
							},
						},
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Prefix{
											Prefix: fmt.Sprintf("/serving/%s/%s/", namespace, serviceName),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp("my-domain.com"),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Exact{
											Exact: fmt.Sprintf("/serving/%s/%s", namespace, serviceName),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp("my-domain.com"),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
							},
							Rewrite: &istiov1beta1.HTTPRewrite{
								Uri: "/",
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								createHTTPRouteDestination(knativeLocalGatewayService),
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{
									Set: map[string]string{
										"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
										"KServe-Isvc-Name":      serviceName,
										"KServe-Isvc-Namespace": namespace,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "found predictor status with the additional ingress domains",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
				UrlScheme:                  "http",
				IngressDomain:              "my-domain.com",
				AdditionalIngressDomains:   &[]string{additionalDomain, additionalSecondDomain},
				PathTemplate:               "/serving/{{ .Namespace }}/{{ .Name }}",
				DisableIstioVirtualHost:    false,
			},
			domainList: &[]string{"my-domain-1.com", "example.com"},
			useDefault: false,
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
								Host:   network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
							},
						},
					},
				},
			},
			expectedService: &istioclientv1beta1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1beta1.VirtualService{
					Hosts: []string{
						serviceInternalHostName, serviceHostName, "my-domain.com",
						"my-model.test.my-additional-domain.com", "my-model.test.my-second-additional-domain.com",
					},
					Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway, constants.KnativeIngressGateway},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
										},
									},
									Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
								},
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, domain)),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, additionalDomain)),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, additionalSecondDomain)),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{Set: map[string]string{
									"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
									"KServe-Isvc-Name":      serviceName,
									"KServe-Isvc-Namespace": namespace,
								}},
							},
						},
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Prefix{
											Prefix: fmt.Sprintf("/serving/%s/%s/", namespace, serviceName),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp("my-domain.com"),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Exact{
											Exact: fmt.Sprintf("/serving/%s/%s", namespace, serviceName),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp("my-domain.com"),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
							},
							Rewrite: &istiov1beta1.HTTPRewrite{
								Uri: "/",
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								createHTTPRouteDestination(knativeLocalGatewayService),
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{
									Set: map[string]string{
										"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
										"KServe-Isvc-Name":      serviceName,
										"KServe-Isvc-Namespace": namespace,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "found predictor status with the additional ingress domains with duplication",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
				UrlScheme:                  "http",
				IngressDomain:              "my-domain.com",
				AdditionalIngressDomains:   &[]string{"example.com", additionalDomain, additionalSecondDomain, additionalDomain},
				PathTemplate:               "/serving/{{ .Namespace }}/{{ .Name }}",
				DisableIstioVirtualHost:    false,
			},
			domainList: &[]string{"my-domain-1.com", "example.com"},
			useDefault: false,
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
								Host:   network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
							},
						},
					},
				},
			},
			expectedService: &istioclientv1beta1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1beta1.VirtualService{
					Hosts: []string{
						serviceInternalHostName, serviceHostName, "my-domain.com",
						"my-model.test.my-additional-domain.com", "my-model.test.my-second-additional-domain.com",
					},
					Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway, constants.KnativeIngressGateway},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
										},
									},
									Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
								},
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, domain)),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, additionalDomain)),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, additionalSecondDomain)),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{
										Host: knativeLocalGatewayService,
										Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort},
									},
									Weight: 100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{Set: map[string]string{
									"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
									"KServe-Isvc-Name":      serviceName,
									"KServe-Isvc-Namespace": namespace,
								}},
							},
						},
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Prefix{
											Prefix: fmt.Sprintf("/serving/%s/%s/", namespace, serviceName),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp("my-domain.com"),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Exact{
											Exact: fmt.Sprintf("/serving/%s/%s", namespace, serviceName),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp("my-domain.com"),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
							},
							Rewrite: &istiov1beta1.HTTPRewrite{
								Uri: "/",
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								createHTTPRouteDestination(knativeLocalGatewayService),
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{
									Set: map[string]string{
										"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
										"KServe-Isvc-Name":      serviceName,
										"KServe-Isvc-Namespace": namespace,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "found predictor and explainer status with path template",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    knativeLocalGatewayService,
				UrlScheme:                  "http",
				IngressDomain:              "my-domain.com",
				PathTemplate:               "/serving/{{ .Namespace }}/{{ .Name }}",
				DisableIstioVirtualHost:    false,
			},
			useDefault: false,
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
								Host:   network.GetServiceHostname(constants.ExplainerServiceName(serviceName), namespace),
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
								Host:   network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
							},
						},
					},
				},
			},
			expectedService: &istioclientv1beta1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1beta1.VirtualService{
					Hosts:    []string{serviceInternalHostName, serviceHostName, "my-domain.com"},
					Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway, constants.KnativeIngressGateway},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.ExplainPrefix(),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
										},
									},
									Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
								},
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.ExplainPrefix(),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, domain)),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{
									Set: map[string]string{
										"Host":                        network.GetServiceHostname(constants.ExplainerServiceName(serviceName), namespace),
										constants.IsvcNameHeader:      serviceName,
										constants.IsvcNamespaceHeader: namespace,
									},
								},
							},
						},
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
										},
									},
									Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
								},
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(constants.InferenceServiceHostName(serviceName, namespace, domain)),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{Set: map[string]string{
									"Host":                        network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
									constants.IsvcNameHeader:      serviceName,
									constants.IsvcNamespaceHeader: namespace,
								}},
							},
						},
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: fmt.Sprintf("/serving/%s/%s%s", namespace, serviceName, constants.PathBasedExplainPrefix()),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp("my-domain.com"),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
							},
							Rewrite: &istiov1beta1.HTTPRewrite{
								UriRegexRewrite: &istiov1beta1.RegexRewrite{
									Match:   fmt.Sprintf("/serving/%s/%s%s", namespace, serviceName, constants.PathBasedExplainPrefix()),
									Rewrite: `\1`,
								},
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								createHTTPRouteDestination(knativeLocalGatewayService),
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{
									Set: map[string]string{
										"Host":                        network.GetServiceHostname(constants.ExplainerServiceName(serviceName), namespace),
										constants.IsvcNameHeader:      serviceName,
										constants.IsvcNamespaceHeader: namespace,
									},
								},
							},
						},
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Prefix{
											Prefix: fmt.Sprintf("/serving/%s/%s/", namespace, serviceName),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp("my-domain.com"),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
								{
									Uri: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Exact{
											Exact: fmt.Sprintf("/serving/%s/%s", namespace, serviceName),
										},
									},
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp("my-domain.com"),
										},
									},
									Gateways: []string{constants.KnativeIngressGateway},
								},
							},
							Rewrite: &istiov1beta1.HTTPRewrite{
								Uri: "/",
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								createHTTPRouteDestination(knativeLocalGatewayService),
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{
									Set: map[string]string{
										"Host":                        network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
										constants.IsvcNameHeader:      serviceName,
										constants.IsvcNamespaceHeader: namespace,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "found predictor status with default suffix",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: true,
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
			expectedService: &istioclientv1beta1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1beta1.VirtualService{
					Hosts:    []string{serviceInternalHostName, serviceHostName},
					Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway, constants.KnativeIngressGateway},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: predictorRouteMatch,
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{Set: map[string]string{
									"Host":                  network.GetServiceHostname(constants.DefaultPredictorServiceName(serviceName), namespace),
									"KServe-Isvc-Name":      serviceName,
									"KServe-Isvc-Namespace": namespace,
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "found transformer and predictor status with default suffix",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: true,
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
			expectedService: &istioclientv1beta1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: labels},
				Spec: istiov1beta1.VirtualService{
					Hosts:    []string{serviceInternalHostName, serviceHostName},
					Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway, constants.KnativeIngressGateway},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: predictorRouteMatch,
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{Set: map[string]string{
									"Host":                  network.GetServiceHostname(constants.DefaultTransformerServiceName(serviceName), namespace),
									"KServe-Isvc-Name":      serviceName,
									"KServe-Isvc-Namespace": namespace,
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "transformer is not ready",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: true,
			componentStatus: &v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.PredictorReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   v1beta1.TransformerReady,
							Status: corev1.ConditionFalse,
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
								Host:   network.GetServiceHostname(constants.TransformerServiceName(serviceName), namespace),
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
			expectedService: nil,
		},
		{
			name: "nil explainer status fails with status unknown",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: false,
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
		},
		{
			name: "explainer is not ready",
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: false,
			componentStatus: &v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.PredictorReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   v1beta1.ExplainerReady,
							Status: corev1.ConditionFalse,
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
								Host:   network.GetServiceHostname(constants.ExplainerServiceName(serviceName), namespace),
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
			expectedService: nil,
		},
		{
			name: "isvc labelled with cluster local visibility",
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels: map[string]string{
						constants.VisibilityLabel: constants.ClusterLocalVisibility,
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressGateway:             constants.KnativeIngressGateway,
				KnativeLocalGatewayService: knativeLocalGatewayService,
				LocalGateway:               constants.KnativeLocalGateway,
				LocalGatewayServiceName:    "knative-local-gateway.istio-system.svc.cluster.local",
			},
			useDefault: false,
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
								Host:   network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
							},
						},
					},
				},
			},
			expectedService: &istioclientv1beta1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace, Annotations: annotations, Labels: map[string]string{
					constants.VisibilityLabel: constants.ClusterLocalVisibility,
				}},
				Spec: istiov1beta1.VirtualService{
					Hosts:    []string{serviceInternalHostName},
					Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
					Http: []*istiov1beta1.HTTPRoute{
						{
							Match: []*istiov1beta1.HTTPMatchRequest{
								{
									Authority: &istiov1beta1.StringMatch{
										MatchType: &istiov1beta1.StringMatch_Regex{
											Regex: constants.HostRegExp(network.GetServiceHostname(serviceName, namespace)),
										},
									},
									Gateways: []string{constants.KnativeLocalGateway, constants.IstioMeshGateway},
								},
							},
							Route: []*istiov1beta1.HTTPRouteDestination{
								{
									Destination: &istiov1beta1.Destination{Host: knativeLocalGatewayService, Port: &istiov1beta1.PortSelector{Number: constants.CommonDefaultHttpPort}},
									Weight:      100,
								},
							},
							Headers: &istiov1beta1.Headers{
								Request: &istiov1beta1.Headers_HeaderOperations{Set: map[string]string{
									"Host":                  network.GetServiceHostname(constants.PredictorServiceName(serviceName), namespace),
									"KServe-Isvc-Name":      serviceName,
									"KServe-Isvc-Namespace": namespace,
								}},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var testIsvc *v1beta1.InferenceService

			if tc.isvc == nil {
				testIsvc = &v1beta1.InferenceService{
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
			} else {
				testIsvc = tc.isvc
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

			actualService := createIngress(testIsvc, tc.useDefault, tc.ingressConfig, tc.domainList, defaultInferenceServiceConfig)
			if diff := cmp.Diff(tc.expectedService.DeepCopy(), actualService.DeepCopy(), protocmp.Transform()); diff != "" {
				t.Errorf("Test %q unexpected status (-want +got): %v", tc.name, diff)
			}
		})
	}
}

func TestGetServiceHost(t *testing.T) {
	testCases := []struct {
		name             string
		isvc             *v1beta1.InferenceService
		expectedHostName string
	}{
		{
			name: "using knative domainTemplate: {{.Name}}.{{.Namespace}}.{{.Domain}}",
			isvc: &v1beta1.InferenceService{
				Status: v1beta1.InferenceServiceStatus{
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: &apis.URL{
								Scheme: "http",
								Host:   "kftest-predictor-default.user1.example.com",
							},
						},
					},
				},
			},
			expectedHostName: "kftest.user1.example.com",
		},
		{
			name: "using knative domainTemplate: {{.Name}}-{{.Namespace}}.{{.Domain}}",
			isvc: &v1beta1.InferenceService{
				Status: v1beta1.InferenceServiceStatus{
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: &apis.URL{
								Scheme: "http",
								Host:   "kftest-predictor-default-user1.example.com",
							},
						},
					},
				},
			},
			expectedHostName: "kftest-user1.example.com",
		},
		{
			name: "predictor status not available",
			isvc: &v1beta1.InferenceService{
				Status: v1beta1.InferenceServiceStatus{
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{},
				},
			},
			expectedHostName: "",
		},
		{
			name: "transformer status not available",
			isvc: &v1beta1.InferenceService{
				Spec: v1beta1.InferenceServiceSpec{
					Predictor:   v1beta1.PredictorSpec{},
					Transformer: &v1beta1.TransformerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: &apis.URL{
								Scheme: "http",
								Host:   "kftest-predictor-default.user1.example.com",
							},
						},
					},
				},
			},
			expectedHostName: "",
		},
		{
			name: "transformer without default suffix",
			isvc: &v1beta1.InferenceService{
				Spec: v1beta1.InferenceServiceSpec{
					Predictor:   v1beta1.PredictorSpec{},
					Transformer: &v1beta1.TransformerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: &apis.URL{
								Scheme: "http",
								Host:   "kftest-predictor-default.user1.example.com",
							},
						},
						v1beta1.TransformerComponent: {
							URL: &apis.URL{
								Scheme: "http",
								Host:   "kserveTest-transformer.user1.example.com",
							},
						},
					},
				},
			},
			expectedHostName: "kserveTest.user1.example.com",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result := getServiceHost(tt.isvc)
			if diff := cmp.Diff(tt.expectedHostName, result); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
			}
		})
	}
}

func TestGetServiceUrl(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	serviceName := "my-model"
	namespace := "test"
	isvcAnnotations := map[string]string{"test": "test", "kubectl.kubernetes.io/last-applied-configuration": "test"}
	labels := map[string]string{"test": "test"}
	defaultPredictorUrl, _ := url.Parse("http://my-model-predictor-default.example.com")
	defaultTransformerUrl, _ := url.Parse("http://my-model-transformer-default.example.com")
	predictorUrl, _ := url.Parse("http://my-model-predictor.example.com")
	transformerUrl, _ := url.Parse("http://my-model-transformer.example.com")

	cases := map[string]struct {
		isvc          *v1beta1.InferenceService
		ingressConfig *v1beta1.IngressConfig
		matcher       gomegaTypes.GomegaMatcher
	}{
		"component is empty": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				UrlScheme:               "http",
				DisableIstioVirtualHost: false,
			},
			matcher: gomega.Equal(""),
		},
		"predictor url is empty": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status:  duckv1.Status{},
					Address: nil,
					URL:     nil,
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {},
					},
					ModelStatus: v1beta1.ModelStatus{},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				UrlScheme:               "http",
				DisableIstioVirtualHost: false,
			},
			matcher: gomega.Equal(""),
		},
		"predictor url is not empty": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status:  duckv1.Status{},
					Address: nil,
					URL:     nil,
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: (*apis.URL)(defaultPredictorUrl),
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				UrlScheme:               "http",
				DisableIstioVirtualHost: false,
			},
			matcher: gomega.Equal("http://my-model.example.com"),
		},
		"transformer is not empty": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
					Transformer: &v1beta1.TransformerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status:  duckv1.Status{},
					Address: nil,
					URL:     nil,
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: (*apis.URL)(defaultPredictorUrl),
						},
						v1beta1.TransformerComponent: {
							URL: (*apis.URL)(defaultTransformerUrl),
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				UrlScheme:               "http",
				DisableIstioVirtualHost: false,
			},
			matcher: gomega.Equal("http://my-model.example.com"),
		},
		"predictor is empty": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{},
				Status: v1beta1.InferenceServiceStatus{
					Status:     duckv1.Status{},
					Address:    nil,
					URL:        nil,
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				UrlScheme:               "http",
				DisableIstioVirtualHost: false,
			},
			matcher: gomega.Equal(""),
		},
		"transformer status is not available": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
					Transformer: &v1beta1.TransformerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status:  duckv1.Status{},
					Address: nil,
					URL:     nil,
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: (*apis.URL)(defaultPredictorUrl),
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				UrlScheme:               "http",
				DisableIstioVirtualHost: false,
			},
			matcher: gomega.Equal(""),
		},
		"transformer url is empty": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
					Transformer: &v1beta1.TransformerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status:  duckv1.Status{},
					Address: nil,
					URL:     nil,
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: (*apis.URL)(defaultPredictorUrl),
						},
						v1beta1.TransformerComponent: {
							URL: nil,
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				UrlScheme:               "http",
				DisableIstioVirtualHost: false,
			},
			matcher: gomega.Equal(""),
		},
		"transformer without default suffix": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
					Transformer: &v1beta1.TransformerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status:  duckv1.Status{},
					Address: nil,
					URL:     nil,
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: (*apis.URL)(predictorUrl),
						},
						v1beta1.TransformerComponent: {
							URL: (*apis.URL)(transformerUrl),
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				UrlScheme:               "http",
				DisableIstioVirtualHost: false,
			},
			matcher: gomega.Equal("http://my-model.example.com"),
		},
		"predictor without default suffix": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status:  duckv1.Status{},
					Address: nil,
					URL:     nil,
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: (*apis.URL)(predictorUrl),
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				UrlScheme:               "http",
				DisableIstioVirtualHost: false,
			},
			matcher: gomega.Equal("http://my-model.example.com"),
		},
		"predictor with istio disabled": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status:  duckv1.Status{},
					Address: nil,
					URL:     nil,
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: (*apis.URL)(predictorUrl),
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				UrlScheme:               "http",
				DisableIstioVirtualHost: true,
			},
			matcher: gomega.Equal("http://my-model-predictor.example.com"),
		},
		"transformer with istio disabled": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
					Transformer: &v1beta1.TransformerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status:  duckv1.Status{},
					Address: nil,
					URL:     nil,
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: (*apis.URL)(predictorUrl),
						},
						v1beta1.TransformerComponent: {
							URL: (*apis.URL)(transformerUrl),
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				UrlScheme:               "http",
				DisableIstioVirtualHost: true,
			},
			matcher: gomega.Equal("http://my-model-transformer.example.com"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			url := getServiceUrl(tc.isvc, tc.ingressConfig)
			g.Expect(url).Should(tc.matcher)
		})
	}
}

func TestGetServiceUrlPathBased(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	serviceName := "my-model"
	namespace := "test"
	isvcAnnotations := map[string]string{"test": "test", "kubectl.kubernetes.io/last-applied-configuration": "test"}
	labels := map[string]string{"test": "test"}
	predictorUrl, _ := url.Parse("http://my-model-predictor-default.example.com")
	ingressConfig := &v1beta1.IngressConfig{
		UrlScheme:               "http",
		IngressDomain:           "my-domain.com",
		PathTemplate:            "/serving/{{ .Namespace }}/{{ .Name }}",
		DisableIstioVirtualHost: false,
	}

	cases := map[string]struct {
		isvc    *v1beta1.InferenceService
		matcher gomegaTypes.GomegaMatcher
	}{
		"component is empty": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
				},
			},
			matcher: gomega.Equal(""),
		},
		"predictor url is empty": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status:  duckv1.Status{},
					Address: nil,
					URL:     nil,
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {},
					},
					ModelStatus: v1beta1.ModelStatus{},
				},
			},
			matcher: gomega.Equal(""),
		},
		"predictor url is not empty": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status:  duckv1.Status{},
					Address: nil,
					URL:     nil,
					Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
						v1beta1.PredictorComponent: {
							URL: (*apis.URL)(predictorUrl),
						},
					},
				},
			},
			matcher: gomega.Equal("http://my-domain.com/serving/test/my-model"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			url := getServiceUrl(tc.isvc, ingressConfig)
			g.Expect(url).Should(tc.matcher)
		})
	}
}

func TestGetHostPrefix(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	serviceName := "my-model"
	namespace := "test"
	isvcAnnotations := map[string]string{"test": "test", "kubectl.kubernetes.io/last-applied-configuration": "test"}
	labels := map[string]string{"test": "test"}

	cases := map[string]struct {
		isvc               *v1beta1.InferenceService
		disableVirtualHost bool
		useDefault         bool
		matcher            gomegaTypes.GomegaMatcher
	}{
		"Disable virtual host is false": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
				},
			},
			disableVirtualHost: false,
			useDefault:         false,
			matcher:            gomega.Equal(serviceName),
		},
		"istio is disabled and useDefault is false": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
			},
			disableVirtualHost: true,
			useDefault:         false,
			matcher:            gomega.Equal(serviceName + "-predictor"),
		},
		"istio is disabled and useDefault is false with transformer": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
					Transformer: &v1beta1.TransformerSpec{
						PodSpec: v1beta1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "kserve-container",
									Image: "kserve/transformer:latest",
								},
							},
						},
					},
				},
			},
			disableVirtualHost: true,
			useDefault:         false,
			matcher:            gomega.Equal(serviceName + "-transformer"),
		},
		"istio is disabled and useDefault is true": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
			},
			disableVirtualHost: true,
			useDefault:         true,
			matcher:            gomega.Equal(serviceName + "-predictor-default"),
		},
		"istio is disabled and useDefault is true with transformer": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   namespace,
					Annotations: isvcAnnotations,
					Labels:      labels,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
					Transformer: &v1beta1.TransformerSpec{
						PodSpec: v1beta1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "kserve-container",
									Image: "kserve/transformer:latest",
								},
							},
						},
					},
				},
			},
			disableVirtualHost: true,
			useDefault:         true,
			matcher:            gomega.Equal(serviceName + "-transformer-default"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			host := getHostPrefix(tc.isvc, tc.disableVirtualHost, tc.useDefault)
			g.Expect(host).Should(tc.matcher)
		})
	}
}

func TestIngressReconciler_Reconcile(t *testing.T) {
	type fields struct {
		ingressConfig *v1beta1.IngressConfig
		isvcConfig    *v1beta1.InferenceServicesConfig
	}
	type args struct {
		isvc *v1beta1.InferenceService
	}
	tests := []struct {
		name           string
		fields         fields
		args           args
		setupClient    func(*v1beta1.InferenceService) client.Client
		setupClientset func() kubernetes.Interface
		wantErr        bool
		wantURLHost    string
		wantAddress    string
	}{
		{
			name: "Istio virtual host enabled, predictor ready, creates virtualservice",
			fields: fields{
				ingressConfig: &v1beta1.IngressConfig{
					DisableIstioVirtualHost:    false,
					KnativeLocalGatewayService: "knative-local-gateway",
					LocalGateway:               "knative-local-gateway",
					IngressGateway:             "istio-ingressgateway",
					IngressDomain:              "example.com",
					UrlScheme:                  "http",
				},
				isvcConfig: &v1beta1.InferenceServicesConfig{
					ServiceAnnotationDisallowedList: []string{},
					ServiceLabelDisallowedList:      []string{},
				},
			},
			args: args{
				isvc: &v1beta1.InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc",
						Namespace: "ns",
					},
					Spec: v1beta1.InferenceServiceSpec{
						Predictor: v1beta1.PredictorSpec{
							PodSpec: v1beta1.PodSpec{
								Containers: []corev1.Container{{Name: "predictor"}},
							},
						},
					},
					Status: v1beta1.InferenceServiceStatus{
						Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
							v1beta1.PredictorComponent: {
								URL: &apis.URL{Scheme: "http", Host: "svc-predictor.ns"},
							},
						},
						Status: duckv1.Status{
							Conditions: duckv1.Conditions{
								{
									Type:   v1beta1.PredictorReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			setupClient: func(isvc *v1beta1.InferenceService) client.Client {
				s := runtime.NewScheme()
				_ = v1beta1.AddToScheme(s)
				_ = istioclientv1beta1.AddToScheme(s)
				_ = corev1.AddToScheme(s)
				cl := fake.NewClientBuilder().WithScheme(s).WithObjects(isvc).Build()
				return cl
			},
			setupClientset: func() kubernetes.Interface {
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-domain",
						Namespace: "knative-serving",
					},
					Data: map[string]string{"example.com": ""},
				}
				fake := kubernetesfake.NewSimpleClientset(cm)
				return fake
			},
			wantErr:     false,
			wantURLHost: "svc.ns",
			wantAddress: "svc.ns.svc.cluster.local",
		},
		{
			name: "Predictor not ready, does not set URL",
			fields: fields{
				ingressConfig: &v1beta1.IngressConfig{
					DisableIstioVirtualHost:    false,
					KnativeLocalGatewayService: "knative-local-gateway",
					LocalGateway:               "knative-local-gateway",
					IngressGateway:             "istio-ingressgateway",
					IngressDomain:              "example.com",
					UrlScheme:                  "http",
				},
				isvcConfig: &v1beta1.InferenceServicesConfig{},
			},
			args: args{
				isvc: &v1beta1.InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc",
						Namespace: "ns",
					},
					Spec: v1beta1.InferenceServiceSpec{
						Predictor: v1beta1.PredictorSpec{},
					},
					Status: v1beta1.InferenceServiceStatus{
						Status: duckv1.Status{
							Conditions: duckv1.Conditions{
								{
									Type:   v1beta1.PredictorReady,
									Status: corev1.ConditionFalse,
								},
							},
						},
					},
				},
			},
			setupClient: func(isvc *v1beta1.InferenceService) client.Client {
				s := runtime.NewScheme()
				_ = v1beta1.AddToScheme(s)
				_ = corev1.AddToScheme(s)
				_ = duckv1.AddToScheme(s)
				cl := fake.NewClientBuilder().WithScheme(s).WithObjects(isvc).Build()
				return cl
			},
			setupClientset: func() kubernetes.Interface {
				return kubernetesfake.NewSimpleClientset()
			},
			wantErr:     false,
			wantURLHost: "",
			wantAddress: "",
		},
		{
			name: "No components in status, returns nil",
			fields: fields{
				ingressConfig: &v1beta1.IngressConfig{
					DisableIstioVirtualHost:    false,
					KnativeLocalGatewayService: "knative-local-gateway",
					LocalGateway:               "knative-local-gateway",
					IngressGateway:             "istio-ingressgateway",
					IngressDomain:              "example.com",
					UrlScheme:                  "http",
				},
				isvcConfig: &v1beta1.InferenceServicesConfig{},
			},
			args: args{
				isvc: &v1beta1.InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc",
						Namespace: "ns",
					},
					Spec: v1beta1.InferenceServiceSpec{
						Predictor: v1beta1.PredictorSpec{},
					},
					Status: v1beta1.InferenceServiceStatus{},
				},
			},
			setupClient: func(isvc *v1beta1.InferenceService) client.Client {
				s := runtime.NewScheme()
				_ = v1beta1.AddToScheme(s)
				_ = corev1.AddToScheme(s)
				_ = duckv1.AddToScheme(s)
				cl := fake.NewClientBuilder().WithScheme(s).WithObjects(isvc).Build()
				return cl
			},
			setupClientset: func() kubernetes.Interface {
				return kubernetesfake.NewSimpleClientset()
			},
			wantErr:     false,
			wantURLHost: "",
			wantAddress: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := tt.setupClient(tt.args.isvc)
			clientset := tt.setupClientset()
			s := runtime.NewScheme()
			_ = v1beta1.AddToScheme(s)
			_ = corev1.AddToScheme(s)
			_ = duckv1.AddToScheme(s)
			r := &IngressReconciler{
				client:        cl,
				clientset:     clientset,
				scheme:        s,
				ingressConfig: tt.fields.ingressConfig,
				isvcConfig:    tt.fields.isvcConfig,
			}
			err := r.Reconcile(context.TODO(), tt.args.isvc)
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantURLHost != "" {
				if tt.args.isvc.Status.URL == nil || tt.args.isvc.Status.URL.Host != tt.wantURLHost {
					t.Errorf("Expected URL host %v, got %v", tt.wantURLHost, tt.args.isvc.Status.URL)
				}
			} else if tt.args.isvc.Status.URL != nil {
				t.Errorf("Expected URL to be nil, got %v", tt.args.isvc.Status.URL)
			}
			if tt.wantAddress != "" {
				if tt.args.isvc.Status.Address == nil || tt.args.isvc.Status.Address.URL == nil ||
					tt.args.isvc.Status.Address.URL.Host != tt.wantAddress {
					t.Errorf("Expected Address host %v, got %v", tt.wantAddress, tt.args.isvc.Status.Address)
				}
			} else if tt.args.isvc.Status.Address != nil {
				t.Errorf("Expected Address to be nil, got %v", tt.args.isvc.Status.Address)
			}
		})
	}
}

func TestNewIngressReconciler(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Create fake controller-runtime client
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create fake kubernetes clientset
	clientset := kubernetesfake.NewSimpleClientset()

	// Create configs
	ingressConfig := &v1beta1.IngressConfig{
		IngressGateway:             "test-ingress-gateway",
		LocalGateway:               "test-local-gateway",
		KnativeLocalGatewayService: "test-knative-local-gateway-service",
		DisableIstioVirtualHost:    false,
		UrlScheme:                  "http",
		IngressDomain:              "example.com",
	}
	isvcConfig := &v1beta1.InferenceServicesConfig{}

	// Call constructor
	reconciler := NewIngressReconciler(client, clientset, scheme, ingressConfig, isvcConfig)

	// Assertions
	g.Expect(reconciler).NotTo(gomega.BeNil())
	g.Expect(reconciler.client).To(gomega.Equal(client))
	g.Expect(reconciler.clientset).To(gomega.Equal(clientset))
	g.Expect(reconciler.scheme).To(gomega.Equal(scheme))
	g.Expect(reconciler.ingressConfig).To(gomega.Equal(ingressConfig))
	g.Expect(reconciler.isvcConfig).To(gomega.Equal(isvcConfig))
}
