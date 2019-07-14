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
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/pkg/apis"
	"github.com/knative/serving/pkg/apis/autoscaling"
	net "github.com/knative/serving/pkg/apis/networking"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
)

func TestPodAutoscalerSpecValidation(t *testing.T) {
	tests := []struct {
		name string
		rs   *PodAutoscalerSpec
		want *apis.FieldError
	}{{
		name: "valid",
		rs: &PodAutoscalerSpec{
			ContainerConcurrency: 0,
			ServiceName:          "foo",
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "bar",
			},
		},
		want: nil,
	}, {
		name: "has missing scaleTargetRef",
		rs: &PodAutoscalerSpec{
			ContainerConcurrency: 0,
			ServiceName:          "foo",
		},
		want: apis.ErrMissingField("scaleTargetRef"),
	}, {
		name: "has missing scaleTargetRef kind",
		rs: &PodAutoscalerSpec{
			ContainerConcurrency: 0,
			ServiceName:          "foo",
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Name:       "bar",
			},
		},
		want: apis.ErrMissingField("scaleTargetRef.kind"),
	}, {
		name: "has missing scaleTargetRef apiVersion",
		rs: &PodAutoscalerSpec{
			ContainerConcurrency: 0,
			ServiceName:          "foo",
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				Kind: "Deployment",
				Name: "bar",
			},
		},
		want: apis.ErrMissingField("scaleTargetRef.apiVersion"),
	}, {
		name: "has missing scaleTargetRef name",
		rs: &PodAutoscalerSpec{
			ContainerConcurrency: 0,
			ServiceName:          "foo",
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
		},
		want: apis.ErrMissingField("scaleTargetRef.name"),
	}, {
		name: "has missing serviceName",
		rs: &PodAutoscalerSpec{
			ContainerConcurrency: 0,
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "bar",
			},
		},
		want: apis.ErrMissingField("serviceName"),
	}, {
		name: "bad container concurrency",
		rs: &PodAutoscalerSpec{
			ContainerConcurrency: -1,
			ServiceName:          "foo",
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "bar",
			},
		},
		want: apis.ErrOutOfBoundsValue(-1, 0,
			v1beta1.RevisionContainerConcurrencyMax, "containerConcurrency"),
	}, {
		name: "multi invalid, bad concurrency and missing ref kind",
		rs: &PodAutoscalerSpec{
			ContainerConcurrency: -2,
			ServiceName:          "foo",
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Name:       "bar",
			},
		},
		want: apis.ErrOutOfBoundsValue(-2, 0,
			v1beta1.RevisionContainerConcurrencyMax, "containerConcurrency").Also(
			apis.ErrMissingField("scaleTargetRef.kind")),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.rs.Validate(context.Background())
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("Validate (-want, +got) = %v", diff)
			}
		})
	}
}

func TestPodAutoscalerValidation(t *testing.T) {
	tests := []struct {
		name string
		r    *PodAutoscaler
		want *apis.FieldError
	}{{
		name: "valid",
		r: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
				Annotations: map[string]string{
					"minScale": "2",
				},
			},
			Spec: PodAutoscalerSpec{
				ServiceName: "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
				ProtocolType: net.ProtocolHTTP1,
			},
		},
		want: nil,
	}, {
		name: "valid, optional fields",
		r: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
				Annotations: map[string]string{
					"minScale": "2",
				},
			},
			Spec: PodAutoscalerSpec{
				ServiceName: "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
			},
		},
		want: nil,
	}, {
		name: "bad protocol",
		r: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
				Annotations: map[string]string{
					"minScale": "2",
				},
			},
			Spec: PodAutoscalerSpec{
				ServiceName: "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
				ProtocolType: net.ProtocolType("WebSocket"),
			},
		},
		want: apis.ErrInvalidValue("WebSocket", "spec.protocolType"),
	}, {
		name: "bad scale bounds",
		r: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
				Annotations: map[string]string{
					autoscaling.MinScaleAnnotationKey: "FOO",
				},
			},
			Spec: PodAutoscalerSpec{
				ServiceName: "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
			},
		},
		want: (&apis.FieldError{
			Message: fmt.Sprintf("Invalid %s annotation value: must be an integer greater than 0", autoscaling.MinScaleAnnotationKey),
			Paths:   []string{autoscaling.MinScaleAnnotationKey},
		}).ViaField("annotations").ViaField("metadata"),
	}, {
		name: "empty spec",
		r: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
			},
		},
		want: apis.ErrMissingField("spec"),
	}, {
		name: "nested spec error",
		r: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
			},
			Spec: PodAutoscalerSpec{
				ContainerConcurrency: -1,
				ServiceName:          "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
			},
		},
		want: apis.ErrOutOfBoundsValue(-1, 0,
			v1beta1.RevisionContainerConcurrencyMax, "spec.containerConcurrency"),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.r.Validate(context.Background())
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("Validate (-want, +got) = %v", diff)
			}
		})
	}
}

