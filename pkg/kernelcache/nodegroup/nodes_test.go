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

package nodegroup

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func makeNode(name string, labels map[string]string, ready bool) corev1.Node {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: status}},
		},
	}
}

func newFakeClient(t *testing.T, nodes ...corev1.Node) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("register corev1 scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("register v1alpha1 scheme: %v", err)
	}
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range nodes {
		builder = builder.WithObjects(&nodes[i])
	}
	return builder
}

func TestGetNodesSelectsOnlyMatchingLabels(t *testing.T) {
	nodes := []corev1.Node{
		makeNode("gpu-a", map[string]string{"role": "gpu", "zone": "east"}, true),
		makeNode("gpu-b", map[string]string{"role": "gpu", "zone": "east"}, false),
		makeNode("cpu-a", map[string]string{"role": "cpu", "zone": "east"}, true),
	}
	c := newFakeClient(t, nodes...).Build()

	group := &v1alpha1.KernelCacheNodeGroup{
		Spec: v1alpha1.KernelCacheNodeGroupSpec{
			NodeSelector: map[string]string{"role": "gpu"},
		},
	}
	ready, notReady, err := GetNodes(context.Background(), group, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ready.Items) != 1 || ready.Items[0].Name != "gpu-a" {
		t.Fatalf("unexpected ready nodes: %+v", ready.Items)
	}
	if len(notReady.Items) != 1 || notReady.Items[0].Name != "gpu-b" {
		t.Fatalf("unexpected notReady nodes: %+v", notReady.Items)
	}
}

func TestGetNodesRejectsEmptySelector(t *testing.T) {
	c := newFakeClient(t).Build()
	group := &v1alpha1.KernelCacheNodeGroup{}
	if _, _, err := GetNodes(context.Background(), group, c); err == nil {
		t.Fatalf("expected error for empty selector")
	}
}

func TestGetNodesRejectsNilGroup(t *testing.T) {
	c := newFakeClient(t).Build()
	if _, _, err := GetNodes(context.Background(), nil, c); err == nil {
		t.Fatalf("expected error for nil group")
	}
}

func TestGetNodesRequiresAllSelectorLabels(t *testing.T) {
	nodes := []corev1.Node{
		makeNode("partial", map[string]string{"role": "gpu"}, true),
		makeNode("full", map[string]string{"role": "gpu", "zone": "east"}, true),
	}
	c := newFakeClient(t, nodes...).Build()
	group := &v1alpha1.KernelCacheNodeGroup{
		Spec: v1alpha1.KernelCacheNodeGroupSpec{
			NodeSelector: map[string]string{"role": "gpu", "zone": "east"},
		},
	}
	ready, _, err := GetNodes(context.Background(), group, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ready.Items) != 1 || ready.Items[0].Name != "full" {
		t.Fatalf("expected only full node to match: %+v", ready.Items)
	}
}

func TestMatchingGroups(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"role": "gpu", "zone": "east"}}}
	groups := []v1alpha1.KernelCacheNodeGroup{
		{ObjectMeta: metav1.ObjectMeta{Name: "gpu-oci"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "gpu-zone"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{
			NodeSelector: map[string]string{"role": "gpu", "zone": "east"},
		}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cpu"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "cpu"}}},
	}

	matches := MatchingGroups(node, groups)
	if len(matches) != 2 || matches[0].Name != "gpu-oci" || matches[1].Name != "gpu-zone" {
		t.Fatalf("unexpected matches: %#v", matches)
	}
}
