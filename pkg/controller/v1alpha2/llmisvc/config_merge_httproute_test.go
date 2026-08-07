/*
Copyright 2025 The KServe Authors.

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

package llmisvc_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

func TestMergeSpecs_HTTPRouteHostnamesPreserveRules(t *testing.T) {
	ctx := t.Context()
	base := v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Route: &v1alpha2.GatewayRoutesSpec{
				HTTP: &v1alpha2.HTTPRouteSpec{
					Spec: &gwapiv1.HTTPRouteSpec{
						CommonRouteSpec: gwapiv1.CommonRouteSpec{
							ParentRefs: []gwapiv1.ParentReference{{Name: "kserve-ingress-gateway"}},
						},
						Rules: []gwapiv1.HTTPRouteRule{{
							Name: ptr.To(gwapiv1.SectionName("v1-completions-path")),
							BackendRefs: []gwapiv1.HTTPBackendRef{{
								BackendRef: gwapiv1.BackendRef{
									BackendObjectReference: gwapiv1.BackendObjectReference{
										Name: "pool",
										Kind: ptr.To(gwapiv1.Kind("InferencePool")),
									},
								},
							}},
						}},
					},
				},
			},
		},
	}
	override := v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Route: &v1alpha2.GatewayRoutesSpec{
				HTTP: &v1alpha2.HTTPRouteSpec{
					Spec: &gwapiv1.HTTPRouteSpec{
						Hostnames: []gwapiv1.Hostname{"my-svc.example.com"},
					},
				},
			},
		},
	}

	merged, err := llmisvc.MergeSpecs(ctx, base, override)
	if err != nil {
		t.Fatalf("MergeSpecs() error = %v", err)
	}
	got := merged.Router.Route.HTTP.Spec
	if len(got.Hostnames) != 1 || got.Hostnames[0] != "my-svc.example.com" {
		t.Fatalf("hostnames not applied: %#v", got.Hostnames)
	}
	if len(got.ParentRefs) != 1 {
		t.Fatalf("parentRefs wiped: %#v", got.ParentRefs)
	}
	if len(got.Rules) != 1 || len(got.Rules[0].BackendRefs) != 1 {
		t.Fatalf("preset rules wiped by hostnames-only override: %#v", got.Rules)
	}
}

func TestMergeSpecs_HTTPRouteRuleDefaultsOverlay(t *testing.T) {
	ctx := t.Context()
	base := v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Route: &v1alpha2.GatewayRoutesSpec{
				HTTP: &v1alpha2.HTTPRouteSpec{
					Spec: &gwapiv1.HTTPRouteSpec{
						CommonRouteSpec: gwapiv1.CommonRouteSpec{
							ParentRefs: []gwapiv1.ParentReference{{Name: "kserve-ingress-gateway"}},
						},
						Rules: []gwapiv1.HTTPRouteRule{
							{
								Name: ptr.To(gwapiv1.SectionName("v1-completions-path")),
								BackendRefs: []gwapiv1.HTTPBackendRef{{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Name: "pool",
											Kind: ptr.To(gwapiv1.Kind("InferencePool")),
										},
									},
								}},
							},
							{
								Name: ptr.To(gwapiv1.SectionName("catch-all")),
								BackendRefs: []gwapiv1.HTTPBackendRef{{
									BackendRef: gwapiv1.BackendRef{
										BackendObjectReference: gwapiv1.BackendObjectReference{
											Name: "svc",
											Kind: ptr.To(gwapiv1.Kind("Service")),
										},
									},
								}},
							},
						},
					},
				},
			},
		},
	}
	override := v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Route: &v1alpha2.GatewayRoutesSpec{
				HTTP: &v1alpha2.HTTPRouteSpec{
					RuleDefaults: &v1alpha2.HTTPRouteRuleDefaults{
						Timeouts: &gwapiv1.HTTPRouteTimeouts{
							Request:        ptr.To(gwapiv1.Duration("300s")),
							BackendRequest: ptr.To(gwapiv1.Duration("300s")),
						},
						Retry: &gwapiv1.HTTPRouteRetry{
							Attempts: ptr.To(3),
						},
					},
				},
			},
		},
	}

	merged, err := llmisvc.MergeSpecs(ctx, base, override)
	if err != nil {
		t.Fatalf("MergeSpecs() error = %v", err)
	}
	llmisvc.ApplyHTTPRouteRuleDefaults(&base, &override, &merged)
	got := merged.Router.Route.HTTP.Spec
	if len(got.Rules) != 2 {
		t.Fatalf("expected preset rules preserved, got %d rules: %#v", len(got.Rules), got.Rules)
	}
	for i, rule := range got.Rules {
		if len(rule.BackendRefs) != 1 {
			t.Fatalf("rule %d lost backendRefs: %#v", i, rule)
		}
		if rule.Timeouts == nil || ptr.Deref(rule.Timeouts.Request, "") != "300s" {
			t.Fatalf("rule %d missing ruleDefaults timeouts: %#v", i, rule.Timeouts)
		}
		if rule.Retry == nil || ptr.Deref(rule.Retry.Attempts, 0) != 3 {
			t.Fatalf("rule %d missing ruleDefaults retry: %#v", i, rule.Retry)
		}
	}
}

func TestMergeSpecs_HTTPRouteFullRulesStillReplace(t *testing.T) {
	ctx := t.Context()
	base := v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Route: &v1alpha2.GatewayRoutesSpec{
				HTTP: &v1alpha2.HTTPRouteSpec{
					Spec: &gwapiv1.HTTPRouteSpec{
						Rules: []gwapiv1.HTTPRouteRule{{
							Name: ptr.To(gwapiv1.SectionName("preset")),
							BackendRefs: []gwapiv1.HTTPBackendRef{{
								BackendRef: gwapiv1.BackendRef{
									BackendObjectReference: gwapiv1.BackendObjectReference{Name: "pool"},
								},
							}},
						}},
					},
				},
			},
		},
	}
	override := v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Route: &v1alpha2.GatewayRoutesSpec{
				HTTP: &v1alpha2.HTTPRouteSpec{
					Spec: &gwapiv1.HTTPRouteSpec{
						Rules: []gwapiv1.HTTPRouteRule{{
							Name: ptr.To(gwapiv1.SectionName("custom")),
							BackendRefs: []gwapiv1.HTTPBackendRef{{
								BackendRef: gwapiv1.BackendRef{
									BackendObjectReference: gwapiv1.BackendObjectReference{Name: "custom-svc"},
								},
							}},
						}},
					},
				},
			},
		},
	}

	merged, err := llmisvc.MergeSpecs(ctx, base, override)
	if err != nil {
		t.Fatalf("MergeSpecs() error = %v", err)
	}
	got := merged.Router.Route.HTTP.Spec.Rules
	want := override.Router.Route.HTTP.Spec.Rules
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("full rules override should replace (-want +got):\n%s", diff)
	}
}

func TestMergeSpecs_HTTPRouteTimeoutOnlyRulesReplace(t *testing.T) {
	ctx := t.Context()
	base := v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Route: &v1alpha2.GatewayRoutesSpec{
				HTTP: &v1alpha2.HTTPRouteSpec{
					Spec: &gwapiv1.HTTPRouteSpec{
						Rules: []gwapiv1.HTTPRouteRule{{
							Name: ptr.To(gwapiv1.SectionName("preset")),
							BackendRefs: []gwapiv1.HTTPBackendRef{{
								BackendRef: gwapiv1.BackendRef{
									BackendObjectReference: gwapiv1.BackendObjectReference{Name: "pool"},
								},
							}},
						}},
					},
				},
			},
		},
	}
	override := v1alpha2.LLMInferenceServiceSpec{
		Router: &v1alpha2.RouterSpec{
			Route: &v1alpha2.GatewayRoutesSpec{
				HTTP: &v1alpha2.HTTPRouteSpec{
					Spec: &gwapiv1.HTTPRouteSpec{
						Rules: []gwapiv1.HTTPRouteRule{{
							Timeouts: &gwapiv1.HTTPRouteTimeouts{
								Request: ptr.To(gwapiv1.Duration("300s")),
							},
						}},
					},
				},
			},
		},
	}

	merged, err := llmisvc.MergeSpecs(ctx, base, override)
	if err != nil {
		t.Fatalf("MergeSpecs() error = %v", err)
	}
	got := merged.Router.Route.HTTP.Spec.Rules
	if len(got) != 1 {
		t.Fatalf("expected timeout-only rule to replace preset rules, got %d rules: %#v", len(got), got)
	}
	if got[0].Timeouts == nil || ptr.Deref(got[0].Timeouts.Request, "") != "300s" {
		t.Fatalf("timeout-only rule not preserved as replacement: %#v", got[0].Timeouts)
	}
	if len(got[0].BackendRefs) != 0 {
		t.Fatalf("timeout-only rule unexpectedly inherited preset backendRefs: %#v", got[0])
	}
}
