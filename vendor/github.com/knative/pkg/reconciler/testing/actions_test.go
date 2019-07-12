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

	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgotesting "k8s.io/client-go/testing"
)

func TestActionsByVerb(t *testing.T) {
	list := ActionRecorderList{
		fakeRecorder{
			newGetAction(),
		},
		fakeRecorder{
			newCreateAction(),
			newUpdateAction(),
			newDeleteAction(),
			newPatchAction(),
		},
		fakeRecorder{
			newCreateAction(),
		},
		fakeRecorder{
			newUpdateAction(),
		},
		fakeRecorder{
			newDeleteAction(),
		},
		fakeRecorder{
			newPatchAction(),
		},
	}

	actions, err := list.ActionsByVerb()

	if err != nil {
		t.Errorf("Unexpected error sorting actions by verb %s", err)
	}

	if got, want := len(actions.Creates), 2; got != want {
		t.Errorf("Create action count = %d, want %d", got, want)
	}

	if got, want := len(actions.Updates), 2; got != want {
		t.Errorf("Update action count = %d; want %d", got, want)
	}

	if got, want := len(actions.Deletes), 2; got != want {
		t.Errorf("Delete action count is incorrect got %d - want %d", got, want)
	}

	if got, want := len(actions.Patches), 2; got != want {
		t.Errorf("Patch action = %d; want %d", got, want)
	}
}

func TestActionsByVerb_UnrecognizedVerb(t *testing.T) {
	list := ActionRecorderList{
		fakeRecorder{
			clientgotesting.ActionImpl{Verb: "unknown"},
		},
	}

	if _, err := list.ActionsByVerb(); err == nil {
		t.Error("Expected an error to have occurred when grouping actions")
	}
}

func newGetAction() clientgotesting.Action {
	return clientgotesting.NewGetAction(schema.GroupVersionResource{}, "namespace", "name")
}

func newCreateAction() clientgotesting.Action {
	return clientgotesting.NewCreateAction(schema.GroupVersionResource{}, "namespace", nil)
}

func newUpdateAction() clientgotesting.Action {
	return clientgotesting.NewUpdateAction(schema.GroupVersionResource{}, "namespace", nil)
}

func newDeleteAction() clientgotesting.Action {
	return clientgotesting.NewDeleteAction(schema.GroupVersionResource{}, "namespace", "name")
}

func newPatchAction() clientgotesting.Action {
	return clientgotesting.NewPatchAction(schema.GroupVersionResource{}, "namespace", "name", nil)
}

type fakeRecorder []clientgotesting.Action

func (f fakeRecorder) Actions() []clientgotesting.Action {
	return f
}

var _ ActionRecorder = (fakeRecorder)(nil)
