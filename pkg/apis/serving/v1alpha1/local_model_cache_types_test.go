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

package v1alpha1

import (
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestLocalModelCache_MatchStorageURI(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	uriSpec := LocalModelCacheSpec{
		SourceModelUri: "gs://bucket/model",
		ModelSize:      resource.MustParse("123Gi"),
		NodeGroups:     []string{"gpu"},
	}
	nonUriSpec := LocalModelCacheSpec{
		SourceModelUri: "foo:bar:baz",
		ModelSize:      resource.MustParse("123Gi"),
		NodeGroups:     []string{"gpu"},
	}
	cases := []struct {
		name       string
		spec       LocalModelCacheSpec
		storageUri string
		isMatch    bool
	}{
		{
			name:       "exact match for uri",
			spec:       uriSpec,
			storageUri: "gs://bucket/model",
			isMatch:    true,
		},
		{
			name:       "uri subpath match",
			spec:       uriSpec,
			storageUri: "gs://bucket/model/modelfiles/",
			isMatch:    true,
		},
		{
			name:       "model file match",
			spec:       uriSpec,
			storageUri: "gs://bucket/model/modelfiles/model.pt",
			isMatch:    true,
		},
		{
			name:       "uriSpec does not match",
			spec:       uriSpec,
			storageUri: "gs://bucket/model2",
			isMatch:    false,
		},
		{
			name:       "uriSpec does not match (2nd)",
			spec:       uriSpec,
			storageUri: "gs://bucket/foo",
			isMatch:    false,
		},
		{
			name:       "nonUriSpec match",
			spec:       nonUriSpec,
			storageUri: "foo:bar:baz",
			isMatch:    true,
		},
		{
			name:       "nonUriSpec does not match",
			spec:       nonUriSpec,
			storageUri: "foo:bar:bazzzz",
			isMatch:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			match := tc.spec.MatchStorageURI(tc.storageUri)
			g.Expect(match).To(gomega.Equal(tc.isMatch))
		})
	}
}
