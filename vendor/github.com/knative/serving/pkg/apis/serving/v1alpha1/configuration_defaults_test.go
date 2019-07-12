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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/knative/pkg/ptr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/knative/serving/pkg/apis/config"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
)

var (
	defaultResources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("400m"),
		},
	}
	ignoreUnexportedResources = cmpopts.IgnoreUnexported(resource.Quantity{})
)

func TestConfigurationDefaulting(t *testing.T) {
	tests := []struct {
		name string
		in   *Configuration
		want *Configuration
	}{{
		name: "empty",
		in:   &Configuration{},
		want: &Configuration{},
	}, {
		name: "shell",
		in: &Configuration{
			Spec: ConfigurationSpec{
				RevisionTemplate: &RevisionTemplateSpec{
					Spec: RevisionSpec{
						Container: &corev1.Container{},
					},
				},
			},
		},
		want: &Configuration{
			Spec: ConfigurationSpec{
				RevisionTemplate: &RevisionTemplateSpec{
					Spec: RevisionSpec{
						RevisionSpec: v1beta1.RevisionSpec{
							TimeoutSeconds: ptr.Int64(config.DefaultRevisionTimeoutSeconds),
						},
						Container: &corev1.Container{
							Resources: defaultResources,
						},
					},
				},
			},
		},
	}, {
		name: "shell podspec",
		in: &Configuration{
			Spec: ConfigurationSpec{
				RevisionTemplate: &RevisionTemplateSpec{
					Spec: RevisionSpec{
						RevisionSpec: v1beta1.RevisionSpec{
							PodSpec: v1beta1.PodSpec{
								Containers: []corev1.Container{{}},
							},
						},
					},
				},
			},
		},
		want: &Configuration{
			Spec: ConfigurationSpec{
				RevisionTemplate: &RevisionTemplateSpec{
					Spec: RevisionSpec{
						RevisionSpec: v1beta1.RevisionSpec{
							TimeoutSeconds: ptr.Int64(config.DefaultRevisionTimeoutSeconds),
							PodSpec: v1beta1.PodSpec{
								Containers: []corev1.Container{{
									Resources: defaultResources,
								}},
							},
						},
					},
				},
			},
		},
	}, {
		name: "no overwrite values",
		in: &Configuration{
			Spec: ConfigurationSpec{
				RevisionTemplate: &RevisionTemplateSpec{
					Spec: RevisionSpec{
						RevisionSpec: v1beta1.RevisionSpec{
							ContainerConcurrency: 1,
							TimeoutSeconds:       ptr.Int64(99),
						},
						Container: &corev1.Container{
							Resources: defaultResources,
						},
					},
				},
			},
		},
		want: &Configuration{
			Spec: ConfigurationSpec{
				RevisionTemplate: &RevisionTemplateSpec{
					Spec: RevisionSpec{
						RevisionSpec: v1beta1.RevisionSpec{
							ContainerConcurrency: 1,
							TimeoutSeconds:       ptr.Int64(99),
						},
						Container: &corev1.Container{
							Resources: defaultResources,
						},
					},
				},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.in
			got.SetDefaults(context.Background())
			if diff := cmp.Diff(test.want, got, ignoreUnexportedResources); diff != "" {
				t.Errorf("SetDefaults (-want, +got) = %v", diff)
			}
		})
	}
}
