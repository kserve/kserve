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
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
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
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
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
			host := getRawServiceHost(context.Background(), tc.isvc, client)
			g.Expect(tc.expectedHost).To(BeComparableTo(host))
		})
	}
}

func TestCreateHTTPRouteMatch(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := map[string]struct {
		prefix             string
		expectedHTTPRoutes gatewayapiv1.HTTPRouteMatch
	}{
		"basic case": {
			prefix: "^.*$",
			expectedHTTPRoutes: gatewayapiv1.HTTPRouteMatch{
				Path: &gatewayapiv1.HTTPPathMatch{
					Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
					Value: ptr.To("^.*$"),
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			matches := createHTTPRouteMatch(tc.prefix)
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
						Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
						Value: ptr.To("/predict"),
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
			rule := createHTTPRouteRule(tc.matches, tc.filters, tc.serviceName, "default", tc.servicePort, DefaultTimeout)
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
			expectedReason:  ptr.To("Route not accepted"),
			expectedMessage: ptr.To("Route not accepted"),
		},
		"no parent status": {
			httpRouteStatus: gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{},
				},
			},
			expectedReady:   false,
			expectedReason:  ptr.To(HTTPRouteParentStatusNotAvailable),
			expectedMessage: ptr.To(HTTPRouteNotReady),
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

func TestCreateRawTopLevelHTTPRoute(t *testing.T) {
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
				EnableGatewayAPI:         true,
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default.example.com", "test-isvc-default.additional.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      ptr.To(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: ptr.To(gatewayapiv1.Namespace("kserve")),
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
				EnableGatewayAPI:         true,
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
				EnableGatewayAPI:         true,
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default.example.com", "test-isvc-default.additional.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-transformer",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      ptr.To(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: ptr.To(gatewayapiv1.Namespace("kserve")),
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
				EnableGatewayAPI:         true,
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
				EnableGatewayAPI:         true,
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default.example.com", "test-isvc-default.additional.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To(constants.ExplainPrefix()),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      ptr.To(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: ptr.To(gatewayapiv1.Namespace("kserve")),
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
				EnableGatewayAPI:         true,
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
				EnableGatewayAPI:         true,
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default.example.com", "test-isvc-default.additional.example.com", "example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To(constants.ExplainPrefix()),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("/serving/default/test-isvc" + constants.PathBasedExplainPrefix()),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("/serving/default/test-isvc/"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      ptr.To(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: ptr.To(gatewayapiv1.Namespace("kserve")),
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
				EnableGatewayAPI:         true,
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default.example.com", "test-isvc-default.additional.example.com", "example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-transformer",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("/serving/default/test-isvc/"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-transformer",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      ptr.To(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: ptr.To(gatewayapiv1.Namespace("kserve")),
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
				EnableGatewayAPI:         true,
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc-default",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default-default.example.com", "test-isvc-default-default.additional.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-default-predictor-default",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      ptr.To(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: ptr.To(gatewayapiv1.Namespace("kserve")),
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
				Spec: corev1.ServiceSpec{},
			})
			isvcConfig := &v1beta1.InferenceServicesConfig{
				ServiceAnnotationDisallowedList: []string{},
				ServiceLabelDisallowedList:      []string{},
			}
			httpRoute, err := createRawTopLevelHTTPRoute(context.Background(), tc.isvc, tc.ingressConfig, isvcConfig, client)

			g.Expect(err).ToNot(HaveOccurred())
			if tc.expected != nil {
				g.Expect(httpRoute.Spec).To(BeComparableTo(tc.expected.Spec))
				g.Expect(httpRoute.ObjectMeta).To(BeComparableTo(tc.expected.ObjectMeta, cmpopts.IgnoreFields(httpRoute.ObjectMeta, "CreationTimestamp")))
			} else {
				g.Expect(httpRoute).To(BeNil())
			}
		})
	}
}

func TestCreateRawPredictorHTTPRoute(t *testing.T) {
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
				IngressDomain:        "example.com",
				UrlScheme:            "http",
				DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway: "kserve/kserve-gateway",
				EnableGatewayAPI:     true,
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc-predictor",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-predictor-default.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Kind:      (*gatewayapiv1.Kind)(ptr.To(constants.GatewayKind)),
								Namespace: (*gatewayapiv1.Namespace)(ptr.To("kserve")),
								Name:      gatewayapiv1.ObjectName("kserve-gateway"),
							},
						},
					},
				},
			},
		},
		"Predictor not ready": {
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
				IngressDomain:        "example.com",
				UrlScheme:            "http",
				DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway: "kserve/kserve-gateway",
				EnableGatewayAPI:     true,
			},
			expected: nil,
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
				IngressDomain:        "example.com",
				UrlScheme:            "http",
				DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway: "kserve/kserve-gateway",
				EnableGatewayAPI:     true,
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc-default-predictor",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default-predictor-default-default.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-default-predictor-default",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Kind:      (*gatewayapiv1.Kind)(ptr.To(constants.GatewayKind)),
								Namespace: (*gatewayapiv1.Namespace)(ptr.To("kserve")),
								Name:      gatewayapiv1.ObjectName("kserve-gateway"),
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
				Spec: corev1.ServiceSpec{},
			})
			isvcConfig := &v1beta1.InferenceServicesConfig{
				ServiceAnnotationDisallowedList: []string{},
				ServiceLabelDisallowedList:      []string{},
			}
			httpRoute, err := createRawPredictorHTTPRoute(context.Background(), tc.isvc, tc.ingressConfig, isvcConfig, client)

			g.Expect(err).ToNot(HaveOccurred())
			if tc.expected != nil {
				g.Expect(httpRoute.Spec).To(BeComparableTo(tc.expected.Spec))
				g.Expect(httpRoute.ObjectMeta).To(BeComparableTo(tc.expected.ObjectMeta, cmpopts.IgnoreFields(httpRoute.ObjectMeta, "CreationTimestamp")))
			} else {
				g.Expect(httpRoute).To(BeNil())
			}
		})
	}
}

