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

package aggregator

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChainFilters(t *testing.T) {
	backends := []Backend{
		{Name: "a", Namespace: "team-a"},
		{Name: "b", Namespace: "team-b"},
		{Name: "c", Namespace: "team-a"},
	}
	byNamespace := func(_ *http.Request, in []Backend) []Backend {
		var out []Backend
		for _, b := range in {
			if b.Namespace == "team-a" {
				out = append(out, b)
			}
		}
		return out
	}
	byName := func(_ *http.Request, in []Backend) []Backend {
		var out []Backend
		for _, b := range in {
			if b.Name != "c" {
				out = append(out, b)
			}
		}
		return out
	}

	got := ChainFilters(byNamespace, byName)(&http.Request{}, backends)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].Name)

	got = ChainFilters(nil, byNamespace)(&http.Request{}, backends)
	require.Len(t, got, 2)
}

func TestBackendIDIncludesAddress(t *testing.T) {
	assert.Equal(t, "llama", Backend{Name: "llama"}.ID())
	assert.Equal(t, "ns/llama", Backend{Name: "llama", Namespace: "ns"}.ID())
	assert.Equal(t, "ns/llama@internal", Backend{Name: "llama", Namespace: "ns", Address: "internal"}.ID())
}
