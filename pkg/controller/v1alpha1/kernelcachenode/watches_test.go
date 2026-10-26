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

package kernelcachenode

import (
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	kernelcachelabels "github.com/kserve/kserve/pkg/kernelcache/labels"
)

func TestCurrentNodeReadinessPredicate(t *testing.T) {
	reconciler := &KernelCacheNodeReconciler{NodeName: "gpu-node"}
	pred := reconciler.currentNodeReadinessPredicate()

	readyNode := func(name string, ready bool) *corev1.Node {
		status := corev1.ConditionFalse
		if ready {
			status = corev1.ConditionTrue
		}
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: status}}},
		}
	}

	if !pred.Update(event.UpdateEvent{ObjectOld: readyNode("gpu-node", false), ObjectNew: readyNode("gpu-node", true)}) {
		t.Fatal("expected NotReady to Ready transition to enqueue the node")
	}
	if !pred.Update(event.UpdateEvent{ObjectOld: readyNode("gpu-node", true), ObjectNew: readyNode("gpu-node", false)}) {
		t.Fatal("expected Ready to NotReady transition to enqueue the node")
	}
	if pred.Update(event.UpdateEvent{ObjectOld: readyNode("gpu-node", true), ObjectNew: readyNode("gpu-node", true)}) {
		t.Fatal("did not expect an enqueue for an unchanged readiness state")
	}
	oldLabeledNode := readyNode("gpu-node", true)
	newLabeledNode := readyNode("gpu-node", true)
	oldLabeledNode.Labels = map[string]string{"node-group": "gpu"}
	newLabeledNode.Labels = map[string]string{"node-group": "cpu"}
	if !pred.Update(event.UpdateEvent{ObjectOld: oldLabeledNode, ObjectNew: newLabeledNode}) {
		t.Fatal("expected a label change to enqueue the node")
	}
	if pred.Update(event.UpdateEvent{ObjectOld: readyNode("other-node", false), ObjectNew: readyNode("other-node", true)}) {
		t.Fatal("did not expect another node to enqueue this agent")
	}
}

func TestCurrentNodePredicateIgnoresStatusOnlyUpdates(t *testing.T) {
	reconciler := &KernelCacheNodeReconciler{NodeName: "gpu-node"}
	pred := reconciler.currentNodePredicate()

	currentNode := func(name string, generation int64) *v1alpha1.KernelCacheNode {
		return &v1alpha1.KernelCacheNode{ObjectMeta: metav1.ObjectMeta{Name: name, Generation: generation}}
	}

	if !pred.Create(event.CreateEvent{Object: currentNode("gpu-node", 1)}) {
		t.Fatal("expected the current node create event to enqueue the node")
	}
	if pred.Create(event.CreateEvent{Object: currentNode("other-node", 1)}) {
		t.Fatal("did not expect another node create event to enqueue this agent")
	}

	oldNode := currentNode("gpu-node", 1)
	newNode := currentNode("gpu-node", 1)
	newNode.Status.Counts = &v1alpha1.KernelCacheNodeCounts{CachesReady: 1}
	if pred.Update(event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}) {
		t.Fatal("did not expect a status-only update to enqueue the node")
	}

	newNode = currentNode("gpu-node", 2)
	if !pred.Update(event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}) {
		t.Fatal("expected a generation change to enqueue the node")
	}

	newNode = currentNode("other-node", 2)
	if pred.Update(event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}) {
		t.Fatal("did not expect another node generation change to enqueue this agent")
	}
}

