// +build e2e

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

package e2e

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/build/pkg/apis/build/v1alpha1"
)

// TestInvalidBuild tests that invalid builds are rejected by the webhook
// admission controller.
func TestInvalidBuild(t *testing.T) {
	buildTestNamespace, clients := initialize(t)
	defer teardownNamespace(t, clients, buildTestNamespace)

	for _, b := range []*v1alpha1.Build{{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      "name-too-long" + strings.Repeat("a", 1000),
		},
		Spec: v1alpha1.BuildSpec{
			Steps: []corev1.Container{{Image: "busybox"}},
		},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      "name.contains.dots",
		},
		Spec: v1alpha1.BuildSpec{
			Steps: []corev1.Container{{Image: "busybox"}},
		},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      "no-steps",
		},
		Spec: v1alpha1.BuildSpec{
			Steps: []corev1.Container{},
		},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      "negative-timeout",
		},
		Spec: v1alpha1.BuildSpec{
			Steps:   []corev1.Container{{Image: "busybox"}},
			Timeout: &metav1.Duration{time.Duration(-1 * time.Hour)},
		},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      "too-long-timeout",
		},
		Spec: v1alpha1.BuildSpec{
			Steps:   []corev1.Container{{Image: "busybox"}},
			Timeout: &metav1.Duration{time.Duration(36 * time.Hour)},
		},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      "source-with-duplicate-targetpath",
		},
		Spec: v1alpha1.BuildSpec{
			Sources: []v1alpha1.SourceSpec{{
				TargetPath: "paht/foo",
				Git: &v1alpha1.GitSourceSpec{
					Url:      "some-url",
					Revision: "master",
				},
			}, {
				TargetPath: "path/foo",
				Git: &v1alpha1.GitSourceSpec{
					Url:      "some-url",
					Revision: "master",
				},
			}},
			Steps: []corev1.Container{{Image: "busybox"}},
		},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      "source-with-multiple-subpath",
		},
		Spec: v1alpha1.BuildSpec{
			Sources: []v1alpha1.SourceSpec{{
				Name: "source1",
				Git: &v1alpha1.GitSourceSpec{
					Url:      "some-url",
					Revision: "master",
				},
				SubPath: "go",
			}, {
				Name: "source2",
				Git: &v1alpha1.GitSourceSpec{
					Url:      "some-url",
					Revision: "master",
				},
				SubPath: "go2",
			}},
			Steps: []corev1.Container{{Image: "busybox"}},
		},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      "source-and-sources-declared",
		},
		Spec: v1alpha1.BuildSpec{
			Source: &v1alpha1.SourceSpec{
				Name: "source1",
				Git: &v1alpha1.GitSourceSpec{
					Url:      "some-url",
					Revision: "master",
				},
			},
			Sources: []v1alpha1.SourceSpec{{
				Name: "sources1",
				Git: &v1alpha1.GitSourceSpec{
					Url:      "some-url",
					Revision: "master",
				},
			}},
			Steps: []corev1.Container{{Image: "busybox"}},
		},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      "custom-source-with-targetPath",
		},
		Spec: v1alpha1.BuildSpec{
			Sources: []v1alpha1.SourceSpec{{
				Name:       "sources1",
				TargetPath: "a/b",
				Custom: &corev1.Container{
					Image: "ubuntu",
				},
			}},
			Steps: []corev1.Container{{Image: "busybox"}},
		},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      "sources-with-duplicate-names",
		},
		Spec: v1alpha1.BuildSpec{
			Sources: []v1alpha1.SourceSpec{{
				Name: "sources1",
				Custom: &corev1.Container{
					Image: "ubuntu",
				},
			}, {
				Name: "sources1",
				Custom: &corev1.Container{
					Image: "ubuntu",
				},
			}},
			Steps: []corev1.Container{{Image: "busybox"}},
		},
	}} {
		if _, err := clients.buildClient.builds.Create(b); err == nil {
			t.Errorf("Expected error creating invalid build %q, got nil", b.ObjectMeta.Name)
		}
	}
}
