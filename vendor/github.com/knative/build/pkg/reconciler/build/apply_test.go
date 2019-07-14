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
	"reflect"
	"testing"

	"github.com/knative/build/pkg/apis/build/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func TestApplyTemplate(t *testing.T) {
	world := "world"
	defaultStr := "default"
	empty := ""
	for i, c := range []struct {
		build *v1alpha1.Build
		tmpl  v1alpha1.BuildTemplateInterface
		want  *v1alpha1.Build // if nil, expect error.
	}{{
		// Build's Steps are overwritten. This doesn't pass
		// ValidateBuild anyway.
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name: "from-build",
				}},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "from-template",
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name: "from-template",
				}},
			},
		},
	}, {
		// Volumes from both build and template.
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Volumes: []corev1.Volume{{
					Name: "from-build",
				}},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Volumes: []corev1.Volume{{
					Name: "from-template",
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Volumes: []corev1.Volume{{
					Name: "from-build",
				}, {
					Name: "from-template",
				}},
			},
		},
	}, {
		// Parameter placeholders are filled by arg value in all
		// fields.
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "FOO",
						Value: "world",
					}},
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name:  "hello ${FOO}",
					Image: "busybox:${FOO}",
					Args:  []string{"hello", "to the ${FOO}"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "is ${FOO}",
					}},
					Command:    []string{"cmd", "${FOO}"},
					WorkingDir: "/dir/${FOO}/bar",
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "${FOO}",
						MountPath: "path/to/${FOO}",
						SubPath:   "sub/${FOO}/path",
					}},
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "FOO",
				}},
				Volumes: []corev1.Volume{{
					Name: "${FOO}",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "${FOO}",
						},
					},
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name:  "hello world",
					Image: "busybox:world",
					Args:  []string{"hello", "to the world"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "is world",
					}},
					Command:    []string{"cmd", "world"},
					WorkingDir: "/dir/world/bar",
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "world",
						MountPath: "path/to/world",
						SubPath:   "sub/world/path",
					}},
				}},
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "FOO",
						Value: "world",
					}},
				},
				Volumes: []corev1.Volume{{
					Name: "world",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "world",
						},
					},
				}},
			},
		},
	}, {
		// $-prefixed strings (e.g., env vars in a bash script) are untouched.
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "FOO",
						Value: "world",
					}},
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name:    "ubuntu",
					Command: []string{"bash"},
					Args:    []string{"-c", "echo $BAR ${FOO}"},
					Env: []corev1.EnvVar{{
						Name:  "BAR",
						Value: "terrible",
					}},
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "FOO",
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name:    "ubuntu",
					Command: []string{"bash"},
					Args:    []string{"-c", "echo $BAR world"},
					Env: []corev1.EnvVar{{
						Name:  "BAR",
						Value: "terrible",
					}},
				}},
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "FOO",
						Value: "world",
					}},
				},
			},
		},
	}, {
		// $$-prefixed strings are untouched, even if they conflict with a
		// parameter name.
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "FOO",
						Value: "world",
					}},
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name:    "ubuntu",
					Command: []string{"bash"},
					Args:    []string{"-c", "echo $FOO ${FOO}"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "terrible",
					}},
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "FOO",
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name:    "ubuntu",
					Command: []string{"bash"},
					Args:    []string{"-c", "echo $FOO world"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "terrible",
					}},
				}},
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "FOO",
						Value: "world",
					}},
				},
			},
		},
	}, {
		// Parameter with default value.
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name:  "hello ${FOO}",
					Image: "busybox:${FOO}",
					Args:  []string{"hello", "to the ${FOO}"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "is ${FOO}",
					}},
					Command:    []string{"cmd", "${FOO}"},
					WorkingDir: "/dir/${FOO}/bar",
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name:    "FOO",
					Default: &world,
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name:  "hello world",
					Image: "busybox:world",
					Args:  []string{"hello", "to the world"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "is world",
					}},
					Command:    []string{"cmd", "world"},
					WorkingDir: "/dir/world/bar",
				}},
			},
		},
	}, {
		// Parameter with empty default value.
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "hello ${FOO}",
					Args: []string{"hello", "to the ${FOO}"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "is ${FOO}",
					}},
					Command:    []string{"cmd", "${FOO}"},
					WorkingDir: "/dir/${FOO}/bar",
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name:    "FOO",
					Default: &empty,
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name: "hello ",
					Args: []string{"hello", "to the "},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "is ",
					}},
					Command:    []string{"cmd", ""},
					WorkingDir: "/dir//bar",
				}},
			},
		},
	}, {
		// Parameter with default value, which build overrides.
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "FOO",
						Value: "world",
					}},
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "hello ${FOO}",
					Args: []string{"hello", "to the ${FOO}"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "is ${FOO}",
					}},
					Command:    []string{"cmd", "${FOO}"},
					WorkingDir: "/dir/${FOO}/bar",
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name:    "FOO",
					Default: &defaultStr,
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name: "hello world",
					Args: []string{"hello", "to the world"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "is world",
					}},
					Command:    []string{"cmd", "world"},
					WorkingDir: "/dir/world/bar",
				}},
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "FOO",
						Value: "world",
					}},
				},
			},
		},
	}, {
		// Unsatisfied parameter (no default), so it's not replaced.
		// This doesn't pass ValidateBuild anyway.
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "hello ${FOO}",
					Args: []string{"hello", "to the ${FOO}"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "is ${FOO}",
					}},
					Command:    []string{"cmd", "${FOO}"},
					WorkingDir: "/dir/${FOO}/bar",
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "FOO",
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name: "hello ${FOO}",
					Args: []string{"hello", "to the ${FOO}"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "is ${FOO}",
					}},
					Command:    []string{"cmd", "${FOO}"},
					WorkingDir: "/dir/${FOO}/bar",
				}},
			},
		},
	}, {
		// Build with arg for unknown param (ignored).
		// TODO(jasonhall): Should this be an error?
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "FOO",
						Value: "world",
					}},
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "hello",
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name: "hello",
				}},
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "FOO",
						Value: "world",
					}},
				},
			},
		},
	}, {
		// Template doesn't specify that ${FOO} is a parameter, so it's not
		// replaced.
		// TODO(jasonhall): Should this be an error?
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "hello ${FOO}",
					Args: []string{"hello", "to the ${FOO}"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "is ${FOO}",
					}},
					Command:    []string{"cmd", "${FOO}"},
					WorkingDir: "/dir/${FOO}/bar",
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name: "hello ${FOO}",
					Args: []string{"hello", "to the ${FOO}"},
					Env: []corev1.EnvVar{{
						Name:  "FOO",
						Value: "is ${FOO}",
					}},
					Command:    []string{"cmd", "${FOO}"},
					WorkingDir: "/dir/${FOO}/bar",
				}},
			},
		},
	}, {
		// Malformed placeholders are ignored.
		// TODO(jasonhall): Should this be an error?
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "FOO",
						Value: "world",
					}},
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "hello ${FOO",
				}},
				Parameters: []v1alpha1.ParameterSpec{{
					Name: "FOO",
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name: "hello ${FOO",
				}},
				Template: &v1alpha1.TemplateInstantiationSpec{
					Arguments: []v1alpha1.ArgumentSpec{{
						Name:  "FOO",
						Value: "world",
					}},
				},
			},
		},
	}, {
		// A build's template initiation spec contains
		// env vars
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Env: []corev1.EnvVar{{
						Name:  "SOME_ENV_VAR",
						Value: "foo",
					}},
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "hello",
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name: "hello",
					Env: []corev1.EnvVar{{
						Name:  "SOME_ENV_VAR",
						Value: "foo",
					}},
				}},
				Template: &v1alpha1.TemplateInstantiationSpec{
					Env: []corev1.EnvVar{{
						Name:  "SOME_ENV_VAR",
						Value: "foo",
					}},
				},
			},
		},
	}, {
		// A cluster build template
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Kind: v1alpha1.ClusterBuildTemplateKind,
				},
			},
		},
		tmpl: &v1alpha1.ClusterBuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "hello",
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name: "hello",
				}},
				Template: &v1alpha1.TemplateInstantiationSpec{
					Kind: v1alpha1.ClusterBuildTemplateKind,
				},
			},
		},
	}, {
		// A build template with kind BuildTemplate
		build: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Template: &v1alpha1.TemplateInstantiationSpec{
					Kind: v1alpha1.BuildTemplateKind,
				},
			},
		},
		tmpl: &v1alpha1.BuildTemplate{
			Spec: v1alpha1.BuildTemplateSpec{
				Steps: []corev1.Container{{
					Name: "hello",
				}},
			},
		},
		want: &v1alpha1.Build{
			Spec: v1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name: "hello",
				}},
				Template: &v1alpha1.TemplateInstantiationSpec{
					Kind: v1alpha1.BuildTemplateKind,
				},
			},
		},
	}} {
		wantErr := c.want == nil
		got, err := ApplyTemplate(c.build, c.tmpl)
		if err != nil && !wantErr {
			t.Errorf("ApplyTemplate(%d); unexpected error %v", i, err)
		} else if err == nil && wantErr {
			t.Errorf("ApplyTemplate(%d); unexpected success; got %v", i, got)
		} else if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ApplyTemplate(%d);\n got %v\nwant %v", i, got, c.want)
		}
	}
}

