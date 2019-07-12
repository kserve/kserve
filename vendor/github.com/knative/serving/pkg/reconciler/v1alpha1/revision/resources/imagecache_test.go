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
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	caching "github.com/knative/caching/pkg/apis/caching/v1alpha1"
	"github.com/knative/pkg/ptr"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
)

func TestMakeImageCache(t *testing.T) {
	tests := []struct {
		name string
		rev  *v1alpha1.Revision
		want *caching.Image
	}{{
		name: "simple container",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				Annotations: map[string]string{
					"a":                                     "b",
					serving.RevisionLastPinnedAnnotationKey: "c",
				},
				UID: "1234",
			},
			Spec: v1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					ContainerConcurrency: 1,
				},
				Container: &corev1.Container{
					Image: "busybox",
				},
			},
			Status: v1alpha1.RevisionStatus{
				ImageDigest: "busybox@sha256:deadbeef",
			},
		},
		want: &caching.Image{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar-cache",
				Labels: map[string]string{
					serving.RevisionLabelKey: "bar",
					serving.RevisionUID:      "1234",
					AppLabelKey:              "bar",
				},
				Annotations: map[string]string{
					"a": "b",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         v1alpha1.SchemeGroupVersion.String(),
					Kind:               "Revision",
					Name:               "bar",
					UID:                "1234",
					Controller:         ptr.Bool(true),
					BlockOwnerDeletion: ptr.Bool(true),
				}},
			},
			Spec: caching.ImageSpec{
				Image: "busybox@sha256:deadbeef",
			},
		},
	}, {
		name: "with service account",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
			},
			Spec: v1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					ContainerConcurrency: 1,
					PodSpec: v1beta1.PodSpec{
						ServiceAccountName: "privilegeless",
					},
				},
				Container: &corev1.Container{
					Image: "busybox",
				},
			},
		},
		want: &caching.Image{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar-cache",
				Labels: map[string]string{
					serving.RevisionLabelKey: "bar",
					serving.RevisionUID:      "1234",
					AppLabelKey:              "bar",
				},
				Annotations: map[string]string{},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         v1alpha1.SchemeGroupVersion.String(),
					Kind:               "Revision",
					Name:               "bar",
					UID:                "1234",
					Controller:         ptr.Bool(true),
					BlockOwnerDeletion: ptr.Bool(true),
				}},
			},
			Spec: caching.ImageSpec{
				Image:              "busybox",
				ServiceAccountName: "privilegeless",
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MakeImageCache(test.rev)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("MakeImageCache (-want, +got) = %v", diff)
			}
		})
	}
}
