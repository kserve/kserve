/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	. "github.com/knative/pkg/testing"
)

func TestSendGlobalUpdate(t *testing.T) {
	called := make(map[interface{}]bool)
	handler := cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, new interface{}) {
			called[new] = true
		},
	}
	SendGlobalUpdates(&dummyInformer{}, handler)
	for _, obj := range dummyObjs {
		if updated, _ := called[obj]; !updated {
			t.Errorf("Expected obj %v to be updated but wasn't", obj)
		}
	}
	if len(dummyObjs) != len(called) {
		t.Errorf("Expected to see %d updates, saw %d", len(dummyObjs), len(called))
	}
}

func TestEnsureTypeMeta(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "foo.bar.com",
		Version: "v23",
		Kind:    "Magic",
	}
	apiVersion, kind := gvk.ToAPIVersionAndKind()

	tests := []struct {
		name string
		obj  interface{}
		want runtime.Object
	}{{
		name: "not a runtime.Object",
		obj:  struct{}{},
	}, {
		name: "called with type meta",
		obj: &Resource{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "thing",
			},
		},
		want: &Resource{
			TypeMeta: metav1.TypeMeta{
				APIVersion: apiVersion,
				Kind:       kind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "thing",
			},
		},
	}, {
		name: "called with deleted obj",
		obj: cache.DeletedFinalStateUnknown{
			Key: "default/thing",
			Obj: &Resource{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "thing",
				},
			},
		},
		want: &Resource{
			TypeMeta: metav1.TypeMeta{
				APIVersion: apiVersion,
				Kind:       kind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "thing",
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var cb Callback
			if test.want != nil {
				cb = func(got interface{}) {
					if diff := cmp.Diff(got, test.want); diff != "" {
						t.Errorf("EnsureTypeMeta = %s", diff)
					}
				}
			} else {
				cb = func(got interface{}) {
					t.Errorf("Wanted no call, got %#v", got)
				}
			}

			ncb := EnsureTypeMeta(cb, gvk)

			// This should either invoke the callback or not, the
			// rest of the test is in the configured callback.
			ncb(test.obj)
		})
	}
}
