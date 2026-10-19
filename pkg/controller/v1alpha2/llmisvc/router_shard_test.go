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
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func pathRule(name string, matches int) gwapiv1.HTTPRouteRule {
	rule := gwapiv1.HTTPRouteRule{
		BackendRefs: []gwapiv1.HTTPBackendRef{{BackendRef: gwapiv1.BackendRef{
			BackendObjectReference: gwapiv1.BackendObjectReference{Name: "backend"},
		}}},
	}
	if name != "" {
		rule.Name = ptr.To(gwapiv1.SectionName(name))
	}
	for i := range matches {
		rule.Matches = append(rule.Matches, gwapiv1.HTTPRouteMatch{
			Path: &gwapiv1.HTTPPathMatch{Value: ptr.To(fmt.Sprintf("/%s/%d", name, i))},
		})
	}
	return rule
}

func assertWithinGatewayLimits(t *testing.T, rules []gwapiv1.HTTPRouteRule) {
	t.Helper()
	if len(rules) > httpRouteMaxRules {
		t.Fatalf("shard has %d rules, limit is %d", len(rules), httpRouteMaxRules)
	}
	total := 0
	names := map[gwapiv1.SectionName]bool{}
	for i := range rules {
		if len(rules[i].Matches) > httpRouteMaxRuleMatches {
			t.Fatalf("rule %d has %d matches, limit is %d", i, len(rules[i].Matches), httpRouteMaxRuleMatches)
		}
		if rules[i].Name != nil {
			if names[*rules[i].Name] {
				t.Fatalf("duplicate rule name %q in shard", *rules[i].Name)
			}
			names[*rules[i].Name] = true
		}
		total += ruleMatchCount(&rules[i])
	}
	if total > httpRouteMaxTotalMatches {
		t.Fatalf("shard has %d matches, limit is %d", total, httpRouteMaxTotalMatches)
	}
}

func flatten(shards [][]gwapiv1.HTTPRouteRule) []gwapiv1.HTTPBackendRef {
	var out []gwapiv1.HTTPBackendRef
	for _, s := range shards {
		for _, r := range s {
			out = append(out, r.BackendRefs...)
		}
	}
	return out
}

func TestShardHTTPRouteRules(t *testing.T) {
	tests := []struct {
		name       string
		rules      []gwapiv1.HTTPRouteRule
		wantShards int
		wantRules  []int
	}{
		{name: "no rules yields one empty shard", rules: nil, wantShards: 1, wantRules: []int{0}},
		{name: "fits in one route", rules: repeatRules(16, 1), wantShards: 1, wantRules: []int{16}},
		{name: "rule cap splits", rules: repeatRules(17, 1), wantShards: 2, wantRules: []int{16, 1}},
		{name: "rule cap splits many", rules: repeatRules(64, 0), wantShards: 4, wantRules: []int{16, 16, 16, 16}},
		{name: "total match cap splits", rules: repeatRules(3, 60), wantShards: 2, wantRules: []int{2, 1}},
		{name: "chunks of a named rule go to separate shards", rules: []gwapiv1.HTTPRouteRule{pathRule("lora", 150)}, wantShards: 3, wantRules: []int{1, 1, 1}},
		{name: "chunks of an unnamed rule share a shard", rules: []gwapiv1.HTTPRouteRule{pathRule("", 150)}, wantShards: 2, wantRules: []int{2, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shards, err := shardHTTPRouteRules(tt.rules)
			if err != nil {
				t.Fatal(err)
			}
			if len(shards) != tt.wantShards {
				t.Fatalf("got %d shards, want %d", len(shards), tt.wantShards)
			}
			for i, s := range shards {
				assertWithinGatewayLimits(t, s)
				if len(s) != tt.wantRules[i] {
					t.Errorf("shard %d has %d rules, want %d", i, len(s), tt.wantRules[i])
				}
			}
		})
	}
}

func repeatRules(n, matches int) []gwapiv1.HTTPRouteRule {
	rules := make([]gwapiv1.HTTPRouteRule, 0, n)
	for i := range n {
		rules = append(rules, pathRule(fmt.Sprintf("r%d", i), matches))
	}
	return rules
}