func TestImmutableFields(t *testing.T) {
	tests := []struct {
		name string
		new  *PodAutoscaler
		old  *PodAutoscaler
		want *apis.FieldError
	}{{
		name: "good (no change)",
		new: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
			},
			Spec: PodAutoscalerSpec{
				ServiceName: "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
			},
		},
		old: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
			},
			Spec: PodAutoscalerSpec{
				ServiceName: "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
			},
		},
		want: nil,
	}, {
		name: "good (protocol added)",
		new: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
			},
			Spec: PodAutoscalerSpec{
				ServiceName: "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
				ProtocolType: net.ProtocolHTTP1,
			},
		},
		old: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
			},
			Spec: PodAutoscalerSpec{
				ServiceName: "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
			},
		},
		want: nil,
	}, {
		name: "bad (concurrency model change)",
		new: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
			},
			Spec: PodAutoscalerSpec{
				ServiceName: "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
			},
		},
		old: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
			},
			Spec: PodAutoscalerSpec{
				ContainerConcurrency: 1,
				ServiceName:          "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
			},
		},
		want: &apis.FieldError{
			Message: "Immutable fields changed (-old +new)",
			Paths:   []string{"spec"},
			Details: `{v1alpha1.PodAutoscalerSpec}.ContainerConcurrency:
	-: "1"
	+: "0"
`,
		},
	}, {
		name: "bad (container concurrency change)",
		new: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
			},
			Spec: PodAutoscalerSpec{
				ContainerConcurrency: 0,
				ServiceName:          "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
			},
		},
		old: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
			},
			Spec: PodAutoscalerSpec{
				ContainerConcurrency: 1,
				ServiceName:          "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
			},
		},
		want: &apis.FieldError{
			Message: "Immutable fields changed (-old +new)",
			Paths:   []string{"spec"},
			Details: `{v1alpha1.PodAutoscalerSpec}.ContainerConcurrency:
	-: "1"
	+: "0"
`,
		},
	}, {
		name: "bad (multiple changes)",
		new: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
			},
			Spec: PodAutoscalerSpec{
				ServiceName: "foo",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "bar",
				},
			},
		},
		old: &PodAutoscaler{
			ObjectMeta: v1.ObjectMeta{
				Name: "valid",
			},
			Spec: PodAutoscalerSpec{
				ContainerConcurrency: 1,
				ServiceName:          "food",
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "baz",
				},
			},
		},
		want: &apis.FieldError{
			Message: "Immutable fields changed (-old +new)",
			Paths:   []string{"spec"},
			Details: `{v1alpha1.PodAutoscalerSpec}.ContainerConcurrency:
	-: "1"
	+: "0"
{v1alpha1.PodAutoscalerSpec}.ScaleTargetRef.Name:
	-: "baz"
	+: "bar"
{v1alpha1.PodAutoscalerSpec}.ServiceName:
	-: "food"
	+: "foo"
`,
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = apis.WithinUpdate(ctx, test.old)
			got := test.new.Validate(ctx)
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("Validate (-want, +got) = %v", diff)
			}
		})
	}
}
