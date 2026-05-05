/*
Copyright 2026 The KServe Authors.

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

package inferenceservice

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/constants"
)

func TestNewCacheOptions(t *testing.T) {
	opts, err := NewCacheOptions()
	if err != nil {
		t.Fatalf("NewCacheOptions returned error: %v", err)
	}

	var (
		podEntryFound       bool
		configMapEntryFound bool
	)

	for obj, byObj := range opts.ByObject {
		switch obj.(type) {
		case *corev1.Pod:
			podEntryFound = true
			if byObj.Label == nil {
				t.Errorf("Pod cache entry missing label selector")
				break
			}
			if !strings.Contains(byObj.Label.String(), constants.InferenceServicePodLabelKey) {
				t.Errorf("Pod label selector %q does not reference %q",
					byObj.Label.String(), constants.InferenceServicePodLabelKey)
			}
		case *corev1.ConfigMap:
			configMapEntryFound = true
			if _, ok := byObj.Namespaces[constants.KServeNamespace]; !ok {
				t.Errorf("ConfigMap cache not scoped to %q; got namespaces %v",
					constants.KServeNamespace, byObj.Namespaces)
			}
			if len(byObj.Namespaces) != 1 {
				t.Errorf("ConfigMap cache should be scoped to exactly one namespace, got %d: %v",
					len(byObj.Namespaces), byObj.Namespaces)
			}
		}
	}

	if !podEntryFound {
		t.Errorf("expected Pod entry in ByObject, none found")
	}
	if !configMapEntryFound {
		t.Errorf("expected ConfigMap entry in ByObject, none found")
	}
}