func TestCreateRawTransformerHTTPRoute(t *testing.T) {
	format.MaxLength = 0
	g := NewGomegaWithT(t)
	testCases := map[string]struct {
		isvc          *v1beta1.InferenceService
		ingressConfig *v1beta1.IngressConfig
		expected      *gatewayapiv1.HTTPRoute
	}{
		"Transformer ready": {
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
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:        "example.com",
				UrlScheme:            "http",
				DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway: "kserve/kserve-gateway",
				EnableGatewayAPI:     true,
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc-transformer",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-transformer-default.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-transformer",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Kind:      (*gatewayapiv1.Kind)(ptr.To(constants.GatewayKind)),
								Namespace: (*gatewayapiv1.Namespace)(ptr.To("kserve")),
								Name:      gatewayapiv1.ObjectName("kserve-gateway"),
							},
						},
					},
				},
			},
		},
		"Transformer not ready": {
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
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:        "example.com",
				UrlScheme:            "http",
				DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway: "kserve/kserve-gateway",
				EnableGatewayAPI:     true,
			},
			expected: nil,
		},
		"Transformer with default suffix": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc-default",
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
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:        "example.com",
				UrlScheme:            "http",
				DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway: "kserve/kserve-gateway",
				EnableGatewayAPI:     true,
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc-default-transformer",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default-transformer-default-default.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-default-transformer-default",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Kind:      (*gatewayapiv1.Kind)(ptr.To(constants.GatewayKind)),
								Namespace: (*gatewayapiv1.Namespace)(ptr.To("kserve")),
								Name:      gatewayapiv1.ObjectName("kserve-gateway"),
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
					Name:      "test-isvc-default-transformer-default",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{},
			})
			isvcConfig := &v1beta1.InferenceServicesConfig{
				ServiceAnnotationDisallowedList: []string{},
				ServiceLabelDisallowedList:      []string{},
			}
			httpRoute, err := createRawTransformerHTTPRoute(context.Background(), tc.isvc, tc.ingressConfig, isvcConfig, client)

			g.Expect(err).ToNot(HaveOccurred())
			if tc.expected != nil {
				g.Expect(httpRoute.Spec).To(BeComparableTo(tc.expected.Spec))
				g.Expect(httpRoute.ObjectMeta).To(BeComparableTo(tc.expected.ObjectMeta, cmpopts.IgnoreFields(httpRoute.ObjectMeta, "CreationTimestamp")))
			} else {
				g.Expect(httpRoute).To(BeNil())
			}
		})
	}
}

