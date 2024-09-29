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
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	v1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

func TestCreateRawURL(t *testing.T) {
	g := NewGomegaWithT(t)

	testCases := map[string]struct {
		isvc            *v1beta1.InferenceService
		ingressConfig   *v1beta1.IngressConfig
		expectedURL     string
		isErrorExpected bool
	}{
		"basic case": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: "default",
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:  "example.com",
				UrlScheme:      "http",
				DomainTemplate: "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
			},
			isErrorExpected: false,
			expectedURL:     "http://test-isvc-default.example.com",
		},
		"basic case with empty domain template": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: "default",
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:  "example.com",
				UrlScheme:      "http",
				DomainTemplate: "",
			},
			expectedURL:     "",
			isErrorExpected: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			url, err := createRawURL(tc.isvc, tc.ingressConfig)
			if tc.isErrorExpected {
				g.Expect(err).ToNot(BeNil())
			} else {
				g.Expect(err).To(BeNil())
			}
			g.Expect(tc.expectedURL).To(BeComparableTo(url.String()))
		})
	}
}

func TestGetRawServiceHost(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := map[string]struct {
		isvc         *v1beta1.InferenceService
		expectedHost string
	}{
		"basic case": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
				},
			},
			expectedHost: "test-isvc-predictor.default.svc.cluster.local",
		},
		"basic case with transformer": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor:   v1beta1.PredictorSpec{},
					Transformer: &v1beta1.TransformerSpec{},
				},
			},
			expectedHost: "test-isvc-transformer.default.svc.cluster.local",
		},
		"predictor with default suffix": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc-pred-default",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
				},
			},
			expectedHost: "test-isvc-pred-default-predictor.default.svc.cluster.local",
		},
		"transformer with default suffix": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc-pred-default",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor:   v1beta1.PredictorSpec{},
					Transformer: &v1beta1.TransformerSpec{},
				},
			},
			expectedHost: "test-isvc-pred-default-transformer.default.svc.cluster.local",
		},
	}

	s := scheme.Scheme
	s.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.InferenceService{})
	client := fake.NewClientBuilder().WithScheme(s).Build()
	// Create a dummy service to test default suffix cases
	client.Create(context.Background(), &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-isvc-pred-default",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{},
	})
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			host := getRawServiceHost(tc.isvc, client)
			g.Expect(tc.expectedHost).To(BeComparableTo(host))
		})
	}
}

func TestCreateHTTPRouteMatch(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := map[string]struct {
		prefix             string
		targetHosts        []string
		internalHosts      []string
		additionalHosts    *[]string
		expectedHTTPRoutes []gatewayapiv1.HTTPRouteMatch
	}{
		"basic case": {
			prefix:          "^.*$",
			targetHosts:     []string{"example.com"},
			internalHosts:   []string{"internal.example.com"},
			additionalHosts: &[]string{"additional.example.com"},
			expectedHTTPRoutes: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: &gatewayapiv1.HTTPPathMatch{
						Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
						Value: utils.ToPointer("^.*$"),
					},
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						{
							Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
							Name:  gatewayapiv1.HTTPHeaderName("Host"),
							Value: constants.HostRegExp("internal.example.com"),
						},
					},
				},
				{
					Path: &gatewayapiv1.HTTPPathMatch{
						Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
						Value: utils.ToPointer("^.*$"),
					},
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						{
							Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
							Name:  gatewayapiv1.HTTPHeaderName("Host"),
							Value: constants.HostRegExp("example.com"),
						},
					},
				},
				{
					Path: &gatewayapiv1.HTTPPathMatch{
						Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
						Value: utils.ToPointer("^.*$"),
					},
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						{
							Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
							Name:  gatewayapiv1.HTTPHeaderName("Host"),
							Value: constants.HostRegExp("additional.example.com"),
						},
					},
				},
			},
		},
		"no additional hosts": {
			prefix:          "^.*$",
			targetHosts:     []string{"example.com"},
			internalHosts:   []string{"internal.example.com"},
			additionalHosts: nil,
			expectedHTTPRoutes: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: &gatewayapiv1.HTTPPathMatch{
						Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
						Value: utils.ToPointer("^.*$"),
					},
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						{
							Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
							Name:  gatewayapiv1.HTTPHeaderName("Host"),
							Value: constants.HostRegExp("internal.example.com"),
						},
					},
				},
				{
					Path: &gatewayapiv1.HTTPPathMatch{
						Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
						Value: utils.ToPointer("^.*$"),
					},
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						{
							Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
							Name:  gatewayapiv1.HTTPHeaderName("Host"),
							Value: constants.HostRegExp("example.com"),
						},
					},
				},
			},
		},
		"no internal hosts": {
			prefix:          "^.*$",
			targetHosts:     []string{"example.com"},
			internalHosts:   nil,
			additionalHosts: nil,
			expectedHTTPRoutes: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: &gatewayapiv1.HTTPPathMatch{
						Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
						Value: utils.ToPointer("^.*$"),
					},
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						{
							Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
							Name:  gatewayapiv1.HTTPHeaderName("Host"),
							Value: constants.HostRegExp("example.com"),
						},
					},
				},
			},
		},
		"empty prefix": {
			prefix:          "",
			targetHosts:     []string{"example.com"},
			internalHosts:   []string{"internal.example.com"},
			additionalHosts: &[]string{"additional.example.com"},
			expectedHTTPRoutes: []gatewayapiv1.HTTPRouteMatch{
				{
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						{
							Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
							Name:  gatewayapiv1.HTTPHeaderName("Host"),
							Value: constants.HostRegExp("internal.example.com"),
						},
					},
				},
				{
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						{
							Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
							Name:  gatewayapiv1.HTTPHeaderName("Host"),
							Value: constants.HostRegExp("example.com"),
						},
					},
				},
				{
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						{
							Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
							Name:  gatewayapiv1.HTTPHeaderName("Host"),
							Value: constants.HostRegExp("additional.example.com"),
						},
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			matches := createHTTPRouteMatch(tc.prefix, tc.targetHosts, tc.internalHosts, tc.additionalHosts, false)
			g.Expect(matches).To(BeComparableTo(tc.expectedHTTPRoutes))
		})
	}
}

