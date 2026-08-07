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

func TestExtractRoutePaths_PublisherPath(t *testing.T) {
	headerName := "X-Gateway-Model-Name"

	pathRule := func(path string, kind string) gwapiv1.HTTPRouteRule {
		return gwapiv1.HTTPRouteRule{
			Matches: []gwapiv1.HTTPRouteMatch{{
				Path: &gwapiv1.HTTPPathMatch{Value: &path},
			}},
			BackendRefs: []gwapiv1.HTTPBackendRef{{
				BackendRef: gwapiv1.BackendRef{
					BackendObjectReference: gwapiv1.BackendObjectReference{
						Kind: ptr.To(gwapiv1.Kind(kind)),
					},
				},
			}},
		}
	}

	headerRule := func(path string) gwapiv1.HTTPRouteRule {
		return gwapiv1.HTTPRouteRule{
			Matches: []gwapiv1.HTTPRouteMatch{{
				Path: &gwapiv1.HTTPPathMatch{Value: &path},
				Headers: []gwapiv1.HTTPHeaderMatch{{
					Name:  gwapiv1.HTTPHeaderName(headerName),
					Value: "publishers/ns/models/llama-70b",
				}},
			}},
			BackendRefs: []gwapiv1.HTTPBackendRef{{
				BackendRef: gwapiv1.BackendRef{
					BackendObjectReference: gwapiv1.BackendObjectReference{
						Kind: ptr.To(gwapiv1.Kind("InferencePool")),
					},
				},
			}},
		}
	}

	t.Run("publisher path appears alongside per-participant and model-routing paths", func(t *testing.T) {
		route := &gwapiv1.HTTPRoute{
			Spec: gwapiv1.HTTPRouteSpec{
				Rules: []gwapiv1.HTTPRouteRule{
					pathRule("/ns/my-llm/v1/chat/completions", "InferencePool"),
					headerRule("/v1/chat/completions"),
					pathRule("/publishers/ns/models/llama-70b", "InferencePool"),
					pathRule("/ns/my-llm", "Service"),
				},
			},
		}

		paths := extractRoutePaths(route, headerName)

		assert.Contains(t, paths, "/ns/my-llm", "per-participant path")
		assert.Contains(t, paths, "/publishers/ns/models/llama-70b", "publisher path")
		assert.Contains(t, paths, "/v1/chat/completions", "model-routing path")
		assert.Len(t, paths, 3)
	})

	t.Run("publisher path present without model-routing", func(t *testing.T) {
		route := &gwapiv1.HTTPRoute{
			Spec: gwapiv1.HTTPRouteSpec{
				Rules: []gwapiv1.HTTPRouteRule{
					pathRule("/ns/my-llm", "Service"),
					pathRule("/publishers/ns/models/llama-70b", "InferencePool"),
				},
			},
		}

		paths := extractRoutePaths(route, headerName)

		assert.Contains(t, paths, "/ns/my-llm")
		assert.Contains(t, paths, "/publishers/ns/models/llama-70b")
		assert.Len(t, paths, 2)
	})

	t.Run("no publisher path - only per-participant and model-routing", func(t *testing.T) {
		route := &gwapiv1.HTTPRoute{
			Spec: gwapiv1.HTTPRouteSpec{
				Rules: []gwapiv1.HTTPRouteRule{
					pathRule("/ns/my-llm", "Service"),
					headerRule("/v1/chat/completions"),
				},
			},
		}

		paths := extractRoutePaths(route, headerName)

		assert.Contains(t, paths, "/ns/my-llm")
		assert.Contains(t, paths, "/v1/chat/completions")
		assert.NotContains(t, paths, "/publishers/ns/models/llama-70b")
		assert.Len(t, paths, 2)
	})
}
