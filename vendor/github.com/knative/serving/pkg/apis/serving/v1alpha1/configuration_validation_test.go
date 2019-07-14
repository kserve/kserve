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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	"github.com/knative/pkg/apis"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
)

func TestConfigurationSpecValidation(t *testing.T) {
	tests := []struct {
		name string
		c    *ConfigurationSpec
		want *apis.FieldError
	}{{
		name: "valid",
		c: &ConfigurationSpec{
			RevisionTemplate: &RevisionTemplateSpec{
				Spec: RevisionSpec{
					Container: &corev1.Container{
						Image: "hellworld",
					},
				},
			},
		},
		want: nil,
	}, {
		name: "valid podspec",
		c: &ConfigurationSpec{
			RevisionTemplate: &RevisionTemplateSpec{
				Spec: RevisionSpec{
					RevisionSpec: v1beta1.RevisionSpec{
						PodSpec: v1beta1.PodSpec{
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
		name: "propagate revision failures",
		c: &ConfigurationSpec{
			RevisionTemplate: &RevisionTemplateSpec{
				Spec: RevisionSpec{
					Container: &corev1.Container{
						Name:  "stuart",
						Image: "hellworld",
					},
				},
			},
		},
		want: apis.ErrDisallowedFields("revisionTemplate.spec.container.name"),
	}, {
		name: "build is a BuildSpec",
		c: &ConfigurationSpec{
			Build: &RawExtension{
				BuildSpec: &buildv1alpha1.BuildSpec{
					Steps: []corev1.Container{{
						Image: "foo",
					}},
				},
			},
			RevisionTemplate: &RevisionTemplateSpec{
				Spec: RevisionSpec{
					Container: &corev1.Container{
						Image: "hellworld",
					},
				},
			},
		},
		want: nil,
	}, {
		name: "build is an Object",
		c: &ConfigurationSpec{
			Build: &RawExtension{
				Object: &buildv1alpha1.Build{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "build.knative.dev/v1alpha1",
						Kind:       "Build",
					},
					Spec: buildv1alpha1.BuildSpec{
						Steps: []corev1.Container{{
							Image: "foo",
						}},
					},
				},
			},
			RevisionTemplate: &RevisionTemplateSpec{
				Spec: RevisionSpec{
					Container: &corev1.Container{
						Image: "hellworld",
					},
				},
			},
		},
		want: nil,
	}, {
		name: "build is missing TypeMeta",
		c: &ConfigurationSpec{
			Build: &RawExtension{
				Object: &buildv1alpha1.Build{
					Spec: buildv1alpha1.BuildSpec{
						Steps: []corev1.Container{{
							Image: "foo",
						}},
					},
				},
			},
			RevisionTemplate: &RevisionTemplateSpec{
				Spec: RevisionSpec{
					Container: &corev1.Container{
						Image: "hellworld",
					},
				},
			},
		},
		want: apis.ErrInvalidValue("Object 'Kind' is missing in '{\"metadata\":{\"creationTimestamp\":null},\"spec\":{\"steps\":[{\"name\":\"\",\"image\":\"foo\",\"resources\":{}}],\"Status\":\"\"},\"status\":{\"stepsCompleted\":null}}'", "build"),
	}, {
		name: "build is not an object",
		c: &ConfigurationSpec{
			Build: &RawExtension{
				Raw: []byte(`"foo"`),
			},
			RevisionTemplate: &RevisionTemplateSpec{
				Spec: RevisionSpec{
					Container: &corev1.Container{
						Image: "hellworld",
					},
				},
			},
		},
		want: apis.ErrInvalidValue("json: cannot unmarshal string into Go value of type map[string]interface {}", "build"),
	}, {
		name: "no revision template",
		c: &ConfigurationSpec{
			Build: &RawExtension{
				BuildSpec: &buildv1alpha1.BuildSpec{
					Steps: []corev1.Container{{
						Image: "foo",
					}},
				},
			},
		},
		want: apis.ErrMissingOneOf("revisionTemplate", "template"),
	}, {
		name: "too many revision templates",
		c: &ConfigurationSpec{
			RevisionTemplate: &RevisionTemplateSpec{
				Spec: RevisionSpec{
					Container: &corev1.Container{
						Image: "hellworld",
					},
				},
			},
			Template: &RevisionTemplateSpec{
				Spec: RevisionSpec{
					Container: &corev1.Container{
						Image: "hellworld",
					},
				},
			},
		},
		want: apis.ErrMultipleOneOf("revisionTemplate", "template"),
	}, {
		name: "just template",
		c: &ConfigurationSpec{
			Template: &RevisionTemplateSpec{
				Spec: RevisionSpec{
					Container: &corev1.Container{
						Image: "hellworld",
					},
				},
			},
		},
		want: nil,
	}, {
		name: "just template (don't allow deprecated fields)",
		c: &ConfigurationSpec{
			Template: &RevisionTemplateSpec{
				Spec: RevisionSpec{
					DeprecatedConcurrencyModel: "Multi",
					Container: &corev1.Container{
						Image: "hellworld",
					},
				},
			},
		},
		want: apis.ErrDisallowedFields("template.spec.concurrencyModel"),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.c.Validate(context.Background())
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateContainer (-want, +got) = %v", diff)
			}
		})
	}
}

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
				RevisionTemplate: &RevisionTemplateSpec{
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Image: "hellworld",
						},
					},
				},
			},
		},
		want: nil,
	}, {
		name: "propagate revision failures",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: ConfigurationSpec{
				RevisionTemplate: &RevisionTemplateSpec{
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Name:  "stuart",
							Image: "hellworld",
						},
					},
				},
			},
		},
		want: apis.ErrDisallowedFields("spec.revisionTemplate.spec.container.name"),
	}, {
		name: "propagate revision failures (template)",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
			Spec: ConfigurationSpec{
				Template: &RevisionTemplateSpec{
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Name:  "stuart",
							Image: "hellworld",
						},
					},
				},
			},
		},
		want: apis.ErrDisallowedFields("spec.template.spec.container.name"),
	}, {
		name: "empty spec",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid",
			},
		},
		want: apis.ErrMissingField("spec"),
	}, {
		name: "invalid name - dots",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "do.not.use.dots",
			},
		},
		want: (&apis.FieldError{
			Message: "not a DNS 1035 label: [a DNS-1035 label must consist of lower case alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character (e.g. 'my-name',  or 'abc-123', regex used for validation is '[a-z]([-a-z0-9]*[a-z0-9])?')]",
			Paths:   []string{"metadata.name"},
		}).Also(apis.ErrMissingField("spec")),
	}, {
		name: "invalid name - too long",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: strings.Repeat("a", 64),
			},
		},
		want: (&apis.FieldError{
			Message: "not a DNS 1035 label: [must be no more than 63 characters]",
			Paths:   []string{"metadata.name"},
		}).Also(apis.ErrMissingField("spec")),
	}, {
		name: "valid BYO name",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "byo-name",
			},
			Spec: ConfigurationSpec{
				RevisionTemplate: &RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Image: "hellworld",
						},
					},
				},
			},
		},
		want: nil,
	}, {
		name: "invalid BYO name (with generateName)",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "byo-name-",
			},
			Spec: ConfigurationSpec{
				RevisionTemplate: &RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Image: "hellworld",
						},
					},
				},
			},
		},
		want: apis.ErrDisallowedFields("spec.revisionTemplate.metadata.name"),
	}, {
		name: "invalid BYO name (not prefixed)",
		c: &Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "byo-name",
			},
			Spec: ConfigurationSpec{
				RevisionTemplate: &RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Image: "hellworld",
						},
					},
				},
			},
		},
		want: apis.ErrInvalidValue(`"foo" must have prefix "byo-name-"`,
			"spec.revisionTemplate.metadata.name"),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.c.Validate(context.Background())
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateContainer (-want, +got) = %v", diff)
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
				RevisionTemplate: &RevisionTemplateSpec{
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Image: "helloworld:foo",
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
				RevisionTemplate: &RevisionTemplateSpec{
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Image: "helloworld:bar",
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
				RevisionTemplate: &RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Image: "helloworld:foo",
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
				RevisionTemplate: &RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-bar",
					},
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Image: "helloworld:bar",
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
				RevisionTemplate: &RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Image: "helloworld:foo",
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
				RevisionTemplate: &RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Image: "helloworld:foo",
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
				RevisionTemplate: &RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Image: "helloworld:foo",
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
				RevisionTemplate: &RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "byo-name-foo",
					},
					Spec: RevisionSpec{
						Container: &corev1.Container{
							Image: "helloworld:bar",
						},
					},
				},
			},
		},
		want: &apis.FieldError{
			Message: "Saw the following changes without a name change (-old +new)",
			Paths:   []string{"spec.revisionTemplate"},
			Details: "{*v1alpha1.RevisionTemplateSpec}.Spec.Container.Image:\n\t-: \"helloworld:bar\"\n\t+: \"helloworld:foo\"\n",
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
