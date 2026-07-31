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

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

const latencyProducerConfig = `
plugins:
- type: predicted-latency-producer
- type: latency-scorer
- type: weighted-random-picker
`

const noLatencyConfig = `
plugins:
- type: queue-scorer
- type: max-score-picker
`

func TestHasLatencyProducerInSpec(t *testing.T) {
	tests := []struct {
		name string
		spec v1alpha2.LLMInferenceServiceSpec
		want bool
	}{
		{
			name: "plugin in inline config",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Config: &v1alpha2.SchedulerConfigSpec{
							Inline: &runtime.RawExtension{Raw: []byte(latencyProducerConfig)},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "no plugin",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Config: &v1alpha2.SchedulerConfigSpec{
							Inline: &runtime.RawExtension{Raw: []byte(noLatencyConfig)},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "nil config",
			spec: v1alpha2.LLMInferenceServiceSpec{},
			want: false,
		},
		{
			name: "nil inline",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Config: &v1alpha2.SchedulerConfigSpec{},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			g.Expect(hasLatencyProducerInSpec(tt.spec)).To(Equal(tt.want))
		})
	}
}
