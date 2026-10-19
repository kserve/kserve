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
	"fmt"
	"regexp"
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
		{name: "oversized rule is split into chunks", rules: []gwapiv1.HTTPRouteRule{pathRule("lora", 150)}, wantShards: 2, wantRules: []int{2, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shards := shardHTTPRouteRules(tt.rules)
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
	out := splitOversizedRules([]gwapiv1.HTTPRouteRule{pathRule("first", 1), rule, pathRule("last", 1)})

	if len(out) != 5 {
		t.Fatalf("got %d rules, want 5", len(out))
	}
	wantNames := []string{"first", "lora", "lora-1", "lora-2", "last"}
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

	shards := shardHTTPRoute(llmSvc, route)
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
	again := shardHTTPRoute(llmSvc, route)
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

var sectionNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

func TestSplitOversizedRulesNamesAreValidAndUnique(t *testing.T) {
	long := strings.Repeat("a", 252) + "b"
	dotted := strings.Repeat("x", 247) + ".y-z"
	cases := map[string][]gwapiv1.HTTPRouteRule{
		"plain":                     {pathRule("lora", 130)},
		"collides with user rule":   {pathRule("lora", 130), pathRule("lora-1", 1), pathRule("lora-2", 1)},
		"user rule declared before": {pathRule("lora-1", 1), pathRule("lora", 70)},
		"max length name":           {pathRule(long, 200)},
		"cut lands on separator":    {pathRule(dotted, 70)},
	}
	for name, rules := range cases {
		t.Run(name, func(t *testing.T) {
			out := splitOversizedRules(rules)
			seen := map[gwapiv1.SectionName]bool{}
			for _, rule := range out {
				if rule.Name == nil {
					continue
				}
				n := *rule.Name
				if len(n) > 253 || !sectionNamePattern.MatchString(string(n)) {
					t.Fatalf("invalid rule name %q (len %d)", n, len(n))
				}
				if seen[n] {
					t.Fatalf("duplicate rule name %q", n)
				}
				seen[n] = true
			}
			again := splitOversizedRules(rules)
			for i := range out {
				if ptr.Deref(out[i].Name, "") != ptr.Deref(again[i].Name, "") {
					t.Fatalf("rule %d name is not stable: %q vs %q", i, ptr.Deref(out[i].Name, ""), ptr.Deref(again[i].Name, ""))
				}
			}
		})
	}
}

func TestChunkRuleNameKeepsPlainFormWhenPossible(t *testing.T) {
	if got := chunkRuleName("lora", 1, map[gwapiv1.SectionName]struct{}{"lora": {}}); got != "lora-1" {
		t.Fatalf("got %q, want lora-1", got)
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
