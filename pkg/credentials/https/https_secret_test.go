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

package https

import (
	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

var (
	header1      = "username"
	header2      = "password"
	headerValue1 = "someUsername"
	headerValue2 = "somePassword"
	uriHost      = "example.com"
)

func TestHTTPSSecret(t *testing.T) {
	scenarios := map[string]struct {
		secret   *v1.Secret
		expected []v1.EnvVar
	}{
		"noUriHost": {
			secret: &v1.Secret{
				Data: map[string][]byte{
					header1: []byte(headerValue1),
					header2: []byte(headerValue2),
				},
			},
			expected: []v1.EnvVar{},
		},
		"noHeaders": {
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{},
				Data: map[string][]byte{
					HTTPSHost: []byte(uriHost),
				},
			},
			expected: []v1.EnvVar{},
		},
		"secretEnvs": {
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{},
				Data: map[string][]byte{
					HTTPSHost: []byte(uriHost),
					HEADERS:   []byte(`{` + NEWLINE + header1 + ColonSeparator + headerValue1 + NEWLINE + header2 + ColonSeparator + headerValue2 + NEWLINE + `}`),
				},
			},
			expected: []v1.EnvVar{
				{
					Name:  uriHost + HeadersSuffix,
					Value: `{` + NEWLINE + header1 + ColonSeparator + headerValue1 + NEWLINE + header2 + ColonSeparator + headerValue2 + NEWLINE + `}`,
				},
			},
		},
	}

	for name, scenario := range scenarios {
		envs := BuildSecretEnvs(scenario.secret)

		if diff := cmp.Diff(scenario.expected, envs); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
