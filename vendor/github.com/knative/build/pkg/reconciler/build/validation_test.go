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
package build

import (
	"testing"

	"github.com/knative/build/pkg/apis/build/v1alpha1"
	fakebuildclientset "github.com/knative/build/pkg/client/clientset/versioned/fake"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
)

func TestValidateBuild(t *testing.T) {
	hasDefault := "has-default"
	empty := ""
	for _, c := range []struct {
		desc    string
		build   *v1alpha1.Build
		tmpl    *v1alpha1.BuildTemplate
		ctmpl   *v1alpha1.ClusterBuildTemplate
		sa      *corev1.ServiceAccount
		secrets []*corev1.Secret
		reason  string // if "", expect success.
	}{{
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "foo",
						Value: "hello",
					}, {
						Name:  "foo",
						Value: "world",
					}},
				},
			},
			Status: v1alpha1.BuildStatus{
				Cluster: &v1alpha1.ClusterSpec{
					PodName: "foo",
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "foo",
				}},
			},
		},
		reason: "DuplicateArgName",
	}, {
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Name: "foo-bar",
				},
				Volumes: []corev1.Volume{{
					Name: "foo",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				}},
			},
			Status: v1alpha1.BuildStatus{
				Cluster: &v1alpha1.ClusterSpec{
					PodName: "foo",
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
				Volumes: []corev1.Volume{{
					Name: "foo",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				}},
			},
		},
		reason: "DuplicateVolumeNameForBuildTemplate",
	}, {
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Name: "foo-bar",
					Kind: v1alpha1.ClusterBuildTemplateKind,
				},
			},
			Status: v1alpha1.BuildStatus{
				Cluster: &v1alpha1.ClusterSpec{
					PodName: "foo",
				},
			},
		},
		ctmpl: &v1alpha1.ClusterBuildTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "foo-bar"},
			Spec: v1alpha1.BuildTemplateSpec{
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
		reason: "DuplicateVolumeNameForClusterBuildTemplate",
	}, {
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
				Template: &v1alpha1.TemplateInstantiationSpec{
					Name: "template",
				},
			},
			Status: v1alpha1.BuildStatus{
				Cluster: &v1alpha1.ClusterSpec{
					PodName: "foo",
				},
			},
		},
		reason: "TemplateAndSteps",
	}, {
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Name: "template",
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "foo",
						Value: "hello",
					}},
				},
			},
			Status: v1alpha1.BuildStatus{
				Cluster: &v1alpha1.ClusterSpec{
					PodName: "foo",
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "template"},
			Spec: v1alpha1.BuildTemplateSpec{
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "foo",
				}, {
					Name: "bar",
				}},
			},
		},
		reason: "UnsatisfiedParameter",
	}, {
		desc: "Arg doesn't match any parameter",
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Name: "template",
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "bar",
						Value: "hello",
					}},
				},
			},
			Status: v1alpha1.BuildStatus{
				Cluster: &v1alpha1.ClusterSpec{
					PodName: "foo",
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "template"},
			Spec:       v1alpha1.BuildTemplateSpec{},
		},
	}, {
		desc: "Unsatisfied parameter has a default",
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Name: "template",
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "foo",
						Value: "hello",
					}},
				},
			},
			Status: v1alpha1.BuildStatus{
				Cluster: &v1alpha1.ClusterSpec{
					PodName: "foo",
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "template"},
			Spec: v1alpha1.BuildTemplateSpec{
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "foo",
				}, {
					Name:    "bar",
					Default: &hasDefault,
				}},
			},
		},
	}, {
		desc: "Unsatisfied parameter has empty default",
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Name: "empty-default",
					Kind: "BuildTemplate",
				},
			},
			Status: v1alpha1.BuildStatus{
				Cluster: &v1alpha1.ClusterSpec{
					PodName: "foo",
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-default"},
			Spec: v1alpha1.BuildTemplateSpec{
				Parameters: []v1alpha1.ParameterSpec{{
					Name:    "foo",
					Default: &empty,
				}},
			},
		},
	}, {
		desc: "build template cluster",
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Name: "empty-default",
					Kind: "ClusterBuildTemplate",
				},
			},
			Status: v1alpha1.BuildStatus{
				Cluster: &v1alpha1.ClusterSpec{
					PodName: "foo",
				},
			},
		},
		ctmpl: &v1alpha1.ClusterBuildTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-default"},
			Spec: v1alpha1.BuildTemplateSpec{
				Parameters: []v1alpha1.ParameterSpec{{
					Name:    "foo",
					Default: &empty,
				}},
			},
		},
	}, {
		desc: "Acceptable secret annotations",
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				// ServiceAccountName will default to "default"
				Steps: []corev1.Container{{Image: "hello"}},
			},
			Status: v1alpha1.BuildStatus{
				Cluster: &v1alpha1.ClusterSpec{
					PodName: "foo",
				},
			},
		},
		sa: &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Secrets: []corev1.ObjectReference{
				{Name: "good-sekrit"},
				{Name: "another-good-sekrit"},
				{Name: "one-more-good-sekrit"},
				{Name: "last-one-promise"},
			},
		},
		secrets: []*corev1.Secret{{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "good-sekrit",
				Annotations: map[string]string{"build.knative.dev/docker-0": "https://index.docker.io/v1/"},
			},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:        "another-good-sekrit",
				Annotations: map[string]string{"unrelated": "index.docker.io"},
			},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:        "one-more-good-sekrit",
				Annotations: map[string]string{"build.knative.dev/docker-1": "gcr.io"},
			},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:        "last-one-promise",
				Annotations: map[string]string{"docker-0": "index.docker.io"},
			},
		}},
	}, {
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				ServiceAccountName: "serviceaccount",
				Steps:              []corev1.Container{{Image: "hello"}},
			},
			Status: v1alpha1.BuildStatus{
				Cluster: &v1alpha1.ClusterSpec{
					PodName: "foo",
				},
			},
		},
		sa: &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "serviceaccount"},
			Secrets:    []corev1.ObjectReference{{Name: "bad-sekrit"}},
		},
		secrets: []*corev1.Secret{{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "bad-sekrit",
				Annotations: map[string]string{"build.knative.dev/docker-0": "index.docker.io"},
			},
		}},
		reason: "BadSecretAnnotation",
	}} {
		name := c.desc
		if c.reason != "" {
			name = "invalid-" + c.reason
		}
		t.Run(name, func(t *testing.T) {
			client := fakekubeclientset.NewSimpleClientset()
			buildClient := fakebuildclientset.NewSimpleClientset()
			// Create a BuildTemplate.
			if c.tmpl != nil {
				if _, err := buildClient.BuildV1alpha1().BuildTemplates("").Create(c.tmpl); err != nil {
					t.Fatalf("Failed to create BuildTemplate: %v", err)
				}
			} else if c.ctmpl != nil {
				if _, err := buildClient.BuildV1alpha1().ClusterBuildTemplates().Create(c.ctmpl); err != nil {
					t.Fatalf("Failed to create ClusterBuildTemplate: %v", err)
				}
			}
			// Create ServiceAccount or create the default ServiceAccount.
			if c.sa != nil {
				if _, err := client.CoreV1().ServiceAccounts(c.sa.Namespace).Create(c.sa); err != nil {
					t.Fatalf("Failed to create ServiceAccount: %v", err)
				}
			} else {
				if _, err := client.CoreV1().ServiceAccounts("").Create(&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{Name: "default"},
				}); err != nil {
					t.Fatalf("Failed to create ServiceAccount: %v", err)
				}
			}
			// Create any necessary Secrets.
			for _, s := range c.secrets {
				if _, err := client.CoreV1().Secrets("").Create(s); err != nil {
					t.Fatalf("Failed to create Secret %q: %v", s.Name, err)
				}
			}
			testLogger := zap.NewNop().Sugar()

			ac := &Reconciler{
				kubeclientset:  client,
				buildclientset: buildClient,
				Logger:         testLogger,
			}

			verr := ac.validateBuild(c.build)
			if gotErr, wantErr := verr != nil, c.reason != ""; gotErr != wantErr {
				t.Errorf("validateBuild(%s); got %v, want %q", name, verr, c.reason)
			}
		})
	}
}

