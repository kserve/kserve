/*
Copyright 2021 The KServe Authors.

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
	corev1 "k8s.io/api/core/v1"
	corev1helpers "k8s.io/component-helpers/scheduling/corev1"
)

// CheckNodeAffinity returns true if the node matches the node affinity specified in the PV Spec
func CheckNodeAffinity(pvSpec *corev1.PersistentVolumeSpec, node corev1.Node) (bool, error) {
	if pvSpec.NodeAffinity == nil || pvSpec.NodeAffinity.Required == nil {
		return false, nil
	}

	terms := pvSpec.NodeAffinity.Required
	return corev1helpers.MatchNodeSelectorTerms(&node, terms)
}
