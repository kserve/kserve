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
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
)

type Enclosing struct {
	Build *RawExtension `json:"build,omitempty"`
}

func TestMarshal(t *testing.T) {
	tests := []struct {
		name string
		obj  interface{}
		want string
	}{{
		name: "omit raw extension",
		obj:  &Enclosing{},
		want: `{}`,
	}, {
		name: "empty raw extension",
		obj: &Enclosing{
			Build: &RawExtension{},
		},
		want: `{"build":null}`,
	}, {
		name: "raw extension with bytes",
		obj: &Enclosing{
			Build: &RawExtension{
				Raw: []byte(`"this is a string"`),
			},
		},
		want: `{"build":"this is a string"}`,
	}, {
		name: "raw extension with object",
		obj: &Enclosing{
			Build: &RawExtension{
				Object: &buildv1alpha1.Build{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "build.knative.dev",
						Kind:       "Build",
					},
					Spec: buildv1alpha1.BuildSpec{
						Steps: []corev1.Container{{
							Image: "busybox",
						}},
					},
				},
			},
		},
		// TODO(mattmoor): This should not include Status.
		want: `{"build":{"kind":"Build","apiVersion":"build.knative.dev","metadata":{"creationTimestamp":null},"spec":{"steps":[{"name":"","image":"busybox","resources":{}}],"Status":""},"status":{"stepsCompleted":null}}}`,
	}, {
		name: "raw extension with buildspec",
		obj: &Enclosing{
			Build: &RawExtension{
				BuildSpec: &buildv1alpha1.BuildSpec{
					Steps: []corev1.Container{{
						Image: "busybox",
					}},
				},
			},
		},
		want: `{"build":{"steps":[{"name":"","image":"busybox","resources":{}}],"Status":""}}`,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b, err := json.Marshal(test.obj)
			if err != nil {
				t.Fatalf("Marshal() = %v", err)
			}

			if got := string(b); got != test.want {
				t.Errorf("Marshal() = %v, wanted %v", got, test.want)
			}
		})
	}
}

func TestUnmarshal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *RawExtension
	}{{
		name:  "null",
		input: "null",
		want:  &RawExtension{},
	}, {
		name:  "other content",
		input: `"a string"`,
		want: &RawExtension{
			Raw: []byte(`"a string"`),
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := &RawExtension{}
			err := json.Unmarshal([]byte(test.input), got)
			if err != nil {
				t.Fatalf("Unmarshal() = %v", err)
			}

			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("Unmarshal (-want, +got) = %v", diff)
			}
		})
	}
}

func TestUnmarshalError(t *testing.T) {
	re := (*RawExtension)(nil)
	err := re.UnmarshalJSON([]byte("{}"))
	if err == nil {
		t.Errorf("Unmarshal() = %#v, wanted error", re)
	}
}

func TestAs(t *testing.T) {
	var x string
	hello := "hello"
	tests := []struct {
		name  string
		input *RawExtension
		got   interface{}
		want  interface{}
	}{{
		name:  "empty runtime.Object",
		input: &RawExtension{Raw: []byte(`{}`)},
		got:   &buildv1alpha1.Build{},
		want:  &buildv1alpha1.Build{},
	}, {
		name:  "non-empty runtime.Object",
		input: &RawExtension{Raw: []byte(`{"apiVersion":"build.knative.dev/v1alpha1","kind":"Build","spec": {"steps":[]}}`)},
		got:   &buildv1alpha1.Build{},
		want: &buildv1alpha1.Build{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "build.knative.dev/v1alpha1",
				Kind:       "Build",
			},
			Spec: buildv1alpha1.BuildSpec{
				Steps: []corev1.Container{},
			},
		},
	}, {
		name:  "non-empty BuildSpec",
		input: &RawExtension{Raw: []byte(`{"steps":[{"image":"busybox"}]}`)},
		got:   &buildv1alpha1.BuildSpec{},
		want: &buildv1alpha1.BuildSpec{
			Steps: []corev1.Container{{
				Image: "busybox",
			}},
		},
	}, {
		name: "actual BuildSpec",
		input: &RawExtension{BuildSpec: &buildv1alpha1.BuildSpec{
			Steps: []corev1.Container{{
				Image: "busybox",
			}},
		}},
		got: &buildv1alpha1.BuildSpec{},
		want: &buildv1alpha1.BuildSpec{
			Steps: []corev1.Container{{
				Image: "busybox",
			}},
		},
	}, {
		name:  "non-empty string",
		input: &RawExtension{Raw: []byte(`"hello"`)},
		got:   &x,
		want:  &hello,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.input.As(test.got); err != nil {
				t.Errorf("As() = %v", err)
			}

			if diff := cmp.Diff(test.want, test.got); diff != "" {
				t.Errorf("As (-want, +got) = %v", diff)
			}
		})
	}
}