func TestCurrentJobPredicate(t *testing.T) {
	reconciler := &KernelCacheNodeReconciler{NodeName: "gpu-node"}
	pred := reconciler.currentJobPredicate()

	job := func(namespace string, labels map[string]string) *batchv1.Job {
		return &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Labels: labels}}
	}
	relevantLabels := map[string]string{
		kernelCacheNameLabel:      "cache",
		kernelCacheNamespaceLabel: "team-a",
		kernelCacheNodeLabel:      "gpu-node",
	}

	if !pred.Create(event.CreateEvent{Object: job("kernel-cache-jobs", relevantLabels)}) {
		t.Fatal("expected the configured node Job to enqueue the node")
	}
	if !pred.Create(event.CreateEvent{Object: job("other-jobs", relevantLabels)}) {
		t.Fatal("expected a labeled Job to pass the predicate before namespace resolution")
	}
	missingNodeLabel := map[string]string{
		kernelCacheNameLabel:      "cache",
		kernelCacheNamespaceLabel: "team-a",
	}
	if pred.Create(event.CreateEvent{Object: job("kernel-cache-jobs", missingNodeLabel)}) {
		t.Fatal("did not expect a Job without a node label to enqueue this node")
	}
	if pred.Create(event.CreateEvent{Object: job("kernel-cache-jobs", map[string]string{})}) {
		t.Fatal("did not expect an unrelated Job to enqueue the node")
	}
	wrongNodeLabels := map[string]string{
		kernelCacheNameLabel:      "cache",
		kernelCacheNamespaceLabel: "team-a",
		kernelCacheNodeLabel:      "other-node",
	}
	if pred.Create(event.CreateEvent{Object: job("kernel-cache-jobs", wrongNodeLabels)}) {
		t.Fatal("did not expect another node's Job to enqueue this node")
	}
}

func TestCurrentJobPredicateMatchesHashedLongNodeName(t *testing.T) {
	nodeName := "gpu-node-" + strings.Repeat("n", 70)
	reconciler := &KernelCacheNodeReconciler{NodeName: nodeName}
	pred := reconciler.currentJobPredicate()

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		kernelCacheNameLabel:      "cache",
		kernelCacheNamespaceLabel: "team-a",
		kernelCacheNodeLabel:      kernelcachelabels.Value(nodeName),
	}}}
	if !pred.Create(event.CreateEvent{Object: job}) {
		t.Fatal("expected a Job with the hashed node label to enqueue this node")
	}
}

func TestEnqueueCurrentNodeForJobUsesConfiguredNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{"kernelcache": `{"jobNamespace":"kernel-cache-jobs"}`},
	}
	reconciler := &KernelCacheNodeReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build(),
		NodeName: "gpu-node",
	}
	labels := map[string]string{
		kernelCacheNameLabel:      "cache",
		kernelCacheNamespaceLabel: "team-a",
		kernelCacheNodeLabel:      "gpu-node",
	}
	job := func(namespace string) *batchv1.Job {
		return &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Labels: labels}}
	}

	if requests := reconciler.enqueueCurrentNodeForJob(t.Context(), job("kernel-cache-jobs")); len(requests) != 1 {
		t.Fatalf("expected one request for the configured Job namespace, got %#v", requests)
	}
	if requests := reconciler.enqueueCurrentNodeForJob(t.Context(), job("other-jobs")); len(requests) != 0 {
		t.Fatalf("did not expect a request for another Job namespace, got %#v", requests)
	}
}

func TestInferenceServiceConfigMapPredicate(t *testing.T) {
	reconciler := &KernelCacheNodeReconciler{}
	pred := reconciler.inferenceServiceConfigMapPredicate()

	configMap := func(namespace, name string) *corev1.ConfigMap {
		return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}}
	}
	if !pred.Create(event.CreateEvent{Object: configMap(constants.KServeNamespace, constants.InferenceServiceConfigMapName)}) {
		t.Fatal("expected the InferenceService ConfigMap to enqueue the node")
	}
	if pred.Create(event.CreateEvent{Object: configMap("other", constants.InferenceServiceConfigMapName)}) {
		t.Fatal("did not expect a ConfigMap from another namespace to enqueue the node")
	}
	if pred.Create(event.CreateEvent{Object: configMap(constants.KServeNamespace, "other")}) {
		t.Fatal("did not expect another ConfigMap to enqueue the node")
	}
}

func TestCurrentPodPredicate(t *testing.T) {
	reconciler := &KernelCacheNodeReconciler{}
	pred := reconciler.currentPodPredicate()

	withISVCLabel := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{constants.InferenceServicePodLabelKey: "model"},
	}}
	withoutISVCLabel := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{kernelCacheNameLabel: "cache"},
	}}

	if !pred.Create(event.CreateEvent{Object: withISVCLabel}) {
		t.Fatal("expected an InferenceService Pod to be watched")
	}
	if pred.Create(event.CreateEvent{Object: withoutISVCLabel}) {
		t.Fatal("did not expect a Pod without an InferenceService label to be watched")
	}
}