func TestCreateRawExplainerHTTPRoute(t *testing.T) {
	format.MaxLength = 0
	g := NewGomegaWithT(t)
	testCases := map[string]struct {
		isvc          *v1beta1.InferenceService
		ingressConfig *v1beta1.IngressConfig
		expected      *gatewayapiv1.HTTPRoute
	}{
		"Explainer ready": {
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
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:        "example.com",
				UrlScheme:            "http",
				DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway: "kserve/kserve-gateway",
				EnableGatewayAPI:     true,
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc-explainer",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-explainer-default.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Kind:      (*gatewayapiv1.Kind)(ptr.To(constants.GatewayKind)),
								Namespace: (*gatewayapiv1.Namespace)(ptr.To("kserve")),
								Name:      gatewayapiv1.ObjectName("kserve-gateway"),
							},
						},
					},
				},
			},
		},
		"Explainer not ready": {
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
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:        "example.com",
				UrlScheme:            "http",
				DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway: "kserve/kserve-gateway",
				EnableGatewayAPI:     true,
			},
			expected: nil,
		},
		"Explainer with default suffix": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc-default",
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
						},
					},
				},
			},
			ingressConfig: &v1beta1.IngressConfig{
				IngressDomain:        "example.com",
				UrlScheme:            "http",
				DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
				KserveIngressGateway: "kserve/kserve-gateway",
				EnableGatewayAPI:     true,
			},
			expected: &gatewayapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc-default-explainer",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{"test-isvc-default-explainer-default-default.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
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
											Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-default-explainer-default",
											Namespace: (*gatewayapiv1.Namespace)(ptr.To("default")),
											Port:      (*gatewayapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gatewayapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Kind:      (*gatewayapiv1.Kind)(ptr.To(constants.GatewayKind)),
								Namespace: (*gatewayapiv1.Namespace)(ptr.To("kserve")),
								Name:      gatewayapiv1.ObjectName("kserve-gateway"),
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
					Name:      "test-isvc-default-explainer-default",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{},
			})
			isvcConfig := &v1beta1.InferenceServicesConfig{
				ServiceAnnotationDisallowedList: []string{},
				ServiceLabelDisallowedList:      []string{},
			}
			httpRoute, err := createRawExplainerHTTPRoute(context.Background(), tc.isvc, tc.ingressConfig, isvcConfig, client)

			g.Expect(err).ToNot(HaveOccurred())
			if tc.expected != nil {
				g.Expect(httpRoute.Spec).To(BeComparableTo(tc.expected.Spec))
				g.Expect(httpRoute.ObjectMeta).To(BeComparableTo(tc.expected.ObjectMeta, cmpopts.IgnoreFields(httpRoute.ObjectMeta, "CreationTimestamp")))
			} else {
				g.Expect(httpRoute).To(BeNil())
			}
		})
	}
}

