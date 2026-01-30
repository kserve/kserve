/*
Copyright 2021 The KServe Authors.

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
package pod

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testDefaultNodepool = "default-pool"
	testCustomNodepool  = "custom-pool"
	testCustomLabelKey  = "custom.io/nodepool"
)

func TestAffinityInjector(t *testing.T) {
	scenarios := map[string]struct {
		envNodepool string
		envLabelKey string
		original    *v1.Pod
		expected    *v1.Pod
	}{
		"AddAffinityWithDefaultLabelKey": {
			envNodepool: testDefaultNodepool,
			envLabelKey: "",
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Affinity: nil,
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Affinity: defaultAffinity(defaultNodepoolLabelKey, testDefaultNodepool),
				},
			},
		},
		"AddAffinityWithCustomLabelKey": {
			envNodepool: testDefaultNodepool,
			envLabelKey: testCustomLabelKey,
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Affinity: nil,
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Affinity: defaultAffinity(testCustomLabelKey, testDefaultNodepool),
				},
			},
		},
		"DoNotOverwriteExistingAffinity": {
			envNodepool: testDefaultNodepool,
			envLabelKey: "",
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Affinity: defaultAffinity(defaultNodepoolLabelKey, testCustomNodepool),
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Affinity: defaultAffinity(defaultNodepoolLabelKey, testCustomNodepool),
				},
			},
		},
		"SkipWhenEnvNotSet": {
			envNodepool: "",
			envLabelKey: "",
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Affinity: nil,
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Affinity: nil,
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			// Set environment variables
			if scenario.envNodepool != "" {
				assert.Nil(t, os.Setenv(DefaultNodepoolEnvVar, scenario.envNodepool))
				defer os.Unsetenv(DefaultNodepoolEnvVar)
			} else {
				os.Unsetenv(DefaultNodepoolEnvVar)
			}

			if scenario.envLabelKey != "" {
				assert.Nil(t, os.Setenv(DefaultNodepoolLabelKey, scenario.envLabelKey))
				defer os.Unsetenv(DefaultNodepoolLabelKey)
			} else {
				os.Unsetenv(DefaultNodepoolLabelKey)
			}

			// Run the injector
			assert.Nil(t, InjectAffinity(scenario.original))
			assert.Equal(t, scenario.expected.Spec.Affinity, scenario.original.Spec.Affinity)
		})
	}
}
