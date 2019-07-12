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

package tracker

import (
	"regexp"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	. "github.com/knative/pkg/testing"
)

func TestHappyPaths(t *testing.T) {
	calls := 0
	f := func(key string) {
		calls = calls + 1
	}

	trk := New(f, 10*time.Millisecond)

	thing1 := &Resource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "ref.knative.dev/v1alpha1",
			Kind:       "Thing1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "foo",
		},
	}
	objRef := objectReference(thing1)

	thing2 := &Resource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "reffer.knative.dev/v1alpha1",
			Kind:       "Thing2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "bar.baz.this-is-fine",
		},
	}

	t.Run("Not tracked yet", func(t *testing.T) {
		trk.OnChanged(thing1)
		if got, want := calls, 0; got != want {
			t.Errorf("OnChanged() = %v, wanted %v", got, want)
		}
	})

	t.Run("Tracked gets called", func(t *testing.T) {
		if err := trk.Track(objRef, thing2); err != nil {
			t.Errorf("Track() = %v", err)
		}
		// New registrations should result in an immediate callback.
		if got, want := calls, 1; got != want {
			t.Errorf("Track() = %v, wanted %v", got, want)
		}

		trk.OnChanged(thing1)
		if got, want := calls, 2; got != want {
			t.Errorf("OnChanged() = %v, wanted %v", got, want)
		}
	})

	t.Run("Still gets called", func(t *testing.T) {
		trk.OnChanged(thing1)
		if got, want := calls, 3; got != want {
			t.Errorf("OnChanged() = %v, wanted %v", got, want)
		}
	})

	// Check that after the sleep duration, we stop getting called.
	time.Sleep(20 * time.Millisecond)
	t.Run("Stops getting called", func(t *testing.T) {
		trk.OnChanged(thing1)
		if got, want := calls, 3; got != want {
			t.Errorf("OnChanged() = %v, wanted %v", got, want)
		}
		if _, stillThere := trk.(*impl).mapping[objRef]; stillThere {
			t.Error("Timeout passed, but mapping for objectReference is still there")
		}
	})

	t.Run("Starts getting called again", func(t *testing.T) {
		if err := trk.Track(objRef, thing2); err != nil {
			t.Errorf("Track() = %v", err)
		}
		// New registrations should result in an immediate callback.
		if got, want := calls, 4; got != want {
			t.Errorf("Track() = %v, wanted %v", got, want)
		}

		trk.OnChanged(thing1)
		if got, want := calls, 5; got != want {
			t.Errorf("OnChanged() = %v, wanted %v", got, want)
		}
	})

	t.Run("OnChanged non-accessor", func(t *testing.T) {
		// Check that passing in a resource that doesn't implement
		// accessor won't panic.
		trk.OnChanged("not an accessor")

		if got, want := calls, 5; got != want {
			t.Errorf("OnChanged() = %v, wanted %v", got, want)
		}
	})

	t.Run("OnChanged non-accessor in DeletedFinalStateUnknown", func(t *testing.T) {
		// Check that passing in a DeletedFinalStateUnknown instance
		// with a resource that doesn't implement accessor won't get
		// Tracked called, and won't panic.
		trk.OnChanged(cache.DeletedFinalStateUnknown{
			Key: "ns/foo",
			Obj: "not an accessor",
		})

		if got, want := calls, 5; got != want {
			t.Errorf("OnChanged() = %v, wanted %v", got, want)
		}
	})

	t.Run("Tracked gets called by DeletedFinalStateUnknown", func(t *testing.T) {
		trk.OnChanged(cache.DeletedFinalStateUnknown{
			Key: "ns/foo",
			Obj: thing1,
		})
		if got, want := calls, 6; got != want {
			t.Errorf("OnChanged() = %v, wanted %v", got, want)
		}
	})

	t.Run("Track bad object", func(t *testing.T) {
		if err := trk.Track(objRef, struct{}{}); err == nil {
			t.Error("Track() = nil, wanted error")
		}
	})
}

func TestAllowedObjectReferences(t *testing.T) {
	trk := New(func(key string) {}, 10*time.Millisecond)
	thing1 := &Resource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "ref.knative.dev/v1alpha1",
			Kind:       "Thing1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "foo",
		},
	}
	tests := []struct {
		name   string
		objRef corev1.ObjectReference
	}{{
		name: "Pod",
		objRef: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Pod",
			Namespace:  "default",
			Name:       "test",
		},
	}, {
		name: "Non-core resource",
		objRef: corev1.ObjectReference{
			APIVersion: "custom.example.com/v1alpha17",
			Kind:       "Widget",
			Namespace:  "default",
			Name:       "test",
		},
	}, {
		name: "Complex Kind",
		objRef: corev1.ObjectReference{
			APIVersion: "custom.example.com/v1alpha17",
			Kind:       "Widget_v3",
			Namespace:  "default",
			Name:       "test",
		},
	}, {
		name: "Dashed Namespace",
		objRef: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Namespace:  "not-default",
			Name:       "test",
		},
	}, {
		name: "Complex Name",
		objRef: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Namespace:  "default",
			Name:       "test.example.cluster.local",
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := trk.Track(test.objRef, thing1); err != nil {
				t.Errorf("Track() on %v returned error: %v", test.objRef, err)
			}
		})
	}
}