func TestRawHTTPRouteReconciler_reconcilePredictorHTTPRoute(t *testing.T) {
	g := NewGomegaWithT(t)
	s := scheme.Scheme
	_ = v1beta1.AddToScheme(s)
	_ = gatewayapiv1.Install(s)
	_ = corev1.AddToScheme(s)

	ingressConfig := &v1beta1.IngressConfig{
		IngressDomain:        "example.com",
		UrlScheme:            "http",
		DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
		KserveIngressGateway: "kserve/knative-gateway",
	}
	isvcConfig := &v1beta1.InferenceServicesConfig{}

	t.Run("creates predictor HTTPRoute if not exists", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(s).Build()
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		isvc.Status.SetCondition(v1beta1.PredictorReady, &apis.Condition{
			Type:   v1beta1.PredictorReady,
			Status: corev1.ConditionTrue,
		})

		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}

		err := reconciler.reconcilePredictorHTTPRoute(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gatewayapiv1.HTTPRoute{}
		err = client.Get(context.TODO(), types.NamespacedName{
			Name:      "foo-predictor",
			Namespace: "default",
		}, route)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(route.Spec.Hostnames).To(ContainElement(gatewayapiv1.Hostname("foo-predictor-default.example.com")))
	})

	t.Run("does nothing if desired is nil (PredictorReady false)", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(s).Build()
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		// PredictorReady is not set or false

		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}

		err := reconciler.reconcilePredictorHTTPRoute(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gatewayapiv1.HTTPRoute{}
		err = client.Get(context.TODO(), types.NamespacedName{
			Name:      "foo-predictor",
			Namespace: "default",
		}, route)
		g.Expect(apierr.IsNotFound(err)).To(BeTrue())
	})

	t.Run("updates HTTPRoute if spec changes", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(s).Build()
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		isvc.Status.SetCondition(v1beta1.PredictorReady, &apis.Condition{
			Type:   v1beta1.PredictorReady,
			Status: corev1.ConditionTrue,
		})

		// Create an existing HTTPRoute with a different hostname
		existing := &gatewayapiv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo-predictor",
				Namespace: "default",
			},
			Spec: gatewayapiv1.HTTPRouteSpec{
				Hostnames: []gatewayapiv1.Hostname{"old-host.example.com"},
			},
		}
		_ = client.Create(context.TODO(), existing)

		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}

		err := reconciler.reconcilePredictorHTTPRoute(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gatewayapiv1.HTTPRoute{}
		err = client.Get(context.TODO(), types.NamespacedName{
			Name:      "foo-predictor",
			Namespace: "default",
		}, route)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(route.Spec.Hostnames).To(ContainElement(gatewayapiv1.Hostname("foo-predictor-default.example.com")))
	})
}

func TestRawHTTPRouteReconciler_reconcileTransformerHTTPRoute(t *testing.T) {
	g := NewGomegaWithT(t)
	s := scheme.Scheme
	s.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.InferenceService{})
	s.AddKnownTypes(schema.GroupVersion{Group: gatewayapiv1.GroupVersion.Group, Version: gatewayapiv1.GroupVersion.Version}, &gatewayapiv1.HTTPRoute{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Service{})

	ingressConfig := &v1beta1.IngressConfig{
		IngressDomain:        "example.com",
		UrlScheme:            "http",
		DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
		KserveIngressGateway: "kserve/knative-gateway",
	}
	isvcConfig := &v1beta1.InferenceServicesConfig{}

	t.Run("creates transformer HTTPRoute if not exists", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
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
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.TransformerReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		}
		client := fake.NewClientBuilder().WithScheme(s).Build()
		// Create a dummy transformer service so the reconciler finds it
		client.Create(context.TODO(), &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc-transformer",
				Namespace: "default",
			},
		})
		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}
		err := reconciler.reconcileTransformerHTTPRoute(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gatewayapiv1.HTTPRoute{}
		err = client.Get(context.TODO(), types.NamespacedName{
			Name:      "test-isvc-transformer",
			Namespace: "default",
		}, route)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(route.Spec.Hostnames).ToNot(BeEmpty())
	})

	t.Run("updates transformer HTTPRoute if spec changed", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc2",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor:   v1beta1.PredictorSpec{},
				Transformer: &v1beta1.TransformerSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.TransformerReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		}
		client := fake.NewClientBuilder().WithScheme(s).Build()
		client.Create(context.TODO(), &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc2-transformer",
				Namespace: "default",
			},
		})
		// Create an existing HTTPRoute with different spec
		existingRoute := &gatewayapiv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc2-transformer",
				Namespace: "default",
			},
			Spec: gatewayapiv1.HTTPRouteSpec{
				Hostnames: []gatewayapiv1.Hostname{"oldhost.example.com"},
			},
		}
		client.Create(context.TODO(), existingRoute)

		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}
		err := reconciler.reconcileTransformerHTTPRoute(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gatewayapiv1.HTTPRoute{}
		err = client.Get(context.TODO(), types.NamespacedName{
			Name:      "test-isvc2-transformer",
			Namespace: "default",
		}, route)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(route.Spec.Hostnames).To(ContainElement(gatewayapiv1.Hostname("test-isvc2-transformer-default.example.com")))
	})

	t.Run("does nothing if desired is nil", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc3",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor:   v1beta1.PredictorSpec{},
				Transformer: &v1beta1.TransformerSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.TransformerReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
		}
		client := fake.NewClientBuilder().WithScheme(s).Build()
		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}
		err := reconciler.reconcileTransformerHTTPRoute(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
	})
}

