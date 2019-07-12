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
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/pkg/ptr"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/revision/config"
)

func TestMakeFluentdConfigMap(t *testing.T) {
	tests := []struct {
		name string
		rev  *v1alpha1.Revision
		oc   *config.Observability
		want *corev1.ConfigMap
	}{{
		name: "empty config",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
				Annotations: map[string]string{
					"a":                                     "b",
					serving.RevisionLastPinnedAnnotationKey: "c",
				},
			},
		},
		oc: &config.Observability{},
		want: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar-fluentd",
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
			Data: map[string]string{
				"varlog.conf": fluentdSidecarPreOutputConfig,
			},
		},
	}, {
		name: "non-empty config",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
			},
		},
		oc: &config.Observability{
			FluentdSidecarOutputConfig: "foo bar config",
		},
		want: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar-fluentd",
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
			Data: map[string]string{
				"varlog.conf": fluentdSidecarPreOutputConfig + "foo bar config",
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MakeFluentdConfigMap(test.rev, test.oc)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("MakeFluentdConfigMap (-want, +got) = %v", diff)
			}
		})
	}
}

func TestMakeFluentdConfigMapVolume(t *testing.T) {
	tests := []struct {
		name string
		rev  *v1alpha1.Revision
		want *corev1.Volume
	}{{
		name: "happy path",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
			},
		},
		want: &corev1.Volume{
			Name: fluentdConfigMapVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "bar-fluentd",
					},
				},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := makeFluentdConfigMapVolume(test.rev)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("makeFluentdConfigMapVolume (-want, +got) = %v", diff)
			}
		})
	}
}

func TestMakeFluentdContainer(t *testing.T) {
	tests := []struct {
		name string
		rev  *v1alpha1.Revision
		oc   *config.Observability
		want *corev1.Container
	}{{
		name: "no owner no observability config",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
			},
		},
		oc: &config.Observability{},
		want: &corev1.Container{
			// These are effectively constant
			Name:      FluentdContainerName,
			Resources: fluentdResources,
			Image:     "",
			// These changed based on the Revision and configs passed in.
			Env: []corev1.EnvVar{{
				Name:  "FLUENTD_ARGS",
				Value: "--no-supervisor -q",
			}, {
				Name:  "SERVING_CONTAINER_NAME",
				Value: UserContainerName, // matches name
			}, {
				Name:  "SERVING_CONFIGURATION",
				Value: "", // No OwnerReference
			}, {
				Name:  "SERVING_REVISION",
				Value: "bar",
			}, {
				Name:  "SERVING_NAMESPACE",
				Value: "foo", // matches namespace
			}, {
				Name:      "SERVING_POD_NAME",
				ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}},
			}},
			VolumeMounts: fluentdVolumeMounts,
		},
	}, {
		name: "owner no obdervability",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         v1alpha1.SchemeGroupVersion.String(),
					Kind:               "Configuration",
					Name:               "the-parent-config-name",
					Controller:         ptr.Bool(true),
					BlockOwnerDeletion: ptr.Bool(true),
				}},
			},
		},
		oc: &config.Observability{},
		want: &corev1.Container{
			// These are effectively constant
			Name:      FluentdContainerName,
			Resources: fluentdResources,
			Image:     "",
			// These changed based on the Revision and configs passed in.
			Env: []corev1.EnvVar{{
				Name:  "FLUENTD_ARGS",
				Value: "--no-supervisor -q",
			}, {
				Name:  "SERVING_CONTAINER_NAME",
				Value: UserContainerName, // matches name
			}, {
				Name:  "SERVING_CONFIGURATION",
				Value: "the-parent-config-name", // With OwnerReference
			}, {
				Name:  "SERVING_REVISION",
				Value: "bar",
			}, {
				Name:  "SERVING_NAMESPACE",
				Value: "foo", // matches namespace
			}, {
				Name:      "SERVING_POD_NAME",
				ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}},
			}},
			VolumeMounts: fluentdVolumeMounts,
		},
	}, {
		name: "no owner with obdervability options",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
			},
		},
		oc: &config.Observability{
			FluentdSidecarImage: "some-test-image",
		},
		want: &corev1.Container{
			// These are effectively constant
			Name:      FluentdContainerName,
			Resources: fluentdResources,
			Image:     "some-test-image",
			// These changed based on the Revision and configs passed in.
			Env: []corev1.EnvVar{{
				Name:  "FLUENTD_ARGS",
				Value: "--no-supervisor -q",
			}, {
				Name:  "SERVING_CONTAINER_NAME",
				Value: UserContainerName, // matches name
			}, {
				Name:  "SERVING_CONFIGURATION",
				Value: "", // no OwnerReference
			}, {
				Name:  "SERVING_REVISION",
				Value: "bar",
			}, {
				Name:  "SERVING_NAMESPACE",
				Value: "foo", // matches namespace
			}, {
				Name:      "SERVING_POD_NAME",
				ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}},
			}},
			VolumeMounts: fluentdVolumeMounts,
		},
	},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := makeFluentdContainer(test.rev, test.oc)
			if diff := cmp.Diff(test.want, got, cmpopts.IgnoreUnexported(resource.Quantity{})); diff != "" {
				t.Errorf("makeFluentdContainer (-want, +got) = %v", diff)
			}
		})
	}
}