func TestBadObjectReferences(t *testing.T) {
	trk := New(func(key string) {}, 10*time.Millisecond)
	thing1 := &Resource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "ref.knative.dev/v1alpha1",
			Kind:       "Thing1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "foo",
		},
	}

	tests := []struct {
		name   string
		objRef corev1.ObjectReference
		match  string
	}{{
		name: "Missing APIVersion",
		objRef: corev1.ObjectReference{
			// APIVersion: "build.knative.dev/v1alpha1",
			Kind:      "Build",
			Namespace: "default",
			Name:      "kaniko",
		},
		match: "APIVersion",
	}, {
		name: "Bad char in APIVersion",
		objRef: corev1.ObjectReference{
			APIVersion: "build.knative.dev%v1alpha1",
			Kind:       "Build",
			Namespace:  "default",
			Name:       "kaniko",
		},
		match: "APIVersion",
	}, {
		name: "Extra slashes in APIVersion",
		objRef: corev1.ObjectReference{
			APIVersion: "build.knative.dev/v1/alpha1",
			Kind:       "Build",
			Namespace:  "default",
			Name:       "kaniko",
		},
		match: "APIVersion",
	}, {
		name: "Missing Kind",
		objRef: corev1.ObjectReference{
			APIVersion: "build.knative.dev/v1alpha1",
			// Kind:      "Build",
			Namespace: "default",
			Name:      "kaniko",
		},
		match: "Kind",
	}, {
		name: "Invalid Kind",
		objRef: corev1.ObjectReference{
			APIVersion: "build.knative.dev/v1alpha1",
			Kind:       "Build.1",
			Namespace:  "default",
			Name:       "kaniko",
		},
		match: "Kind",
	}, {
		name: "Missing Namespace",
		objRef: corev1.ObjectReference{
			APIVersion: "build.knative.dev/v1alpha1",
			Kind:       "Build",
			// Namespace: "default",
			Name: "kaniko",
		},
		match: "Namespace",
	}, {
		name: "Capital in Namespace",
		objRef: corev1.ObjectReference{
			APIVersion: "build.knative.dev/v1alpha1",
			Kind:       "Build",
			Namespace:  "Default",
			Name:       "kaniko",
		},
		match: "Namespace",
	}, {
		name: "Domain-separated Namespace",
		objRef: corev1.ObjectReference{
			APIVersion: "build.knative.dev/v1alpha1",
			Kind:       "Build",
			Namespace:  "not.default",
			Name:       "kaniko",
		},
		match: "Namespace",
	}, {
		name: "Missing Name",
		objRef: corev1.ObjectReference{
			APIVersion: "build.knative.dev/v1alpha1",
			Kind:       "Build",
			Namespace:  "default",
			// Name:      "kaniko",
		},
		match: "Name",
	}, {
		name: "Capital in Name",
		objRef: corev1.ObjectReference{
			APIVersion: "build.knative.dev/v1alpha1",
			Kind:       "Build",
			Namespace:  "default",
			Name:       "Kaniko",
		},
		match: "Name",
	}, {
		name: "Bad char in Name",
		objRef: corev1.ObjectReference{
			APIVersion: "build.knative.dev/v1alpha1",
			Kind:       "Build",
			Namespace:  "default",
			Name:       "kaniko_small",
		},
		match: "Name",
	}, {
		name:   "Missing All",
		objRef: corev1.ObjectReference{
			// APIVersion: "build.knative.dev/v1alpha1",
			// Kind:       "Build",
			// Namespace:  "default",
			// Name:      "kaniko",
		},
		match: "\nAPIVersion:.*\nKind:.*\nName:.*\nNamespace:",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := trk.Track(test.objRef, thing1); err == nil {
				t.Error("Track() = nil, wanted error")
			} else {
				match, e2 := regexp.Match(test.match, []byte(err.Error()))
				if e2 != nil {
					t.Errorf("Failed to compile %q: %v", e2, test.match)
				} else if !match {
					t.Errorf("Track() = %v, wanted match: %s", err, test.match)
				}
			}
		})
	}
}
