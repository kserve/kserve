/*
Copyright 2019 The Knative Authors.

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
package testing

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestObjectSorter(t *testing.T) {
	sorter := NewObjectSorter(testScheme())

	sorter.AddObjects(
		newTestObject1("first"),
		newTestObject2("second"),
		newTestObject2("third"),
		newTestObject2("fourth"),
	)

	objects := sorter.ObjectsForScheme(testSchemeSubset1())

	if got, want := len(objects), 1; got != want {
		t.Errorf("ObjectsForScheme did not return the correct number of elements - got %d, want %d",
			got, want)
	}

	objects = sorter.ObjectsForSchemeFunc(testSchemeSubset2Func)

	if got, want := len(objects), 3; got != want {
		t.Errorf("ObjectsForSchemeFunc did not return the correct number of elements - got %d, want %d",
			got, want)
	}

	indexer := sorter.IndexerForObjectType(&testObject2{})

	if got, want := len(indexer.List()), 3; got != want {
		t.Errorf("IndexerForObjectType did not return the correct number of elements - got %d, want %d",
			got, want)
	}
}

func TestObjectSorterAddUnrecognizedType(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("AddObjects did not panic when receiving an unrecognized type ")
		}
	}()

	sorter := NewObjectSorter(testSchemeSubset1())
	sorter.AddObjects(
		&testObject2{},
	)
}

func TestObjectSorterIndexerUnrecognizedType(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("IndexerForObjectType did not panic when receiving an unrecognized type ")
		}
	}()

	sorter := NewObjectSorter(testSchemeSubset1())
	sorter.IndexerForObjectType(&testObject2{})
}

func testScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(
		schema.GroupVersion{
			Group:   "test.group",
			Version: "v1",
		},
		&testObject1{},
		&testObject2{},
	)
	return scheme
}

func testSchemeSubset1() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(
		schema.GroupVersion{
			Group:   "test.group",
			Version: "v1",
		},
		&testObject1{},
	)
	return scheme
}

func testSchemeSubset2Func(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(
		schema.GroupVersion{
			Group:   "test.group",
			Version: "v1",
		},
		&testObject2{},
	)
	return nil
}

type testObject1 struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

type testObject2 struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (in *testObject1) DeepCopyObject() runtime.Object {
	return in
}

func (in *testObject2) DeepCopyObject() runtime.Object {
	return in
}

func newTestObject1(name string) *testObject1 {
	return &testObject1{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func newTestObject2(name string) *testObject2 {
	return &testObject2{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