func TestSplitOversizedRulesKeepsMatchesAndOrder(t *testing.T) {
	rule := pathRule("lora", 130)
	rule.Filters = []gwapiv1.HTTPRouteFilter{{Type: gwapiv1.HTTPRouteFilterURLRewrite}}
	out, origins := splitOversizedRules([]gwapiv1.HTTPRouteRule{pathRule("first", 1), rule, pathRule("last", 1)})
	if !slices.Equal(origins, []int{0, 1, 1, 1, 2}) {
		t.Fatalf("origins %v", origins)
	}

	if len(out) != 5 {
		t.Fatalf("got %d rules, want 5", len(out))
	}
	wantNames := []string{"first", "lora", "lora", "lora", "last"}
	wantMatches := []int{1, 64, 64, 2, 1}
	next := 0
	for i := range out {
		if string(*out[i].Name) != wantNames[i] {
			t.Errorf("rule %d name %q, want %q", i, *out[i].Name, wantNames[i])
		}
		if len(out[i].Matches) != wantMatches[i] {
			t.Errorf("rule %d has %d matches, want %d", i, len(out[i].Matches), wantMatches[i])
		}
		if i >= 1 && i <= 3 {
			if len(out[i].Filters) != 1 || len(out[i].BackendRefs) != 1 {
				t.Errorf("chunk %d lost filters or backends", i)
			}
			for _, m := range out[i].Matches {
				if want := fmt.Sprintf("/lora/%d", next); *m.Path.Value != want {
					t.Fatalf("chunk %d match %q, want %q", i, *m.Path.Value, want)
				}
				next++
			}
		}
	}
	// The source rule must not be mutated.
	if len(rule.Matches) != 130 {
		t.Errorf("source rule mutated: %d matches", len(rule.Matches))
	}
}

func TestShardHTTPRoute(t *testing.T) {
	llmSvc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"}}
	route := &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        managedHTTPRouteName("svc", 0),
			Namespace:   "ns",
			Labels:      map[string]string{constants.KubernetesAppNameLabelKey: "svc"},
			Annotations: map[string]string{AnnotationInferencePoolMigrated: "v1"},
		},
		Spec: gwapiv1.HTTPRouteSpec{
			CommonRouteSpec: gwapiv1.CommonRouteSpec{ParentRefs: []gwapiv1.ParentReference{{Name: "gw"}}},
			Hostnames:       []gwapiv1.Hostname{"example.com"},
			Rules:           repeatRules(40, 1),
		},
	}

	shards, err := shardHTTPRoute(llmSvc, route)
	if err != nil {
		t.Fatal(err)
	}
	if len(shards) != 3 {
		t.Fatalf("got %d shards, want 3", len(shards))
	}
	wantNames := []string{"svc-kserve-route", "svc-kserve-route-1", "svc-kserve-route-2"}
	for i, s := range shards {
		if s.Name != wantNames[i] {
			t.Errorf("shard %d name %q, want %q", i, s.Name, wantNames[i])
		}
		if len(s.Spec.ParentRefs) != 1 || len(s.Spec.Hostnames) != 1 {
			t.Errorf("shard %d did not inherit parentRefs or hostnames", i)
		}
		if s.Annotations[AnnotationInferencePoolMigrated] != "v1" {
			t.Errorf("shard %d lost annotations", i)
		}
		if s.Labels[constants.KubernetesAppNameLabelKey] != "svc" {
			t.Errorf("shard %d lost labels", i)
		}
		shardLabel, ok := s.Labels[constants.LLMHTTPRouteShardLabelKey]
		if i == 0 && ok {
			t.Errorf("first shard must not carry the shard label")
		}
		if i > 0 && shardLabel != strconv.Itoa(i) {
			t.Errorf("shard %d label %q", i, shardLabel)
		}
		assertWithinGatewayLimits(t, s.Spec.Rules)
	}
	if _, ok := route.Labels[constants.LLMHTTPRouteShardLabelKey]; ok {
		t.Errorf("input route labels mutated")
	}
	if got := len(flatten([][]gwapiv1.HTTPRouteRule{shards[0].Spec.Rules, shards[1].Spec.Rules, shards[2].Spec.Rules})); got != 40 {
		t.Errorf("rules lost while sharding: %d backends, want 40", got)
	}

	// Sharding is deterministic.
	again, err := shardHTTPRoute(llmSvc, route)
	if err != nil {
		t.Fatal(err)
	}
	for i := range shards {
		if again[i].Name != shards[i].Name || len(again[i].Spec.Rules) != len(shards[i].Spec.Rules) {
			t.Fatalf("sharding is not deterministic at shard %d", i)
		}
	}
}

