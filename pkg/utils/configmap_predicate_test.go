/*
Copyright 2025 The KServe Authors.

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

package utils

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestInferenceServiceConfigPredicate(t *testing.T) {
	pred := InferenceServiceConfigPredicate("inferenceservice-config", "kserve")

	matchingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "inferenceservice-config",
			Namespace:       "kserve",
			ResourceVersion: "1",
		},
	}
	nonMatchingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "other-config",
			Namespace:       "kserve",
			ResourceVersion: "1",
		},
	}
	wrongNamespaceCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "inferenceservice-config",
			Namespace:       "other-ns",
			ResourceVersion: "1",
		},
	}

	// Create events
	if !pred.Create(event.CreateEvent{Object: matchingCM}) {
		t.Error("Expected Create to return true for matching ConfigMap")
	}
	if pred.Create(event.CreateEvent{Object: nonMatchingCM}) {
		t.Error("Expected Create to return false for non-matching ConfigMap")
	}
	if pred.Create(event.CreateEvent{Object: wrongNamespaceCM}) {
		t.Error("Expected Create to return false for wrong namespace ConfigMap")
	}

	// Update events with ResourceVersion change
	updatedCM := matchingCM.DeepCopy()
	updatedCM.ResourceVersion = "2"
	if !pred.Update(event.UpdateEvent{ObjectOld: matchingCM, ObjectNew: updatedCM}) {
		t.Error("Expected Update to return true for matching ConfigMap with changed ResourceVersion")
	}
	if pred.Update(event.UpdateEvent{ObjectOld: nonMatchingCM, ObjectNew: nonMatchingCM}) {
		t.Error("Expected Update to return false for non-matching ConfigMap")
	}

	// Update events with same ResourceVersion should be filtered by ResourceVersionChangedPredicate
	if pred.Update(event.UpdateEvent{ObjectOld: matchingCM, ObjectNew: matchingCM}) {
		t.Error("Expected Update to return false when ResourceVersion unchanged")
	}

	// Delete events always return false
	if pred.Delete(event.DeleteEvent{Object: matchingCM}) {
		t.Error("Expected Delete to always return false")
	}

	// Generic events always return false
	if pred.Generic(event.GenericEvent{Object: matchingCM}) {
		t.Error("Expected Generic to always return false")
	}
}
