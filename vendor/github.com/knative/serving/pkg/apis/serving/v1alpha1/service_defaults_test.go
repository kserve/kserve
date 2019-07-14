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
	"github.com/knative/pkg/ptr"
	corev1 "k8s.io/api/core/v1"

	"github.com/knative/serving/pkg/apis/config"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
)

func TestServiceDefaulting(t *testing.T) {
	tests := []struct {
		name string
		in   *Service
		want *Service
	}{{
		name: "empty",
		in:   &Service{},
		// Do nothing when no type is provided
		want: &Service{},
	}, {
		name: "manual",
		in: &Service{
			Spec: ServiceSpec{
				Manual: &ManualType{},
			},
		},
		// Manual does not take a configuration so do nothing
		want: &Service{
			Spec: ServiceSpec{
				Manual: &ManualType{},
			},
		},
	}, {
		name: "run latest",
		in: &Service{
			Spec: ServiceSpec{
				RunLatest: &RunLatestType{
					Configuration: ConfigurationSpec{
						RevisionTemplate: &RevisionTemplateSpec{
							Spec: RevisionSpec{
								Container: &corev1.Container{},
							},
						},
					},
				},
			},
		},
		want: &Service{
			Spec: ServiceSpec{
				RunLatest: &RunLatestType{
					Configuration: ConfigurationSpec{
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
			},
		},
	}, {
		name: "run latest - no overwrite",
		in: &Service{
			Spec: ServiceSpec{
				RunLatest: &RunLatestType{
					Configuration: ConfigurationSpec{
						RevisionTemplate: &RevisionTemplateSpec{
							Spec: RevisionSpec{
								Container: &corev1.Container{},
								RevisionSpec: v1beta1.RevisionSpec{
									ContainerConcurrency: 1,
									TimeoutSeconds:       ptr.Int64(config.DefaultRevisionTimeoutSeconds),
								},
							},
						},
					},
				},
			},
		},
		want: &Service{
			Spec: ServiceSpec{
				RunLatest: &RunLatestType{
					Configuration: ConfigurationSpec{
						RevisionTemplate: &RevisionTemplateSpec{
							Spec: RevisionSpec{
								RevisionSpec: v1beta1.RevisionSpec{
									ContainerConcurrency: 1,
									TimeoutSeconds:       ptr.Int64(config.DefaultRevisionTimeoutSeconds),
								},
								Container: &corev1.Container{
									Resources: defaultResources,
								},
							},
						},
					},
				},
			},
		},
	}, {
		name: "pinned",
		in: &Service{
			Spec: ServiceSpec{
				DeprecatedPinned: &PinnedType{
					Configuration: ConfigurationSpec{
						RevisionTemplate: &RevisionTemplateSpec{
							Spec: RevisionSpec{
								Container: &corev1.Container{},
							},
						},
					},
				},
			},
		},
		want: &Service{
			Spec: ServiceSpec{
				DeprecatedPinned: &PinnedType{
					Configuration: ConfigurationSpec{
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
			},
		},
	}, {
		name: "pinned - no overwrite",
		in: &Service{
			Spec: ServiceSpec{
				DeprecatedPinned: &PinnedType{
					Configuration: ConfigurationSpec{
						RevisionTemplate: &RevisionTemplateSpec{
							Spec: RevisionSpec{
								Container: &corev1.Container{},
								RevisionSpec: v1beta1.RevisionSpec{
									ContainerConcurrency: 1,
									TimeoutSeconds:       ptr.Int64(99),
								},
							},
						},
					},
				},
			},
		},
		want: &Service{
			Spec: ServiceSpec{
				DeprecatedPinned: &PinnedType{
					Configuration: ConfigurationSpec{
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
			},
		},
	}, {
		name: "release",
		in: &Service{
			Spec: ServiceSpec{
				Release: &ReleaseType{
					Configuration: ConfigurationSpec{
						RevisionTemplate: &RevisionTemplateSpec{
							Spec: RevisionSpec{
								Container: &corev1.Container{},
							},
						},
					},
				},
			},
		},
		want: &Service{
			Spec: ServiceSpec{
				Release: &ReleaseType{
					Configuration: ConfigurationSpec{
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
			},
		},
	}, {
		name: "release - no overwrite",
		in: &Service{
			Spec: ServiceSpec{
				Release: &ReleaseType{
					Configuration: ConfigurationSpec{
						RevisionTemplate: &RevisionTemplateSpec{
							Spec: RevisionSpec{
								Container: &corev1.Container{},
								RevisionSpec: v1beta1.RevisionSpec{
									ContainerConcurrency: 1,
									TimeoutSeconds:       ptr.Int64(99),
								},
							},
						},
					},
				},
			},
		},
		want: &Service{
			Spec: ServiceSpec{
				Release: &ReleaseType{
					Configuration: ConfigurationSpec{
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