func TestAddIsvcHeaders(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := map[string]struct {
		isvcName      string
		isvcNamespace string
	}{
		"basic case": {
			isvcName:      "test-isvc",
			isvcNamespace: "default",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			headers := addIsvcHeaders(tc.isvcName, tc.isvcNamespace)
			g.Expect(headers.Type).To(BeComparableTo(gatewayapiv1.HTTPRouteFilterRequestHeaderModifier))
			g.Expect(headers.RequestHeaderModifier.Set).To(HaveLen(2))
			g.Expect(string(headers.RequestHeaderModifier.Set[0].Name)).To(BeComparableTo(constants.IsvcNameHeader))
			g.Expect(headers.RequestHeaderModifier.Set[0].Value).To(BeComparableTo(tc.isvcName))
			g.Expect(string(headers.RequestHeaderModifier.Set[1].Name)).To(BeComparableTo(constants.IsvcNamespaceHeader))
			g.Expect(headers.RequestHeaderModifier.Set[1].Value).To(BeComparableTo(tc.isvcNamespace))
		})
	}
}

func TestCreateHTTPRouteRule(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := map[string]struct {
		matches       []gatewayapiv1.HTTPRouteMatch
		filters       []gatewayapiv1.HTTPRouteFilter
		serviceName   string
		servicePort   int32
		expectedRules int
	}{
		"basic case": {
			matches: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: &gatewayapiv1.HTTPPathMatch{
						Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
						Value: utils.ToPointer("/predict"),
					},
				},
			},
			filters: []gatewayapiv1.HTTPRouteFilter{
				addIsvcHeaders("test-isvc", "default"),
			},
			serviceName:   "test-service",
			servicePort:   80,
			expectedRules: 1,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rule := createHTTPRouteRule(tc.matches, tc.filters, tc.serviceName, "default", tc.servicePort)
			g.Expect(rule.Matches).To(HaveLen(tc.expectedRules))
			g.Expect(rule.Filters).To(HaveLen(tc.expectedRules))
			g.Expect(rule.BackendRefs).To(HaveLen(tc.expectedRules))
			g.Expect(string(rule.BackendRefs[0].Name)).To(BeComparableTo(tc.serviceName))
		})
	}
}

func TestSemanticHttpRouteEquals(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := map[string]struct {
		desired       *gatewayapiv1.HTTPRoute
		existing      *gatewayapiv1.HTTPRoute
		expectedEqual bool
	}{
		"equal routes": {
			desired: &gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"example.com"},
				},
			},
			existing: &gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"example.com"},
				},
			},
			expectedEqual: true,
		},
		"different routes": {
			desired: &gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"example.com"},
				},
			},
			existing: &gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"different.com"},
				},
			},
			expectedEqual: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			g.Expect(semanticHttpRouteEquals(tc.desired, tc.existing)).To(BeComparableTo(tc.expectedEqual))
		})
	}
}