func TestRawHTTPRouteReconciler_reconcileExplainerHTTPRoute(t *testing.T) {
	g := NewGomegaWithT(t)
	s := scheme.Scheme
	s.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.InferenceService{})
	s.AddKnownTypes(schema.GroupVersion{Group: gatewayapiv1.GroupVersion.Group, Version: gatewayapiv1.GroupVersion.Version}, &gatewayapiv1.HTTPRoute{})
	ctx := context.TODO()

	ingressConfig := &v1beta1.IngressConfig{
		IngressDomain:        "example.com",
		UrlScheme:            "http",
		DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
		KserveIngressGateway: "kserve-system/kserve-gateway",
	}
	isvcConfig := &v1beta1.InferenceServicesConfig{}

	t.Run("creates explainer HTTPRoute when ready", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
				Explainer: &v1beta1.ExplainerSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		isvc.Status.SetCondition(v1beta1.ExplainerReady, &apis.Condition{
			Type:   v1beta1.ExplainerReady,
			Status: corev1.ConditionTrue,
		})

		client := fake.NewClientBuilder().WithScheme(s).Build()
		// Create explainer service so default suffix is not used
		client.Create(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo-explainer",
				Namespace: "bar",
			},
		})

		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}

		err := reconciler.reconcileExplainerHTTPRoute(ctx, isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gatewayapiv1.HTTPRoute{}
		err = client.Get(ctx, types.NamespacedName{Name: "foo-explainer", Namespace: "bar"}, route)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(route.Spec.Hostnames).ToNot(BeEmpty())
	})

	t.Run("does not create HTTPRoute if explainer not ready", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
				Explainer: &v1beta1.ExplainerSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		// ExplainerReady not set or false

		client := fake.NewClientBuilder().WithScheme(s).Build()
		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}

		err := reconciler.reconcileExplainerHTTPRoute(ctx, isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gatewayapiv1.HTTPRoute{}
		err = client.Get(ctx, types.NamespacedName{Name: "foo-explainer", Namespace: "bar"}, route)
		g.Expect(apierr.IsNotFound(err)).To(BeTrue())
	})

	t.Run("uses default suffix if explainer service with default suffix exists", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
				Explainer: &v1beta1.ExplainerSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		isvc.Status.SetCondition(v1beta1.ExplainerReady, &apis.Condition{
			Type:   v1beta1.ExplainerReady,
			Status: corev1.ConditionTrue,
		})

		// Register HTTPRoute and corev1.Service types in the scheme for this test
		s.AddKnownTypes(schema.GroupVersion{Group: gatewayapiv1.GroupVersion.Group, Version: gatewayapiv1.GroupVersion.Version}, &gatewayapiv1.HTTPRoute{})
		s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Service{})

		client := fake.NewClientBuilder().WithScheme(s).
			WithObjects(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-explainer-default",
					Namespace: "bar",
				},
			}).Build()

		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}

		err := reconciler.reconcileExplainerHTTPRoute(ctx, isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gatewayapiv1.HTTPRoute{}
		_ = client.Get(ctx, types.NamespacedName{Name: "foo-explainer-default", Namespace: "bar"}, route)
	})
}

