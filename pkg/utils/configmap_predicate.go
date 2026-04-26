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
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// InferenceServiceConfigPredicate filters ConfigMap events for controller watches.
// It matches only Create and Update events for the specified ConfigMap name and namespace,
// and uses ResourceVersionChangedPredicate to skip no-op updates.
func InferenceServiceConfigPredicate(configMapName, configMapNamespace string) predicate.Predicate {
	nameFilter := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			cm, ok := e.Object.(*corev1.ConfigMap)
			if !ok {
				return false
			}
			return cm.Name == configMapName && cm.Namespace == configMapNamespace
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			cmNew, ok := e.ObjectNew.(*corev1.ConfigMap)
			if !ok {
				return false
			}
			return cmNew.Name == configMapName && cmNew.Namespace == configMapNamespace
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
	dataChangedFilter := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			cmOld, okOld := e.ObjectOld.(*corev1.ConfigMap)
			cmNew, okNew := e.ObjectNew.(*corev1.ConfigMap)
			if !okOld || !okNew {
				return true
			}
			return !reflect.DeepEqual(cmOld.Data, cmNew.Data)
		},
	}
	return predicate.And(nameFilter, predicate.ResourceVersionChangedPredicate{}, dataChangedFilter)
}
