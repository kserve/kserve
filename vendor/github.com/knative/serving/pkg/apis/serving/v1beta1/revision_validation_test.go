/*
Copyright 2019 The Knative Authors

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

package v1beta1

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	"github.com/knative/pkg/ptr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/serving/pkg/apis/networking"
)

func TestRevisionValidation(t *testing.T) {
	tests := []struct {
		name string
		r    *Revision
		want *apis.FieldError
	}{{
		name: "valid",
		r: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Image: "busybox",
					}},
				},
			},
		},
		want: nil,
	}, {
		name: "invalid container concurrency",
		r: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Image: "busybox",
					}},
				},
				ContainerConcurrency: -10,
			},
		},
		want: apis.ErrOutOfBoundsValue(
			-10, 0, RevisionContainerConcurrencyMax,
			"spec.containerConcurrency"),
	}}

	// TODO(dangerd): PodSpec validation failures.

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.r.Validate(context.Background())
			if !cmp.Equal(test.want.Error(), got.Error()) {
				t.Errorf("Validate (-want, +got) = %v",
					cmp.Diff(test.want.Error(), got.Error()))
			}
		})
	}
}

func TestContainerConcurrencyValidation(t *testing.T) {
	tests := []struct {
		name string
		cc   RevisionContainerConcurrencyType
		want *apis.FieldError
	}{{
		name: "single",
		cc:   1,
		want: nil,
	}, {
		name: "unlimited",
		cc:   0,
		want: nil,
	}, {
		name: "ten",
		cc:   10,
		want: nil,
	}, {
		name: "invalid container concurrency (too small)",
		cc:   -1,
		want: apis.ErrOutOfBoundsValue(-1, 0, RevisionContainerConcurrencyMax,
			apis.CurrentField),
	}, {
		name: "invalid container concurrency (too large)",
		cc:   RevisionContainerConcurrencyMax + 1,
		want: apis.ErrOutOfBoundsValue(RevisionContainerConcurrencyMax+1,
			0, RevisionContainerConcurrencyMax, apis.CurrentField),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.cc.Validate(context.Background())
			if !cmp.Equal(test.want.Error(), got.Error()) {
				t.Errorf("Validate (-want, +got) = %v",
					cmp.Diff(test.want.Error(), got.Error()))
			}
		})
	}
}

func TestRevisionSpecValidation(t *testing.T) {
	tests := []struct {
		name string
		rs   *RevisionSpec
		want *apis.FieldError
	}{{
		name: "valid",
		rs: &RevisionSpec{
			PodSpec: PodSpec{
				Containers: []corev1.Container{{
					Image: "helloworld",
				}},
			},
		},
		want: nil,
	}, {
		name: "with volume (ok)",
		rs: &RevisionSpec{
			PodSpec: PodSpec{
				Containers: []corev1.Container{{
					Image: "helloworld",
					VolumeMounts: []corev1.VolumeMount{{
						MountPath: "/mount/path",
						Name:      "the-name",
						ReadOnly:  true,
					}},
				}},
				Volumes: []corev1.Volume{{
					Name: "the-name",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "foo",
						},
					},
				}},
			},
		},
		want: nil,
	}, {
		name: "with volume name collision",
		rs: &RevisionSpec{
			PodSpec: PodSpec{
				Containers: []corev1.Container{{
					Image: "helloworld",
					VolumeMounts: []corev1.VolumeMount{{
						MountPath: "/mount/path",
						Name:      "the-name",
						ReadOnly:  true,
					}},
				}},
				Volumes: []corev1.Volume{{
					Name: "the-name",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "foo",
						},
					},
				}, {
					Name: "the-name",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{},
					},
				}},
			},
		},
		want: (&apis.FieldError{
			Message: fmt.Sprintf(`duplicate volume name "the-name"`),
			Paths:   []string{"name"},
		}).ViaFieldIndex("volumes", 1),
	}, {
		name: "bad pod spec",
		rs: &RevisionSpec{
			PodSpec: PodSpec{
				Containers: []corev1.Container{{
					Name:  "steve",
					Image: "helloworld",
				}},
			},
		},
		want: apis.ErrDisallowedFields("containers[0].name"),
	}, {
		name: "missing container",
		rs: &RevisionSpec{
			PodSpec: PodSpec{
				Containers: []corev1.Container{},
			},
		},
		want: apis.ErrMissingField("containers"),
	}, {
		name: "too many containers",
		rs: &RevisionSpec{
			PodSpec: PodSpec{
				Containers: []corev1.Container{{
					Image: "busybox",
				}, {
					Image: "helloworld",
				}},
			},
		},
		want: apis.ErrMultipleOneOf("containers"),
	}, {
		name: "exceed max timeout",
		rs: &RevisionSpec{
			PodSpec: PodSpec{
				Containers: []corev1.Container{{
					Image: "helloworld",
				}},
			},
			TimeoutSeconds: ptr.Int64(6000),
		},
		want: apis.ErrOutOfBoundsValue(
			6000, 0, networking.DefaultTimeout.Seconds(),
			"timeoutSeconds"),
	}, {
		name: "negative timeout",
		rs: &RevisionSpec{
			PodSpec: PodSpec{
				Containers: []corev1.Container{{
					Image: "helloworld",
				}},
			},
			TimeoutSeconds: ptr.Int64(-30),
		},
		want: apis.ErrOutOfBoundsValue(
			-30, 0, networking.DefaultTimeout.Seconds(),
			"timeoutSeconds"),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.rs.Validate(context.Background())
			if !cmp.Equal(test.want.Error(), got.Error()) {
				t.Errorf("Validate (-want, +got) = %v",
					cmp.Diff(test.want.Error(), got.Error()))
			}
		})
	}
}

func TestImmutableFields(t *testing.T) {
	tests := []struct {
		name string
		new  *Revision
		old  *Revision
		want *apis.FieldError
	}{{
		name: "good (no change)",
		new: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Image: "helloworld",
					}},
				},
			},
		},
		old: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Image: "helloworld",
					}},
				},
			},
		},
		want: nil,
	}, {
		name: "bad (resources image change)",
		new: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName("cpu"): resource.MustParse("50m"),
							},
						},
					}},
				},
			},
		},
		old: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName("cpu"): resource.MustParse("100m"),
							},
						},
					}},
				},
			},
		},
		want: &apis.FieldError{
			Message: "Immutable fields changed (-old +new)",
			Paths:   []string{"spec"},
			Details: `{v1beta1.RevisionSpec}.PodSpec.Containers[0].Resources.Requests["cpu"]:
	-: resource.Quantity: "{i:{value:100 scale:-3} d:{Dec:<nil>} s:100m Format:DecimalSI}"
	+: resource.Quantity: "{i:{value:50 scale:-3} d:{Dec:<nil>} s:50m Format:DecimalSI}"
`,
		},
	}, {
		name: "bad (container image change)",
		new: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Image: "helloworld",
					}},
				},
			},
		},
		old: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Image: "busybox",
					}},
				},
			},
		},
		want: &apis.FieldError{
			Message: "Immutable fields changed (-old +new)",
			Paths:   []string{"spec"},
			Details: `{v1beta1.RevisionSpec}.PodSpec.Containers[0].Image:
	-: "busybox"
	+: "helloworld"
`,
		},
	}, {
		name: "bad (concurrency model change)",
		new: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Image: "helloworld",
					}},
				},
				ContainerConcurrency: 1,
			},
		},
		old: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Image: "helloworld",
					}},
				},
				ContainerConcurrency: 2,
			},
		},
		want: &apis.FieldError{
			Message: "Immutable fields changed (-old +new)",
			Paths:   []string{"spec"},
			Details: `{v1beta1.RevisionSpec}.ContainerConcurrency:
	-: "2"
	+: "1"
`,
		},
	}, {
		name: "bad (new field added)",
		new: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Image: "helloworld",
					}},
				},
				ContainerConcurrency: 42,
			},
		},
		old: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Image: "helloworld",
					}},
				},
			},
		},
		want: &apis.FieldError{
			Message: "Immutable fields changed (-old +new)",
			Paths:   []string{"spec"},
			Details: `{v1beta1.RevisionSpec}.ContainerConcurrency:
	-: "0"
	+: "42"
`,
		},
	}, {
		name: "bad (multiple changes)",
		new: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Image: "helloworld",
					}},
				},
				ContainerConcurrency: 2,
			},
		},
		old: &Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: RevisionSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{{
						Image: "busybox",
					}},
				},
				ContainerConcurrency: 4,
			},
		},
		want: &apis.FieldError{
			Message: "Immutable fields changed (-old +new)",
			Paths:   []string{"spec"},
			Details: `{v1beta1.RevisionSpec}.PodSpec.Containers[0].Image:
	-: "busybox"
	+: "helloworld"
{v1beta1.RevisionSpec}.ContainerConcurrency:
	-: "4"
	+: "2"
`,
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := apis.WithinUpdate(context.Background(), test.old)
			got := test.new.Validate(ctx)
			if !cmp.Equal(test.want.Error(), got.Error()) {
				t.Errorf("Validate (-want, +got) = %v",
					cmp.Diff(test.want.Error(), got.Error()))
			}
		})
	}
}
