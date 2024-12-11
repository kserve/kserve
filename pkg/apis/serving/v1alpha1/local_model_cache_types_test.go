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

package v1alpha1

import (
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestModelCacheSpec_ValidateName(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	spec := LocalModelCacheSpec{
		SourceModelUri: "s3://bucket/model",
		ModelSize:      resource.MustParse("1Gi"),
		NodeGroup:      "gpu",
	}
	cases := []struct {
		name  string
		cache LocalModelCache
		valid bool
	}{
		{
			name: "name without dot",
			cache: LocalModelCache{
				TypeMeta: metav1.TypeMeta{
					Kind:       "LocalModelCache",
					APIVersion: "v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "llama-3-2",
				},
				Spec: spec,
			},
			valid: true,
		},
		{
			cache: LocalModelCache{
				TypeMeta: metav1.TypeMeta{
					Kind:       "LocalModelCache",
					APIVersion: "v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "llama-3.2",
				},
				Spec: spec,
			},
			valid: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errors := tc.cache.Validate()
			if tc.valid {
				g.Expect(errors).To(gomega.BeEmpty())
			} else {
				g.Expect(errors).NotTo(gomega.BeEmpty())
			}
		})
	}
}
