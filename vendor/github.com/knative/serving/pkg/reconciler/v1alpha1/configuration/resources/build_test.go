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
	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuilds(t *testing.T) {
	tests := []struct {
		name          string
		configuration *v1alpha1.Configuration
		want          *unstructured.Unstructured
	}{{
		name: "no build",
		configuration: &v1alpha1.Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "no",
				Name:      "build",
			},
			Spec: v1alpha1.ConfigurationSpec{
				RevisionTemplate: &v1alpha1.RevisionTemplateSpec{
					Spec: v1alpha1.RevisionSpec{
						Container: &corev1.Container{
							Image: "busybox",
						},
					},
				},
			},
		},
		want: nil,
	}, {
		name: "simple build",
		configuration: &v1alpha1.Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "simple",
				Name:      "build",
			},
			Spec: v1alpha1.ConfigurationSpec{
				Build: &v1alpha1.RawExtension{BuildSpec: &buildv1alpha1.BuildSpec{
					Steps: []corev1.Container{{
						Image: "busybox",
					}},
				}},
				RevisionTemplate: &v1alpha1.RevisionTemplateSpec{
					Spec: v1alpha1.RevisionSpec{
						Container: &corev1.Container{
							Image: "busybox",
						},
					},
				},
			},
		},
		want: UnstructuredWithContent(map[string]interface{}{
			"apiVersion": "build.knative.dev/v1alpha1",
			"kind":       "Build",
			"metadata": map[string]interface{}{
				"namespace":    "simple",
				"generateName": "build-",
				"ownerReferences": []interface{}{map[string]interface{}{
					"apiVersion":         v1alpha1.SchemeGroupVersion.String(),
					"kind":               "Configuration",
					"name":               "build",
					"uid":                "",
					"controller":         true,
					"blockOwnerDeletion": true,
				}},
				"labels": map[string]interface{}{
					"serving.knative.dev/buildHash": "9f8c94db36e279e90614ea28d10da38587c36be4f5f01f78450df33c5eae417",
				},
				"creationTimestamp": nil,
			},
			"spec": map[string]interface{}{
				"steps": []interface{}{map[string]interface{}{
					"name":      "",
					"image":     "busybox",
					"resources": map[string]interface{}{},
				}},
				"Status": "",
			},
			"status": map[string]interface{}{
				"stepsCompleted": nil,
			},
		}),
	}, {
		// If a user specifies a build object with metadata.name set
		// we 'ignore' and clear that value
		name: "complex with a name",
		configuration: &v1alpha1.Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "simple",
				Name:      "build",
			},
			Spec: v1alpha1.ConfigurationSpec{
				Build: &v1alpha1.RawExtension{
					Object: &buildv1alpha1.Build{
						TypeMeta: metav1.TypeMeta{
							APIVersion: buildv1alpha1.SchemeGroupVersion.String(),
							Kind:       "Build",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "simple",
							Name:      "simple",
						},
						Spec: buildv1alpha1.BuildSpec{
							Steps: []corev1.Container{{
								Image: "busybox",
							}},
						},
					}},
				RevisionTemplate: &v1alpha1.RevisionTemplateSpec{
					Spec: v1alpha1.RevisionSpec{
						Container: &corev1.Container{
							Image: "busybox",
						},
					},
				},
			},
		},
		want: UnstructuredWithContent(map[string]interface{}{
			"apiVersion": "build.knative.dev/v1alpha1",
			"kind":       "Build",
			"metadata": map[string]interface{}{
				"namespace":    "simple",
				"generateName": "build-",
				"ownerReferences": []interface{}{map[string]interface{}{
					"apiVersion":         v1alpha1.SchemeGroupVersion.String(),
					"kind":               "Configuration",
					"name":               "build",
					"uid":                "",
					"controller":         true,
					"blockOwnerDeletion": true,
				}},
				"labels": map[string]interface{}{
					"serving.knative.dev/buildHash": "9745cf57f7dca85b845ccb7122d15952daee1db77dd06f7a60943b7f6465039",
				},
				"creationTimestamp": nil,
			},
			"spec": map[string]interface{}{
				"steps": []interface{}{map[string]interface{}{
					"name":      "",
					"image":     "busybox",
					"resources": map[string]interface{}{},
				}},
				"Status": "",
			},
			"status": map[string]interface{}{
				"stepsCompleted": nil,
			},
		}),
	}, {
		name: "simple build with template",
		configuration: &v1alpha1.Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "simple",
				Name:      "build-template",
			},
			Spec: v1alpha1.ConfigurationSpec{
				Build: &v1alpha1.RawExtension{BuildSpec: &buildv1alpha1.BuildSpec{
					Template: &buildv1alpha1.TemplateInstantiationSpec{
						Name: "buildpacks",
						Arguments: []buildv1alpha1.ArgumentSpec{{
							Name:  "foo",
							Value: "bar",
						}},
					},
				}},
				RevisionTemplate: &v1alpha1.RevisionTemplateSpec{
					Spec: v1alpha1.RevisionSpec{
						Container: &corev1.Container{
							Image: "busybox",
						},
					},
				},
			},
		},
		want: UnstructuredWithContent(map[string]interface{}{
			"apiVersion": "build.knative.dev/v1alpha1",
			"kind":       "Build",
			"metadata": map[string]interface{}{
				"namespace":    "simple",
				"generateName": "build-template-",
				"ownerReferences": []interface{}{map[string]interface{}{
					"apiVersion":         v1alpha1.SchemeGroupVersion.String(),
					"kind":               "Configuration",
					"name":               "build-template",
					"uid":                "",
					"controller":         true,
					"blockOwnerDeletion": true,
				}},
				"labels": map[string]interface{}{
					"serving.knative.dev/buildHash": "f547847100b5aab740cbaa1fc56b5d68c20200bcc935452d100ca6df2691610",
				},
				"creationTimestamp": nil,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"name": "buildpacks",
					"arguments": []interface{}{map[string]interface{}{
						"name":  "foo",
						"value": "bar",
					}},
				},
				"Status": "",
			},
			"status": map[string]interface{}{
				"stepsCompleted": nil,
			},
		}),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MakeBuild(test.configuration)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("MakeBuild (-want, +got) = %v", diff)
			}
		})
	}
}
