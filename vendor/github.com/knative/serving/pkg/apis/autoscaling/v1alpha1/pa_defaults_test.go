/*
Copyright 2018 The Knative Authors

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
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/serving/pkg/apis/autoscaling"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodAutoscalerDefaulting(t *testing.T) {
	tests := []struct {
		name string
		in   *PodAutoscaler
		want *PodAutoscaler
	}{{
		name: "empty",
		in:   &PodAutoscaler{},
		want: &PodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					autoscaling.ClassAnnotationKey:  autoscaling.KPA,
					autoscaling.MetricAnnotationKey: autoscaling.Concurrency,
				},
			},
			Spec: PodAutoscalerSpec{
				ContainerConcurrency: 0,
			},
		},
	}, {
		name: "no overwrite",
		in: &PodAutoscaler{
			Spec: PodAutoscalerSpec{
				ContainerConcurrency: 1,
			},
		},
		want: &PodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					autoscaling.ClassAnnotationKey:  autoscaling.KPA,
					autoscaling.MetricAnnotationKey: autoscaling.Concurrency,
				},
			},
			Spec: PodAutoscalerSpec{
				ContainerConcurrency: 1,
			},
		},
	}, {
		name: "partially initialized",
		in: &PodAutoscaler{
			Spec: PodAutoscalerSpec{},
		},
		want: &PodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					autoscaling.ClassAnnotationKey:  autoscaling.KPA,
					autoscaling.MetricAnnotationKey: autoscaling.Concurrency,
				},
			},
			Spec: PodAutoscalerSpec{
				ContainerConcurrency: 0,
			},
		},
	}, {
		name: "hpa class is not overwritten and defaults to cpu",
		in: &PodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					autoscaling.ClassAnnotationKey: autoscaling.HPA,
				},
			},
		},
		want: &PodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					autoscaling.ClassAnnotationKey:  autoscaling.HPA,
					autoscaling.MetricAnnotationKey: autoscaling.CPU,
				},
			},
			Spec: PodAutoscalerSpec{
				ContainerConcurrency: 0,
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.in
			got.SetDefaults(context.Background())
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("SetDefaults (-want, +got) = %v", diff)
			}
		})
	}
}
