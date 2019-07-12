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
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/pkg/apis"
)

func TestConfigurationValidation(t *testing.T) {
	tests := []struct {
		name string
		c    *Configuration
		want *apis.FieldError
	}{{
		name: "valid",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "busybox",
							}},
						},
					},
				},
			},
		},
		want: nil,
	}, {
		name: "invalid container concurrency",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "busybox",
							}},
						},
						ContainerConcurrency: -10,
					},
				},
			},
		},
		want: apis.ErrOutOfBoundsValue(
			-10, 0, RevisionContainerConcurrencyMax,
			"spec.template.spec.containerConcurrency"),
	}, {
		name: "valid BYO name",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "byo-name",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "hellworld",
							}},
						},
					},
				},
			},
		},
		want: nil,
	}, {
		name: "valid BYO name (with generateName)",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "byo-name-",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "hellworld",
							}},
						},
					},
				},
			},
		},
		want: nil,
	}, {
		name: "invalid BYO name (not prefixed)",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "byo-name",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "hellworld",
							}},
						},
					},
				},
			},
		},
		want: apis.ErrInvalidValue(`"foo" must have prefix "byo-name-"`,
			"spec.template.metadata.name"),
	}}

	// TODO(dangerd): PodSpec validation failures.

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.c.Validate(context.Background())
			if !cmp.Equal(test.want.Error(), got.Error()) {
				t.Errorf("Validate (-want, +got) = %v",
					cmp.Diff(test.want.Error(), got.Error()))
			}
		})
	}
}

func TestImmutableConfigurationFields(t *testing.T) {
	tests := []struct {
		name string
		new  *Configuration
		old  *Configuration
		want *apis.FieldError
	}{{
		name: "without byo-name",
		new: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "no-byo-name",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "helloworld:foo",
							}},
						},
					},
				},
			},
		},
		old: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "no-byo-name",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "helloworld:bar",
							}},
						},
					},
				},
			},
		},
		want: nil,
	}, {
		name: "good byo name change",
		new: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "byo-name",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "helloworld:foo",
							}},
						},
					},
				},
			},
		},
		old: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "byo-name",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-bar",
					},
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "helloworld:bar",
							}},
						},
					},
				},
			},
		},
		want: nil,
	}, {
		name: "good byo name (no change)",
		new: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "byo-name",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "helloworld:foo",
							}},
						},
					},
				},
			},
		},
		old: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "byo-name",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "helloworld:foo",
							}},
						},
					},
				},
			},
		},
		want: nil,
	}, {
		name: "bad byo name change",
		new: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "byo-name",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "helloworld:foo",
							}},
						},
					},
				},
			},
		},
		old: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "byo-name",
			},
			Spec: ConfigurationSpec{
				Template: RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						PodSpec: PodSpec{
							Containers: []corev1.Container{{
								Image: "helloworld:bar",
							}},
						},
					},
				},
			},
		},
		want: &apis.FieldError{
			Message: "Saw the following changes without a name change (-old +new)",
			Paths:   []string{"spec.template"},
			Details: "{*v1beta1.RevisionTemplateSpec}.Spec.PodSpec.Containers[0].Image:\n\t-: \"helloworld:bar\"\n\t+: \"helloworld:foo\"\n",
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = apis.WithinUpdate(ctx, test.old)
			got := test.new.Validate(ctx)
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("Validate (-want, +got) = %v\nwant: %v\ngot: %v",
					diff, test.want, got)
			}
		})
	}
}