func TestManagedHTTPRouteNameStaysWithinDNSLimit(t *testing.T) {
	long := "a-very-long-llm-inference-service-name-that-is-close-to-the-limit"
	for i := range 4 {
		if n := managedHTTPRouteName(long, i); len(n) > 63 {
			t.Errorf("name %q exceeds 63 characters", n)
		}
	}
	if managedHTTPRouteName(long, 1) == managedHTTPRouteName(long, 2) {
		t.Errorf("shard names collide")
	}
}

func TestHTTPRouteMaxShardsFor(t *testing.T) {
	svc := func(ann map[string]string) *v1alpha2.LLMInferenceService {
		s := &v1alpha2.LLMInferenceService{}
		s.Spec.Annotations = ann
		return s
	}
	for _, tt := range []struct {
		name string
		svc  *v1alpha2.LLMInferenceService
		cfg  *Config
		want int
		src  string
	}{
		{name: "default when unset", svc: svc(nil), cfg: &Config{}, want: 1, src: "ConfigMap"},
		{name: "nil config", svc: svc(nil), cfg: nil, want: 1, src: "ConfigMap"},
		{name: "config value", svc: svc(nil), cfg: &Config{HTTPRouteMaxShards: 4}, want: 4, src: "ConfigMap"},
		{name: "annotation wins", svc: svc(map[string]string{constants.HTTPRouteMaxShardsAnnotationKey: " 3 "}), cfg: &Config{HTTPRouteMaxShards: 4}, want: 3, src: "annotation"},
		{name: "bad annotation falls back", svc: svc(map[string]string{constants.HTTPRouteMaxShardsAnnotationKey: "x"}), cfg: &Config{HTTPRouteMaxShards: 2}, want: 2, src: "ConfigMap"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, src := httpRouteMaxShardsFor(tt.svc, tt.cfg)
			if got != tt.want || !strings.Contains(src, tt.src) {
				t.Fatalf("got %d from %q, want %d from %q", got, src, tt.want, tt.src)
			}
		})
	}
}

func backendRule(path string, pathType gwapiv1.PathMatchType, backend string) gwapiv1.HTTPRouteRule {
	return gwapiv1.HTTPRouteRule{
		Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: ptr.To(pathType), Value: ptr.To(path)}}},
		BackendRefs: []gwapiv1.HTTPBackendRef{{BackendRef: gwapiv1.BackendRef{
			BackendObjectReference: gwapiv1.BackendObjectReference{Name: gwapiv1.ObjectName(backend)},
		}}},
	}
}

// shardOf returns the index of the shard holding the rule whose first match has path.
func shardOf(t *testing.T, shards [][]gwapiv1.HTTPRouteRule, path string, backend string) int {
	t.Helper()
	for s := range shards {
		for _, r := range shards[s] {
			if *r.Matches[0].Path.Value == path && string(r.BackendRefs[0].Name) == backend {
				return s
			}
		}
	}
	t.Fatalf("rule %s -> %s not found", path, backend)
	return -1
}