func TestIsHTTPRouteReady(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := map[string]struct {
		httpRouteStatus gatewayapiv1.HTTPRouteStatus
		expectedReady   bool
		expectedReason  *string
		expectedMessage *string
	}{
		"route accepted": {
			httpRouteStatus: gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:   string(gatewayapiv1.RouteConditionAccepted),
									Status: metav1.ConditionTrue,
								},
								{
									Type:   string(gatewayapiv1.RouteConditionResolvedRefs),
									Status: metav1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			expectedReady:   true,
			expectedReason:  nil,
			expectedMessage: nil,
		},
		"route not accepted": {
			httpRouteStatus: gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:    string(gatewayapiv1.RouteConditionAccepted),
									Status:  metav1.ConditionFalse,
									Reason:  "Route not accepted",
									Message: "Route not accepted",
								},
								{
									Type:   string(gatewayapiv1.RouteConditionResolvedRefs),
									Status: metav1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			expectedReady:   false,
			expectedReason:  utils.ToPointer("Route not accepted"),
			expectedMessage: utils.ToPointer("Route not accepted"),
		},
		"no parent status": {
			httpRouteStatus: gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{},
				},
			},
			expectedReady:   false,
			expectedReason:  utils.ToPointer(HTTPRouteParentStatusNotAvailable),
			expectedMessage: utils.ToPointer(HTTPRouteNotReady),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ready, reason, message := isHTTPRouteReady(tc.httpRouteStatus)
			g.Expect(ready).To(BeComparableTo(tc.expectedReady))
			g.Expect(reason).To(BeComparableTo(tc.expectedReason))
			g.Expect(message).To(BeComparableTo(tc.expectedMessage))
		})
	}
}

