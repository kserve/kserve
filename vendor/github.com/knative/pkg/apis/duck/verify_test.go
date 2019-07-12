/*
Copyright 2017 The Knative Authors

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
	"errors"
	"testing"
)

func TestMatches(t *testing.T) {
	tests := []struct {
		name     string
		instance interface{}
		iface    Implementable
	}{{
		name:     "foo matches fooable",
		instance: &Foo{},
		iface:    &Fooable{},
	}, {
		name:     "bar matches barable",
		instance: &Bar{},
		iface:    &Barable{},
	}, {
		name:     "slice matches sliceable",
		instance: &Slice{},
		iface:    &Sliceable{},
	}, {
		name:     "string matches stringable",
		instance: &String{},
		iface:    &emptyStringable,
	}, {
		name: "other matches foo",
		instance: &struct {
			Status struct {
				Fooable *Fooable `json:"fooable,omitempty"`
			} `json:"status,omitempty"`
		}{},
		iface: &Fooable{},
	}, {
		name: "other (all) matches fooable",
		instance: &struct {
			Status struct {
				Fooable    *Fooable    `json:"fooable,omitempty"`
				Barable    *Barable    `json:"barable,omitempty"`
				Sliceable  Sliceable   `json:"sliceable,omitempty"`
				Stringable *Stringable `json:"stringable,omitempty"`
			} `json:"status,omitempty"`
		}{},
		iface: &Fooable{},
	}, {
		name: "other (all) matches barable",
		instance: &struct {
			Status struct {
				Fooable    *Fooable    `json:"fooable,omitempty"`
				Barable    *Barable    `json:"barable,omitempty"`
				Sliceable  Sliceable   `json:"sliceable,omitempty"`
				Stringable *Stringable `json:"stringable,omitempty"`
			} `json:"status,omitempty"`
		}{},
		iface: &Barable{},
	}, {
		name: "other (all) matches sliceable",
		instance: &struct {
			Status struct {
				Fooable    *Fooable    `json:"fooable,omitempty"`
				Barable    *Barable    `json:"barable,omitempty"`
				Sliceable  Sliceable   `json:"sliceable,omitempty"`
				Stringable *Stringable `json:"stringable,omitempty"`
			} `json:"status,omitempty"`
		}{},
		iface: &Sliceable{},
	}, {
		name: "other (all) matches stringable",
		instance: &struct {
			Status struct {
				Fooable    *Fooable    `json:"fooable,omitempty"`
				Barable    *Barable    `json:"barable,omitempty"`
				Sliceable  Sliceable   `json:"sliceable,omitempty"`
				Stringable *Stringable `json:"stringable,omitempty"`
			} `json:"status,omitempty"`
		}{},
		iface: &emptyStringable,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := VerifyType(test.instance, test.iface); err != nil {
				t.Error(err)
			}

			ok, err := ConformsToType(test.instance, test.iface)

			if err != nil {
				t.Error(err)
			}

			if !ok {
				t.Errorf("Expected %T to conform to %T", test.instance, test.iface)
			}
		})
	}
}

func TestMismatches(t *testing.T) {
	tests := []struct {
		name     string
		instance interface{}
		iface    Implementable
	}{{
		name:     "foo doesn't match barable",
		instance: &Foo{},
		iface:    &Barable{},
	}, {
		name:     "bar doesn't match fooable",
		instance: &Bar{},
		iface:    &Fooable{},
	}, {
		name:     "foo doesn't match sliceable",
		instance: &Foo{},
		iface:    &Sliceable{},
	}, {
		name: "other matches neither (foo)",
		instance: &struct {
			Status struct {
				Done bool `json:"done,omitempty"`
			} `json:"status,omitempty"`
		}{},
		iface: &Fooable{},
	}, {
		name: "other matches neither (slice)",
		instance: &struct {
			Status struct {
				Done bool `json:"done,omitempty"`
			} `json:"status,omitempty"`
		}{},
		iface: &Sliceable{},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := VerifyType(test.instance, test.iface); err == nil {
				t.Errorf("Unexpected success %T implements %T", test.instance, test.iface)
			}

			ok, err := ConformsToType(test.instance, test.iface)

			if err != nil {
				t.Error(err)
			}

			if ok {
				t.Errorf("Expected %T to not conform to %T", test.instance, test.iface)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name     string
		instance interface{}
		iface    Implementable
	}{{
		name:     "duck type - fails to marshal",
		instance: &Foo{},
		iface:    &UnableToMarshal{},
	}, {
		name:     "duck type - fails to unmarshal",
		instance: &Foo{},
		iface:    &UnableToUnmarshal{},
	}, {
		name:     "instance - fails to unmarshal",
		instance: &UnableToUnmarshal{},
		iface:    &Fooable{},
	}, {
		name:     "instance - fails to marshal",
		instance: &UnableToMarshal{},
		iface:    &Fooable{},
	}, {
		name:     "duck type - unexported fields",
		instance: &Foo{},
		iface:    &UnexportedFields{},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := VerifyType(test.instance, test.iface); err == nil {
				t.Error("expected VerifyType to return an error")
			}

			if _, err := ConformsToType(test.instance, test.iface); err == nil {
				t.Error("expected ConformsToType to return an error")
			}
		})
	}
}

// Define a "Fooable" duck type.
type Fooable struct {
	Field1 string `json:"field1,omitempty"`
	Field2 string `json:"field2,omitempty"`
}
type Foo struct {
	Status FooStatus `json:"status"`
}
type FooStatus struct {
	Fooable *Fooable `json:"fooable,omitempty"`
}

var _ Implementable = (*Fooable)(nil)
var _ Populatable = (*Foo)(nil)

func (*Fooable) GetFullType() Populatable {
	return &Foo{}
}

func (f *Foo) Populate() {
	f.Status.Fooable = &Fooable{
		// Populate ALL fields
		Field1: "foo",
		Field2: "bar",
	}
}

// Define a "Barable" duck type.
type Barable struct {
	Field1 int  `json:"field1,omitempty"`
	Field2 bool `json:"field2,omitempty"`
}
type Bar struct {
	Status BarStatus `json:"status"`
}
type BarStatus struct {
	Barable *Barable `json:"barable,omitempty"`
}

var _ Implementable = (*Barable)(nil)
var _ Populatable = (*Bar)(nil)

func (*Barable) GetFullType() Populatable {
	return &Bar{}
}

func (f *Bar) Populate() {
	f.Status.Barable = &Barable{
		// Populate ALL fields
		Field1: 42,
		Field2: true,
	}
}

// Define a "Sliceable" duck type.
type AStruct struct {
	Field string `json:"field,omitempty"`
}
type Sliceable []AStruct
type Slice struct {
	Status SliceStatus `json:"status"`
}
type SliceStatus struct {
	Sliceable *Sliceable `json:"sliceable,omitempty"`
}

var _ Implementable = (*Sliceable)(nil)
var _ Populatable = (*Slice)(nil)

func (*Sliceable) GetFullType() Populatable {
	return &Slice{}
}

func (f *Slice) Populate() {
	f.Status.Sliceable = &Sliceable{{"foo"}, {"bar"}}
}

// Define a "Stringable" duck type.
type Stringable string
type String struct {
	Status StringStatus `json:"status"`
}
type StringStatus struct {
	Stringable Stringable `json:"stringable,omitempty"`
}

var _ Implementable = (*Stringable)(nil)
var _ Populatable = (*String)(nil)

func (*Stringable) GetFullType() Populatable {
	return &String{}
}

func (f *String) Populate() {
	f.Status.Stringable = Stringable("hello duck")
}

// We have to do this for Stringable because we're aliasing a value type.
var emptyStringable Stringable

// For testing this doubles as the 'Implementable'
// and 'Populataable'
type UnableToMarshal struct{}

var _ Implementable = (*UnableToMarshal)(nil)
var _ Populatable = (*UnableToMarshal)(nil)

func (u *UnableToMarshal) GetFullType() Populatable {
	return u
}

func (u *UnableToMarshal) Populate() {
	return
}

func (u *UnableToMarshal) MarshalJSON() ([]byte, error) {
	return nil, errors.New("I will never marshal for you")
}

// For testing this doubles as the 'Implementable'
// and 'Populatable'
type UnableToUnmarshal struct{}

var _ Implementable = (*UnableToUnmarshal)(nil)
var _ Populatable = (*UnableToUnmarshal)(nil)

func (u *UnableToUnmarshal) GetFullType() Populatable {
	return u
}

func (u *UnableToUnmarshal) Populate() {
	return
}

func (u *UnableToUnmarshal) UnmarshalJSON([]byte) error {
	return errors.New("I will never unmarshal for you")
}

// For testing this doubles as the 'Implementable'
// and 'Populatable'
type UnexportedFields struct {
	a string
}

var _ Implementable = (*UnexportedFields)(nil)
var _ Populatable = (*UnexportedFields)(nil)

func (u *UnexportedFields) GetFullType() Populatable {
	return &UnexportedFields{}
}

func (u *UnexportedFields) Populate() {
	u.a = "hello"
	return
}
