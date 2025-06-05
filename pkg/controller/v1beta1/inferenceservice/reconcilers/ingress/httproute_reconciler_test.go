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
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

// httpRouteClientInterceptor wraps a client.Client to simulate errors for testing
type httpRouteClientInterceptor struct {
	client.Client
	blockHTTPRouteCreation bool
}

func (c *httpRouteClientInterceptor) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	// Allow HTTPRoute creation to proceed normally
	return c.Client.Create(ctx, obj, opts...)
}

// Get intercepts Get calls to return NotFound for HTTPRoutes when blocked
func (c *httpRouteClientInterceptor) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	// Return NotFound for HTTPRoute Get calls when blocked
	// This simulates the case where HTTPRoute was created but then deleted or not found during status checks
	if c.blockHTTPRouteCreation {
		if _, ok := obj.(*gwapiv1.HTTPRoute); ok {
			return apierr.NewNotFound(schema.GroupResource{
				Group:    gwapiv1.GroupVersion.Group,
				Resource: "httproutes",
			}, key.Name)
		}
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

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
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			host := getRawServiceHost(tc.isvc)
			g.Expect(tc.expectedHost).To(BeComparableTo(host))
		})
	}
}

func TestCreateHTTPRouteMatch(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := map[string]struct {
		prefix             string
		expectedHTTPRoutes gwapiv1.HTTPRouteMatch
	}{
		"basic case": {
			prefix: "^.*$",
			expectedHTTPRoutes: gwapiv1.HTTPRouteMatch{
				Path: &gwapiv1.HTTPPathMatch{
					Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
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
			g.Expect(headers.Type).To(BeComparableTo(gwapiv1.HTTPRouteFilterRequestHeaderModifier))
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
		matches       []gwapiv1.HTTPRouteMatch
		filters       []gwapiv1.HTTPRouteFilter
		serviceName   string
		servicePort   int32
		expectedRules int
	}{
		"basic case": {
			matches: []gwapiv1.HTTPRouteMatch{
				{
					Path: &gwapiv1.HTTPPathMatch{
						Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
						Value: ptr.To("/predict"),
					},
				},
			},
			filters: []gwapiv1.HTTPRouteFilter{
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
		desired       *gwapiv1.HTTPRoute
		existing      *gwapiv1.HTTPRoute
		expectedEqual bool
	}{
		"equal routes": {
			desired: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					Hostnames: []gwapiv1.Hostname{"example.com"},
				},
			},
			existing: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					Hostnames: []gwapiv1.Hostname{"example.com"},
				},
			},
			expectedEqual: true,
		},
		"different routes": {
			desired: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					Hostnames: []gwapiv1.Hostname{"example.com"},
				},
			},
			existing: &gwapiv1.HTTPRoute{
				Spec: gwapiv1.HTTPRouteSpec{
					Hostnames: []gwapiv1.Hostname{"different.com"},
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
		httpRouteStatus gwapiv1.HTTPRouteStatus
		expectedReady   bool
		expectedReason  *string
		expectedMessage *string
	}{
		"route accepted": {
			httpRouteStatus: gwapiv1.HTTPRouteStatus{
				RouteStatus: gwapiv1.RouteStatus{
					Parents: []gwapiv1.RouteParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:   string(gwapiv1.RouteConditionAccepted),
									Status: metav1.ConditionTrue,
								},
								{
									Type:   string(gwapiv1.RouteConditionResolvedRefs),
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
			httpRouteStatus: gwapiv1.HTTPRouteStatus{
				RouteStatus: gwapiv1.RouteStatus{
					Parents: []gwapiv1.RouteParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:    string(gwapiv1.RouteConditionAccepted),
									Status:  metav1.ConditionFalse,
									Reason:  "Route not accepted",
									Message: "Route not accepted",
								},
								{
									Type:   string(gwapiv1.RouteConditionResolvedRefs),
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
			httpRouteStatus: gwapiv1.HTTPRouteStatus{
				RouteStatus: gwapiv1.RouteStatus{
					Parents: []gwapiv1.RouteParentStatus{},
				},
			},
			expectedReady:   false,
			expectedReason:  ptr.To(HTTPRouteParentStatusNotAvailable),
			expectedMessage: ptr.To(HTTPRouteNotReady),
		},
		"resolved refs not ready": {
			httpRouteStatus: gwapiv1.HTTPRouteStatus{
				RouteStatus: gwapiv1.RouteStatus{
					Parents: []gwapiv1.RouteParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:   string(gwapiv1.RouteConditionAccepted),
									Status: metav1.ConditionTrue,
								},
								{
									Type:    string(gwapiv1.RouteConditionResolvedRefs),
									Status:  metav1.ConditionFalse,
									Reason:  "BackendNotFound",
									Message: "Backend service not found",
								},
							},
						},
					},
				},
			},
			expectedReady:   false,
			expectedReason:  ptr.To("BackendNotFound"),
			expectedMessage: ptr.To("Backend service not found"),
		},
		"multiple parents with one not ready": {
			httpRouteStatus: gwapiv1.HTTPRouteStatus{
				RouteStatus: gwapiv1.RouteStatus{
					Parents: []gwapiv1.RouteParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:   string(gwapiv1.RouteConditionAccepted),
									Status: metav1.ConditionTrue,
								},
								{
									Type:   string(gwapiv1.RouteConditionResolvedRefs),
									Status: metav1.ConditionTrue,
								},
							},
						},
						{
							Conditions: []metav1.Condition{
								{
									Type:    string(gwapiv1.RouteConditionAccepted),
									Status:  metav1.ConditionFalse,
									Reason:  "GatewayNotFound",
									Message: "Gateway not found",
								},
							},
						},
					},
				},
			},
			expectedReady:   false,
			expectedReason:  ptr.To("GatewayNotFound"),
			expectedMessage: ptr.To("Gateway not found"),
		},
		"both accepted and resolved refs false": {
			httpRouteStatus: gwapiv1.HTTPRouteStatus{
				RouteStatus: gwapiv1.RouteStatus{
					Parents: []gwapiv1.RouteParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:    string(gwapiv1.RouteConditionAccepted),
									Status:  metav1.ConditionFalse,
									Reason:  "NotAccepted",
									Message: "Route not accepted",
								},
								{
									Type:    string(gwapiv1.RouteConditionResolvedRefs),
									Status:  metav1.ConditionFalse,
									Reason:  "BackendNotFound",
									Message: "Backend not found",
								},
							},
						},
					},
				},
			},
			expectedReady:   false,
			expectedReason:  ptr.To("NotAccepted"),
			expectedMessage: ptr.To("Route not accepted"),
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
		expected      *gwapiv1.HTTPRoute
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
			expected: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gwapiv1.HTTPRouteSpec{
					Hostnames: []gwapiv1.Hostname{"test-isvc-default.example.com", "test-isvc-default.additional.example.com"},
					Rules: []gwapiv1.HTTPRouteRule{
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      ptr.To(gwapiv1.Kind(constants.GatewayKind)),
								Group:     (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
								Namespace: ptr.To(gwapiv1.Namespace("kserve")),
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
			expected: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gwapiv1.HTTPRouteSpec{
					Hostnames: []gwapiv1.Hostname{"test-isvc-default.example.com", "test-isvc-default.additional.example.com"},
					Rules: []gwapiv1.HTTPRouteRule{
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-transformer",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      ptr.To(gwapiv1.Kind(constants.GatewayKind)),
								Group:     (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
								Namespace: ptr.To(gwapiv1.Namespace("kserve")),
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
			expected: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gwapiv1.HTTPRouteSpec{
					Hostnames: []gwapiv1.Hostname{"test-isvc-default.example.com", "test-isvc-default.additional.example.com"},
					Rules: []gwapiv1.HTTPRouteRule{
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To(constants.ExplainPrefix()),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      ptr.To(gwapiv1.Kind(constants.GatewayKind)),
								Group:     (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
								Namespace: ptr.To(gwapiv1.Namespace("kserve")),
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
			expected: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gwapiv1.HTTPRouteSpec{
					Hostnames: []gwapiv1.Hostname{"test-isvc-default.example.com", "test-isvc-default.additional.example.com", "example.com"},
					Rules: []gwapiv1.HTTPRouteRule{
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To(constants.ExplainPrefix()),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To("/serving/default/test-isvc" + constants.PathBasedExplainPrefix()),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To("/serving/default/test-isvc/"),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      ptr.To(gwapiv1.Kind(constants.GatewayKind)),
								Group:     (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
								Namespace: ptr.To(gwapiv1.Namespace("kserve")),
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
			expected: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gwapiv1.HTTPRouteSpec{
					Hostnames: []gwapiv1.Hostname{"test-isvc-default.example.com", "test-isvc-default.additional.example.com", "example.com"},
					Rules: []gwapiv1.HTTPRouteRule{
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-transformer",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To("/serving/default/test-isvc/"),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-transformer",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{
								Name:      "kserve-gateway",
								Kind:      ptr.To(gwapiv1.Kind(constants.GatewayKind)),
								Group:     (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
								Namespace: ptr.To(gwapiv1.Namespace("kserve")),
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
			s.AddKnownTypes(schema.GroupVersion{Group: gwapiv1.GroupVersion.Group, Version: gwapiv1.GroupVersion.Version},
				&gwapiv1.HTTPRoute{})
			client := fake.NewClientBuilder().WithScheme(s).Build()
			// Create a dummy service to test default suffix case
			client.Create(t.Context(), &corev1.Service{
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
			httpRoute, err := createRawTopLevelHTTPRoute(tc.isvc, tc.ingressConfig, isvcConfig)

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
		expected      *gwapiv1.HTTPRoute
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
			expected: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc-predictor",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gwapiv1.HTTPRouteSpec{
					Hostnames: []gwapiv1.Hostname{"test-isvc-predictor-default.example.com"},
					Rules: []gwapiv1.HTTPRouteRule{
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-predictor",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{
								Group:     (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
								Kind:      (*gwapiv1.Kind)(ptr.To(constants.GatewayKind)),
								Namespace: (*gwapiv1.Namespace)(ptr.To("kserve")),
								Name:      gwapiv1.ObjectName("kserve-gateway"),
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
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			s := scheme.Scheme
			s.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.InferenceService{})
			s.AddKnownTypes(schema.GroupVersion{Group: gwapiv1.GroupVersion.Group, Version: gwapiv1.GroupVersion.Version},
				&gwapiv1.HTTPRoute{})
			client := fake.NewClientBuilder().WithScheme(s).Build()
			// Create a dummy service to test default suffix case
			client.Create(t.Context(), &corev1.Service{
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
			httpRoute, err := createRawPredictorHTTPRoute(tc.isvc, tc.ingressConfig, isvcConfig)

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
		expected      *gwapiv1.HTTPRoute
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
			expected: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc-transformer",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gwapiv1.HTTPRouteSpec{
					Hostnames: []gwapiv1.Hostname{"test-isvc-transformer-default.example.com"},
					Rules: []gwapiv1.HTTPRouteRule{
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-transformer",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{
								Group:     (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
								Kind:      (*gwapiv1.Kind)(ptr.To(constants.GatewayKind)),
								Namespace: (*gwapiv1.Namespace)(ptr.To("kserve")),
								Name:      gwapiv1.ObjectName("kserve-gateway"),
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
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			s := scheme.Scheme
			s.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.InferenceService{})
			s.AddKnownTypes(schema.GroupVersion{Group: gwapiv1.GroupVersion.Group, Version: gwapiv1.GroupVersion.Version},
				&gwapiv1.HTTPRoute{})
			client := fake.NewClientBuilder().WithScheme(s).Build()
			// Create a dummy service to test default suffix case
			client.Create(t.Context(), &corev1.Service{
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
			httpRoute, err := createRawTransformerHTTPRoute(tc.isvc, tc.ingressConfig, isvcConfig)

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
		expected      *gwapiv1.HTTPRoute
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
			expected: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-isvc-explainer",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gwapiv1.HTTPRouteSpec{
					Hostnames: []gwapiv1.Hostname{"test-isvc-explainer-default.example.com"},
					Rules: []gwapiv1.HTTPRouteRule{
						{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchRegularExpression),
										Value: ptr.To("^/.*$"),
									},
								},
							},
							Filters: []gwapiv1.HTTPRouteFilter{
								{
									Type: gwapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwapiv1.HTTPHeaderFilter{
										Set: []gwapiv1.HTTPHeader{
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
							BackendRefs: []gwapiv1.HTTPBackendRef{
								{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Kind:      ptr.To(gwapiv1.Kind(constants.ServiceKind)),
											Name:      "test-isvc-explainer",
											Namespace: (*gwapiv1.Namespace)(ptr.To("default")),
											Port:      (*gwapiv1.PortNumber)(ptr.To(int32(constants.CommonDefaultHttpPort))),
										},
									},
								},
							},
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("60s")),
							},
						},
					},
					CommonRouteSpec: gwapiv1.CommonRouteSpec{
						ParentRefs: []gwapiv1.ParentReference{
							{
								Group:     (*gwapiv1.Group)(&gwapiv1.GroupVersion.Group),
								Kind:      (*gwapiv1.Kind)(ptr.To(constants.GatewayKind)),
								Namespace: (*gwapiv1.Namespace)(ptr.To("kserve")),
								Name:      gwapiv1.ObjectName("kserve-gateway"),
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
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			s := scheme.Scheme
			s.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.InferenceService{})
			s.AddKnownTypes(schema.GroupVersion{Group: gwapiv1.GroupVersion.Group, Version: gwapiv1.GroupVersion.Version},
				&gwapiv1.HTTPRoute{})
			client := fake.NewClientBuilder().WithScheme(s).Build()
			// Create a dummy service to test default suffix case
			client.Create(t.Context(), &corev1.Service{
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
			httpRoute, err := createRawExplainerHTTPRoute(tc.isvc, tc.ingressConfig, isvcConfig)

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
	_ = gwapiv1.Install(s)
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

		err := reconciler.reconcilePredictorHTTPRoute(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gwapiv1.HTTPRoute{}
		err = client.Get(t.Context(), types.NamespacedName{
			Name:      "foo-predictor",
			Namespace: "default",
		}, route)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(route.Spec.Hostnames).To(ContainElement(gwapiv1.Hostname("foo-predictor-default.example.com")))
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

		err := reconciler.reconcilePredictorHTTPRoute(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gwapiv1.HTTPRoute{}
		err = client.Get(t.Context(), types.NamespacedName{
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
		existing := &gwapiv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo-predictor",
				Namespace: "default",
			},
			Spec: gwapiv1.HTTPRouteSpec{
				Hostnames: []gwapiv1.Hostname{"old-host.example.com"},
			},
		}
		_ = client.Create(t.Context(), existing)

		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}

		err := reconciler.reconcilePredictorHTTPRoute(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gwapiv1.HTTPRoute{}
		err = client.Get(t.Context(), types.NamespacedName{
			Name:      "foo-predictor",
			Namespace: "default",
		}, route)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(route.Spec.Hostnames).To(ContainElement(gwapiv1.Hostname("foo-predictor-default.example.com")))
	})
}

func TestRawHTTPRouteReconciler_reconcileTransformerHTTPRoute(t *testing.T) {
	g := NewGomegaWithT(t)
	s := scheme.Scheme
	s.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.InferenceService{})
	s.AddKnownTypes(schema.GroupVersion{Group: gwapiv1.GroupVersion.Group, Version: gwapiv1.GroupVersion.Version}, &gwapiv1.HTTPRoute{})
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
		client.Create(t.Context(), &corev1.Service{
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
		err := reconciler.reconcileTransformerHTTPRoute(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gwapiv1.HTTPRoute{}
		err = client.Get(t.Context(), types.NamespacedName{
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
		client.Create(t.Context(), &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc2-transformer",
				Namespace: "default",
			},
		})
		// Create an existing HTTPRoute with different spec
		existingRoute := &gwapiv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc2-transformer",
				Namespace: "default",
			},
			Spec: gwapiv1.HTTPRouteSpec{
				Hostnames: []gwapiv1.Hostname{"oldhost.example.com"},
			},
		}
		client.Create(t.Context(), existingRoute)

		reconciler := &RawHTTPRouteReconciler{
			client:        client,
			scheme:        s,
			ingressConfig: ingressConfig,
			isvcConfig:    isvcConfig,
		}
		err := reconciler.reconcileTransformerHTTPRoute(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gwapiv1.HTTPRoute{}
		err = client.Get(t.Context(), types.NamespacedName{
			Name:      "test-isvc2-transformer",
			Namespace: "default",
		}, route)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(route.Spec.Hostnames).To(ContainElement(gwapiv1.Hostname("test-isvc2-transformer-default.example.com")))
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
		err := reconciler.reconcileTransformerHTTPRoute(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
	})
}

func TestRawHTTPRouteReconciler_reconcileExplainerHTTPRoute(t *testing.T) {
	g := NewGomegaWithT(t)
	s := scheme.Scheme
	s.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.InferenceService{})
	s.AddKnownTypes(schema.GroupVersion{Group: gwapiv1.GroupVersion.Group, Version: gwapiv1.GroupVersion.Version}, &gwapiv1.HTTPRoute{})
	ctx := t.Context()

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

		route := &gwapiv1.HTTPRoute{}
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

		route := &gwapiv1.HTTPRoute{}
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
		s.AddKnownTypes(schema.GroupVersion{Group: gwapiv1.GroupVersion.Group, Version: gwapiv1.GroupVersion.Version}, &gwapiv1.HTTPRoute{})
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

		route := &gwapiv1.HTTPRoute{}
		_ = client.Get(ctx, types.NamespacedName{Name: "foo-explainer-default", Namespace: "bar"}, route)
	})
}

func TestRawHTTPRouteReconciler_reconcileTopLevelHTTPRoute(t *testing.T) {
	g := NewGomegaWithT(t)
	s := scheme.Scheme
	_ = gwapiv1.Install(s)
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

		err := reconciler.reconcileTopLevelHTTPRoute(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gwapiv1.HTTPRoute{}
		err = client.Get(t.Context(), types.NamespacedName{
			Name:      "test-isvc",
			Namespace: "default",
		}, route)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(route.Spec.Hostnames).To(ContainElement(gwapiv1.Hostname("test-isvc-default.example.com")))
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

		err := reconciler.reconcileTopLevelHTTPRoute(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gwapiv1.HTTPRoute{}
		err = client.Get(t.Context(), types.NamespacedName{
			Name:      "test-isvc2",
			Namespace: "default",
		}, route)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(route.Spec.Hostnames).To(ContainElement(gwapiv1.Hostname("test-isvc2-default.example.com")))
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
		err := reconciler.reconcileTopLevelHTTPRoute(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())

		route := &gwapiv1.HTTPRoute{}
		err = client.Get(t.Context(), types.NamespacedName{
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
	s.AddKnownTypes(schema.GroupVersion{Group: gwapiv1.GroupVersion.Group, Version: gwapiv1.GroupVersion.Version}, &gwapiv1.HTTPRoute{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Service{})

	// Helper to create a ready HTTPRoute status
	readyHTTPRoute := func(name, namespace string) *gwapiv1.HTTPRoute {
		return &gwapiv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: gwapiv1.HTTPRouteStatus{
				RouteStatus: gwapiv1.RouteStatus{
					Parents: []gwapiv1.RouteParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:   string(gwapiv1.RouteConditionAccepted),
									Status: metav1.ConditionTrue,
								},
								{
									Type:   string(gwapiv1.RouteConditionResolvedRefs),
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
		result, err := reconciler.Reconcile(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{}))
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
		result, err := reconciler.Reconcile(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{}))
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
	})

	t.Run("Reconcile returns requeue if HTTPRoute not found", func(t *testing.T) {
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
		result, err := reconciler.Reconcile(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
		// When HTTPRoutes are newly created, they have empty status which leads to ParentStatusNotAvailable
		g.Expect(cond.Reason).To(Equal("ParentStatusNotAvailable"))
		g.Expect(cond.Message).To(Equal("Predictor HttpRouteNotReady"))
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
		notReadyHTTPRoute := &gwapiv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.PredictorServiceName(isvc.Name),
				Namespace: isvc.Namespace,
			},
			Status: gwapiv1.HTTPRouteStatus{
				RouteStatus: gwapiv1.RouteStatus{
					Parents: []gwapiv1.RouteParentStatus{
						{
							Conditions: []metav1.Condition{
								{
									Type:    string(gwapiv1.RouteConditionAccepted),
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
		result, err := reconciler.Reconcile(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
		g.Expect(cond.Reason).To(Equal("ParentStatusNotAvailable"))
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
		result, err := reconciler.Reconcile(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		// HTTPRoutes get updated by reconciler which resets their status, causing requeue
		g.Expect(result.Requeue).To(BeTrue())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
	})

	t.Run("Reconcile requeues when HTTPRoute not found", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc-requeue",
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

		// Create client without HTTPRoute objects - HTTPRoutes get created during reconciliation but have empty status
		client := fake.NewClientBuilder().WithScheme(s).Build()
		reconciler := NewRawHTTPRouteReconciler(client, s, ingressConfig, isvcConfig)
		result, err := reconciler.Reconcile(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
		// HTTPRoutes are created during reconciliation but have empty status, leading to ParentStatusNotAvailable
		g.Expect(cond.Reason).To(Equal("ParentStatusNotAvailable"))
		g.Expect(cond.Message).To(ContainSubstring("HttpRouteNotReady"))
	})

	t.Run("Reconcile requeues when HTTPRoute not ready", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc-not-ready",
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

		// Create HTTPRoute with not ready status - needs to have the correct spec to avoid being updated
		desiredPredictorRoute, err := createRawPredictorHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredPredictorRoute).NotTo(BeNil())

		notReadyHTTPRoute := desiredPredictorRoute.DeepCopy()
		// Set controller reference to match what the reconciler will set
		err = controllerutil.SetControllerReference(isvc, notReadyHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())
		notReadyHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:    string(gwapiv1.RouteConditionAccepted),
								Status:  metav1.ConditionFalse,
								Reason:  "NotAccepted",
								Message: "Route not accepted by gateway",
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
				readyHTTPRoute(isvc.Name, isvc.Namespace), // Top-level route is ready
			).
			Build()

		reconciler := NewRawHTTPRouteReconciler(client, s, ingressConfig, isvcConfig)
		result, err := reconciler.Reconcile(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
		g.Expect(cond.Reason).To(Equal("NotAccepted"))
		g.Expect(cond.Message).To(ContainSubstring("Predictor"))
	})

	t.Run("Reconcile requeues when transformer HTTPRoute not ready", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc-transformer",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor:   v1beta1.PredictorSpec{},
				Transformer: &v1beta1.TransformerSpec{},
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

		// Create ready predictor HTTPRoute but not ready transformer HTTPRoute
		desiredPredictorRoute, err := createRawPredictorHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredPredictorRoute).NotTo(BeNil())

		readyPredictorHTTPRoute := desiredPredictorRoute.DeepCopy()
		// Set controller reference to match what the reconciler will set
		err = controllerutil.SetControllerReference(isvc, readyPredictorHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())
		readyPredictorHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:   string(gwapiv1.RouteConditionAccepted),
								Status: metav1.ConditionTrue,
							},
							{
								Type:   string(gwapiv1.RouteConditionResolvedRefs),
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
		}

		desiredTransformerRoute, err := createRawTransformerHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredTransformerRoute).NotTo(BeNil())

		notReadyTransformerHTTPRoute := desiredTransformerRoute.DeepCopy()
		// Set controller reference to match what the reconciler will set
		err = controllerutil.SetControllerReference(isvc, notReadyTransformerHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Create a temporary client to perform dry-run update on the test HTTPRoute
		// This ensures it has the same defaults as what the reconciler will apply
		tempClient := fake.NewClientBuilder().WithScheme(s).Build()
		err = tempClient.Create(t.Context(), notReadyTransformerHTTPRoute)
		g.Expect(err).ToNot(HaveOccurred())

		// Perform the same dry-run update that the reconciler will do
		err = tempClient.Update(t.Context(), notReadyTransformerHTTPRoute, client.DryRunAll)
		g.Expect(err).ToNot(HaveOccurred())

		notReadyTransformerHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:    string(gwapiv1.RouteConditionResolvedRefs),
								Status:  metav1.ConditionFalse,
								Reason:  "BackendNotFound",
								Message: "Backend service not found",
							},
						},
					},
				},
			},
		}

		// Create ready top-level HTTPRoute
		desiredTopLevelRoute, err := createRawTopLevelHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredTopLevelRoute).NotTo(BeNil())

		readyTopLevelHTTPRoute := desiredTopLevelRoute.DeepCopy()
		// Set controller reference to match what the reconciler will set
		err = controllerutil.SetControllerReference(isvc, readyTopLevelHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())
		readyTopLevelHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:   string(gwapiv1.RouteConditionAccepted),
								Status: metav1.ConditionTrue,
							},
							{
								Type:   string(gwapiv1.RouteConditionResolvedRefs),
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(
				readyPredictorHTTPRoute,
				notReadyTransformerHTTPRoute,
				readyTopLevelHTTPRoute,
			).
			Build()

		reconciler := NewRawHTTPRouteReconciler(client, s, ingressConfig, isvcConfig)
		result, err := reconciler.Reconcile(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
		g.Expect(cond.Reason).To(Equal("BackendNotFound"))
		g.Expect(cond.Message).To(ContainSubstring("Transformer"))
	})

	t.Run("Reconcile requeues when explainer HTTPRoute not ready", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc-explainer",
				Namespace: "default",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{},
				Explainer: &v1beta1.ExplainerSpec{},
			},
			Status: v1beta1.InferenceServiceStatus{},
		}
		isvc.Status.SetCondition(v1beta1.PredictorReady, &apis.Condition{
			Type:   v1beta1.PredictorReady,
			Status: corev1.ConditionTrue,
		})
		isvc.Status.SetCondition(v1beta1.ExplainerReady, &apis.Condition{
			Type:   v1beta1.ExplainerReady,
			Status: corev1.ConditionTrue,
		})

		// Create ready predictor HTTPRoute but not ready explainer HTTPRoute
		desiredPredictorRoute, err := createRawPredictorHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredPredictorRoute).NotTo(BeNil())

		readyPredictorHTTPRoute := desiredPredictorRoute.DeepCopy()
		// Set controller reference to match what the reconciler will set
		err = controllerutil.SetControllerReference(isvc, readyPredictorHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())
		readyPredictorHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:   string(gwapiv1.RouteConditionAccepted),
								Status: metav1.ConditionTrue,
							},
							{
								Type:   string(gwapiv1.RouteConditionResolvedRefs),
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
		}

		desiredExplainerRoute, err := createRawExplainerHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredExplainerRoute).NotTo(BeNil())

		notReadyExplainerHTTPRoute := desiredExplainerRoute.DeepCopy()
		// Set controller reference to match what the reconciler will set
		err = controllerutil.SetControllerReference(isvc, notReadyExplainerHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())
		notReadyExplainerHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:    string(gwapiv1.RouteConditionAccepted),
								Status:  metav1.ConditionFalse,
								Reason:  "UnsupportedProtocol",
								Message: "Protocol not supported",
							},
						},
					},
				},
			},
		}

		desiredTopLevelRoute, err := createRawTopLevelHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredTopLevelRoute).NotTo(BeNil())

		readyTopLevelHTTPRoute := desiredTopLevelRoute.DeepCopy()
		// Set controller reference to match what the reconciler will set
		err = controllerutil.SetControllerReference(isvc, readyTopLevelHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())
		readyTopLevelHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:   string(gwapiv1.RouteConditionAccepted),
								Status: metav1.ConditionTrue,
							},
							{
								Type:   string(gwapiv1.RouteConditionResolvedRefs),
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(
				readyPredictorHTTPRoute,
				notReadyExplainerHTTPRoute,
				readyTopLevelHTTPRoute,
			).
			Build()

		reconciler := NewRawHTTPRouteReconciler(client, s, ingressConfig, isvcConfig)
		result, err := reconciler.Reconcile(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
		g.Expect(cond.Reason).To(Equal("UnsupportedProtocol"))
		g.Expect(cond.Message).To(ContainSubstring("Explainer"))
	})

	t.Run("Reconcile requeues when top-level HTTPRoute not ready", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc-toplevel",
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

		// Create ready predictor HTTPRoute but not ready top-level HTTPRoute
		desiredTopLevelRoute, err := createRawTopLevelHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredTopLevelRoute).NotTo(BeNil())

		notReadyTopLevelHTTPRoute := desiredTopLevelRoute.DeepCopy()
		// Set controller reference to match what the reconciler will set
		err = controllerutil.SetControllerReference(isvc, notReadyTopLevelHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())
		notReadyTopLevelHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:   string(gwapiv1.RouteConditionAccepted),
								Status: metav1.ConditionTrue,
							},
							{
								Type:    string(gwapiv1.RouteConditionResolvedRefs),
								Status:  metav1.ConditionFalse,
								Reason:  "InvalidKind",
								Message: "Invalid backend kind",
							},
						},
					},
				},
			},
		}

		desiredPredictorRoute, err := createRawPredictorHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredPredictorRoute).NotTo(BeNil())

		readyPredictorHTTPRoute := desiredPredictorRoute.DeepCopy()
		// Set controller reference to match what the reconciler will set
		err = controllerutil.SetControllerReference(isvc, readyPredictorHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())
		readyPredictorHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:   string(gwapiv1.RouteConditionAccepted),
								Status: metav1.ConditionTrue,
							},
							{
								Type:   string(gwapiv1.RouteConditionResolvedRefs),
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(
				readyPredictorHTTPRoute,
				notReadyTopLevelHTTPRoute,
			).
			Build()

		reconciler := NewRawHTTPRouteReconciler(client, s, ingressConfig, isvcConfig)
		result, err := reconciler.Reconcile(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
		g.Expect(cond.Reason).To(Equal("InvalidKind"))
		g.Expect(cond.Message).To(ContainSubstring("InferenceService"))
	})

	t.Run("Reconcile does not requeue when all HTTPRoutes are ready", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc-all-ready",
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

		desiredPredictorRoute, err := createRawPredictorHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredPredictorRoute).NotTo(BeNil())

		readyPredictorHTTPRoute := desiredPredictorRoute.DeepCopy()
		err = controllerutil.SetControllerReference(isvc, readyPredictorHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())
		readyPredictorHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:   string(gwapiv1.RouteConditionAccepted),
								Status: metav1.ConditionTrue,
							},
							{
								Type:   string(gwapiv1.RouteConditionResolvedRefs),
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
		}

		desiredTransformerRoute, err := createRawTransformerHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredTransformerRoute).NotTo(BeNil())

		readyTransformerHTTPRoute := desiredTransformerRoute.DeepCopy()
		err = controllerutil.SetControllerReference(isvc, readyTransformerHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())
		readyTransformerHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:   string(gwapiv1.RouteConditionAccepted),
								Status: metav1.ConditionTrue,
							},
							{
								Type:   string(gwapiv1.RouteConditionResolvedRefs),
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
		}

		desiredExplainerRoute, err := createRawExplainerHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredExplainerRoute).NotTo(BeNil())

		readyExplainerHTTPRoute := desiredExplainerRoute.DeepCopy()
		err = controllerutil.SetControllerReference(isvc, readyExplainerHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())
		readyExplainerHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:   string(gwapiv1.RouteConditionAccepted),
								Status: metav1.ConditionTrue,
							},
							{
								Type:   string(gwapiv1.RouteConditionResolvedRefs),
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
		}

		desiredTopLevelRoute, err := createRawTopLevelHTTPRoute(isvc, ingressConfig, isvcConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredTopLevelRoute).NotTo(BeNil())

		readyTopLevelHTTPRoute := desiredTopLevelRoute.DeepCopy()
		err = controllerutil.SetControllerReference(isvc, readyTopLevelHTTPRoute, s)
		g.Expect(err).ToNot(HaveOccurred())
		readyTopLevelHTTPRoute.Status = gwapiv1.HTTPRouteStatus{
			RouteStatus: gwapiv1.RouteStatus{
				Parents: []gwapiv1.RouteParentStatus{
					{
						Conditions: []metav1.Condition{
							{
								Type:   string(gwapiv1.RouteConditionAccepted),
								Status: metav1.ConditionTrue,
							},
							{
								Type:   string(gwapiv1.RouteConditionResolvedRefs),
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(
				readyPredictorHTTPRoute,
				readyTransformerHTTPRoute,
				readyExplainerHTTPRoute,
				readyTopLevelHTTPRoute,
			).
			Build()

		reconciler := NewRawHTTPRouteReconciler(client, s, ingressConfig, isvcConfig)
		result, err := reconciler.Reconcile(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeFalse())
		g.Expect(result.RequeueAfter).To(BeZero())
		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
	})

	t.Run("Reconcile handles HTTPRoute not found (IsNotFound error)", func(t *testing.T) {
		// This test specifically covers the edge case where checkHTTPRouteStatuses
		// encounters an IsNotFound error when trying to get HTTPRoutes
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc-not-found",
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

		// Create an interceptor client that blocks HTTPRoute creation
		// This ensures that when checkHTTPRouteStatuses tries to get HTTPRoutes,
		// it will encounter IsNotFound errors and trigger the edge case
		baseClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(
				// Add a Service to simulate that components are ready
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.PredictorServiceName(isvc.Name),
						Namespace: isvc.Namespace,
					},
				},
			).
			Build()

		interceptorClient := &httpRouteClientInterceptor{
			Client:                 baseClient,
			blockHTTPRouteCreation: true,
		}

		reconciler := NewRawHTTPRouteReconciler(interceptorClient, s, ingressConfig, isvcConfig)
		result, err := reconciler.Reconcile(t.Context(), isvc)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())

		cond := isvc.Status.GetCondition(v1beta1.IngressReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
		// This is the specific edge case we're testing - when HTTPRoute is not found,
		// the condition should be set with reason "Predictor Deployment NotReady" and
		// message "Predictor HTTPRoute not created"
		g.Expect(cond.Reason).To(Equal("Predictor Deployment NotReady"))
		g.Expect(cond.Message).To(Equal("Predictor HTTPRoute not created"))
	})
}
