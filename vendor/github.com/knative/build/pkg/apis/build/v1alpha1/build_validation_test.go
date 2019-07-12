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
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateBuild(t *testing.T) {
	for _, c := range []struct {
		name  string
		build *Build
		want  *apis.FieldError
	}{{
		name: "Valid Container",
		build: &Build{
			Spec: BuildSpec{
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: nil,
	}, {
		name: "Both source and sources present",
		build: &Build{
			Spec: BuildSpec{
				Sources: []SourceSpec{{
					Name: "sources",
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
				}},
				Source: &SourceSpec{
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
				},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: apis.ErrMultipleOneOf("spec.source", "spec.sources"),
	}, {
		name: "Source defined without a name",
		build: &Build{
			Spec: BuildSpec{
				Sources: []SourceSpec{{
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
				}},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: nil,
	}, {
		name: "source with targetPath",
		build: &Build{
			Spec: BuildSpec{
				Sources: []SourceSpec{{
					TargetPath: "/path/a/b",
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
				}},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: nil,
	}, {
		name: "Multiple sources with empty targetPaths",
		build: &Build{
			Spec: BuildSpec{
				Sources: []SourceSpec{{
					Name:       "gitpathab",
					TargetPath: "/path/a/b",
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
				}, {
					Name: "gcsnopath1",
					GCS: &GCSSourceSpec{
						Type:     GCSArchive,
						Location: "blah",
					},
				}, {
					Name: "gcsnopath", // 2 sources with empty target path
					GCS: &GCSSourceSpec{
						Type:     GCSArchive,
						Location: "blah",
					},
				}},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: apis.ErrInvalidValue("Empty Target Path", "targetPath").ViaField("sources").ViaField("spec"),
	}, {
		name: "Defining a targetPath while using a custom source",
		build: &Build{
			Spec: BuildSpec{
				Sources: []SourceSpec{{
					Name:       "customwithpath",
					TargetPath: "a/b",
					Custom: &corev1.Container{
						Image: "something:latest",
					},
				}},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: apis.ErrInvalidValue("a/b", "targetPath").ViaField("sources").ViaField("spec"),
	}, {
		name: "Multiple custom sources without a targetPath",
		build: &Build{
			Spec: BuildSpec{
				Sources: []SourceSpec{{
					Name: "customwithpath",
					Custom: &corev1.Container{
						Image: "something:latest",
					},
				}, {
					Name: "customwithpath1",
					Custom: &corev1.Container{
						Image: "something:latest",
					},
				}},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
	}, {
		name: "Sources with targetPaths that overlap with a common parent directory",
		build: &Build{
			Spec: BuildSpec{
				Sources: []SourceSpec{{
					Name:       "gitpathab",
					TargetPath: "/a/b",
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
				}, {
					Name:       "gcsnonestedpath",
					TargetPath: "/a/b/c",
					GCS: &GCSSourceSpec{
						Type:     GCSArchive,
						Location: "blah",
					},
				}},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: &apis.FieldError{
			Message: "Overlapping Target Paths",
			Paths:   []string{"spec.sources.targetPath"},
		},
	}, {
		name: "Sources with combination of individual targetpath",
		build: &Build{
			Spec: BuildSpec{
				Sources: []SourceSpec{{
					Name:       "gitpathab",
					TargetPath: "basel",
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
				}, {
					Name:       "gcsnonestedpath",
					TargetPath: "baselrocks",
					GCS: &GCSSourceSpec{
						Type:     GCSArchive,
						Location: "blah",
					},
				}},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: nil,
	}, {
		name: "Mix of sources with and without target path",
		build: &Build{
			Spec: BuildSpec{
				Sources: []SourceSpec{{
					Name:       "gitpathab",
					TargetPath: "gitpath",
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
				}, {
					Name: "gcsnopath1",
					GCS: &GCSSourceSpec{
						Type:     GCSArchive,
						Location: "blah",
					},
				}, {
					Name:       "gcswithpath",
					TargetPath: "gcs",
					GCS: &GCSSourceSpec{
						Type:     GCSArchive,
						Location: "blah",
					},
				}},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: nil,
	}, {
		name: "Multiple sources with duplicate names",
		build: &Build{
			Spec: BuildSpec{
				Sources: []SourceSpec{{
					Name: "sname",
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
				}, {
					Name: "sname",
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
				}},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: apis.ErrMultipleOneOf("spec.sources.name"),
	}, {
		name: "Source with a subpath",
		build: &Build{
			Spec: BuildSpec{
				Sources: []SourceSpec{{
					Name: "sname",
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
					SubPath: "go",
				}},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: nil,
	}, {
		name: "Multiple sources with subpaths",
		build: &Build{
			Spec: BuildSpec{
				Sources: []SourceSpec{{
					Name: "sname",
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
					SubPath: "go",
				}, {
					Name: "anothername",
					Git: &GitSourceSpec{
						Url:      "someurl",
						Revision: "revision",
					},
					SubPath: "ruby",
				}},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: apis.ErrMultipleOneOf("spec.sources.subpath"),
	}, {
		name: "Negative build timeout",
		build: &Build{
			Spec: BuildSpec{
				Timeout: &metav1.Duration{Duration: -48 * time.Hour},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: apis.ErrOutOfBoundsValue("-48h0m0s", "0", "24", "spec.timeout"),
	}, {
		name: "No template and steps",
		build: &Build{
			Spec: BuildSpec{},
		},
		want: apis.ErrMissingOneOf("spec.template", "spec.steps"),
	}, {
		name: "Invalid template Kind",
		build: &Build{
			Spec: BuildSpec{
				Template: &TemplateInstantiationSpec{
					Kind: "bad-kind",
					Name: "bad-tmpl",
				},
			},
		},
		want: apis.ErrInvalidValue("bad-kind", "spec.template.kind"),
	}, {
		name: "Valid template Kind",
		build: &Build{
			Spec: BuildSpec{
				Template: &TemplateInstantiationSpec{
					Kind: ClusterBuildTemplateKind,
					Name: "goo-tmpl",
				},
			},
		},
		want: nil,
	}, {
		name: "Greater than maximum build timeout",
		build: &Build{
			Spec: BuildSpec{
				Timeout: &metav1.Duration{Duration: 48 * time.Hour},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: apis.ErrOutOfBoundsValue("48h0m0s", "0", "24", "spec.timeout"),
	}, {
		name: "5 minute build timeout",
		build: &Build{
			Spec: BuildSpec{
				Timeout: &metav1.Duration{Duration: 5 * time.Minute},
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
		want: nil,
	}, {
		name: "Multiple unnamed steps",
		build: &Build{
			Spec: BuildSpec{
				Steps: []corev1.Container{{
					Image: "gcr.io/foo-bar/baz:latest",
				}, {
					Image: "gcr.io/foo-bar/baz:latest",
				}, {
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
	}, {
		name: "Multiple steps with the same name",
		build: &Build{
			Spec: BuildSpec{
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}, {
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:oops",
				}},
			},
		},
		want: apis.ErrMultipleOneOf("spec.steps.name"),
	}, {
		name: "Missing step image",
		build: &Build{
			Spec: BuildSpec{
				Steps: []corev1.Container{{Name: "foo"}},
			},
		},
		want: apis.ErrMissingField("spec.steps.Image"),
	}, {
		name: "Multiple volumes with the same name",
		build: &Build{
			Spec: BuildSpec{
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
				Volumes: []corev1.Volume{{
					Name: "foo",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				}, {
					Name: "foo",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				}},
			},
		},
		want: apis.ErrMultipleOneOf("spec.volumes.name"),
	}, {
		name: "Template and Steps both defined",
		build: &Build{
			Spec: BuildSpec{
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
				Template: &TemplateInstantiationSpec{
					Name: "template",
				},
			},
		},
		want: apis.ErrMultipleOneOf("spec.template", "spec.steps"),
	}, {
		name: "No template name defined",
		build: &Build{
			Spec: BuildSpec{
				Template: &TemplateInstantiationSpec{},
			},
		},
		want: apis.ErrMissingField("spec.template.name"),
	}} {
		name := c.name
		t.Run(name, func(t *testing.T) {
			got := c.build.Validate(context.Background())
			if diff := cmp.Diff(c.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateBuild(%s) (-want, +got) = %v", name, diff)
			}
		})
	}
}
