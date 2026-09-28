/*
Copyright 2026 The KServe Authors.

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

package llmisvc

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	kservetesting "github.com/kserve/kserve/pkg/testing"
)

const modelRoutingTestHeader = "X-Gateway-Model-Name"

// renderShippedRouterRoute renders config-llm-router-route.yaml for llmSvc, so
// the strip is exercised against the rules users actually get rather than a
// hand-built approximation.
func renderShippedRouterRoute(t *testing.T, llmSvc *v1alpha2.LLMInferenceService) []gwapiv1.HTTPRouteRule {
	t.Helper()

	path := filepath.Join(kservetesting.ProjectRoot(), "config", "llmisvcconfig", "config-llm-router-route.yaml")
	data, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)

	cfg := &v1alpha2.LLMInferenceServiceConfig{}
	require.NoError(t, yaml.Unmarshal(data, cfg))

	rendered, err := ReplaceVariables(llmSvc, cfg, &Config{ModelBasedRoutingHeaderName: modelRoutingTestHeader})
	require.NoError(t, err)
	require.NotNil(t, rendered.Spec.Router)
	require.NotNil(t, rendered.Spec.Router.Route)
	require.True(t, rendered.Spec.Router.Route.HTTP.HasSpec())

	return rendered.Spec.Router.Route.HTTP.Spec.Rules
}

func TestStripPerModelPathMatches_ShippedRoute(t *testing.T) {
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "granite", Namespace: "team-a"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{Name: ptr.To("ibm/granite-3")},
		},
	}
	rules := renderShippedRouterRoute(t, llmSvc)

	modelRoutingMatches := 0
	for _, rule := range rules {
		for _, match := range rule.Matches {
			if isModelBasedRoutingMatch(match, modelRoutingTestHeader) {
				modelRoutingMatches++
			}
		}
	}
	require.Positive(t, modelRoutingMatches, "shipped route lost its model-routing tree")

	got := stripPerModelPathMatches(rules, perModelPathPrefixes(llmSvc), modelRoutingTestHeader)

	names := make([]string, 0, len(got))
	remaining := 0
	for _, rule := range got {
		names = append(names, string(ptr.Deref(rule.Name, "")))
		for _, match := range rule.Matches {
			assert.True(t, isModelBasedRoutingMatch(match, modelRoutingTestHeader),
				"rule %q kept a match outside the model-routing tree: %+v", ptr.Deref(rule.Name, ""), match)
			remaining++
		}
	}
	assert.Equal(t, []string{"v1-model-routing", "v1-catch-all-model-routing"}, names)
	assert.Equal(t, modelRoutingMatches, remaining, "model-routing matches must survive untouched")
}

func TestStripPerModelPathMatches(t *testing.T) {
	perModel := []string{"/ns/name", "/publishers/ns/models/org/model"}

	pathMatch := func(matchType gwapiv1.PathMatchType, value string) gwapiv1.HTTPRouteMatch {
		return gwapiv1.HTTPRouteMatch{
			Path: &gwapiv1.HTTPPathMatch{Type: ptr.To(matchType), Value: ptr.To(value)},
		}
	}
	prefixMatch := func(value string) gwapiv1.HTTPRouteMatch {
		return pathMatch(gwapiv1.PathMatchPathPrefix, value)
	}
	withHeader := func(m gwapiv1.HTTPRouteMatch, name, value string) gwapiv1.HTTPRouteMatch {
		m.Headers = append(m.Headers, gwapiv1.HTTPHeaderMatch{
			Type:  ptr.To(gwapiv1.HeaderMatchExact),
			Name:  gwapiv1.HTTPHeaderName(name),
			Value: value,
		})
		return m
	}
	rule := func(name string, matches ...gwapiv1.HTTPRouteMatch) gwapiv1.HTTPRouteRule {
		return gwapiv1.HTTPRouteRule{Name: ptr.To(gwapiv1.SectionName(name)), Matches: matches}
	}

	tests := []struct {
		name     string
		rules    []gwapiv1.HTTPRouteRule
		prefixes []string
		want     []gwapiv1.HTTPRouteRule
	}{
		{
			name: "strips endpoint and catch-all matches under both per-model prefixes",
			rules: []gwapiv1.HTTPRouteRule{
				rule("completions", prefixMatch("/ns/name/v1/completions")),
				rule("catch-all", prefixMatch("/ns/name")),
				rule("publisher-chat", prefixMatch("/publishers/ns/models/org/model/v1/chat/completions")),
				rule("publisher-catch-all", prefixMatch("/publishers/ns/models/org/model")),
			},
			prefixes: perModel,
			want:     nil,
		},
		{
			name: "keeps model-routing matches even when their path sits under a per-model prefix",
			rules: []gwapiv1.HTTPRouteRule{
				rule("model-routing",
					withHeader(pathMatch(gwapiv1.PathMatchExact, "/v1/completions"), modelRoutingTestHeader, "publishers/ns/models/org/model"),
					withHeader(prefixMatch("/ns/name/v1/completions"), modelRoutingTestHeader, "publishers/ns/models/org/model"),
				),
			},
			prefixes: perModel,
			want: []gwapiv1.HTTPRouteRule{
				rule("model-routing",
					withHeader(pathMatch(gwapiv1.PathMatchExact, "/v1/completions"), modelRoutingTestHeader, "publishers/ns/models/org/model"),
					withHeader(prefixMatch("/ns/name/v1/completions"), modelRoutingTestHeader, "publishers/ns/models/org/model"),
				),
			},
		},
		{
			name: "compares the model-routing header name case-insensitively",
			rules: []gwapiv1.HTTPRouteRule{
				rule("model-routing", withHeader(prefixMatch("/ns/name"), "x-gateway-model-name", "publishers/ns/models/org/model")),
			},
			prefixes: perModel,
			want: []gwapiv1.HTTPRouteRule{
				rule("model-routing", withHeader(prefixMatch("/ns/name"), "x-gateway-model-name", "publishers/ns/models/org/model")),
			},
		},
		{
			name: "strips a per-model path that also carries an unrelated header",
			rules: []gwapiv1.HTTPRouteRule{
				rule("canary", withHeader(prefixMatch("/ns/name/v1/completions"), "X-Canary", "true")),
			},
			prefixes: perModel,
			want:     nil,
		},
		{
			name: "strips an Exact match on the prefix itself",
			rules: []gwapiv1.HTTPRouteRule{
				rule("exact", pathMatch(gwapiv1.PathMatchExact, "/ns/name")),
			},
			prefixes: perModel,
			want:     nil,
		},
		{
			name: "treats a path match without a type as PathPrefix",
			rules: []gwapiv1.HTTPRouteRule{
				rule("untyped", gwapiv1.HTTPRouteMatch{Path: &gwapiv1.HTTPPathMatch{Value: ptr.To("/ns/name/v1/responses")}}),
			},
			prefixes: perModel,
			want:     nil,
		},
		{
			name: "keeps paths that only share a string prefix with a per-model prefix",
			rules: []gwapiv1.HTTPRouteRule{
				rule("sibling", prefixMatch("/ns/name-other/v1/completions")),
				rule("sibling-model", prefixMatch("/publishers/ns/models/org/model-2")),
			},
			prefixes: perModel,
			want: []gwapiv1.HTTPRouteRule{
				rule("sibling", prefixMatch("/ns/name-other/v1/completions")),
				rule("sibling-model", prefixMatch("/publishers/ns/models/org/model-2")),
			},
		},
		{
			name: "keeps a user rule on an unrelated path",
			rules: []gwapiv1.HTTPRouteRule{
				rule("health", prefixMatch("/healthz")),
			},
			prefixes: perModel,
			want: []gwapiv1.HTTPRouteRule{
				rule("health", prefixMatch("/healthz")),
			},
		},
		{
			name: "keeps a regex path match, whose value is not a literal path",
			rules: []gwapiv1.HTTPRouteRule{
				rule("regex", pathMatch(gwapiv1.PathMatchRegularExpression, "/ns/name/v1/.*")),
			},
			prefixes: perModel,
			want: []gwapiv1.HTTPRouteRule{
				rule("regex", pathMatch(gwapiv1.PathMatchRegularExpression, "/ns/name/v1/.*")),
			},
		},
		{
			name: "keeps a path match without a value, which Gateway API reads as /",
			rules: []gwapiv1.HTTPRouteRule{
				rule("root", gwapiv1.HTTPRouteMatch{Path: &gwapiv1.HTTPPathMatch{Type: ptr.To(gwapiv1.PathMatchPathPrefix)}}),
			},
			prefixes: perModel,
			want: []gwapiv1.HTTPRouteRule{
				rule("root", gwapiv1.HTTPRouteMatch{Path: &gwapiv1.HTTPPathMatch{Type: ptr.To(gwapiv1.PathMatchPathPrefix)}}),
			},
		},
		{
			name: "keeps a header-only match without a path",
			rules: []gwapiv1.HTTPRouteRule{
				rule("header-only", gwapiv1.HTTPRouteMatch{Headers: []gwapiv1.HTTPHeaderMatch{{Name: "X-Tenant", Value: "a"}}}),
			},
			prefixes: perModel,
			want: []gwapiv1.HTTPRouteRule{
				rule("header-only", gwapiv1.HTTPRouteMatch{Headers: []gwapiv1.HTTPHeaderMatch{{Name: "X-Tenant", Value: "a"}}}),
			},
		},
		{
			// A rule with an empty match list applies to every request, so it
			// is not a per-model path and must not be dropped as "emptied".
			name: "keeps a rule that had no matches to begin with",
			rules: []gwapiv1.HTTPRouteRule{
				rule("match-all"),
			},
			prefixes: perModel,
			want: []gwapiv1.HTTPRouteRule{
				rule("match-all"),
			},
		},
		{
			name: "mixed rule: strips only the per-model match",
			rules: []gwapiv1.HTTPRouteRule{
				rule("mixed", prefixMatch("/ns/name/v1/completions"), prefixMatch("/legacy/completions")),
			},
			prefixes: perModel,
			want: []gwapiv1.HTTPRouteRule{
				rule("mixed", prefixMatch("/legacy/completions")),
			},
		},
		{
			name: "no prefixes: returns rules unchanged",
			rules: []gwapiv1.HTTPRouteRule{
				rule("completions", prefixMatch("/ns/name/v1/completions")),
			},
			prefixes: nil,
			want: []gwapiv1.HTTPRouteRule{
				rule("completions", prefixMatch("/ns/name/v1/completions")),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripPerModelPathMatches(tt.rules, tt.prefixes, modelRoutingTestHeader)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPerModelPathPrefixes(t *testing.T) {
	tests := []struct {
		name      string
		modelName *string
		want      []string
	}{
		{
			name:      "uses spec.model.name for the publisher prefix",
			modelName: ptr.To("org/model"),
			want:      []string{"/ns/svc", "/publishers/ns/models/org/model"},
		},
		{
			name: "falls back to metadata.name when spec.model.name is unset",
			want: []string{"/ns/svc", "/publishers/ns/models/svc"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
				Spec:       v1alpha2.LLMInferenceServiceSpec{Model: v1alpha2.LLMModelSpec{Name: tt.modelName}},
			}
			assert.Equal(t, tt.want, perModelPathPrefixes(llmSvc))
		})
	}
}

func TestResolveModelBasedRoutingOnly(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	require.NoError(t, gwapiv1.Install(scheme))

	// The store holds no GatewayClass for the Gateways' class, so every case
	// also shows the decision never depends on reading one.
	gateway := func(namespace, name, setting string) *gwapiv1.Gateway {
		gw := &gwapiv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       gwapiv1.GatewaySpec{GatewayClassName: "missing-class"},
		}
		if setting != "" {
			gw.Annotations = map[string]string{AnnotationModelBasedRoutingOnly: setting}
		}
		return gw
	}
	parent := func(namespace, name string) gwapiv1.ParentReference {
		ref := gwapiv1.ParentReference{Name: gwapiv1.ObjectName(name)}
		if namespace != "" {
			ref.Namespace = ptr.To(gwapiv1.Namespace(namespace))
		}
		return ref
	}

	tests := []struct {
		name           string
		serviceSetting string
		gateways       []*gwapiv1.Gateway
		parents        []gwapiv1.ParentReference
		want           perModelPathsDecision
		wantMessage    string
		wantErr        bool
	}{
		{
			name: "nothing configured, no parents",
			want: perModelPathsDecision{},
		},
		{
			name:     "nothing configured on the service or the Gateway",
			gateways: []*gwapiv1.Gateway{gateway("gw-ns", "gw-a", "")},
			parents:  []gwapiv1.ParentReference{parent("gw-ns", "gw-a")},
			want:     perModelPathsDecision{},
		},
		{
			name:           "service true opts in alone, without reading Gateways",
			serviceSetting: "true",
			parents:        []gwapiv1.ParentReference{parent("gw-ns", "gw-missing")},
			want:           perModelPathsDecision{configured: true, drop: true, reason: reasonSetOnService},
		},
		{
			name:           "service value is parsed leniently",
			serviceSetting: "True",
			want:           perModelPathsDecision{configured: true, drop: true, reason: reasonSetOnService},
		},
		{
			name:     "single Gateway set drops the paths",
			gateways: []*gwapiv1.Gateway{gateway("gw-ns", "gw-a", "true")},
			parents:  []gwapiv1.ParentReference{parent("gw-ns", "gw-a")},
			want:     perModelPathsDecision{configured: true, drop: true, reason: reasonSetOnAllGateways},
		},
		{
			name:     "Gateway value is parsed leniently",
			gateways: []*gwapiv1.Gateway{gateway("gw-ns", "gw-a", "1")},
			parents:  []gwapiv1.ParentReference{parent("gw-ns", "gw-a")},
			want:     perModelPathsDecision{configured: true, drop: true, reason: reasonSetOnAllGateways},
		},
		{
			name:     "parent without a namespace resolves in the route's namespace",
			gateways: []*gwapiv1.Gateway{gateway("ns", "gw-local", "true")},
			parents:  []gwapiv1.ParentReference{parent("", "gw-local")},
			want:     perModelPathsDecision{configured: true, drop: true, reason: reasonSetOnAllGateways},
		},
		{
			name:     "every Gateway set drops the paths",
			gateways: []*gwapiv1.Gateway{gateway("gw-ns", "gw-a", "true"), gateway("gw-ns", "gw-b", "true")},
			parents:  []gwapiv1.ParentReference{parent("gw-ns", "gw-a"), parent("gw-ns", "gw-b")},
			want:     perModelPathsDecision{configured: true, drop: true, reason: reasonSetOnAllGateways},
		},
		{
			name:        "partial Gateway setting keeps the paths and names the rest, sorted and deduplicated",
			gateways:    []*gwapiv1.Gateway{gateway("gw-ns", "gw-a", "true"), gateway("gw-ns", "gw-c", ""), gateway("gw-ns", "gw-b", "no")},
			parents:     []gwapiv1.ParentReference{parent("gw-ns", "gw-c"), parent("gw-ns", "gw-a"), parent("gw-ns", "gw-b"), parent("gw-ns", "gw-c")},
			want:        perModelPathsDecision{configured: true, reason: reasonNotSetOnAllGateways},
			wantMessage: "parent Gateways gw-ns/gw-b, gw-ns/gw-c;",
		},
		{
			name:     "non-Gateway parent counts as unset",
			gateways: []*gwapiv1.Gateway{gateway("gw-ns", "gw-a", "true")},
			parents: []gwapiv1.ParentReference{
				parent("gw-ns", "gw-a"),
				{Group: ptr.To(gwapiv1.Group("")), Kind: ptr.To(gwapiv1.Kind("Service")), Name: "mesh-svc"},
			},
			want: perModelPathsDecision{configured: true, reason: reasonNotSetOnAllGateways},
		},
		{
			name:           "service false over a Gateway that sets it reports the override",
			serviceSetting: "false",
			gateways:       []*gwapiv1.Gateway{gateway("gw-ns", "gw-a", "true")},
			parents:        []gwapiv1.ParentReference{parent("gw-ns", "gw-a")},
			want:           perModelPathsDecision{configured: true, reason: reasonDisabledOnService},
			wantMessage:    "overriding parent Gateways gw-ns/gw-a;",
		},
		{
			name:           "service false with no Gateway asking is not reported",
			serviceSetting: "false",
			gateways:       []*gwapiv1.Gateway{gateway("gw-ns", "gw-a", "")},
			parents:        []gwapiv1.ParentReference{parent("gw-ns", "gw-a")},
			want:           perModelPathsDecision{},
		},
		{
			name:           "service false ignores unreadable Gateways",
			serviceSetting: "false",
			parents:        []gwapiv1.ParentReference{parent("gw-ns", "gw-missing")},
			want:           perModelPathsDecision{},
		},
		{
			name:           "unrecognized service value defers to Gateways that all set it",
			serviceSetting: "yes",
			gateways:       []*gwapiv1.Gateway{gateway("gw-ns", "gw-a", "true")},
			parents:        []gwapiv1.ParentReference{parent("gw-ns", "gw-a")},
			want:           perModelPathsDecision{configured: true, drop: true, reason: reasonSetOnAllGateways},
		},
		{
			name:           "unrecognized service value is reported when the paths are kept",
			serviceSetting: "yes",
			gateways:       []*gwapiv1.Gateway{gateway("gw-ns", "gw-a", "true"), gateway("gw-ns", "gw-b", "")},
			parents:        []gwapiv1.ParentReference{parent("gw-ns", "gw-a"), parent("gw-ns", "gw-b")},
			want:           perModelPathsDecision{configured: true, reason: reasonUnrecognizedValue},
			wantMessage:    `unrecognized value "yes"`,
		},
		{
			name:     "unreadable Gateway with no unset parent is returned, not defaulted",
			gateways: []*gwapiv1.Gateway{gateway("gw-ns", "gw-a", "true")},
			parents:  []gwapiv1.ParentReference{parent("gw-ns", "gw-a"), parent("gw-ns", "gw-missing")},
			wantErr:  true,
		},
		{
			name:     "an unset Gateway decides whatever the parent order",
			gateways: []*gwapiv1.Gateway{gateway("gw-ns", "gw-a", "true"), gateway("gw-ns", "gw-b", "")},
			parents:  []gwapiv1.ParentReference{parent("gw-ns", "gw-missing"), parent("gw-ns", "gw-a"), parent("gw-ns", "gw-b")},
			want:     perModelPathsDecision{configured: true, reason: reasonNotSetOnAllGateways},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := make([]client.Object, 0, len(tt.gateways))
			for _, gw := range tt.gateways {
				objs = append(objs, gw)
			}
			r := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()}

			route := &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "svc-kserve-route", Namespace: "ns"},
				Spec: gwapiv1.HTTPRouteSpec{
					CommonRouteSpec: gwapiv1.CommonRouteSpec{ParentRefs: tt.parents},
				},
			}
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
			}
			if tt.serviceSetting != "" {
				llmSvc.Spec.Annotations = map[string]string{AnnotationModelBasedRoutingOnly: tt.serviceSetting}
			}

			got, err := r.resolveModelBasedRoutingOnly(t.Context(), llmSvc, route)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, got.message, tt.wantMessage)
			got.message = ""
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestApplyModelBasedRoutingOnly_KeepsPathsWithoutModelRoutingTree(t *testing.T) {
	llmSvc := func(rules ...gwapiv1.HTTPRouteRule) *v1alpha2.LLMInferenceService {
		return &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{Annotations: map[string]string{AnnotationModelBasedRoutingOnly: "true"}},
				Router: &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{HTTP: &v1alpha2.HTTPRouteSpec{
					Spec: &gwapiv1.HTTPRouteSpec{Rules: rules},
				}}},
			},
		}
	}
	pathRule := gwapiv1.HTTPRouteRule{Matches: []gwapiv1.HTTPRouteMatch{{
		Path: &gwapiv1.HTTPPathMatch{Type: ptr.To(gwapiv1.PathMatchPathPrefix), Value: ptr.To("/ns/svc")},
	}}}
	modelRule := gwapiv1.HTTPRouteRule{Matches: []gwapiv1.HTTPRouteMatch{{
		Headers: []gwapiv1.HTTPHeaderMatch{{Name: modelRoutingTestHeader, Value: "publishers/ns/models/svc"}},
	}}}
	cfg := &Config{ModelBasedRoutingHeaderName: modelRoutingTestHeader}

	tests := []struct {
		name       string
		configured []gwapiv1.HTTPRouteRule
		cfg        *Config
		wantReason string
	}{
		{
			name:       "model-routing matches stripped by the model-routing transform",
			configured: []gwapiv1.HTTPRouteRule{pathRule, modelRule},
			cfg:        cfg,
			wantReason: reasonModelBasedRoutingNotEnabled,
		},
		{
			name:       "no model-routing header configured",
			configured: []gwapiv1.HTTPRouteRule{pathRule, modelRule},
			cfg:        &Config{},
			wantReason: reasonModelBasedRoutingNotEnabled,
		},
		{
			name:       "route spec never had model-routing matches",
			configured: []gwapiv1.HTTPRouteRule{pathRule},
			cfg:        cfg,
			wantReason: reasonNoModelRoutingMatches,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The rendered route as the model-routing transform leaves it: only
			// the path rule.
			route := &gwapiv1.HTTPRoute{Spec: gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{*pathRule.DeepCopy()}}}

			got, err := (&LLMISVCReconciler{}).applyModelBasedRoutingOnly(t.Context(), llmSvc(tt.configured...), tt.cfg, route)
			require.NoError(t, err)
			assert.True(t, got.configured)
			assert.False(t, got.drop)
			assert.Equal(t, tt.wantReason, got.reason)
			assert.Equal(t, []gwapiv1.HTTPRouteRule{pathRule}, route.Spec.Rules, "the paths must stay")
		})
	}
}

// A GatewayClass that cannot be read - deleted, or not yet in the cache - fails
// status discovery, but only after the route is reconciled. The route decision
// must not depend on it, or the stored route regains the per-model paths every
// parent Gateway disabled.
func TestReconcileRouter_StrippedRouteSurvivesGatewayClassLookupFailure(t *testing.T) {
	seedInferencePoolV1Alpha2Discovery(t)

	parents := []gwapiv1.ParentReference{{Name: "gw", Namespace: ptr.To(gwapiv1.Namespace("gw-ns"))}}
	backendRefs := []gwapiv1.HTTPBackendRef{{BackendRef: gwapiv1.BackendRef{
		BackendObjectReference: gwapiv1.BackendObjectReference{Name: "test-llm-kserve-workload-svc", Port: ptr.To(gwapiv1.PortNumber(8000))},
	}}}
	pathRule := gwapiv1.HTTPRouteRule{
		Name: ptr.To(gwapiv1.SectionName("v1-catch-all-path")),
		Matches: []gwapiv1.HTTPRouteMatch{{
			Path: &gwapiv1.HTTPPathMatch{Type: ptr.To(gwapiv1.PathMatchPathPrefix), Value: ptr.To("/test-ns/test-llm")},
		}},
		BackendRefs: backendRefs,
	}
	modelRoutingRule := gwapiv1.HTTPRouteRule{
		Name: ptr.To(gwapiv1.SectionName("v1-catch-all-model-routing")),
		Matches: []gwapiv1.HTTPRouteMatch{{
			Headers: []gwapiv1.HTTPHeaderMatch{{
				Type:  ptr.To(gwapiv1.HeaderMatchExact),
				Name:  modelRoutingTestHeader,
				Value: "publishers/test-ns/models/test-model",
			}},
		}},
		BackendRefs: backendRefs,
	}

	llmSvc := invalidRouteService()
	llmSvc.Spec.Annotations = map[string]string{AnnotationModelBasedRoutingEnabled: "true"}
	llmSvc.Spec.Router.Route.HTTP.Spec = &gwapiv1.HTTPRouteSpec{
		CommonRouteSpec: gwapiv1.CommonRouteSpec{ParentRefs: parents},
		Rules:           []gwapiv1.HTTPRouteRule{pathRule, modelRoutingRule},
	}

	// given - the route as the last successful pass stored it, with the paths
	// already stripped, and a model-routing-only Gateway whose class is gone
	stored := managedRoute(llmSvc)
	stored.Spec = gwapiv1.HTTPRouteSpec{
		CommonRouteSpec: gwapiv1.CommonRouteSpec{ParentRefs: parents},
		Rules:           []gwapiv1.HTTPRouteRule{*modelRoutingRule.DeepCopy()},
	}
	gateway := &gwapiv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "gw",
			Namespace:   "gw-ns",
			Annotations: map[string]string{AnnotationModelBasedRoutingOnly: "true"},
		},
		Spec: gwapiv1.GatewaySpec{GatewayClassName: "deleted-class"},
	}

	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{httpRouteGV})
	restMapper.Add(httpRouteGV.WithKind("HTTPRoute"), meta.RESTScopeNamespace)
	writes := 0
	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(invalidRouteScheme(t)).
			WithRESTMapper(restMapper).
			WithObjects(llmSvc, stored, gateway, testInferenceServiceConfigMap()).
			WithInterceptorFuncs(interceptor.Funcs{
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					options := &client.UpdateOptions{}
					for _, opt := range opts {
						opt.ApplyToUpdate(options)
					}
					if _, isRoute := obj.(*gwapiv1.HTTPRoute); isRoute && !slices.Contains(options.DryRun, metav1.DryRunAll) {
						writes++
					}
					return c.Update(ctx, obj, opts...)
				},
			}).
			Build(),
		EventRecorder: record.NewFakeRecorder(10),
	}

	// when
	err := reconciler.reconcileRouter(t.Context(), llmSvc, &Config{ModelBasedRoutingHeaderName: modelRoutingTestHeader})

	// then
	require.ErrorContains(t, err, `GatewayClass "deleted-class"`, "status discovery still needs the class")
	assert.Zero(t, writes, "the stored route must not regain the paths its Gateway disabled")
	current := &gwapiv1.HTTPRoute{}
	require.NoError(t, reconciler.Get(t.Context(), client.ObjectKeyFromObject(stored), current))
	assert.Equal(t, stored.Spec.Rules, current.Spec.Rules)
}
