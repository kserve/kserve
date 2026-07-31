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
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestIsPerParticipantRule(t *testing.T) {
	prefix := "/default/my-svc"

	tests := []struct {
		name string
		rule gwapiv1.HTTPRouteRule
		want bool
	}{
		{
			name: "exact prefix match",
			rule: gwapiv1.HTTPRouteRule{
				Matches: []gwapiv1.HTTPRouteMatch{{
					Path: &gwapiv1.HTTPPathMatch{Value: ptr.To(prefix)},
				}},
			},
			want: true,
		},
		{
			name: "prefix with trailing path",
			rule: gwapiv1.HTTPRouteRule{
				Matches: []gwapiv1.HTTPRouteMatch{{
					Path: &gwapiv1.HTTPPathMatch{Value: ptr.To(prefix + "/v1/completions")},
				}},
			},
			want: true,
		},
		{
			name: "non-matching path",
			rule: gwapiv1.HTTPRouteRule{
				Matches: []gwapiv1.HTTPRouteMatch{{
					Path: &gwapiv1.HTTPPathMatch{Value: ptr.To("/v1/chat/completions")},
				}},
			},
			want: false,
		},
		{
			name: "nil path value",
			rule: gwapiv1.HTTPRouteRule{
				Matches: []gwapiv1.HTTPRouteMatch{{
					Path: &gwapiv1.HTTPPathMatch{Value: nil},
				}},
			},
			want: false,
		},
		{
			name: "empty matches",
			rule: gwapiv1.HTTPRouteRule{
				Matches: []gwapiv1.HTTPRouteMatch{},
			},
			want: false,
		},
		{
			name: "partial name collision - segment boundary guard",
			rule: gwapiv1.HTTPRouteRule{
				Matches: []gwapiv1.HTTPRouteMatch{{
					Path: &gwapiv1.HTTPPathMatch{Value: ptr.To("/default/my-svc-v2")},
				}},
			},
			want: false,
		},
		{
			name: "header-only match no path",
			rule: gwapiv1.HTTPRouteRule{
				Matches: []gwapiv1.HTTPRouteMatch{{
					Headers: []gwapiv1.HTTPHeaderMatch{{
						Name:  "x-model",
						Value: "test",
					}},
				}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isPerParticipantRule(tt.rule, prefix))
		})
	}
}
