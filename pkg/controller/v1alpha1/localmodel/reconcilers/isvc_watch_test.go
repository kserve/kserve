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

package reconcilers

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func isvcWithLabels(labels map[string]string) *v1beta1.InferenceService {
	return &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model",
			Namespace: "test",
			Labels:    labels,
		},
	}
}

func TestIsvcNamespaceCachePredicates(t *testing.T) {
	pred := isvcNamespaceCachePredicates()
	labels := func(cache, ns string) map[string]string {
		return map[string]string{
			constants.LocalModelLabel:          cache,
			constants.LocalModelNamespaceLabel: ns,
		}
	}

	t.Run("update passes when the cache name changes within the same namespace", func(t *testing.T) {
		e := event.UpdateEvent{
			ObjectOld: isvcWithLabels(labels("cache-a", "test")),
			ObjectNew: isvcWithLabels(labels("cache-b", "test")),
		}
		if !pred.UpdateFunc(e) {
			t.Error("expected a cache-name relabel (storageUri replacement) to enqueue a reconcile")
		}
	})

	t.Run("update passes when the cache namespace label changes", func(t *testing.T) {
		e := event.UpdateEvent{
			ObjectOld: isvcWithLabels(labels("cache-a", "test")),
			ObjectNew: isvcWithLabels(labels("cache-a", "other")),
		}
		if !pred.UpdateFunc(e) {
			t.Error("expected a cache-namespace label change to enqueue a reconcile")
		}
	})

	t.Run("update drops an unchanged ISVC", func(t *testing.T) {
		e := event.UpdateEvent{
			ObjectOld: isvcWithLabels(labels("cache-a", "test")),
			ObjectNew: isvcWithLabels(labels("cache-a", "test")),
		}
		if pred.UpdateFunc(e) {
			t.Error("expected an update with unchanged cache labels to be dropped")
		}
	})

	t.Run("update drops an ISVC leaving the cache", func(t *testing.T) {
		e := event.UpdateEvent{
			ObjectOld: isvcWithLabels(labels("cache-a", "test")),
			ObjectNew: isvcWithLabels(map[string]string{}),
		}
		if !pred.UpdateFunc(e) {
			t.Error("expected a label removal to enqueue a reconcile so the old cache drops the ISVC")
		}
	})

	t.Run("create passes only for labelled ISVCs", func(t *testing.T) {
		if !pred.CreateFunc(event.CreateEvent{Object: isvcWithLabels(labels("cache-a", "test"))}) {
			t.Error("expected create with cache labels to pass")
		}
		if pred.CreateFunc(event.CreateEvent{Object: isvcWithLabels(map[string]string{})}) {
			t.Error("expected create without the namespace cache label to be dropped")
		}
	})

	t.Run("delete passes only for labelled ISVCs", func(t *testing.T) {
		if !pred.DeleteFunc(event.DeleteEvent{Object: isvcWithLabels(labels("cache-a", "test"))}) {
			t.Error("expected delete with cache labels to pass")
		}
		if pred.DeleteFunc(event.DeleteEvent{Object: isvcWithLabels(map[string]string{})}) {
			t.Error("expected delete without the namespace cache label to be dropped")
		}
	})
}