func TestAsDuck(t *testing.T) {
	var x string
	hello := "hello"
	tests := []struct {
		name  string
		input *RawExtension
		got   interface{}
		want  interface{}
	}{{
		name:  "empty runtime.Object",
		input: &RawExtension{Raw: []byte(`{}`)},
		got:   &buildv1alpha1.Build{},
		want:  &buildv1alpha1.Build{},
	}, {
		name:  "non-empty runtime.Object",
		input: &RawExtension{Raw: []byte(`{"apiVersion":"build.knative.dev/v1alpha1","kind":"Build","spec": {"steps":[]}}`)},
		got:   &buildv1alpha1.Build{},
		want: &buildv1alpha1.Build{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "build.knative.dev/v1alpha1",
				Kind:       "Build",
			},
			Spec: buildv1alpha1.BuildSpec{
				Steps: []corev1.Container{},
			},
		},
	}, {
		name:  "non-empty BuildSpec",
		input: &RawExtension{Raw: []byte(`{"steps":[{"image":"busybox"}]}`)},
		got:   &buildv1alpha1.BuildSpec{},
		want: &buildv1alpha1.BuildSpec{
			Steps: []corev1.Container{{
				Image: "busybox",
			}},
		},
	}, {
		name: "actual BuildSpec",
		input: &RawExtension{BuildSpec: &buildv1alpha1.BuildSpec{
			Steps: []corev1.Container{{
				Image: "busybox",
			}},
		}},
		got: &buildv1alpha1.BuildSpec{},
		want: &buildv1alpha1.BuildSpec{
			Steps: []corev1.Container{{
				Image: "busybox",
			}},
		},
	}, {
		name:  "non-empty string",
		input: &RawExtension{Raw: []byte(`"hello"`)},
		got:   &x,
		want:  &hello,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.input.AsDuck(test.got); err != nil {
				t.Errorf("As() = %v", err)
			}

			if diff := cmp.Diff(test.want, test.got); diff != "" {
				t.Errorf("As (-want, +got) = %v", diff)
			}
		})
	}
}

func TestAsWithExtraFields(t *testing.T) {
	var obj struct {
		Some string `json:"some"`
	}

	re := &RawExtension{
		Raw: []byte(`{"another":"value"}`),
	}

	// We should get an error trying to use As with an incomplete type.
	if err := re.As(&obj); err == nil {
		t.Errorf("As() = %#v, wanted error", obj)
	}
}

func TestNilBytesAsStuff(t *testing.T) {
	obj := &buildv1alpha1.Build{}

	re := &RawExtension{}
	if err := re.As(obj); err == nil {
		t.Errorf("As() = %#v, wanted error", obj)
	}

	if err := re.AsDuck(obj); err == nil {
		t.Errorf("As() = %#v, wanted error", obj)
	}
}

func TestMarshalError(t *testing.T) {
	obj := &badObject{
		Foo: doNotMarshal{},
	}

	re := &RawExtension{
		Object: obj,
	}
	if err := re.As(obj); err == nil {
		t.Errorf("As() = %#v, wanted error", obj)
	}

	if err := re.AsDuck(obj); err == nil {
		t.Errorf("As() = %#v, wanted error", obj)
	}
}

var errNoMarshal = errors.New("this cannot be marshalled")

type badObject struct {
	Foo doNotMarshal `json:"foo"`
}

type doNotMarshal struct{}

var _ json.Marshaler = (*doNotMarshal)(nil)

func (*doNotMarshal) MarshalJSON() ([]byte, error) {
	return nil, errNoMarshal
}

func (bo *badObject) GetObjectKind() schema.ObjectKind {
	return &metav1.TypeMeta{}
}

func (bo *badObject) DeepCopyObject() runtime.Object {
	return &badObject{}
}
