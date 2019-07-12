/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gc

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"

	. "github.com/knative/pkg/configmap/testing"
)

func TestOurConfig(t *testing.T) {
	actual, example := ConfigMapsFromTestFile(t, "config-gc")
	for _, tt := range []struct {
		name string
		fail bool
		want Config
		data *corev1.ConfigMap
	}{{
		name: "actual config",
		fail: false,
		want: Config{
			StaleRevisionCreateDelay:        24 * time.Hour,
			StaleRevisionTimeout:            15 * time.Hour,
			StaleRevisionMinimumGenerations: 1,
			StaleRevisionLastpinnedDebounce: 5 * time.Hour,
		},
		data: actual,
	}, {
		name: "example config",
		fail: false,
		want: Config{
			StaleRevisionCreateDelay:        24 * time.Hour,
			StaleRevisionTimeout:            15 * time.Hour,
			StaleRevisionMinimumGenerations: 1,
			StaleRevisionLastpinnedDebounce: 5 * time.Hour,
		},
		data: example,
	}, {
		name: "With value overrides",
		want: Config{
			StaleRevisionCreateDelay:        15 * time.Hour,
			StaleRevisionTimeout:            15 * time.Hour,
			StaleRevisionMinimumGenerations: 10,
			StaleRevisionLastpinnedDebounce: 5 * time.Hour,
		},
		data: &corev1.ConfigMap{
			Data: map[string]string{
				"stale-revision-create-delay":        "15h",
				"stale-revision-minimum-generations": "10",
			},
		},
	}, {
		name: "Invalid duration",
		fail: true,
		want: Config{},
		data: &corev1.ConfigMap{
			Data: map[string]string{
				"stale-revision-create-delay": "invalid",
			},
		},
	}, {
		name: "Invalid minimum generation",
		fail: true,
		want: Config{},
		data: &corev1.ConfigMap{
			Data: map[string]string{
				"stale-revision-minimum-generations": "invalid",
			},
		},
	}} {
		t.Run(tt.name, func(t *testing.T) {
			testConfig, err := NewConfigFromConfigMap(tt.data)
			if tt.fail != (err != nil) {
				t.Errorf("Unexpected error value: %v", err)
			}

			if tt.fail {
				return
			}

			if diff := cmp.Diff(tt.want, *testConfig); diff != "" {
				t.Errorf("Unexpected controller config (-want, +got): %v", diff)
			}
		})
	}
}