func TestCreateRawHTTPRoute(t *testing.T) {
	format.MaxLength = 0
	g := NewGomegaWithT(t)
	testCases := map[string]struct {
		isvc          *v1beta1.InferenceService
		ingressConfig *v1beta1.IngressConfig
		expected      *gatewayapiv1.HTTPRoute
	}{
		"Predictor ready": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:            "example.com",
				UrlScheme:                "http",
				DomainTemplate:           "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway:     "kserve/kserve-gateway",
				AdditionalIngressDomains: &[]string{"additional.example.com"},
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default.example.com", "additional.example.com", "test-isvc-predictor-default.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-default.example.com"),
										},
									},
								},
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("additional.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-predictor-default.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace("kserve")),
								Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
							},
						},
					},
				},
			},
		},
		"When predictor not ready, httproute should not be created": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:            "example.com",
				UrlScheme:                "http",
				DomainTemplate:           "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway:     "kserve/kserve-gateway",
				AdditionalIngressDomains: &[]string{"additional.example.com"},
			},
			expected: nil,
		},
		"With transformer ready": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor:   v1beta1.PredictorSpec{},
					Transformer: &v1beta1.TransformerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.TransformerReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:            "example.com",
				UrlScheme:                "http",
				DomainTemplate:           "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway:     "kserve/kserve-gateway",
				AdditionalIngressDomains: &[]string{"additional.example.com"},
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default.example.com", "additional.example.com", "test-isvc-transformer-default.example.com", "test-isvc-predictor-default.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-default.example.com"),
										},
									},
								},
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-transformer-default.example.com"),
										},
									},
								},
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("additional.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-transformer",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-predictor-default.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace("kserve")),
								Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
							},
						},
					},
				},
			},
		},
		"When transformer not ready, httproute should not be created": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor:   v1beta1.PredictorSpec{},
					Transformer: &v1beta1.TransformerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.TransformerReady,
								Status: corev1.ConditionFalse,
							},
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:            "example.com",
				UrlScheme:                "http",
				DomainTemplate:           "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway:     "kserve/kserve-gateway",
				AdditionalIngressDomains: &[]string{"additional.example.com"},
			},
			expected: nil,
		},
		"With explainer ready": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
					Explainer: &v1beta1.ExplainerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.ExplainerReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:            "example.com",
				UrlScheme:                "http",
				DomainTemplate:           "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway:     "kserve/kserve-gateway",
				AdditionalIngressDomains: &[]string{"additional.example.com"},
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default.example.com", "additional.example.com", "test-isvc-explainer-default.example.com", "test-isvc-predictor-default.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.ExplainPrefix()),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-default.example.com"),
										},
									},
								},
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.ExplainPrefix()),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("additional.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-explainer-default.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-default.example.com"),
										},
									},
								},
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("additional.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-predictor-default.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace("kserve")),
								Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
							},
						},
					},
				},
			},
		},
		"When explainer not ready, httproute should not be created": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
					Explainer: &v1beta1.ExplainerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.ExplainerReady,
								Status: corev1.ConditionFalse,
							},
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:            "example.com",
				UrlScheme:                "http",
				DomainTemplate:           "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway:     "kserve/kserve-gateway",
				AdditionalIngressDomains: &[]string{"additional.example.com"},
			},
			expected: nil,
		},
		"Path based routing with explainer": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
					Explainer: &v1beta1.ExplainerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.ExplainerReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:            "example.com",
				UrlScheme:                "http",
				DomainTemplate:           "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway:     "kserve/kserve-gateway",
				AdditionalIngressDomains: &[]string{"additional.example.com"},
				PathTemplate:             "/serving/{{ .Namespace }}/{{ .Name }}",
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default.example.com", "additional.example.com", "test-isvc-explainer-default.example.com", "test-isvc-predictor-default.example.com", "example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.ExplainPrefix()),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-default.example.com"),
										},
									},
								},
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.ExplainPrefix()),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("additional.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-explainer-default.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-default.example.com"),
										},
									},
								},
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("additional.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-predictor-default.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("/serving/default/test-isvc" + constants.PathBasedExplainPrefix()),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("/serving/default/test-isvc/"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace("kserve")),
								Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
							},
						},
					},
				},
			},
		},
		"Path based routing with transformer": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor:   v1beta1.PredictorSpec{},
					Transformer: &v1beta1.TransformerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.TransformerReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:            "example.com",
				UrlScheme:                "http",
				DomainTemplate:           "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway:     "kserve/kserve-gateway",
				AdditionalIngressDomains: &[]string{"additional.example.com"},
				PathTemplate:             "/serving/{{ .Namespace }}/{{ .Name }}",
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default.example.com", "additional.example.com", "test-isvc-transformer-default.example.com", "test-isvc-predictor-default.example.com", "example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-default.example.com"),
										},
									},
								},
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-transformer-default.example.com"),
										},
									},
								},
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("additional.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-transformer",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-predictor-default.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("/serving/default/test-isvc/"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-transformer",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace("kserve")),
								Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
							},
						},
					},
				},
			},
		},
		"Predictor with default suffix": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc-default",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:            "example.com",
				UrlScheme:                "http",
				DomainTemplate:           "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway:     "kserve/kserve-gateway",
				AdditionalIngressDomains: &[]string{"additional.example.com"},
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc-default",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default-default.example.com", "additional.example.com", "test-isvc-default-predictor-default-default.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-default-default.example.com"),
										},
									},
								},
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("additional.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc-default",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-default-predictor-default",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer("^/.*$"),
									},
									Headers: []gatewayapiv1.HTTPHeaderMatch{
										{
											Type:  utils.ToPointer(gatewayapiv1.HeaderMatchRegularExpression),
											Name:  gatewayapiv1.HTTPHeaderName("Host"),
											Value: constants.HostRegExp("test-isvc-default-predictor-default-default.example.com"),
										},
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: "test-isvc-default",
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: "default",
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-default-predictor-default",
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer("default")),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace("kserve")),
								Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			s := scheme.Scheme
			s.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.InferenceService{})
			s.AddKnownTypes(schema.GroupVersion{Group: gatewayapiv1.GroupVersion.Group, Version: gatewayapiv1.GroupVersion.Version},
				&gatewayapiv1.HTTPRoute{})
			client := fake.NewClientBuilder().WithScheme(s).Build()
			// Create a dummy service to test default suffix case
			client.Create(context.Background(), &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc-default-predictor-default",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{}})

			httpRoute, err := createRawHTTPRoute(tc.isvc, tc.ingressConfig, client)

			g.Expect(err).To(BeNil())
			if tc.expected != nil {
				g.Expect(httpRoute.Spec).To(BeComparableTo(tc.expected.Spec))
				g.Expect(httpRoute.ObjectMeta).To(BeComparableTo(tc.expected.ObjectMeta, cmpopts.IgnoreFields(httpRoute.ObjectMeta, "CreationTimestamp")))
			} else {
				g.Expect(httpRoute).To(BeNil())
			}
		})
	}
}
