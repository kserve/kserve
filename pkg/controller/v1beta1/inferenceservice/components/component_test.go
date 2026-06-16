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

package components

import (
	"testing"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/stretchr/testify/assert"
)

func TestAddOtelSidecarAnnotation(t *testing.T) {
	predictorName := "my-model-predictor"

	tests := []struct {
		name        string
		autoScaling *v1beta1.AutoScalingSpec
		annotations map[string]string
		wantPresent bool
		wantValue   string
	}{
		{
			name: "injects annotation when PodMetric backend is opentelemetry",
			autoScaling: &v1beta1.AutoScalingSpec{
				Metrics: []v1beta1.MetricsSpec{
					{
						Type: v1beta1.PodMetricSourceType,
						PodMetric: &v1beta1.PodMetricSource{
							Metric: v1beta1.PodMetrics{
								Backend: v1beta1.OpenTelemetryBackend,
							},
						},
					},
				},
			},
			annotations: map[string]string{},
			wantPresent: true,
			wantValue:   predictorName,
		},
		{
			name: "does not inject when metric type is Resource",
			autoScaling: &v1beta1.AutoScalingSpec{
				Metrics: []v1beta1.MetricsSpec{
					{
						Type: v1beta1.ResourceMetricSourceType,
					},
				},
			},
			annotations: map[string]string{},
			wantPresent: false,
		},
		{
			name: "does not inject when metric type is External",
			autoScaling: &v1beta1.AutoScalingSpec{
				Metrics: []v1beta1.MetricsSpec{
					{
						Type: v1beta1.ExternalMetricSourceType,
					},
				},
			},
			annotations: map[string]string{},
			wantPresent: false,
		},
		{
			name: "does not inject when PodMetric backend is not opentelemetry",
			autoScaling: &v1beta1.AutoScalingSpec{
				Metrics: []v1beta1.MetricsSpec{
					{
						Type: v1beta1.PodMetricSourceType,
						PodMetric: &v1beta1.PodMetricSource{
							Metric: v1beta1.PodMetrics{
								Backend: "prometheus",
							},
						},
					},
				},
			},
			annotations: map[string]string{},
			wantPresent: false,
		},
		{
			name:        "does not inject when AutoScaling is nil",
			autoScaling: nil,
			annotations: map[string]string{},
			wantPresent: false,
		},
		{
			name: "does not inject when Metrics slice is empty",
			autoScaling: &v1beta1.AutoScalingSpec{
				Metrics: []v1beta1.MetricsSpec{},
			},
			annotations: map[string]string{},
			wantPresent: false,
		},
		{
			name: "preserves user-provided annotation and does not overwrite",
			autoScaling: &v1beta1.AutoScalingSpec{
				Metrics: []v1beta1.MetricsSpec{
					{
						Type: v1beta1.PodMetricSourceType,
						PodMetric: &v1beta1.PodMetricSource{
							Metric: v1beta1.PodMetrics{
								Backend: v1beta1.OpenTelemetryBackend,
							},
						},
					},
				},
			},
			annotations: map[string]string{
				constants.OTelSidecarInjectAnnotationKey: "my-custom-collector",
			},
			wantPresent: true,
			wantValue:   "my-custom-collector",
		},
		{
			name: "injects when mixed metrics include one PodMetric+opentelemetry",
			autoScaling: &v1beta1.AutoScalingSpec{
				Metrics: []v1beta1.MetricsSpec{
					{
						Type: v1beta1.ResourceMetricSourceType,
					},
					{
						Type: v1beta1.PodMetricSourceType,
						PodMetric: &v1beta1.PodMetricSource{
							Metric: v1beta1.PodMetrics{
								Backend: v1beta1.OpenTelemetryBackend,
							},
						},
					},
				},
			},
			annotations: map[string]string{},
			wantPresent: true,
			wantValue:   predictorName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addOtelSidecarAnnotation(tt.autoScaling, predictorName, tt.annotations)

			val, present := tt.annotations[constants.OTelSidecarInjectAnnotationKey]
			assert.Equal(t, tt.wantPresent, present)
			if tt.wantPresent {
				assert.Equal(t, tt.wantValue, val)
			}
		})
	}
}