func TestShardHTTPRouteRulesPrecedenceGroups(t *testing.T) {
	filler := func(n int) []gwapiv1.HTTPRouteRule {
		rules := make([]gwapiv1.HTTPRouteRule, 0, n)
		for i := range n {
			rules = append(rules, backendRule(fmt.Sprintf("/fill-%02d", i), gwapiv1.PathMatchPathPrefix, "fill"))
		}
		return rules
	}

	t.Run("overlapping equal precedence rules with different actions stay together in order", func(t *testing.T) {
		rules := append(filler(15),
			backendRule("/v1/chat", gwapiv1.PathMatchPathPrefix, "first"),
			backendRule("/v1/chat", gwapiv1.PathMatchPathPrefix, "second"))
		shards, err := shardHTTPRouteRules(rules)
		if err != nil {
			t.Fatal(err)
		}
		a, b := shardOf(t, shards, "/v1/chat", "first"), shardOf(t, shards, "/v1/chat", "second")
		if a != b {
			t.Fatalf("order-dependent rules split over shards %d and %d", a, b)
		}
		order := make([]string, 0, len(shards[a]))
		for _, r := range shards[a] {
			order = append(order, string(r.BackendRefs[0].Name))
		}
		if !slices.Equal(order[len(order)-2:], []string{"first", "second"}) {
			t.Fatalf("list order not kept within the shard: %v", order)
		}
	})

	t.Run("disjoint paths of equal length split freely", func(t *testing.T) {
		rules := append(filler(15),
			backendRule("/v1/aaaa", gwapiv1.PathMatchPathPrefix, "a"),
			backendRule("/v1/bbbb", gwapiv1.PathMatchPathPrefix, "b"))
		shards, err := shardHTTPRouteRules(rules)
		if err != nil {
			t.Fatal(err)
		}
		if len(shards) != 2 || shardOf(t, shards, "/v1/bbbb", "b") != 1 {
			t.Fatalf("expected plain packing, got %d shards", len(shards))
		}
	})

	t.Run("different precedence keys are ordered by specificity and split freely", func(t *testing.T) {
		rules := append(filler(15),
			backendRule("/v1", gwapiv1.PathMatchPathPrefix, "short"),
			backendRule("/v1/completions", gwapiv1.PathMatchPathPrefix, "long"))
		shards, err := shardHTTPRouteRules(rules)
		if err != nil {
			t.Fatal(err)
		}
		if shardOf(t, shards, "/v1", "short") == shardOf(t, shards, "/v1/completions", "long") {
			t.Fatalf("rules with different precedence keys should pack in order, not group")
		}
	})

	t.Run("regular expressions are treated as overlapping", func(t *testing.T) {
		rules := append(filler(15),
			backendRule("/v1/a.*", gwapiv1.PathMatchRegularExpression, "x"),
			backendRule("/v1/b.*", gwapiv1.PathMatchRegularExpression, "y"))
		shards, err := shardHTTPRouteRules(rules)
		if err != nil {
			t.Fatal(err)
		}
		if shardOf(t, shards, "/v1/a.*", "x") != shardOf(t, shards, "/v1/b.*", "y") {
			t.Fatalf("regex rules of the same key must stay together")
		}
	})

	t.Run("exact headers with different values are disjoint", func(t *testing.T) {
		withHeader := func(backend, model string) gwapiv1.HTTPRouteRule {
			r := backendRule("/v1/chat", gwapiv1.PathMatchPathPrefix, backend)
			r.Matches[0].Headers = []gwapiv1.HTTPHeaderMatch{{Name: "X-Gateway-Model-Name", Value: model}}
			return r
		}
		rules := append(filler(15), withHeader("m1", "model-1"), withHeader("m2", "model-2"))
		shards, err := shardHTTPRouteRules(rules)
		if err != nil {
			t.Fatal(err)
		}
		if len(shards) != 2 {
			t.Fatalf("got %d shards, want 2", len(shards))
		}
	})

	t.Run("same action rules never group, so lora chunks split", func(t *testing.T) {
		shards, err := shardHTTPRouteRules([]gwapiv1.HTTPRouteRule{pathRule("lora", 300)})
		if err != nil {
			t.Fatal(err)
		}
		if len(shards) != 5 {
			t.Fatalf("got %d shards, want 5", len(shards))
		}
		for i, s := range shards {
			if len(s) != 1 || ptr.Deref(s[0].Name, "") != "lora" {
				t.Fatalf("shard %d should hold one chunk that keeps the rule name", i)
			}
		}
	})

	t.Run("an order-dependent group larger than one route is rejected", func(t *testing.T) {
		rules := make([]gwapiv1.HTTPRouteRule, 0, 17)
		for i := range 17 {
			rules = append(rules, backendRule("/v1/chat", gwapiv1.PathMatchPathPrefix, fmt.Sprintf("b%d", i)))
		}
		_, err := shardHTTPRouteRules(rules)
		if !errors.Is(err, ErrPreconditionNotMet) {
			t.Fatalf("want ErrPreconditionNotMet, got %v", err)
		}
		if !strings.Contains(err.Error(), "17 rules") {
			t.Fatalf("message should give the group size: %v", err)
		}
	})
}
