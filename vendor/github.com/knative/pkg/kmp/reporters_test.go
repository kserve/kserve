/*
Copyright 2019 The Knative Authors

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

package kmp

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type testStruct struct {
	A           string `json:"a"`
	StringField string `json:"foo"`
	IntField    int
	StructField childStruct `json:"child"`
	Omit        string      `json:"omit,omitempty"`
	Ignore      string      `json:"-"`
	Dash        string      `json:"-,"`
	MultiComma  string      `json:"multi,omitempty,somethingelse"`
}

type childStruct struct {
	ChildString string
	ChildInt    int
}

type privateStruct struct {
	privateField string
}

func TestFieldListReporter(t *testing.T) {

	tests := []struct {
		name string
		x    interface{}
		y    interface{}
		want []string
		opts []cmp.Option
	}{{
		name: "No diff",
		x: testStruct{
			StringField: "foo",
		},
		y: testStruct{
			StringField: "foo",
		},
		want: nil,
	}, {
		name: "Both nil objects",
		want: nil,
	}, {
		name: "Nil second object",
		x: testStruct{
			StringField: "foo",
		},
		want: []string{"root"},
	}, {
		name: "Single character field name",
		x: testStruct{
			A: "foo",
		},
		y:    testStruct{},
		want: []string{"a"},
	}, {
		name: "Single field",
		x: testStruct{
			StringField: "foo",
		},
		y: testStruct{
			StringField: "bar",
		},
		want: []string{"foo"},
	}, {
		name: "Multi field",
		x: testStruct{
			StringField: "foo",
			IntField:    5,
		},
		y: testStruct{
			StringField: "bar",
			IntField:    6,
		},
		want: []string{"IntField", "foo"},
	}, {
		name: "Missing field",
		x: testStruct{
			StringField: "test",
		},
		y:    testStruct{},
		want: []string{"foo"},
	}, {
		name: "Nested field",
		x: testStruct{
			StructField: childStruct{
				ChildString: "baz",
			},
		},
		y: testStruct{
			StructField: childStruct{
				ChildString: "zap",
				ChildInt:    1,
			},
		},
		want: []string{"child"},
	}, {
		name: "Non Struct",
		x:    "foo",
		y:    "bar",
		want: []string{"{string}"},
	}, {
		name: "Private field allowed",
		x: privateStruct{
			privateField: "Foo",
		},
		y: privateStruct{
			privateField: "Bar",
		},
		want: []string{"privateField"},
		opts: []cmp.Option{cmp.AllowUnexported(privateStruct{})},
	}, {
		name: "Private field ignored",
		x: privateStruct{
			privateField: "Foo",
		},
		y: privateStruct{
			privateField: "Bar",
		},
		want: nil,
		opts: []cmp.Option{cmpopts.IgnoreUnexported(privateStruct{})},
	}, {
		name: "Omit empty",
		x: testStruct{
			Omit: "Foo",
		},
		y: testStruct{
			Omit: "Bar",
		},
		want: []string{"omit"},
	}, {
		name: "Ignore JSON",
		x: testStruct{
			Ignore: "Foo",
		},
		y: testStruct{
			Ignore: "Bar",
		},
		want: []string{"Ignore"},
	}, {
		name: "Dash json name",
		x: testStruct{
			Dash: "Foo",
		},
		y: testStruct{
			Dash: "Bar",
		},
		want: []string{"-"},
	}, {
		name: "Multi comma in json tag",
		x: testStruct{
			MultiComma: "Foo",
		},
		y: testStruct{
			MultiComma: "Bar",
		},
		want: []string{"multi"},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reporter := new(FieldListReporter)
			cmp.Equal(test.x, test.y, append(test.opts, cmp.Reporter(reporter))...)
			got := reporter.Fields()
			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Errorf("%s: Fields() = %v, Want %v", test.name, got, test.want)
			}
		})
	}
}

func TestImmutableReporter(t *testing.T) {
	tests := []struct {
		name      string
		x         interface{}
		y         interface{}
		want      string
		expectErr bool
		opts      []cmp.Option
	}{{
		name: "No diff",
		x: testStruct{
			StringField: "foo",
		},
		y: testStruct{
			StringField: "foo",
		},
		want: "",
	}, {
		name: "Both nil objects",
		want: "",
	}, {
		name: "Nil second object",
		x: testStruct{
			StringField: "foo",
		},
		expectErr: true,
	}, {
		name: "Single character field name",
		x: testStruct{
			A: "foo",
		},
		y: testStruct{},
		want: `{kmp.testStruct}.A:
	-: "foo"
	+: ""
`,
	}, {
		name: "Single field",
		x: testStruct{
			StringField: "foo",
		},
		y: testStruct{
			StringField: "bar",
		},
		want: `{kmp.testStruct}.StringField:
	-: "foo"
	+: "bar"
`,
	}, {
		name: "Multi field",
		x: testStruct{
			StringField: "foo",
			IntField:    5,
		},
		y: testStruct{
			StringField: "bar",
			IntField:    6,
		},
		want: `{kmp.testStruct}.StringField:
	-: "foo"
	+: "bar"
{kmp.testStruct}.IntField:
	-: "5"
	+: "6"
`,
	}, {
		name: "Missing field",
		x: testStruct{
			StringField: "foo",
		},
		y: testStruct{},
		want: `{kmp.testStruct}.StringField:
	-: "foo"
	+: ""
`,
	}, {
		name: "Nested field",
		x: testStruct{
			StructField: childStruct{
				ChildString: "baz",
			},
		},
		y: testStruct{
			StructField: childStruct{
				ChildString: "zap",
				ChildInt:    1,
			},
		},
		want: `{kmp.testStruct}.StructField.ChildString:
	-: "baz"
	+: "zap"
{kmp.testStruct}.StructField.ChildInt:
	-: "0"
	+: "1"
`,
	}, {
		name: "Non Struct",
		x:    "foo",
		y:    "bar",
		want: `{string}:
	-: "foo"
	+: "bar"
`,
	}, {
		name: "Private field allowed",
		x: privateStruct{
			privateField: "Foo",
		},
		y: privateStruct{
			privateField: "Bar",
		},
		want: `{kmp.privateStruct}.privateField:
	-: "Foo"
	+: "Bar"
`,
		opts: []cmp.Option{cmp.AllowUnexported(privateStruct{})},
	}, {
		name: "Private field ignored",
		x: privateStruct{
			privateField: "Foo",
		},
		y: privateStruct{
			privateField: "Bar",
		},
		want: "",
		opts: []cmp.Option{cmpopts.IgnoreUnexported(privateStruct{})},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reporter := new(ShortDiffReporter)
			cmp.Equal(test.x, test.y, append(test.opts, cmp.Reporter(reporter))...)
			got, err := reporter.Diff()
			if test.expectErr {
				if err == nil {
					t.Fatalf("%s: Diff(), expected err, got nil", test.name)
				}
			} else {
				if err != nil {
					t.Fatalf("%s: Diff(), unexpected err: %v", test.name, err)
				}
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Errorf("%s: Diff() (-want, +got):\n %s", test.name, diff)
				}
			}
		})
	}
}