func TestValidateTemplate(t *testing.T) {
	for _, c := range []struct {
		desc   string
		tmpl   *v1alpha1.BuildTemplate
		reason string // if "", expect success.
	}{{
		desc: "Single named step",
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name:  "foo",
					Image: "gcr.io/foo-bar/baz:latest",
				}},
			},
		},
	}, {
		desc: "Multiple unnamed steps",
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
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
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "step-name-${FOO${BAR}}",
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "FOO",
				}},
			},
		},
		reason: "NestedPlaceholder",
	}, {
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "step-name",
					Args: []string{"step-name-${FOO${BAR}}"},
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "FOO",
				}},
			},
		},
		reason: "NestedPlaceholder",
	}, {
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "step-name",
					Env: []corev1.EnvVar{{
						Value: "step-name-${FOO${BAR}}",
					}},
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "FOO",
				}},
			},
		},
		reason: "NestedPlaceholder",
	}, {
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name:       "step-name",
					WorkingDir: "step-name-${FOO${BAR}}",
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "FOO",
				}},
			},
		},
		reason: "NestedPlaceholder",
	}, {
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name:    "step-name",
					Command: []string{"step-name-${FOO${BAR}}"},
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "FOO",
				}},
			},
		},
		reason: "NestedPlaceholder",
	}} {
		name := c.desc
		if c.reason != "" {
			name = "invalid-" + c.reason
		}
		t.Run(name, func(t *testing.T) {
			verr := validateTemplate(c.tmpl)
			if gotErr, wantErr := verr != nil, c.reason != ""; gotErr != wantErr {
				t.Errorf("validateTemplate(%s); got %v, want %q", name, verr, c.reason)
			}
		})
	}
}