func TestRawHTTPRouteReconciler_reconcileTopLevelHTTPRoute(t *testing.T) {
	g := NewGomegaWithT(t)
	s := scheme.Scheme
	_ = gatewayapiv1.Install(s)
	_ = v1beta1.AddToScheme(s)

	ingressConfig := &v1beta1.IngressConfig{
		IngressDomain:        "example.com",
		UrlScheme:            "http",
		DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
		KserveIngressGateway: "kserve/kserve-gateway",
	}
	isvcConfig := &v1beta1.InferenceServicesConfig{}

	t.Run("creates top-level HTTPRoute for predictor only", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		isvc.Status.SetCondition(v1beta1.PredictorReady, &apis.Condition{
			Type:   v1beta1.PredictorReady,
			Status: corev1.ConditionTrue,
		})

		client := fake.NewClientBuilder().WithScheme(s).WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc-predictor",
					Namespace: "default",
				},
			},
		).Build()

		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}

		err := reconciler.reconcileTopLevelHTTPRoute(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gatewayapiv1.HTTPRoute{}
		err = client.Get(context.TODO(), types.NamespacedName{
			Name:      "test-isvc",
			Namespace: "default",
		}, route)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(route.Spec.Hostnames).To(ContainElement(gatewayapiv1.Hostname("test-isvc-default.example.com")))
	})

	t.Run("creates top-level HTTPRoute for predictor+transformer+explainer", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc2",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor:   v1beta1.PredictorSpec{},
				Transformer: &v1beta1.TransformerSpec{},
				Explainer:   &v1beta1.ExplainerSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		isvc.Status.SetCondition(v1beta1.PredictorReady, &apis.Condition{
			Type:   v1beta1.PredictorReady,
			Status: corev1.ConditionTrue,
		})
		isvc.Status.SetCondition(v1beta1.TransformerReady, &apis.Condition{
			Type:   v1beta1.TransformerReady,
			Status: corev1.ConditionTrue,
		})
		isvc.Status.SetCondition(v1beta1.ExplainerReady, &apis.Condition{
			Type:   v1beta1.ExplainerReady,
			Status: corev1.ConditionTrue,
		})

		client := fake.NewClientBuilder().WithScheme(s).WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc2-predictor",
					Namespace: "default",
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc2-transformer",
					Namespace: "default",
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc2-explainer",
					Namespace: "default",
				},
			},
		).Build()

		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}

		err := reconciler.reconcileTopLevelHTTPRoute(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gatewayapiv1.HTTPRoute{}
		err = client.Get(context.TODO(), types.NamespacedName{
			Name:      "test-isvc2",
			Namespace: "default",
		}, route)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(route.Spec.Hostnames).To(ContainElement(gatewayapiv1.Hostname("test-isvc2-default.example.com")))
		g.Expect(route.Spec.Rules).ToNot(BeEmpty())
	})

	t.Run("does not create HTTPRoute if predictor not ready", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc3",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		client := fake.NewClientBuilder().WithScheme(s).Build()
		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}
		err := reconciler.reconcileTopLevelHTTPRoute(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gatewayapiv1.HTTPRoute{}
		err = client.Get(context.TODO(), types.NamespacedName{
			Name:      "test-isvc3",
			Namespace: "default",
		}, route)
		g.Expect(apierr.IsNotFound(err)).To(BeTrue())
	})
}

