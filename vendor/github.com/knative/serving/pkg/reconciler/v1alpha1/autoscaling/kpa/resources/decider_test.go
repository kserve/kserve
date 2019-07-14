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

package resources

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/serving/pkg/apis/autoscaling"
	"github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	"github.com/knative/serving/pkg/autoscaler"
	. "github.com/knative/serving/pkg/reconciler/v1alpha1/testing"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMakeDecider(t *testing.T) {
	cases := []struct {
		name string
		pa   *v1alpha1.PodAutoscaler
		want *autoscaler.Decider
	}{{
		name: "defaults",
		pa:   pa(),
		want: decider(withTarget(100.0)),
	}, {
		name: "with container concurrency 1",
		pa:   pa(WithContainerConcurrency(1)),
		want: decider(withTarget(1.0)),
	}, {
		name: "with target annotation 1",
		pa:   pa(WithTargetAnnotation("1")),
		want: decider(withTarget(1.0), withTargetAnnotation("1")),
	}, {
		name: "with container concurrency greater than target annotation (ok)",
		pa:   pa(WithContainerConcurrency(10), WithTargetAnnotation("1")),
		want: decider(withTarget(1.0), withTargetAnnotation("1")),
	}, {
		name: "with target annotation greater than container concurrency (ignore annotation for safety)",
		pa:   pa(WithContainerConcurrency(1), WithTargetAnnotation("10")),
		want: decider(withTarget(1.0), withTargetAnnotation("10")),
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, MakeDecider(context.Background(), tc.pa, config)); diff != "" {
				t.Errorf("%q (-want, +got):\n%v", tc.name, diff)
			}
		})
	}
}

func pa(options ...PodAutoscalerOption) *v1alpha1.PodAutoscaler {
	p := &v1alpha1.PodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-name",
			Annotations: map[string]string{
				autoscaling.ClassAnnotationKey: autoscaling.KPA,
			},
		},
		Spec: v1alpha1.PodAutoscalerSpec{
			ContainerConcurrency: 0,
		},
		Status: v1alpha1.PodAutoscalerStatus{},
	}
	for _, fn := range options {
		fn(p)
	}
	return p
}

func decider(options ...DeciderOption) *autoscaler.Decider {
	m := &autoscaler.Decider{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-name",
			Annotations: map[string]string{
				autoscaling.ClassAnnotationKey: autoscaling.KPA,
			},
		},
		Spec: autoscaler.DeciderSpec{
			TargetConcurrency: float64(100),
		},
	}
	for _, fn := range options {
		fn(m)
	}
	return m
}

type DeciderOption func(*autoscaler.Decider)

func withTarget(target float64) DeciderOption {
	return func(decider *autoscaler.Decider) {
		decider.Spec.TargetConcurrency = target
	}
}

func withTargetAnnotation(target string) DeciderOption {
	return func(decider *autoscaler.Decider) {
		decider.Annotations[autoscaling.TargetAnnotationKey] = target
	}
}

var config = &autoscaler.Config{
	EnableScaleToZero:                    true,
	ContainerConcurrencyTargetPercentage: 1.0,
	ContainerConcurrencyTargetDefault:    100.0,
	MaxScaleUpRate:                       10.0,
	StableWindow:                         60 * time.Second,
	PanicWindow:                          6 * time.Second,
	TickInterval:                         2 * time.Second,
	ScaleToZeroGracePeriod:               30 * time.Second,
}
