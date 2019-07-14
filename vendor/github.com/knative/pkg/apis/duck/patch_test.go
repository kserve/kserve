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

package duck

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCreateMergePatch(t *testing.T) {
	tests := []struct {
		name    string
		before  interface{}
		after   interface{}
		wantErr bool
		want    []byte
	}{{
		name: "patch single field",
		before: &Patch{
			Spec: PatchSpec{
				Patchable: &Patchable{
					Field1: 12,
				},
			},
		},
		after: &Patch{
			Spec: PatchSpec{
				Patchable: &Patchable{
					Field1: 13,
				},
			},
		},
		want: []byte(`{"status":{"patchable":{"field1":13}}}`),
	}, {
		name: "patch two fields",
		before: &Patch{
			Spec: PatchSpec{
				Patchable: &Patchable{
					Field1: 12,
					Field2: true,
				},
			},
		},
		after: &Patch{
			Spec: PatchSpec{
				Patchable: &Patchable{
					Field1: 42,
					Field2: false,
				},
			},
		},
		want: []byte(`{"status":{"patchable":{"field1":42,"field2":null}}}`),
	}, {
		name: "patch array",
		before: &Patch{
			Spec: PatchSpec{
				Patchable: &Patchable{
					Array: []string{"foo", "baz"},
				},
			},
		},
		after: &Patch{
			Spec: PatchSpec{
				Patchable: &Patchable{
					Array: []string{"foo", "bar", "baz"},
				},
			},
		},
		want: []byte(`{"status":{"patchable":{"array":["foo","bar","baz"]}}}`),
	}, {
		name:    "before doesn't marshal",
		before:  &DoesntMarshal{},
		after:   &Patch{},
		wantErr: true,
	}, {
		name:    "after doesn't marshal",
		before:  &Patch{},
		after:   &DoesntMarshal{},
		wantErr: true,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CreateMergePatch(test.before, test.after)
			if err != nil {
				if !test.wantErr {
					t.Errorf("CreateMergePatch() = %v", err)
				}
				return
			} else if test.wantErr {
				t.Errorf("CreateMergePatch() = %v, wanted error", got)
				return
			}

			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("CreatePatch (-want, +got) = %v, %s", diff, got)
			}
		})
	}
}

func TestCreatePatch(t *testing.T) {
	tests := []struct {
		name    string
		before  interface{}
		after   interface{}
		wantErr bool
		want    JSONPatch
	}{{
		name: "patch single field",
		before: &Patch{
			Spec: PatchSpec{
				Patchable: &Patchable{
					Field1: 12,
				},
			},
		},
		after: &Patch{
			Spec: PatchSpec{
				Patchable: &Patchable{
					Field1: 13,
				},
			},
		},
		want: JSONPatch{{
			Operation: "replace",
			Path:      "/status/patchable/field1",
			Value:     13.0,
		}},
	}, {
		name: "patch two fields",
		before: &Patch{
			Spec: PatchSpec{
				Patchable: &Patchable{
					Field1: 12,
					Field2: true,
				},
			},
		},
		after: &Patch{
			Spec: PatchSpec{
				Patchable: &Patchable{
					Field1: 42,
					Field2: false,
				},
			},
		},
		want: JSONPatch{{
			Operation: "replace",
			Path:      "/status/patchable/field1",
			Value:     42.0,
		}, {
			Operation: "remove",
			Path:      "/status/patchable/field2",
		}},
	}, {
		name: "patch array",
		before: &Patch{
			Spec: PatchSpec{
				Patchable: &Patchable{
					Array: []string{"foo", "baz"},
				},
			},
		},
		after: &Patch{
			Spec: PatchSpec{
				Patchable: &Patchable{
					Array: []string{"foo", "bar", "baz"},
				},
			},
		},
		want: JSONPatch{{
			Operation: "add",
			Path:      "/status/patchable/array/1",
			Value:     "bar",
		}},
	}, {
		name:    "before doesn't marshal",
		before:  &DoesntMarshal{},
		after:   &Patch{},
		wantErr: true,
	}, {
		name:    "after doesn't marshal",
		before:  &Patch{},
		after:   &DoesntMarshal{},
		wantErr: true,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CreatePatch(test.before, test.after)
			if err != nil {
				if !test.wantErr {
					t.Errorf("CreatePatch() = %v", err)
				}
				return
			} else if test.wantErr {
				t.Errorf("CreatePatch() = %v, wanted error", got)
				return
			}

			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("CreatePatch (-want, +got) = %v", diff)
			}
		})
	}
}

func TestPatchToJSON(t *testing.T) {
	input := JSONPatch{{
		Operation: "replace",
		Path:      "/status/patchable/field1",
		Value:     42.0,
	}, {
		Operation: "remove",
		Path:      "/status/patchable/field2",
	}}

	b, err := input.MarshalJSON()
	if err != nil {
		t.Errorf("MarshalJSON() = %v", err)
	}

	want := `[{"op":"replace","path":"/status/patchable/field1","value":42},{"op":"remove","path":"/status/patchable/field2"}]`

	got := string(b)
	if got != want {
		t.Errorf("MarshalJSON() = %v, wanted %v", got, want)
	}
}

type DoesntMarshal struct{}

var _ json.Marshaler = (*DoesntMarshal)(nil)

func (*DoesntMarshal) MarshalJSON() ([]byte, error) {
	return nil, errors.New("what did you expect?")
}

// Define a "Patchable" duck type.
type Patchable struct {
	Field1 int      `json:"field1,omitempty"`
	Field2 bool     `json:"field2,omitempty"`
	Array  []string `json:"array,omitempty"`
}
type Patch struct {
	Spec PatchSpec `json:"status"`
}
type PatchSpec struct {
	Patchable *Patchable `json:"patchable,omitempty"`
}

var _ Implementable = (*Patchable)(nil)
var _ Populatable = (*Patch)(nil)

func (*Patchable) GetFullType() Populatable {
	return &Patch{}
}

func (f *Patch) Populate() {
	f.Spec.Patchable = &Patchable{
		// Populate ALL fields
		Field1: 42,
		Field2: true,
	}
}
