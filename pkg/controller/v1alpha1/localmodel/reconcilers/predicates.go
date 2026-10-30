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

package reconcilers

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/kserve/kserve/pkg/constants"
)

// NamespaceCacheISVCPredicate returns a predicate for the InferenceService events that can
// change which namespace-scoped LocalModelNamespaceCache an InferenceService uses.
//
// The cache name is the field index key, so an InferenceService that moves from one cache to
// another in the same namespace changes only LocalModelLabel. Comparing the namespace label
// alone drops that update, and the cache the InferenceService joined never reconciles.
func NamespaceCacheISVCPredicate() predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return namespaceCacheLabels(e.ObjectOld) != namespaceCacheLabels(e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			_, ok := e.Object.GetLabels()[constants.LocalModelNamespaceLabel]
			return ok
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			_, ok := e.Object.GetLabels()[constants.LocalModelNamespaceLabel]
			return ok
		},
	}
}

// namespaceCacheLabels returns the labels that select a namespace-scoped cache. They are
// compared as a pair so that a change to either one is an update.
func namespaceCacheLabels(obj client.Object) [2]string {
	labels := obj.GetLabels()
	return [2]string{labels[constants.LocalModelLabel], labels[constants.LocalModelNamespaceLabel]}
}

// NodeReadyPredicate returns a predicate that triggers when a node transitions from not-ready to ready
func NodeReadyPredicate() predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode := e.ObjectOld.(*corev1.Node)
			newNode := e.ObjectNew.(*corev1.Node)
			return !IsNodeReady(*oldNode) && IsNodeReady(*newNode)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}