func TestRawHTTPRouteReconciler_Reconcile(t *testing.T) {
	g := NewGomegaWithT(t)

	// Setup minimal IngressConfig and InferenceServicesConfig
	ingressConfig := &v1beta1.IngressConfig{
		IngressDomain:        "example.com",
		UrlScheme:            "http",
		DomainTemplate:       "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
		KserveIngressGateway: "kserve/knative-gateway",
	}
	isvcConfig := &v1beta1.InferenceServicesConfig{
		ServiceAnnotationDisallowedList: []string{},
		ServiceLabelDisallowedList:      []string{},
	}

	// Setup scheme and fake client
	s := scheme.Scheme
	s.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.InferenceService{})
	s.AddKnownTypes(schema.GroupVersion{Group: gatewayapiv1.GroupVersion.Group, Version: gatewayapiv1.GroupVersion.Version}, &gatewayapiv1.HTTPRoute{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Service{})

	// Helper to create a ready HTTPRoute status
	readyHTTPRoute := func(name, namespace string) *gatewayapiv1.HTTPRoute {
		return &gatewayapiv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: gatewayapiv1.HTTPRouteStatus{
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
		}
	}

	t.Run("Reconcile disables ingress creation for cluster-local label", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "default",
				Labels: map[string]string{
					constants.NetworkVisibility: constants.ClusterLocalVisibility,
				},
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		client := fake.NewClientBuilder().WithScheme(s).Build()
		reconciler := NewRawHTTPRouteReconciler(client, s, ingressConfig, isvcConfig)
		err := reconciler.Reconcile(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
	})

	t.Run("Reconcile disables ingress creation for cluster-local domain", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		clusterLocalConfig := *ingressConfig
		clusterLocalConfig.IngressDomain = constants.ClusterLocalDomain
		client := fake.NewClientBuilder().WithScheme(s).Build()
		reconciler := NewRawHTTPRouteReconciler(client, s, &clusterLocalConfig, isvcConfig)
		err := reconciler.Reconcile(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
	})

	t.Run("Reconcile returns error if HTTPRoute not found", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		isvc.Status.SetCondition(v1beta1.PredictorReady, &apis.Condition{
			Type:   v1beta1.PredictorReady,
			Status: corev1.ConditionTrue,
		})
		client := fake.NewClientBuilder().WithScheme(s).Build()
		reconciler := NewRawHTTPRouteReconciler(client, s, ingressConfig, isvcConfig)
		err := reconciler.Reconcile(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("Reconcile sets IngressReady to False if HTTPRoute not ready", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		isvc.Status.SetCondition(v1beta1.PredictorReady, &apis.Condition{
			Type:   v1beta1.PredictorReady,
			Status: corev1.ConditionTrue,
		})
		notReadyHTTPRoute := &gatewayapiv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.PredictorServiceName(isvc.Name),
				Namespace: isvc.Namespace,
			},
			Status: gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:    string(gatewayapiv1.RouteConditionAccepted),
									Status:  metav1.ConditionFalse,
									Reason:  "NotAccepted",
									Message: "Route not accepted",
								},
							},
						},
					},
				},
			},
		}
		client := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(
				notReadyHTTPRoute,
				readyHTTPRoute(isvc.Name, isvc.Namespace),
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.PredictorServiceName(isvc.Name),
						Namespace: isvc.Namespace,
					},
				},
			).
			Build()
		reconciler := NewRawHTTPRouteReconciler(client, s, ingressConfig, isvcConfig)
		err := reconciler.Reconcile(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
		g.Expect(cond.Message).To(ContainSubstring("Predictor"))
	})

	t.Run("Reconcile with transformer and explainer, all HTTPRoutes ready", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor:   v1beta1.PredictorSpec{},
				Transformer: &v1beta1.TransformerSpec{},
				Explainer:   &v1beta1.ExplainerSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		isvc.Status.SetCondition(v1beta1.PredictorReady, &apis.Condition{
			Type:   v1beta1.PredictorReady,
			Status: corev1.ConditionTrue,
		})
		isvc.Status.SetCondition(v1beta1.TransformerReady, &apis.Condition{
			Type:   v1beta1.TransformerReady,
			Status: corev1.ConditionTrue,
		})
		isvc.Status.SetCondition(v1beta1.ExplainerReady, &apis.Condition{
			Type:   v1beta1.ExplainerReady,
			Status: corev1.ConditionTrue,
		})

		client := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(
				readyHTTPRoute(constants.PredictorServiceName(isvc.Name), isvc.Namespace),
				readyHTTPRoute(constants.TransformerServiceName(isvc.Name), isvc.Namespace),
				readyHTTPRoute(constants.ExplainerServiceName(isvc.Name), isvc.Namespace),
				readyHTTPRoute(isvc.Name, isvc.Namespace),
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.PredictorServiceName(isvc.Name),
						Namespace: isvc.Namespace,
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.TransformerServiceName(isvc.Name),
						Namespace: isvc.Namespace,
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.ExplainerServiceName(isvc.Name),
						Namespace: isvc.Namespace,
					},
				},
			).
			Build()

		reconciler := NewRawHTTPRouteReconciler(client, s, ingressConfig, isvcConfig)
		err := reconciler.Reconcile(context.TODO(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
	})
}
