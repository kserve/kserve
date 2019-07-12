package v1alpha1

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// Validate both cluster build template and build template
func TestValidateClusterBuildTemplate(t *testing.T) {
	hasDefault := "has-default"
	for _, c := range []struct {
		desc   string
		tmpl   BuildTemplateSpec
		reason string // if "", expect success.
	}{{
		desc: "Single named step",
		tmpl: BuildTemplateSpec{
			Steps: []corev1.Container{{
				Name:  "foo",
				Image: "gcr.io/foo-bar/baz:latest",
			}},
		},
	}, {
		desc: "Multiple unnamed steps",
		tmpl: BuildTemplateSpec{
			Steps: []corev1.Container{{
				Image: "gcr.io/foo-bar/baz:latest",
			}, {
				Image: "gcr.io/foo-bar/baz:latest",
			}, {
				Image: "gcr.io/foo-bar/baz:latest",
			}},
		},
	}, {
		tmpl: BuildTemplateSpec{
			Steps: []corev1.Container{{
				Name:  "foo",
				Image: "gcr.io/foo-bar/baz:latest",
			}, {
				Name:  "foo",
				Image: "gcr.io/foo-bar/baz:oops",
			}},
		},
		reason: "DuplicateStepName",
	}, {
		tmpl: BuildTemplateSpec{
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
		reason: "DuplicateVolumeName",
	}, {
		tmpl: BuildTemplateSpec{
			Parameters: []ParameterSpec{{
				Name: "foo",
			}, {
				Name: "foo",
			}},
		},
		reason: "DuplicateParamName",
	}, {
		tmpl: BuildTemplateSpec{
			Steps: []corev1.Container{{
				Name: "step-name-${FOO${BAR}}",
			}},
			Parameters: []ParameterSpec{{
				Name: "FOO",
			}, {
				Name:    "BAR",
				Default: &hasDefault,
			}},
		},
		reason: "NestedPlaceholder",
	}} {
		name := c.desc
		if c.reason != "" {
			name = "invalid-" + c.reason
		}

		t.Run("namespaced-"+c.desc, func(t *testing.T) {
			testTmpl := &BuildTemplate{Spec: c.tmpl}
			verr := testTmpl.Validate(context.Background())
			if gotErr, wantErr := verr != nil, c.reason != ""; gotErr != wantErr {
				t.Errorf("validateBuildTemplate(%s); got %v, want %q", name, verr, c.reason)
			}
		})

		t.Run("cluster-namespaced-"+c.desc, func(t *testing.T) {
			testClusterTmpl := &ClusterBuildTemplate{Spec: c.tmpl}
			verr := testClusterTmpl.Validate(context.Background())

			if gotErr, wantErr := verr != nil, c.reason != ""; gotErr != wantErr {
				t.Errorf("validateClusterBuildTemplate(%s); got %v, want %q", name, verr, c.reason)
			}
		})
	}
}