func TestApplyReplacements(t *testing.T) {
	type args struct {
		build        *v1alpha1.Build
		replacements map[string]string
	}
	tests := []struct {
		name string
		args args
		want *v1alpha1.Build
	}{
		{
			name: "no replacements",
			args: args{
				build: &v1alpha1.Build{
					Spec: v1alpha1.BuildSpec{},
				},
				replacements: map[string]string{},
			},
			want: &v1alpha1.Build{
				Spec: v1alpha1.BuildSpec{},
			},
		},
		{
			name: "$ is not replaced",
			args: args{
				build: &v1alpha1.Build{
					Spec: v1alpha1.BuildSpec{
						Steps: []corev1.Container{
							{
								Name:  "$foo",
								Image: "${foo}",
							},
						},
					},
				},
				replacements: map[string]string{
					"foo": "bar",
				},
			},
			want: &v1alpha1.Build{
				Spec: v1alpha1.BuildSpec{
					Steps: []corev1.Container{
						{
							Name:  "$foo",
							Image: "bar",
						},
					},
				},
			},
		},
		{
			name: "replacement in steps",
			args: args{
				build: &v1alpha1.Build{
					Spec: v1alpha1.BuildSpec{
						Steps: []corev1.Container{
							{
								Name:    "${a}-name",
								Image:   "${b}-img",
								Args:    []string{"--foo=${foo}"},
								Command: []string{"cmd", "${command}"},
								Env: []corev1.EnvVar{
									{
										Name:  "key",
										Value: "${value}",
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "${volume}",
										MountPath: "${mountpath}",
										SubPath:   "${subpath}",
									},
								},
								WorkingDir: "/${workdir}",
							},
						},
					},
				},
				replacements: map[string]string{
					"a":         "1",
					"b":         "2",
					"foo":       "bar",
					"value":     "myvalue",
					"volume":    "myvolume",
					"mountpath": "mymountpath",
					"subpath":   "mysubpath",
					"workdir":   "myworkdir",
					"command":   "mycommand",
				},
			},
			want: &v1alpha1.Build{
				Spec: v1alpha1.BuildSpec{
					Steps: []corev1.Container{
						{
							Name:    "1-name",
							Image:   "2-img",
							Args:    []string{"--foo=bar"},
							Command: []string{"cmd", "mycommand"},
							Env:     []corev1.EnvVar{{Name: "key", Value: "myvalue"}},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "myvolume",
									MountPath: "mymountpath",
									SubPath:   "mysubpath",
								},
							},
							WorkingDir: "/myworkdir",
						},
					},
				},
			},
		},
		{
			name: "replacement in volumes",
			args: args{
				build: &v1alpha1.Build{
					Spec: v1alpha1.BuildSpec{
						Volumes: []corev1.Volume{{
							Name: "${name}",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									corev1.LocalObjectReference{"${configmapname}"},
									nil,
									nil,
									nil,
								},
							}},
						},
					},
				},
				replacements: map[string]string{
					"name":          "myname",
					"configmapname": "cfgmapname",
				},
			},
			want: &v1alpha1.Build{
				Spec: v1alpha1.BuildSpec{
					Volumes: []corev1.Volume{{
						Name: "myname",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								corev1.LocalObjectReference{"cfgmapname"},
								nil,
								nil,
								nil,
							},
						}},
					},
				},
			},
		},
		{
			name: "replacement in volumes (secret) ",
			args: args{
				build: &v1alpha1.Build{
					Spec: v1alpha1.BuildSpec{
						Volumes: []corev1.Volume{{
							Name: "${name}",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									"${secretname}",
									nil,
									nil,
									nil,
								},
							}},
						},
					},
				},
				replacements: map[string]string{
					"name":       "mysecret",
					"secretname": "totallysecure",
				},
			},
			want: &v1alpha1.Build{
				Spec: v1alpha1.BuildSpec{
					Volumes: []corev1.Volume{{
						Name: "mysecret",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								"totallysecure",
								nil,
								nil,
								nil,
							},
						}},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ApplyReplacements(tt.args.build, tt.args.replacements); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ApplyReplacements() = %v, want %v", got, tt.want)
			}
		})
	}
}
